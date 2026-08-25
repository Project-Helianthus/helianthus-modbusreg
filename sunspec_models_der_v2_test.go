package modbusreg

import (
	"os"
	"strings"
	"testing"
)

func TestSunSpecV2DERDefinitionsAndApacheNotice(t *testing.T) {
	t.Run("Apache attribution", func(t *testing.T) {
		contents, err := os.ReadFile("THIRD_PARTY_NOTICES.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"SunSpec/models",
			"https://github.com/sunspec/models",
			"90b4a331dcca1d6eac69c1bead952fddcc5852e0",
			"Models 701/153 and 702/50",
			"modified by Helianthus",
			"Apache License",
			"Version 2.0, January 2004",
		} {
			if !strings.Contains(string(contents), want) {
				t.Fatalf("third-party notice lacks %q", want)
			}
		}
	})

	t.Run("exact V2 shapes", func(t *testing.T) {
		registry, err := NewStandardSunSpecDecoderRegistry(SunSpecModelsRevisionV2)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []struct {
			id, length uint16
			points     int
			first      []string
			last       string
		}{
			{701, 153, 72, []string{"ID", "L", "ACType"}, "MnAlrmInfo"},
			{702, 50, 51, []string{"ID", "L", "WMaxRtg"}, "S_SF"},
		} {
			definition, ok := registry.definition(SunSpecDecoderKey{ModelID: want.id, ModelLength: want.length, SchemaRevision: SunSpecModelsRevisionV2})
			if !ok || len(definition.points) != want.points {
				t.Fatalf("Model %d/%d definition=%v points=%d", want.id, want.length, ok, len(definition.points))
			}
			for index, name := range want.first {
				if definition.points[index].name != name {
					t.Fatalf("Model %d point %d=%q want=%q", want.id, index, definition.points[index].name, name)
				}
			}
			if definition.points[len(definition.points)-1].name != want.last {
				t.Fatalf("Model %d last=%q want=%q", want.id, definition.points[len(definition.points)-1].name, want.last)
			}
		}
	})
}
