package modbusreg_test

import (
	"encoding/json"
	"errors"
	"github.com/Project-Helianthus/helianthus-modbusreg/growatt"
	"testing"
)

func recordValidationJSON() map[string]any {
	return map[string]any{"family": "MAX", "device_type": 17, "model_build_word_82": 23, "model_build_word_83": 41, "reported_protocol_word": 256, "source_schema_ref": "synthetic:source:edition-a:amendment-a", "function": 4, "offset": 0, "width_words": 59, "field_schema_ref": "synthetic:fields:a", "tuple_evidence_ref": "synthetic:tuple:a"}
}
func recordValidationBytes(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func recordValidationOperands(t *testing.T) (growatt.FC04Record, growatt.ProtocolIIIdentityObservation, growatt.FC04SymbolicSchema) {
	t.Helper()
	r, e := growatt.ParseGrowattFC04Record(recordValidationBytes(t, recordValidationJSON()))
	if e != nil {
		t.Fatal(e)
	}
	in := publicGrowattProtocolIIIdentityInput()
	in.Profile.DeviceType = 17
	in.Profile.ModelBuild = [2]uint16{23, 41}
	in.Profile.ProtocolVersion = 256
	in.Slices[2].Words = []uint16{17}
	in.Slices[3].Words = []uint16{23, 41}
	in.Slices[4].Words = []uint16{256}
	i, e := growatt.DecodeGrowattProtocolIIIdentity(in)
	if e != nil {
		t.Fatal(e)
	}
	s := growatt.FC04SymbolicSchema{SourceSchemaRef: r.SourceSchemaRef, FieldSchemaRef: r.FieldSchemaRef, DocumentEdition: "1.24", AmendmentIdentity: "synthetic:amendment:a", Function: 4, Offset: 0, WidthWords: 59, FieldExtents: []growatt.FC04FieldExtent{{Offset: 0, WidthWords: 1}, {Offset: 57, WidthWords: 2}}}
	return r, i, s
}
func TestGrowattFc04RecordValidationParsing(t *testing.T) {
	for key := range recordValidationJSON() {
		t.Run("missing/"+key, func(t *testing.T) {
			m := recordValidationJSON()
			delete(m, key)
			if _, e := growatt.ParseGrowattFC04Record(recordValidationBytes(t, m)); !errors.Is(e, growatt.ErrGrowattFC04Missing) {
				t.Fatalf("missing: %v", e)
			}
		})
		for _, bad := range []any{nil, true, []any{}, map[string]any{}} {
			t.Run("type/"+key, func(t *testing.T) {
				m := recordValidationJSON()
				m[key] = bad
				if _, e := growatt.ParseGrowattFC04Record(recordValidationBytes(t, m)); !errors.Is(e, growatt.ErrGrowattFC04Invalid) {
					t.Fatalf("type: %v", e)
				}
			})
		}
	}
	for _, raw := range []string{`null`, `[]`, `{} {}`, `{"family":"MAX","family":"MAX"}`, `{"extra":1}`, `{"family":4}`} {
		if _, e := growatt.ParseGrowattFC04Record([]byte(raw)); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for key, values := range map[string][]any{"family": {"OTHER", ""}, "device_type": {-1, 65536, 1.5, "17"}, "model_build_word_82": {-1, 65536}, "model_build_word_83": {-1, 65536}, "reported_protocol_word": {"1.24", 1.24, -1, 65536}, "function": {3, 260}, "offset": {1, -1, 65536}, "width_words": {0, 58, 60}, "source_schema_ref": {"", " spaced "}, "field_schema_ref": {""}, "tuple_evidence_ref": {""}} {
		for _, bad := range values {
			m := recordValidationJSON()
			m[key] = bad
			if _, e := growatt.ParseGrowattFC04Record(recordValidationBytes(t, m)); e == nil {
				t.Fatalf("accepted %s=%v", key, bad)
			}
		}
	}
}
func TestGrowattFc04RecordValidationComparisons(t *testing.T) {
	r, i, s := recordValidationOperands(t)
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s}); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*growatt.FC04Record){
		"family": func(x *growatt.FC04Record) { x.Family = "MID" }, "device_type": func(x *growatt.FC04Record) { x.DeviceType++ }, "model_word_82": func(x *growatt.FC04Record) { x.ModelBuildWord82++ }, "model_word_83": func(x *growatt.FC04Record) { x.ModelBuildWord83++ }, "protocol_word": func(x *growatt.FC04Record) { x.ReportedProtocolWord++ }, "source_schema": func(x *growatt.FC04Record) { x.SourceSchemaRef = "synthetic:source:b" }, "function": func(x *growatt.FC04Record) { x.Function = 3 }, "offset": func(x *growatt.FC04Record) { x.Offset = 1 }, "width": func(x *growatt.FC04Record) { x.WidthWords = 58 }, "field_schema": func(x *growatt.FC04Record) { x.FieldSchemaRef = "synthetic:fields:b" }, "tuple_evidence": func(x *growatt.FC04Record) { x.TupleEvidenceRef = "synthetic:tuple:b" },
	} {
		t.Run(name, func(t *testing.T) {
			other := r
			change(&other)
			if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{other}, []growatt.FC04SymbolicSchema{s}); e == nil {
				t.Fatal("omitted equality")
			}
		})
	}
	for _, side := range []string{"identity", "schema"} {
		t.Run(side, func(t *testing.T) {
			if side == "identity" {
				i = growatt.ProtocolIIIdentityObservation{}
			} else {
				s.FieldExtents = []growatt.FC04FieldExtent{{Offset: 58, WidthWords: 2}}
			}
			if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s}); e == nil {
				t.Fatal("invalid operand")
			}
		})
		_, i, s = recordValidationOperands(t)
	}
	if e := growatt.CompareGrowattFC04Record(r, i, nil, []growatt.FC04SymbolicSchema{s}); !errors.Is(e, growatt.ErrGrowattFC04Missing) {
		t.Fatal(e)
	}
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r, r}, []growatt.FC04SymbolicSchema{s}); !errors.Is(e, growatt.ErrGrowattFC04Ambiguous) {
		t.Fatal(e)
	}
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, nil); !errors.Is(e, growatt.ErrGrowattFC04Missing) {
		t.Fatal(e)
	}
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s, s}); !errors.Is(e, growatt.ErrGrowattFC04Ambiguous) {
		t.Fatal(e)
	}
}
func TestGrowattFc04RecordValidationExternalAdmissionDenied(t *testing.T) {
	r, i, s := recordValidationOperands(t)
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s}); e != nil {
		t.Fatal(e)
	}
	status, e := growatt.ValidateGrowattFC04RecordAdmission(recordValidationBytes(t, recordValidationJSON()), i)
	if status != "NO_ADMISSIBLE_PROFILE" || !errors.Is(e, growatt.ErrGrowattFC04Missing) {
		t.Fatalf("admitted: %s %v", status, e)
	}
	if _, e := growatt.DecodeGrowattProtocolIIFC04Telemetry(i, growatt.ProtocolIIFC04Applicability{}, growatt.ProtocolIIFC04Slice{Words: make([]uint16, 59)}); e == nil {
		t.Fatal("typed telemetry admitted")
	}
	if o, e := growatt.NewProtocolIIFC04RTUObserver(i, growatt.ProtocolIIFC04Applicability{}, publicGrowattProtocolIIFC04Session{}); e == nil || o != nil {
		t.Fatal("observer admitted")
	}
}

