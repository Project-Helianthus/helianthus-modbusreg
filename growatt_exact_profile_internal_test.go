package modbusreg

import (
	"reflect"
	"testing"
)

func TestM52GrowattAdjacentRevisionsReachRevisionComparison(t *testing.T) {
	input := validGrowattProtocolIIIdentityInput()
	input.Slices[1].Words = []uint16{0x5445, 0x5354, 0, 0, 0}
	identity, err := DecodeGrowattProtocolIIIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	admission := syntheticGrowattProtocolIIFC04FixtureAdmission(input.Profile)
	raw := GrowattProtocolIIFC04Slice{Words: make([]uint16, 59)}
	raw.Words[0] = 1

	for _, test := range []struct {
		name     string
		delta    int
		accepted bool
	}{
		{name: "matching_revision", accepted: true},
		{name: "previous_revision", delta: -1},
		{name: "next_revision", delta: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Keep the caller profile fixed so its whole-struct equality cannot
			// mask the final observed-revision comparison. Only package tests
			// can construct this deliberately inconsistent observation.
			candidate := identity
			candidate.protocolVersion = uint16(int(identity.ProtocolVersion()) + test.delta)
			if !admission.syntheticFixture || admission.profile != candidate.Profile() ||
				admission.profile.DeviceType != candidate.DeviceType() ||
				admission.profile.ModelBuild != candidate.ModelBuild() {
				t.Fatal("revision comparison prerequisites are not satisfied")
			}
			restored := candidate
			restored.protocolVersion = identity.ProtocolVersion()
			if !reflect.DeepEqual(restored, identity) {
				t.Fatal("fixture changed beyond the observed protocol revision")
			}
			if got := admission.profile.ProtocolVersion == candidate.ProtocolVersion(); got != test.accepted {
				t.Fatal("fixture does not isolate the intended revision comparison")
			}
			if got := admission.matches(candidate); got != test.accepted {
				t.Fatalf("synthetic admission matches = %t, want %t", got, test.accepted)
			}

			telemetry, err := DecodeGrowattProtocolIIFC04Telemetry(candidate, admission, raw)
			if test.accepted {
				if err != nil || telemetry.InverterState != GrowattProtocolIIStateNormal ||
					!reflect.DeepEqual(telemetry.Identity(), candidate) || !reflect.DeepEqual(telemetry.RawSlice(), raw) {
					t.Fatalf("matching synthetic revision did not reach telemetry decoding: telemetry=%#v err=%v", telemetry, err)
				}
			} else if err == nil || !reflect.DeepEqual(telemetry, GrowattProtocolIIFC04Telemetry{}) {
				t.Fatalf("adjacent revision produced telemetry: telemetry=%#v err=%v", telemetry, err)
			}
			if candidate.OutboundAllowed() || telemetry.OutboundAllowed() {
				t.Fatal("synthetic fixture gained outbound authority")
			}
			if (GrowattProtocolIIFC04Applicability{}).matches(candidate) {
				t.Fatal("missing synthetic fixture admission matched")
			}
		})
	}
}
