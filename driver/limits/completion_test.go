package limits

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

// Each case names the completion identity field and the production mutation it catches.
func TestCompletionIdentityFields(t *testing.T) {
	at := moment("1")
	later := moment("2")
	base := Evidence(operation.ContractOperationV1, kindRefusal, "refusal:guard_mismatch")
	other := Evidence(operation.ContractOperationV1, kindRefusal, "refusal:owner")
	sum := sha256.Sum256([]byte("refusal:guard_mismatch"))
	if base.Digest != semreg.Digest("sha256:"+hex.EncodeToString(sum[:])) || base.Digest == other.Digest {
		t.Fatalf("evidence digest = %s, other %s; mutation of the material must change the digest", base.Digest, other.Digest)
	}

	refused := NotSent("attempt:sunspec-limit:one", at, "WMaxLimPct", "guard_mismatch")
	if err := refused.Validate(); err != nil {
		t.Fatalf("NotSent validate: %v", err)
	}
	moved := NotSent("attempt:sunspec-limit:two", at, "WMaxLim_Ena", "owner")
	if refused.Dispatch.AttemptID == moved.Dispatch.AttemptID {
		t.Fatal("attempt id: a different AttemptID argument left Dispatch.AttemptID unchanged")
	}
	if refused.CompoundExecution.Steps[0].Point == moved.CompoundExecution.Steps[0].Point {
		t.Fatal("point: a different point argument left the step point unchanged")
	}
	if refused.CompoundExecution.Steps[0].Reason == moved.CompoundExecution.Steps[0].Reason {
		t.Fatal("reason: a different reason argument left the step reason unchanged")
	}
	if refused.Dispatch.Delivery != operation.DeliveryNotSent || refused.CompoundExecution.Steps[0].Attempted {
		t.Fatalf("not sent: delivery %s attempted %v", refused.Dispatch.Delivery, refused.CompoundExecution.Steps[0].Attempted)
	}
	lost := Unknown("attempt:sunspec-limit:one", at, "WMaxLimPct", base)
	if lost.Dispatch.Delivery != operation.DeliveryUnknown || lost.Outcome != operation.OutcomeIndeterminate {
		t.Fatalf("unknown delivery: %+v", lost.Dispatch)
	}
	if refused.Dispatch.Delivery == lost.Dispatch.Delivery {
		t.Fatal("delivery: NotSent and Unknown produced the same delivery")
	}

	accepted := Acknowledged("attempt:sunspec-limit:one", at, later, "WMaxLimPct", operation.AckAccepted, base)
	rejected := Acknowledged("attempt:sunspec-limit:one", at, later, "WMaxLimPct", operation.AckRejected, base)
	if accepted.Acknowledgement.State == rejected.Acknowledgement.State {
		t.Fatal("ack state: AckAccepted and AckRejected produced the same acknowledgement")
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("Acknowledged validate: %v", err)
	}
	noted := Note(accepted, other)
	if noted.Readback != nil || len(noted.OutcomeEvidence) != 2 {
		t.Fatalf("note: readback=%v evidence=%d; a note must not become a readback", noted.Readback != nil, len(noted.OutcomeEvidence))
	}

	// CompoundExecution is a pointer, so each readback starts from its own acknowledgement.
	confirmed := WithReadback(Acknowledged("attempt:sunspec-limit:one", at, later, "WMaxLimPct", operation.AckAccepted, base), testSnapshot("snapshot:one"), testCandidate("candidate:one"), testRoute("binding:one", "source:one", "epoch:one", "1"), operation.ReadbackConfirms, other)
	conflict := WithReadback(Acknowledged("attempt:sunspec-limit:two", at, later, "WMaxLim_Ena", operation.AckAccepted, base), testSnapshot("snapshot:two"), testCandidate("candidate:two"), testRoute("binding:two", "source:two", "epoch:two", "2"), operation.ReadbackContradicts, other)
	if confirmed.Outcome != operation.OutcomeApplied || conflict.Outcome != operation.OutcomeConflict {
		t.Fatalf("relation: outcomes %s %s", confirmed.Outcome, conflict.Outcome)
	}
	if confirmed.Readback.Relation == conflict.Readback.Relation {
		t.Fatal("readback relation: confirms and contradicts produced the same relation")
	}
	if confirmed.Readback.CandidateID == conflict.Readback.CandidateID {
		t.Fatal("candidate id: a different candidate left CandidateID unchanged")
	}
	if confirmed.Readback.SnapshotID == conflict.Readback.SnapshotID {
		t.Fatal("snapshot id: a different snapshot left SnapshotID unchanged")
	}
	if confirmed.Readback.BindingID == conflict.Readback.BindingID {
		t.Fatal("binding: a different route binding left BindingID unchanged")
	}
	if confirmed.Readback.SourceID == conflict.Readback.SourceID {
		t.Fatal("source: a different route source left SourceID unchanged")
	}
	if confirmed.Readback.SourceEpochID == conflict.Readback.SourceEpochID {
		t.Fatal("epoch: a different route epoch left SourceEpochID unchanged")
	}
	if confirmed.Readback.DriverGeneration == conflict.Readback.DriverGeneration {
		t.Fatal("generation: a different route generation left DriverGeneration unchanged")
	}
	if err := confirmed.Readback.Validate(); err != nil {
		t.Fatalf("readback validate: %v", err)
	}
	if len(confirmed.CompoundExecution.Steps) != 2 || confirmed.CompoundExecution.Steps[1].Point != "readback" {
		t.Fatalf("readback step: %+v", confirmed.CompoundExecution.Steps)
	}
}

func moment(tick string) Moment {
	return Moment{
		Wall: wallNow(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
		Mono: semreg.MonotonicPoint{ClockEpochID: monoEpoch, Nanoseconds: semreg.Uint64(tick)},
	}
}

func testSnapshot(id semreg.SnapshotID) semreg.Snapshot {
	return semreg.Snapshot{
		SnapshotID: id, EvaluatedAt: wallNow(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
		EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: monoEpoch, Nanoseconds: "3"},
		Revisions:         semreg.RevisionVector{Semantic: "2", Identity: "1", Facts: "1", Services: "1", Capabilities: "1"},
	}
}

func testCandidate(id semreg.CandidateID) semreg.FactCandidate {
	return semreg.FactCandidate{CandidateID: id, Revision: "1"}
}

func testRoute(binding semreg.NativeBindingID, source semreg.SourceID, epoch semreg.SourceEpochID, generation semreg.Uint64) operation.Route {
	return operation.Route{
		CapabilityInstance: "capability:sunspec-limit:one", ServiceInstance: "service:sunspec-inverter:one",
		BindingID: binding, SourceID: source, SourceEpochID: epoch, DriverGeneration: generation,
	}
}
