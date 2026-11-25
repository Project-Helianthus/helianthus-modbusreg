package tesla

import (
	"bytes"
	"context"
	"errors"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// This file holds the per-exchange half of the Gen3 read acquisition
// lifecycle: the single-attempt exchange and its classification, recovery,
// publication and retention of records, and the private-function admission
// decision for the owned endpoint.

// exchange performs one single-attempt production exchange and classifies it.
func (acquisition *Gen3ReadAcquisition) exchange(
	ctx context.Context,
	endpoint Gen3ReadEndpoint,
	operation FC100Operation,
	request modbus.PrivateFunctionRequest,
	policy modbus.PrivateFunctionResponsePolicy,
	attempt int,
) (Gen3ReadRecord, teslaGen3ReadFollowUp) {
	spec := acquisition.exchangeSpec(operation)
	acquisition.mu.Lock()
	epoch := acquisition.epoch
	instance := acquisition.instance
	acquisition.plan = &teslaGen3ReadPlan{instance: instance, payload: request.Payload()}
	acquisition.mu.Unlock()
	responses, evidence, err := endpoint.ExchangePrivateFunction(
		ctx, acquisition.config.Profile.Node(), request, policy, teslaGen3ReadClassifier{spec: spec, request: request.Payload()})
	acquisition.mu.Lock()
	acquisition.plan = nil
	acquisition.mu.Unlock()

	grade, _ := acquisition.config.Gen3.readApplicability()
	record := Gen3ReadRecord{
		ReadApplicabilityGrade: grade,
		Endpoint:               acquisition.config.Endpoint, Node: acquisition.config.Profile.Node(), Operation: operation,
		Family: spec.family, RequestTag: spec.requestTag, TerminalTag: spec.responseTag,
		Profile: HSCProfileName, ProfileVersion: HSCCompatibilityV1,
		OperationVersion: HSCFC100OperationCompatibilityV1,
		Gen3Version:      acquisition.config.Gen3.Version(), Gen3VersionDisposition: acquisition.config.Gen3.VersionDisposition(),
		Source: Gen3ReadSourceRTUPrivateFunction, LifecycleEpoch: epoch,
		EndpointInstance: instance, Generation: evidence.Generation, RequestID: evidence.RequestID,
		Attempt: attempt, MaxAttempts: acquisition.config.MaxAttempts,
		ObservedAt: acquisition.config.Now(), ReceiptWall: evidence.ReceiptWall,
		SentAt: evidence.SentAt, ReceivedAt: evidence.ReceivedAt, ClockEpoch: evidence.ClockEpoch,
		TransportOutcome: evidence.TerminalOutcome, IntegrityValid: evidence.IntegrityValid,
		requestADU: cloneTeslaGen3Bytes(evidence.RequestADU),
	}
	if evidence.ResponseADUs != nil {
		record.responseADUs = make([][]byte, len(evidence.ResponseADUs))
		for index, frame := range evidence.ResponseADUs {
			record.responseADUs[index] = cloneTeslaGen3Bytes(frame)
		}
	}
	if err != nil {
		record.Cause = err.Error()
	}
	switch {
	case err != nil && evidence.RequestID == 0:
		record.State = Gen3ReadEvidenceNoSend
		return record, teslaGen3ReadFollowUpNone
	case err != nil && evidence.TerminalOutcome == "exception":
		record.State = Gen3ReadEvidenceRejected
		if frames := evidence.ResponseADUs; len(frames) != 0 {
			if last := frames[len(frames)-1]; len(last) == 5 && last[1] == request.FunctionCode().Byte()|0x80 {
				record.HasExceptionStatus = true
				record.ExceptionStatus = last[2]
			}
		}
		return record, teslaGen3ReadFollowUpNone
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		record.State = Gen3ReadEvidenceTimeout
		return record, teslaGen3ReadFollowUpRecover
	case err != nil:
		record.State = Gen3ReadEvidenceIndeterminate
		return record, teslaGen3ReadFollowUpRecover
	}
	// A receipt that was not current at the transport never carries a value.
	// Ownership of the endpoint and epoch is checked again when the record
	// is published (see publish), under the lifecycle mutex.
	if !evidence.Current {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = "receipt is not current for the owned endpoint epoch"
		return record, teslaGen3ReadFollowUpNone
	}
	payloads := make([][]byte, len(responses))
	for index, response := range responses {
		payloads[index] = response.Payload()
	}
	results, decodeErr := decodeTeslaOperationSequence(spec, request.Payload(), payloads)
	if decodeErr != nil {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = decodeErr.Error()
		return record, teslaGen3ReadFollowUpRetire
	}
	record.results = results
	record.State = Gen3ReadEvidenceAck
	for _, result := range results {
		if result.Kind == FC100OperationApplicationError {
			record.State = Gen3ReadEvidenceRejected
			record.HasApplicationStatus = true
			record.ApplicationStatus = result.Status
		}
	}
	return classifyTeslaCurrentLimitTerminal(record)
}

func classifyTeslaCurrentLimitTerminal(record Gen3ReadRecord) (Gen3ReadRecord, teslaGen3ReadFollowUp) {
	if record.State != Gen3ReadEvidenceAck {
		return record, teslaGen3ReadFollowUpNone
	}
	operation := record.Operation
	if operation == FC100OperationWCGetProvisional {
		readback, err := decodeTeslaProvisionalReadback(record.TerminalBody())
		if err != nil {
			record.State, record.Cause = Gen3ReadEvidenceIndeterminate, err.Error()
			return record, teslaGen3ReadFollowUpRetire
		}
		record.provisionalReadback = readback
	}
	if operation == FC100OperationWCSetProvisional {
		if err := validateTeslaProvisionalTerminal(record); err != nil {
			record.State, record.Cause = Gen3ReadEvidenceIndeterminate, err.Error()
			return record, teslaGen3ReadFollowUpRetire
		}
	}
	if operation == FC100OperationWCConfigureSettings {
		amps, present, err := decodeTeslaPersistentSettingsBody(record.TerminalBody())
		if err != nil {
			record.State, record.Cause = Gen3ReadEvidenceIndeterminate, err.Error()
			return record, teslaGen3ReadFollowUpRetire
		}
		record.persistentCurrent, record.hasPersistentCurrent = amps, present
	}
	return record, teslaGen3ReadFollowUpNone
}

// recover runs the transport's bounded recovery. Success returns to ready in
// the same epoch and clears the pinned generation so the next admitted
// exchange pins the successor generation of this endpoint instance. The
// activation driver is not called. Failure leaves the lifecycle unavailable.
func (acquisition *Gen3ReadAcquisition) recover(ctx context.Context, endpoint Gen3ReadEndpoint) bool {
	acquisition.mu.Lock()
	if acquisition.closed || acquisition.endpoint != endpoint {
		acquisition.mu.Unlock()
		return false
	}
	acquisition.state = Gen3ReadLifecycleRecovering
	acquisition.mu.Unlock()
	// Recovery stays inside the acquisition budget. An expired or cancelled
	// acquisition leaves the endpoint fenced and the lifecycle unavailable;
	// Restart is the bounded way back.
	if ctx.Err() != nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return false
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, acquisition.config.ResponseTimeout+acquisition.config.Timing.InterFrame())
	defer cancel()
	if err := endpoint.Recover(recoveryCtx); err != nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return false
	}
	acquisition.mu.Lock()
	closed := acquisition.closed
	owned := acquisition.endpoint == endpoint
	if !closed && owned {
		acquisition.lastGeneration = acquisition.epochGeneration
		acquisition.epochGeneration = 0
		acquisition.readbackSeen = 0
		acquisition.state = Gen3ReadLifecycleReady
		acquisition.mu.Unlock()
		return true
	}
	acquisition.mu.Unlock()
	if !closed {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
	}
	return false
}

