package modbusreg

import (
	"reflect"
	"testing"
)

func lunaInput() LunaOptionalInput {
	return LunaOptionalInput{
		Binding:          LunaOptionalBinding{Parent: LunaOptionalIdentity{"parent-family", "parent-firmware", "P"}, Group: LunaOptionalIdentity{"battery-family", "group-firmware", "G"}, Attachment: "attached", ParentEpoch: "PE", GroupEpoch: "GE", EvidenceRevision: "public-revision"},
		ParentIdentified: true, EpochsAssociated: true, Event: LunaDocumentaryPresence,
		ObservationParentEpoch: "PE", ObservationGroupEpoch: "GE", Now: 10, Started: 9,
		Evidence: []LunaOptionalEvidence{{Reference: "synthetic-presence", NativeOutcome: "documentary presence", ObservedAt: 10}},
		Policy:   LunaOptionalPolicy{MaxRecords: 4, MaxBytes: 2048, Deadline: 20, FreshnessAge: 10, RetentionAge: 30, HistoryRecords: 8, HistoryBytes: 8192},
	}
}
func lunaDenied(t *testing.T, s LunaOptionalState) {
	t.Helper()
	if s.Outcome() != "NO_ADMISSIBLE_PROFILE" || s.CatalogRegistered() || s.AutomaticRuntimeAdmission() || s.SupportClaim() || s.WriteAuthority() || s.Qualified() {
		t.Fatal("admission escaped denial")
	}
}
func TestLunaOptionalStateTransitions(t *testing.T) {
	in := lunaInput()
	s := ReduceLunaOptionalState(LunaOptionalState{}, in)
	if s.State() != LunaPresentUnqualified || len(s.History()) != 1 {
		t.Fatal(s)
	}
	for _, event := range []LunaOptionalEvent{LunaOptionalFailure, LunaMalformedEvidence} {
		f := in
		f.Event = event
		f.Now = 12
		f.Evidence = []LunaOptionalEvidence{{"synthetic-failure", "optional exception", 12}}
		got := ReduceLunaOptionalState(s, f)
		if got.State() != LunaTemporarilyUnreadable || !got.ParentIdentified() || got.History()[0].Evidence.ObservedAt != 10 {
			t.Fatal(got)
		}
		lunaDenied(t, got)
	}
	for _, event := range []LunaOptionalEvent{LunaInventoryOnly, LunaNoObservation, LunaOptionalEvent("qualified"), LunaOptionalEvent("observed absent")} {
		in.Event = event
		got := ReduceLunaOptionalState(LunaOptionalState{}, in)
		if got.State() != LunaUnknown {
			t.Fatal(event, got)
		}
		lunaDenied(t, got)
	}
	lunaDenied(t, s)
}
func TestLunaOptionalStateEveryBindingAxis(t *testing.T) {
	changes := map[string]func(*LunaOptionalBinding){
		"parent model":      func(b *LunaOptionalBinding) { b.Parent.Model = "other" },
		"parent firmware":   func(b *LunaOptionalBinding) { b.Parent.Firmware = "other" },
		"parent unit":       func(b *LunaOptionalBinding) { b.Parent.Unit = "other" },
		"group model":       func(b *LunaOptionalBinding) { b.Group.Model = "other" },
		"group firmware":    func(b *LunaOptionalBinding) { b.Group.Firmware = "other" },
		"group unit":        func(b *LunaOptionalBinding) { b.Group.Unit = "other" },
		"attachment":        func(b *LunaOptionalBinding) { b.Attachment = "other" },
		"parent epoch":      func(b *LunaOptionalBinding) { b.ParentEpoch = "other" },
		"group epoch":       func(b *LunaOptionalBinding) { b.GroupEpoch = "other" },
		"evidence revision": func(b *LunaOptionalBinding) { b.EvidenceRevision = "other" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			in := lunaInput()
			old := ReduceLunaOptionalState(LunaOptionalState{}, in)
			change(&in.Binding)
			in.ObservationParentEpoch = in.Binding.ParentEpoch
			in.ObservationGroupEpoch = in.Binding.GroupEpoch
			got := ReduceLunaOptionalState(old, in)
			if got.State() != LunaIdentityChanged || len(got.History()) != 1 || got.History()[0].Applicable {
				t.Fatal(got)
			}
			next := ReduceLunaOptionalState(got, in)
			if next.State() != LunaPresentUnqualified || next.History()[0].Applicable {
				t.Fatal(next)
			}
			lunaDenied(t, got)
			lunaDenied(t, next)
		})
	}
}
func TestLunaOptionalStateTimeAndDetachedHistory(t *testing.T) {
	in := lunaInput()
	s := ReduceLunaOptionalState(LunaOptionalState{}, in)
	in.Evidence[0].Reference = "changed"
	h := s.History()
	h[0].Evidence.Reference = "changed"
	if s.History()[0].Evidence.Reference != "synthetic-presence" {
		t.Fatal("alias")
	}
	in.Event = LunaOptionalFailure
	in.Now = 21
	in.Policy.Deadline = 25
	in.Evidence = nil
	s = ReduceLunaOptionalState(s, in)
	if s.State() != LunaTemporarilyUnreadable || s.History()[0].Fresh || s.History()[0].Evidence.ObservedAt != 10 {
		t.Fatal("failure refreshed age", s)
	}
	in.Now = 41
	in.Policy = LunaOptionalPolicy{}
	s = ReduceLunaOptionalState(s, in)
	if len(s.History()) != 0 {
		t.Fatal("missing policy extended retention")
	}
}
func TestLunaOptionalStateRejectsAndBounds(t *testing.T) {
	cases := map[string]func(*LunaOptionalInput){
		"missing policy":      func(i *LunaOptionalInput) { i.Policy = LunaOptionalPolicy{} },
		"records":             func(i *LunaOptionalInput) { i.Policy.MaxRecords = 1; i.Evidence = append(i.Evidence, i.Evidence[0]) },
		"bytes":               func(i *LunaOptionalInput) { i.Policy.MaxBytes = 1 },
		"deadline":            func(i *LunaOptionalInput) { i.Policy.Deadline = i.Now },
		"future evidence":     func(i *LunaOptionalInput) { i.Evidence[0].ObservedAt = 11 },
		"missing evidence":    func(i *LunaOptionalInput) { i.Evidence = nil },
		"missing reference":   func(i *LunaOptionalInput) { i.Evidence[0].Reference = "" },
		"missing parent":      func(i *LunaOptionalInput) { i.ParentIdentified = false },
		"incoherent":          func(i *LunaOptionalInput) { i.EpochsAssociated = false },
		"missing identity":    func(i *LunaOptionalInput) { i.Binding.Group.Firmware = "" },
		"backward evaluation": func(i *LunaOptionalInput) { i.Now = 8 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			in := lunaInput()
			old := ReduceLunaOptionalState(LunaOptionalState{}, in)
			change(&in)
			got := ReduceLunaOptionalState(old, in)
			if got.State() == LunaPresentUnqualified {
				t.Fatal(got)
			}
			lunaDenied(t, got)
		})
	}
	in := lunaInput()
	old := ReduceLunaOptionalState(LunaOptionalState{}, in)
	for _, parent := range []bool{true, false} {
		late := in
		if parent {
			late.ObservationParentEpoch = "retired"
		} else {
			late.ObservationGroupEpoch = "retired"
		}
		got := ReduceLunaOptionalState(old, late)
		if !reflect.DeepEqual(got.History(), old.History()) || got.State() != old.State() {
			t.Fatal("retired overwrite")
		}
	}
	in.Policy.HistoryRecords = 1
	in.Now = 11
	in.Evidence[0].ObservedAt = 11
	got := ReduceLunaOptionalState(old, in)
	if len(got.History()) != 1 || got.History()[0].Evidence.ObservedAt != 11 {
		t.Fatal("eviction", got)
	}
	in.Policy.HistoryBytes = 1
	got = ReduceLunaOptionalState(old, in)
	if len(got.History()) != 0 {
		t.Fatal("byte bound")
	}
}

