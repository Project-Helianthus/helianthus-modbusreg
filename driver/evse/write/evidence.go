package write

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

const evidenceOwner semreg.DefinitionID = "modbus.evse.charger"

// The kinds of evidence a completion carries.
const (
	KindRequest   semreg.DefinitionID = "native.tesla.wc3.current_limit.request"
	KindAck       semreg.DefinitionID = "native.tesla.wc3.current_limit.acknowledgement"
	KindReturned  semreg.DefinitionID = "native.tesla.wc3.current_limit.returned"
	KindRelease   semreg.DefinitionID = "native.tesla.wc3.current_limit.release"
	KindRefusal   semreg.DefinitionID = "native.tesla.wc3.current_limit.refusal"
	KindConfigure semreg.DefinitionID = "native.tesla.wc3.current_limit.configured"
)

func evidence(kind semreg.DefinitionID, material string) semreg.EvidenceRef {
	sum := sha256.Sum256([]byte(material))
	return semreg.EvidenceRef{Owner: evidenceOwner, Kind: kind, Digest: semreg.Digest("sha256:" + hex.EncodeToString(sum[:])),
		Contract: operation.ContractOperationV1, Access: semreg.EvidenceAccessAuthorized, Redaction: semreg.RedactionMetadataOnly}
}

func ordered(refs ...semreg.EvidenceRef) []semreg.EvidenceRef {
	seen := map[semreg.EvidenceRef]bool{}
	var out []semreg.EvidenceRef
	for _, ref := range refs {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Digest < b.Digest
	})
	return out
}

func requestMaterial(r tesla.ProvisionalCurrentLimitRequest) string {
	return fmt.Sprintf("sent:%d:%d:%t", r.LimitCurrentMaxAmps, r.LimitTimeoutSeconds, r.InhibitCharging)
}

func returnedMaterial(r tesla.ProvisionalCurrentLimitReadback, matches bool) string {
	return fmt.Sprintf("returned:%d/%t:%d/%t:%t/%t:%d/%t:matches=%t", r.LimitCurrentMaxAmps, r.HasLimitCurrent, r.LimitTimeoutSeconds, r.HasLimitTimeout,
		r.InhibitCharging, r.HasInhibit, r.ConfiguredCurrentAmps, r.HasConfiguredCurrent, matches)
}

func (w *Writer) wall(t time.Time) semreg.TimePoint {
	return semreg.TimePoint{UnixNanoseconds: semreg.Int64(strconv.FormatInt(t.UTC().UnixNano(), 10)), ClockID: "clock.utc", UncertaintyNS: "0"}
}

func (w *Writer) at(t time.Time) semreg.EvaluationContext {
	mono := t.Sub(w.origin)
	if mono < 0 {
		mono = 0
	}
	return semreg.EvaluationContext{EvaluatedAt: w.wall(t),
		EvaluateMonotonic: semreg.MonotonicPoint{ClockEpochID: w.cfg.ClockEpoch, Nanoseconds: semreg.Uint64(strconv.FormatInt(mono.Nanoseconds(), 10))}}
}

func (w *Writer) attempt() semreg.AttemptID {
	w.mu.Lock()
	w.attempts++
	n := w.attempts
	w.mu.Unlock()
	p := w.cfg.Publication
	sum := sha256.Sum256([]byte(fmt.Sprint(p.Asset, p.Source, p.Epoch, p.Generation, n)))
	return semreg.AttemptID("attempt:modbus-evse:" + hex.EncodeToString(sum[:16]))
}

func outcomeOf(level Level) operation.Outcome {
	switch level {
	case LevelReadbackConfirmed:
		return operation.OutcomeApplied
	case LevelConflict:
		return operation.OutcomeConflict
	case LevelProtocolAck:
		return operation.OutcomeAcknowledgedUnverified
	case LevelNoEffect:
		return operation.OutcomeNoEffect
	case LevelUnknown:
		return operation.OutcomeIndeterminate
	}
	return operation.OutcomeFailedNoContact
}

