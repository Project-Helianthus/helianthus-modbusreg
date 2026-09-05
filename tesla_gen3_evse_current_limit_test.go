package modbusreg

import (
	"bytes"
	"testing"
)

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

func currentLimitVarint(value uint64) []byte {
	return appendTeslaFC100Varint(nil, value)
}

func currentLimitField(field uint64, body []byte) []byte {
	result := appendTeslaFC100Varint(nil, field<<3|2)
	result = appendTeslaFC100Varint(result, uint64(len(body)))
	return append(result, body...)
}

func persistentCurrentLimitBody(amps uint32) []byte {
	settings := appendTeslaFC100Varint(nil, 1<<3)
	settings = append(settings, currentLimitVarint(uint64(amps))...)
	return currentLimitField(1, settings)
}

func provisionalCurrentLimitFields(amps, timeout uint32, inhibit bool) []byte {
	result := appendTeslaFC100Varint(nil, 1<<3)
	result = append(result, currentLimitVarint(uint64(amps))...)
	result = appendTeslaFC100Varint(result, 2<<3)
	result = append(result, currentLimitVarint(uint64(timeout))...)
	result = appendTeslaFC100Varint(result, 3<<3)
	if inhibit {
		return append(result, 1)
	}
	return append(result, 0)
}

func provisionalCurrentLimitBody(amps, timeout uint32, inhibit bool) []byte {
	return currentLimitField(1, provisionalCurrentLimitFields(amps, timeout, inhibit))
}

func provisionalCurrentLimitReadbackBody(amps, timeout, configured uint32, inhibit bool) []byte {
	result := provisionalCurrentLimitBody(amps, timeout, inhibit)
	result = appendTeslaFC100Varint(result, 2<<3)
	return append(result, currentLimitVarint(uint64(configured))...)
}

func TestTeslaGen3EVSECurrentLimitRetainsPersistentAndProvisionalRecords(t *testing.T) {
	persistent, err := NewTeslaGen3PersistentCurrentLimit(TeslaGen3PersistentCurrentLimitSpec{
		OperationVersion:     TeslaGen3CurrentLimitOperationVersion24443,
		MaxOutputCurrentAmps: 16,
		RequestPayload:       currentLimitRequest(t, TeslaFC100OperationWCConfigureSettings, persistentCurrentLimitBody(16)),
		TerminalPayload:      currentLimitTerminal(TeslaFC100OperationWCConfigureSettings, persistentCurrentLimitBody(16)),
	})
	if err != nil || persistent.MaxOutputCurrentAmps() != 16 || persistent.OperationVersion() != TeslaGen3CurrentLimitOperationVersion24443 {
		t.Fatalf("persistent record = %#v, %v", persistent, err)
	}

	setRequest := currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, provisionalCurrentLimitBody(16, 600, false))
	ack := currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil)
	readbackRequest := currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil)
	readbackTerminal := currentLimitTerminal(TeslaFC100OperationWCGetProvisional, provisionalCurrentLimitReadbackBody(16, 600, 32, false))
	setRequestWant := append([]byte(nil), setRequest...)
	ackWant := append([]byte(nil), ack...)
	readbackRequestWant := append([]byte(nil), readbackRequest...)
	readbackTerminalWant := append([]byte(nil), readbackTerminal...)
	provisional, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
		OperationVersion:        TeslaGen3CurrentLimitOperationVersion24443,
		LimitCurrentMaxAmps:     16,
		LimitTimeoutSeconds:     600,
		InhibitCharging:         false,
		SetRequestPayload:       setRequest,
		AckPayload:              ack,
		ReadbackRequestPayload:  readbackRequest,
		ReadbackTerminalPayload: readbackTerminal,
	})
	if err != nil || provisional.LimitCurrentMaxAmps() != 16 || provisional.LimitTimeoutSeconds() != 600 || provisional.ConfiguredCurrentAmps() != 32 || provisional.InhibitCharging() {
		t.Fatalf("provisional record = %#v, %v", provisional, err)
	}
	for _, source := range [][]byte{setRequest, ack, readbackRequest, readbackTerminal} {
		source[0] ^= 0xff
	}
	for _, evidence := range []struct {
		name string
		want []byte
		get  func() []byte
	}{
		{"set request", setRequestWant, provisional.SetRequestPayload},
		{"ack", ackWant, provisional.AckPayload},
		{"readback request", readbackRequestWant, provisional.ReadbackRequestPayload},
		{"readback terminal", readbackTerminalWant, provisional.ReadbackTerminalPayload},
	} {
		got := evidence.get()
		if !bytes.Equal(got, evidence.want) {
			t.Fatalf("%s = %x, want %x", evidence.name, got, evidence.want)
		}
		got[0] ^= 0xff
		if bytes.Equal(got, evidence.get()) {
			t.Fatalf("%s getter did not defensively copy", evidence.name)
		}
	}
}

func TestTeslaGen3CurrentLimitRetainsUnknownWireMembersAsRawEvidence(t *testing.T) {
	setBody := append(provisionalCurrentLimitBody(16, 600, false), 0x28, 0x2a)
	readbackBody := append(provisionalCurrentLimitReadbackBody(16, 600, 32, false), 0x28, 0x2a)
	setRequest := currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, setBody)
	readbackTerminal := currentLimitTerminal(TeslaFC100OperationWCGetProvisional, readbackBody)
	limit, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
		OperationVersion:    TeslaGen3CurrentLimitOperationVersion24443,
		LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 600,
		SetRequestPayload:       setRequest,
		AckPayload:              currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil),
		ReadbackRequestPayload:  currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil),
		ReadbackTerminalPayload: readbackTerminal,
	})
	if err != nil || !bytes.Equal(limit.SetRequestPayload(), setRequest) || !bytes.Equal(limit.ReadbackTerminalPayload(), readbackTerminal) {
		t.Fatalf("unknown wire evidence = %#v, %v", limit, err)
	}
}

