package modbusreg

type LunaOptionalStatus string

const (
	LunaUnknown               LunaOptionalStatus = "unknown"
	LunaPresentUnqualified    LunaOptionalStatus = "present-unqualified"
	LunaTemporarilyUnreadable LunaOptionalStatus = "temporarily unreadable"
	LunaIdentityChanged       LunaOptionalStatus = "identity changed"
)

type LunaOptionalEvent string

const (
	LunaNoObservation       LunaOptionalEvent = "no observation"
	LunaInventoryOnly       LunaOptionalEvent = "inventory only"
	LunaDocumentaryPresence LunaOptionalEvent = "documentary presence"
	LunaOptionalFailure     LunaOptionalEvent = "optional failure"
	LunaMalformedEvidence   LunaOptionalEvent = "malformed evidence"
)

type LunaOptionalIdentity struct{ Model, Firmware, Unit string }
type LunaOptionalBinding struct {
	Parent, Group                                         LunaOptionalIdentity
	Attachment, ParentEpoch, GroupEpoch, EvidenceRevision string
}
type LunaOptionalPolicy struct {
	MaxRecords, MaxBytes                 int
	Deadline, FreshnessAge, RetentionAge int64
	HistoryRecords, HistoryBytes         int
}
type LunaOptionalEvidence struct {
	Reference, NativeOutcome string
	ObservedAt               int64
}
type LunaOptionalInput struct {
	Binding                                       LunaOptionalBinding
	ParentIdentified, EpochsAssociated            bool
	ObservationParentEpoch, ObservationGroupEpoch string
	Event                                         LunaOptionalEvent
	Evidence                                      []LunaOptionalEvidence
	Started, Now                                  int64
	Policy                                        LunaOptionalPolicy
}
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

func (s LunaOptionalState) State() LunaOptionalStatus {
	if s.status == "" {
		return LunaUnknown
	}
	return s.status
}
func (s LunaOptionalState) ParentIdentified() bool { return s.parentIdentified }
func (s LunaOptionalState) History() []LunaOptionalHistory {
	return append([]LunaOptionalHistory(nil), s.history...)
}
func (LunaOptionalState) Outcome() string                 { return "NO_ADMISSIBLE_PROFILE" }
func (LunaOptionalState) CatalogRegistered() bool         { return false }
func (LunaOptionalState) AutomaticRuntimeAdmission() bool { return false }
func (LunaOptionalState) SupportClaim() bool              { return false }
func (LunaOptionalState) WriteAuthority() bool            { return false }
func (LunaOptionalState) Qualified() bool                 { return false }

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
