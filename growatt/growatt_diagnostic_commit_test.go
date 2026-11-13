package growatt

import (
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func diagnosticCommitFixture(t *testing.T) (DiagnosticCommitBase, DiagnosticCommitContext, BMSMultiGroupDiagnosticInput) {
	t.Helper()
	input := validGrowattBMSTypedReadOnlyInput()
	input.Slices[2].Words[9] = 0
	base, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
	if err != nil {
		t.Fatal(err)
	}
	c := DiagnosticCommitContext{
		BaseRecordID: "synthetic-base-a", SourceInstance: "synthetic-source-a", Epoch: 1, Cycle: 2,
		UnitID: 1, Function: modbusreg.FunctionReadHoldingRegisters, Revision: input.Revision,
		NativeIdentity: DiagnosticCommitNativeIdentity{
			MCUVersion: input.Slices[0].Words[0], BMS: input.Slices[1].Words[0],
			Pack: input.Slices[1].Words[1], Topology: input.Slices[1].Words[18], CellSeries: input.Slices[1].Words[28],
		},
	}
	ref, reason := NewDiagnosticCommitBase(base, c)
	if reason != "" {
		t.Fatal(reason)
	}
	return ref, c, validGrowattBMSMultiGroupDiagnosticInput()
}

func TestGrowattDiagnosticCommitCompleteDetachedBundle(t *testing.T) {
	ref, c, input := diagnosticCommitFixture(t)
	input.Slices[1].Words[5] = 0
	got, reason := CheckGrowattDiagnosticCommit(ref, c, 9, input)
	if reason != "" {
		t.Fatal(reason)
	}
	d := got.Diagnostic()
	if got.Context() != c || got.Identity() != (DiagnosticCommitIdentity{BatteryID: 3, ReportedGroupWord: 9}) {
		t.Fatal("candidate lost frozen identity/context")
	}
	for group := 0; group < 2; group++ {
		wantOffset := uint16(0x0071 + group*16)
		if d.CellVoltageGroups[group].Offset != wantOffset {
			t.Fatal("cell offset changed")
		}
		for cell, actual := range d.CellVoltageGroups[group].Millivolts {
			want := uint16(3201 + group*100 + cell)
			if group == 0 && cell == 5 {
				want = 0
			}
			if actual != want {
				t.Fatalf("group %d cell %d = %d want %d", group, cell, actual, want)
			}
		}
	}
	for _, state := range d.NativeBalanceStates {
		if state != BMSNativeCellStateOff {
			t.Fatal("zero balance lost")
		}
	}
	raw := d.RawSlices()
	for i, offset := range []uint16{0x001f, 0x0070, 0x0071, 0x0081, 0x0109} {
		if len(raw) != 5 || raw[i].Offset != offset || raw[i].UnitID != 1 || raw[i].Function != modbusreg.FunctionReadHoldingRegisters || raw[i].Revision != c.Revision {
			t.Fatal("raw order/provenance changed")
		}
	}
	if raw[0].SourceSliceOffset != 0x000d || raw[0].Words[0] != 0x0300 || raw[1].Words[0] != 9 || raw[4].SourceSliceOffset != 0x0100 || raw[4].Words[0] != 0 {
		t.Fatal("native identity/balance lost")
	}
	for group := 0; group < 2; group++ {
		if raw[group+2].SourceSliceOffset != uint16(0x0071+group*16) || len(raw[group+2].Words) != 16 {
			t.Fatal("raw cell extent changed")
		}
		for cell, word := range raw[group+2].Words {
			if word != d.CellVoltageGroups[group].Millivolts[cell] {
				t.Fatal("raw cell position changed")
			}
		}
	}
	before := got.Diagnostic()
	before.rawSlices = before.RawSlices()
	for i := range input.Slices {
		for j := range input.Slices[i].Words {
			input.Slices[i].Words[j] = 65535
		}
	}
	for i := range d.rawSlices {
		d.rawSlices[i].Words[0] = 65535
	}
	for i := range raw {
		raw[i].Words[0] = 65535
	}
	d.CellVoltageGroups[0].Millivolts[0] = 65535
	c.BaseRecordID = "changed"
	if !reflect.DeepEqual(before, got.Diagnostic()) || got.Context().BaseRecordID != "synthetic-base-a" {
		t.Fatal("candidate aliases caller storage")
	}
}

func TestGrowattDiagnosticCommitContextAxes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DiagnosticCommitContext)
		reason DiagnosticCommitRejection
	}{
		{"base_record", func(c *DiagnosticCommitContext) { c.BaseRecordID += "-other" }, DiagnosticCommitContextMismatch},
		{"source", func(c *DiagnosticCommitContext) { c.SourceInstance += "-other" }, DiagnosticCommitContextMismatch},
		{"epoch", func(c *DiagnosticCommitContext) { c.Epoch++ }, DiagnosticCommitContextMismatch},
		{"cycle", func(c *DiagnosticCommitContext) { c.Cycle++ }, DiagnosticCommitContextMismatch},
		{"unit", func(c *DiagnosticCommitContext) { c.UnitID++ }, DiagnosticCommitContextMismatch},
		{"function", func(c *DiagnosticCommitContext) { c.Function = modbusreg.FunctionReadInputRegisters }, DiagnosticCommitContextMismatch},
		{"family", func(c *DiagnosticCommitContext) { c.Revision.Family += "-other" }, DiagnosticCommitContextMismatch},
		{"file_revision", func(c *DiagnosticCommitContext) { c.Revision.FileRevision += "-other" }, DiagnosticCommitContextMismatch},
		{"header_version", func(c *DiagnosticCommitContext) { c.Revision.HeaderVersion += "-other" }, DiagnosticCommitContextMismatch},
		{"cumulative_revision", func(c *DiagnosticCommitContext) { c.Revision.CumulativeRevision += "-other" }, DiagnosticCommitContextMismatch},
		{"mcu", func(c *DiagnosticCommitContext) { c.NativeIdentity.MCUVersion++ }, DiagnosticCommitContextMismatch},
		{"bms", func(c *DiagnosticCommitContext) { c.NativeIdentity.BMS++ }, DiagnosticCommitContextMismatch},
		{"pack", func(c *DiagnosticCommitContext) { c.NativeIdentity.Pack++ }, DiagnosticCommitContextMismatch},
		{"cell_series", func(c *DiagnosticCommitContext) { c.NativeIdentity.CellSeries++ }, DiagnosticCommitContextMismatch},
		{"topology", func(c *DiagnosticCommitContext) { c.NativeIdentity.Topology ^= 1 }, DiagnosticCommitTopologyChanged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, c, input := diagnosticCommitFixture(t)
			tc.mutate(&c)
			got, reason := CheckGrowattDiagnosticCommit(ref, c, 9, input)
			if reason != tc.reason || !reflect.DeepEqual(got, DiagnosticCommitCandidate{}) {
				t.Fatalf("got candidate %#v reason %q", got, reason)
			}
		})
	}
}

