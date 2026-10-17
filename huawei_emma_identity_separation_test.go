package modbusreg

import (
	"reflect"
	"strings"
	"testing"
)

// These synthetic vectors follow the EMMA candidate and child-inventory rules in
// helianthus-docs-modbus/protocols/huawei/gateway-readonly-v1.md.
// Parsed enrichment is an observation, not qualification or runtime admission.
func TestK20EMMARequiredIdentityIndependentOfOptionalInventory(t *testing.T) {
	identities := []struct {
		name, offering, model, firmware string
		matched                         bool
		reason                          HuaweiEMMAOfflineReason
	}{
		{"valid R024", "SmartHEMS", "EMMA-A01", "SmartHEMS V100R024C00SPC100", true, HuaweiEMMAOfflineMatched},
		{"valid R025 padded", "SmartHEMS\x00 ", "EMMA-A02\x00 ", "SmartHEMS V100R025C00SPC102\x00 ", true, HuaweiEMMAOfflineMatched},
		{"missing tuple", "", "", "", false, HuaweiEMMAOfflineOfferingMismatch},
		{"missing offering", "", "EMMA-A02", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineOfferingMismatch},
		{"malformed offering", "Smart\nHEMS", "EMMA-A02", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineOfferingMismatch},
		{"missing model", "SmartHEMS", "", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineModelMismatch},
		{"near EMMA string", "SmartHEMS", "EMMA-A02-extra", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineModelMismatch},
		{"SmartLogger model with EMMA firmware", "SmartHEMS", "SmartLogger", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineModelMismatch},
		{"S-Dongle model with EMMA firmware", "SmartHEMS", "S-DongleA-05", "SmartHEMS V100R025C00SPC102", false, HuaweiEMMAOfflineModelMismatch},
		{"missing firmware", "SmartHEMS", "EMMA-A02", "", false, HuaweiEMMAOfflineFirmwareMismatch},
		{"malformed firmware", "SmartHEMS", "EMMA-A02", "SmartHEMS V100R025C00SPC10x", false, HuaweiEMMAOfflineFirmwareMismatch},
		{"below branch floor", "SmartHEMS", "EMMA-A02", "SmartHEMS V100R025C00SPC101", false, HuaweiEMMAOfflineFirmwareMismatch},
		{"unknown branch", "SmartHEMS", "EMMA-A02", "SmartHEMS V100R026C00SPC999", false, HuaweiEMMAOfflineFirmwareMismatch},
		{"SmartLogger firmware", "SmartHEMS", "EMMA-A02", "V300R024C10SPC191", false, HuaweiEMMAOfflineFirmwareMismatch},
		{"S-Dongle firmware", "SmartHEMS", "EMMA-A02", "V200R025C00SPC120", false, HuaweiEMMAOfflineFirmwareMismatch},
	}
	for _, identity := range identities {
		t.Run(identity.name, func(t *testing.T) {
			for _, enrichment := range []struct {
				name       string
				self       string
				failure    string
				wantParsed bool
			}{
				{"EMMA inventory", "EMMA-A02", "", true},
				{"other EMMA variant inventory", "EMMA-A01", "", true},
				{"no response pages", "EMMA-A02", "absent", false},
				{"unstable guard", "EMMA-A02", "guard", false},
				{"malformed child", "EMMA-A02", "child", false},
				{"SmartLogger self", "SmartLogger", "", false},
				{"S-Dongle self", "S-DongleA-05", "", false},
			} {
				t.Run(enrichment.name, func(t *testing.T) {
					input := HuaweiEMMAQualificationInput{UnitID: 0, Offering: identity.offering, Model: identity.model, Firmware: identity.firmware}
					before := NewHuaweiEMMAQualificationCard().Evaluate(input)
					optional := k20Inventory(enrichment.self, "SmartHEMS V100R025C00SPC102")
					switch enrichment.failure {
					case "absent":
						optional.Pages = nil
					case "guard":
						optional.InventoryGuardAfter++
					case "child":
						optional.Pages[0].Objects[2].Value = []byte("1=SUN2000;5=")
					}
					inventory, err := ParseHuaweiEMMAOfflineInventory(optional)
					if (err == nil) != enrichment.wantParsed {
						t.Fatalf("optional parse error = %v, want parsed %t", err, enrichment.wantParsed)
					}
					if !inventory.DefaultDenied() {
						t.Fatal("optional inventory granted admission")
					}
					if enrichment.wantParsed {
						children := inventory.Children()
						if inventory.DeclaredChildren() != 1 || len(children) != 1 || children[0].Address() != "1" || children[0].Attribute("model") != "SUN2000" {
							t.Fatalf("native child observation lost: %#v", inventory)
						}
					} else if inventory.DeclaredChildren() != 0 || inventory.ChildCount() != 0 {
						t.Fatalf("failed enumeration returned partial inventory: %#v", inventory)
					}
					decision := EvaluateHuaweiEMMAOfflineIdentity(input.Offering, input.Model, input.Firmware)
					if decision.Matched() != identity.matched || decision.Reason() != identity.reason || !decision.DefaultDenied() {
						t.Fatalf("identity = %#v, want matched=%t reason=%s", decision, identity.matched, identity.reason)
					}
					result := NewHuaweiEMMAQualificationCard().Evaluate(input)
					if !reflect.DeepEqual(before, result) {
						t.Fatalf("optional enumeration changed identity disposition: before=%#v after=%#v", before, result)
					}
					if result.CandidateClass() != HuaweiEMMACanonicalClass || result.IdentityMatched() != identity.matched {
						t.Fatalf("wrong candidate identity: %#v", result)
					}
					if identity.matched {
						variant := strings.TrimRight(identity.model, "\x00 ")
						if decision.CanonicalClass() != HuaweiEMMACanonicalClass || decision.ProfileID() != HuaweiEMMAReadOnlyProfileID || decision.ModelVariant() != variant ||
							result.SelectedClass() != HuaweiEMMACanonicalClass || result.ModelVariant() != variant ||
							result.Readiness() != HuaweiQualificationEvidenceBlocked || result.Reason() != HuaweiQualificationIdentityMatchedCapabilitiesMissing ||
							!reflect.DeepEqual(result.MissingEvidence(), []string{"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture"}) {
							t.Fatalf("valid identity lost its evidence-blocked disposition: %#v / %#v", decision, result)
						}
					} else if decision.CanonicalClass() != "" || decision.ProfileID() != "" || decision.ModelVariant() != "" ||
						result.SelectedClass() != "" || result.ModelVariant() != "" || result.Readiness() != HuaweiQualificationInsufficientEvidence || result.Reason() != HuaweiQualificationIdentityInvalid {
						t.Fatalf("optional evidence repaired invalid identity: %#v / %#v", decision, result)
					}
					if retained, ok := result.Inventory(); ok || retained.ChildCount() != 0 {
						t.Fatalf("optional children promoted into qualification: %#v", retained)
					}
					k20RequireDenied(t, result)
				})
			}
		})
	}
}

