package modbusreg

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateHuaweiSDongleOfflineObservationSelectsUnderTheObservedModel(t *testing.T) {
	for _, model := range []string{"S-DongleA-05", "S-DongleB-03", "S-DongleB-06"} {
		decision := EvaluateHuaweiSDongleOfflineObservation(HuaweiSDongleOfflineObservation{
			UnitID: 100, Model: model, Firmware: "V200R025C00SPC121", ProtocolMajor: 5,
			ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
		})
		if !decision.Matched() {
			t.Fatalf("%s: not matched: %v", model, decision.Reason())
		}
		selection := decision.Selection()
		if selection.Model() != strings.Replace(model, "S-Dongle", "SDongle", 1) {
			t.Fatalf("%s: selection recorded model %q", model, selection.Model())
		}
		floor, ok := selection.Floor()
		if !ok {
			t.Fatalf("%s: no floor recorded", model)
		}
		want := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
			Family: HuaweiSDongleCanonicalClass, Scope: HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: 100},
			Model: selection.Model(), Software: "V200R025C00SPC121",
		})
		wantFloor, _ := want.Floor()
		if model == "S-DongleB-03" {
			for _, id := range floor.EditionIDs() {
				if id == "s-dongle-v200r022cc10-02-2023-04-20" {
					t.Fatalf("%s: inherited the edition that excludes it: %v", model, floor.EditionIDs())
				}
			}
		}
		if !reflect.DeepEqual(floor.EditionIDs(), wantFloor.EditionIDs()) {
			t.Fatalf("%s: editions %v, want %v", model, floor.EditionIDs(), wantFloor.EditionIDs())
		}
	}
}

func TestEvaluateHuaweiSDongleOfflineObservationRecognizesFloorAdmittedDefaultDeniedTuples(t *testing.T) {
	tests := []struct {
		name    string
		input   HuaweiSDongleOfflineObservation
		matched bool
		variant string
		reason  HuaweiSDongleOfflineReason
	}{
		{
			name: "A05 current documented tuple",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleA-05", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 4, ChildCount: 2,
			},
			matched: true, variant: "S-DongleA-05", reason: HuaweiSDongleOfflineMatched,
		},
		{
			name: "A05 V200R022C10 build falls under the one floor and remains default denied",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleA-05\x00 ", Firmware: "V200R022C10SPC312 ", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			matched: true, variant: "S-DongleA-05", reason: HuaweiSDongleOfflineMatched,
		},
		{
			name: "child unit is not a gateway observation",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 1, Model: "S-DongleA-05", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineUnitMismatch,
		},
		{
			name: "model must be exact",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleA-05-Pro", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineModelMismatch,
		},
		{
			name: "build above the floor that no edition names",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-03", Firmware: "V200R025C00SPC121", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			matched: true, variant: "S-DongleB-03", reason: HuaweiSDongleOfflineMatched,
		},
		{
			name: "build below the V200R022C10 floor",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-03", Firmware: "V200R022C00SPC900", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineFirmwareMismatch,
		},
		{
			name: "malformed firmware",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-03", Firmware: "V200R025C00SPC12", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineFirmwareMismatch,
		},
		{
			name: "compatible release revision above the observed one",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-06", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 1,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			matched: true, variant: "S-DongleB-06", reason: HuaweiSDongleOfflineMatched,
		},
		{
			name: "main revision differs",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-06", Firmware: "V200R025C00SPC120", ProtocolMajor: 6, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineProtocolMismatch,
		},
		{
			name: "search incomplete",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-06", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 1, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineSearchIncomplete,
		},
		{
			name: "sequence changes",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-06", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 8, Capacity: 1, ChildCount: 1,
			},
			reason: HuaweiSDongleOfflineSequenceMismatch,
		},
		{
			name: "child count exceeds capacity",
			input: HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: "S-DongleB-06", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 2,
			},
			reason: HuaweiSDongleOfflineCapacityMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := EvaluateHuaweiSDongleOfflineObservation(test.input)
			if decision.Matched() != test.matched || decision.Reason() != test.reason || decision.DefaultDenied() != true {
				t.Fatalf("decision=(matched=%t reason=%q defaultDenied=%t), want (matched=%t reason=%q defaultDenied=true)", decision.Matched(), decision.Reason(), decision.DefaultDenied(), test.matched, test.reason)
			}
			if test.matched && (decision.CanonicalClass() != "S-Dongle" || decision.ModelVariant() != test.variant || decision.ProfileID() != "huawei.sdongle.readonly.v1") {
				t.Fatalf("unexpected recognized decision: %#v", decision)
			}
			if selection := decision.Selection(); test.matched {
				if floor, ok := selection.Floor(); !ok || floor.Minimum != "V200R022C10" || floor.Domain != HuaweiGatewayFloorLine ||
					selection.Reason() != HuaweiGatewayAtOrAboveFloor || selection.Grade() != HuaweiGradeHypothesis {
					t.Fatalf("matched tuple selection %+v", selection)
				}
			}
			if !test.matched && (decision.CanonicalClass() != "" || decision.ProfileID() != "") {
				t.Fatalf("unmatched decision must not expose a profile: %#v", decision)
			}
		})
	}
}
