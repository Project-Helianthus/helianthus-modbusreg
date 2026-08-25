package modbusreg

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestTeslaFC100WCVitalsExecutorUsesOneQualifiedInjectedExchange(t *testing.T) {
	profile := testTeslaFC100WCVitalsQualifiedProfile(t)
	exchanger := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{
		{0x04, 0x32, 0x02, 0x0a, 0x00},
		{0x06, 0x32, 0x04, 0x12, 0x02, 0x0a, 0x00},
	}}
	executor, err := NewTeslaFC100WCVitalsExecutor(profile, exchanger)
	if err != nil {
		t.Fatal(err)
	}

	replays, err := executor.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if exchanger.calls != 1 || len(exchanger.requests) != 1 {
		t.Fatalf("injected exchanges = %d, requests = %d", exchanger.calls, len(exchanger.requests))
	}
	request := exchanger.requests[0]
	if got, want := request.FunctionCode(), teslaHSCFunction100; got != want {
		t.Fatalf("request function = %d, want %d", got, want)
	}
	if got, want := request.Payload(), teslaFC100WCVitalsRequestPDU; !bytes.Equal(got, want) {
		t.Fatalf("request payload = %x, want %x", got, want)
	}
	if len(replays) != 2 || replays[0].Kind != TeslaFC100WCVitalsIntermediate ||
		replays[1].Kind != TeslaFC100WCVitalsTerminal ||
		replays[1].SnapshotLength != 0 || replays[1].SnapshotDigest == "" {
		t.Fatalf("redacted replays = %#v", replays)
	}
}

func TestTeslaFC100WCVitalsExecutorFailsClosedBeforeOrAfterInjectedExchange(t *testing.T) {
	unqualified, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedExchanger := &qualifiedFunctionTestTransport{}
	if _, err := NewTeslaFC100WCVitalsExecutor(unqualified, blockedExchanger); !errors.Is(err, ErrQualifiedFunctionNoSend) {
		t.Fatalf("unqualified executor error = %v", err)
	}
	if blockedExchanger.calls != 0 {
		t.Fatalf("unqualified executor exchanges = %d", blockedExchanger.calls)
	}

	profile := testTeslaFC100WCVitalsQualifiedProfile(t)
	badExchanger := &qualifiedFunctionTestTransport{responsePayloads: [][]byte{{0x06, 0x32, 0x04, 0x12, 0x02, 0x12, 0x00}}}
	executor, err := NewTeslaFC100WCVitalsExecutor(profile, badExchanger)
	if err != nil {
		t.Fatal(err)
	}
	if replays, err := executor.Execute(context.Background()); err == nil || replays != nil {
		t.Fatalf("malformed correlated response = %#v, %v", replays, err)
	}
	if badExchanger.calls != 1 {
		t.Fatalf("malformed response exchanges = %d", badExchanger.calls)
	}
}

func testTeslaFC100WCVitalsQualifiedProfile(t *testing.T) TeslaHSCProfile {
	t.Helper()
	profile, err := NewTeslaHSCProfile(TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		WCVitalsOperationVersion: TeslaHSCWCVitalsCompatibilityV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
