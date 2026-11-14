package tesla

import (
	"bytes"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

func TestTeslaGen3HSCNativeOperationRecordRetainsFC100TerminalContext(t *testing.T) {
	payload := []byte{0x0a, 0x01, 0xff}
	record, err := NewGen3HSCNativeOperationRecord(Gen3HSCNativeOperationRecordConfig{
		Profile:          HSCProfileName,
		ProfileVersion:   HSCCompatibilityV1,
		OperationVersion: HSCFC100OperationCompatibilityV1,
		Operation:        FC100OperationWCGetVitals,
		Function:         modbus.PrivateFunctionCode(100),
		Direction:        Gen3HSCResponseDirection,
		Outcome:          Gen3HSCNormalOutcome,
		Payload:          payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Profile() != HSCProfileName ||
		record.ProfileVersion() != HSCCompatibilityV1 ||
		record.OperationVersion() != HSCFC100OperationCompatibilityV1 ||
		record.Operation() != FC100OperationWCGetVitals ||
		record.Function() != modbus.PrivateFunctionCode(100) ||
		record.Direction() != Gen3HSCResponseDirection ||
		record.Outcome() != Gen3HSCNormalOutcome ||
		!bytes.Equal(record.Payload(), payload) {
		t.Fatalf("record=%#v", record)
	}
	payload[0] = 0
	if got := record.Payload(); !bytes.Equal(got, []byte{0x0a, 0x01, 0xff}) {
		t.Fatalf("payload was not copied: %x", got)
	}
}

func TestTeslaGen3HSCNativeOperationRecordAllowsOpaqueFC101AndFC102(t *testing.T) {
	for _, function := range []modbus.PrivateFunctionCode{101, 102} {
		t.Run(string(rune(function)), func(t *testing.T) {
			record, err := NewGen3HSCNativeOperationRecord(Gen3HSCNativeOperationRecordConfig{
				Profile:        HSCProfileName,
				ProfileVersion: HSCCompatibilityV1,
				Function:       function,
				Direction:      Gen3HSCResponseDirection,
				Outcome:        Gen3HSCNormalOutcome,
				Payload:        []byte{byte(function), 0x00},
			})
			if err != nil || record.Operation() != "" || !bytes.Equal(record.Payload(), []byte{byte(function), 0x00}) {
				t.Fatalf("record=%#v err=%v", record, err)
			}
		})
	}
}

func TestTeslaGen3HSCNativeOperationRecordRejectsInvalidPayloadAndContext(t *testing.T) {
	if _, err := NewGen3HSCNativeOperationRecord(Gen3HSCNativeOperationRecordConfig{
		Profile: HSCProfileName, ProfileVersion: HSCCompatibilityV1,
		Function: 101, Direction: Gen3HSCRequestDirection, Outcome: Gen3HSCExceptionOutcome,
	}); err == nil {
		t.Fatal("accepted exception request")
	}
	if _, err := NewGen3HSCNativeOperationRecord(Gen3HSCNativeOperationRecordConfig{
		Profile: HSCProfileName, ProfileVersion: HSCCompatibilityV1,
		Function: 100, Direction: Gen3HSCResponseDirection, Outcome: Gen3HSCNormalOutcome,
	}); err == nil {
		t.Fatal("accepted unqualified FC100 context")
	}
	if _, err := NewGen3HSCNativeOperationRecord(Gen3HSCNativeOperationRecordConfig{
		Profile: HSCProfileName, ProfileVersion: HSCCompatibilityV1,
		Function: 101, Direction: Gen3HSCResponseDirection, Outcome: Gen3HSCNormalOutcome,
		Payload: bytes.Repeat([]byte{0x01}, 253),
	}); err == nil {
		t.Fatal("accepted oversized native payload")
	}
}
