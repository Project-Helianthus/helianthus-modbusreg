package modbusreg_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestM303CompletionGoldenJSONIsStableAndRoundTrips(t *testing.T) {
	golden, err := os.ReadFile("testdata/fmv3-m3-03-completion-v2.json")
	if err != nil {
		t.Fatalf("ReadFile(golden): %v", err)
	}
	record, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("NewCurrentFMV3M303CompletionRecord: %v", err)
	}
	encoded, err := reg.MarshalFMV3M303CompletionRecord(record)
	if err != nil {
		t.Fatalf("MarshalFMV3M303CompletionRecord: %v", err)
	}
	if !bytes.Equal(encoded, bytes.TrimSpace(golden)) {
		t.Fatalf("canonical JSON changed\n got: %s\nwant: %s", encoded, golden)
	}
	decoded, err := reg.UnmarshalFMV3M303CompletionRecord(golden)
	if err != nil {
		t.Fatalf("UnmarshalFMV3M303CompletionRecord(golden): %v", err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("json.Marshal(round trip): %v", err)
	}
	if !bytes.Equal(roundTrip, encoded) {
		t.Fatalf("round trip changed JSON\n got: %s\nwant: %s", roundTrip, encoded)
	}
	if decoded.Disposition() != reg.CompletionDispositionStandardOnly || decoded.OverlayPresent() ||
		!decoded.ReadOnly() || !decoded.TransportNeutral() || decoded.WriteCapable() ||
		decoded.AutomaticProductQualification() {
		t.Fatalf("canonical decision leaked capability: %#v", decoded.Spec())
	}
}

func TestM303CompletionRejectsIncompatibleOrUnsafeRecords(t *testing.T) {
	golden, err := os.ReadFile("testdata/fmv3-m3-03-completion-v2.json")
	if err != nil {
		t.Fatalf("ReadFile(golden): %v", err)
	}
	mutations := []struct {
		name string
		from string
		to   string
	}{
		{"wrong schema", `"schema_id":"helianthus.fmv3-m3-03-completion.v2"`, `"schema_id":"other"`},
		{"wrong version", `"schema_version":2`, `"schema_version":3`},
		{"unknown disposition", `"disposition":"STANDARD_ONLY"`, `"disposition":"UNKNOWN"`},
		{"standard record has overlay", `"overlay_present":false`, `"overlay_present":true`},
		{"transport dependent", `"transport_neutral":true`, `"transport_neutral":false`},
		{"write capable", `"write_capable":false`, `"write_capable":true`},
		{"automatic qualification", `"automatic_product_qualification":false`, `"automatic_product_qualification":true`},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := bytes.Replace(golden, []byte(mutation.from), []byte(mutation.to), 1)
			if bytes.Equal(candidate, golden) {
				t.Fatal("test mutation did not apply")
			}
			if _, err := reg.UnmarshalFMV3M303CompletionRecord(candidate); err == nil {
				t.Fatal("unsafe completion record was accepted")
			}
		})
	}
}

