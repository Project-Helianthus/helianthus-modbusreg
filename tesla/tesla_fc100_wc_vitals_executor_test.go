package tesla

import (
	"bytes"
	"context"
	"errors"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/internal/qualifiedfunctiontest"
)

func TestTeslaFC100WCVitalsExecutorUsesOneQualifiedInjectedExchange(t *testing.T) {
	profile := testTeslaFC100WCVitalsQualifiedProfile(t)
	exchanger := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{
		{0x04, 0x32, 0x02, 0x0a, 0x00},
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x08, 0x01},
	}}
	executor, err := NewFC100WCVitalsExecutor(profile, exchanger)
	if err != nil {
		t.Fatal(err)
	}

	replays, err := executor.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if exchanger.Calls != 1 || len(exchanger.Requests) != 1 {
		t.Fatalf("injected exchanges = %d, requests = %d", exchanger.Calls, len(exchanger.Requests))
	}
	request := exchanger.Requests[0]
	if got, want := request.FunctionCode(), teslaHSCFunction100; got != want {
		t.Fatalf("request function = %d, want %d", got, want)
	}
	if got, want := request.Payload(), teslaFC100WCVitalsRequestPDU; !bytes.Equal(got, want) {
		t.Fatalf("request payload = %x, want %x", got, want)
	}
	if len(replays) != 2 || replays[0].Kind != FC100WCVitalsIntermediate ||
		replays[1].Kind != FC100WCVitalsTerminal ||
		replays[1].SnapshotLength != 2 || replays[1].SnapshotDigest == "" {
		t.Fatalf("unversioned replays = %#v", replays)
	}
	t.Run("firmware scoped", func(t *testing.T) {
		exchanger.ResponsePayloads[1] = []byte{8, 0x32, 6, 0x12, 4, 0x0a, 2, 0x08, 0}
		replays, err := executor.ExecuteForFirmware(context.Background(), "26.34.0")
		if err != nil || len(replays) != 2 || exchanger.Calls != 2 {
			t.Fatalf("typed replay exchange: %v, %v, calls %d", replays, err, exchanger.Calls)
		}
		if _, ok := replays[0].Vitals(); ok {
			t.Fatal("echo has a typed value")
		}
		vitals, ok := replays[1].Vitals()
		if !ok || !vitals.UptimeSeconds.Present || vitals.UptimeSeconds.Value != 0 || replays[1].TypedValueDisposition() != "typed" {
			t.Fatalf("terminal vitals = %+v, present %v", vitals, ok)
		}
		exchanger.ResponsePayloads[1][7] = 0x0d // uptime with a fixed32 key, not a varint
		if replays, err := executor.ExecuteForFirmware(context.Background(), "26.34.0"); err == nil || replays != nil || exchanger.Calls != 3 {
			t.Fatalf("malformed typed replay = %v, %v, calls %d", replays, err, exchanger.Calls)
		}
	})
}

func TestTeslaFC100WCVitalsExecutorFailsClosedBeforeOrAfterInjectedExchange(t *testing.T) {
	unqualified, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedExchanger := &qualifiedfunctiontest.Exchange{}
	if _, err := NewFC100WCVitalsExecutor(unqualified, blockedExchanger); !errors.Is(err, modbusreg.ErrQualifiedFunctionNoSend) {
		t.Fatalf("unqualified executor error = %v", err)
	}
	if blockedExchanger.Calls != 0 {
		t.Fatalf("unqualified executor exchanges = %d", blockedExchanger.Calls)
	}

	profile := testTeslaFC100WCVitalsQualifiedProfile(t)
	badExchanger := &qualifiedfunctiontest.Exchange{ResponsePayloads: [][]byte{{0x06, 0x32, 0x04, 0x0a, 0x02, 0x12, 0x00}}}
	executor, err := NewFC100WCVitalsExecutor(profile, badExchanger)
	if err != nil {
		t.Fatal(err)
	}
	if replays, err := executor.Execute(context.Background()); err == nil || replays != nil {
		t.Fatalf("malformed correlated response = %#v, %v", replays, err)
	}
	if badExchanger.Calls != 1 {
		t.Fatalf("malformed response exchanges = %d", badExchanger.Calls)
	}
}

