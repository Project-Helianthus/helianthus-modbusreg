package modbusreg

import (
	"reflect"
	"testing"
)

func TestSunSpecBitfieldLowWordRegistryDecode(t *testing.T) {
	registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV1)
	if err != nil {
		t.Fatal(err)
	}
	key := SunSpecDecoderKey{ModelID: 103, ModelLength: 50, SchemaRevision: SunSpecModelsRevisionV1}
	definition, ok := registry.definition(key)
	if !ok {
		t.Fatal("standard inverter definition absent")
	}
	var event sunSpecPointDefinition
	for _, point := range definition.points {
		if point.name == "Evt1" {
			event = point
		}
	}
	if event.fieldID != "inverter.events.1" || event.pointType != SunSpecTypeBitfield32 || event.size != 2 {
		t.Fatalf("Evt1 is not the declared inverter bitfield32: %+v", event)
	}
	allSymbols := []string{
		"GROUND_FAULT", "DC_OVER_VOLT", "AC_DISCONNECT", "DC_DISCONNECT",
		"GRID_DISCONNECT", "CABINET_OPEN", "MANUAL_SHUTDOWN", "OVER_TEMP",
		"OVER_FREQUENCY", "UNDER_FREQUENCY", "AC_OVER_VOLT", "AC_UNDER_VOLT",
		"BLOWN_STRING_FUSE", "UNDER_TEMP", "MEMORY_LOSS", "HW_TEST_FAILURE",
	}
	for _, tc := range []struct {
		name    string
		words   [2]uint16
		state   SunSpecValueState
		bits    uint64
		unknown uint64
		symbols []string
	}{
		{"zero_high_all_ones_low", [2]uint16{0x0000, 0xffff}, SunSpecValueValid, 0x0000ffff, 0, allSymbols},
		{"different_high_same_low", [2]uint16{0x0001, 0xffff}, SunSpecValueValid, 0x0001ffff, 0x00010000, allSymbols},
		{"same_high_different_low", [2]uint16{0x0000, 0xfffe}, SunSpecValueValid, 0x0000fffe, 0, allSymbols[1:]},
		{"all_ones_high_valid_low", [2]uint16{0xffff, 0xfffe}, SunSpecValueValid, 0xfffffffe, 0xffff0000, allSymbols[1:]},
		{"full_sentinel", [2]uint16{0xffff, 0xffff}, SunSpecValueNotImplemented, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := make([]uint16, int(key.ModelLength)+2)
			words[0], words[1] = key.ModelID, key.ModelLength
			copy(words[event.offset:event.offset+event.size], tc.words[:])
			wantRaw := append([]uint16(nil), words...)
			wantSpans := []SunSpecSourceSpan{{LogicalViewID: 7, PDUOffset: 2, WordCount: uint16(len(words))}}
			occurrence := SunSpecOccurrence{
				Ordinal: 3, WireKey: SunSpecWireKey{ModelID: key.ModelID, ModelLength: key.ModelLength},
				SchemaRevision: key.SchemaRevision, Disposition: SunSpecChainDispositionAdmitted,
				decoderKey: &key, words: words, spans: append([]SunSpecSourceSpan(nil), wantSpans...),
			}
			decoded, err := registry.DecodeOccurrence(occurrence)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Key() != key || decoded.Ordinal() != 3 || !decoded.GeometryValid() {
				t.Fatalf("decoded provenance: key=%+v ordinal=%d geometry=%v", decoded.Key(), decoded.Ordinal(), decoded.GeometryValid())
			}
			if !reflect.DeepEqual(decoded.RawWords(), wantRaw) || !reflect.DeepEqual(decoded.SourceSpans(), wantSpans) {
				t.Fatal("decoded model lost native words or source spans")
			}
			fact, ok := decoded.Fact("inverter.events.1")
			if !ok || fact.PointName != "Evt1" {
				t.Fatal("declared inverter event fact absent")
			}
			value := fact.Value
			if value.PointType() != SunSpecTypeBitfield32 || value.State() != tc.state || !reflect.DeepEqual(value.RawWords(), tc.words[:]) {
				t.Fatalf("type=%s state=%s raw=%x; want bitfield32 state=%s raw=%x", value.PointType(), value.State(), value.RawWords(), tc.state, tc.words)
			}
			bits, unknown, present := value.Bitfield()
			if bits != tc.bits || unknown != tc.unknown || present != (tc.state == SunSpecValueValid) {
				t.Errorf("Bitfield()=(%#x,%#x,%v); want (%#x,%#x,%v)", bits, unknown, present, tc.bits, tc.unknown, tc.state == SunSpecValueValid)
			}
			if !reflect.DeepEqual(value.BitfieldSymbols(), tc.symbols) {
				t.Errorf("symbols=%v; want %v", value.BitfieldSymbols(), tc.symbols)
			}
			if tc.state == SunSpecValueNotImplemented {
				_, signed := value.Signed()
				_, unsigned := value.Unsigned()
				_, decimal := value.Decimal()
				_, unsignedDecimal := value.UnsignedDecimal()
				_, floating := value.Float32()
				_, text := value.Text()
				_, _, enum := value.Enum()
				if signed || unsigned || decimal || unsignedDecimal || floating || text || enum {
					t.Fatal("not-implemented sentinel exposed a typed value")
				}
			}
		})
	}
}
