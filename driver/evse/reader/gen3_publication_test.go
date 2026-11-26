package reader

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *clock) set(at time.Time) {
	c.mu.Lock()
	c.at = at
	c.mu.Unlock()
}

// runWith starts a reader with an injected clock and no scripted polls.
func runWith(t *testing.T, cfg ChargerConfig, pub ChargerPublication, links func([]semreg.NativeBinding) []semreg.IdentityLink) (*ChargerReader, *fakeSession, *charger, *clock) {
	t.Helper()
	c := newCharger(t)
	clk := &clock{at: time.Unix(5, 0)}
	c.deps.Now = clk.now
	open := c.deps.Open
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		endpoint, err := open(config)
		if err == nil && len(c.endpoint.steps) == 0 && c.opens == 1 {
			pushPolls(c.endpoint, cfg.Operations[:1], scalars{})
		}
		return endpoint, err
	}
	reader, err := NewChargerReader(cfg, pub, c.deps)
	if err != nil || reader == nil {
		t.Fatalf("reader %v err %v", reader, err)
	}
	session := &fakeSession{links: links}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	return reader, session, c, clk
}

func mutationConfig() ChargerConfig {
	cfg := syntheticConfig()
	cfg.Operations = []tesla.FC100Operation{tesla.FC100OperationCommonSystemInfo}
	cfg.Windows[tesla.FC100OperationCommonSystemInfo] = tesla.Gen3ReadWindow{Freshness: time.Hour, Retention: 2 * time.Hour}
	cfg.Windows[tesla.FC100OperationWCGetProvisional] = tesla.Gen3ReadWindow{Freshness: time.Hour, Retention: 2 * time.Hour}
	cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = true, true
	return cfg
}

func startMutation(t *testing.T) (*ChargerReader, *fakeSession, *charger, *clock) {
	t.Helper()
	reader, session, c, clk := runWith(t, mutationConfig(), syntheticPublication(), nil)
	waitFor(t, "the first poll", func() bool { return len(c.endpoint.sent()) == 1 })
	return reader, session, c, clk
}

func pushMutation(c *charger, prereq, readback scalars, setWall, readbackWall int64) {
	c.endpoint.push(
		step{echo: true, terminal: terminal(6, 28, readbackBody(prereq))},
		step{terminal: terminal(6, 26, nil), wall: setWall},
		step{echo: true, terminal: terminal(6, 28, readbackBody(readback)), wall: readbackWall},
	)
}

func settle(t *testing.T, reader *ChargerReader, request tesla.ProvisionalCurrentLimitRequest) tesla.ProvisionalCurrentLimitResult {
	t.Helper()
	var result tesla.ProvisionalCurrentLimitResult
	waitFor(t, "the setter to be free", func() bool {
		var err error
		result, err = reader.Acquisition().SetProvisionalCurrentLimit(context.Background(), request)
		return err == nil || result.Reason != tesla.ErrTeslaGen3ReadAcquisitionBusy.Error()
	})
	return result
}

func TestMismatchedReadbackFeedsTheReturnedValuesImmediately(t *testing.T) {
	reader, session, c, _ := startMutation(t)
	pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
		scalars{limit: u(12), timeout: u(900), inhibit: b(false), configured: u(32)}, 1000, 1010)
	result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
	if result.Comparison != tesla.CurrentLimitMismatch {
		t.Fatalf("comparison %s: %+v", result.Comparison, result)
	}
	before := len(session.committed())
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	view := reader.View()
	wantAmps(t, view, ElementAllocatedCurrent, 12)
	wantAmps(t, view, ElementConfiguredCurrent, 32)
	expiry, _ := view.Fact(ElementAllocatedExpiry)
	if !expiry.Time.Equal(time.Unix(1000+900, 0).UTC()) {
		t.Fatalf("the expiry %v does not run from the set acknowledgement", expiry.Time)
	}
	if len(session.committed()) <= before {
		t.Fatalf("the mismatching readback was not published at once")
	}
	for _, sent := range c.endpoint.sent() {
		if sent.FunctionCode() == 0 {
			t.Fatal("empty request")
		}
	}
}

func TestSetAnchoredExpiryAtSixtyAndSeventySeconds(t *testing.T) {
	for name, tc := range map[string]struct {
		offset time.Duration
		held   bool
	}{"before expiry": {59 * time.Second, false}, "at expiry": {60 * time.Second, true}, "after expiry": {61 * time.Second, true}} {
		t.Run(name, func(t *testing.T) {
			reader, session, c, clk := startMutation(t)
			pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
				scalars{limit: u(12), timeout: u(60), inhibit: b(false), configured: u(32)}, 1000, 1070)
			result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
			clk.set(time.Unix(1000, 0).Add(tc.offset))
			if err := reader.IngestSetResult(context.Background(), result); err != nil {
				t.Fatal(err)
			}
			view := reader.View()
			expiry, ok := view.Fact(ElementAllocatedExpiry)
			if !ok || !expiry.Time.Equal(time.Unix(1060, 0).UTC()) {
				t.Fatalf("the expiry %v is not the set acknowledgement plus 60 s", expiry.Time)
			}
			if tc.held {
				if _, ok := view.Fact(ElementAllocatedCurrent); ok || view.Reason(ElementAllocatedCurrent) != ReasonProvisionalExpired {
					t.Fatalf("the allocation is not withheld: %+v", view)
				}
			} else {
				wantAmps(t, view, ElementAllocatedCurrent, 12)
			}
			_ = session
		})
	}
}

