package outback

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/internal/sunspecfixture"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

// These are synthetic geometry inputs from docs-modbus #145, not device
// observations or qualified register maps.
func TestOutBackAXSReadOnlyDecoderRejectsBothSourceRevisions(t *testing.T) {
	decoder, err := NewAXSReadOnlyDecoder()
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
			candidate := outBackAXSOccurrence(t, tc.id, tc.length, 2)
			words := candidate.Words()
			for i := 2; i < len(words); i++ {
				words[i] = uint16(i)
			}
			original := append([]uint16(nil), words...)
			for _, d := range []AXSReadOnlyDecoder{decoder, {}} {
				decoded, err := d.Decode(words)
				if !errors.Is(err, ErrOutBackAXSSourceNotQualified) {
					t.Fatalf("want source qualification rejection, got %v", err)
				}
				if !reflect.DeepEqual(decoded, sunspec.DecodedModel{}) {
					t.Fatalf("rejection emitted partial output: %#v", decoded)
				}
				if !reflect.DeepEqual(words, original) {
					t.Fatal("rejection changed caller raw evidence")
				}
			}
			standard := sunspecfixture.Registry(t)
			if slices.Contains(standard.DecoderKeys(), sunspec.DecoderKey{ModelID: tc.id, ModelLength: tc.length, SchemaRevision: sunspec.ModelsRevisionV1}) {
				t.Fatal("candidate admitted to standard registry")
			}
			if _, err := standard.DecodeOccurrence(candidate); err == nil {
				t.Fatal("standard registry decoded the candidate")
			}
		})
	}
}

func TestOutBackAXSReadOnlyDecoderFailsClosed(t *testing.T) {
	decoder, err := NewAXSReadOnlyDecoder()
	if err != nil {
		t.Fatal(err)
	}
	for _, words := range [][]uint16{nil, {64110}, {64110, 281}, {64110, 282}, {64110, 420}, {64112, 64}, {64111, 22}} {
		decoded, err := decoder.Decode(words)
		if err == nil || !reflect.DeepEqual(decoded, sunspec.DecodedModel{}) {
			t.Fatalf("accepted unsupported or malformed words: %#v", words)
		}
	}
}
