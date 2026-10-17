package modbusreg

import "testing"

func TestEvaluateHuaweiEMMAOfflineIdentityUsesFiniteAliasesAndBranchFloors(t *testing.T) {
	tests := []struct {
		name, offering, model, firmware string
		matched                         bool
		variant                         string
		reason                          HuaweiEMMAOfflineReason
	}{
		{
			name: "A01 R024 below floor", offering: "SmartHEMS", model: "EMMA-A01", firmware: "SmartHEMS V100R024C00SPC099",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "A01 R024 floor", offering: "SmartHEMS", model: "EMMA-A01", firmware: "SmartHEMS V100R024C00SPC100",
			matched: true, variant: "EMMA-A01", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "A02 R024 below floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R024C00SPC099",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "A02 R024 floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R024C00SPC100",
			matched: true, variant: "EMMA-A02", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "A02 R025 above floor with terminal padding", offering: "SmartHEMS\x00 ", model: "EMMA-A02\x00 ", firmware: "SmartHEMS V100R025C00SPC131 ",
			matched: true, variant: "EMMA-A02", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "R025 below floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC101",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "R025 floor", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC102",
			matched: true, variant: "EMMA-A02", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "unknown R branch", offering: "SmartHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R026C00SPC999",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "near model suffix", offering: "SmartHEMS", model: "EMMA-A02-Pro", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "canonical class alone is not an alias", offering: "SmartHEMS", model: "EMMA", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "case differs", offering: "SmartHEMS", model: "emma-a02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "leading whitespace is not padding", offering: "SmartHEMS", model: " EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "empty offering rejects partial tuple", offering: "", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineOfferingMismatch,
		},
		{
			name: "offering non printable ASCII rejects partial tuple", offering: "Smart\nHEMS", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineOfferingMismatch,
		},
		{
			name: "offering longer than FC03 field rejects partial tuple", offering: "1234567890123456789012345678901", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineOfferingMismatch,
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
			if decision.CanonicalClass() != HuaweiEMMACanonicalClass || decision.ModelVariant() != test.variant ||
				decision.ProfileID() != HuaweiEMMAReadOnlyProfileID {
				t.Fatalf("unexpected accepted decision: %+v", decision)
			}
		})
	}
}
