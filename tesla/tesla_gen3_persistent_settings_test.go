package tesla

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func teslaPersistentHarness(t *testing.T, mutate func(*Gen3HSCProfileConfig)) *teslaGen3ReadHarness {
	t.Helper()
	return newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
		gen3 := config.Gen3.config
		gen3.Version = "26.34.0"
		gen3.PersistentSettingsOperationVersion = HSCFC100OperationCompatibilityV1
		gen3.PersistentCurrentOnlyQualified = true
		if mutate != nil {
			mutate(&gen3)
		}
		config.Gen3 = mustTeslaGen3Profile(t, gen3)
		config.PersistentIdentityCapacity = 3
		config.Windows[FC100OperationWCConfigureSettings] = Gen3ReadWindow{Freshness: time.Second, Retention: 2 * time.Second}
	})
}

func TestTeslaPersistentSettingsRetainsOriginalReceipt(t *testing.T) {
	h := teslaPersistentHarness(t, nil)
	ctx := context.Background()
	input := PersistentSettingsRequest{MaxOutputCurrentAmps: 16, OperationIdentity: "persistent-a", Deadline: time.Now().Add(time.Second)}
	returned := []byte{0x0a, 0x04, 0x08, 0x0c, 0x68, 0x01}
	teslaPushTerminal(t, h, FC100OperationWCConfigureSettings, returned)
	var result PersistentSettingsResult
	result, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || result.Outcome != Gen3ReadEvidenceAck || result.Comparison != CurrentLimitMismatch || result.Requested != input || result.ReturnedMaxOutputCurrentAmps != 12 || !result.HasReturnedCurrent || !result.Current || !result.Executed || result.Duplicate {
		t.Fatalf("result=%+v, err=%v", result, err)
	}
	receipt := result.Receipt
	expectedRequest := []byte{8, 0x32, 6, 0x3a, 4, 0x0a, 2, 0x08, 16}
	if receipt.RequestTag != 7 || receipt.TerminalTag != 8 || receipt.Generation == 0 || receipt.RequestID == 0 || !bytes.Equal(privateFunctionPayload(receipt.RequestADU()), expectedRequest) || !bytes.Equal(receipt.TerminalBody(), returned) || !bytes.Equal(receipt.terminalPayload(), teslaGen3ReadTerminal(t, FC100OperationWCConfigureSettings, returned)) {
		t.Fatalf("receipt=%+v", receipt)
	}
	mutated := receipt.ResponseADUs()
	mutated[0][2] ^= 1
	for _, change := range []func(*PersistentSettingsRequest){
		func(r *PersistentSettingsRequest) { r.MaxOutputCurrentAmps++ },
		func(r *PersistentSettingsRequest) { r.Deadline = r.Deadline.Add(time.Millisecond) },
	} {
		other := input
		change(&other)
		result, err := h.acquisition.ConfigurePersistentCurrent(ctx, other)
		if err == nil || !result.NoSend || h.endpoint().exchanges() != 1 {
			t.Fatalf("conflicting intent=%+v, %v", result, err)
		}
	}
	if err := h.acquisition.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	duplicate, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || !duplicate.Duplicate || duplicate.Executed || duplicate.Current || duplicate.Receipt.RequestID != receipt.RequestID || duplicate.Receipt.EndpointInstance != receipt.EndpointInstance || !bytes.Equal(duplicate.Receipt.ResponseADUs()[0], receipt.ResponseADUs()[0]) || h.endpoint().exchanges() != 0 {
		t.Fatalf("duplicate=%+v, %v", duplicate, err)
	}
	other := input
	other.OperationIdentity = "persistent-b"
	teslaPushTerminal(t, h, FC100OperationWCConfigureSettings, EncodeTeslaPersistentSettingsBody(16))
	next, err := h.acquisition.ConfigurePersistentCurrent(ctx, other)
	if err != nil || next.Comparison != CurrentLimitMatch || next.Receipt.EndpointInstance == receipt.EndpointInstance || h.endpoint().exchanges() != 1 {
		t.Fatalf("new intent=%+v, %v", next, err)
	}
	h.clock.advance(3 * time.Second)
	expired, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || !expired.Duplicate || expired.Executed || len(expired.Receipt.ResponseADUs()) != 0 || h.endpoint().exchanges() != 1 {
		t.Fatalf("expired duplicate=%+v, %v", expired, err)
	}
}

