package modbusreg

import (
	"context"
	"errors"
	"time"
)

type SDongleOfflineState string

const (
	SDongleOfflineReady   SDongleOfflineState = "ready"
	SDongleOfflineBlocked SDongleOfflineState = "blocked"
	SDongleOfflineDenied  SDongleOfflineState = "denied"
)

var (
	ErrSDongleOfflineInput       = errors.New("sdongle offline: invalid or excessive input")
	ErrSDongleOfflineAssociation = errors.New("sdongle offline: requested and returned keys differ")
)

// SDongleOfflineLookup reads supplied offline records only. It must obey ctx
// and the caller's input limits; it has no acquisition or sending authority.
type SDongleOfflineLookup func(context.Context, SDongleRouteKey) (SDongleRoutePlan, error)

// SDongleOfflineBounds limits one local lookup and the strings/reference entries
// accepted from it. These limits are independent of native documentary bounds.
type SDongleOfflineBounds struct {
	LookupTimeout time.Duration
	MaxInputBytes uint32
	MaxReferences uint16
}

// SDongleOfflineRecord retains a complete prior documentary success. EvaluatedAt
// is local attempt context, never a native capture time or a freshness renewal.
type SDongleOfflineRecord struct {
	Decision    SDongleRouteDecision
	EvaluatedAt time.Time
}

type SDongleOfflineRequest struct {
	Key         SDongleRouteKey
	Bounds      SDongleOfflineBounds
	AttemptedAt time.Time
	Previous    *SDongleOfflineRecord
}

// SDongleOfflineResult separates this attempt from detached last-known-good
// history. Retained keeps its original key, Current assertion and evaluation
// time; its presence does not assert freshness for the current request.
type SDongleOfflineResult struct {
	State    SDongleOfflineState
	Decision *SDongleRouteDecision
	Retained *SDongleOfflineRecord
}

func (SDongleOfflineResult) DefaultDenied() bool { return true }

// RunSDongleOfflineProvider composes one cooperative, bounded offline lookup
// with the merged route decision. Ready means documentary readiness only.
// An uncooperative callback cannot be forcibly interrupted; no goroutine or
// background work is created, and late results are rejected after it returns.
func RunSDongleOfflineProvider(ctx context.Context, req SDongleOfflineRequest, lookup SDongleOfflineLookup) (SDongleOfflineResult, error) {
	result := SDongleOfflineResult{State: SDongleOfflineBlocked}
	b := req.Bounds
	if ctx == nil || lookup == nil || req.AttemptedAt.IsZero() || b.LookupTimeout <= 0 || b.LookupTimeout > 15*time.Second || b.MaxInputBytes == 0 || b.MaxInputBytes > 1<<20 || b.MaxReferences == 0 || b.MaxReferences > 256 || !sdongleOfflineKeyComplete(req.Key) || !sdongleOfflineFits(b, sdongleOfflineKeyStrings(req.Key), nil) {
		return result, ErrSDongleOfflineInput
	}
	if req.Previous != nil {
		d := req.Previous.Decision
		if d.Outcome != SDonglePlanned || req.Previous.EvaluatedAt.IsZero() || !sdongleOfflineDecisionFits(b, d) {
			return result, ErrSDongleOfflineInput
		}
		result.Retained = &SDongleOfflineRecord{Decision: sdongleOfflineDetach(d), EvaluatedAt: req.Previous.EvaluatedAt}
	}
	work, cancel := context.WithTimeout(ctx, b.LookupTimeout)
	defer cancel()
	if err := work.Err(); err != nil {
		return result, err
	}
	plan, err := lookup(work, req.Key)
	// A reader may return usable-looking data after cancellation, or with an
	// error. Neither case may commit it as a new decision or retained record.
	if fence := work.Err(); fence != nil {
		return result, fence
	}
	if err != nil {
		return result, err
	}
	if !sdongleOfflinePlanFits(b, plan) {
		return result, ErrSDongleOfflineInput
	}
	if plan.Key != req.Key {
		return result, ErrSDongleOfflineAssociation
	}
	decision := DecideSDongleRoute(plan)
	if fence := work.Err(); fence != nil {
		return result, fence
	}
	result.Decision = &decision
	switch decision.Outcome {
	case SDonglePlanned:
		result.State = SDongleOfflineReady
		result.Retained = &SDongleOfflineRecord{Decision: sdongleOfflineDetach(decision), EvaluatedAt: req.AttemptedAt}
	case SDongleNoSend:
		result.State = SDongleOfflineDenied
	}
	return result, nil
}

func sdongleOfflineKeyComplete(k SDongleRouteKey) bool {
	return k.Family != "" && k.Model != "" && k.Firmware != "" && k.Role != "" && k.Request != "" && k.Scope != "" && k.Revision != ""
}

func sdongleOfflineDetach(d SDongleRouteDecision) SDongleRouteDecision {
	if d.Evidence.References != nil {
		d.Evidence.References = append([]string{}, d.Evidence.References...)
	}
	return d
}

func sdongleOfflineKeyStrings(k SDongleRouteKey) []string {
	p := k.Child.Parent
	return []string{k.Family, k.Model, k.Firmware, string(k.Role), string(k.Request), k.Scope, k.Revision, p.Family, p.Model, p.Firmware, string(p.Role), string(p.Request), p.Scope, p.Revision, k.Child.EvidenceRevision, k.Child.Qualification}
}

func sdongleOfflinePlanFits(b SDongleOfflineBounds, p SDongleRoutePlan) bool {
	return sdongleOfflineDecisionFits(b, SDongleRouteDecision{Key: p.Key, Observation: p.Observation, Evidence: p.Evidence})
}

func sdongleOfflineDecisionFits(b SDongleOfflineBounds, d SDongleRouteDecision) bool {
	fields := append(sdongleOfflineKeyStrings(d.Key), sdongleOfflineKeyStrings(d.Evidence.Key)...)
	fields = append(fields, d.Observation.Model, d.Observation.Firmware, string(d.Outcome), d.Reason, string(d.EvaluatorReason))
	for _, r := range []SDongleRouteReference{d.Evidence.Identity, d.Evidence.Protocol, d.Evidence.Allowlist, d.Evidence.Route} {
		fields = append(fields, r.Revision, r.Reference)
	}
	return sdongleOfflineFits(b, fields, d.Evidence.References)
}

func sdongleOfflineFits(b SDongleOfflineBounds, fields, refs []string) bool {
	if len(refs) > int(b.MaxReferences) {
		return false
	}
	remaining := uint64(b.MaxInputBytes)
	for _, values := range [][]string{fields, refs} {
		for _, s := range values {
			if uint64(len(s)) > remaining {
				return false
			}
			remaining -= uint64(len(s))
		}
	}
	return true
}