func TestGrowattDiagnosticCommitRejectsWholeBundle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*BMSMultiGroupDiagnosticInput)
		reason DiagnosticCommitRejection
	}{
		{"missing_group", func(i *BMSMultiGroupDiagnosticInput) { i.Slices = i.Slices[:2] }, DiagnosticCommitInvalidDiagnostic},
		{"first_malformed_good_sibling", func(i *BMSMultiGroupDiagnosticInput) { i.Slices[1].Words = i.Slices[1].Words[:15] }, DiagnosticCommitInvalidDiagnostic},
		{"second_malformed_good_sibling", func(i *BMSMultiGroupDiagnosticInput) { i.Slices[2].Words = i.Slices[2].Words[:15] }, DiagnosticCommitInvalidDiagnostic},
		{"empty_group", func(i *BMSMultiGroupDiagnosticInput) { i.Slices[2].Words = nil }, DiagnosticCommitInvalidDiagnostic},
		{"swapped_order", func(i *BMSMultiGroupDiagnosticInput) { i.Slices[1], i.Slices[2] = i.Slices[2], i.Slices[1] }, DiagnosticCommitInvalidDiagnostic},
		{"swapped_offsets", func(i *BMSMultiGroupDiagnosticInput) {
			i.Slices[1].Offset, i.Slices[2].Offset = i.Slices[2].Offset, i.Slices[1].Offset
		}, DiagnosticCommitInvalidDiagnostic},
		{"unit", func(i *BMSMultiGroupDiagnosticInput) { i.UnitID++ }, DiagnosticCommitInvalidDiagnostic},
		{"revision", func(i *BMSMultiGroupDiagnosticInput) { i.Revision.FileRevision += "-other" }, DiagnosticCommitInvalidDiagnostic},
		{"reported_group", func(i *BMSMultiGroupDiagnosticInput) { i.Slices[0].Words[0]++ }, DiagnosticCommitLineageChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, c, input := diagnosticCommitFixture(t)
			tc.mutate(&input)
			got, reason := CheckGrowattDiagnosticCommit(ref, c, 9, input)
			if reason != tc.reason || !reflect.DeepEqual(got, DiagnosticCommitCandidate{}) {
				t.Fatalf("partial bundle %#v / %q", got, reason)
			}
			if _, reason := CheckGrowattDiagnosticCommit(ref, c, 9, validGrowattBMSMultiGroupDiagnosticInput()); reason != "" {
				t.Fatal("rejection mutated base")
			}
		})
	}
}