func TestM303CompletionConstructorRejectsNonCanonicalConclusion(t *testing.T) {
	current, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("NewCurrentFMV3M303CompletionRecord: %v", err)
	}
	mutations := []struct {
		name   string
		mutate func(*reg.CompletionRecordSpec)
	}{
		{"docs evidence SHA", func(spec *reg.CompletionRecordSpec) {
			spec.Evidence.DocsEvidenceSHA = strings.Repeat("a", 40)
		}},
		{"M3-02 merge SHA", func(spec *reg.CompletionRecordSpec) {
			spec.Evidence.M302MergeSHA = strings.Repeat("b", 40)
		}},
		{"official models SHA", func(spec *reg.CompletionRecordSpec) {
			spec.Evidence.OfficialModelsSHA = strings.Repeat("c", 40)
		}},
		{"standard profile ID", func(spec *reg.CompletionRecordSpec) {
			spec.StandardProfileID = "sunspec.other"
		}},
		{"standard profile version", func(spec *reg.CompletionRecordSpec) {
			spec.StandardProfileVersion = "1.0.1"
		}},
		{"applicability entry", func(spec *reg.CompletionRecordSpec) {
			spec.Applicability[0] = "qualified documentary boundary"
		}},
		{"applicability missing", func(spec *reg.CompletionRecordSpec) {
			spec.Applicability = []string{}
		}},
		{"applicability extra", func(spec *reg.CompletionRecordSpec) {
			spec.Applicability = append(spec.Applicability, "extra applicability")
		}},
		{"limitations missing", func(spec *reg.CompletionRecordSpec) {
			spec.Limitations = append([]string(nil), spec.Limitations[:len(spec.Limitations)-1]...)
		}},
		{"limitations extra", func(spec *reg.CompletionRecordSpec) {
			spec.Limitations = append(spec.Limitations, "extra product: UNKNOWN")
		}},
		{"limitations reordered", func(spec *reg.CompletionRecordSpec) {
			spec.Limitations[0], spec.Limitations[1] = spec.Limitations[1], spec.Limitations[0]
		}},
		{"rollback missing", func(spec *reg.CompletionRecordSpec) {
			spec.InvalidationRollback = append([]string(nil), spec.InvalidationRollback[:len(spec.InvalidationRollback)-1]...)
		}},
		{"rollback extra", func(spec *reg.CompletionRecordSpec) {
			spec.InvalidationRollback = append(spec.InvalidationRollback, "extra rollback")
		}},
		{"rollback reordered", func(spec *reg.CompletionRecordSpec) {
			spec.InvalidationRollback[0], spec.InvalidationRollback[1] = spec.InvalidationRollback[1], spec.InvalidationRollback[0]
		}},
	}
	for index := range current.Limitations() {
		index := index
		mutations = append(mutations, struct {
			name   string
			mutate func(*reg.CompletionRecordSpec)
		}{
			name: "limitation entry " + current.Limitations()[index],
			mutate: func(spec *reg.CompletionRecordSpec) {
				spec.Limitations[index] = "mutated product: UNKNOWN"
			},
		})
	}
	for index := range current.InvalidationRollback() {
		index := index
		mutations = append(mutations, struct {
			name   string
			mutate func(*reg.CompletionRecordSpec)
		}{
			name: "rollback entry " + current.InvalidationRollback()[index],
			mutate: func(spec *reg.CompletionRecordSpec) {
				spec.InvalidationRollback[index] = "mutated rollback"
			},
		})
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := current.Spec()
			mutation.mutate(&candidate)
			if _, err := reg.NewFMV3M303CompletionRecord(candidate); err == nil {
				t.Fatal("constructor accepted non-canonical completion conclusion")
			}
		})
	}
}

func TestM303CompletionJSONRejectsNonCanonicalConclusion(t *testing.T) {
	golden, err := os.ReadFile("testdata/fmv3-m3-03-completion-v2.json")
	if err != nil {
		t.Fatalf("ReadFile(golden): %v", err)
	}
	applicability := "qualified documentary GEN24 Primo/Symo ROW int+SF boundary requires runtime chain discovery"
	limitations := []string{
		"Verto: UNKNOWN",
		"Tauro: UNKNOWN",
		"older Datamanager: UNKNOWN",
		"SnapINverter: UNKNOWN",
		"live installations: UNKNOWN",
	}
	rollback := []string{
		"evidence change invalidates decision",
		"retain standard SunSpec/raw access",
		"no automatic side effect",
	}
	type replacement struct{ from, to string }
	mutations := []struct {
		name         string
		replacements []replacement
	}{
		{"docs evidence SHA", []replacement{{"59218d21163acb868687ed3d8196f0aa1496aab7", strings.Repeat("a", 40)}}},
		{"M3-02 merge SHA", []replacement{{"867c8275c090d3c703a9638548b48ea6846e8c56", strings.Repeat("b", 40)}}},
		{"official models SHA", []replacement{{"7abdf8982d5364f8ae916deee18aac86c11be36d", strings.Repeat("c", 40)}}},
		{"standard profile ID", []replacement{{`"standard_profile_id":"sunspec.phase1"`, `"standard_profile_id":"sunspec.other"`}}},
		{"standard profile version", []replacement{{`"standard_profile_version":"1.0.0"`, `"standard_profile_version":"1.0.1"`}}},
		{"applicability entry", []replacement{{applicability, "qualified documentary boundary"}}},
		{"applicability missing", []replacement{{`"applicability":["` + applicability + `"]`, `"applicability":[]`}}},
		{"applicability extra", []replacement{{`"applicability":["` + applicability + `"]`, `"applicability":["` + applicability + `","extra applicability"]`}}},
		{"limitations missing", []replacement{{`,"` + limitations[4] + `"`, ""}}},
		{"limitations extra", []replacement{{`"` + limitations[4] + `"]`, `"` + limitations[4] + `","extra product: UNKNOWN"]`}}},
		{"limitations reordered", []replacement{{`"` + limitations[0] + `","` + limitations[1] + `"`, `"` + limitations[1] + `","` + limitations[0] + `"`}}},
		{"rollback missing", []replacement{{`,"` + rollback[2] + `"`, ""}}},
		{"rollback extra", []replacement{{`"` + rollback[2] + `"]`, `"` + rollback[2] + `","extra rollback"]`}}},
		{"rollback reordered", []replacement{{`"` + rollback[0] + `","` + rollback[1] + `"`, `"` + rollback[1] + `","` + rollback[0] + `"`}}},
	}
	for _, value := range limitations {
		mutations = append(mutations, struct {
			name         string
			replacements []replacement
		}{"limitation entry " + value, []replacement{{value, "mutated product: UNKNOWN"}}})
	}
	for _, value := range rollback {
		mutations = append(mutations, struct {
			name         string
			replacements []replacement
		}{"rollback entry " + value, []replacement{{value, "mutated rollback"}}})
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := append([]byte(nil), golden...)
			for _, replacement := range mutation.replacements {
				updated := bytes.Replace(candidate, []byte(replacement.from), []byte(replacement.to), 1)
				if bytes.Equal(updated, candidate) {
					t.Fatalf("JSON mutation did not apply: %q", replacement.from)
				}
				candidate = updated
			}
			if _, err := reg.UnmarshalFMV3M303CompletionRecord(candidate); err == nil {
				t.Fatal("JSON unmarshal accepted non-canonical completion conclusion")
			}
		})
	}
}

