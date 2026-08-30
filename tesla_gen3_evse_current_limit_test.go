package modbusreg

import "testing"

func currentLimitRequest(t *testing.T, operation TeslaFC100Operation, body []byte) []byte {
	t.Helper()
	request, err := BuildTeslaFC100OperationRequest(TeslaGen3CurrentLimitOperationVersion24443, operation, body)
	if err != nil {
		t.Fatal(err)
	}
	return request.Payload()
}

func currentLimitTerminal(operation TeslaFC100Operation, body []byte) []byte {
	spec := teslaFC100OperationSpecs[operation]
	inner := appendTeslaFC100Varint(nil, spec.responseTag<<3|2)
	inner = appendTeslaFC100Varint(inner, uint64(len(body)))
	inner = append(inner, body...)
	message := appendTeslaFC100Varint(nil, spec.family<<3|2)
	message = appendTeslaFC100Varint(message, uint64(len(inner)))
	message = append(message, inner...)
	return append([]byte{byte(len(message))}, message...)
}

func TestTeslaGen3EVSECurrentLimitRetainsPersistentAndProvisionalRecords(t *testing.T) {
	persistent, err := NewTeslaGen3PersistentCurrentLimit(TeslaGen3PersistentCurrentLimitSpec{
		OperationVersion:     TeslaGen3CurrentLimitOperationVersion24443,
		MaxOutputCurrentAmps: 16,
		RequestPayload:       currentLimitRequest(t, TeslaFC100OperationWCConfigureSettings, []byte{0x08, 0x10}),
		TerminalPayload:      currentLimitTerminal(TeslaFC100OperationWCConfigureSettings, []byte{0x08, 0x10}),
	})
	if err != nil || persistent.MaxOutputCurrentAmps() != 16 || persistent.OperationVersion() != TeslaGen3CurrentLimitOperationVersion24443 {
		t.Fatalf("persistent record = %#v, %v", persistent, err)
	}

	provisional, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
		OperationVersion:        TeslaGen3CurrentLimitOperationVersion24443,
		LimitCurrentMaxAmps:     16,
		LimitTimeoutSeconds:     600,
		InhibitCharging:         false,
		SetRequestPayload:       currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, []byte{0x08, 0x10}),
		AckPayload:              currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil),
		ReadbackRequestPayload:  currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil),
		ReadbackTerminalPayload: currentLimitTerminal(TeslaFC100OperationWCGetProvisional, []byte{0x08, 0x10}),
	})
	if err != nil || provisional.LimitCurrentMaxAmps() != 16 || provisional.LimitTimeoutSeconds() != 600 || provisional.InhibitCharging() {
		t.Fatalf("provisional record = %#v, %v", provisional, err)
	}
	if provisional.OperationVersion() != TeslaGen3CurrentLimitOperationVersion24443 || provisional.ReadbackTerminalPayload()[0] == 0 {
		t.Fatalf("provisional evidence is not retrievable")
	}
	readback := provisional.ReadbackTerminalPayload()
	readback[0] = 0
	if provisional.ReadbackTerminalPayload()[0] == 0 {
		t.Fatal("ReadbackRaw() did not defensively copy")
	}
}

func TestTeslaGen3EVSECurrentLimitInteroperableProjectionIsBounded(t *testing.T) {
	for _, tc := range []struct {
		amps, timeout uint32
		valid         bool
	}{{6, 1, true}, {5, 1, false}, {6, 0, false}, {6, 86400, false}} {
		_, err := NewTeslaGen3InteroperableCurrentLimit(tc.amps, tc.timeout)
		if (err == nil) != tc.valid {
			t.Fatalf("amps=%d timeout=%d err=%v", tc.amps, tc.timeout, err)
		}
	}
	limit, err := NewTeslaGen3InteroperableCurrentLimit(6, 1)
	if err != nil || limit.MaxAmps() != 6 || limit.TimeoutSeconds() != 1 {
		t.Fatalf("bounded projection = %#v, %v", limit, err)
	}
}
