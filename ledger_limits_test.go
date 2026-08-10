package modbusreg_test

import (
	"reflect"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestLedgerLimitsRejectZeroAndAcceptFiniteConfiguration(t *testing.T) {
	profile := profileFixture(t)
	state, err := reg.EmptySampleLedgerState("bounded-ledger", profile)
	if err != nil {
		t.Fatalf("EmptySampleLedgerState: %v", err)
	}
	if ledger, err := reg.NewSampleLedger(state, 0, reg.LedgerLimits{}); err == nil || ledger != nil {
		t.Fatal("zero ledger limits were accepted")
	}
	limits := reg.DefaultLedgerLimits()
	ledger, err := reg.NewSampleLedger(state, 0, limits)
	if err != nil {
		t.Fatalf("NewSampleLedger: %v", err)
	}
	if got := ledger.Limits(); got != limits {
		t.Fatalf("ledger limits changed: got %+v want %+v", got, limits)
	}
	limits.AuditTombstoneMaxEncodedBytes = 1
	if ledger, err := reg.NewSampleLedger(state, 0, limits); err == nil || ledger != nil {
		t.Fatal("an unusable audit tombstone byte bound was accepted")
	}
	limits = reg.DefaultLedgerLimits()
	limits.AuditTombstoneLimit = 20000
	limits.AuditTombstoneMaxEncodedBytes = 256
	if ledger, err := reg.NewSampleLedger(state, 0, limits); err == nil || ledger != nil {
		t.Fatal("audit limits exceeding the aggregate serialization bound were accepted")
	}
	limits = reg.DefaultLedgerLimits()
	limits.MaxDependencySetEncodedBytes = 1
	boundedLedger, err := reg.NewSampleLedger(state, 0, limits)
	if err != nil {
		t.Fatalf("NewSampleLedger(dependency bound): %v", err)
	}
	if factory, err := reg.NewObservationFactory(
		profile,
		boundedLedger,
		&memoryPublicationCommitter{state: state},
	); err == nil || factory != nil {
		t.Fatal("canonical dependency bytes exceeded their configured bound")
	}
}

func TestFixtureReplayIsEvidenceOnlyAndHasNoProductionSurface(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	encoded, err := reg.MarshalFixtureSpec(spec)
	if err != nil {
		t.Fatalf("MarshalFixtureSpec: %v", err)
	}
	replayer, err := reg.NewFixtureReplayer(profile)
	if err != nil {
		t.Fatalf("NewFixtureReplayer: %v", err)
	}
	replay, err := replayer.Replay(encoded)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replay.FixtureID() == "" {
		t.Fatal("fixture replay lacks its evidence-scoped identity")
	}
	if replay.Spec().SampleID != "" {
		t.Fatal("fixture replay received a production sample ID")
	}
	production := spec
	production.SampleID = "runtime-ledger:1"
	if _, err := reg.MarshalFixtureSpec(production); err == nil {
		t.Fatal("fixture input carrying a production sample ID was accepted")
	}
	typeOfReplay := reflect.TypeOf(replay)
	for _, forbidden := range []string{"Capability", "Claim", "Seal", "Publish", "SampleID"} {
		if _, exists := typeOfReplay.MethodByName(forbidden); exists {
			t.Fatalf("fixture replay exposes forbidden production method %s", forbidden)
		}
	}
}
