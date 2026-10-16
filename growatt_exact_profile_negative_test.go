package modbusreg_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestK21GrowattExactProfileNegativeAdmissionMatrix(t *testing.T) {
	// This existing detector fixture models qualified eligibility only. It is
	// independent of Growatt and creates no protocol qualification evidence.
	control := detectionProfile(t, "fixture.standard.control", "1.0.0", reg.MaturityQualified, reg.ProfileActive, true)
	catalog, err := reg.NewCatalog(control)
	if err != nil {
		t.Fatal(err)
	}
	before := catalog.Profiles()
	tests := []struct {
		name       string
		mutate     func(*reg.GrowattProtocolIIIdentityInput)
		rawAllowed bool
	}{
		{"exact_but_unqualified", func(*reg.GrowattProtocolIIIdentityInput) {}, true},
		{"similar_family_text", func(in *reg.GrowattProtocolIIIdentityInput) { in.Profile.Family = "MAX-X" }, false},
		{"wrong_function", func(in *reg.GrowattProtocolIIIdentityInput) { in.Function = reg.FunctionReadInputRegisters }, false},
		{"absent_schema_revision", func(in *reg.GrowattProtocolIIIdentityInput) { in.Profile.Schema = "" }, false},
		{"previous_schema_revision", func(in *reg.GrowattProtocolIIIdentityInput) { in.Profile.Schema = "Protocol II v1.23 TL3-X" }, false},
		{"next_schema_revision", func(in *reg.GrowattProtocolIIIdentityInput) { in.Profile.Schema = "Protocol II v1.25 TL3-X" }, false},
		// Matching caller/raw values may be retained as observations, but are
		// never a source-backed mapping from a wire value to a manual revision.
		{"absent_wire_revision", func(in *reg.GrowattProtocolIIIdentityInput) {
			in.Profile.ProtocolVersion, in.Slices[4].Words[0] = 0, 0
		}, true},
		{"adjacent_wire_revision", func(in *reg.GrowattProtocolIIIdentityInput) {
			in.Profile.ProtocolVersion++
			in.Slices[4].Words[0] = in.Profile.ProtocolVersion
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := publicGrowattProtocolIIIdentityInput()
			test.mutate(&input)
			rawBefore := input.Slices
			identity, identityErr := reg.DecodeGrowattProtocolIIIdentity(input)
			if (identityErr == nil) != test.rawAllowed {
				t.Fatalf("raw identity retention: allowed=%t err=%v", test.rawAllowed, identityErr)
			}
			if identityErr == nil && (!reflect.DeepEqual(identity.Slices(), rawBefore) || identity.Profile() != input.Profile || identity.OutboundAllowed()) {
				t.Fatal("raw identity changed or gained outbound authority")
			}

			// The missing runtime profile cannot borrow the control's qualified
			// binding, even when manufacturer/model strings look plausible.
			for _, id := range []string{"growatt.direct-inverter", "growatt.protocol-ii.fc04"} {
				spec := detectionCandidate(t, control, 100, true, false).Spec()
				spec.ProfileID = id
				spec.Manufacturer.Expected = "Growatt"
				spec.Model.Expected = input.Profile.Family
				candidate, err := reg.NewDetectionCandidate(spec, detectionLimits())
				if err != nil {
					t.Fatal(err)
				}
				detector, err := reg.NewProfileDetector(reg.ProfileDetectorSpec{
					DetectorVersion: control.DetectorVersion(), Plan: detectionPlan(t),
					Catalog: catalog, Candidates: []reg.DetectionCandidate{candidate}, Limits: detectionLimits(),
				})
				if err == nil || detector != nil {
					t.Fatalf("unregistered profile acquired a detector: id=%s detector=%v err=%v", id, detector, err)
				}
				if profile, ok := catalog.Lookup(id); ok || len(profile.Spec().Codecs) != 0 {
					t.Fatalf("unregistered profile acquired codecs: id=%s profile=%v", id, profile)
				}
			}

			// Public callers cannot construct the missing exact FC04 admission.
			// In particular, retaining an FC03 observation is insufficient.
			var admission reg.GrowattProtocolIIFC04Applicability
			session := &k21InputSession{}
			observer, err := reg.NewGrowattProtocolIIFC04RTUObserver(identity, admission, session)
			if !errors.Is(err, reg.ErrGrowattProtocolIIFC04RTUObserverInvalid) || observer != nil {
				t.Fatalf("candidate created executable read: observer=%v err=%v", observer, err)
			}
			if session.reads != 0 {
				t.Fatalf("rejected candidate read count=%d", session.reads)
			}
			telemetry, err := reg.DecodeGrowattProtocolIIFC04Telemetry(identity, admission, reg.GrowattProtocolIIFC04Slice{Words: make([]uint16, 59)})
			if err == nil || telemetry.Identity().UnitID() != 0 || len(telemetry.RawSlice().Words) != 0 {
				t.Fatalf("candidate acquired typed FC04 decoder output: telemetry=%v err=%v", telemetry, err)
			}
			if !reflect.DeepEqual(catalog.Profiles(), before) {
				t.Fatal("negative admission changed catalog or qualification metadata")
			}
			// Recheck selection after every rejection to catch poisoning of an
			// independently eligible profile by the negative admission path.
			decision, err := newDetector(t, catalog, detectionCandidate(t, control, 100, true, false)).Detect(context.Background(), detectionReader(t), reg.DetectionOptions{})
			if err != nil || decision.Outcome() != reg.DetectionMatched || decision.SelectedProfileID() != control.ID() || decision.SelectedProfileVersion() != control.Version() {
				t.Fatalf("qualified control became unselectable: decision=%v err=%v", decision, err)
			}
		})
	}
}

type k21InputSession struct{ reads int }

func (s *k21InputSession) ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	s.reads++
	return modbus.ReadRegistersResponse{}, errors.New("unexpected synthetic read")
}
