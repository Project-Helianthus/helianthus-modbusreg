package modbusreg

import (
	"errors"
	"reflect"
	"testing"
)

// These are synthetic geometry inputs from docs-modbus #145, not device
// observations or qualified register maps.
func TestOutBackAXSReadOnlyDecoderRejectsBothSourceRevisions(t *testing.T) {
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		id, length uint16
	}{
		{"owner manual rev C interface", 64110, 282},
		{"application note rev 5 interface", 64110, 420},
		{"owner manual rev C opaque charge", 64111, 23},
		{"application note rev 5 opaque charge", 64111, 26},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := outBackAXSOccurrence(tc.id, tc.length, 2).Words()
			for i := 2; i < len(words); i++ {
				words[i] = uint16(i)
			}
			original := append([]uint16(nil), words...)
			for _, d := range []OutBackAXSReadOnlyDecoder{decoder, {}} {
				decoded, err := d.Decode(words)
				if !errors.Is(err, ErrOutBackAXSSourceNotQualified) {
					t.Fatalf("want source qualification rejection, got %v", err)
				}
				if !reflect.DeepEqual(decoded, SunSpecDecodedModel{}) {
					t.Fatalf("rejection emitted partial output: %#v", decoded)
				}
				if !reflect.DeepEqual(words, original) {
					t.Fatal("rejection changed caller raw evidence")
				}
			}
			standard := mustStandardSunSpecRegistry(t)
			if _, ok := standard.definition(SunSpecDecoderKey{ModelID: tc.id, ModelLength: tc.length, SchemaRevision: SunSpecModelsRevisionV1}); ok {
				t.Fatal("candidate admitted to standard registry")
			}
		})
	}
}

func TestOutBackAXSReadOnlyDecoderFailsClosed(t *testing.T) {
	decoder, err := NewOutBackAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, words := range [][]uint16{nil, {64110}, {64110, 281}, {64110, 282}, {64110, 420}, {64112, 64}, {64111, 22}} {
		decoded, err := decoder.Decode(words)
		if err == nil || !reflect.DeepEqual(decoded, SunSpecDecodedModel{}) {
			t.Fatalf("accepted unsupported or malformed words: %#v", words)
		}
	}
}
