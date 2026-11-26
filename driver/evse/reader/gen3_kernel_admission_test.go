package reader

import (
	"context"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// The kernel with the EVSE pack takes every batch the reader commits as it is:
// no candidate without a value, and no setter capability whose typed constraint
// has no source. A refused batch fails the test with the kernel's refusal.
func TestKernelAdmitsEveryBatchTheReaderCommitsUnchanged(t *testing.T) {
	scenarios := map[string]func(t *testing.T) (*ChargerReader, *fakeSession){
		"zero current and zero timeout dispositions": func(t *testing.T) (*ChargerReader, *fakeSession) {
			cfg := syntheticConfig(fiveOperations...)
			cfg.Operations = append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)
			cfg.ProvisionalReadbackQualified = true
			reader, session, _ := run(t, cfg, func(e *scriptedEndpoint) {
				pushPolls(e, cfg.Operations, scalars{limit: u(0), timeout: u(0), inhibit: b(false), configured: u(0)})
			})
			waitFor(t, "the dispositions", func() bool { return len(reader.Dispositions()) == 3 })
			return reader, session
		},
		"allocation expired before it was published": func(t *testing.T) (*ChargerReader, *fakeSession) {
			reader, session, c, clk := startMutation(t)
			pushMutation(c, scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(32)},
				scalars{limit: u(12), timeout: u(60), inhibit: b(false), configured: u(32)}, 1000, 1070)
			result := settle(t, reader, tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 16, LimitTimeoutSeconds: 900})
			clk.set(time.Unix(1060, 0))
			if err := reader.IngestSetResult(context.Background(), result); err != nil {
				t.Fatal(err)
			}
			return reader, session
		},
		"allocation published and then expired": func(t *testing.T) (*ChargerReader, *fakeSession) {
			reader, session, c, clk := startAfterFirstPoll(t)
			clk.set(time.Unix(105, 0))
			ingestReadback(t, reader, c, step{echo: true, wall: 100, epoch: "endpoint-clock", mono: 70 * time.Second,
				terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(60), inhibit: b(false), configured: u(32)}))})
			clk.set(time.Unix(160, 0))
			if err := reader.refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
			return reader, session
		},
		"inhibit with its supply and expiry": func(t *testing.T) (*ChargerReader, *fakeSession) {
			reader, session, c, _ := startAfterFirstPoll(t)
			ingestReadback(t, reader, c, step{echo: true, wall: 2000, epoch: "endpoint-clock", mono: 2 * time.Second,
				terminal: terminal(6, 28, readbackBody(scalars{limit: u(16), timeout: u(600), inhibit: b(true), configured: u(32)}))})
			return reader, session
		},
		"first sequence": func(t *testing.T) (*ChargerReader, *fakeSession) {
			d := loadDonor(t)
			_, _ = d.replay(t, d.payloads(t))
			return nil, nil
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			reader, session := scenario(t)
			if reader == nil {
				return // the replay applies and checks every batch itself
			}
			pub := syntheticPublication()
			if _, batches, err := tryApplyToKernel(pub.Asset, syntheticSources(t, pub), session); err != nil {
				t.Fatal(err)
			} else {
				for i, b := range batches {
					for _, f := range b.FactUpserts {
						if f.Value == nil {
							t.Fatalf("batch %d carries the candidate %s without a value", i+1, f.Key.FactID)
						}
					}
					for _, c := range b.CapabilityUpserts {
						if c.Definition.ID == SetAllocated.Capability {
							t.Fatalf("batch %d carries the setter capability", i+1)
						}
					}
				}
			}
		})
	}
}

// The check above fails on a refused batch: a candidate without a value, in the
// withheld form the reader used to publish, is refused by the kernel and
// reported with its batch number.
func TestKernelRefusalOfABatchIsReported(t *testing.T) {
	reader, session, c, _ := startAfterFirstPoll(t)
	ingestReadback(t, reader, c, step{echo: true, wall: 2000, epoch: "endpoint-clock", mono: 2 * time.Second,
		terminal: terminal(6, 28, readbackBody(scalars{limit: u(12), timeout: u(600), inhibit: b(false), configured: u(32)}))})
	pub := syntheticPublication()
	for i := range session.batches {
		for j := range session.batches[i].FactUpserts {
			f := &session.batches[i].FactUpserts[j]
			f.Value = nil
			f.Quality.Availability, f.Quality.Reasons = semreg.AvailabilityWithdrawn, []semreg.DefinitionID{ReasonProvisionalMissing}
		}
		digest, err := session.batches[i].ComputedDigest()
		if err != nil {
			t.Fatal(err)
		}
		session.batches[i].BatchDigest = digest
	}
	if _, _, err := tryApplyToKernel(pub.Asset, syntheticSources(t, pub), session); err == nil {
		t.Fatal("a candidate without a value was admitted by the kernel")
	}
	_ = semreg.ContractKernelV1
}
