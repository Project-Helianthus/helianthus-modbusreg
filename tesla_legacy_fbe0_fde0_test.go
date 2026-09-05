package modbusreg

import (
	"bytes"
	"reflect"
	"testing"
)

func TestTeslaLegacyFDE0ObservationRejectsCallerSuppliedRawMismatch(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible)
	raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, []byte{0x00, 0x01, 0x00, 0x02, 0x09, 0x07, 0xd0, 0x03, 0xe8})
	got, err := NewTeslaLegacyFDE0Observation(TeslaLegacyFDE0ObservationSpec{
		Profile: profile, Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible,
		State: TeslaLegacyFDE0StateCharging, AllocatedMaxCA: 1, ActualCurrentCA: 2, Raw: raw,
	})
	if err == nil {
		t.Fatal("caller-supplied values that disagree with the complete raw FDE0 frame were accepted")
	}
	if !reflect.DeepEqual(got, TeslaLegacyFDE0Observation{}) {
		t.Fatalf("rejection retained a partial observation: %#v", got)
	}
}

func TestTeslaLegacyDynamicCurrentFramesBindFBE0AndFDE0NativeFields(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible)
	fbe0Raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, []byte{0x00, 0x01, 0x00, 0x02, 0x09, 0x07, 0xd0, 0xaa, 0xbb})
	wantFBE0Raw := append([]byte(nil), fbe0Raw...)
	fbe0, err := NewTeslaLegacyFBE0Observation(TeslaLegacyFBE0ObservationSpec{
		Profile: profile, Direction: TeslaLegacyWallConnectorRequest, Qualification: TeslaLegacyFamilyCompatible,
		State: TeslaLegacyFDE0StateCharging, AllocatedMaxCA: 2000, Raw: fbe0Raw,
	})
	if err != nil {
		t.Fatalf("NewTeslaLegacyFBE0Observation() error = %v", err)
	}
	if fbe0.SourceID() != 1 || fbe0.DestinationID() != 2 || fbe0.State() != TeslaLegacyFDE0StateCharging || fbe0.AllocatedMaxCA() != 2000 {
		t.Fatalf("FBE0 fields=%#v", fbe0)
	}
	fbe0Raw[1] = 0
	if !bytes.Equal(fbe0.Raw(), wantFBE0Raw) {
		t.Fatal("FBE0 did not retain opaque padding bytes")
	}

	fde0Raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, []byte{0x00, 0x02, 0x00, 0x01, 0x09, 0x07, 0xd0, 0x03, 0xe8})
	fde0, err := NewTeslaLegacyFDE0Observation(TeslaLegacyFDE0ObservationSpec{
		Profile: profile, Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible,
		State: TeslaLegacyFDE0StateCharging, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: fde0Raw,
	})
	if err != nil {
		t.Fatalf("NewTeslaLegacyFDE0Observation() error = %v", err)
	}
	if fde0.SourceID() != 2 || fde0.DestinationID() != 1 || fde0.AllocatedMaxCA() != 2000 || fde0.ActualCurrentCA() != 1000 {
		t.Fatalf("FDE0 fields=%#v", fde0)
	}
	if fde0.AllocatedMaxCA() == fde0.ActualCurrentCA() {
		t.Fatal("allocated and actual current roles collapsed")
	}
	copy := fde0.Raw()
	copy[0] = 0
	if !bytes.Equal(fde0.Raw(), fde0Raw) {
		t.Fatal("Raw() did not defensively copy the validated frame")
	}
}

func TestTeslaLegacyFBE0RejectsInvalidContextAtomically(t *testing.T) {
	valid := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, []byte{0, 1, 0, 2, 0x05, 0x02, 0x58})
	for _, test := range []struct {
		name string
		spec TeslaLegacyFBE0ObservationSpec
	}{
		{"wrong direction", TeslaLegacyFBE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x05, AllocatedMaxCA: 600, Raw: valid}},
		{"unsupported length", TeslaLegacyFBE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorRequest, Qualification: TeslaLegacyFamilyCompatible, State: 0x05, AllocatedMaxCA: 600, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, []byte{0, 1, 0, 2, 5, 2, 88, 0, 0, 0})}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := NewTeslaLegacyFBE0Observation(test.spec)
			if err == nil {
				t.Fatal("invalid context was accepted")
			}
			if !reflect.DeepEqual(got, TeslaLegacyFBE0Observation{}) {
				t.Fatalf("rejection retained a partial observation: %#v", got)
			}
		})
	}
}

func TestTeslaLegacyDynamicCurrentAbsoluteOfferBoundary(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorBuildConfirmed)
	for _, test := range []struct {
		state TeslaLegacyFDE0State
		want  bool
	}{
		{TeslaLegacyFDE0StatePreCharge, true},
		{TeslaLegacyFDE0StateCharging, true},
		{0x06, false},
		{0x07, false},
		{0x08, false},
	} {
		raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, []byte{0, 2, 0, 1, byte(test.state), 0x07, 0xd0, 0x03, 0xe8})
		observation, err := NewTeslaLegacyFDE0Observation(TeslaLegacyFDE0ObservationSpec{
			Profile: profile, Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyBuildConfirmed,
			State: test.state, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: raw,
		})
		if err != nil {
			t.Fatalf("state=%#x: %v", test.state, err)
		}
		got, ok := observation.AbsoluteOfferCA()
		if ok != test.want || (ok && got != 2000) {
			t.Fatalf("state=%#x absolute offer=(%d,%t)", test.state, got, ok)
		}
	}
}

func TestTeslaLegacyDynamicCurrentRejectsEveryInvalidContextAtomically(t *testing.T) {
	validFBE0 := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, []byte{0, 1, 0, 2, 0x05, 0x02, 0x58})
	validFDE0 := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, []byte{0, 2, 0, 1, 0x09, 0x07, 0xd0, 0x03, 0xe8})
	badChecksum := append([]byte(nil), validFDE0...)
	badChecksum[len(badChecksum)-2] ^= 1
	for _, test := range []struct {
		name string
		spec TeslaLegacyFDE0ObservationSpec
	}{
		{"wrong direction", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorRequest, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0}},
		{"wrong command", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFBE0}},
		{"unsupported length", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, []byte{0, 2, 0, 1, 9, 7, 208, 3})}},
		{"invalid checksum", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: badChecksum}},
		{"truncated frame", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0[:len(validFDE0)-1]}},
		{"disabled profile", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, false, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0}},
		{"unknown profile", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorUnknown), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0}},
		{"qualification mismatch", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorBuildConfirmed), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := NewTeslaLegacyFDE0Observation(test.spec)
			if err == nil {
				t.Fatal("invalid context was accepted")
			}
			if !reflect.DeepEqual(got, TeslaLegacyFDE0Observation{}) {
				t.Fatalf("rejection retained a partial observation: %#v", got)
			}
		})
	}
}

func teslaLegacyProfile(t *testing.T, enabled bool, compatibility TeslaLegacyWallConnectorCompatibility) TeslaLegacyWallConnectorProfile {
	t.Helper()
	profile, err := NewTeslaLegacyWallConnectorProfile(TeslaLegacyWallConnectorProfileConfig{Enabled: enabled, Compatibility: compatibility})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func teslaLegacyDynamicFrame(t *testing.T, command TeslaLegacyCommand, payload []byte) []byte {
	t.Helper()
	raw, err := EncodeTeslaLegacyWallConnectorFrame(command, payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
