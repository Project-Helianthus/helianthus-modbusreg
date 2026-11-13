package modbusreg

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"unicode/utf8"
)

// Sun2000EvidenceInputContract is the exact contract value the input envelope
// must carry. The input contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/sun2000-detection-input-v1.md.
const Sun2000EvidenceInputContract = "sun2000-detection-input-v1"

// Sun2000EvidenceInputLimits bounds bytes and all JSON objects, including
// wrappers, alternatives and typed values. Both limits must be positive.
type Sun2000EvidenceInputLimits struct{ MaxBytes, MaxRecords int }

// Sun2000EvidenceReference is one validated evidence reference record. Its
// claim level grades only the referenced claim and confers no admission.
type Sun2000EvidenceReference struct{ id, source, locator, context, claimLevel string }

// ID returns the reference identifier, unique within the input.
func (r Sun2000EvidenceReference) ID() string { return r.id }

// Source returns the evidence source: native_identity, gateway_inventory,
// map_document or synthetic.
func (r Sun2000EvidenceReference) Source() string { return r.source }

// Locator returns where the retained evidence is found; it is never the
// evidence itself.
func (r Sun2000EvidenceReference) Locator() string { return r.locator }

// Context returns the subject or documentary applicability scope of the
// evidence.
func (r Sun2000EvidenceReference) Context() string { return r.context }

// ClaimLevel returns Proven, Hypothesis or Unknown.
func (r Sun2000EvidenceReference) ClaimLevel() string { return r.claimLevel }

// Sun2000UnitEvidence is the typed unit value of a candidate. Its context
// always equals the input subject.
type Sun2000UnitEvidence struct{ context, binding string }

// Context returns the caller-local subject the unit belongs to.
func (v Sun2000UnitEvidence) Context() string { return v.context }

// Binding returns the locator of the retained native unit-binding evidence.
func (v Sun2000UnitEvidence) Binding() string { return v.binding }

// Sun2000MapEvidence is the typed map value of a candidate: a document and
// revision citation and the locator of its applicability. It never means the
// map is admitted.
type Sun2000MapEvidence struct{ document, revision, applicability string }

// Document returns the cited map document.
func (v Sun2000MapEvidence) Document() string { return v.document }

// Revision returns the cited document revision.
func (v Sun2000MapEvidence) Revision() string { return v.revision }

// Applicability returns the locator of the family, model, unit and software
// floor applicability statement.
func (v Sun2000MapEvidence) Applicability() string { return v.applicability }

type sun2000Value interface {
	string | Sun2000UnitEvidence | Sun2000MapEvidence
}

// Sun2000EvidenceAlternative is one conflicting value of a contradictory
// field, with the references that support it.
type Sun2000EvidenceAlternative[T sun2000Value] struct {
	value T
	refs  []string
}

// Value returns the alternative value; for a family it may name another
// family, retained only as the mismatch.
func (a Sun2000EvidenceAlternative[T]) Value() T { return a.value }

// Refs returns a copy of the non-empty supporting reference identifiers.
func (a Sun2000EvidenceAlternative[T]) Refs() []string { return append([]string(nil), a.refs...) }

// Sun2000EvidencePresence is the presence wrapper of one candidate field. The
// state is missing, unavailable, present or contradictory; only present
// carries a value, and only contradictory carries alternatives, whose
// references together equal the wrapper's references.
type Sun2000EvidencePresence[T sun2000Value] struct {
	state, detail string
	value         T
	refs          []string
	alternatives  []Sun2000EvidenceAlternative[T]
}

// State returns missing, unavailable, present or contradictory.
func (p Sun2000EvidencePresence[T]) State() string { return p.state }

// Detail returns the optional explanation, empty when absent. It never
// discriminates identity.
func (p Sun2000EvidencePresence[T]) Detail() string { return p.detail }

// Value returns the field value; the second result is true only when the
// state is present. Any other state returns the zero value, never a wildcard.
func (p Sun2000EvidencePresence[T]) Value() (T, bool) { return p.value, p.state == "present" }

// Refs returns a copy of the reference identifiers; empty only for missing.
func (p Sun2000EvidencePresence[T]) Refs() []string { return append([]string(nil), p.refs...) }

