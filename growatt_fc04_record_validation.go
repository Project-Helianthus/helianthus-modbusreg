package modbusreg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrGrowattFC04Missing   = errors.New("growatt FC04 missing or insufficient evidence")
	ErrGrowattFC04Invalid   = errors.New("growatt FC04 invalid record or comparison")
	ErrGrowattFC04Ambiguous = errors.New("growatt FC04 conflicting references")
)

// GrowattFC04Record is an untrusted proposed mapping, never reviewed evidence.
// The two model words retain FC03 register order. SourceSchemaRef identifies
// an exact document edition and amendment, separately from ReportedProtocolWord.
type GrowattFC04Record struct {
	Family               string `json:"family"`
	DeviceType           uint16 `json:"device_type"`
	ModelBuildWord82     uint16 `json:"model_build_word_82"`
	ModelBuildWord83     uint16 `json:"model_build_word_83"`
	ReportedProtocolWord uint16 `json:"reported_protocol_word"`
	SourceSchemaRef      string `json:"source_schema_ref"`
	Function             uint16 `json:"function"`
	Offset               uint16 `json:"offset"`
	WidthWords           uint16 `json:"width_words"`
	FieldSchemaRef       string `json:"field_schema_ref"`
	TupleEvidenceRef     string `json:"tuple_evidence_ref"`
}

// GrowattFC04FieldExtent supplies only symbolic geometry, not decoding rules.
type GrowattFC04FieldExtent struct{ Offset, WidthWords uint16 }

// GrowattFC04SymbolicSchema is a caller-supplied comparison operand. Neither
// its references nor its document metadata establish public review or admission.
type GrowattFC04SymbolicSchema struct {
	SourceSchemaRef, FieldSchemaRef, DocumentEdition, AmendmentIdentity string
	Function, Offset, WidthWords                                        uint16
	FieldExtents                                                        []GrowattFC04FieldExtent
}

func growattFC04Reference(s string) bool {
	return s != "" && utf8.ValidString(s) && !strings.ContainsAny(s, "\ufffd") && strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}
func (r GrowattFC04Record) valid() bool {
	return (r.Family == "MAX" || r.Family == "MID" || r.Family == "MAC") && r.Function == 4 && r.Offset == 0 && r.WidthWords == 59 && growattFC04Reference(r.SourceSchemaRef) && growattFC04Reference(r.FieldSchemaRef) && growattFC04Reference(r.TupleEvidenceRef)
}

// ParseGrowattFC04Record accepts one closed JSON object. It rejects missing,
// duplicate and unknown keys, nulls, coercions and out-of-domain extents.
func ParseGrowattFC04Record(data []byte) (GrowattFC04Record, error) {
	var zero GrowattFC04Record
	if !utf8.Valid(data) {
		return zero, ErrGrowattFC04Invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return zero, ErrGrowattFC04Invalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return zero, ErrGrowattFC04Invalid
		}
		key, ok := k.(string)
		if !ok {
			return zero, ErrGrowattFC04Invalid
		}
		if _, ok := fields[key]; ok {
			return zero, fmt.Errorf("%w: duplicate %s", ErrGrowattFC04Invalid, key)
		}
		switch key {
		case "family", "device_type", "model_build_word_82", "model_build_word_83", "reported_protocol_word", "source_schema_ref", "function", "offset", "width_words", "field_schema_ref", "tuple_evidence_ref":
		default:
			return zero, fmt.Errorf("%w: unknown key", ErrGrowattFC04Invalid)
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(raw, []byte("null")) {
			return zero, ErrGrowattFC04Invalid
		}
		fields[key] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') {
		return zero, ErrGrowattFC04Invalid
	}
	if _, e = d.Token(); e != io.EOF {
		return zero, ErrGrowattFC04Invalid
	}
	// Decode present fields before classifying omissions, so malformed values
	// never become defaults and never masquerade as merely missing evidence.
	encoded, e := json.Marshal(fields)
	if e != nil {
		return zero, ErrGrowattFC04Invalid
	}
	var r GrowattFC04Record
	if json.Unmarshal(encoded, &r) != nil {
		return zero, ErrGrowattFC04Invalid
	}
	if len(fields) != 11 {
		return zero, ErrGrowattFC04Missing
	}
	if !r.valid() {
		return zero, ErrGrowattFC04Invalid
	}
	return r, nil
}

