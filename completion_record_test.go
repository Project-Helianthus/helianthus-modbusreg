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

func TestFMV3M303CompletionGoldenJSONIsStableAndRoundTrips(t *testing.T) {
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

func TestFMV3M303CompletionRejectsIncompatibleOrUnsafeRecords(t *testing.T) {
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

func TestFMV3M303CompletionDispositionSetIsClosed(t *testing.T) {
	current, err := reg.NewCurrentFMV3M303CompletionRecord()
	if err != nil {
		t.Fatalf("NewCurrentFMV3M303CompletionRecord: %v", err)
	}
	overlayRequired := current.Spec()
	overlayRequired.Disposition = reg.CompletionDispositionOverlayRequired
	overlayRequired.OverlayPresent = true
	if _, err := reg.NewFMV3M303CompletionRecord(overlayRequired); err != nil {
		t.Fatalf("closed schema value OVERLAY_REQUIRED was rejected: %v", err)
	}
	overlayRequired.Disposition = reg.CompletionDisposition("OTHER")
	if _, err := reg.NewFMV3M303CompletionRecord(overlayRequired); err == nil {
		t.Fatal("unknown disposition was accepted")
	}
}

func TestFMV3M303CompletionUsesDefensiveCopies(t *testing.T) {
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

func TestFMV3M303CompletionAddsNoVendorOverlayDetectorOrTransportImport(t *testing.T) {
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