// Alternatives returns deep copies of the conflicting alternatives in input
// order; empty unless the state is contradictory. Order chooses no winner.
func (p Sun2000EvidencePresence[T]) Alternatives() []Sun2000EvidenceAlternative[T] {
	out := append([]Sun2000EvidenceAlternative[T](nil), p.alternatives...)
	for i := range out {
		out[i].refs = out[i].Refs()
	}
	return out
}

// Sun2000EvidenceCandidate is one asserted identity tuple. Its five fields are
// presence wrappers; a candidate is a claim to check, not a detection result.
// Field accessors return deep copies.
type Sun2000EvidenceCandidate struct {
	id                      string
	family, model, firmware Sun2000EvidencePresence[string]
	unit                    Sun2000EvidencePresence[Sun2000UnitEvidence]
	mapping                 Sun2000EvidencePresence[Sun2000MapEvidence]
}

// ID returns the opaque candidate identifier, unique within the input.
func (c Sun2000EvidenceCandidate) ID() string { return c.id }

// Family returns the family wrapper; a present value is exactly SUN2000.
func (c Sun2000EvidenceCandidate) Family() Sun2000EvidencePresence[string] {
	return sun2000ClonePresence(c.family)
}

// Model returns the model wrapper; a present value is the exact model token.
func (c Sun2000EvidenceCandidate) Model() Sun2000EvidencePresence[string] {
	return sun2000ClonePresence(c.model)
}

// Firmware returns the firmware wrapper; a present value is retained exactly
// as supplied.
func (c Sun2000EvidenceCandidate) Firmware() Sun2000EvidencePresence[string] {
	return sun2000ClonePresence(c.firmware)
}

// Unit returns the unit wrapper.
func (c Sun2000EvidenceCandidate) Unit() Sun2000EvidencePresence[Sun2000UnitEvidence] {
	return sun2000ClonePresence(c.unit)
}

// Map returns the map wrapper.
func (c Sun2000EvidenceCandidate) Map() Sun2000EvidencePresence[Sun2000MapEvidence] {
	return sun2000ClonePresence(c.mapping)
}

// Sun2000EvidenceInput is a structural snapshot, never a detection result.
// Its zero value contains no validated input; only a successful parse validates it.
type Sun2000EvidenceInput struct {
	subject    string
	evidence   []Sun2000EvidenceReference
	candidates []Sun2000EvidenceCandidate
}

// Subject returns the opaque caller-local endpoint and unit context.
func (s Sun2000EvidenceInput) Subject() string { return s.subject }

// Evidence returns a copy of the reference records in input order.
func (s Sun2000EvidenceInput) Evidence() []Sun2000EvidenceReference {
	return append([]Sun2000EvidenceReference(nil), s.evidence...)
}

// Candidates returns deep copies of the candidates in input order; order
// chooses no winner.
func (s Sun2000EvidenceInput) Candidates() []Sun2000EvidenceCandidate {
	out := append([]Sun2000EvidenceCandidate(nil), s.candidates...)
	for i := range out {
		out[i].family = out[i].Family()
		out[i].model = out[i].Model()
		out[i].firmware = out[i].Firmware()
		out[i].unit = out[i].Unit()
		out[i].mapping = out[i].Map()
	}
	return out
}

type sun2000Invalid string

func (e sun2000Invalid) Error() string { return "sun2000 evidence input: " + string(e) }
func sun2000Require(ok bool, reason string) {
	if !ok {
		panic(sun2000Invalid(reason))
	}
}

