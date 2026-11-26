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

// wantNoCandidate requires that no committed batch carries a candidate for the
// element: an element without a value is never published as a candidate.
func wantNoCandidate(t *testing.T, batches []semreg.PublicationBatch, element string) {
	t.Helper()
	for _, b := range batches {
		for _, f := range b.FactUpserts {
			if string(f.Key.FactID) == element {
				t.Fatalf("a candidate %+v was published for %s, an element without a value", f, element)
			}
		}
	}
}

// wantDisposition requires the reader's published disposition of the element.
func wantDisposition(t *testing.T, reader *ChargerReader, element, reason string) Disposition {
	t.Helper()
	for _, d := range reader.Dispositions() {
		if d.Element == element {
			if d.Reason != reason {
				t.Fatalf("disposition of %s is %q, want %q", element, d.Reason, reason)
			}
			return d
		}
	}
	t.Fatalf("no disposition for %s in %+v", element, reader.Dispositions())
	return Disposition{}
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
	wantNoCandidate(t, batches, ElementAllocatedCurrent)
	for i, b := range batches {
		// Only a candidate the reader published is withdrawn; this one never was.
		if len(b.FactWithdrawals) != 0 {
			t.Fatalf("batch %d withdraws %v", i+1, b.FactWithdrawals)
		}
	}
	wantDisposition(t, reader, ElementAllocatedCurrent, ReasonProvisionalExpired)
	if f := candidateOf(t, batches, ElementAllocatedExpiry); f.Value == nil || f.Quality.Availability != semreg.AvailabilityAvailable {
		t.Fatalf("the expiry fact does not remain: %+v", f)
	}
}

func TestCommittedBatchCarriesTheUnresolvedZeroDispositions(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	cfg.Operations = append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)
	cfg.ProvisionalReadbackQualified = true
	reader, session, _ := run(t, cfg, func(e *scriptedEndpoint) {
		pushPolls(e, cfg.Operations, scalars{limit: u(0), timeout: u(0), inhibit: b(false), configured: u(0)})
	})
	waitFor(t, "the dispositions", func() bool { return len(reader.Dispositions()) == 3 })
	batches := session.committed()
	for element, reason := range map[string]string{ElementAllocatedCurrent: ReasonZeroCurrent, ElementConfiguredCurrent: ReasonZeroCurrent, ElementAllocatedExpiry: ReasonZeroTimeout} {
		wantNoCandidate(t, batches, element)
		wantDisposition(t, reader, element, reason)
	}
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
			reader, err := NewChargerReader(cfg, syntheticPublication(), newCharger(t).deps)
			if err != nil {
				t.Fatal(err)
			}
			first, _, _, _ := reader.batch(View{}, At{Wall: time.Unix(5, 0)})
			if digest, err := first.ComputedDigest(); err != nil {
				t.Fatal(err)
			} else {
				first.BatchDigest = digest
			}
			if err := first.Validate(); err != nil {
				t.Fatal(err)
			}
			// The qualified operation is not a published capability: the setter
			// capability is never in the batch and is reported as not published,
			// with the build's own reason where the operation is not qualified
			// and the open pack rule where it is.
			for _, cp := range first.CapabilityUpserts {
				if cp.Definition.ID == "evse.capability.set_allocated_current" {
					t.Fatalf("the setter capability is published: %+v", cp)
				}
			}
			unpublished := reader.UnpublishedCapabilities()
			if len(unpublished) != 1 || unpublished[0].Capability != SetAllocated.Capability || unpublished[0].Reason == "" {
				t.Fatalf("unpublished capabilities %+v", unpublished)
			}
			if got, want := unpublished[0].Reason == ReasonSetterConstraintOpen, tc.want; got != want {
				t.Fatalf("reason %q names the open pack rule %v, want %v", unpublished[0].Reason, got, want)
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
	reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
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
