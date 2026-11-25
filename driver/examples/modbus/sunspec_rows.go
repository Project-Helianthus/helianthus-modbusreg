package modbus

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"

	wire "github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

// SunSpecSelector identifies a point within one model occurrence. Word zero is the model ID.
type SunSpecSelector struct {
	Function              wire.FunctionCode
	ModelID               uint16
	Occurrence            uint32
	WordOffset, WordCount uint16
}

const (
	inverterRole = "inverter"
	powerField   = "inverter.ac.power.active"
	limitField   = "der.control.active_power_limit.percent"
	ratingField  = "der.settings.max_active_power"
	watt         = "unit.watt"
	ratio        = "unit.ratio"
	pvPolicy     = "pv.telemetry.fast.v1"
)

var (
	inverterKey = sunspec.DecoderKey{ModelID: 103, ModelLength: 50, SchemaRevision: sunspec.ModelsRevisionV1}
	settingsKey = sunspec.DecoderKey{ModelID: 121, ModelLength: 30, SchemaRevision: sunspec.ModelsRevisionV1}
	controlKey  = sunspec.DecoderKey{ModelID: 123, ModelLength: 24, SchemaRevision: sunspec.ModelsRevisionV1}
	pvPack      = semreg.PackRef{ID: "helianthus.pack.pv", Version: "1.0.0"}
)

func ref(pack semreg.PackRef, id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: pack, ID: id, Version: "1.0.0"}
}

func decimal(n int64, exp int32) semreg.Decimal {
	if n == 0 {
		return semreg.Decimal{Coefficient: "0"}
	}
	for exp < 18 && n%10 == 0 {
		n /= 10
		exp++
	}
	return semreg.Decimal{Coefficient: strconv.FormatInt(n, 10), Exponent10: exp}
}

func quantity(n semreg.Decimal, unit semreg.DefinitionID) semreg.Value {
	return semreg.Value{Kind: semreg.ValueQuantity, Quantity: &semreg.Quantity{Number: n, Unit: unit}}
}

func affine(num, den int64, exponent string) roweval.Convert {
	return roweval.Convert{Numeric: &roweval.Affine{Num: num, Den: den, ExponentRow: exponent}}
}

func integerCodec(signed bool) roweval.Codec {
	return roweval.Codec{
		Decode: func(raw uint64, width uint16) (semreg.Decimal, error) {
			n := int64(raw)
			if signed && raw&(1<<(width-1)) != 0 {
				n -= 1 << width
			}
			return decimal(n, 0), nil
		},
		Encode: func(d semreg.Decimal, width uint16) (uint64, error) {
			n := rational(d)
			if n != nil && n.IsInt() && n.Sign() >= 0 && n.Num().BitLen() <= int(width) {
				return n.Num().Uint64(), nil
			}
			return 0, fmt.Errorf("not an unsigned %d-bit integer", width)
		},
	}
}

func descriptors[S any]() roweval.Descriptors[S] {
	identity := func(n uint64, _ uint16) (uint64, error) { return n, nil }
	bigEndian := func(n uint64, width uint16) (uint64, error) {
		var out uint64
		for i := uint16(0); i < width; i += 8 {
			out = out<<8 | n&0xff
			n >>= 8
		}
		return out, nil
	}
	return roweval.Descriptors[S]{
		Codecs:     map[string]roweval.Codec{"uint": integerCodec(false), "int": integerCodec(true)},
		ByteOrders: map[string]roweval.Order{"big": {Decode: bigEndian, Encode: bigEndian}},
		WordOrders: map[string]roweval.Order{"one": {Decode: identity, Encode: identity}},
		BitOrders:  map[string]roweval.Order{"lsb": {Decode: identity, Encode: identity}},
	}
}

func layout(offset, width uint16, codec string) roweval.Layout {
	return roweval.Layout{BitOffset: offset * 8, Width: width * 8, Codec: codec, ByteOrder: "big", WordOrder: "one", BitOrder: "lsb"}
}

type sunSpecRows struct {
	key        sunspec.DecoderKey
	first, end uint16
	table      roweval.Table[SunSpecSelector]
}