// ParseSun2000EvidenceInput performs zero I/O and validates only structure and
// reference coherence. Errors are one bounded static message, without input text.
// A valid snapshot confers no provenance, qualification, support or admission.
func ParseSun2000EvidenceInput(data []byte, limits Sun2000EvidenceInputLimits) (result Sun2000EvidenceInput, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if invalid, ok := failure.(sun2000Invalid); ok {
				result = Sun2000EvidenceInput{}
				err = invalid
			} else {
				panic(failure)
			}
		}
	}()
	sun2000Require(limits.MaxBytes > 0 && limits.MaxRecords > 0, "invalid budgets")
	sun2000Require(len(data) <= limits.MaxBytes, "byte budget exceeded")
	sun2000Require(utf8.Valid(data), "invalid JSON")
	d := json.NewDecoder(bytes.NewReader(data))
	records := 0
	root := sun2000Object(sun2000Read(d, &records, limits.MaxRecords, 0), "contract", "subject", "evidence", "candidates")
	_, tail := d.Token()
	sun2000Require(errors.Is(tail, io.EOF), "trailing JSON")
	sun2000Require(sun2000String(root["contract"]) == Sun2000EvidenceInputContract, "invalid contract")
	result.subject = sun2000String(root["subject"])
	refs := map[string]bool{}
	for _, raw := range sun2000Array(root["evidence"]) {
		obj := sun2000Object(raw, "id", "source", "locator", "context", "claim_level")
		r := Sun2000EvidenceReference{sun2000String(obj["id"]), sun2000String(obj["source"]), sun2000String(obj["locator"]), sun2000String(obj["context"]), sun2000String(obj["claim_level"])}
		sun2000Require(!refs[r.id], "duplicate evidence ID")
		refs[r.id] = true
		sun2000Require(r.source == "native_identity" || r.source == "gateway_inventory" || r.source == "map_document" || r.source == "synthetic", "invalid evidence source")
		sun2000Require(r.claimLevel == "Proven" || r.claimLevel == "Hypothesis" || r.claimLevel == "Unknown", "invalid claim level")
		result.evidence = append(result.evidence, r)
	}
	ids, tuples := map[string]bool{}, map[string]bool{}
	for _, raw := range sun2000Array(root["candidates"]) {
		obj := sun2000Object(raw, "id", "family", "model", "firmware", "unit", "map")
		c := Sun2000EvidenceCandidate{id: sun2000String(obj["id"])}
		sun2000Require(!ids[c.id], "duplicate candidate ID")
		ids[c.id] = true
		family := func(v any, alternative bool) string {
			s := sun2000String(v)
			sun2000Require(alternative || s == "SUN2000", "invalid family")
			return s
		}
		text := func(v any, _ bool) string { return sun2000String(v) }
		unit := func(v any, _ bool) Sun2000UnitEvidence {
			o := sun2000Object(v, "context", "binding")
			u := Sun2000UnitEvidence{sun2000String(o["context"]), sun2000String(o["binding"])}
			sun2000Require(u.context == result.subject, "unit subject mismatch")
			return u
		}
		mapping := func(v any, _ bool) Sun2000MapEvidence {
			o := sun2000Object(v, "document", "revision", "applicability")
			return Sun2000MapEvidence{sun2000String(o["document"]), sun2000String(o["revision"]), sun2000String(o["applicability"])}
		}
		c.family = sun2000Presence(obj["family"], refs, family)
		c.model = sun2000Presence(obj["model"], refs, text)
		c.firmware = sun2000Presence(obj["firmware"], refs, text)
		c.unit = sun2000Presence(obj["unit"], refs, unit)
		c.mapping = sun2000Presence(obj["map"], refs, mapping)
		// Identity excludes evidence IDs, detail and serialization order.
		key := sun2000TupleKey(obj)
		sun2000Require(!tuples[key], "duplicate candidate tuple")
		tuples[key] = true
		result.candidates = append(result.candidates, c)
	}
	return result, nil
}

