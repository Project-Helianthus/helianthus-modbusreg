package growatt

import "fmt"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

// BMSMultiGroupDiagnosticInput is one caller-supplied, read-only
// diagnostic observation. It has no endpoint, session, or request authority.
// Its three slices are admitted only after a complete base status qualifies.
type BMSMultiGroupDiagnosticInput struct {
	UnitID   byte
	Function modbusreg.FunctionCode
	Revision BMSRevisionTuple
	Slices   []BMSReadOnlySlice
}

// BMSDiagnosticRawSlice retains one exact diagnostic word range with
// its selected Modbus tuple and the base or diagnostic slice from which it
// came. Words are immutable to callers of RawSlices.
type BMSDiagnosticRawSlice struct {
	UnitID            byte
	Revision          BMSRevisionTuple
	Function          modbusreg.FunctionCode
	Offset            uint16
	SourceSliceOffset uint16
	Words             []uint16
}

// BMSCellVoltageGroup is one of the two documented fixed cell blocks.
// Its ordinal is not a pack number, selector, or discovery result.
type BMSCellVoltageGroup struct {
	Offset     uint16
	Millivolts [16]uint16
}

// BMSNativeCellState is the documented native off/on bit value of one
// cell in the revision-scoped 0x0109 word. It has no control semantics.
type BMSNativeCellState bool

// Native cell bit values: Off is a clear bit and On is a set bit of the 0x0109 word.
const (
	BMSNativeCellStateOff BMSNativeCellState = false
	BMSNativeCellStateOn  BMSNativeCellState = true
)

// BMSMultiGroupDiagnostic is a bounded, independent diagnostic view.
// It neither changes nor extends the qualified base telemetry result.
type BMSMultiGroupDiagnostic struct {
	Revision            BMSRevisionTuple
	BatteryID           uint8
	ReportedGroupID     uint16
	CellVoltageGroups   [2]BMSCellVoltageGroup
	NativeBalanceStates [16]BMSNativeCellState
	rawSlices           []BMSDiagnosticRawSlice
}

// RawSlices returns independent retained raw evidence for 0x001F, 0x0070,
// 0x0071--0x0080, 0x0081--0x0090, and 0x0109, in that fixed order.
func (diagnostic BMSMultiGroupDiagnostic) RawSlices() []BMSDiagnosticRawSlice {
	result := make([]BMSDiagnosticRawSlice, len(diagnostic.rawSlices))
	for index, slice := range diagnostic.rawSlices {
		result[index] = BMSDiagnosticRawSlice{
			UnitID: slice.UnitID, Revision: slice.Revision, Function: slice.Function,
			Offset: slice.Offset, SourceSliceOffset: slice.SourceSliceOffset,
			Words: append([]uint16(nil), slice.Words...),
		}
	}
	return result
}

// OutboundAllowed is permanently false: a diagnostic observation cannot
// authorize any follow-up request, selector, handshake, or control action.
func (BMSMultiGroupDiagnostic) OutboundAllowed() bool { return false }

// DecodeGrowattBMSMultiGroupDiagnostic decodes the three fixed FC03 slices
// only after a valid base status from the same selected unicast unit and exact
// revision. It returns no partial diagnostic on insufficient evidence. This is
// deliberately one stateless observation: a lifecycle consumer that retains
// observations must reject changing, duplicate, missing, or contradictory pack
// identity before it emits repeated facts; this decoder never merges groups or
// establishes cross-observation identity continuity.
func DecodeGrowattBMSMultiGroupDiagnostic(
	base BMSTypedReadOnlyStatus,
	input BMSMultiGroupDiagnosticInput,
) (BMSMultiGroupDiagnostic, error) {
	baseObservation := base.NativeObservation()
	if base.nativeObservation == nil || input.UnitID == 0 || input.UnitID > 247 ||
		input.Function != modbusreg.FunctionReadHoldingRegisters || input.Revision != growattBMSExactRevision() ||
		input.UnitID != baseObservation.UnitID() || input.Revision != baseObservation.Revision() {
		return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic identity is invalid")
	}
	if _, err := DecodeGrowattBMSTypedReadOnlyStatus(BMSReadOnlyInput{
		UnitID: baseObservation.UnitID(), Function: modbusreg.FunctionReadHoldingRegisters,
		Revision: baseObservation.Revision(), Slices: baseObservation.Slices(),
	}); err != nil {
		return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group base status is unavailable: %w", err)
	}

	want := [...]struct{ offset, words uint16 }{{0x0070, 1}, {0x0071, 16}, {0x0081, 16}}
	if len(input.Slices) != len(want) {
		return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic slice count is invalid")
	}
	for index, expected := range want {
		if input.Slices[index].Offset != expected.offset || len(input.Slices[index].Words) != int(expected.words) {
			return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic slice %d is invalid", index)
		}
	}

	baseSlices := baseObservation.Slices()
	statusWords, extensionWords := baseSlices[1].Words, baseSlices[2].Words
	if len(statusWords) != 29 || len(extensionWords) != 12 {
		return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group base extents are invalid")
	}
	batteryIdentity := statusWords[18]
	batteryID := uint8((batteryIdentity >> 8) & 0x3f)
	if batteryIdentity&0xc000 != 0 {
		return BMSMultiGroupDiagnostic{}, fmt.Errorf("growatt BMS multi-group diagnostic identity evidence is insufficient")
	}

	result := BMSMultiGroupDiagnostic{
		Revision: input.Revision, BatteryID: batteryID, ReportedGroupID: input.Slices[0].Words[0],
		CellVoltageGroups: [2]BMSCellVoltageGroup{{Offset: 0x0071}, {Offset: 0x0081}},
	}
	for group := range result.CellVoltageGroups {
		copy(result.CellVoltageGroups[group].Millivolts[:], input.Slices[group+1].Words)
	}
	balanceWord := extensionWords[9]
	for cell := range result.NativeBalanceStates {
		result.NativeBalanceStates[cell] = BMSNativeCellState(balanceWord&(1<<uint(cell)) != 0)
	}
	result.rawSlices = []BMSDiagnosticRawSlice{
		growattBMSDiagnosticRawSlice(input, 0x001f, 0x000d, []uint16{batteryIdentity}),
		growattBMSDiagnosticRawSlice(input, 0x0070, 0x0070, input.Slices[0].Words),
		growattBMSDiagnosticRawSlice(input, 0x0071, 0x0071, input.Slices[1].Words),
		growattBMSDiagnosticRawSlice(input, 0x0081, 0x0081, input.Slices[2].Words),
		growattBMSDiagnosticRawSlice(input, 0x0109, 0x0100, []uint16{balanceWord}),
	}
	return result, nil
}

func growattBMSExactRevision() BMSRevisionTuple {
	return BMSRevisionTuple{
		Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
		HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
	}
}

func growattBMSDiagnosticRawSlice(
	input BMSMultiGroupDiagnosticInput,
	offset, sourceSliceOffset uint16,
	words []uint16,
) BMSDiagnosticRawSlice {
	return BMSDiagnosticRawSlice{
		UnitID: input.UnitID, Revision: input.Revision, Function: input.Function,
		Offset: offset, SourceSliceOffset: sourceSliceOffset, Words: append([]uint16(nil), words...),
	}
}
