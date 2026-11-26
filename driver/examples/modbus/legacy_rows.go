package modbus

import (
	"bytes"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

// LegacySelector addresses a slice of the complete post-command payload.
type LegacySelector struct {
	Command                              tesla.LegacyCommand
	Direction                            tesla.LegacyDirection
	PayloadLength, ByteOffset, ByteCount uint16
}

var (
	legacyRequest  = tesla.LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}
	legacyResponse = tesla.LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}
	evsePack       = semreg.PackRef{ID: "helianthus.pack.evse", Version: "1.0.0"}
)

func legacyRows(length uint16, policy semreg.PolicyID) roweval.Table[LegacySelector] {
	var table roweval.Table[LegacySelector]
	if length != 11 && length != 13 {
		return table
	}
	for _, direction := range []tesla.LegacyDirection{tesla.LegacyWallConnectorRequest, tesla.LegacyWallConnectorResponse} {
		command, prefix := legacyRequest, "fbe0"
		if direction == tesla.LegacyWallConnectorResponse {
			command, prefix = legacyResponse, "fde0"
		}
		for _, point := range []struct {
			name, unit    string
			offset, count uint16
		}{
			{"state", "", 4, 1}, {"allocation", "cA", 5, 2}, {"actual_current", "cA", 7, 2},
		} {
			if direction == tesla.LegacyWallConnectorRequest && point.offset == 7 {
				continue
			}
			at := LegacySelector{command, direction, length, point.offset, point.count}
			table.Native = append(table.Native, roweval.NativeRow[LegacySelector]{ID: prefix + "." + point.name, Unit: point.unit, SpecRef: prefix + "." + point.name, At: at, Wire: layout(point.offset, point.count, "uint")})
		}
	}
	table.Bindings = []roweval.BindingRow{
		connectorBinding("fde0.state", "evse.status.charging", "evse.status.charging", policy, "charging", roweval.Convert{Symbols: map[string]string{"9": "evse.status.charging.charging"}}),
		connectorBinding("fde0.allocation", "evse.limit.allocated_current", "unit.ampere", policy, "absolute-allocation", affine(1, 100, "")),
		connectorBinding("fde0.actual_current", "evse.limit.actual_current", "unit.ampere", policy, "", affine(1, 100, "")),
	}
	return table
}

func connectorBinding(native string, fact, unit semreg.DefinitionID, policy semreg.PolicyID, feature string, convert roweval.Convert) roweval.BindingRow {
	b := roweval.BindingRow{NativeID: native, SubjectRole: "connector", SpecRef: native,
		Service: ref(evsePack, "evse.service.connector"), Capability: ref(evsePack, "evse.capability.read.connector"),
		Read: &roweval.ReadRow{Fact: ref(evsePack, fact), Unit: unit, Policy: policy, ToRegistry: convert, Dimensions: map[semreg.DefinitionID]string{"evse.dimension.connector": "connector-resource"}}}
	if feature != "" {
		b.When.Requires = []string{feature}
	}
	return b
}

type legacyEvaluation struct {
	Raw []byte
	roweval.Evaluation
}

func evaluateLegacyRequest(table roweval.Table[LegacySelector], spec tesla.LegacyFBE0ObservationSpec, a roweval.Acquired[LegacySelector]) (legacyEvaluation, error) {
	evidence, err := tesla.NewLegacyFBE0Observation(spec)
	if err != nil {
		return legacyEvaluation{Raw: bytes.Clone(spec.Raw)}, err
	}
	return evaluateLegacy(table, evidence.Raw(), spec.Direction, a)
}

func evaluateLegacyResponse(table roweval.Table[LegacySelector], spec tesla.LegacyFDE0ObservationSpec, a roweval.Acquired[LegacySelector]) (legacyEvaluation, error) {
	evidence, err := tesla.NewLegacyFDE0Observation(spec)
	if err != nil {
		return legacyEvaluation{Raw: bytes.Clone(spec.Raw)}, err
	}
	_, absolute := evidence.AbsoluteOfferCA()
	a.Qualifiers.Features = map[string]bool{"charging": evidence.State() == tesla.LegacyFDE0StateCharging, "absolute-allocation": absolute}
	return evaluateLegacy(table, evidence.Raw(), spec.Direction, a)
}

func evaluateLegacy(table roweval.Table[LegacySelector], raw []byte, direction tesla.LegacyDirection, a roweval.Acquired[LegacySelector]) (legacyEvaluation, error) {
	frame, err := tesla.DecodeTeslaLegacyWallConnectorFrame(raw)
	if err != nil {
		return legacyEvaluation{Raw: raw}, err
	}
	payload := frame.Payload()
	if a.Selector.Command != frame.Command() || a.Selector.Direction != direction || int(a.Selector.PayloadLength) != len(payload) {
		return legacyEvaluation{Raw: raw, Evaluation: roweval.Evaluation{Reason: roweval.ReasonNoMatch}}, nil
	}
	a.Descriptors = descriptors[LegacySelector]()
	a.Payload = payload
	got, err := roweval.Evaluate(table, a)
	return legacyEvaluation{Raw: raw, Evaluation: got}, err
}