func testTeslaFC100WCVitalsQualifiedProfile(t *testing.T) HSCProfile {
	t.Helper()
	profile, err := NewHSCProfile(HSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: HSCCompatibilityV1,
		WCVitalsOperationVersion: HSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func testWCVitalsAcquisitionRetention(t *testing.T) {
	t.Helper()
	for name, failed := range map[string]teslaGen3FakeStep{
		"wrong wire type":    {terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x0a, 2, 0x28, 0})},
		"duplicate singular": {terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x0a, 4, 0x18, 0, 0x18, 1})},
		"stale receipt":      {staleReceipt: true},
		"wrong generation":   {receiptGeneration: 2},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			h := newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
				config.Gen3 = mustTeslaGen3Profile(t, Gen3HSCProfileConfig{Enabled: true, Version: "26.34.0", ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true})
				config.Operations = []FC100Operation{FC100OperationWCGetVitals}
				config.MaxAttempts = 1
			})
			endpoint := h.endpoint()
			goodBody := []byte{0x0a, 4, 0x08, 12, 0x10, 7}
			endpoint.push(teslaGen3FakeStep{echo: true, terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, goodBody)})
			records, err := h.acquisition.PollOnce(ctx)
			if err != nil || len(records) != 1 || records[0].State != Gen3ReadEvidenceAck {
				t.Fatalf("first poll = %v, %v", records, err)
			}
			good := records[0]
			vitals, typed := good.Vitals()
			if !typed || vitals.UptimeSeconds != (WCVitalsField[uint32]{Value: 12, Present: true}) {
				t.Fatalf("first vitals = %+v, %v", vitals, typed)
			}
			original := vitals
			vitals.UptimeSeconds.Value = 99
			goodTerminalFrame := bytes.Clone(good.ResponseADUs()[1])
			good.TerminalBody()[0] ^= 1
			good.Results()[1].Body[0] ^= 1
			good.ResponseADUs()[1][0] ^= 1
			if failed.terminal == nil {
				failed.terminal = teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x0a, 2, 0x08, 42})
			}
			endpoint.push(failed)
			bad, err := h.acquisition.Acquire(ctx, FC100OperationWCGetVitals)
			if err != nil || bad.State != Gen3ReadEvidenceIndeterminate || !bad.IntegrityValid || bad.TerminalBody() != nil || endpoint.exchanges() != 2 {
				t.Fatalf("failed receipt = %+v, %v, exchanges %d", bad, err, endpoint.exchanges())
			}
			if _, ok := bad.Vitals(); ok {
				t.Fatal("failed receipt published vitals")
			}
			view, ok := h.acquisition.Retained(FC100OperationWCGetVitals)
			if !ok || !view.HasLastGood || !view.HasLastAttempt || view.LastGood.RequestID != good.RequestID || view.LastAttempt.RequestID != bad.RequestID || view.LastAttempt.State != Gen3ReadEvidenceIndeterminate {
				t.Fatalf("retention = %+v", view)
			}
			frames := view.LastAttempt.ResponseADUs()
			if len(frames) != 1 || len(frames[0]) != len(failed.terminal)+4 || !bytes.Equal(frames[0][2:len(frames[0])-2], failed.terminal) {
				t.Fatalf("failure frames = %x", frames)
			}
			retained, ok := view.LastGood.Vitals()
			if !ok || retained != original || !bytes.Equal(view.LastGood.TerminalBody(), goodBody) || !bytes.Equal(view.LastGood.ResponseADUs()[1], goodTerminalFrame) {
				t.Fatalf("last good changed: %+v", retained)
			}
		})
	}
}
