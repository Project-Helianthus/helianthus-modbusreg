package tesla

import (
	"bytes"
	"context"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func teslaProvisionalHarness(t *testing.T, poll bool) *teslaGen3ReadHarness {
	t.Helper()
	return newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
		gen3 := config.Gen3.config
		gen3.ProvisionalSetQualified = true
		config.Gen3 = mustTeslaGen3Profile(t, gen3)
		hsc := teslaGen3ReadProfileConfig()
		hsc.WCProvisionalReadbackOperationVersion = HSCFC100OperationCompatibilityV1
		hsc.WCProvisionalReadbackQualified = true
		config.Profile = mustTeslaHSCProfile(t, hsc)
		config.Windows[FC100OperationWCGetProvisional] = Gen3ReadWindow{Freshness: time.Second, Retention: 2 * time.Second}
		if poll {
			config.Operations = append(config.Operations, FC100OperationWCGetProvisional)
		}
	})
}

func teslaReadbackBody(input ProvisionalCurrentLimitRequest, configured *uint32, omitInhibit bool) []byte {
	body, _ := EncodeTeslaProvisionalCurrentLimitBody(input)
	if omitInhibit {
		body = body[:len(body)-2]
		body[1] -= 2
	}
	if configured != nil {
		body = appendTeslaFC100Varint(append(body, 16), uint64(*configured))
	}
	return body
}

func teslaPushTerminal(t *testing.T, h *teslaGen3ReadHarness, op FC100Operation, body []byte) {
	t.Helper()
	h.endpoint().push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, op, body)})
}

func TestTeslaProvisionalCurrentLimitEncoder(t *testing.T) {
	for _, input := range []ProvisionalCurrentLimitRequest{{16, 900, false}, {6, 900, true}} {
		body, err := EncodeTeslaProvisionalCurrentLimitBody(input)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := decodeTeslaGen3ProvisionalCurrentLimitBody(body, false)
		if err != nil || wire.amps != input.LimitCurrentMaxAmps || wire.timeout != input.LimitTimeoutSeconds || wire.inhibit != input.InhibitCharging {
			t.Fatalf("round trip = %+v, %v", wire, err)
		}
		if _, err := ValidateTeslaFC100RequestMatrix(HSCFC100OperationCompatibilityV1, FC100OperationWCSetProvisional, body); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTeslaProvisionalCurrentLimitSetReadback(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		request, encoded, returned  ProvisionalCurrentLimitRequest
		omitInhibit, omitConfigured bool
		comparison                  CurrentLimitComparison
		validated                   bool
	}{
		{"match", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, false, false, CurrentLimitMatch, true},
		{"different amps", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{12, 900, false}, false, false, CurrentLimitMismatch, false},
		{"different duration", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 899, false}, false, false, CurrentLimitMismatch, false},
		{"different inhibit", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, true}, false, false, CurrentLimitMismatch, false},
		{"missing inhibit", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, true, false, CurrentLimitUnknown, false},
		{"missing configured", ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, ProvisionalCurrentLimitRequest{16, 900, false}, false, true, CurrentLimitMatch, false},
		{"pause", ProvisionalCurrentLimitRequest{0, 60, false}, ProvisionalCurrentLimitRequest{6, 900, true}, ProvisionalCurrentLimitRequest{6, 900, true}, false, false, CurrentLimitMatch, true},
		{"pause mismatch", ProvisionalCurrentLimitRequest{0, 0, true}, ProvisionalCurrentLimitRequest{6, 900, true}, ProvisionalCurrentLimitRequest{7, 900, true}, false, false, CurrentLimitMismatch, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := teslaProvisionalHarness(t, false)
			ctx := context.Background()
			bound := uint32(32)
			teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(tc.encoded, &bound, false))
			teslaPushTerminal(t, h, FC100OperationWCSetProvisional, nil)
			configured := &bound
			if tc.omitConfigured {
				configured = nil
			}
			body := append(teslaReadbackBody(tc.returned, configured, tc.omitInhibit), 0x38, 0x09)
			teslaPushTerminal(t, h, FC100OperationWCGetProvisional, body)
			result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, tc.request)
			if err != nil || result.Requested != tc.request || result.Encoded != tc.encoded || result.Comparison != tc.comparison || result.HasValidated != tc.validated || !result.Acknowledged || h.endpoint().exchanges() != 3 {
				t.Fatalf("result=%+v, err=%v", result, err)
			}
			if result.EffectIndeterminate != (tc.comparison == CurrentLimitUnknown) {
				t.Fatalf("comparison %s: effect indeterminate = %t", result.Comparison, result.EffectIndeterminate)
			}
			got, ok := result.Readback.ProvisionalReadback()
			if !ok || got.LimitCurrentMaxAmps != tc.returned.LimitCurrentMaxAmps || got.HasInhibit == tc.omitInhibit || got.HasConfiguredCurrent == tc.omitConfigured || !sameTeslaReceiptGeneration(result.Set, result.Readback) || !sameTeslaReceiptGeneration(result.Prerequisite, result.Set) {
				t.Fatalf("readback=%+v", got)
			}
			sent := h.endpoint().requests
			for _, i := range []int{0, 2} {
				request, err := teslaGen3ExactFC100RequestBody(FC100OperationWCGetProvisional, sent[i].Payload())
				if err != nil || len(request) != 0 {
					t.Fatalf("t27 payload=%x: %v", sent[i].Payload(), err)
				}
			}
			wantSetBody, _ := EncodeTeslaProvisionalCurrentLimitBody(tc.encoded)
			gotSetBody, err := teslaGen3ExactFC100RequestBody(FC100OperationWCSetProvisional, sent[1].Payload())
			if err != nil || !bytes.Equal(gotSetBody, wantSetBody) || !bytes.Equal(result.Readback.TerminalBody(), body) || !bytes.Equal(result.Readback.terminalPayload(), teslaGen3ReadTerminal(t, FC100OperationWCGetProvisional, body)) {
				t.Fatalf("exchange evidence is not exact: %x %v", gotSetBody, err)
			}
			retained, ok := h.acquisition.Retained(FC100OperationWCGetProvisional)
			if !ok || !retained.Current || retained.LastGood.RequestID != result.Readback.RequestID {
				t.Fatalf("off-poll retention missing: %+v", retained)
			}
		})
	}
}

