package sunspec

import (
	"fmt"
	"strconv"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// Monitoring capability identifiers for per-segment SunSpec evaluations.
const (
	InverterMonitoringCapabilityID = "sunspec.inverter.monitoring"
	MPPTMonitoringCapabilityID     = "sunspec.mppt.monitoring"
	StorageMonitoringCapabilityID  = "sunspec.storage.monitoring"
	MeterMonitoringCapabilityID    = "sunspec.meter.monitoring"
	DERMeasurementCapabilityID     = "sunspec.der.measurement"
	DERStateCapabilityID           = "sunspec.der.state"
)

// CapabilityEntry holds the admitted facts for a single repeated group. Index
// is the one-based wire group position, not a register or a module identifier.
// Role is "unknown" for model 160 modules and empty for other groups.
type CapabilityEntry struct {
	GroupID string
	Index   uint16
	Role    string
	Facts   []CapabilityFact
}

// RepeatedEntries returns detached entries in wire order. Facts within an entry
// start with the derived index and retain any decoded group metadata, including
// a DC port's type when implemented.
func (d CapabilityDecision) RepeatedEntries() []CapabilityEntry {
	out := make([]CapabilityEntry, len(d.entries))
	for i, entry := range d.entries {
		entry.Facts = cloneSunSpecCapabilityFacts(entry.Facts)
		out[i] = entry
	}
	return out
}

// CommonOccurrence returns the opening Common model of this decision's segment.
func (d CapabilityDecision) CommonOccurrence() (Occurrence, bool) {
	if d.common == nil {
		return Occurrence{}, false
	}
	return cloneOccurrence(*d.common), true
}

// Topology returns the phase topology declared by an admitted inverter model.
func (d CapabilityDecision) Topology() Topology {
	if d.topology == "" {
		return TopologyNone
	}
	return d.topology
}

// ScaleFactor returns the native base-ten exponent if this is a scaled value.
// An unscaled point (including any IEEE float) has no scale factor.
func (f CapabilityFact) ScaleFactor() (int16, bool) {
	if d, ok := f.sourceValue.Decimal(); ok {
		return d.Exponent, true
	}
	if d, ok := f.sourceValue.UnsignedDecimal(); ok {
		return d.Exponent, true
	}
	return 0, false
}

type capabilityProjection struct {
	facts   []CapabilityFact
	entries []CapabilityEntry
}

type capabilityRule struct {
	id      string
	project func(DecodedModel) (capabilityProjection, bool)
}

var (
	inverterCapabilityRule       = capabilityRule{InverterMonitoringCapabilityID, projectInverterCapability}
	stateCapabilityRule          = capabilityRule{DERStateCapabilityID, projectDERState}
	storageCapabilityRule        = capabilityRule{StorageMonitoringCapabilityID, projectStorage}
	meterCapabilityRule          = capabilityRule{MeterMonitoringCapabilityID, projectMeter}
	batteryCapabilityRule        = capabilityRule{StorageMonitoringCapabilityID, projectBatteryCapability}
	derMeasurementCapabilityRule = capabilityRule{DERMeasurementCapabilityID, projectDERMeasurement}
)

var monitoringCapabilityRules = map[uint16][]capabilityRule{
	101: {inverterCapabilityRule}, 102: {inverterCapabilityRule}, 103: {inverterCapabilityRule},
	111: {inverterCapabilityRule}, 112: {inverterCapabilityRule}, 113: {inverterCapabilityRule},
	120: {stateCapabilityRule, storageCapabilityRule}, 121: {stateCapabilityRule}, 122: {stateCapabilityRule}, 123: {stateCapabilityRule},
	124: {storageCapabilityRule}, 160: {{MPPTMonitoringCapabilityID, projectMPPT}},
	201: {meterCapabilityRule}, 202: {meterCapabilityRule}, 203: {meterCapabilityRule}, 204: {meterCapabilityRule},
	211: {meterCapabilityRule}, 212: {meterCapabilityRule}, 213: {meterCapabilityRule}, 214: {meterCapabilityRule},
	701: {derMeasurementCapabilityRule}, 713: {batteryCapabilityRule}, 714: {derMeasurementCapabilityRule},
	802: {batteryCapabilityRule}, 803: {batteryCapabilityRule}, 804: {batteryCapabilityRule}, 805: {batteryCapabilityRule},
}

// EvaluateCapabilities verifies a terminal snapshot and evaluates each Common
// segment with the same capability table. A source is decoded once under its
// retained decoder key, including in chains with both V1 and V2 models.
// Results follow occurrence order, with separate decisions for repeated model
// occurrences. Leading sources without a Common anchor are ambiguous. Bad
// required values or bad model geometry publish no facts for that source;
// a bad Common model prevents admission of its segment. Evaluation performs no I/O.
func EvaluateCapabilities(snapshot ChainSnapshot) ([]CapabilityDecision, error) {
	if _, ok := snapshot.WireChain(); !ok {
		return nil, fmt.Errorf("SunSpec capability snapshot is not a verified terminal chain")
	}
	segments := SegmentChain(snapshot)
	prefix := len(snapshot.occurrences)
	if len(segments) > 0 {
		prefix = int(segments[0].occurrences[0].Ordinal) - 1
	}
	registries := make(capabilityRegistries)
	var decisions []CapabilityDecision
	decisions = registries.evaluateSegment(decisions, nil, snapshot.occurrences[:prefix], snapshot.sources)
	for _, segment := range segments {
		decisions = registries.evaluateSegment(decisions, &segment.occurrences[0], segment.occurrences[1:], snapshot.sources)
	}
	return decisions, nil
}

type capabilityRegistries map[SchemaRevision]DecoderRegistry

func (registries capabilityRegistries) decode(source Occurrence) (DecodedModel, CapabilityReason) {
	key, ok := source.DecoderKey()
	if !ok {
		return DecodedModel{}, CapabilityReasonSourceUnsupported
	}
	registry, loaded := registries[key.SchemaRevision]
	if !loaded {
		var err error
		registry, err = NewStandardSunSpecDecoderRegistry(key.SchemaRevision)
		if err != nil {
			return DecodedModel{}, CapabilityReasonSourceUnsupported
		}
		registries[key.SchemaRevision] = registry
	}
	model, err := registry.DecodeOccurrence(source)
	switch {
	case err != nil:
		return DecodedModel{}, CapabilityReasonSourceUnsupported
	case !model.GeometryValid():
		return DecodedModel{}, CapabilityReasonInvalidChain
	default:
		return model, CapabilityReasonAdmitted
	}
}

func (registries capabilityRegistries) evaluateSegment(decisions []CapabilityDecision, common *Occurrence, sources []Occurrence, views []modbusreg.LogicalViewSnapshot) []CapabilityDecision {
	reason := CapabilityReasonAmbiguousSource
	if common != nil {
		model, admission := registries.decode(*common)
		reason = CapabilityReasonInvalidChain
		if admission == CapabilityReasonAdmitted && model.Qualifies() {
			reason = CapabilityReasonAdmitted
		}
	}
	for _, source := range sources {
		rules := monitoringCapabilityRules[source.ModelID()]
		if len(rules) == 0 {
			continue
		}
		model, sourceReason := DecodedModel{}, reason
		if sourceReason == CapabilityReasonAdmitted {
			model, sourceReason = registries.decode(source)
		}
		for _, rule := range rules {
			decision := CapabilityDecision{profileID: rule.id, common: common, source: &source, reason: sourceReason}
			if sourceReason == CapabilityReasonAdmitted {
				projection, ok := rule.project(model)
				if !ok {
					decision.reason = CapabilityReasonInvalidRequiredFact
				} else {
					decision.admitted, decision.topology = true, model.Topology()
					decision.facts, decision.entries, decision.views = projection.facts, projection.entries, views
				}
			}
			decisions = append(decisions, decision)
		}
	}
	return decisions
}

// projectCapabilityPoints uses the decoded catalog's units, types, scale and
// required flags. The selector names only points in the monitoring contract,
// and can override the published identifier, unit or meter sign convention.
func projectCapabilityPoints(model DecodedModel, indexID, role string, selectPoint func(Fact) (id, unit string, sign MeterSignConvention)) (capabilityProjection, bool) {
	var out capabilityProjection
	for _, point := range model.facts {
		id, unit, sign := selectPoint(point)
		if id == "" || point.Value.PointType() == TypeScaleFactor || point.Value.PointType() == TypePad {
			continue
		}
		if point.Repeated && startsCapabilityEntry(out.entries, point) {
			out.entries = append(out.entries, CapabilityEntry{
				GroupID: point.GroupID, Index: point.RepeatIndex, Role: role,
				Facts: []CapabilityFact{{fieldID: indexID, unit: "none", value: CanonicalValue{kind: CanonicalNumber, number: strconv.FormatUint(uint64(point.RepeatIndex), 10)}}},
			})
		}
		fact, ok := projectCapabilityFact(point, id, unit, sign, point.Required)
		if !ok {
			if point.Required {
				return capabilityProjection{}, false
			}
			continue
		}
		if point.Repeated {
			entry := &out.entries[len(out.entries)-1]
			entry.Facts = append(entry.Facts, fact)
		} else {
			out.facts = append(out.facts, fact)
		}
	}
	return out, true
}

func startsCapabilityEntry(entries []CapabilityEntry, point Fact) bool {
	if len(entries) == 0 {
		return true
	}
	last := entries[len(entries)-1]
	return last.GroupID != point.GroupID || last.Index != point.RepeatIndex
}

func projectCapabilityFact(point Fact, id, unit string, sign MeterSignConvention, requireCanonical bool) (CapabilityFact, bool) {
	if point.Value.State() != ValueValid {
		return CapabilityFact{}, false
	}
	value, ok := canonicalSunSpecValue(point.Value)
	if requireCanonical && !ok {
		return CapabilityFact{}, false
	}
	if unit == "" {
		unit = "none"
	}
	return CapabilityFact{fieldID: id, unit: unit, sourceValue: point.Value, value: value, sign: sign}, true
}

var inverterExtendedOptionalFields = []sunSpecCapabilityField{
	{"inverter.ac.power_factor", "Pct", "Pct"},
	{"inverter.dc.current", "A", "A"},
	{"inverter.dc.voltage", "V", "V"},
	{"inverter.dc.power", "W", "W"},
	{"inverter.temperature.heatsink", "C", "C"},
	{"inverter.temperature.transformer", "C", "C"},
	{"inverter.temperature.other", "C", "C"},
	{"inverter.vendor_state", "", "none"},
	{"inverter.vendor_events.1", "", "none"},
	{"inverter.vendor_events.2", "", "none"},
	{"inverter.vendor_events.3", "", "none"},
	{"inverter.vendor_events.4", "", "none"},
}

func projectInverterCapability(model DecodedModel) (capabilityProjection, bool) {
	facts, ok := inverterCapabilityFacts(model)
	return capabilityProjection{facts: facts}, ok
}

func inverterCapabilityFacts(model DecodedModel) ([]CapabilityFact, bool) {
	byID := make(map[string][]Fact, len(model.facts))
	for _, f := range model.facts {
		byID[f.FieldID] = append(byID[f.FieldID], f)
	}
	var facts []CapabilityFact
	for _, fields := range []struct {
		rows                []sunSpecCapabilityField
		required, canonical bool
	}{
		{threePhaseMonitoringFields, true, true},
		{threePhaseMonitoringOptionalFields, false, true},
		{inverterExtendedOptionalFields, false, false},
	} {
		for _, row := range fields.rows {
			if !inverterPhaseIncludes(model.Topology(), row.id) {
				continue
			}
			matches := byID[row.id]
			if len(matches) == 1 && matches[0].Unit == row.sourceUnit && (!fields.required || matches[0].Required) {
				if fact, ok := projectCapabilityFact(matches[0], row.id, row.unit, "", fields.canonical); ok {
					facts = append(facts, fact)
					continue
				}
			}
			if fields.required {
				return nil, false
			}
		}
	}
	return facts, true
}

func inverterPhaseIncludes(topology Topology, id string) bool {
	switch id {
	case "inverter.ac.current.phase_c", "inverter.ac.voltage.phase_c", "inverter.ac.voltage.line_bc", "inverter.ac.voltage.line_ca":
		return topology == TopologyThreePhase
	case "inverter.ac.current.phase_b", "inverter.ac.voltage.phase_b", "inverter.ac.voltage.line_ab":
		return topology == TopologyThreePhase || topology == TopologySplitPhase
	default:
		return true
	}
}

// derMonitoringRows names only documented fixed and repeated monitoring points.
// The two scopes remain separate even when the register point names coincide.
type derMonitoringRows struct {
	fixed, repeated map[string]string
	indexID         string
}

// Signed DER current and directional energy keep distinct identifiers; only
// quantities with matching meanings share inverter or meter fact identifiers.
var derMeasurementPoints = map[uint16]derMonitoringRows{
	701: {
		fixed: map[string]string{
			"ACType":       "der.ac.wiring_type",
			"St":           "der.ac.operating_state",
			"InvSt":        "der.ac.inverter_state",
			"ConnSt":       "der.ac.connection_state",
			"Alrm":         "der.ac.alarm",
			"DERMode":      "der.ac.mode",
			"W":            "inverter.ac.power.active",
			"VA":           "inverter.ac.power.apparent",
			"Var":          "inverter.ac.power.reactive",
			"PF":           "inverter.ac.power_factor",
			"A":            "der.ac.current",
			"LLV":          "meter.ac.voltage.line_line.average",
			"LNV":          "meter.ac.voltage.line_neutral.average",
			"Hz":           "inverter.ac.frequency",
			"TotWhInj":     "der.ac.energy.injected",
			"TotWhAbs":     "der.ac.energy.absorbed",
			"TotVarhInj":   "der.ac.energy.reactive_injected",
			"TotVarhAbs":   "der.ac.energy.reactive_absorbed",
			"TmpAmb":       "der.ac.temperature.ambient",
			"TmpCab":       "inverter.temperature.cabinet",
			"TmpSnk":       "inverter.temperature.heatsink",
			"TmpTrns":      "inverter.temperature.transformer",
			"TmpSw":        "der.ac.temperature.switchgear",
			"TmpOt":        "inverter.temperature.other",
			"WL1":          "der.ac.power.active.phase_a",
			"VAL1":         "der.ac.power.apparent.phase_a",
			"VarL1":        "der.ac.power.reactive.phase_a",
			"PFL1":         "der.ac.power_factor.phase_a",
			"AL1":          "der.ac.current.phase_a",
			"VL1L2":        "inverter.ac.voltage.line_ab",
			"VL1":          "inverter.ac.voltage.phase_a",
			"TotWhInjL1":   "der.ac.energy.injected.phase_a",
			"TotWhAbsL1":   "der.ac.energy.absorbed.phase_a",
			"TotVarhInjL1": "der.ac.energy.reactive_injected.phase_a",
			"TotVarhAbsL1": "der.ac.energy.reactive_absorbed.phase_a",
			"WL2":          "der.ac.power.active.phase_b",
			"VAL2":         "der.ac.power.apparent.phase_b",
			"VarL2":        "der.ac.power.reactive.phase_b",
			"PFL2":         "der.ac.power_factor.phase_b",
			"AL2":          "der.ac.current.phase_b",
			"VL2L3":        "inverter.ac.voltage.line_bc",
			"VL2":          "inverter.ac.voltage.phase_b",
			"TotWhInjL2":   "der.ac.energy.injected.phase_b",
			"TotWhAbsL2":   "der.ac.energy.absorbed.phase_b",
			"TotVarhInjL2": "der.ac.energy.reactive_injected.phase_b",
			"TotVarhAbsL2": "der.ac.energy.reactive_absorbed.phase_b",
			"WL3":          "der.ac.power.active.phase_c",
			"VAL3":         "der.ac.power.apparent.phase_c",
			"VarL3":        "der.ac.power.reactive.phase_c",
			"PFL3":         "der.ac.power_factor.phase_c",
			"AL3":          "der.ac.current.phase_c",
			"VL3L1":        "inverter.ac.voltage.line_ca",
			"VL3":          "inverter.ac.voltage.phase_c",
			"TotWhInjL3":   "der.ac.energy.injected.phase_c",
			"TotWhAbsL3":   "der.ac.energy.absorbed.phase_c",
			"TotVarhInjL3": "der.ac.energy.reactive_injected.phase_c",
			"TotVarhAbsL3": "der.ac.energy.reactive_absorbed.phase_c",
			"ThrotPct":     "der.ac.throttle.percent",
			"ThrotSrc":     "der.ac.throttle.source",
			"MnAlrmInfo":   "der.ac.alarm_info",
		},
	},
	714: {
		fixed: map[string]string{
			"PrtAlrms": "der.dc.alarm",
			"NPrt":     "der.dc.port.count",
			"DCA":      "der.dc.current",
			"DCW":      "der.dc.power",
			"DCWhInj":  "der.dc.energy.injected",
			"DCWhAbs":  "der.dc.energy.absorbed",
		},
		repeated: map[string]string{
			"PrtTyp":  "der.dc.port.type",
			"ID":      "der.dc.port.id",
			"IDStr":   "der.dc.port.label",
			"DCA":     "der.dc.port.current",
			"DCV":     "der.dc.port.voltage",
			"DCW":     "der.dc.port.power",
			"DCWhInj": "der.dc.port.energy.injected",
			"DCWhAbs": "der.dc.port.energy.absorbed",
			"Tmp":     "der.dc.port.temperature",
			"DCSta":   "der.dc.port.state",
			"DCAlrm":  "der.dc.port.alarm",
		},
		indexID: "der.dc.port.index",
	},
}

func projectDERMeasurement(model DecodedModel) (capabilityProjection, bool) {
	return projectDERMonitoring(model, derMeasurementPoints[model.Key().ModelID])
}

func projectDERMonitoring(model DecodedModel, points derMonitoringRows) (capabilityProjection, bool) {
	return projectCapabilityPoints(model, points.indexID, "", func(point Fact) (string, string, MeterSignConvention) {
		unit := derMonitoringUnit(model.Key().ModelID, point)
		if point.Repeated {
			return points.repeated[point.PointName], unit, ""
		}
		return points.fixed[point.PointName], unit, ""
	})
}

// The monitoring contract supplies percent units where the decoder catalog
// leaves the unit token empty. The values remain percent, not ratios.
func derMonitoringUnit(id uint16, point Fact) string {
	switch id {
	case 701:
		switch point.PointName {
		case "PF", "PFL1", "PFL2", "PFL3", "ThrotPct":
			return "Pct"
		}
	case 713:
		if point.PointName == "SoC" || point.PointName == "SoH" {
			return "Pct"
		}
	case 803:
		if point.PointName == "StrSoC" || point.PointName == "StrSoH" {
			return "%"
		}
	}
	return point.Unit
}
