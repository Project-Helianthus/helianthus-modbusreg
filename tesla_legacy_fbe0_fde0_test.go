package modbusreg

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestTeslaLegacyFDE0ObservationRejectsCallerSuppliedRawMismatch(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible)
	raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, teslaLegacyFDE0Payload(false, 0x09, 2000, 1000))
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

func TestTeslaLegacyDynamicCurrentFramesBindCompleteProtocol1AndProtocol2Records(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible)
	for _, test := range []struct {
		name      string
		command   TeslaLegacyCommand
		direction TeslaLegacyDirection
		payload   []byte
		actual    uint16
	}{
		{"FBE0 protocol 1", TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, TeslaLegacyWallConnectorRequest, teslaLegacyFBE0Payload(false, 0x09, 2000), 0},
		{"FBE0 protocol 2", TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, TeslaLegacyWallConnectorRequest, teslaLegacyFBE0Payload(true, 0x09, 2000), 0},
		{"FDE0 protocol 1", TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, TeslaLegacyWallConnectorResponse, teslaLegacyFDE0Payload(false, 0x09, 2000, 1000), 1000},
		{"FDE0 protocol 2", TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, TeslaLegacyWallConnectorResponse, teslaLegacyFDE0Payload(true, 0x09, 2000, 1000), 1000},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := teslaLegacyDynamicFrame(t, test.command, test.payload)
			wantRaw := append([]byte(nil), raw...)
			if test.direction == TeslaLegacyWallConnectorRequest {
				observation, err := NewTeslaLegacyFBE0Observation(TeslaLegacyFBE0ObservationSpec{
					Profile: profile, Direction: test.direction, Qualification: TeslaLegacyFamilyCompatible,
					State: TeslaLegacyFDE0StateCharging, AllocatedMaxCA: 2000, Raw: raw,
				})
				if err != nil {
					t.Fatal(err)
				}
				if observation.SourceID() != 1 || observation.DestinationID() != 2 || observation.AllocatedMaxCA() != 2000 {
					t.Fatalf("FBE0 fields=%#v", observation)
				}
				raw[1] = 0
				if !bytes.Equal(observation.Raw(), wantRaw) {
					t.Fatal("FBE0 did not defensively retain the complete opaque heartbeat record")
				}
				return
			}
			observation, err := NewTeslaLegacyFDE0Observation(TeslaLegacyFDE0ObservationSpec{
				Profile: profile, Direction: test.direction, Qualification: TeslaLegacyFamilyCompatible,
				State: TeslaLegacyFDE0StateCharging, AllocatedMaxCA: 2000, ActualCurrentCA: test.actual, Raw: raw,
			})
			if err != nil {
				t.Fatal(err)
			}
			if observation.SourceID() != 2 || observation.DestinationID() != 1 || observation.AllocatedMaxCA() != 2000 || observation.ActualCurrentCA() != test.actual {
				t.Fatalf("FDE0 fields=%#v", observation)
			}
			if observation.AllocatedMaxCA() == observation.ActualCurrentCA() {
				t.Fatal("allocated and actual current roles collapsed")
			}
			copy := observation.Raw()
			copy[0] = 0
			if !bytes.Equal(observation.Raw(), wantRaw) {
				t.Fatal("FDE0 did not defensively retain the complete opaque heartbeat record")
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
		raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, teslaLegacyFDE0Payload(false, byte(test.state), 2000, 1000))
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

func TestTeslaLegacyFBE0RejectsPostCommandTruncationAtomically(t *testing.T) {
	profile := teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible)
	for _, length := range []int{7, 9, 10, 12, 14} {
		t.Run(fmt.Sprintf("length-%d", length), func(t *testing.T) {
			raw := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, make([]byte, length))
			got, err := NewTeslaLegacyFBE0Observation(TeslaLegacyFBE0ObservationSpec{
				Profile: profile, Direction: TeslaLegacyWallConnectorRequest, Qualification: TeslaLegacyFamilyCompatible,
				State: 0, AllocatedMaxCA: 0, Raw: raw,
			})
			if err == nil {
				t.Fatalf("post-command length %d was accepted", length)
			}
			if !reflect.DeepEqual(got, TeslaLegacyFBE0Observation{}) {
				t.Fatalf("post-command length %d retained a partial observation: %#v", length, got)
			}
		})
	}
}

func TestTeslaLegacyDynamicCurrentRejectsInvalidContextAtomically(t *testing.T) {
	validFBE0 := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, teslaLegacyFBE0Payload(false, 0x05, 600))
	validFDE0 := teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, teslaLegacyFDE0Payload(false, 0x09, 2000, 1000))
	badChecksum := append([]byte(nil), validFDE0...)
	badChecksum[len(badChecksum)-2] ^= 1
	for _, test := range []struct {
		name string
		spec TeslaLegacyFDE0ObservationSpec
	}{
		{"wrong direction", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorRequest, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFDE0}},
		{"wrong command", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: validFBE0}},
		{"post-command length 7", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, make([]byte, 7))}},
		{"post-command length 9", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, make([]byte, 9))}},
		{"post-command length 10", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, make([]byte, 10))}},
		{"post-command length 12", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, make([]byte, 12))}},
		{"post-command length 14", TeslaLegacyFDE0ObservationSpec{Profile: teslaLegacyProfile(t, true, TeslaLegacyWallConnectorFamilyCompatible), Direction: TeslaLegacyWallConnectorResponse, Qualification: TeslaLegacyFamilyCompatible, State: 0x09, AllocatedMaxCA: 2000, ActualCurrentCA: 1000, Raw: teslaLegacyDynamicFrame(t, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, make([]byte, 14))}},
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

func teslaLegacyFBE0Payload(protocol2 bool, state byte, allocated uint16) []byte {
	payload := []byte{0, 1, 0, 2, state, byte(allocated >> 8), byte(allocated), 0xaa, 0xbb, 0xcc, 0xdd}
	if protocol2 {
		return append(payload, 0xee, 0xff)
	}
	return payload
}

func teslaLegacyFDE0Payload(protocol2 bool, state byte, allocated, actual uint16) []byte {
	payload := []byte{0, 2, 0, 1, state, byte(allocated >> 8), byte(allocated), byte(actual >> 8), byte(actual), 0xaa, 0xbb}
	if protocol2 {
		return append(payload, 0xcc, 0xdd)
	}
	return payload
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
