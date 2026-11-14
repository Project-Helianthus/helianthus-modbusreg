package huawei

// LunaOptionalStatus is the observation state of the optional LUNA2000
// battery group attached to a SUN2000 parent. No status is a qualification;
// the retention contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/luna2000-optional-group-v1.md.
type LunaOptionalStatus string

// Optional-group states. The contract's observed-absent and qualified states
// are not reachable in this revision.
const (
	// LunaUnknown means there is no usable current presence result; it is the
	// zero-value state.
	LunaUnknown LunaOptionalStatus = "unknown"
	// LunaPresentUnqualified means presence evidence exists under a valid
	// binding but exact qualification is missing.
	LunaPresentUnqualified LunaOptionalStatus = "present-unqualified"
	// LunaTemporarilyUnreadable means a previously present group failed to
	// evaluate; its bounded history survives and failure is never absence.
	LunaTemporarilyUnreadable LunaOptionalStatus = "temporarily unreadable"
	// LunaIdentityChanged fences the former binding after a binding change;
	// it neither identifies nor qualifies the replacement.
	LunaIdentityChanged LunaOptionalStatus = "identity changed"
)

// LunaOptionalEvent is the kind of observation supplied to
// ReduceLunaOptionalState. An unrecognized event yields LunaUnknown and is not
// retained.
type LunaOptionalEvent string

// Optional-group events.
const (
	// LunaNoObservation carries no presence evidence.
	LunaNoObservation LunaOptionalEvent = "no observation"
	// LunaInventoryOnly is a gateway listing without native battery evidence;
	// it never establishes presence.
	LunaInventoryOnly LunaOptionalEvent = "inventory only"
	// LunaDocumentaryPresence is presence evidence; with at least one
	// evidence record it yields LunaPresentUnqualified.
	LunaDocumentaryPresence LunaOptionalEvent = "documentary presence"
	// LunaOptionalFailure is a failed optional-group read, such as an
	// exception or timeout.
	LunaOptionalFailure LunaOptionalEvent = "optional failure"
	// LunaMalformedEvidence is partial or malformed group evidence.
	LunaMalformedEvidence LunaOptionalEvent = "malformed evidence"
)

// LunaOptionalIdentity is an exact model, firmware and unit identity. It is
// valid only when all three are non-empty.
type LunaOptionalIdentity struct{ Model, Firmware, Unit string }

// LunaOptionalBinding binds the battery Group to its SUN2000 Parent through
// an explicit Attachment, the separate ParentEpoch and GroupEpoch source
// associations, and the EvidenceRevision. Every field must be non-empty; any
// change to an established binding fences its history.
type LunaOptionalBinding struct {
	Parent, Group                                         LunaOptionalIdentity
	Attachment, ParentEpoch, GroupEpoch, EvidenceRevision string
}

// LunaOptionalPolicy is the caller-supplied finite budget of one evaluation;
// every field must be positive or the evaluation is rejected. MaxRecords and
// MaxBytes bound the evidence of one input, the bytes counted over the binding
// and evidence strings plus a fixed allowance per record. Deadline is an
// absolute tick that Now must precede. FreshnessAge and RetentionAge are ages
// in ticks; HistoryRecords and HistoryBytes bound the retained history, newest
// first.
type LunaOptionalPolicy struct {
	MaxRecords, MaxBytes                 int
	Deadline, FreshnessAge, RetentionAge int64
	HistoryRecords, HistoryBytes         int
}

// LunaOptionalEvidence is one native evidence record: a non-empty Reference
// and NativeOutcome, retained as given, and the caller tick it was observed
// at, which may not be later than the evaluation's Now.
type LunaOptionalEvidence struct {
	Reference, NativeOutcome string
	ObservedAt               int64
}

// LunaOptionalInput is one evaluation request. The observation epochs must
// equal the binding's epochs or the input is a retired result: only retention
// ageing is applied. ParentIdentified and EpochsAssociated must both hold for the
// binding to be usable. Started and Now are caller ticks on the same basis as
// the policy.
type LunaOptionalInput struct {
	Binding                                       LunaOptionalBinding
	ParentIdentified, EpochsAssociated            bool
	ObservationParentEpoch, ObservationGroupEpoch string
	Event                                         LunaOptionalEvent
	Evidence                                      []LunaOptionalEvidence
	Started, Now                                  int64
	Policy                                        LunaOptionalPolicy
}