func growattFC04IdentityValid(i GrowattProtocolIIIdentityObservation) bool {
	decoded, e := DecodeGrowattProtocolIIIdentity(GrowattProtocolIIIdentityInput{UnitID: i.UnitID(), Function: FunctionReadHoldingRegisters, Profile: i.Profile(), Slices: i.Slices()})
	return e == nil && decoded.DeviceType() == i.DeviceType() && decoded.ModelBuild() == i.ModelBuild() && decoded.ProtocolVersion() == i.ProtocolVersion()
}

// CompareGrowattFC04Record diagnoses symbolic equality only. Supplied entries
// are not a reviewed catalog. Success creates no applicability, telemetry,
// observer, qualification or cached admission decision.
func CompareGrowattFC04Record(r GrowattFC04Record, i GrowattProtocolIIIdentityObservation, entries []GrowattFC04Record, schemas []GrowattFC04SymbolicSchema) error {
	if !r.valid() || !growattFC04IdentityValid(i) {
		return ErrGrowattFC04Invalid
	}
	var candidates []GrowattFC04Record
	for _, entry := range entries {
		if entry.TupleEvidenceRef == r.TupleEvidenceRef {
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) > 1 {
		return ErrGrowattFC04Ambiguous
	}
	if len(candidates) == 0 {
		return ErrGrowattFC04Missing
	}
	e := candidates[0]
	var sources []GrowattFC04SymbolicSchema
	for _, schema := range schemas {
		if schema.SourceSchemaRef == r.SourceSchemaRef {
			sources = append(sources, schema)
		}
	}
	if len(sources) > 1 {
		return ErrGrowattFC04Ambiguous
	}
	if len(sources) == 0 {
		return ErrGrowattFC04Missing
	}
	s := sources[0]
	if r.Family != i.Profile().Family || r.Family != e.Family ||
		r.DeviceType != i.DeviceType() || r.DeviceType != e.DeviceType ||
		r.ModelBuildWord82 != i.ModelBuild()[0] || r.ModelBuildWord82 != e.ModelBuildWord82 ||
		r.ModelBuildWord83 != i.ModelBuild()[1] || r.ModelBuildWord83 != e.ModelBuildWord83 ||
		r.ReportedProtocolWord != i.ProtocolVersion() || r.ReportedProtocolWord != e.ReportedProtocolWord ||
		r.SourceSchemaRef != e.SourceSchemaRef || r.SourceSchemaRef != s.SourceSchemaRef ||
		r.Function != e.Function || r.Function != s.Function ||
		r.Offset != e.Offset || r.Offset != s.Offset ||
		r.WidthWords != e.WidthWords || r.WidthWords != s.WidthWords ||
		r.FieldSchemaRef != e.FieldSchemaRef || r.FieldSchemaRef != s.FieldSchemaRef ||
		r.TupleEvidenceRef != e.TupleEvidenceRef {
		return ErrGrowattFC04Invalid
	}
	if !growattFC04Reference(s.DocumentEdition) || !growattFC04Reference(s.AmendmentIdentity) || len(s.FieldExtents) == 0 {
		return ErrGrowattFC04Invalid
	}
	for _, f := range s.FieldExtents {
		if f.WidthWords == 0 || uint32(f.Offset) < uint32(s.Offset) || uint32(f.Offset)+uint32(f.WidthWords) > uint32(s.Offset)+uint32(s.WidthWords) {
			return ErrGrowattFC04Invalid
		}
	}
	return nil
}

// ValidateGrowattFC04RecordAdmission uses the current empty reviewed public
// catalog. No caller operands, approval flag or synthetic record can populate
// that catalog. Every result remains NO_ADMISSIBLE_PROFILE.
func ValidateGrowattFC04RecordAdmission(data []byte, i GrowattProtocolIIIdentityObservation) (string, error) {
	const denied = "NO_ADMISSIBLE_PROFILE"
	if _, e := ParseGrowattFC04Record(data); e != nil {
		return denied, e
	}
	if !growattFC04IdentityValid(i) {
		return denied, ErrGrowattFC04Invalid
	}
	return denied, ErrGrowattFC04Missing
}