func TestTeslaProvisionalCurrentLimitUsesBoundUntilRetentionExpires(t *testing.T) {
	h := teslaProvisionalHarness(t, false)
	ctx := context.Background()
	input := ProvisionalCurrentLimitRequest{16, 900, false}
	bound := uint32(32)
	body := teslaReadbackBody(input, &bound, false)
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, body)
	seed, err := h.acquisition.SetProvisionalCurrentLimit(ctx, ProvisionalCurrentLimitRequest{33, 900, false})
	if err == nil || !seed.NoSend || seed.Prerequisite.State != Gen3ReadEvidenceAck || h.endpoint().exchanges() != 1 {
		t.Fatalf("initial bound: %+v, %v", seed, err)
	}

	h.clock.advance(1500 * time.Millisecond)
	retained, ok := h.acquisition.Retained(FC100OperationWCGetProvisional)
	if !ok || !retained.Current || retained.Fresh {
		t.Fatalf("expected a non-fresh but retained bound: %+v", retained)
	}
	teslaPushTerminal(t, h, FC100OperationWCSetProvisional, nil)
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, body)
	result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, input)
	if err != nil || !result.Acknowledged || result.Comparison != CurrentLimitMatch || result.Prerequisite.State != "" || result.Bound.RequestID != seed.Prerequisite.RequestID || h.endpoint().exchanges() != 3 {
		t.Fatalf("retained bound: %+v, %v", result, err)
	}

	h.clock.advance(3 * time.Second)
	result, err = h.acquisition.SetProvisionalCurrentLimit(ctx, input)
	if err == nil || !result.NoSend || result.Outcome != Gen3ReadEvidenceRejected || h.endpoint().exchanges() != 3 {
		t.Fatalf("expired bound: %+v, %v", result, err)
	}
}

func TestTeslaProvisionalCurrentLimitExpiredPartialDoesNotReopenBound(t *testing.T) {
	h := teslaProvisionalHarness(t, false)
	ctx := context.Background()
	input := ProvisionalCurrentLimitRequest{16, 900, false}
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(input, nil, true))
	for i := 0; i < 3; i++ {
		if i == 2 {
			h.clock.advance(3 * time.Second)
			if _, ok := h.acquisition.Retained(FC100OperationWCGetProvisional); ok {
				t.Fatal("expired t28 still retained")
			}
		}
		result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, input)
		if err == nil || !result.NoSend || result.Outcome != Gen3ReadEvidenceRejected || h.endpoint().exchanges() != 1 {
			t.Fatalf("attempt %d: %+v, %v", i, result, err)
		}
	}
	if err := h.acquisition.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	bound := uint32(12)
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(input, &bound, false))
	result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, input)
	if err == nil || !result.NoSend || h.endpoint().exchanges() != 1 || result.Prerequisite.State != Gen3ReadEvidenceAck {
		t.Fatalf("successor: %+v, %v", result, err)
	}
}

