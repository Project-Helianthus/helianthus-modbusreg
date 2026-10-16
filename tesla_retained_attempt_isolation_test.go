package modbusreg

import (
	"context"
	"reflect"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// These tests enter the existing retention publication seam after acquisition.
// Ready epochs and decoded ACKs are synthetic preconditions, not activation or
// qualification evidence. Opaque bodies make no identity or value-field claim.
type k22NoIOEndpoint struct {
	t *testing.T
}

func (endpoint *k22NoIOEndpoint) ExchangePrivateFunction(context.Context, byte, modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, modbus.RTUPrivateFunctionResponseClassifier) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error) {
	endpoint.t.Fatal("retention test attempted an operation")
	return nil, modbus.RTUPrivateFunctionEvidence{}, nil
}

func (endpoint *k22NoIOEndpoint) Recover(context.Context) error {
	endpoint.t.Fatal("retention test attempted recovery")
	return nil
}

func (endpoint *k22NoIOEndpoint) Close() error {
	endpoint.t.Fatal("retention test attempted endpoint I/O")
	return nil
}

func k22RetentionFixture(t *testing.T) (*TeslaGen3ReadAcquisition, *time.Time, *k22NoIOEndpoint) {
	t.Helper()
	now := time.Unix(0, 0).UTC()
	endpoint := &k22NoIOEndpoint{t: t}
	operation := TeslaFC100OperationWCGetVitals
	return &TeslaGen3ReadAcquisition{
		config: TeslaGen3ReadAcquisitionConfig{Now: func() time.Time { return now }},
		state:  TeslaGen3ReadLifecycleReady, endpoint: endpoint, epoch: 1,
		windows: map[TeslaFC100Operation]TeslaGen3ReadWindow{
			operation: {Freshness: time.Second, Retention: time.Minute},
		},
		retained: map[TeslaFC100Operation]*teslaGen3ReadRetention{
			operation: {},
		},
	}, &now, endpoint
}

func k22SyntheticACK(epoch, request uint64, at time.Time, body []byte) TeslaGen3ReadRecord {
	return TeslaGen3ReadRecord{
		Endpoint: "synthetic-family", Node: 0x10,
		Operation: TeslaFC100OperationWCGetVitals,
		Family:    6, RequestTag: 1, TerminalTag: 2,
		Profile: TeslaHSCProfileName, ProfileVersion: TeslaHSCCompatibilityV1,
		OperationVersion: TeslaHSCFC100OperationCompatibilityV1,
		Source:           TeslaGen3ReadSourceRTUPrivateFunction,
		LifecycleEpoch:   epoch, Generation: epoch, RequestID: request,
		Attempt: 1, MaxAttempts: 1, ObservedAt: at,
		State: TeslaGen3ReadEvidenceAck,
		// Deliberately opaque fixture bytes, not frames from a device.
		requestADU: []byte{0xa1}, responseADUs: [][]byte{body},
		results: []TeslaFC100OperationResult{{Kind: TeslaFC100OperationTerminal, Body: body}},
	}
}

func k22Retained(t *testing.T, acquisition *TeslaGen3ReadAcquisition) TeslaGen3ReadRetained {
	t.Helper()
	retained, ok := acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || !retained.HasLastGood || !retained.HasLastAttempt {
		t.Fatalf("missing retained evidence: %+v, present=%v", retained, ok)
	}
	return retained
}