func TestStandaloneReadbackExpiryUsesItsOwnReceipt(t *testing.T) {
	reader, _, c, clk := startMutation(t)
	pushMutation(c, scalars{limit: u(6), timeout: u(60), inhibit: b(false), configured: u(32)},
		scalars{limit: u(12), timeout: u(900), inhibit: b(false), configured: u(32)}, 1000, 1070)
	result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
	clk.set(time.Unix(1070, 0))
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	// The prerequisite t28 is a standalone read: its 60 s runs from its own
	// receipt, and the later mutation readback replaced it with 900 s anchored
	// at the set acknowledgement.
	expiry, _ := reader.View().Fact(ElementAllocatedExpiry)
	if !expiry.Time.Equal(time.Unix(1900, 0).UTC()) {
		t.Fatalf("expiry %v", expiry.Time)
	}
	standalone := NewFeed()
	prerequisite := result.Prerequisite
	standalone.Provisional(ProvisionalObservation{Readback: mustReadback(t, prerequisite), Receipt: receiptOf(prerequisite), Anchor: receiptOf(prerequisite)})
	own, _ := standalone.View(At{Wall: time.Unix(0, 0)}).Fact(ElementAllocatedExpiry)
	if !own.Time.Equal(prerequisite.ReceiptWall.Add(60 * time.Second).UTC()) {
		t.Fatalf("standalone expiry %v", own.Time)
	}
}

func mustReadback(t *testing.T, record tesla.Gen3ReadRecord) tesla.ProvisionalCurrentLimitReadback {
	t.Helper()
	readback, ok := record.ProvisionalReadback()
	if !ok {
		t.Fatal("no readback")
	}
	return readback
}

func TestPersistentEntryFeedsTheReturnedSettingsAndTakesTheLaterReceipt(t *testing.T) {
	cfg := mutationConfig()
	cfg.Version, cfg.PersistentSettingsQualified = "26.34.0", true
	reader, session, c, _ := runWith(t, cfg, syntheticPublication(), nil)
	// An older t28 is retained first: configured 32 A.
	c.endpoint.push(step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(600), inhibit: b(false), configured: u(32)}))})
	waitFor(t, "the polled t28", func() bool {
		record, err := reader.Acquisition().Acquire(context.Background(), tesla.FC100OperationWCGetProvisional)
		if err != nil {
			return false
		}
		return reader.IngestPolled(context.Background(), record) == nil && record.State == tesla.Gen3ReadEvidenceAck
	})
	wantAmps(t, reader.View(), ElementConfiguredCurrent, 32)
	c.endpoint.push(step{terminal: terminal(6, 8, []byte{0x0a, 0x02, 0x08, 0x12})})
	var result tesla.PersistentSettingsResult
	waitFor(t, "t7/t8", func() bool {
		var err error
		result, err = reader.Acquisition().ConfigurePersistentCurrent(context.Background(), tesla.PersistentSettingsRequest{
			MaxOutputCurrentAmps: 20, OperationIdentity: "persistent-a", Deadline: time.Now().Add(5 * time.Second)})
		return err == nil
	})
	if result.Comparison != tesla.CurrentLimitMismatch || result.ReturnedMaxOutputCurrentAmps != 18 {
		t.Fatalf("result %+v", result)
	}
	before := len(session.committed())
	if err := reader.IngestPersistent(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	wantAmps(t, reader.View(), ElementConfiguredCurrent, 18)
	if len(session.committed()) != before+1 {
		t.Fatal("the returned t8 was not published")
	}
	// A retained duplicate, replayed through the native entry, has no effect.
	duplicate, err := reader.Acquisition().ConfigurePersistentCurrent(context.Background(), tesla.PersistentSettingsRequest{
		MaxOutputCurrentAmps: 20, OperationIdentity: "persistent-a", Deadline: result.Requested.Deadline})
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("duplicate %+v err %v", duplicate, err)
	}
	if err := reader.IngestPersistent(context.Background(), duplicate); err != nil || len(session.committed()) != before+1 {
		t.Fatalf("the duplicate had an effect: %v", err)
	}
	// The persistent-only publication withdrew the provisional allocation.
	view := reader.View()
	if _, ok := view.Fact(ElementAllocatedCurrent); ok || view.Reason(ElementAllocatedCurrent) != ReasonProvisionalMissing {
		t.Fatalf("view %+v", view)
	}
	last := session.committed()[len(session.committed())-1]
	if err := last.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, fact := range last.FactUpserts {
		if fact.Key.FactID == ElementAllocatedCurrent {
			// The allocation is not republished or retimed: the candidate carries
			// no value and states why it is withheld.
			if fact.Value != nil || fact.Quality.Availability != semreg.AvailabilityWithdrawn ||
				len(fact.Quality.Reasons) != 1 || fact.Quality.Reasons[0] != ReasonProvisionalMissing {
				t.Fatalf("the allocation candidate %+v", fact)
			}
			return
		}
	}
	t.Fatal("the allocation withdrawal carries no reason")
}