// LunaOptionalHistory is one retained evidence record with the binding and
// event it arrived under. Applicable is false once its binding no longer
// matches the current one; Fresh reports whether its age is within the
// freshness budget at the last evaluation. History is never current evidence.
type LunaOptionalHistory struct {
	Binding           LunaOptionalBinding
	Event             LunaOptionalEvent
	Evidence          LunaOptionalEvidence
	Applicable, Fresh bool
}

// LunaOptionalState is immutable outside this package; its zero value is unknown.
type LunaOptionalState struct {
	status                  LunaOptionalStatus
	binding                 LunaOptionalBinding
	bound, parentIdentified bool
	history                 []LunaOptionalHistory
	policy                  LunaOptionalPolicy
	evaluatedAt             int64
}

// State returns the observation state, LunaUnknown for the zero value.
func (s LunaOptionalState) State() LunaOptionalStatus {
	if s.status == "" {
		return LunaUnknown
	}
	return s.status
}

// ParentIdentified reports whether the latest non-retired input positively
// identified a valid parent; it grants no battery capability.
func (s LunaOptionalState) ParentIdentified() bool { return s.parentIdentified }

// History returns a copy of the retained records, oldest first, within the
// last accepted retention policy.
func (s LunaOptionalState) History() []LunaOptionalHistory {
	return append([]LunaOptionalHistory(nil), s.history...)
}

// Outcome is always NO_ADMISSIBLE_PROFILE: no LUNA2000 profile exists.
func (LunaOptionalState) Outcome() string { return "NO_ADMISSIBLE_PROFILE" }

// CatalogRegistered is always false.
func (LunaOptionalState) CatalogRegistered() bool { return false }

// AutomaticRuntimeAdmission is always false.
func (LunaOptionalState) AutomaticRuntimeAdmission() bool { return false }

// SupportClaim is always false.
func (LunaOptionalState) SupportClaim() bool { return false }

// WriteAuthority is always false.
func (LunaOptionalState) WriteAuthority() bool { return false }

// Qualified is always false: the qualified transition is conditional and
// unmet.
func (LunaOptionalState) Qualified() bool { return false }