// publish linearizes the final ownership check, ACK promotion, and retention
// with Close and endpoint retirement: all three run in one critical section
// of the lifecycle mutex, which Close also takes. When the lifecycle no longer
// owns the endpoint and epoch the record was acquired in, an ACK is downgraded
// to indeterminate with its frames preserved and decoded values cleared.
func (acquisition *Gen3ReadAcquisition) publish(record Gen3ReadRecord, endpoint Gen3ReadEndpoint) Gen3ReadRecord {
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if record.State == Gen3ReadEvidenceAck &&
		(acquisition.endpoint != endpoint || !acquisition.recordCurrentLocked(record)) {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = "receipt is not current for the owned endpoint epoch"
		record.results = nil
	}
	acquisition.retainLocked(record)
	return record.clone()
}

func (acquisition *Gen3ReadAcquisition) retainLocked(record Gen3ReadRecord) {
	entry := acquisition.retained[record.Operation]
	if entry == nil {
		return
	}
	entry.lastAttempt = record.clone()
	entry.hasLastAttempt = true
	if record.State == Gen3ReadEvidenceAck {
		if record.Operation == FC100OperationWCGetProvisional {
			acquisition.readbackSeen = record.Generation
		}
		entry.lastGood = record.clone()
		entry.hasLastGood = true
	}
}

