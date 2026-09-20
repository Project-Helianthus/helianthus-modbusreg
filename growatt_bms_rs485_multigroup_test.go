package modbusreg

import "testing"

func TestDecodeGrowattBMSMultiGroupDiagnosticRetainsBoundedNativeEvidence(t *testing.T) {
	base, err := DecodeGrowattBMSTypedReadOnlyStatus(validGrowattBMSTypedReadOnlyInput())
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(base, validGrowattBMSMultiGroupDiagnosticInput())
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.BatteryID != 3 || diagnostic.ReportedGroupID != 9 || diagnostic.OutboundAllowed() {
		t.Fatalf("diagnostic identity = %#v", diagnostic)
	}
	if diagnostic.CellVoltageGroups[0].Offset != 0x0071 || diagnostic.CellVoltageGroups[0].Millivolts[0] != 3201 ||
		diagnostic.CellVoltageGroups[1].Offset != 0x0081 || diagnostic.CellVoltageGroups[1].Millivolts[15] != 3316 {
		t.Fatalf("groups = %#v", diagnostic.CellVoltageGroups)
	}
	if diagnostic.NativeBalanceStates[0] != GrowattBMSNativeCellStateOn ||
		diagnostic.NativeBalanceStates[1] != GrowattBMSNativeCellStateOff ||
		diagnostic.NativeBalanceStates[15] != GrowattBMSNativeCellStateOn {
		t.Fatalf("balance states = %#v", diagnostic.NativeBalanceStates)
	}
	raw := diagnostic.RawSlices()
	if len(raw) != 5 || raw[0].Offset != 0x001f || raw[0].SourceSliceOffset != 0x000d || raw[0].Words[0] != 0x0300 ||
		raw[1].Offset != 0x0070 || raw[2].Offset != 0x0071 || raw[3].Offset != 0x0081 ||
		raw[4].Offset != 0x0109 || raw[4].SourceSliceOffset != 0x0100 || raw[4].Words[0] != 0x8001 {
		t.Fatalf("raw slices = %#v", raw)
	}
	if raw[2].UnitID != 1 || raw[2].Function != FunctionReadHoldingRegisters || raw[2].Revision != base.Revision {
		t.Fatalf("raw provenance = %#v", raw[2])
	}
	raw[2].Words[0] = 0
	if diagnostic.RawSlices()[2].Words[0] != 3201 {
		t.Fatal("raw slices alias diagnostic")
	}
}

func TestDecodeGrowattBMSMultiGroupDiagnosticMapsEveryNativeBalanceBit(t *testing.T) {
	baselineInput := validGrowattBMSTypedReadOnlyInput()
	baselineInput.Slices[2].Words[9] = 0
	baseline, err := DecodeGrowattBMSTypedReadOnlyStatus(baselineInput)
	if err != nil {
		t.Fatal(err)
	}
	baselineDiagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(baseline, validGrowattBMSMultiGroupDiagnosticInput())
	if err != nil {
		t.Fatal(err)
	}
	for cell, state := range baselineDiagnostic.NativeBalanceStates {
		if state != GrowattBMSNativeCellStateOff {
			t.Fatalf("all-off baseline cell %d = %v", cell, state)
		}
	}

	for selectedCell := range baselineDiagnostic.NativeBalanceStates {
		t.Run("single native on bit", func(t *testing.T) {
			baseInput := validGrowattBMSTypedReadOnlyInput()
			baseInput.Slices[2].Words[9] = 1 << uint(selectedCell)
			base, err := DecodeGrowattBMSTypedReadOnlyStatus(baseInput)
			if err != nil {
				t.Fatal(err)
			}
			diagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(base, validGrowattBMSMultiGroupDiagnosticInput())
			if err != nil {
				t.Fatal(err)
			}
			for cell, state := range diagnostic.NativeBalanceStates {
				want := GrowattBMSNativeCellStateOff
				if cell == selectedCell {
					want = GrowattBMSNativeCellStateOn
				}
				if state != want {
					t.Fatalf("bit %d mapped cell %d as %v, want %v", selectedCell, cell, state, want)
				}
			}
		})
	}
}

