package modbusreg_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// TestGrowattProtocolIIFC04HasNoPublicAdmissionPath keeps the source-backed
// qualification boundary at the public-package boundary. The pinned manual has
// no admissible device/model-build/protocol tuple, so raw FC03 observations may
// be retained but no public value, including a caller string, may admit FC04.
func TestGrowattProtocolIIFC04HasNoPublicAdmissionPath(t *testing.T) {
	command := exec.Command("go", "doc", ".", "NewGrowattProtocolIIFC04Applicability")
	command.Env = append(command.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "no symbol") {
		t.Fatalf("public FC04 applicability constructor remains reachable: output=%q err=%v", output, err)
	}

	words := make([]uint16, 59)
	words[0] = 1
	for name, mutate := range map[string]func(*reg.GrowattProtocolIIIdentityInput){
		"combined tuple": func(input *reg.GrowattProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[2].Words[0] = 0xbeef
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
			input.Slices[4].Words[0] = 0x9999
		},
		"device type": func(input *reg.GrowattProtocolIIIdentityInput) {
			input.Profile.DeviceType = 0xbeef
			input.Slices[2].Words[0] = 0xbeef
		},
		"model build": func(input *reg.GrowattProtocolIIIdentityInput) {
			input.Profile.ModelBuild = [2]uint16{0x1111, 0x2222}
			input.Slices[3].Words = []uint16{0x1111, 0x2222}
		},
		"protocol version": func(input *reg.GrowattProtocolIIIdentityInput) {
			input.Profile.ProtocolVersion = 0x9999
			input.Slices[4].Words[0] = 0x9999
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := publicGrowattProtocolIIIdentityInput()
			mutate(&input)
			identity, err := reg.DecodeGrowattProtocolIIIdentity(input)
			if err != nil {
				t.Fatalf("raw FC03 identity was not retained: %v", err)
			}

			var unadmitted reg.GrowattProtocolIIFC04Applicability
			if telemetry, err := reg.DecodeGrowattProtocolIIFC04Telemetry(identity, unadmitted, reg.GrowattProtocolIIFC04Slice{Words: words}); err == nil || telemetry.Identity().UnitID() != 0 {
				t.Fatalf("unadmitted tuple produced typed telemetry: telemetry=%#v err=%v", telemetry, err)
			}
			if observer, err := reg.NewGrowattProtocolIIFC04RTUObserver(identity, unadmitted, publicGrowattProtocolIIFC04Session{}); err == nil || observer != nil {
				t.Fatalf("unadmitted tuple created observer: observer=%#v err=%v", observer, err)
			}
		})
	}
}

func publicGrowattProtocolIIIdentityInput() reg.GrowattProtocolIIIdentityInput {
	return reg.GrowattProtocolIIIdentityInput{
		UnitID:   1,
		Function: reg.FunctionReadHoldingRegisters,
		Profile: reg.GrowattProtocolIIIdentityProfile{
			Schema:          "Protocol II v1.24 TL3-X",
			Family:          "MAX",
			DeviceType:      0x1234,
			ModelBuild:      [2]uint16{0x0102, 0x0304},
			ProtocolVersion: 0x0100,
		},
		Slices: []reg.GrowattProtocolIIIdentitySlice{
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
