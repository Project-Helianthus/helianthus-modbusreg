package sunspec

import (
	"fmt"
	"math/big"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Model 123 points, by decoded field ID. The five points from the percent
// through the enable are consecutive single-word points, so one Write Multiple
// Registers request covers them (Modbus Application Protocol V1.1b3, 6.12).
const (
	fieldWMaxLimPct     = "der.control.active_power_limit.percent"
	fieldWMaxLimWinTms  = "der.control.active_power_limit.window_seconds"
	fieldWMaxLimRvrtTms = "der.control.active_power_limit.revert_seconds"
	fieldWMaxLimRmpTms  = "der.control.active_power_limit.ramp_seconds"
	fieldWMaxLimEna     = "der.control.active_power_limit.enabled"
	fieldWMax           = "der.settings.max_active_power"
)

// ControlWriteReason names why a control write is answered before encoding.
type ControlWriteReason string

// Reasons a request is refused before any request is built.
const (
	ControlReasonModelAbsent        ControlWriteReason = "model_absent"
	ControlReasonOccurrenceInvalid  ControlWriteReason = "occurrence_invalid"
	ControlReasonBoundAbsent        ControlWriteReason = "bound_absent"
	ControlReasonArgumentOutOfBound ControlWriteReason = "argument_out_of_bound"
	ControlReasonPointUnavailable   ControlWriteReason = "point_unavailable"
	ControlReasonUnencodable        ControlWriteReason = "unencodable"
)

// ControlWriteError is a refusal before encoding. Nothing was built.
type ControlWriteError struct {
	Reason ControlWriteReason
	Detail string
}

func (e *ControlWriteError) Error() string {
	return fmt.Sprintf("SunSpec control write refused (%s): %s", e.Reason, e.Detail)
}

func controlRefusal(reason ControlWriteReason, format string, args ...any) error {
	return &ControlWriteError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// ReadbackState is the comparison of a re-read with what was written.
type ReadbackState string

// Match: every written point re-read as written. Mismatch: a point re-read
// with a different established value. Unknown: the re-read does not establish
// the written points.
const (
	ReadbackMatch    ReadbackState = "match"
	ReadbackMismatch ReadbackState = "mismatch"
	ReadbackUnknown  ReadbackState = "unknown"
)

type controlStepPoint struct {
	def    sunSpecPointDefinition
	offset uint16
	scale  *Value
}

// ControlWriteStep is one prepared FC16 request with the points it covers.
// It sends nothing and grants no runtime admission.
type ControlWriteStep struct {
	Fields  []string
	Request PointWriteRequest
	points  []controlStepPoint
}

// Admission returns the predicate that admits exactly this request's span.
func (q PointWriteRequest) Admission() modbus.RegisterWriteAdmission {
	return func(request modbus.RegisterWriteAdmissionRequest) bool {
		return q.WordCount != 0 && uint64(q.Function) == 16 &&
			uint64(request.Function) == uint64(modbus.FunctionWriteMultipleRegisters) &&
			request.Address == q.Address && request.Quantity == q.WordCount &&
			len(request.Registers) == int(q.WordCount)
	}
}

// Admission returns the predicate that admits exactly this step's span.
func (s ControlWriteStep) Admission() modbus.RegisterWriteAdmission { return s.Request.Admission() }

// CompareReadback compares the words re-read over this step's span with the
// words the step wrote. A re-read of the wrong width, or a different word that
// holds a not-implemented or invalid encoding, establishes nothing (unknown);
// a different established value is a mismatch and outranks an unknown point.
func (s ControlWriteStep) CompareReadback(words []uint16) ReadbackState {
	if s.Request.WordCount == 0 || len(words) != int(s.Request.WordCount) || len(s.points) == 0 {
		return ReadbackUnknown
	}
	state := ReadbackMatch
	for _, p := range s.points {
		end := int(p.offset) + int(p.def.size)
		got, wrote := words[p.offset:end], s.Request.Words[p.offset:end]
		if equalWords(got, wrote) {
			continue
		}
		if decodeSunSpecValue(p.def, got, p.scale).State() != ValueValid {
			state = ReadbackUnknown
			continue
		}
		return ReadbackMismatch
	}
	return state
}

// CombineReadback folds the comparisons of the steps of one command: a
// mismatch of any step is a mismatch, else any unknown (or no comparison) is
// unknown, else match.
func CombineReadback(states ...ReadbackState) ReadbackState {
	result := ReadbackMatch
	if len(states) == 0 {
		return ReadbackUnknown
	}
	for _, state := range states {
		switch state {
		case ReadbackMatch:
		case ReadbackMismatch:
			return ReadbackMismatch
		default:
			result = ReadbackUnknown
		}
	}
	return result
}

func equalWords(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ActivePowerLimitRequest asks for a limit of Watts on the model 123 instance
// at ControlsOrdinal, bounded by WMax of the model 121 instance at
// SettingsOrdinal. A nil Duration keeps the device's revert time.
type ActivePowerLimitRequest struct {
	SettingsOrdinal, ControlsOrdinal uint32
	Watts                            Decimal
	Duration                         *uint16
}

// EncodeActivePowerLimit builds the one contiguous request from WMaxLimPct
// through WMaxLim_Ena. The percent is Watts of WMax, rounded down in the
// point's scale. WMaxLimPct_WinTms and WMaxLimPct_RmpTms hold the values in
// the snapshot; WMaxLimPct_RvrtTms holds the requested duration, or the value
// in the snapshot when none is requested. WMaxLim_Ena is 1.
func EncodeActivePowerLimit(r DecoderRegistry, snapshot ChainSnapshot, q ActivePowerLimitRequest) (ControlWriteStep, error) {
	settings, definitions, err := controlsOccurrence(r, snapshot, q.SettingsOrdinal, 121)
	if err != nil {
		return ControlWriteStep{}, err
	}
	wmax, ok := fixedDecimal(settings, definitions, fieldWMax)
	if !ok {
		return ControlWriteStep{}, controlRefusal(ControlReasonBoundAbsent, "model 121 reports no WMax")
	}
	occurrence, _, err := controlsOccurrence(r, snapshot, q.ControlsOrdinal, 123)
	if err != nil {
		return ControlWriteStep{}, err
	}
	writes, err := PointWrites(r, snapshot, q.ControlsOrdinal)
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonOccurrenceInvalid, "%v", err)
	}
	percent, window, revert, ramp, enable, err := activePowerPoints(writes)
	if err != nil {
		return ControlWriteStep{}, err
	}
	rate, err := percentOfBound(q.Watts, wmax, int16(percent.scale.signed))
	if err != nil {
		return ControlWriteStep{}, err
	}
	pctRequest, err := percent.Encode(rate)
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "%v", err)
	}
	enableRequest, err := enable.Encode(uint16(1))
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "%v", err)
	}
	words := []uint16{pctRequest.Words[0], 0, 0, 0, enableRequest.Words[0]}
	words[1] = occurrence.words[window.address-occurrence.HeaderOffset]
	words[3] = occurrence.words[ramp.address-occurrence.HeaderOffset]
	words[2] = occurrence.words[revert.address-occurrence.HeaderOffset]
	if q.Duration != nil {
		duration, err := revert.Encode(*q.Duration)
		if err != nil {
			return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "%v", err)
		}
		words[2] = duration.Words[0]
	}
	step := ControlWriteStep{
		Fields:  []string{fieldWMaxLimPct, fieldWMaxLimWinTms, fieldWMaxLimRvrtTms, fieldWMaxLimRmpTms, fieldWMaxLimEna},
		Request: PointWriteRequest{Function: 16, Address: percent.address, WordCount: 5, Words: words},
	}
	for i, p := range []PointWrite{percent, window, revert, ramp, enable} {
		step.points = append(step.points, controlStepPoint{def: p.point, offset: uint16(i), scale: p.scale})
	}
	return step, nil
}

