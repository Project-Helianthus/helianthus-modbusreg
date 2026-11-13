package growatt

import "fmt"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

const (
	growattBMSFamily             = "1xSxxP ESS"
	growattBMSFileRevision       = "Rev2.01"
	growattBMSHeaderVersion      = "V2.0"
	growattBMSCumulativeRevision = "2.02"
)

// BMSRevisionTuple is supplied independently of wire values because
// the bounded read set does not identify a protocol revision by itself.
type BMSRevisionTuple struct {
	Family, FileRevision, HeaderVersion, CumulativeRevision string
}

// BMSReadOnlySlice is one exact FC03 PDU slice. Its words are opaque
// until a separate field-definition contract makes a particular field typed.
type BMSReadOnlySlice struct {
	Offset uint16
	Words  []uint16
}

// BMSReadOnlyInput is an already-read, caller-supplied offline
// observation. It carries no endpoint, session, request, or write authority.
type BMSReadOnlyInput struct {
	UnitID   byte
	Function modbusreg.FunctionCode
	Revision BMSRevisionTuple
	Slices   []BMSReadOnlySlice
}

// BMSReadOnlyObservation retains exactly one validated, raw-only
// candidate observation. It creates no detector, catalog, runtime activation,
// or telemetry publication surface.
type BMSReadOnlyObservation struct {
	unitID   byte
	revision BMSRevisionTuple
	slices   []BMSReadOnlySlice
}

// UnitID returns the validated Modbus unit identifier (1-247).
func (o BMSReadOnlyObservation) UnitID() byte { return o.unitID }

// Revision returns the caller-supplied revision tuple the observation was validated against.
func (o BMSReadOnlyObservation) Revision() BMSRevisionTuple {
	return o.revision
}

// Slices returns a deep copy of the raw FC03 slices in contract order.
func (o BMSReadOnlyObservation) Slices() []BMSReadOnlySlice {
	return cloneGrowattBMSReadOnlySlices(o.slices)
}

// OutboundAllowed is permanently false: this observation can never authorize
// a control operation or a further Modbus request.
func (BMSReadOnlyObservation) OutboundAllowed() bool { return false }

// DecodeGrowattBMSReadOnlyObservation validates the four individually bounded
// FC03 slices permitted by the Growatt BMS read-only contract.
func DecodeGrowattBMSReadOnlyObservation(input BMSReadOnlyInput) (BMSReadOnlyObservation, error) {
	if input.UnitID == 0 || input.UnitID > 247 || input.Function != modbusreg.FunctionReadHoldingRegisters ||
		input.Revision != (BMSRevisionTuple{
			Family: growattBMSFamily, FileRevision: growattBMSFileRevision,
			HeaderVersion: growattBMSHeaderVersion, CumulativeRevision: growattBMSCumulativeRevision,
		}) {
		return BMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only observation identity is invalid")
	}
	want := [...]struct{ offset, words uint16 }{
		{0x0001, 7}, {0x000d, 29}, {0x0100, 12}, {0x010d, 2},
	}
	if len(input.Slices) != len(want) {
		return BMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only slice count is invalid")
	}
	for index, expected := range want {
		slice := input.Slices[index]
		if slice.Offset != expected.offset || len(slice.Words) != int(expected.words) {
			return BMSReadOnlyObservation{}, fmt.Errorf("growatt BMS read-only slice %d is invalid", index)
		}
	}
	return BMSReadOnlyObservation{
		unitID:   input.UnitID,
		revision: input.Revision,
		slices:   cloneGrowattBMSReadOnlySlices(input.Slices),
	}, nil
}

func cloneGrowattBMSReadOnlySlices(slices []BMSReadOnlySlice) []BMSReadOnlySlice {
	cloned := make([]BMSReadOnlySlice, len(slices))
	for index, slice := range slices {
		cloned[index] = BMSReadOnlySlice{Offset: slice.Offset, Words: append([]uint16(nil), slice.Words...)}
	}
	return cloned
}