// ReduceLunaOptionalState interprets supplied documentary observations only.
// All times are caller-supplied nonnegative ticks on one monotonic time basis.
func ReduceLunaOptionalState(prior LunaOptionalState, in LunaOptionalInput) LunaOptionalState {
	s := prior
	s.history = prior.History()
	now := in.Now
	if now < s.evaluatedAt {
		now = s.evaluatedAt
	}
	s.evaluatedAt = now
	s.retain(now)
	// Reject retired results before allowing them to alter binding or policy.
	if in.ObservationParentEpoch != in.Binding.ParentEpoch || in.ObservationGroupEpoch != in.Binding.GroupEpoch {
		return s
	}
	bindingLimit := s.policy.MaxBytes
	if in.Policy.valid() {
		bindingLimit = in.Policy.MaxBytes
	}
	validBinding := in.ParentIdentified && in.EpochsAssociated && in.Binding.valid()
	s.parentIdentified = in.ParentIdentified && in.Binding.Parent.valid()
	if !validBinding {
		s.status = LunaUnknown
		s.bound = false
		for i := range s.history {
			s.history[i].Applicable = false
		}
		return s
	}
	if s.bound && s.binding != in.Binding {
		if bindingLimit > 0 && lunaRecordBytes(in.Binding, LunaOptionalEvidence{}) <= bindingLimit {
			s.binding = in.Binding
		} else {
			s.binding = LunaOptionalBinding{}
			s.bound = false
		}
		s.status = LunaIdentityChanged
		for i := range s.history {
			s.history[i].Applicable = false
		}
		return s
	}
	if bindingLimit <= 0 || lunaRecordBytes(in.Binding, LunaOptionalEvidence{}) > bindingLimit {
		if s.State() == LunaPresentUnqualified || s.State() == LunaTemporarilyUnreadable {
			s.status = LunaTemporarilyUnreadable
		} else {
			s.status = LunaUnknown
		}
		return s
	}
	s.binding = in.Binding
	s.bound = true
	policyOK := in.Policy.valid()
	valid := policyOK && in.Now >= 0 && in.Now >= prior.evaluatedAt && in.Started >= 0 && in.Started <= in.Now && in.Now < in.Policy.Deadline && len(in.Evidence) <= in.Policy.MaxRecords
	bytes := 0
	if valid {
		for _, e := range in.Evidence {
			n := lunaRecordBytes(in.Binding, e)
			if e.Reference == "" || e.NativeOutcome == "" || e.ObservedAt < 0 || e.ObservedAt > in.Now || n > in.Policy.MaxBytes-bytes {
				valid = false
				break
			}
			bytes += n
		}
	}
	failure := !valid || in.Event == LunaOptionalFailure || in.Event == LunaMalformedEvidence
	if failure {
		if s.State() == LunaPresentUnqualified || s.State() == LunaTemporarilyUnreadable {
			s.status = LunaTemporarilyUnreadable
		} else {
			s.status = LunaUnknown
		}
	} else {
		switch in.Event {
		case LunaDocumentaryPresence:
			if len(in.Evidence) > 0 {
				s.status = LunaPresentUnqualified
			} else {
				s.status = LunaUnknown
			}
		case LunaInventoryOnly, LunaNoObservation:
			s.status = LunaUnknown
		default:
			// Unknown events cannot introduce future qualification or absence semantics.
			s.status = LunaUnknown
			valid = false
		}
	}
	if valid {
		s.policy = in.Policy
		s.retain(now)
		for _, e := range in.Evidence {
			s.history = append(s.history, LunaOptionalHistory{Binding: in.Binding, Event: in.Event, Evidence: e, Applicable: true})
		}
	}
	s.retain(now)
	return s
}
func (i LunaOptionalIdentity) valid() bool { return i.Model != "" && i.Firmware != "" && i.Unit != "" }
func (b LunaOptionalBinding) valid() bool {
	return b.Parent.valid() && b.Group.valid() && b.Attachment != "" && b.ParentEpoch != "" && b.GroupEpoch != "" && b.EvidenceRevision != ""
}
func (p LunaOptionalPolicy) valid() bool {
	return p.MaxRecords > 0 && p.MaxBytes > 0 && p.Deadline > 0 && p.FreshnessAge > 0 && p.RetentionAge > 0 && p.HistoryRecords > 0 && p.HistoryBytes > 0
}
func lunaRecordBytes(b LunaOptionalBinding, e LunaOptionalEvidence) int {
	// Charge all retained strings and a conservative fixed scalar allowance.
	return 64 + len(b.Parent.Model) + len(b.Parent.Firmware) + len(b.Parent.Unit) + len(b.Group.Model) + len(b.Group.Firmware) + len(b.Group.Unit) + len(b.Attachment) + len(b.ParentEpoch) + len(b.GroupEpoch) + len(b.EvidenceRevision) + len(e.Reference) + len(e.NativeOutcome)
}
func (s *LunaOptionalState) retain(now int64) {
	if !s.policy.valid() {
		s.history = nil
		return
	}
	kept := make([]LunaOptionalHistory, 0)
	bytes := 0
	// Newest entries win; eviction never changes their original observation time.
	for i := len(s.history) - 1; i >= 0; i-- {
		h := s.history[i]
		age := now - h.Evidence.ObservedAt
		n := lunaRecordBytes(h.Binding, h.Evidence)
		if age < 0 || age > s.policy.RetentionAge || len(kept) >= s.policy.HistoryRecords || n > s.policy.HistoryBytes-bytes {
			continue
		}
		h.Applicable = h.Applicable && s.bound && h.Binding == s.binding
		h.Fresh = age <= s.policy.FreshnessAge
		kept = append(kept, h)
		bytes += n
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	s.history = kept
}