// EncodeActivePowerRelease builds the request that writes WMaxLim_Ena = 0 and
// no other point.
func EncodeActivePowerRelease(r DecoderRegistry, snapshot ChainSnapshot, controlsOrdinal uint32) (ControlWriteStep, error) {
	if _, _, err := controlsOccurrence(r, snapshot, controlsOrdinal, 123); err != nil {
		return ControlWriteStep{}, err
	}
	writes, err := PointWrites(r, snapshot, controlsOrdinal)
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonOccurrenceInvalid, "%v", err)
	}
	enable, ok := writeByField(writes, fieldWMaxLimEna)
	if !ok {
		return ControlWriteStep{}, controlRefusal(ControlReasonPointUnavailable, "model 123 offers no WMaxLim_Ena write")
	}
	request, err := enable.Encode(uint16(0))
	if err != nil {
		return ControlWriteStep{}, controlRefusal(ControlReasonUnencodable, "%v", err)
	}
	return ControlWriteStep{
		Fields:  []string{fieldWMaxLimEna},
		Request: request,
		points:  []controlStepPoint{{def: enable.point, scale: enable.scale}},
	}, nil
}

func activePowerPoints(writes []PointWrite) (percent, window, revert, ramp, enable PointWrite, err error) {
	fields := []struct {
		name string
		into *PointWrite
	}{
		{fieldWMaxLimPct, &percent}, {fieldWMaxLimWinTms, &window}, {fieldWMaxLimRvrtTms, &revert},
		{fieldWMaxLimRmpTms, &ramp}, {fieldWMaxLimEna, &enable},
	}
	for i, f := range fields {
		point, ok := writeByField(writes, f.name)
		if !ok {
			return percent, window, revert, ramp, enable, controlRefusal(ControlReasonPointUnavailable, "model 123 offers no write of %s", f.name)
		}
		*f.into = point
		if point.point.size != 1 || point.address != percent.address+uint16(i) {
			return percent, window, revert, ramp, enable, controlRefusal(ControlReasonPointUnavailable, "model 123 points are not one contiguous range at %s", f.name)
		}
	}
	if percent.scale == nil {
		return percent, window, revert, ramp, enable, controlRefusal(ControlReasonPointUnavailable, "WMaxLimPct has no usable scale factor")
	}
	return percent, window, revert, ramp, enable, nil
}

