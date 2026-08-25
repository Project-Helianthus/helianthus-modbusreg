package modbusreg

import "fmt"

const (
	growattBMSFamily             = "1xSxxP ESS"
	growattBMSFileRevision       = "Rev2.01"
	growattBMSHeaderVersion      = "V2.0"
	growattBMSCumulativeRevision = "2.02"
)

// GrowattBMSRevisionTuple is supplied independently of wire values because
// the bounded read set does not identify a protocol revision by itself.
type GrowattBMSRevisionTuple struct {
	Family, FileRevision, HeaderVersion, CumulativeRevision string
}

// GrowattBMSReadOnlySlice is one exact FC03 PDU slice. Its words are opaque
// until a separate field-definition contract makes a particular field typed.
type GrowattBMSReadOnlySlice struct {
	Offset uint16
	Words  []uint16
}

// GrowattBMSReadOnlyInput is an already-read, caller-supplied offline
// observation. It carries no endpoint, session, request, or write authority.
type GrowattBMSReadOnlyInput struct {
	UnitID   byte
	Function FunctionCode
	Revision GrowattBMSRevisionTuple
	Slices   []GrowattBMSReadOnlySlice
}

// GrowattBMSReadOnlyObservation retains exactly one validated, raw-only
// candidate observation. It creates no detector, catalog, runtime activation,
// or telemetry publication surface.
type GrowattBMSReadOnlyObservation struct {
	unitID   byte
	revision GrowattBMSRevisionTuple
	slices   []GrowattBMSReadOnlySlice
}

func (o GrowattBMSReadOnlyObservation) UnitID() byte { return o.unitID }
func (o GrowattBMSReadOnlyObservation) Revision() GrowattBMSRevisionTuple {
	return o.revision
}
func (o GrowattBMSReadOnlyObservation) Slices() []GrowattBMSReadOnlySlice {
	return cloneGrowattBMSReadOnlySlices(o.slices)
}

// OutboundAllowed is permanently false: this observation can never authorize
// a control operation or a further Modbus request.
func (GrowattBMSReadOnlyObservation) OutboundAllowed() bool { return false }

// DecodeGrowattBMSReadOnlyObservation validates the four individually bounded
// FC03 slices permitted by the Growatt BMS read-only contract.
func DecodeGrowattBMSReadOnlyObservation(input GrowattBMSReadOnlyInput) (GrowattBMSReadOnlyObservation, error) {
	if input.UnitID == 0 || input.Function != FunctionReadHoldingRegisters ||
		input.Revision != (GrowattBMSRevisionTuple{
			Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
			HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
		}) {
		return GrowattBMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only observation identity is invalid")
	}
	want := [...]struct{ offset, words uint16 }{
		{0x0001, 7}, {0x000d, 29}, {0x0100, 12}, {0x010d, 2},
	}
	if len(input.Slices) != len(want) {
		return GrowattBMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only slice count is invalid")
	}
	for index, expected := range want {
		slice := input.Slices[index]
		if slice.Offset != expected.offset || len(slice.Words) != int(expected.words) {
			return GrowattBMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only slice %d is invalid", index)
		}
	}
	return GrowattBMSReadOnlyObservation{
		unitID:   input.UnitID,
		revision: input.Revision,
		slices:   cloneGrowattBMSReadOnlySlices(input.Slices),
	}, nil
}

func cloneGrowattBMSReadOnlySlices(slices []GrowattBMSReadOnlySlice) []GrowattBMSReadOnlySlice {
	cloned := make([]GrowattBMSReadOnlySlice, len(slices))
	for index, slice := range slices {
		cloned[index] = GrowattBMSReadOnlySlice{Offset: slice.Offset, Words: append([]uint16(nil), slice.Words...)}
	}
	return cloned
}
