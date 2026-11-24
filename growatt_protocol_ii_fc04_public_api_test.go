package modbusreg_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/growatt"
)

// TestGrowattProtocolIIFC04HasNoPublicAdmissionPath keeps zero applicability
// closed. NewProtocolIIFC04Applicability is public. A zero applicability value
// still yields no telemetry and no observer.
func TestGrowattProtocolIIFC04HasNoPublicAdmissionPath(t *testing.T) {
	command := exec.Command("go", "doc", "./growatt", "NewProtocolIIFC04Applicability")
	command.Env = append(command.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "NewProtocolIIFC04Applicability") {
		t.Fatalf("public FC04 applicability constructor: output=%q err=%v", output, err)
	}

	words := make([]uint16, 250)
	words[0] = 1
	for name, mutate := range map[string]func(*growatt.ProtocolIIIdentityInput){
		"combined tuple": func(input *growatt.ProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[2].Words[0] = 0xbeef
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
			input.Slices[4].Words[0] = 0x9999
		},
		"device type": func(input *growatt.ProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Slices[2].Words[0] = 0xbeef
		},
		"model build": func(input *growatt.ProtocolIIIdentityInput) {
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
		},
		"protocol version": func(input *growatt.ProtocolIIIdentityInput) {
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[4].Words[0] = 0x9999
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := publicGrowattProtocolIIIdentityInput()
			mutate(&input)
			identity, err := growatt.DecodeGrowattProtocolIIIdentity(input)
			if err != nil {
				t.Fatalf("raw FC03 identity was not retained: %v", err)
			}

			var unadmitted growatt.ProtocolIIFC04Applicability
			if telemetry, err := growatt.DecodeGrowattProtocolIIFC04Telemetry(identity, unadmitted, growatt.ProtocolIIFC04Slice{Words: words}); err == nil || telemetry.Identity().UnitID() != 0 {
				t.Fatalf("unadmitted tuple produced typed telemetry: telemetry=%#v err=%v", telemetry, err)
			}
			if observer, err := growatt.NewProtocolIIFC04RTUObserver(identity, unadmitted, publicGrowattProtocolIIFC04Session{}); err == nil || observer != nil {
				t.Fatalf("unadmitted tuple created observer: observer=%#v err=%v", observer, err)
			}
		})
	}
}

func publicGrowattProtocolIIIdentityInput() growatt.ProtocolIIIdentityInput {
	return growatt.ProtocolIIIdentityInput{
		UnitID:   1,
		Function: reg.FunctionReadHoldingRegisters,
		Profile: growatt.ProtocolIIIdentityProfile{
			Schema:          "Protocol II v1.24 TL3-X",
			Family:          "MAX",
			DeviceType:      0x1234,
			ModelBuild:      [2]uint16{0x0102, 0x0304},
			ProtocolVersion: 0x0100,
		},
		Slices: []growatt.ProtocolIIIdentitySlice{
			{Offset: 9, Words: []uint16{0x4657, 0, 0, 0, 0, 0}},
			{Offset: 23, Words: []uint16{0x534e, 0, 0, 0, 0}},
			{Offset: 43, Words: []uint16{0x1234}},
			{Offset: 82, Words: []uint16{0x0102, 0x0304}},
			{Offset: 88, Words: []uint16{0x0100}},
		},
	}
}

type publicGrowattProtocolIIFC04Session struct{}

func (publicGrowattProtocolIIFC04Session) ReadInput(context.Context, byte, modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	return modbus.ReadRegistersResponse{}, nil
}