func writeByField(writes []PointWrite, field string) (PointWrite, bool) {
	for _, write := range writes {
		if write.point.fieldID == field && len(write.path.segments) == 0 && !write.point.repeated {
			return write, true
		}
	}
	return PointWrite{}, false
}

// controlsOccurrence returns the 1-based occurrence of the wanted model from a
// consistent terminal chain with its exact admitted definitions.
func controlsOccurrence(r DecoderRegistry, snapshot ChainSnapshot, ordinal uint32, modelID uint16) (Occurrence, []sunSpecPointDefinition, error) {
	if ordinal == 0 || uint64(ordinal) > uint64(len(snapshot.occurrences)) || snapshot.occurrences[ordinal-1].ModelID() != modelID {
		return Occurrence{}, nil, controlRefusal(ControlReasonModelAbsent, "no model %d at occurrence %d", modelID, ordinal)
	}
	if _, err := PointWrites(r, snapshot, ordinal); err != nil {
		return Occurrence{}, nil, controlRefusal(ControlReasonOccurrenceInvalid, "%v", err)
	}
	occurrence := snapshot.occurrences[ordinal-1]
	definitions, _, err := r.pointWriteDefinitions(occurrence)
	if err != nil || len(definitions) == 0 {
		return Occurrence{}, nil, controlRefusal(ControlReasonOccurrenceInvalid, "model %d has no exact definition", modelID)
	}
	return occurrence, definitions, nil
}

// fixedDecimal reads a scaled point of an occurrence as an exact decimal. It
// is false unless the point and its scale factor are valid and positive.
func fixedDecimal(o Occurrence, definitions []sunSpecPointDefinition, field string) (Decimal, bool) {
	for _, p := range definitions {
		if p.fieldID != field || p.repeated || p.scaleFactor == "" {
			continue
		}
		for _, s := range definitions {
			if s.pointType != TypeScaleFactor || s.name != p.scaleFactor {
				continue
			}
			scale := decodeSunSpecValue(s, o.words[s.offset:s.offset+s.size], nil)
			value := decodeSunSpecValue(p, o.words[p.offset:p.offset+p.size], &scale)
			decimal, ok := value.Decimal()
			if value.State() != ValueValid || !ok || decimal.Coefficient <= 0 {
				return Decimal{}, false
			}
			return decimal, true
		}
	}
	return Decimal{}, false
}

// percentOfBound returns watts as a percent of bound in the exponent scale,
// rounded down so the encoded limit never exceeds the request. A negative
// request, or one above the bound, is refused.
func percentOfBound(watts, bound Decimal, scale int16) (Decimal, error) {
	if watts.Exponent < -10 || watts.Exponent > 10 {
		return Decimal{}, controlRefusal(ControlReasonUnencodable, "watts exponent %d is outside -10 to 10", watts.Exponent)
	}
	if watts.Coefficient < 0 {
		return Decimal{}, controlRefusal(ControlReasonArgumentOutOfBound, "a negative limit is outside the bound")
	}
	ratio := new(big.Rat).Quo(decimalRat(watts), decimalRat(bound))
	if ratio.Cmp(big.NewRat(1, 1)) > 0 {
		return Decimal{}, controlRefusal(ControlReasonArgumentOutOfBound, "the limit exceeds the reported bound")
	}
	scaled := ratio.Mul(ratio, big.NewRat(100, 1))
	scaled.Mul(scaled, decimalRat(Decimal{Coefficient: 1, Exponent: -scale}))
	raw := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if !raw.IsInt64() {
		return Decimal{}, controlRefusal(ControlReasonUnencodable, "the percent does not fit the point")
	}
	return Decimal{Coefficient: raw.Int64(), Exponent: scale}, nil
}

func decimalRat(d Decimal) *big.Rat {
	r := new(big.Rat).SetInt64(d.Coefficient)
	ten := big.NewInt(10)
	exponent := int64(d.Exponent)
	factor := new(big.Int).Exp(ten, big.NewInt(abs64(exponent)), nil)
	if exponent >= 0 {
		return r.Mul(r, new(big.Rat).SetInt(factor))
	}
	return r.Quo(r, new(big.Rat).SetInt(factor))
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