func TestIngestRefusesOutsideTheLifetime(t *testing.T) {
	reader, err := NewChargerReader(syntheticConfig(fiveOperations...), syntheticPublication(), newCharger(t).deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.IngestPersistent(context.Background(), tesla.PersistentSettingsResult{}); err != ErrChargerNotStarted {
		t.Fatalf("before start: %v", err)
	}
	reader.FenceWithdrawal()
	if err := reader.IngestPolled(context.Background(), tesla.Gen3ReadRecord{}); err != ErrChargerFenced {
		t.Fatalf("after fence: %v", err)
	}
}

func TestPublishedBatchIsValidAmpereAndCarriesCapabilities(t *testing.T) {
	for _, writable := range []bool{true, false} {
		cfg := syntheticConfig(fiveOperations...)
		cfg.Operations = append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)
		cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = true, writable
		_, session, c := run(t, cfg, func(e *scriptedEndpoint) {
			pushPolls(e, cfg.Operations, scalars{limit: u(16), timeout: u(600), inhibit: b(true), configured: u(32)})
		})
		_ = c
		waitFor(t, "a batch", func() bool { return len(session.committed()) == 1 })
		batch := session.committed()[0]
		if err := batch.Validate(); err != nil {
			t.Fatalf("writable=%v: %v", writable, err)
		}
		services := map[semreg.ServiceInstanceID]semreg.DefinitionID{}
		for _, s := range batch.ServiceUpserts {
			services[s.InstanceID] = s.Definition.ID
		}
		got := map[semreg.DefinitionID]semreg.DefinitionID{}
		for _, capability := range batch.CapabilityUpserts {
			got[capability.Definition.ID] = services[capability.ServiceInstance]
		}
		want := map[semreg.DefinitionID]semreg.DefinitionID{
			"evse.capability.read.evse": "evse.service.evse", "evse.capability.read.connector": "evse.service.connector"}
		if writable {
			want["evse.capability.set_allocated_current"] = "evse.service.connector"
		}
		if len(got) != len(want) {
			t.Fatalf("writable=%v capabilities %v", writable, got)
		}
		for capability, service := range want {
			if got[capability] != service {
				t.Fatalf("writable=%v %s on %s", writable, capability, got[capability])
			}
		}
		facts := map[semreg.DefinitionID]semreg.FactCandidate{}
		for _, f := range batch.FactUpserts {
			facts[f.Key.FactID] = f
		}
		for _, element := range []string{ElementConfiguredCurrent, ElementAllocatedCurrent} {
			f := facts[semreg.DefinitionID(element)]
			if f.Value == nil || f.Value.Quantity == nil || f.Value.Quantity.Unit != "unit.ampere" {
				t.Fatalf("%s unit %+v", element, f.Value)
			}
		}
		if f := facts[ElementAllocatedCurrent]; f.Value.Quantity.Number.Coefficient != "0" {
			t.Fatalf("inhibit allocation %+v", f.Value.Quantity)
		}
		if f := facts[ElementAllocatedExpiry]; f.Value == nil || f.Value.Kind != semreg.ValueTime || f.Value.Quantity != nil {
			t.Fatalf("expiry %+v", f.Value)
		}
		if f := facts[ElementSupply]; f.Value == nil || f.Value.Symbol == nil || f.Value.Symbol.Token != "disabled" || f.Value.Symbol.Namespace != "evse.status.supply" {
			t.Fatalf("supply %+v", f.Value)
		}
		for element, label := range map[string]string{ElementConfiguredCurrent: "evse-a", ElementAllocatedCurrent: "connector-a", ElementAllocatedExpiry: "connector-a", ElementSupply: "evse-a"} {
			f := facts[semreg.DefinitionID(element)]
			if len(f.Key.Dimensions) != 1 || *f.Key.Dimensions[0].Value.Text != label {
				t.Fatalf("%s dimension %+v", element, f.Key.Dimensions)
			}
		}
		if strings.Contains(string(mustJSON(t, batch)), syntheticSerialPath) {
			t.Fatal("the serial path is in the published batch")
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

var _ = modbus.RTUParityNone
