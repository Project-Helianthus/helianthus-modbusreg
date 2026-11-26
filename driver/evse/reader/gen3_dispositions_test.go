package reader

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func candidateOf(t *testing.T, batches []semreg.PublicationBatch, element string) semreg.FactCandidate {
	t.Helper()
	var found *semreg.FactCandidate
	for _, b := range batches {
		if err := b.Validate(); err != nil {
			t.Fatal(err)
		}
		for i, f := range b.FactUpserts {
			if string(f.Key.FactID) == element {
				found = &b.FactUpserts[i]
			}
		}
	}
	if found == nil {
		t.Fatalf("no candidate for %s", element)
	}
	return *found
}

func wantWithheld(t *testing.T, f semreg.FactCandidate, reason string) {
	t.Helper()
	if f.Value != nil || f.Quality.Availability != semreg.AvailabilityWithdrawn || len(f.Quality.Reasons) != 1 || string(f.Quality.Reasons[0]) != reason {
		t.Fatalf("candidate %s: value %v availability %s reasons %v, want %s", f.Key.FactID, f.Value, f.Quality.Availability, f.Quality.Reasons, reason)
	}
}

func TestCommittedBatchCarriesTheExpiryDisposition(t *testing.T) {
	reader, session, c, clk := startMutation(t)
	pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
		scalars{limit: u(12), timeout: u(60), inhibit: b(false), configured: u(32)}, 1000, 1070)
	result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
	clk.set(time.Unix(1060, 0))
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	batches := session.committed()
	wantWithheld(t, candidateOf(t, batches, ElementAllocatedCurrent), ReasonProvisionalExpired)
	if f := candidateOf(t, batches, ElementAllocatedExpiry); f.Value == nil || f.Quality.Availability != semreg.AvailabilityAvailable {
		t.Fatalf("the expiry fact does not remain: %+v", f)
	}
}

func TestCommittedBatchCarriesTheUnresolvedZeroDispositions(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	cfg.Operations = append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)
	cfg.ProvisionalReadbackQualified = true
	_, session, _ := run(t, cfg, func(e *scriptedEndpoint) {
		pushPolls(e, cfg.Operations, scalars{limit: u(0), timeout: u(0), inhibit: b(false), configured: u(0)})
	})
	waitFor(t, "a batch", func() bool { return len(session.committed()) >= 1 })
	batches := session.committed()
	wantWithheld(t, candidateOf(t, batches, ElementAllocatedCurrent), ReasonZeroCurrent)
	wantWithheld(t, candidateOf(t, batches, ElementConfiguredCurrent), ReasonZeroCurrent)
	wantWithheld(t, candidateOf(t, batches, ElementAllocatedExpiry), ReasonZeroTimeout)
}

func TestSetterCapabilityAgreesWithTheNativeQualificationOfTheBuild(t *testing.T) {
	for name, tc := range map[string]struct {
		version  string
		readback bool
		set      bool
		want     bool
	}{
		"floor with both declarations":     {"wc3_24_44_3", true, true, true},
		"floor without the declaration":    {"wc3_24_44_3", true, false, false},
		"other build with declarations":    {"wc3_25_0_0", true, true, false},
		"MID line with declarations":       {"MID 25.2.2", true, true, false},
		"documented layout, no flags":      {"wc3_26_34_0", false, false, true},
		"documented layout dotted, no set": {"26.42.0", false, false, true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := syntheticConfig(fiveOperations...)
			cfg.Version, cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = tc.version, tc.readback, tc.set
			if got := cfg.CurrentLimitWritable(); got != tc.want {
				t.Fatalf("writable %v, want %v (%+v)", got, tc.want, cfg.Setter())
			}
			reader, err := NewGen3Reader(cfg, syntheticPublication(), newCharger(t).deps)
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := reader.batch(View{}, At{Wall: time.Unix(5, 0)})
			if digest, err := first.ComputedDigest(); err != nil {
				t.Fatal(err)
			} else {
				first.BatchDigest = digest
			}
			if err := first.Validate(); err != nil {
				t.Fatal(err)
			}
			published := false
			for _, cp := range first.CapabilityUpserts {
				if cp.Definition.ID == "evse.capability.set_allocated_current" {
					published = true
				}
			}
			if published != tc.want {
				t.Fatalf("published %v, want %v", published, tc.want)
			}
			if documented := tc.version == "wc3_26_34_0" || tc.version == "26.42.0"; documented {
				return
			}
			// The native operation agrees: it refuses an unqualified build with no
			// traffic, and a qualified build is not refused for its qualification.
			live, _, c := run(t, cfg, func(e *scriptedEndpoint) { pushPolls(e, cfg.Operations, scalars{}) })
			waitFor(t, "five reads", func() bool { return len(c.endpoint.sent()) == 5 })
			var native error
			waitFor(t, "the poll cycle to finish", func() bool {
				_, native = live.Acquisition().SetProvisionalCurrentLimit(context.Background(), tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 6, LimitTimeoutSeconds: 900})
				return !errors.Is(native, tesla.ErrTeslaGen3ReadAcquisitionBusy)
			})
			if refused := refusedForQualification(native); refused == tc.want {
				t.Fatalf("native setter outcome %v disagrees with writable %v", native, tc.want)
			}
		})
	}
}

func refusedForQualification(err error) bool {
	return errors.Is(err, modbusreg.ErrQualifiedFunctionNoSend) && strings.Contains(err.Error(), "not qualified for this build")
}

func TestPollLoopSurfacesPublicationFailures(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	c := newCharger(t)
	c.deps.Now = func() time.Time { return time.Unix(61, 0) }
	open := c.deps.Open
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		endpoint, err := open(config)
		pushPolls(c.endpoint, cfg.Operations, scalars{})
		return endpoint, err
	}
	reader, err := NewGen3Reader(cfg, syntheticPublication(), c.deps)
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("session rejects the commit")
	session := &fakeSession{commitErr: rejected}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	waitFor(t, "the commit failure to be reported", func() bool { return errors.Is(reader.PublicationError(), rejected) })
	if n := len(session.committed()); n != 0 {
		t.Fatalf("%d batches committed", n)
	}
	// The native outcome of the reads is unaffected and distinct from it.
	waitFor(t, "five reads", func() bool { return len(c.endpoint.sent()) == 5 })
}