// makeSunSpecRows keeps the selector in block coordinates, but supplies only
// the contiguous point/scale slice to the evaluator. The inverse of the
// first point is thus one word, not a zero-padded model write.
func makeSunSpecRows(r sunspec.DecoderRegistry, key sunspec.DecoderKey, ordinal uint32) (sunSpecRows, error) {
	var field string
	switch key {
	case inverterKey:
		field = powerField
	case controlKey:
		field = limitField
	default:
		return sunSpecRows{}, fmt.Errorf("no exemplar rows for key %+v", key)
	}
	point, sf, ok := r.FixedPointLayout(key, field)
	if !ok || sf.Type != sunspec.TypeScaleFactor {
		return sunSpecRows{}, fmt.Errorf("missing fixed layout for %s and its scale", field)
	}
	first, end := point.WordOffset, sf.WordOffset+sf.WordWidth
	sel := func(p sunspec.FixedPoint) SunSpecSelector {
		return SunSpecSelector{wire.FunctionReadHoldingRegisters, key.ModelID, ordinal, p.WordOffset, p.WordWidth}
	}
	lo, hi := decimal(-10, 0), decimal(10, 0)
	valueWire := layout(0, point.WordWidth*2, "int")
	valueWire.Sentinels = map[uint64]string{0x8000: string(sunspec.ValueNotImplemented)}
	if point.Type == sunspec.TypeUint16 {
		valueWire.Codec = "uint"
		valueWire.Sentinels = map[uint64]string{0xffff: string(sunspec.ValueNotImplemented)}
	}
	scaleWire := layout((sf.WordOffset-first)*2, sf.WordWidth*2, "int")
	scaleWire.Sentinels = map[uint64]string{0x8000: string(sunspec.ValueNotImplemented)}
	native := []roweval.NativeRow[SunSpecSelector]{
		{ID: point.FieldID, Unit: point.Unit, SpecRef: point.Name, At: sel(point), Wire: valueWire, Decode: affine(1, 1, sf.FieldID)},
		{ID: sf.FieldID, Unit: sf.Unit, SpecRef: sf.Name, At: sel(sf), Wire: scaleWire, Domain: roweval.Bounds{Min: &lo, Max: &hi}},
	}
	binding := roweval.BindingRow{NativeID: point.FieldID, SubjectRole: inverterRole, SpecRef: point.Name,
		Service: ref(pvPack, "pv.service.inverter"), Capability: ref(pvPack, "pv.capability.read.inverter"),
		Read: &roweval.ReadRow{Fact: ref(pvPack, "pv.ac.aggregate_active_power"), Unit: watt, Policy: pvPolicy, Dimensions: map[semreg.DefinitionID]string{"pv.dimension.inverter": "inverter-resource"}}}
	out := sunSpecRows{key: key, first: first, end: end, table: roweval.Table[SunSpecSelector]{Native: native, Bindings: []roweval.BindingRow{binding}}}
	if key == controlKey {
		rating, ratingSF, present := r.FixedPointLayout(settingsKey, ratingField)
		if !present || ratingSF.Type != sunspec.TypeScaleFactor {
			return sunSpecRows{}, fmt.Errorf("missing fixed rating layout")
		}
		b := &out.table.Bindings[0]
		b.Capability = ref(pvPack, "pv.capability.set_active_power_limit")
		b.Read.Fact = ref(pvPack, "pv.limit.active_power_ratio")
		b.Read.Unit = ratio
		b.Read.ToRegistry = affine(1, 100, "")
		b.Write = &roweval.WriteRow{Operation: ref(pvPack, "pv.operation.set_active_power_limit"), Argument: "pv.limit.active_power", TargetRow: point.FieldID, ReadbackRow: point.FieldID, Inverse: roweval.Convert{Hook: "watts-to-limit-word"}}
		out.table.Hooks = map[string]roweval.RowHook{"watts-to-limit-word": limitWordHook(rating, ratingSF, sf, point.Unit)}
	}
	return out, nil
}