func TestTeslaPersistentSettingsDeniesUnqualifiedOrExpired(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*Gen3HSCProfileConfig)
		deadline time.Duration
	}{
		{"schema", func(c *Gen3HSCProfileConfig) { c.PersistentSettingsOperationVersion = "" }, time.Second},
		{"scope", func(c *Gen3HSCProfileConfig) { c.PersistentCurrentOnlyQualified = false }, time.Second},
		{"firmware", func(c *Gen3HSCProfileConfig) { c.Version = "26.34.1" }, time.Second},
		{"expired", nil, -time.Second},
		{"too far", nil, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := teslaPersistentHarness(t, tc.mutate)
			ctx := context.Background()
			result, err := h.acquisition.ConfigurePersistentCurrent(ctx, PersistentSettingsRequest{16, "persistent", time.Now().Add(tc.deadline)})
			if err == nil || !result.NoSend || h.endpoint().exchanges() != 0 {
				t.Fatalf("result=%+v, %v", result, err)
			}
		})
	}
}

func TestTeslaPersistentSettingsDeadlineAndReservation(t *testing.T) {
	h := teslaPersistentHarness(t, nil)
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	input := PersistentSettingsRequest{16, "persistent", time.Now().Add(time.Second)}
	h.endpoint().push(teslaGen3FakeStep{started: started, release: release, terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, nil)})
	done := make(chan error, 1)
	go func() { _, err := h.acquisition.Acquire(ctx, FC100OperationWCGetVitals); done <- err }()
	<-started
	busy, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	close(release)
	if pollErr := <-done; pollErr != nil {
		t.Fatal(pollErr)
	}
	if err != ErrTeslaGen3ReadAcquisitionBusy || !busy.NoSend {
		t.Fatalf("busy=%+v, %v", busy, err)
	}
	input.Deadline = time.Now().Add(40 * time.Millisecond)
	h.endpoint().push(teslaGen3FakeStep{outcome: "block"})
	result, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || result.Outcome != Gen3ReadEvidenceTimeout || !result.PossiblyTransmitted || result.NoSend || result.Current || result.HasReturnedCurrent || h.endpoint().exchanges() != 2 {
		t.Fatalf("timeout=%+v, %v", result, err)
	}
	duplicate, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || !duplicate.Duplicate || duplicate.Current || duplicate.Outcome != Gen3ReadEvidenceTimeout || h.endpoint().exchanges() != 2 {
		t.Fatalf("duplicate timeout=%+v, %v", duplicate, err)
	}
}

func TestTeslaPersistentSettingsLateReceiptDoesNotComplete(t *testing.T) {
	h := teslaPersistentHarness(t, nil)
	ctx := context.Background()
	input := PersistentSettingsRequest{16, "late-settings", time.Now().Add(40 * time.Millisecond)}
	terminal := teslaGen3ReadTerminal(t, FC100OperationWCConfigureSettings, EncodeTeslaPersistentSettingsBody(16))
	h.endpoint().push(teslaGen3FakeStep{release: make(chan struct{}), terminal: terminal})
	result, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || result.Outcome != Gen3ReadEvidenceTimeout || result.HasReturnedCurrent || result.Current || !result.PossiblyTransmitted || !bytes.Equal(result.Receipt.terminalPayload(), terminal) {
		t.Fatalf("late t8: %+v, %v", result, err)
	}
	if err := h.acquisition.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := h.acquisition.ConfigurePersistentCurrent(ctx, input)
	if err != nil || !old.Duplicate || old.Current || old.Receipt.EndpointInstance != result.Receipt.EndpointInstance || h.endpoint().exchanges() != 0 {
		t.Fatalf("old t8: %+v, %v", old, err)
	}
}

func TestTeslaPersistentSettingsSignedCurrent(t *testing.T) {
	for _, current := range []int32{-2147483648, -1, 0, 2147483647} {
		payload := EncodeTeslaPersistentSettingsBody(current)
		if current < 0 && (len(payload) != 13 || payload[11] != 255 || payload[12] != 1) {
			t.Fatalf("noncanonical signed int32: %x", payload)
		}
		decoded, present, err := decodeTeslaPersistentSettingsBody(payload)
		if err != nil || !present || decoded != current {
			t.Fatalf("signed current %d decoded as %d, %t, %v", current, decoded, present, err)
		}
	}
}