func sun2000Read(d *json.Decoder, records *int, max, depth int) any {
	sun2000Require(depth <= 64, "JSON nesting exceeded")
	token, err := d.Token()
	sun2000Require(err == nil, "invalid JSON")
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			*records++
			sun2000Require(*records <= max, "record budget exceeded")
			object := map[string]any{}
			for d.More() {
				k, e := d.Token()
				sun2000Require(e == nil, "invalid JSON")
				key, ok := k.(string)
				sun2000Require(ok, "invalid JSON")
				_, exists := object[key]
				sun2000Require(!exists, "duplicate field")
				object[key] = sun2000Read(d, records, max, depth+1)
			}
			end, e := d.Token()
			sun2000Require(e == nil && end == json.Delim('}'), "invalid JSON")
			return object
		case '[':
			array := []any{}
			for d.More() {
				array = append(array, sun2000Read(d, records, max, depth+1))
			}
			end, e := d.Token()
			sun2000Require(e == nil && end == json.Delim(']'), "invalid JSON")
			return array
		}
		sun2000Require(false, "invalid JSON")
	}
	return token
}
func sun2000Object(v any, fields ...string) map[string]any {
	o, ok := v.(map[string]any)
	sun2000Require(ok, "invalid object")
	optional := len(fields) > 0 && fields[len(fields)-1] == "detail"
	required := len(fields)
	if optional {
		required--
	}
	sun2000Require(len(o) == required || optional && len(o) == len(fields), "invalid fields")
	for i, key := range fields {
		_, exists := o[key]
		sun2000Require(exists || optional && i == len(fields)-1, "invalid fields")
	}
	for key := range o {
		found := false
		for _, allowed := range fields {
			found = found || key == allowed
		}
		sun2000Require(found, "invalid fields")
	}
	return o
}
func sun2000Array(v any) []any { a, ok := v.([]any); sun2000Require(ok, "invalid array"); return a }
func sun2000String(v any) string {
	s, ok := v.(string)
	sun2000Require(ok && s != "", "invalid string")
	return s
}
func sun2000Refs(v any, domain map[string]bool) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range sun2000Array(v) {
		id := sun2000String(raw)
		sun2000Require(!seen[id], "duplicate reference")
		sun2000Require(domain[id], "dangling reference")
		seen[id] = true
		out = append(out, id)
	}
	return out
}
func sun2000Presence[T sun2000Value](raw any, domain map[string]bool, decode func(any, bool) T) Sun2000EvidencePresence[T] {
	o := sun2000Object(raw, "state", "value", "refs", "alternatives", "detail")
	p := Sun2000EvidencePresence[T]{state: sun2000String(o["state"]), refs: sun2000Refs(o["refs"], domain)}
	if detail, ok := o["detail"]; ok {
		p.detail = sun2000String(detail)
	}
	alts := sun2000Array(o["alternatives"])
	switch p.state {
	case "missing":
		sun2000Require(o["value"] == nil && len(p.refs) == 0 && len(alts) == 0, "incoherent missing")
	case "unavailable":
		sun2000Require(o["value"] == nil && len(p.refs) > 0 && len(alts) == 0, "incoherent unavailable")
	case "present":
		sun2000Require(len(p.refs) > 0 && len(alts) == 0, "incoherent present")
		p.value = decode(o["value"], false)
	case "contradictory":
		sun2000Require(o["value"] == nil && len(alts) >= 2, "incoherent contradiction")
		union, values := map[string]bool{}, map[T]bool{}
		for _, rawAlt := range alts {
			a := sun2000Object(rawAlt, "value", "refs")
			value := decode(a["value"], true)
			sun2000Require(!values[value], "compatible alternatives")
			values[value] = true
			refs := sun2000Refs(a["refs"], domain)
			sun2000Require(len(refs) > 0, "empty alternative references")
			for _, id := range refs {
				union[id] = true
			}
			p.alternatives = append(p.alternatives, Sun2000EvidenceAlternative[T]{value, refs})
		}
		sun2000Require(len(union) == len(p.refs), "reference union mismatch")
		for _, id := range p.refs {
			sun2000Require(union[id], "reference union mismatch")
		}
	default:
		sun2000Require(false, "invalid state")
	}
	return p
}
func sun2000TupleKey(o map[string]any) string {
	fields := []any{}
	for _, name := range []string{"family", "model", "firmware", "unit", "map"} {
		p := o[name].(map[string]any)
		alternatives := []string{}
		for _, raw := range p["alternatives"].([]any) {
			b, _ := json.Marshal(raw.(map[string]any)["value"])
			alternatives = append(alternatives, string(b))
		}
		sort.Strings(alternatives)
		fields = append(fields, []any{p["state"], p["value"], alternatives})
	}
	b, _ := json.Marshal(fields)
	return string(b)
}

func sun2000ClonePresence[T sun2000Value](p Sun2000EvidencePresence[T]) Sun2000EvidencePresence[T] {
	p.refs = p.Refs()
	p.alternatives = p.Alternatives()
	return p
}