func TestK22RetiredEvidenceCannotRefreshSuccessor(t *testing.T) {
	for _, fence := range []string{"endpoint", "epoch", "endpoint_and_epoch"} {
		t.Run(fence, func(t *testing.T) {
			acquisition, now, oldEndpoint := k22RetentionFixture(t)
			old := acquisition.publish(k22SyntheticACK(1, 1, *now, []byte{0x11, 0x12}), oldEndpoint)
			if fence != "epoch" {
				acquisition.endpoint = &k22NoIOEndpoint{t: t}
			}
			if fence != "endpoint" {
				acquisition.epoch = 2
			}
			*now = now.Add(2 * time.Second)
			good := acquisition.publish(k22SyntheticACK(acquisition.epoch, 2, *now, []byte{0x21}), acquisition.endpoint)
			before := k22Retained(t, acquisition)
			if !before.Fresh || !before.Current || !reflect.DeepEqual(before.LastGood, good) {
				t.Fatalf("successor ACK was not retained: %+v", before)
			}

			*now = now.Add(2 * time.Second)
			// Even a recent observation timestamp cannot overcome old ownership.
			old.ObservedAt = *now
			delayed := acquisition.publish(old, oldEndpoint)
			wantDelayed := old.clone()
			wantDelayed.State = TeslaGen3ReadEvidenceIndeterminate
			wantDelayed.Cause = "receipt is not current for the owned endpoint epoch"
			wantDelayed.results = nil
			after := k22Retained(t, acquisition)
			if !reflect.DeepEqual(delayed, wantDelayed) || !reflect.DeepEqual(after.LastAttempt, wantDelayed) {
				t.Fatalf("delayed evidence lost old provenance or raw bytes: %+v", after.LastAttempt)
			}
			if !reflect.DeepEqual(after.LastGood, good) || after.Fresh || !after.Current || delayed.TerminalBody() != nil {
				t.Fatalf("retired evidence refreshed successor: %+v", after)
			}
		})
	}
}

func TestK22PartialSuccessorKeepsAgedLastGoodWithoutBorrowing(t *testing.T) {
	acquisition, now, oldEndpoint := k22RetentionFixture(t)
	good := acquisition.publish(k22SyntheticACK(1, 1, *now, []byte{0x31, 0x32}), oldEndpoint)
	acquisition.endpoint = &k22NoIOEndpoint{t: t}
	acquisition.epoch = 2
	*now = now.Add(2 * time.Second)
	// A failed newer receipt has no decoded value or missing identity bytes.
	// This is the existing indeterminate outcome entering retention, not a
	// fabricated ACK and not an attempt to classify a malformed wire frame.
	partial := TeslaGen3ReadRecord{
		Operation:      TeslaFC100OperationWCGetVitals,
		LifecycleEpoch: 2, Generation: 2, RequestID: 2,
		ObservedAt: *now, State: TeslaGen3ReadEvidenceIndeterminate,
		responseADUs: [][]byte{{0x41}},
	}
	published := acquisition.publish(partial, acquisition.endpoint)
	retained := k22Retained(t, acquisition)
	if !reflect.DeepEqual(published, partial) || !reflect.DeepEqual(retained.LastAttempt, partial) ||
		published.TerminalBody() != nil || published.Results() != nil {
		t.Fatalf("partial receipt borrowed identity or value bytes: %+v", retained.LastAttempt)
	}
	if !reflect.DeepEqual(retained.LastGood, good) || retained.Fresh || retained.Current {
		t.Fatalf("old last-good lost its age or provenance: %+v", retained)
	}

	// Late old evidence remains historical and cannot restart the old ACK's age.
	old := good.clone()
	old.ObservedAt = *now
	acquisition.publish(old, oldEndpoint)
	retained = k22Retained(t, acquisition)
	if !reflect.DeepEqual(retained.LastGood, good) || retained.Fresh || retained.Current ||
		retained.LastAttempt.LifecycleEpoch != 1 || retained.LastAttempt.RequestID != 1 ||
		retained.LastAttempt.State != TeslaGen3ReadEvidenceIndeterminate || retained.LastAttempt.TerminalBody() != nil {
		t.Fatalf("delayed old receipt promoted historical state: %+v", retained)
	}
	*now = good.ObservedAt.Add(time.Minute + time.Nanosecond)
	retained, ok := acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || retained.HasLastGood || !retained.HasLastAttempt || retained.Fresh || retained.Current {
		t.Fatalf("delayed evidence extended last-good retention: %+v, present=%v", retained, ok)
	}
}