func TestGrowattDiagnosticCommitZeroAndFrozenBase(t *testing.T) {
	_, c, input := diagnosticCommitFixture(t)
	if got, reason := CheckGrowattDiagnosticCommit(DiagnosticCommitBase{}, c, 9, input); reason != DiagnosticCommitInvalidBase || !reflect.DeepEqual(got, DiagnosticCommitCandidate{}) {
		t.Fatal("zero reference admitted")
	}
	if got, reason := NewDiagnosticCommitBase(BMSTypedReadOnlyStatus{}, c); reason != DiagnosticCommitInvalidBase || !reflect.DeepEqual(got, DiagnosticCommitBase{}) {
		t.Fatal("zero base admitted")
	}
	baseInput := validGrowattBMSTypedReadOnlyInput()
	baseInput.Slices[2].Words[9] = 0
	base, err := DecodeGrowattBMSTypedReadOnlyStatus(baseInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*DiagnosticCommitContext){
		func(c *DiagnosticCommitContext) { c.BaseRecordID = "" }, func(c *DiagnosticCommitContext) { c.SourceInstance = "" },
		func(c *DiagnosticCommitContext) { c.Epoch = 0 }, func(c *DiagnosticCommitContext) { c.Cycle = 0 },
		func(c *DiagnosticCommitContext) { c.UnitID++ }, func(c *DiagnosticCommitContext) { c.NativeIdentity.Topology++ },
	} {
		bad := c
		mutate(&bad)
		if got, reason := NewDiagnosticCommitBase(base, bad); reason == "" || !reflect.DeepEqual(got, DiagnosticCommitBase{}) {
			t.Fatal("invalid base context admitted")
		}
	}
	ref, reason := NewDiagnosticCommitBase(base, c)
	if reason != "" {
		t.Fatal(reason)
	}
	base.nativeObservation.slices[1].Words[18] = 0x0400
	base.nativeObservation.slices[2].Words[9] = 65535
	got, reason := CheckGrowattDiagnosticCommit(ref, c, 9, input)
	if reason != "" || got.Identity().BatteryID != 3 || got.Diagnostic().RawSlices()[4].Words[0] != 0 {
		t.Fatal("base reference aliases decoded base")
	}
	input.Slices[0].Words[0] = 0
	if got, reason := CheckGrowattDiagnosticCommit(ref, c, 0, input); reason != "" || got.Identity().ReportedGroupWord != 0 {
		t.Fatal("valid zero group rejected")
	}
}
