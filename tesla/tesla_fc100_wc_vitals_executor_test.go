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
		t.Fatalf("redacted replays = %#v", replays)
	}
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
