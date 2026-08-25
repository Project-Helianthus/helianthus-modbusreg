package modbusreg

import "testing"

func TestGrowattBMSReadOnlyObservationRetainsOnlyExactDeclaredSlices(t *testing.T) {
	input := validGrowattBMSReadOnlyInput()
	observation, err := DecodeGrowattBMSReadOnlyObservation(input)
	if err != nil {
		t.Fatalf("DecodeGrowattBMSReadOnlyObservation() error = %v", err)
	}
	if observation.UnitID() != 1 || observation.Revision() != (GrowattBMSRevisionTuple{
		Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02",
	}) || observation.OutboundAllowed() {
		t.Fatalf("observation did not preserve a read-only tuple: %#v", observation)
	}
	slices := observation.Slices()
	if len(slices) != 4 || slices[0].Offset != 0x0001 || len(slices[0].Words) != 7 ||
		slices[1].Offset != 0x000d || len(slices[1].Words) != 29 ||
		slices[2].Offset != 0x0100 || len(slices[2].Words) != 12 ||
		slices[3].Offset != 0x010d || len(slices[3].Words) != 2 {
		t.Fatalf("slices=%#v", slices)
	}
	slices[0].Words[0] = 99
	if got := observation.Slices()[0].Words[0]; got != 1 {
		t.Fatalf("slice words alias observation: %d", got)
	}
}

func TestGrowattBMSReadOnlyObservationRejectsAnythingOutsideTheFourSlices(t *testing.T) {
	for name, mutate := range map[string]func(*GrowattBMSReadOnlyInput){
		"broadcast": func(input *GrowattBMSReadOnlyInput) { input.UnitID = 0 },
		"other function": func(input *GrowattBMSReadOnlyInput) {
			input.Function = FunctionReadInputRegisters
		},
		"other revision": func(input *GrowattBMSReadOnlyInput) { input.Revision.HeaderVersion = "V2.1" },
		"missing slice":  func(input *GrowattBMSReadOnlyInput) { input.Slices = input.Slices[:3] },
		"coalesced range": func(input *GrowattBMSReadOnlyInput) {
			input.Slices = []GrowattBMSReadOnlySlice{{Offset: 0x0001, Words: make([]uint16, 41)}}
		},
		"barcode range": func(input *GrowattBMSReadOnlyInput) {
			input.Slices[0] = GrowattBMSReadOnlySlice{Offset: 0x0009, Words: make([]uint16, 7)}
		},
		"extra range": func(input *GrowattBMSReadOnlyInput) {
			input.Slices = append(input.Slices, GrowattBMSReadOnlySlice{Offset: 0x0200, Words: []uint16{1}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validGrowattBMSReadOnlyInput()
			mutate(&input)
			if _, err := DecodeGrowattBMSReadOnlyObservation(input); err == nil {
				t.Fatal("accepted an out-of-bound Growatt BMS observation")
			}
		})
	}
}

func validGrowattBMSReadOnlyInput() GrowattBMSReadOnlyInput {
	return GrowattBMSReadOnlyInput{
		UnitID:   1,
		Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{
			Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02",
		},
		Slices: []GrowattBMSReadOnlySlice{
			{Offset: 0x0001, Words: []uint16{1, 0, 0, 0, 0, 0, 0}},
			{Offset: 0x000d, Words: make([]uint16, 29)},
			{Offset: 0x0100, Words: make([]uint16, 12)},
			{Offset: 0x010d, Words: []uint16{0, 0}},
		},
	}
}
