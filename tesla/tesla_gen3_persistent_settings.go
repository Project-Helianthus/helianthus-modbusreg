package tesla

import (
	"context"
	"fmt"
	"math"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// Gen3PersistentSettingsRequest carries only current on the wire. Identity
// and Deadline bind the local attempt and are never encoded.
type Gen3PersistentSettingsRequest struct {
	MaxOutputCurrentAmps int32
	OperationIdentity    string
	Deadline             time.Time
}

// Gen3PersistentSettingsResult reports returned settings, not an applied
// current or an acknowledgement. A duplicate is always historical.
type Gen3PersistentSettingsResult struct {
	Deadline                                                  time.Time
	Requested                                                 Gen3PersistentSettingsRequest
	ReturnedMaxOutputCurrentAmps                              int32
	HasReturnedCurrent                                        bool
	Outcome                                                   Gen3ReadEvidenceState
	Comparison                                                CurrentLimitComparison
	Receipt                                                   Gen3ReadRecord
	Executed, Duplicate, Current, NoSend, PossiblyTransmitted bool
	Reason                                                    string
}

type teslaPersistentAttempt struct {
	input    Gen3PersistentSettingsRequest
	result   Gen3PersistentSettingsResult
	finished time.Time
}

// EncodeTeslaPersistentSettingsBody encodes only WCSettings field 1
// (TEDAPI Specification §§6.4.4 and 6.6.2). It does not qualify mutation scope.
func EncodeTeslaPersistentSettingsBody(amps int32) []byte {
	settings := appendTeslaFC100Varint([]byte{8}, uint64(int64(amps)))
	return append([]byte{10, byte(len(settings))}, settings...)
}

func decodeTeslaPersistentSettingsBody(body []byte) (int32, bool, error) {
	outer, err := decodeTeslaGen3CurrentLimitFields(body, map[uint64]uint8{1: 2})
	if err != nil {
		return 0, false, err
	}
	scalars, err := decodeTeslaGen3CurrentLimitFields(outer[1].bytes, map[uint64]uint8{1: 0})
	if err != nil {
		return 0, false, err
	}
	value, present := scalars[1]
	if !present {
		return 0, false, nil
	}
	if value.varint > math.MaxInt32 && value.varint < uint64(math.MaxUint64)-math.MaxInt32 {
		return 0, false, fmt.Errorf("Gen3 settings current is not an int32")
	}
	return int32(value.varint), true, nil
}

func (result Gen3PersistentSettingsResult) reject(err error) (Gen3PersistentSettingsResult, error) {
	result.Outcome, result.NoSend, result.Reason = Gen3ReadEvidenceRejected, true, err.Error()
	return result, err
}

// ConfigurePersistentCurrent sends a current-only t7 once and retains the
// correlated t8, using the same reservation as polling and provisional limits.
func (acquisition *Gen3ReadAcquisition) ConfigurePersistentCurrent(parent context.Context, input Gen3PersistentSettingsRequest) (Gen3PersistentSettingsResult, error) {
	result := Gen3PersistentSettingsResult{Requested: input, Comparison: CurrentLimitUnknown}
	noSend := func(reason string) (Gen3PersistentSettingsResult, error) {
		return result.reject(fmt.Errorf("%w: %s", modbusreg.ErrQualifiedFunctionNoSend, reason))
	}
	if parent == nil || !modbusreg.ValidIdentity(input.OperationIdentity) {
		return noSend("invalid persistent operation identity or context")
	}
	if err := acquisition.reserve(); err != nil {
		return result.reject(err)
	}
	defer acquisition.release()
	now := acquisition.config.Now()
	window, hasWindow := acquisition.windows[FC100OperationWCConfigureSettings]
	acquisition.expirePersistentReceipts(now, window.Retention)
	if previous, ok := acquisition.persistentAttempts[input.OperationIdentity]; ok {
		if input.MaxOutputCurrentAmps != previous.input.MaxOutputCurrentAmps || !input.Deadline.Equal(previous.input.Deadline) {
			return noSend("persistent identity has different intent")
		}
		if previous.result.Duplicate {
			result.Duplicate, result.NoSend, result.Outcome, result.Reason = true, true, previous.result.Outcome, "historical receipt expired"
			return result, nil
		}
		result = previous.result
		result.Executed, result.Duplicate, result.Current = false, true, false
		return result, nil
	}
	started := time.Now()
	remaining := input.Deadline.Sub(started)
	if parent.Err() != nil || input.Deadline.IsZero() || remaining <= 0 || remaining > acquisition.config.AcquisitionDeadline {
		return noSend("persistent deadline is missing, expired, or outside the acquisition budget")
	}
	if acquisition.State() != Gen3ReadLifecycleReady {
		return result.reject(ErrTeslaGen3ReadAcquisitionUnavailable)
	}
	gen3 := acquisition.config.Gen3
	if !gen3.documentedCurrentLimitLayout() || gen3.config.PersistentSettingsOperationVersion != HSCFC100OperationCompatibilityV1 || !gen3.config.PersistentCurrentOnlyQualified {
		return noSend("t7/t8 schema or current-only scope not qualified for this build")
	}
	if !hasWindow || len(acquisition.persistentAttempts) >= acquisition.config.PersistentIdentityCapacity {
		return noSend("persistent retention capacity unavailable")
	}
	ctx, cancel := context.WithDeadline(parent, started.Add(remaining))
	defer cancel()
	result.Deadline, _ = ctx.Deadline()
	request, err := BuildTeslaFC100OperationRequest(HSCFC100OperationCompatibilityV1, FC100OperationWCConfigureSettings, EncodeTeslaPersistentSettingsBody(input.MaxOutputCurrentAmps))
	if err != nil {
		return result.reject(err)
	}
	result.Receipt = acquisition.mutationExchange(ctx, FC100OperationWCConfigureSettings, request, modbus.DefaultPrivateFunctionResponsePolicy())
	result.Outcome, result.Reason = result.Receipt.State, result.Receipt.Cause
	result.PossiblyTransmitted = result.Receipt.RequestID != 0
	result.Executed = result.PossiblyTransmitted
	if result.Outcome == Gen3ReadEvidenceNoSend {
		result.Outcome, result.NoSend = Gen3ReadEvidenceRejected, true
		return result, nil
	}
	if result.Outcome == Gen3ReadEvidenceAck {
		result.Current = true
		result.ReturnedMaxOutputCurrentAmps, result.HasReturnedCurrent = result.Receipt.persistentCurrent, result.Receipt.hasPersistentCurrent
		if result.HasReturnedCurrent {
			result.Comparison = CurrentLimitMismatch
			if result.ReturnedMaxOutputCurrentAmps == input.MaxOutputCurrentAmps {
				result.Comparison = CurrentLimitMatch
			}
		}
	}
	acquisition.persistentAttempts[input.OperationIdentity] = teslaPersistentAttempt{input, result, acquisition.config.Now()}
	return result, nil
}

func (acquisition *Gen3ReadAcquisition) expirePersistentReceipts(now time.Time, retention time.Duration) {
	for id, attempt := range acquisition.persistentAttempts {
		if !attempt.result.Duplicate && now.Sub(attempt.finished) > retention {
			attempt.result = Gen3PersistentSettingsResult{Outcome: attempt.result.Outcome, Duplicate: true}
			acquisition.persistentAttempts[id] = attempt
		}
	}
}