func (rows sunSpecRows) evaluate(r sunspec.DecoderRegistry, occurrence sunspec.Occurrence, a roweval.Acquired[SunSpecSelector]) (roweval.Evaluation, error) {
	a.Descriptors = descriptors[SunSpecSelector]()
	a.Payload = nil
	if a.Selector.ModelID != occurrence.WireKey.ModelID || a.Selector.Occurrence != occurrence.Ordinal || occurrence.WireKey.ModelLength != rows.key.ModelLength {
		return roweval.Evaluation{Reason: roweval.ReasonNoMatch}, nil
	}
	for _, row := range rows.table.Native {
		if a.Selector != row.At {
			continue
		}
		if _, err := r.DecodeOccurrence(occurrence); err != nil {
			return roweval.Evaluation{}, err
		}
		a.Payload = wordBytes(occurrence.Words()[rows.first:rows.end])
		break
	}
	return roweval.Evaluate(rows.table, a)
}

func wordBytes(words []uint16) []byte {
	payload := make([]byte, 0, len(words)*2)
	for _, word := range words {
		payload = binary.BigEndian.AppendUint16(payload, word)
	}
	return payload
}

func rational(d semreg.Decimal) *big.Rat {
	if d.Validate() != nil {
		return nil
	}
	n, _ := new(big.Int).SetString(d.Coefficient, 10)
	return new(big.Rat).Mul(new(big.Rat).SetInt(n), power10(int64(d.Exponent10)))
}

func power10(exp int64) *big.Rat {
	magnitude := exp
	if magnitude < 0 {
		magnitude = -magnitude
	}
	r := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(magnitude), nil))
	if exp < 0 {
		r.Inv(r)
	}
	return r
}

func detachedWord(payload []byte, key sunspec.DecoderKey, p sunspec.FixedPoint) (uint16, bool) {
	if len(payload) != 2*(int(key.ModelLength)+2) || binary.BigEndian.Uint16(payload) != key.ModelID || binary.BigEndian.Uint16(payload[2:]) != key.ModelLength {
		return 0, false
	}
	return binary.BigEndian.Uint16(payload[2*int(p.WordOffset):]), true
}

func validScale(word uint16) bool { return int16(word) >= -10 && int16(word) <= 10 }

func limitWordHook(rating, ratingSF, limitSF sunspec.FixedPoint, unit string) roweval.RowHook {
	return func(in roweval.DetachedInputs, _ roweval.RowID, _ roweval.Direction, value roweval.TypedValue) (roweval.DerivedValue, roweval.Reason) {
		var max *big.Rat
		var scale uint16
		var haveScale bool
		for _, p := range in.Inputs() {
			if raw, ok := detachedWord(p.Payload, settingsKey, rating); ok {
				sf, _ := detachedWord(p.Payload, settingsKey, ratingSF)
				if raw != 0 && raw != 0xffff && validScale(sf) {
					max = new(big.Rat).Mul(big.NewRat(int64(raw), 1), power10(int64(int16(sf))))
				}
			}
			if raw, ok := detachedWord(p.Payload, controlKey, limitSF); ok {
				scale, haveScale = raw, true
			}
		}
		if max == nil {
			return roweval.DerivedValue{}, "invalid_wmax"
		}
		if !haveScale || !validScale(scale) {
			return roweval.DerivedValue{}, "invalid_limit_scale"
		}
		// Invert checks the published watt limits before calling the hook.
		// The evaluator tags the hook operand with the target native unit.
		if value.Quantity == nil {
			return roweval.DerivedValue{}, roweval.ReasonArgument
		}
		requested := rational(value.Quantity.Number)
		if requested == nil || requested.Sign() < 0 || requested.Cmp(max) > 0 {
			return roweval.DerivedValue{}, roweval.ReasonArgument
		}
		raw := new(big.Rat).Quo(requested, max)
		raw.Mul(raw, big.NewRat(100, 1)).Quo(raw, power10(int64(int16(scale))))
		word := new(big.Int).Quo(raw.Num(), raw.Denom())
		if !word.IsUint64() || word.Uint64() >= 0xffff {
			return roweval.DerivedValue{}, "invalid_limit_word"
		}
		return roweval.DerivedValue{Value: quantity(decimal(word.Int64(), 0), semreg.DefinitionID(unit))}, ""
	}
}
