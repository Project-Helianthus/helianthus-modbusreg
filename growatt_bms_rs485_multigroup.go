package modbusreg

import "fmt"

// GrowattBMSMultiGroupDiagnosticInput is one caller-supplied, read-only
// diagnostic observation. It has no endpoint, session, or request authority.
// Its three slices are admitted only after a complete base status qualifies.
type GrowattBMSMultiGroupDiagnosticInput struct {
	UnitID   byte
	Function FunctionCode
	Revision GrowattBMSRevisionTuple
	Slices   []GrowattBMSReadOnlySlice
}

// GrowattBMSDiagnosticRawSlice retains one exact diagnostic word range with
// its selected Modbus tuple and the base or diagnostic slice from which it
// came. Words are immutable to callers of RawSlices.
type GrowattBMSDiagnosticRawSlice struct {
	UnitID            byte
	Revision          GrowattBMSRevisionTuple
	Function          FunctionCode
	Offset            uint16
	SourceSliceOffset uint16
	Words             []uint16
}

// GrowattBMSCellVoltageGroup is one of the two documented fixed cell blocks.
// Its ordinal is not a pack number, selector, or discovery result.
type GrowattBMSCellVoltageGroup struct {
	Offset     uint16
	Millivolts [16]uint16
}

// GrowattBMSNativeCellState is the documented native off/on bit value of one
// cell in the revision-scoped 0x0109 word. It has no control semantics.
type GrowattBMSNativeCellState bool

const (
	GrowattBMSNativeCellStateOff GrowattBMSNativeCellState = false
	GrowattBMSNativeCellStateOn  GrowattBMSNativeCellState = true
)

// GrowattBMSMultiGroupDiagnostic is a bounded, independent diagnostic view.
// It neither changes nor extends the qualified base telemetry result.
type GrowattBMSMultiGroupDiagnostic struct {
	Revision            GrowattBMSRevisionTuple
	BatteryID           uint8
	ReportedGroupID     uint16
	CellVoltageGroups   [2]GrowattBMSCellVoltageGroup
	NativeBalanceStates [16]GrowattBMSNativeCellState
	rawSlices           []GrowattBMSDiagnosticRawSlice
}

// RawSlices returns independent retained raw evidence for 0x001F, 0x0070,
// 0x0071--0x0080, 0x0081--0x0090, and 0x0109, in that fixed order.
func (diagnostic GrowattBMSMultiGroupDiagnostic) RawSlices() []GrowattBMSDiagnosticRawSlice {
	result := make([]GrowattBMSDiagnosticRawSlice, len(diagnostic.rawSlices))
	for index, slice := range diagnostic.rawSlices {
		result[index] = GrowattBMSDiagnosticRawSlice{
			UnitID: slice.UnitID, Revision: slice.Revision, Function: slice.Function,
			Offset: slice.Offset, SourceSliceOffset: slice.SourceSliceOffset,
			Words: append([]uint16(nil), slice.Words...),
		}
	}
	return result
}

// OutboundAllowed is permanently false: a diagnostic observation cannot
// authorize any follow-up request, selector, handshake, or control action.
func (GrowattBMSMultiGroupDiagnostic) OutboundAllowed() bool { return false }

// DecodeGrowattBMSMultiGroupDiagnostic decodes the three fixed FC03 slices
// only after a valid base status from the same selected unicast unit and exact
// revision. It returns no partial diagnostic on insufficient evidence.
func DecodeGrowattBMSMultiGroupDiagnostic(
	base GrowattBMSTypedReadOnlyStatus,
	input GrowattBMSMultiGroupDiagnosticInput,
) (GrowattBMSMultiGroupDiagnostic, error) {
	baseObservation := base.NativeObservation()
	if base.nativeObservation == nil || input.UnitID == 0 || input.UnitID > 247 ||
		input.Function != FunctionReadHoldingRegisters || input.Revision != growattBMSExactRevision() ||
		input.UnitID != baseObservation.UnitID() || input.Revision != baseObservation.Revision() {
		return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic identity is invalid")
	}
	if _, err := DecodeGrowattBMSTypedReadOnlyStatus(GrowattBMSReadOnlyInput{
		UnitID: baseObservation.UnitID(), Function: FunctionReadHoldingRegisters,
		Revision: baseObservation.Revision(), Slices: baseObservation.Slices(),
	}); err != nil {
		return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group base status is unavailable: %w", err)
	}

	want := [...]struct{ offset, words uint16 }{{0x0070, 1}, {0x0071, 16}, {0x0081, 16}}
	if len(input.Slices) != len(want) {
		return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic slice count is invalid")
	}
	for index, expected := range want {
		if input.Slices[index].Offset != expected.offset || len(input.Slices[index].Words) != int(expected.words) {
			return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic slice %d is invalid", index)
		}
	}

	baseSlices := baseObservation.Slices()
	statusWords, extensionWords := baseSlices[1].Words, baseSlices[2].Words
	if len(statusWords) != 29 || len(extensionWords) != 12 {
		return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group base extents are invalid")
	}
	batteryIdentity := statusWords[18]
	batteryID := uint8((batteryIdentity >> 8) & 0x3f)
	if batteryIdentity&0xc000 != 0 || batteryID == 0 || statusWords[28] == 0 ||
		baseSlices[0].Words[0] == 0 || statusWords[0] == 0 || statusWords[1] == 0 || input.Slices[0].Words[0] == 0 {
		return GrowattBMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic identity evidence is insufficient")
	}

	result := GrowattBMSMultiGroupDiagnostic{
		Revision: input.Revision, BatteryID: batteryID, ReportedGroupID: input.Slices[0].Words[0],
		CellVoltageGroups: [2]GrowattBMSCellVoltageGroup{{Offset: 0x0071}, {Offset: 0x0081}},
	}
	for group := range result.CellVoltageGroups {
		copy(result.CellVoltageGroups[group].Millivolts[:], input.Slices[group+1].Words)
	}
	balanceWord := extensionWords[9]
	for cell := range result.NativeBalanceStates {
		result.NativeBalanceStates[cell] = GrowattBMSNativeCellState(balanceWord&(1<<uint(cell)) != 0)
	}
	result.rawSlices = []GrowattBMSDiagnosticRawSlice{
		growattBMSDiagnosticRawSlice(input, 0x001f, 0x000d, []uint16{batteryIdentity}),
		growattBMSDiagnosticRawSlice(input, 0x0070, 0x0070, input.Slices[0].Words),
		growattBMSDiagnosticRawSlice(input, 0x0071, 0x0071, input.Slices[1].Words),
		growattBMSDiagnosticRawSlice(input, 0x0081, 0x0081, input.Slices[2].Words),
		growattBMSDiagnosticRawSlice(input, 0x0109, 0x0100, []uint16{balanceWord}),
	}
	return result, nil
}

func growattBMSExactRevision() GrowattBMSRevisionTuple {
	return GrowattBMSRevisionTuple{
		Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
		HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
	}
}

func growattBMSDiagnosticRawSlice(
	input GrowattBMSMultiGroupDiagnosticInput,
	offset, sourceSliceOffset uint16,
	words []uint16,
) GrowattBMSDiagnosticRawSlice {
	return GrowattBMSDiagnosticRawSlice{
		UnitID: input.UnitID, Revision: input.Revision, Function: input.Function,
		Offset: offset, SourceSliceOffset: sourceSliceOffset, Words: append([]uint16(nil), words...),
	}
}
