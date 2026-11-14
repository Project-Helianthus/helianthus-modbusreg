package huawei

import "testing"

func TestEvaluateHuaweiEMMAOfflineIdentityUsesFamilyRowsAndBranchFloors(t *testing.T) {
	tests := []struct {
		name, offering, model, firmware string
		matched                         bool
		variant                         string
		reason                          EMMAOfflineReason
		selection                       GatewaySelectionReason
	}{
		{
			name: "A01 R024 below floor", offering: "SmartHEMS", model: "EMMA-A01", firmware: "SmartHEMS V100R024C00SPC099",
			matched: false, reason: EMMAOfflineFirmwareMismatch,
		},
		{
			name: "A01 R024 floor", offering: "SmartHEMS", model: "EMMA-A01", firmware: "SmartHEMS V100R024C00SPC100",
			matched: true, variant: "EMMA-A01", reason: EMMAOfflineMatched,
		},
		{
			name: "A02 R024 below floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R024C00SPC099",
			matched: false, reason: EMMAOfflineFirmwareMismatch,
		},
		{
			name: "A02 R024 floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R024C00SPC100",
			matched: true, variant: "EMMA-A02", reason: EMMAOfflineMatched,
		},
		{
			name: "A02 R025 above floor with terminal padding", offering: "SmartHEMS\x00 ", model: "EMMA-A02\x00 ", firmware: "SmartHEMS V100R025C00SPC131 ",
			matched: true, variant: "EMMA-A02", reason: EMMAOfflineMatched,
		},
		{
			name: "R025 below floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC101",
			matched: false, reason: EMMAOfflineFirmwareMismatch,
		},
		{
			name: "R025 floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC102",
			matched: true, variant: "EMMA-A02", reason: EMMAOfflineMatched,
		},
		{
			name: "undocumented R branch admitted with grade Hypothesis", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R026C00SPC999",
			matched: true, variant: "EMMA-A02", reason: EMMAOfflineMatched, selection: GatewayNoDocumentedRelation,
		},
		{
			name: "other EMMA family token is its own variant", offering: "SmartHEMS", model: "EMMA-A02-Pro", firmware: "SmartHEMS V100R025C00SPC131",
			matched: true, variant: "EMMA-A02-Pro", reason: EMMAOfflineMatched, selection: GatewayAtOrAboveFloor,
		},
		{
			name: "family token with an empty remainder", offering: "SmartHEMS", model: "EMMA-", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineModelMismatch,
		},
		{
			name: "another family token", offering: "SmartHEMS", model: "EMMB-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineModelMismatch,
		},
		{
			name: "EMMA software text without the family label", offering: "SmartHEMS", model: "EMMA-A02", firmware: "V100R025C00SPC131",
			matched: false, reason: EMMAOfflineFirmwareMismatch,
		},
		{
			name: "EMMA software text without a build number", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00",
			matched: false, reason: EMMAOfflineFirmwareMismatch,
		},
		{
			name: "canonical class alone is not an alias", offering: "SmartHEMS", model: "EMMA", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineModelMismatch,
		},
		{
			name: "case differs", offering: "SmartHEMS", model: "emma-a02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineModelMismatch,
		},
		{
			name: "leading whitespace is not padding", offering: "SmartHEMS", model: " EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineModelMismatch,
		},
		{
			name: "empty offering rejects partial tuple", offering: "", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineOfferingMismatch,
		},
		{
			name: "offering non printable ASCII rejects partial tuple", offering: "Smart\nHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineOfferingMismatch,
		},
		{
			name: "offering longer than FC03 field rejects partial tuple", offering: "1234567890123456789012345678901", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: EMMAOfflineOfferingMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := EvaluateHuaweiEMMAOfflineIdentity(test.offering, test.model, test.firmware)
			if decision.Matched() != test.matched || decision.Reason() != test.reason {
				t.Fatalf("decision=(matched=%t reason=%q), want (matched=%t reason=%q)", decision.Matched(), decision.Reason(), test.matched, test.reason)
			}
			if !decision.DefaultDenied() {
				t.Fatalf("decision must remain default-denied: %+v", decision)
			}
			if !test.matched {
				if decision.ProfileID() != "" || decision.CanonicalClass() != "" {
					t.Fatalf("rejected decision exposes profile or canonical class: %+v", decision)
				}
				return
			}
			if decision.CanonicalClass() != EMMACanonicalClass || decision.ModelVariant() != test.variant ||
				decision.ProfileID() != EMMAReadOnlyProfileID {
				t.Fatalf("unexpected accepted decision: %+v", decision)
			}
			want := test.selection
			if want == "" {
				want = GatewayAtOrAboveFloor
			}
			if selection := decision.Selection(); !selection.Admitted() || selection.Reason() != want || selection.Grade() != GradeHypothesis ||
				decision.ObservedModel() != (test.variant == "EMMA-A01" || test.variant == "EMMA-A02") {
				t.Fatalf("selection %+v observed model %t", selection, decision.ObservedModel())
			}
		})
	}
}