func TestTeslaProvisionalCurrentLimitFailurePhases(t *testing.T) {
	for _, tc := range []struct {
		name               string
		set, read          teslaGen3FakeStep
		outcome, readState Gen3ReadEvidenceState
		ack                bool
	}{
		{"set timeout", teslaGen3FakeStep{outcome: "timeout"}, teslaGen3FakeStep{}, Gen3ReadEvidenceTimeout, "", false},
		{"read timeout", teslaGen3FakeStep{}, teslaGen3FakeStep{outcome: "timeout"}, Gen3ReadEvidenceTimeout, Gen3ReadEvidenceTimeout, true},
		{"read exception", teslaGen3FakeStep{}, teslaGen3FakeStep{outcome: "exception", status: 3}, Gen3ReadEvidenceIndeterminate, Gen3ReadEvidenceRejected, true},
		{"read application status", teslaGen3FakeStep{}, teslaGen3FakeStep{terminal: []byte{8, 0x22, 6, 0x0a, 4, 0x0a, 2, 8, 9}}, Gen3ReadEvidenceIndeterminate, Gen3ReadEvidenceRejected, true},
		{"read no send", teslaGen3FakeStep{}, teslaGen3FakeStep{outcome: "deny"}, Gen3ReadEvidenceIndeterminate, Gen3ReadEvidenceNoSend, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := teslaProvisionalHarness(t, false)
			ctx := context.Background()
			bound := uint32(32)
			input := ProvisionalCurrentLimitRequest{16, 900, false}
			teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(input, &bound, false))
			tc.set.terminal = teslaGen3ReadTerminal(t, FC100OperationWCSetProvisional, nil)
			h.endpoint().push(tc.set)
			h.endpoint().push(tc.read)
			result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, input)
			count := 2
			if tc.ack {
				count++
			}
			if err != nil || result.Outcome != tc.outcome || result.Readback.State != tc.readState || result.Acknowledged != tc.ack || result.NoSend || !result.PossiblyTransmitted || !result.EffectIndeterminate || h.endpoint().exchanges() != count {
				t.Fatalf("result=%+v, error=%v", result, err)
			}
			if tc.ack && !bytes.Equal(result.Set.terminalPayload(), tc.set.terminal) {
				t.Fatal("ack receipt lost")
			}
			if tc.name == "read application status" && (!result.Readback.HasApplicationStatus || result.Readback.ApplicationStatus != 9) {
				t.Fatal("nested numeric status lost")
			}
			if tc.read.status != 0 && (!result.Readback.HasExceptionStatus || result.Readback.ExceptionStatus != tc.read.status) {
				t.Fatal("numeric exception lost")
			}
			if tc.outcome == Gen3ReadEvidenceTimeout && h.acquisition.State() != Gen3ReadLifecycleUnavailable {
				t.Fatal("timeout did not fence lifecycle")
			}
		})
	}
}

func TestTeslaProvisionalCurrentLimitPrerequisiteDeadline(t *testing.T) {
	input := ProvisionalCurrentLimitRequest{16, 900, false}
	t.Run("after send", func(t *testing.T) {
		h := teslaProvisionalHarness(t, false)
		h.endpoint().push(teslaGen3FakeStep{outcome: "timeout"})
		result, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), input)
		if err != nil || result.Outcome != Gen3ReadEvidenceTimeout || result.NoSend || !result.PossiblyTransmitted || !result.EffectIndeterminate || result.Acknowledged || result.HasEncoded || result.Set.State != "" || result.Comparison != CurrentLimitUnknown {
			t.Fatalf("prerequisite timeout: %+v, %v", result, err)
		}
		if result.Prerequisite.State != Gen3ReadEvidenceTimeout || result.Prerequisite.RequestID == 0 || result.Reason != result.Prerequisite.Cause || h.endpoint().exchanges() != 1 || h.acquisition.State() != Gen3ReadLifecycleUnavailable {
			t.Fatalf("prerequisite receipt or lifecycle fence lost: %+v", result)
		}
	})
	t.Run("before send", func(t *testing.T) {
		h := teslaProvisionalHarness(t, false)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		result, err := h.acquisition.SetProvisionalCurrentLimit(ctx, input)
		if err != context.DeadlineExceeded || result.Outcome != Gen3ReadEvidenceRejected || !result.NoSend || result.PossiblyTransmitted || result.EffectIndeterminate || result.Acknowledged || h.endpoint().exchanges() != 0 {
			t.Fatalf("expired context: %+v, %v", result, err)
		}
	})
}