func TestK20HuaweiFamilyEvidenceDoesNotFallBack(t *testing.T) {
	for _, test := range []struct {
		name, model, firmware     string
		emma, smartLogger, dongle bool
	}{
		{"EMMA", "EMMA-A02", "SmartHEMS V100R025C00SPC102", true, false, false},
		{"SmartLogger", "SmartLogger", "V300R024C10SPC191", false, true, false},
		{"S-Dongle", "S-DongleA-05", "V200R025C00SPC120", false, false, true},
		{"SmartLogger with EMMA firmware", "SmartLogger", "SmartHEMS V100R025C00SPC102", false, false, false},
		{"EMMA with SmartLogger firmware", "EMMA-A02", "V300R024C10SPC191", false, false, false},
		{"S-Dongle with EMMA firmware", "S-DongleA-05", "SmartHEMS V100R025C00SPC102", false, false, false},
		{"EMMA with S-Dongle firmware", "EMMA-A02", "V200R025C00SPC120", false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			optional := k20Inventory(test.model, test.firmware)
			emma := NewHuaweiEMMAQualificationCard().Evaluate(HuaweiEMMAQualificationInput{
				UnitID: 0, Offering: "SmartHEMS", Model: test.model, Firmware: test.firmware,
			})
			logger := NewHuaweiSmartLoggerQualificationCard().Evaluate(HuaweiSmartLoggerQualificationInput{
				UnitID: 0, Inventory: HuaweiSmartLoggerOfflineInventoryInput{
					ChangeCounterBefore: optional.InventoryGuardBefore, ChangeCounterAfter: optional.InventoryGuardAfter, Pages: optional.Pages,
				},
			})
			dongle := EvaluateHuaweiSDongleOfflineObservation(HuaweiSDongleOfflineObservation{
				UnitID: 100, Model: test.model, Firmware: test.firmware, ProtocolMajor: 5, ProtocolMinor: 0,
				SearchState: 0, ChangeSequenceBefore: 7, ChangeSequenceAfter: 7, Capacity: 1, ChildCount: 1,
			})
			if emma.IdentityMatched() != test.emma || logger.IdentityMatched() != test.smartLogger {
				t.Fatalf("family evidence crossed: EMMA=%#v SmartLogger=%#v S-Dongle=%#v", emma, logger, dongle)
			}
			if dongle.Matched() != test.dongle {
				t.Fatalf("S-Dongle observation = %#v, want matched=%t", dongle, test.dongle)
			}
			if test.emma && emma.SelectedClass() != HuaweiEMMACanonicalClass || test.smartLogger && logger.SelectedClass() != HuaweiSmartLoggerCanonicalClass ||
				test.dongle && dongle.CanonicalClass() != HuaweiSDongleCanonicalClass {
				t.Fatal("family identity mislabeled")
			}
			k20RequireDenied(t, emma)
			k20RequireDenied(t, logger)
			if !dongle.DefaultDenied() {
				t.Fatalf("S-Dongle observation granted admission: %#v", dongle)
			}
		})
	}
}

func k20Inventory(self, firmware string) HuaweiEMMAOfflineInventoryInput {
	return HuaweiEMMAOfflineInventoryInput{
		InventoryGuardBefore: 7, InventoryGuardAfter: 7,
		Pages: []HuaweiGatewayInventoryPage{{Objects: []HuaweiGatewayInventoryObject{
			{ObjectID: 0x87, Value: []byte{1}},
			{ObjectID: 0x88, Value: []byte("1=" + self + ";2=" + firmware + ";5=0")},
			{ObjectID: 0x89, Value: []byte("1=SUN2000;5=1")},
		}}},
	}
}

func k20RequireDenied(t *testing.T, result HuaweiQualificationResult) {
	t.Helper()
	if !result.DefaultDenied() || result.CatalogRegistered() || result.AutomaticRuntimeAdmission() || result.LiveQualified() ||
		result.HardwareTestReady() || result.SupportClaim() || result.WriteAuthority() || result.NativeFactCount() != 0 ||
		result.AutomaticRequestCount() != 0 || result.FallbackAttempted() {
		t.Fatalf("offline evidence gained authority: %#v", result)
	}
}