func TestDecodeGrowattBMSMultiGroupDiagnosticRetainsGroupWordWithoutIdentityInference(t *testing.T) {
	base, err := DecodeGrowattBMSTypedReadOnlyStatus(validGrowattBMSTypedReadOnlyInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, groupWord := range []uint16{0, 1, 0xffff} {
		input := validGrowattBMSMultiGroupDiagnosticInput()
		input.Slices[0].Words[0] = groupWord
		diagnostic, err := DecodeGrowattBMSMultiGroupDiagnostic(base, input)
		if err != nil {
			t.Fatalf("reported group word %#04x rejected: %v", groupWord, err)
		}
		if diagnostic.ReportedGroupID != groupWord || len(diagnostic.CellVoltageGroups) != 2 ||
			diagnostic.CellVoltageGroups[0].Offset != 0x0071 || diagnostic.CellVoltageGroups[1].Offset != 0x0081 {
			t.Fatalf("reported group word %#04x inferred group identity: %#v", groupWord, diagnostic)
		}
	}
}

func TestDecodeGrowattBMSMultiGroupDiagnosticFailsClosedWithoutAffectingBaseStatus(t *testing.T) {
	base, err := DecodeGrowattBMSTypedReadOnlyStatus(validGrowattBMSTypedReadOnlyInput())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*GrowattBMSTypedReadOnlyStatus, *GrowattBMSMultiGroupDiagnosticInput){
		"base unavailable": func(base *GrowattBMSTypedReadOnlyStatus, _ *GrowattBMSMultiGroupDiagnosticInput) {
			*base = GrowattBMSTypedReadOnlyStatus{}
		},
		"revision mismatch": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Revision.HeaderVersion = "V2.1"
		},
		"unit mismatch": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) { input.UnitID = 2 },
		"other function": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Function = FunctionReadInputRegisters
		},
		"missing diagnostic slice": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Slices = input.Slices[:2]
		},
		"duplicate diagnostic slice": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Slices[2] = input.Slices[1]
		},
		"reordered diagnostic slice": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Slices[0], input.Slices[1] = input.Slices[1], input.Slices[0]
		},
		"extra diagnostic slice": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Slices = append(input.Slices, GrowattBMSReadOnlySlice{Offset: 0x010c, Words: []uint16{0}})
		},
		"malformed extent": func(_ *GrowattBMSTypedReadOnlyStatus, input *GrowattBMSMultiGroupDiagnosticInput) {
			input.Slices[1].Words = input.Slices[1].Words[:15]
		},
		"reserved battery ID bits": func(base *GrowattBMSTypedReadOnlyStatus, _ *GrowattBMSMultiGroupDiagnosticInput) {
			base.nativeObservation.slices[1].Words[18] = 0x4000
		},
	} {
		t.Run(name, func(t *testing.T) {
			status, err := DecodeGrowattBMSTypedReadOnlyStatus(validGrowattBMSTypedReadOnlyInput())
			if err != nil {
				t.Fatal(err)
			}
			input := validGrowattBMSMultiGroupDiagnosticInput()
			mutate(&status, &input)
			if got, err := DecodeGrowattBMSMultiGroupDiagnostic(status, input); err == nil || len(got.RawSlices()) != 0 {
				t.Fatalf("diagnostic/error = %#v/%v", got, err)
			}
		})
	}
	if _, err := DecodeGrowattBMSMultiGroupDiagnostic(base, GrowattBMSMultiGroupDiagnosticInput{}); err == nil || base.SOCPercent != 75 {
		t.Fatalf("failed diagnostic altered qualified base status: %#v/%v", base, err)
	}
}

func validGrowattBMSMultiGroupDiagnosticInput() GrowattBMSMultiGroupDiagnosticInput {
	return GrowattBMSMultiGroupDiagnosticInput{
		UnitID:   1,
		Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		Slices: []GrowattBMSReadOnlySlice{
			{Offset: 0x0070, Words: []uint16{9}},
			{Offset: 0x0071, Words: []uint16{3201, 3202, 3203, 3204, 3205, 3206, 3207, 3208, 3209, 3210, 3211, 3212, 3213, 3214, 3215, 3216}},
			{Offset: 0x0081, Words: []uint16{3301, 3302, 3303, 3304, 3305, 3306, 3307, 3308, 3309, 3310, 3311, 3312, 3313, 3314, 3315, 3316}},
		},
	}
}