func TestTeslaProvisionalCurrentLimitCountdownComparison(t *testing.T) {
	request := ProvisionalCurrentLimitRequest{16, 900, false}
	reply := ProvisionalCurrentLimitReadback{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 899, HasLimitCurrent: true, HasLimitTimeout: true, HasInhibit: true}
	if compareTeslaProvisional(request, reply, true) != CurrentLimitMatch || compareTeslaProvisional(request, reply, false) != CurrentLimitMismatch {
		t.Fatal("remaining time compared as echoed duration")
	}
}

func TestTeslaProvisionalCurrentLimitRejectsInvalidIntent(t *testing.T) {
	h := teslaProvisionalHarness(t, false)
	for _, input := range []ProvisionalCurrentLimitRequest{{5, 900, false}, {16, 0, false}, {16, 86400, false}, {16, 60, false}, {16, 900, true}} {
		got, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), input)
		if err == nil || !got.NoSend || h.endpoint().exchanges() != 0 {
			t.Fatalf("invalid input %+v: %+v, %v", input, got, err)
		}
	}
	if err := h.acquisition.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), ProvisionalCurrentLimitRequest{16, 900, false})
	if err == nil || !got.NoSend || h.endpoint().exchanges() != 0 {
		t.Fatalf("closed lifecycle: %+v, %v", got, err)
	}
}

func TestTeslaProvisionalCurrentLimitZeroOrBelowFloorBound(t *testing.T) {
	for _, bound := range []uint32{0, 5} {
		h := teslaProvisionalHarness(t, false)
		teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(ProvisionalCurrentLimitRequest{6, 900, true}, &bound, false))
		for i := 0; i < 2; i++ {
			result, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), ProvisionalCurrentLimitRequest{})
			observed, ok := result.Bound.ProvisionalReadback()
			if err == nil || !result.NoSend || h.endpoint().exchanges() != 1 || !ok || !observed.HasConfiguredCurrent || observed.ConfiguredCurrentAmps != bound {
				t.Fatalf("bound %d attempt %d: %+v, %v", bound, i, result, err)
			}
		}
	}
}

func TestTeslaProvisionalCurrentLimitStatusIsNotAcknowledgement(t *testing.T) {
	h := teslaProvisionalHarness(t, false)
	bound := uint32(32)
	input := ProvisionalCurrentLimitRequest{16, 900, false}
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(input, &bound, false))
	h.endpoint().push(teslaGen3FakeStep{terminal: []byte{8, 0x22, 6, 0x0a, 4, 0x0a, 2, 8, 3}})
	result, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), input)
	if err != nil || result.Acknowledged || result.Outcome != Gen3ReadEvidenceRejected || result.NoSend || !result.Set.HasApplicationStatus || result.Set.ApplicationStatus != 3 || h.endpoint().exchanges() != 2 {
		t.Fatalf("status reply: %+v, %v", result, err)
	}
}

type teslaProvisionalDeadlineEndpoint struct {
	Gen3ReadEndpoint
	deadlines []time.Time
}

func (endpoint *teslaProvisionalDeadlineEndpoint) ExchangePrivateFunction(ctx context.Context, unit byte, request modbus.PrivateFunctionRequest, policy modbus.PrivateFunctionResponsePolicy, classifier modbus.RTUPrivateFunctionResponseClassifier) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error) {
	deadline, _ := ctx.Deadline()
	endpoint.deadlines = append(endpoint.deadlines, deadline)
	if len(endpoint.deadlines) == 2 {
		select {
		case <-time.After(time.Until(deadline) - 80*time.Millisecond):
		case <-ctx.Done():
		}
	}
	return endpoint.Gen3ReadEndpoint.ExchangePrivateFunction(ctx, unit, request, policy, classifier)
}

func TestTeslaProvisionalCurrentLimitSharesOperationDeadline(t *testing.T) {
	h := teslaProvisionalHarness(t, false)
	h.acquisition.config.AcquisitionDeadline = 200 * time.Millisecond
	endpoint := &teslaProvisionalDeadlineEndpoint{Gen3ReadEndpoint: h.acquisition.endpoint}
	h.acquisition.endpoint = endpoint
	input := ProvisionalCurrentLimitRequest{16, 900, false}
	bound := uint32(32)
	teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(input, &bound, false))
	teslaPushTerminal(t, h, FC100OperationWCSetProvisional, nil)
	h.endpoint().push(teslaGen3FakeStep{outcome: "block"})
	result, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), input)
	if err != nil || result.Outcome != Gen3ReadEvidenceTimeout || !result.Acknowledged || result.Comparison != CurrentLimitUnknown || len(endpoint.deadlines) != 3 {
		t.Fatalf("deadline: %+v, %v", result, err)
	}
	for _, deadline := range endpoint.deadlines {
		if !deadline.Equal(result.Deadline) {
			t.Fatalf("deadline restarted: %v, %v", deadline, result.Deadline)
		}
	}
}