func TestGrowattFc04RecordValidationIdentityJoins(t *testing.T) {
	r, _, s := recordValidationOperands(t)
	for name, change := range map[string]func(*growatt.ProtocolIIIdentityInput){
		"family": func(x *growatt.ProtocolIIIdentityInput) { x.Profile.Family = "MID" },
		"device": func(x *growatt.ProtocolIIIdentityInput) { x.Profile.DeviceType = 18; x.Slices[2].Words[0] = 18 },
		"word82": func(x *growatt.ProtocolIIIdentityInput) { x.Profile.ModelBuild[0] = 24; x.Slices[3].Words[0] = 24 },
		"word83": func(x *growatt.ProtocolIIIdentityInput) { x.Profile.ModelBuild[1] = 42; x.Slices[3].Words[1] = 42 },
		"protocol": func(x *growatt.ProtocolIIIdentityInput) {
			x.Profile.ProtocolVersion = 257
			x.Slices[4].Words[0] = 257
		},
	} {
		t.Run(name, func(t *testing.T) {
			x := publicGrowattProtocolIIIdentityInput()
			x.Profile.DeviceType = 17
			x.Profile.ModelBuild = [2]uint16{23, 41}
			x.Profile.ProtocolVersion = 256
			x.Slices[2].Words[0] = 17
			x.Slices[3].Words = []uint16{23, 41}
			x.Slices[4].Words[0] = 256
			change(&x)
			i, e := growatt.DecodeGrowattProtocolIIIdentity(x)
			if e != nil {
				t.Fatal(e)
			}
			if e = growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s}); !errors.Is(e, growatt.ErrGrowattFC04Invalid) {
				t.Fatal(e)
			}
		})
	}
}
func TestGrowattFc04RecordValidationSchemaJoins(t *testing.T) {
	r, i, s := recordValidationOperands(t)
	for name, change := range map[string]func(*growatt.FC04SymbolicSchema){
		"source_amendment": func(x *growatt.FC04SymbolicSchema) { x.SourceSchemaRef = "synthetic:source:edition-a:amendment-b" },
		"field":            func(x *growatt.FC04SymbolicSchema) { x.FieldSchemaRef = "synthetic:fields:b" },
		"function":         func(x *growatt.FC04SymbolicSchema) { x.Function = 3 },
		"offset":           func(x *growatt.FC04SymbolicSchema) { x.Offset = 1 },
		"width":            func(x *growatt.FC04SymbolicSchema) { x.WidthWords = 60 },
		"empty_edition":    func(x *growatt.FC04SymbolicSchema) { x.DocumentEdition = "" },
		"empty_amendment":  func(x *growatt.FC04SymbolicSchema) { x.AmendmentIdentity = "" },
		"empty_fields":     func(x *growatt.FC04SymbolicSchema) { x.FieldExtents = nil },
		"zero_field": func(x *growatt.FC04SymbolicSchema) {
			x.FieldExtents = []growatt.FC04FieldExtent{{Offset: 0, WidthWords: 0}}
		},
		"overflow_field": func(x *growatt.FC04SymbolicSchema) {
			x.FieldExtents = []growatt.FC04FieldExtent{{Offset: 65535, WidthWords: 2}}
		},
		"outside_field": func(x *growatt.FC04SymbolicSchema) {
			x.FieldExtents = []growatt.FC04FieldExtent{{Offset: 58, WidthWords: 2}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			other := s
			change(&other)
			if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{other}); e == nil {
				t.Fatal("schema mismatch accepted")
			}
		})
	}
	// A competing definition cannot win by order, even when one entry matches.
	conflict := r
	conflict.ModelBuildWord83++
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{conflict, r}, []growatt.FC04SymbolicSchema{s}); !errors.Is(e, growatt.ErrGrowattFC04Ambiguous) {
		t.Fatal(e)
	}
	amended := s
	amended.AmendmentIdentity = "synthetic:amendment:b"
	if e := growatt.CompareGrowattFC04Record(r, i, []growatt.FC04Record{r}, []growatt.FC04SymbolicSchema{s, amended}); !errors.Is(e, growatt.ErrGrowattFC04Ambiguous) {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"family":"MAX","\u0066amily":"MAX"}`, `{"family":"MAX"} true`} {
		if _, e := growatt.ParseGrowattFC04Record([]byte(raw)); !errors.Is(e, growatt.ErrGrowattFC04Invalid) {
			t.Fatal(e)
		}
	}
	if status, e := growatt.ValidateGrowattFC04RecordAdmission([]byte(`null`), i); status != "NO_ADMISSIBLE_PROFILE" || !errors.Is(e, growatt.ErrGrowattFC04Invalid) {
		t.Fatalf("%s %v", status, e)
	}
	if status, e := growatt.ValidateGrowattFC04RecordAdmission(recordValidationBytes(t, recordValidationJSON()), growatt.ProtocolIIIdentityObservation{}); status != "NO_ADMISSIBLE_PROFILE" || !errors.Is(e, growatt.ErrGrowattFC04Invalid) {
		t.Fatalf("%s %v", status, e)
	}
}