func TestTeslaGen3CurrentLimitRejectsForgedTypedValuesAndMalformedBodies(t *testing.T) {
	persistentRequest := currentLimitRequest(t, TeslaFC100OperationWCConfigureSettings, persistentCurrentLimitBody(16))
	persistentTerminal := currentLimitTerminal(TeslaFC100OperationWCConfigureSettings, persistentCurrentLimitBody(16))
	setRequest := currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, provisionalCurrentLimitBody(16, 600, false))
	readbackRequest := currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil)
	readbackTerminal := currentLimitTerminal(TeslaFC100OperationWCGetProvisional, provisionalCurrentLimitReadbackBody(16, 600, 32, false))

	if _, err := NewTeslaGen3PersistentCurrentLimit(TeslaGen3PersistentCurrentLimitSpec{
		OperationVersion: TeslaGen3CurrentLimitOperationVersion24443, MaxOutputCurrentAmps: 32,
		RequestPayload: persistentRequest, TerminalPayload: persistentTerminal,
	}); err == nil {
		t.Fatal("persistent typed value detached from retained bodies was accepted")
	}
	for _, tc := range []struct {
		name, setBody, readbackBody string
		ack                         []byte
	}{
		{"typed", string(provisionalCurrentLimitBody(16, 600, false)), string(provisionalCurrentLimitReadbackBody(16, 600, 32, false)), currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil)},
		{"ack body", string(provisionalCurrentLimitBody(16, 600, false)), string(provisionalCurrentLimitReadbackBody(16, 600, 32, false)), currentLimitTerminal(TeslaFC100OperationWCSetProvisional, []byte{0x08, 0x10})},
		{"mismatch", string(provisionalCurrentLimitBody(16, 600, false)), string(provisionalCurrentLimitReadbackBody(15, 600, 32, false)), currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil)},
		{"duplicate", string(append(provisionalCurrentLimitBody(16, 600, false), currentLimitField(1, provisionalCurrentLimitFields(16, 600, false))...)), string(provisionalCurrentLimitReadbackBody(16, 600, 32, false)), currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil)},
		{"malformed", string([]byte{0x0a, 0x01, 0x08}), string(provisionalCurrentLimitReadbackBody(16, 600, 32, false)), currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := setRequest
			if tc.name != "typed" {
				request = currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, []byte(tc.setBody))
			}
			terminal := readbackTerminal
			if tc.name == "mismatch" || tc.name == "duplicate" || tc.name == "malformed" {
				terminal = currentLimitTerminal(TeslaFC100OperationWCGetProvisional, []byte(tc.readbackBody))
			}
			_, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
				OperationVersion: TeslaGen3CurrentLimitOperationVersion24443, LimitCurrentMaxAmps: 32, LimitTimeoutSeconds: 600,
				SetRequestPayload: request, AckPayload: tc.ack, ReadbackRequestPayload: readbackRequest, ReadbackTerminalPayload: terminal,
			})
			if err == nil {
				t.Fatal("invalid provisional context was accepted")
			}
		})
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

func TestTeslaGen3ProvisionalCurrentLimitRejectsMismatchedRequestOperations(t *testing.T) {
	setRequest := currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, []byte{0x08, 0x10})
	readbackRequest := currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil)
	wrongRequest := currentLimitRequest(t, TeslaFC100OperationWCConfigureSettings, []byte{0x08, 0x10})
	for _, tc := range []struct {
		name                        string
		setRequest, readbackRequest []byte
	}{
		{"set", wrongRequest, readbackRequest},
		{"readback", setRequest, wrongRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewTeslaGen3ProvisionalCurrentLimit(TeslaGen3ProvisionalCurrentLimitSpec{
				OperationVersion:        TeslaGen3CurrentLimitOperationVersion24443,
				LimitCurrentMaxAmps:     16,
				LimitTimeoutSeconds:     600,
				SetRequestPayload:       tc.setRequest,
				AckPayload:              currentLimitTerminal(TeslaFC100OperationWCSetProvisional, nil),
				ReadbackRequestPayload:  tc.readbackRequest,
				ReadbackTerminalPayload: currentLimitTerminal(TeslaFC100OperationWCGetProvisional, []byte{0x08, 0x10}),
			})
			if err == nil {
				t.Fatal("mismatched request operation was accepted")
			}
		})
	}
}

func TestTeslaGen3PersistentCurrentLimitRejectsMismatchedRequestOperations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request []byte
	}{
		{"provisional set", currentLimitRequest(t, TeslaFC100OperationWCSetProvisional, []byte{0x08, 0x10})},
		{"provisional get", currentLimitRequest(t, TeslaFC100OperationWCGetProvisional, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewTeslaGen3PersistentCurrentLimit(TeslaGen3PersistentCurrentLimitSpec{
				OperationVersion:     TeslaGen3CurrentLimitOperationVersion24443,
				MaxOutputCurrentAmps: 16,
				RequestPayload:       tc.request,
				TerminalPayload:      currentLimitTerminal(TeslaFC100OperationWCConfigureSettings, []byte{0x08, 0x10}),
			})
			if err == nil {
				t.Fatal("mismatched request operation was accepted")
			}
		})
	}
}
