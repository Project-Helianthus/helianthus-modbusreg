package modbusreg

import "testing"

func TestEvaluateHuaweiEMMAOfflineIdentityUsesFiniteAliasesAndBranchFloors(t *testing.T) {
	tests := []struct {
		name, model, firmware string
		matched                bool
		variant                string
		reason                 HuaweiEMMAOfflineReason
	}{
		{
			name: "A01 R024 floor", model: "EMMA-A01", firmware: "SmartHEMS V100R024C00SPC100",
			matched: true, variant: "EMMA-A01", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "A02 R025 above floor with terminal padding", model: "EMMA-A02\x00 ", firmware: "SmartHEMS V100R025C00SPC131 ",
			matched: true, variant: "EMMA-A02", reason: HuaweiEMMAOfflineMatched,
		},
		{
			name: "R025 below floor", model: "EMMA-A02", firmware: "SmartHEMS V100R025C00SPC101",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "unknown R branch", model: "EMMA-A02", firmware: "SmartHEMS V100R026C00SPC999",
			matched: false, reason: HuaweiEMMAOfflineFirmwareMismatch,
		},
		{
			name: "near model suffix", model: "EMMA-A02-Pro", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "canonical class alone is not an alias", model: "EMMA", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "case differs", model: "emma-a02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
		{
			name: "leading whitespace is not padding", model: " EMMA-A02", firmware: "SmartHEMS V100R025C00SPC131",
			matched: false, reason: HuaweiEMMAOfflineModelMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := EvaluateHuaweiEMMAOfflineIdentity(test.model, test.firmware)
			if decision.Matched() != test.matched || decision.Reason() != test.reason {
				t.Fatalf("decision=(matched=%t reason=%q), want (matched=%t reason=%q)", decision.Matched(), decision.Reason(), test.matched, test.reason)
			}
			if !test.matched {
				return
			}
			if decision.CanonicalClass() != "EMMA" || decision.ModelVariant() != test.variant ||
				decision.ProfileID() != "huawei.emma.readonly.v1" || !decision.DefaultDenied() {
				t.Fatalf("unexpected accepted decision: %+v", decision)
			}
		})
	}
}
