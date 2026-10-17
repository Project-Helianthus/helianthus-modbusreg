package modbusreg

import (
	"reflect"
	"testing"
)

func TestS06GrowattFixtureFlagRequiredForOtherwiseMatchingAdmission(t *testing.T) {
	input := validGrowattProtocolIIIdentityInput()
	input.Slices[1].Words = []uint16{0x5445, 0x5354, 0, 0, 0}
	identity, err := DecodeGrowattProtocolIIIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	flagged := syntheticGrowattProtocolIIFC04FixtureAdmission(input.Profile)
	unflagged := flagged
	unflagged.syntheticFixture = false
	if !flagged.syntheticFixture || unflagged.syntheticFixture || unflagged.profile != flagged.profile {
		t.Fatal("admission copy must differ only in the fixture flag")
	}
	if unflagged.profile != identity.Profile() ||
		unflagged.profile.DeviceType != identity.DeviceType() ||
		unflagged.profile.ModelBuild != identity.ModelBuild() ||
		unflagged.profile.ProtocolVersion != identity.ProtocolVersion() {
		t.Fatal("unflagged admission must satisfy every identity comparison")
	}
	raw := GrowattProtocolIIFC04Slice{Words: make([]uint16, 59)}
	raw.Words[0] = 1
	raw.Words[2] = 1234
	if identity.OutboundAllowed() {
		t.Fatal("synthetic identity gained outbound authority")
	}

	for _, test := range []struct {
		name      string
		admission GrowattProtocolIIFC04Applicability
		accepted  bool
	}{
		{name: "flagged", admission: flagged, accepted: true},
		{name: "unflagged", admission: unflagged},
		{name: "zero_admission"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.admission.matches(identity); got != test.accepted {
				t.Errorf("admission matches = %t, want %t", got, test.accepted)
			}
			telemetry, err := DecodeGrowattProtocolIIFC04Telemetry(identity, test.admission, raw)
			if test.accepted {
				if err != nil {
					t.Fatalf("flagged synthetic telemetry rejected: %v", err)
				}
				if telemetry.InverterState != GrowattProtocolIIStateNormal || telemetry.PVInputPowerWatts != 123.4 {
					t.Error("flagged telemetry did not decode the expected nonzero synthetic field")
				}
				if !reflect.DeepEqual(telemetry.Identity(), identity) || !reflect.DeepEqual(telemetry.RawSlice(), raw) {
					t.Error("flagged telemetry did not retain native identity and raw evidence")
				}
			} else {
				if err == nil {
					t.Error("telemetry accepted without flagged fixture admission")
				}
				if !reflect.DeepEqual(telemetry, GrowattProtocolIIFC04Telemetry{}) {
					t.Error("rejected fixture returned nonempty telemetry")
				}
			}
			if telemetry.OutboundAllowed() || telemetry.Identity().OutboundAllowed() {
				t.Error("synthetic telemetry gained outbound authority")
			}
		})
	}
}