func TestM303CompletionDispositionSetIsClosed(t *testing.T) {
	current, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("NewCurrentFMV3M303CompletionRecord: %v", err)
	}
	if current.Disposition() != reg.CompletionDispositionStandardOnly {
		t.Fatalf("canonical disposition = %q", current.Disposition())
	}
	overlayRequired := current.Spec()
	overlayRequired.Disposition = reg.CompletionDispositionOverlayRequired
	overlayRequired.OverlayPresent = true
	t.Run("constructor", func(t *testing.T) {
		if _, err := reg.NewFMV3M303CompletionRecord(overlayRequired); err == nil {
			t.Fatal("current evidence contract represented OVERLAY_REQUIRED")
		}
	})
	unknown := overlayRequired
	unknown.Disposition = reg.CompletionDisposition("OTHER")
	if _, err := reg.NewFMV3M303CompletionRecord(unknown); err == nil {
		t.Fatal("unknown disposition was accepted")
	}

	golden, err := os.ReadFile("testdata/fmv3-m3-03-completion-v2.json")
	if err != nil {
		t.Fatalf("ReadFile(golden): %v", err)
	}
	fabricated := bytes.Replace(
		golden,
		[]byte(`"disposition":"STANDARD_ONLY"`),
		[]byte(`"disposition":"OVERLAY_REQUIRED"`),
		1,
	)
	fabricated = bytes.Replace(
		fabricated,
		[]byte(`"overlay_present":false`),
		[]byte(`"overlay_present":true`),
		1,
	)
	if bytes.Equal(fabricated, golden) {
		t.Fatal("OVERLAY_REQUIRED JSON mutation did not apply")
	}
	t.Run("JSON unmarshal", func(t *testing.T) {
		if _, err := reg.UnmarshalFMV3M303CompletionRecord(fabricated); err == nil {
			t.Fatal("current evidence contract unmarshaled fabricated OVERLAY_REQUIRED")
		}
	})
}

func TestM303CompletionUsesDefensiveCopies(t *testing.T) {
	record, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("NewCurrentFMV3M303CompletionRecord: %v", err)
	}
	first := record.Applicability()
	first[0] = "mutated"
	if got := record.Applicability()[0]; got == "mutated" {
		t.Fatal("applicability getter exposed mutable storage")
	}
	spec := record.Spec()
	spec.Limitations[0] = "mutated"
	if got := record.Limitations()[0]; got == "mutated" {
		t.Fatal("Spec exposed mutable storage")
	}
}

func TestM303CompletionAddsNoVendorOverlayDetectorOrTransportImport(t *testing.T) {
	for _, name := range []string{"completion_record.go", "completion_record_serialization.go"} {
		path := filepath.Join(".", name)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(source)), "fronius") ||
			strings.Contains(string(source), "Detector") || strings.Contains(string(source), "OverlayProfile") {
			t.Fatalf("%s contains a vendor overlay or detector symbol", name)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		for _, imported := range parsed.Imports {
			if strings.Contains(imported.Path.Value, "transport") {
				t.Fatalf("%s imports a transport package: %s", name, imported.Path.Value)
			}
		}
	}
	for _, value := range []any{reg.CompletionRecord{}, reg.CompletionRecordSpec{}} {
		typ := reflect.TypeOf(value)
		for method := 0; method < typ.NumMethod(); method++ {
			if name := typ.Method(method).Name; name == "Write" || name == "Control" || name == "Qualify" {
				t.Fatalf("completion API exposes a capability method: %s", name)
			}
		}
	}
}