// Retained returns the retained view of one operation. Records older than the
// operation's retention window are discarded and not returned.
func (acquisition *Gen3ReadAcquisition) Retained(operation FC100Operation) (Gen3ReadRetained, bool) {
	if acquisition == nil {
		return Gen3ReadRetained{}, false
	}
	window, ok := acquisition.windows[operation]
	if !ok {
		return Gen3ReadRetained{}, false
	}
	now := acquisition.config.Now()
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	entry := acquisition.retained[operation]
	if entry == nil {
		return Gen3ReadRetained{}, false
	}
	if entry.hasLastGood && now.Sub(entry.lastGood.ObservedAt) > window.Retention {
		entry.lastGood, entry.hasLastGood = Gen3ReadRecord{}, false
	}
	if entry.hasLastAttempt && now.Sub(entry.lastAttempt.ObservedAt) > window.Retention {
		entry.lastAttempt, entry.hasLastAttempt = Gen3ReadRecord{}, false
	}
	if !entry.hasLastGood && !entry.hasLastAttempt {
		return Gen3ReadRetained{}, false
	}
	view := Gen3ReadRetained{
		LastGood: entry.lastGood.clone(), HasLastGood: entry.hasLastGood,
		LastAttempt: entry.lastAttempt.clone(), HasLastAttempt: entry.hasLastAttempt,
	}
	if entry.hasLastGood {
		age := now.Sub(entry.lastGood.ObservedAt)
		view.Fresh = age >= 0 && age <= window.Freshness
		view.Current = acquisition.recordCurrentLocked(entry.lastGood)
	}
	return view, true
}

// recordCurrentLocked reports whether a retained record is current. The caller
// holds the lifecycle mutex. Publish uses the same comparison.
func (acquisition *Gen3ReadAcquisition) recordCurrentLocked(record Gen3ReadRecord) bool {
	return !acquisition.closed && acquisition.state == Gen3ReadLifecycleReady &&
		record.LifecycleEpoch == acquisition.epoch &&
		record.EndpointInstance == acquisition.instance &&
		record.Generation == acquisition.epochGeneration &&
		acquisition.epochGeneration != 0
}

// admit is the private-function admission decision for the owned endpoint.
func (acquisition *Gen3ReadAcquisition) admit(instance uint64, candidate modbus.RTUPrivateFunctionAdmissionRequest) bool {
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	plan := acquisition.plan
	if acquisition.closed || plan == nil ||
		instance != acquisition.instance || plan.instance != instance ||
		candidate.Endpoint != acquisition.config.Endpoint || candidate.UnitID != acquisition.config.Profile.Node() ||
		candidate.Function != teslaHSCFunction100 || !bytes.Equal(candidate.Payload, plan.payload) {
		return false
	}
	switch acquisition.state {
	case Gen3ReadLifecycleReady:
	case Gen3ReadLifecycleActivating:
		if !acquisition.activatingProbe {
			return false
		}
	default:
		return false
	}
	if acquisition.epochGeneration == 0 {
		if candidate.Generation <= acquisition.lastGeneration {
			return false
		}
		acquisition.epochGeneration = candidate.Generation
		return true
	}
	return candidate.Generation == acquisition.epochGeneration
}
