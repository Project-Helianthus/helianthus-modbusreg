package tesla

import (
	"context"
	"fmt"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// TEDAPIOperationWCProvisionalReadback selects the empty t27 read.
const TEDAPIOperationWCProvisionalReadback = "tesla.wc_get_provisional.v1"

// ProvisionalCurrentLimitReadback retains wire presence separately from numeric values.
type ProvisionalCurrentLimitReadback struct {
	LimitCurrentMaxAmps                                                uint32
	LimitTimeoutSeconds                                                uint32
	InhibitCharging                                                    bool
	ConfiguredCurrentAmps                                              uint32
	HasLimitCurrent, HasLimitTimeout, HasInhibit, HasConfiguredCurrent bool
}

// ProvisionalReadback returns the typed data of an accepted t28, by value.
func (record Gen3ReadRecord) ProvisionalReadback() (ProvisionalCurrentLimitReadback, bool) {
	if record.Operation != FC100OperationWCGetProvisional || record.State != Gen3ReadEvidenceAck {
		return ProvisionalCurrentLimitReadback{}, false
	}
	return record.provisionalReadback, true
}

func (config Gen3ReadAcquisitionConfig) provisionalReadbackAdmitted() bool {
	return config.Profile.OperationAdmissionFor(TEDAPIOperationWCProvisionalReadback).OutboundAllowed
}

func validTeslaReadWindow(window Gen3ReadWindow) bool {
	return window.Freshness > 0 && window.Retention >= window.Freshness && window.Retention <= maxTeslaGen3ReadRetention
}

// ProvisionalCurrentLimitRequest is either a 900-second positive limit or a
// pause (0 A). A pause ignores the caller's timeout and inhibit values.
type ProvisionalCurrentLimitRequest struct {
	LimitCurrentMaxAmps uint32
	LimitTimeoutSeconds uint32
	InhibitCharging     bool
}

// CurrentLimitComparison compares reported state with encoded intent.
type CurrentLimitComparison string

// Comparison outcomes keep missing scalars distinct from disagreement.
const (
	CurrentLimitUnknown  CurrentLimitComparison = "unknown"
	CurrentLimitMatch    CurrentLimitComparison = "match"
	CurrentLimitMismatch CurrentLimitComparison = "mismatch"
)

// ProvisionalCurrentLimitResult separates caller intent, encoded values,
// and the immutable phase receipts. Bound may refer to an earlier t28.
// On the documented path, Bound and Prerequisite hold the installation query.
type ProvisionalCurrentLimitResult struct {
	Requested                                                      ProvisionalCurrentLimitRequest
	Encoded                                                        ProvisionalCurrentLimitRequest
	HasEncoded                                                     bool
	Deadline                                                       time.Time
	Outcome                                                        Gen3ReadEvidenceState
	Comparison                                                     CurrentLimitComparison
	NoSend, PossiblyTransmitted, EffectIndeterminate, Acknowledged bool
	Reason                                                         string
	Bound, Prerequisite, Set, Readback                             Gen3ReadRecord
	Validated                                                      Gen3ProvisionalCurrentLimit
	HasValidated                                                   bool
}

// EncodeTeslaProvisionalCurrentLimitBody encodes the t25 request body
// within the native setter's non-release range. It does not admit a write.
func EncodeTeslaProvisionalCurrentLimitBody(input ProvisionalCurrentLimitRequest) ([]byte, error) {
	if _, err := NewGen3InteroperableCurrentLimit(input.LimitCurrentMaxAmps, input.LimitTimeoutSeconds); err != nil {
		return nil, err
	}
	body := appendTeslaFC100Varint([]byte{8}, uint64(input.LimitCurrentMaxAmps))
	body = appendTeslaFC100Varint(append(body, 16), uint64(input.LimitTimeoutSeconds))
	var inhibit byte
	if input.InhibitCharging {
		inhibit = 1
	}
	body = append(body, 24, inhibit)
	return append([]byte{10, byte(len(body))}, body...), nil
}

func (result ProvisionalCurrentLimitResult) reject(reason error) (ProvisionalCurrentLimitResult, error) {
	result.Outcome, result.NoSend, result.Reason = Gen3ReadEvidenceRejected, true, reason.Error()
	return result, reason
}

// SetProvisionalCurrentLimit performs a firmware-scoped set and same-generation
// readback. The baseline compatibility path requires explicit qualification.
func (acquisition *Gen3ReadAcquisition) SetProvisionalCurrentLimit(parent context.Context, input ProvisionalCurrentLimitRequest) (ProvisionalCurrentLimitResult, error) {
	result := ProvisionalCurrentLimitResult{Requested: input, Comparison: CurrentLimitUnknown}
	if parent == nil {
		return result.reject(modbusreg.ErrQualifiedFunctionNoSend)
	}
	if err := acquisition.reserve(); err != nil {
		return result.reject(err)
	}
	defer acquisition.release()
	if acquisition.State() != Gen3ReadLifecycleReady {
		return result.reject(ErrTeslaGen3ReadAcquisitionUnavailable)
	}
	gen3 := acquisition.config.Gen3
	firmware, ok := teslaReadFirmware(gen3.Version())
	documented := gen3.documentedCurrentLimitLayout()
	if !documented && (!ok || firmware != teslaReadFloor || !gen3.config.ProvisionalSetQualified) {
		return result.reject(fmt.Errorf("%w: t25/t26 capability not qualified for this build", modbusreg.ErrQualifiedFunctionNoSend))
	}
	if _, ok := acquisition.windows[FC100OperationWCGetProvisional]; !ok {
		return result.reject(fmt.Errorf("%w: t27/t28 capability or window missing", modbusreg.ErrQualifiedFunctionNoSend))
	}
	if input.LimitCurrentMaxAmps == 0 {
		input = ProvisionalCurrentLimitRequest{6, 900, true}
	} else if input.LimitTimeoutSeconds != 900 || input.InhibitCharging {
		return result.reject(fmt.Errorf("%w: positive limit requires 900 seconds and inhibit false", modbusreg.ErrQualifiedFunctionNoSend))
	}
	body, err := EncodeTeslaProvisionalCurrentLimitBody(input)
	if err != nil {
		return result.reject(err)
	}
	ctx, cancel := context.WithTimeout(parent, acquisition.config.AcquisitionDeadline)
	defer cancel()
	result.Deadline, _ = ctx.Deadline()
	if ctx.Err() != nil {
		return result.reject(ctx.Err())
	}
	if documented {
		return acquisition.applyDocumentedProvisionalLimit(ctx, input, body, result)
	}
	return acquisition.applyProvisionalLimit(ctx, input, body, result)
}

func (acquisition *Gen3ReadAcquisition) applyProvisionalLimit(ctx context.Context, input ProvisionalCurrentLimitRequest, body []byte, result ProvisionalCurrentLimitResult) (ProvisionalCurrentLimitResult, error) {
	retained, _ := acquisition.Retained(FC100OperationWCGetProvisional)
	acquisition.mu.Lock()
	seen := acquisition.readbackSeen == acquisition.epochGeneration && acquisition.epochGeneration != 0
	acquisition.mu.Unlock()
	if retained.Current {
		result.Bound = retained.LastGood
	}
	if !seen {
		result.Prerequisite = acquisition.mutationReadback(ctx)
		result.Bound = result.Prerequisite
		if result.Prerequisite.State == Gen3ReadEvidenceTimeout && result.Prerequisite.RequestID != 0 {
			result.Outcome, result.Reason = result.Prerequisite.State, result.Prerequisite.Cause
			result.PossiblyTransmitted, result.EffectIndeterminate = true, true
			return result, nil
		}
	} else if !retained.Current {
		return result.reject(fmt.Errorf("%w: latest t28 is not reusable", modbusreg.ErrQualifiedFunctionNoSend))
	}
	bound, ok := result.Bound.ProvisionalReadback()
	if !ok || !bound.HasConfiguredCurrent || bound.ConfiguredCurrentAmps == 0 || input.LimitCurrentMaxAmps > bound.ConfiguredCurrentAmps {
		return result.reject(fmt.Errorf("%w: latest t28 lacks a usable bound for the requested current", modbusreg.ErrQualifiedFunctionNoSend))
	}
	return acquisition.exchangeProvisionalSet(ctx, input, body, result)
}

func (acquisition *Gen3ReadAcquisition) exchangeProvisionalSet(ctx context.Context, input ProvisionalCurrentLimitRequest, body []byte, result ProvisionalCurrentLimitResult) (ProvisionalCurrentLimitResult, error) {
	request, err := BuildTeslaFC100OperationRequest(HSCFC100OperationCompatibilityV1, FC100OperationWCSetProvisional, body)
	if err != nil {
		return result.reject(err)
	}
	result.Encoded, result.HasEncoded = input, true
	result.Set = acquisition.mutationExchange(ctx, FC100OperationWCSetProvisional, request, modbus.DefaultPrivateFunctionResponsePolicy())
	result.Outcome, result.Reason = result.Set.State, result.Set.Cause
	result.PossiblyTransmitted = result.Set.RequestID != 0
	if result.Set.State == Gen3ReadEvidenceNoSend {
		result.Outcome, result.NoSend = Gen3ReadEvidenceRejected, true
		return result, nil
	}
	result.EffectIndeterminate = result.Set.State != Gen3ReadEvidenceRejected
	if result.Set.State != Gen3ReadEvidenceAck {
		return result, nil
	}
	result.Acknowledged = true
	result.Readback = acquisition.mutationReadback(ctx)
	result.Reason = result.Readback.Cause
	if result.Readback.State == Gen3ReadEvidenceTimeout || (result.Readback.State != Gen3ReadEvidenceAck && ctx.Err() == context.DeadlineExceeded) {
		result.Outcome = Gen3ReadEvidenceTimeout
		return result, nil
	}
	readback, ok := result.Readback.ProvisionalReadback()
	if !ok || !sameTeslaReceiptGeneration(result.Set, result.Readback) {
		result.Outcome = Gen3ReadEvidenceIndeterminate
		return result, nil
	}
	firmware, _ := teslaReadFirmware(acquisition.config.Gen3.Version())
	result.Comparison = compareTeslaProvisional(input, readback, firmware == [3]uint64{26, 34, 0})
	result.EffectIndeterminate = result.Comparison == CurrentLimitUnknown
	if result.Comparison == CurrentLimitMatch && result.Set.TerminalTag == 26 {
		result.Validated, err = NewGen3ProvisionalCurrentLimit(Gen3ProvisionalCurrentLimitSpec{
			OperationVersion:    HSCFC100OperationCompatibilityV1,
			LimitCurrentMaxAmps: input.LimitCurrentMaxAmps, LimitTimeoutSeconds: input.LimitTimeoutSeconds, InhibitCharging: input.InhibitCharging,
			SetRequestPayload: request.Payload(), AckPayload: result.Set.terminalPayload(),
			ReadbackRequestPayload: privateFunctionPayload(result.Readback.requestADU), ReadbackTerminalPayload: result.Readback.terminalPayload(),
		})
		result.HasValidated = err == nil
	}
	return result, nil
}

func compareTeslaProvisional(input ProvisionalCurrentLimitRequest, observed ProvisionalCurrentLimitReadback, countdown bool) CurrentLimitComparison {
	if !observed.HasLimitCurrent || !observed.HasLimitTimeout || !observed.HasInhibit {
		return CurrentLimitUnknown
	}
	timeoutDiffers := observed.LimitTimeoutSeconds != input.LimitTimeoutSeconds
	// TEDAPI Specification §7.1.1: on 26.34.0 t28 reports remaining time.
	if countdown {
		timeoutDiffers = observed.LimitTimeoutSeconds > input.LimitTimeoutSeconds
	}
	if observed.LimitCurrentMaxAmps != input.LimitCurrentMaxAmps || timeoutDiffers || observed.InhibitCharging != input.InhibitCharging {
		return CurrentLimitMismatch
	}
	return CurrentLimitMatch
}

func sameTeslaReceiptGeneration(a, b Gen3ReadRecord) bool {
	return a.Generation != 0 && a.Generation == b.Generation && a.EndpointInstance == b.EndpointInstance && a.LifecycleEpoch == b.LifecycleEpoch
}

func privateFunctionPayload(adu []byte) []byte {
	if len(adu) < 4 {
		return nil
	}
	return cloneTeslaGen3Bytes(adu[2 : len(adu)-2])
}

func (record Gen3ReadRecord) terminalPayload() []byte {
	if len(record.responseADUs) == 0 {
		return nil
	}
	return privateFunctionPayload(record.responseADUs[len(record.responseADUs)-1])
}

func (acquisition *Gen3ReadAcquisition) mutationReadback(ctx context.Context) Gen3ReadRecord {
	request, policy, err := acquisition.config.Profile.EncodeQualifiedFunction(TEDAPIOperationWCProvisionalReadback)
	if err != nil {
		return Gen3ReadRecord{Operation: FC100OperationWCGetProvisional, State: Gen3ReadEvidenceNoSend, Cause: err.Error()}
	}
	return acquisition.mutationExchange(ctx, FC100OperationWCGetProvisional, request, policy)
}

func (acquisition *Gen3ReadAcquisition) mutationExchange(ctx context.Context, operation FC100Operation, request modbus.PrivateFunctionRequest, policy modbus.PrivateFunctionResponsePolicy) Gen3ReadRecord {
	acquisition.mu.Lock()
	endpoint, state := acquisition.endpoint, acquisition.state
	acquisition.mu.Unlock()
	if ctx.Err() != nil {
		return Gen3ReadRecord{Operation: operation, State: Gen3ReadEvidenceNoSend, Cause: ctx.Err().Error()}
	}
	if endpoint == nil || state != Gen3ReadLifecycleReady {
		return Gen3ReadRecord{Operation: operation, State: Gen3ReadEvidenceNoSend, Cause: ErrTeslaGen3ReadAcquisitionUnavailable.Error()}
	}
	record, followUp := acquisition.exchange(ctx, endpoint, operation, request, policy, 1)
	record.MaxAttempts = 1
	if err := ctx.Err(); record.State == Gen3ReadEvidenceAck && err != nil {
		record.State, record.Cause = Gen3ReadEvidenceIndeterminate, err.Error()
		if err == context.DeadlineExceeded {
			record.State = Gen3ReadEvidenceTimeout
		}
		followUp = teslaGen3ReadFollowUpRetire
	}
	record = acquisition.publish(record, endpoint)
	if followUp != teslaGen3ReadFollowUpNone {
		acquisition.retireOwned(endpoint)
	}
	return record
}