// completion turns the result of a command into the evidence the router takes:
// the delivery, the acknowledgement of the t26 and, for a set or a pause whose
// returned t28 was published, the exact later snapshot. The t25 that was sent
// and the t28 that was returned are separate evidence. A release carries no
// snapshot and no readback.
func (w *Writer) completion(d driver.Dispatch, r Result) driver.Completion {
	sent := r.T25Sent()
	now := w.clock.Now()
	started := r.Started
	if started.IsZero() {
		started = now
	}
	if !r.Dispatched.IsZero() && r.Dispatched.Before(started) {
		started = r.Dispatched
	}
	var refs []semreg.EvidenceRef
	steps := []driver.CompoundStep{}
	if r.HasPrerequisite {
		steps = append(steps, driver.CompoundStep{Point: "prerequisite_t27_t28", Attempted: true, Reason: "read only"})
		refs = append(refs, evidence(KindConfigure, returnedMaterial(readbackOf(r.Prerequisite), false)))
	}
	dispatch := &operation.DispatchEvidence{AttemptID: w.attempt(), Started: w.at(started), Delivery: operation.DeliveryNotSent}
	completion := driver.Completion{Outcome: outcomeOf(r.Level)}
	if !sent {
		step := driver.CompoundStep{Point: "t25_t26", Attempted: false, Reason: r.Reason}
		steps = append(steps, step)
		refs = append(refs, evidence(KindRefusal, "refusal:"+r.Reason))
		dispatch.Evidence = ordered(evidence(KindRefusal, "refusal:"+r.Reason))
		completion.Dispatch, completion.OutcomeEvidence = dispatch, ordered(refs...)
		completion.CompoundExecution = &driver.CompoundExecution{Steps: steps}
		return completion
	}
	request := evidence(KindRequest, requestMaterial(r.Sent))
	refs = append(refs, request)
	dispatch.Delivery, dispatch.PossibleSideEffect, dispatch.Evidence = operation.DeliveryUnknown, true, ordered(request)
	step := driver.CompoundStep{Point: "t25_t26", Attempted: true, Delivery: operation.DeliveryUnknown}
	if r.Native.Acknowledged || r.Level == LevelNoEffect {
		at := r.AckAt
		if at.IsZero() {
			at = started
		}
		state := operation.AckAccepted
		if r.Level == LevelNoEffect {
			state = operation.AckRejected
		}
		ack := evidence(KindAck, fmt.Sprintf("ack:%s:%d", state, r.Native.Set.RequestID))
		refs = append(refs, ack)
		completion.Acknowledgement = &operation.Acknowledgement{State: state, At: w.wall(at), Evidence: ordered(ack)}
		dispatch.Delivery, step.Delivery, step.Acknowledgement = operation.DeliverySent, operation.DeliverySent, state
		done := w.at(maxTime(started, r.Dispatched))
		dispatch.Completed = &done
	}
	steps = append(steps, step)
	if r.HasReturned {
		kind := KindReturned
		if r.Release {
			kind = KindRelease
		}
		returned := evidence(kind, returnedMaterial(r.Returned, r.Matches))
		refs = append(refs, returned)
		readStep := driver.CompoundStep{Point: "t27_t28", Attempted: true}
		if r.Snapshot != nil && r.Candidate != nil && !r.Release {
			relation := operation.ReadbackContradicts
			if r.Level == LevelReadbackConfirmed {
				relation = operation.ReadbackConfirms
			}
			snapshot := *r.Snapshot
			completion.ReadbackSnapshot = &snapshot
			completion.Readback = &operation.Readback{
				SnapshotID: snapshot.SnapshotID, Revisions: snapshot.Revisions, CandidateID: r.Candidate.CandidateID, CandidateRevision: r.Candidate.Revision,
				BindingID: d.Route.BindingID, SourceID: d.Route.SourceID, SourceEpochID: d.Route.SourceEpochID, DriverGeneration: d.Route.DriverGeneration,
				Relation: relation, Evaluation: semreg.EvaluationContext{EvaluatedAt: snapshot.EvaluatedAt, EvaluateMonotonic: snapshot.EvaluateMonotonic},
				Evidence: ordered(returned),
			}
			readStep.ReadbackRelation = relation
		}
		steps = append(steps, readStep)
	} else if r.Native.Acknowledged {
		steps = append(steps, driver.CompoundStep{Point: "t27_t28", Attempted: true, Reason: r.Reason})
	}
	completion.Dispatch, completion.OutcomeEvidence = dispatch, ordered(refs...)
	completion.CompoundExecution = &driver.CompoundExecution{Steps: steps}
	return completion
}

func readbackOf(record tesla.Gen3ReadRecord) tesla.ProvisionalCurrentLimitReadback {
	readback, _ := record.ProvisionalReadback()
	return readback
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