func TestLunaOptionalStateFencedHistoryNeverReactivates(t *testing.T) {
	in := lunaInput()
	s := ReduceLunaOptionalState(LunaOptionalState{}, in)
	original := in.Binding
	in.Binding.Group.Unit = "replacement"
	s = ReduceLunaOptionalState(s, in)
	s = ReduceLunaOptionalState(s, in)
	in.Binding = original
	s = ReduceLunaOptionalState(s, in)
	s = ReduceLunaOptionalState(s, in)
	if s.History()[0].Applicable {
		t.Fatal("old partition reactivated")
	}
}
func TestLunaOptionalStatePolicyAxes(t *testing.T) {
	for _, field := range []string{"MaxRecords", "MaxBytes", "Deadline", "FreshnessAge", "RetentionAge", "HistoryRecords", "HistoryBytes"} {
		t.Run(field, func(t *testing.T) {
			in := lunaInput()
			reflect.ValueOf(&in.Policy).Elem().FieldByName(field).SetInt(0)
			got := ReduceLunaOptionalState(LunaOptionalState{}, in)
			if got.State() != LunaUnknown || len(got.History()) != 0 {
				t.Fatal(got)
			}
		})
	}
}

func TestLunaOptionalStateRejectedEvaluationCannotExtendPolicy(t *testing.T) {
	in := lunaInput()
	prior := ReduceLunaOptionalState(LunaOptionalState{}, in)
	in.Now = 21
	in.Policy.FreshnessAge = 100
	got := ReduceLunaOptionalState(prior, in)
	if got.State() != LunaTemporarilyUnreadable || got.History()[0].Fresh {
		t.Fatal("expired evaluation extended freshness")
	}
	in.Now = 12
	in.Policy.Deadline = 50
	in.Policy.RetentionAge = 100
	in.Evidence[0].Reference = ""
	got = ReduceLunaOptionalState(prior, in)
	in.Now = 41
	in.Policy = LunaOptionalPolicy{}
	got = ReduceLunaOptionalState(got, in)
	if len(got.History()) != 0 {
		t.Fatal("invalid evidence extended retention")
	}
}
