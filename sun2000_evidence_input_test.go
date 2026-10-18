package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestSun2000EvidenceInputEmpty(t *testing.T) {
	input, err := ParseSun2000EvidenceInput([]byte(`{"contract":"sun2000-detection-input-v1","subject":"synthetic-subject","evidence":[],"candidates":[]}`), Sun2000EvidenceInputLimits{MaxBytes: 4096, MaxRecords: 32})
	if err != nil {
		t.Fatal(err)
	}
	if input.Subject() != "synthetic-subject" || len(input.Candidates()) != 0 {
		t.Fatal("unexpected snapshot")
	}
}

func sun2000TestEnvelope() map[string]any {
	missing := func() any {
		return map[string]any{"state": "missing", "value": nil, "refs": []any{}, "alternatives": []any{}}
	}
	present := func(value any) any {
		return map[string]any{"state": "present", "value": value, "refs": []any{"evidence-a"}, "alternatives": []any{}, "detail": "synthetic explanation"}
	}
	return map[string]any{
		"contract": Sun2000EvidenceInputContract, "subject": "synthetic-subject",
		"evidence": []any{
			map[string]any{"id": "evidence-a", "source": "synthetic", "locator": "synthetic record a", "context": "synthetic-subject", "claim_level": "Hypothesis"},
			map[string]any{"id": "evidence-b", "source": "synthetic", "locator": "synthetic record b", "context": "synthetic-subject", "claim_level": "Unknown"},
		},
		"candidates": []any{map[string]any{"id": "candidate-a", "family": present("SUN2000"), "model": present("synthetic-model-a"), "firmware": present("synthetic-firmware-a"), "unit": present(map[string]any{"context": "synthetic-subject", "binding": "synthetic-binding-a"}), "map": present(map[string]any{"document": "synthetic-document", "revision": "synthetic-revision", "applicability": "synthetic applicability"})}, map[string]any{"id": "candidate-b", "family": missing(), "model": missing(), "firmware": missing(), "unit": missing(), "map": missing()}},
	}
}
func sun2000TestParse(v any) (Sun2000EvidenceInput, error) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return ParseSun2000EvidenceInput(b, Sun2000EvidenceInputLimits{MaxBytes: 65536, MaxRecords: 256})
}
func sun2000TestCandidate(e map[string]any) map[string]any {
	return e["candidates"].([]any)[0].(map[string]any)
}
func sun2000TestWrapper(e map[string]any, name string) map[string]any {
	return sun2000TestCandidate(e)[name].(map[string]any)
}
func sun2000TestReject(t *testing.T, e any) {
	t.Helper()
	s, err := sun2000TestParse(e)
	if err == nil {
		t.Fatal("accepted malformed input")
	}
	if s.Subject() != "" || len(s.Candidates()) != 0 || len(s.Evidence()) != 0 {
		t.Fatal("partial snapshot escaped")
	}
	if len(err.Error()) > 96 {
		t.Fatal("unbounded error")
	}
}

func TestSun2000EvidenceInputStates(t *testing.T) {
	for _, field := range []string{"family", "model", "firmware", "unit", "map"} {
		for _, state := range []string{"missing", "unavailable", "present", "contradictory"} {
			t.Run(field+"/"+state, func(t *testing.T) {
				e := sun2000TestEnvelope()
				p := sun2000TestWrapper(e, field)
				value := p["value"]
				p["state"] = state
				if state != "present" {
					p["value"] = nil
				}
				if state == "missing" {
					p["refs"] = []any{}
				}
				if state == "contradictory" {
					other := any("synthetic-other")
					switch field {
					case "unit":
						other = map[string]any{"context": "synthetic-subject", "binding": "synthetic-binding-b"}
					case "map":
						other = map[string]any{"document": "synthetic-document-b", "revision": "synthetic-revision", "applicability": "synthetic applicability"}
					}
					p["refs"] = []any{"evidence-b", "evidence-a"}
					p["alternatives"] = []any{map[string]any{"value": value, "refs": []any{"evidence-a"}}, map[string]any{"value": other, "refs": []any{"evidence-b"}}}
				}
				if _, err := sun2000TestParse(e); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	for _, source := range []string{"native_identity", "gateway_inventory", "map_document", "synthetic"} {
		for _, level := range []string{"Proven", "Hypothesis", "Unknown"} {
			e := sun2000TestEnvelope()
			r := e["evidence"].([]any)[0].(map[string]any)
			r["source"] = source
			r["claim_level"] = level
			if _, err := sun2000TestParse(e); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Every object/key occurrence is varied independently, including nested values.
func TestSun2000EvidenceInputClosedFields(t *testing.T) {
	for _, contradiction := range []bool{false, true} {
		e := sun2000TestEnvelope()
		if contradiction {
			p := sun2000TestWrapper(e, "model")
			p["state"] = "contradictory"
			p["value"] = nil
			p["refs"] = []any{"evidence-a", "evidence-b"}
			p["alternatives"] = []any{map[string]any{"value": "synthetic-left", "refs": []any{"evidence-a"}}, map[string]any{"value": "synthetic-right", "refs": []any{"evidence-b"}}}
		}
		var visit func(any, string)
		visit = func(v any, path string) {
			switch node := v.(type) {
			case map[string]any:
				t.Run(path+"/unknown", func(t *testing.T) {
					node["unsafe-extra"] = true
					defer delete(node, "unsafe-extra")
					sun2000TestReject(t, e)
				})
				keys := make([]string, 0, len(node))
				for k := range node {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					old := node[k]
					if k != "detail" {
						t.Run(path+"/missing-"+k, func(t *testing.T) { delete(node, k); defer func() { node[k] = old }(); sun2000TestReject(t, e) })
					}
					t.Run(path+"/type-"+k, func(t *testing.T) { node[k] = true; defer func() { node[k] = old }(); sun2000TestReject(t, e) })
					if old != nil {
						t.Run(path+"/null-"+k, func(t *testing.T) { node[k] = nil; defer func() { node[k] = old }(); sun2000TestReject(t, e) })
					}
					// Inject a repeated key into this exact object in serialized input.
					t.Run(path+"/duplicate-"+k, func(t *testing.T) {
						original, _ := json.Marshal(node)
						key, _ := json.Marshal(k)
						val, _ := json.Marshal(old)
						duplicate := append([]byte{'{'}, append(append(append(key, ':'), val...), ',')...)
						duplicate = append(duplicate, original[1:]...)
						all, _ := json.Marshal(e)
						all = bytes.Replace(all, original, duplicate, 1)
						if _, err := ParseSun2000EvidenceInput(all, Sun2000EvidenceInputLimits{65536, 256}); err == nil {
							t.Fatal("accepted duplicate key")
						}
					})
					visit(old, path+"/"+k)
				}
			case []any:
				for i, item := range node {
					visit(item, fmt.Sprintf("%s/%d", path, i))
				}
			}
		}
		visit(e, fmt.Sprint(contradiction))
	}
}

func TestSun2000EvidenceInputMalformedEdges(t *testing.T) {
	cases := map[string]func(map[string]any){
		"contract":           func(e map[string]any) { e["contract"] = "other" },
		"empty-subject":      func(e map[string]any) { e["subject"] = "" },
		"candidate-id":       func(e map[string]any) { e["candidates"].([]any)[1].(map[string]any)["id"] = "candidate-a" },
		"evidence-id":        func(e map[string]any) { e["evidence"].([]any)[1].(map[string]any)["id"] = "evidence-a" },
		"empty-candidate-id": func(e map[string]any) { sun2000TestCandidate(e)["id"] = "" },
		"empty-evidence-id":  func(e map[string]any) { e["evidence"].([]any)[0].(map[string]any)["id"] = "" },
		"source":             func(e map[string]any) { e["evidence"].([]any)[0].(map[string]any)["source"] = "other" },
		"level":              func(e map[string]any) { e["evidence"].([]any)[0].(map[string]any)["claim_level"] = "qualified" },
		"family":             func(e map[string]any) { sun2000TestWrapper(e, "family")["value"] = "other" },
		"subject":            func(e map[string]any) { e["subject"] = "synthetic-other-subject" },
		"unit-context": func(e map[string]any) {
			sun2000TestWrapper(e, "unit")["value"].(map[string]any)["context"] = "synthetic-other-unit"
		},
		"duplicate-tuple": func(e map[string]any) {
			b, _ := json.Marshal(sun2000TestCandidate(e))
			var c map[string]any
			_ = json.Unmarshal(b, &c)
			c["id"] = "distinct-id"
			c["model"].(map[string]any)["refs"] = []any{"evidence-b"}
			e["candidates"] = append(e["candidates"].([]any), c)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) { e := sun2000TestEnvelope(); change(e); sun2000TestReject(t, e) })
	}
	for _, field := range []string{"family", "model", "firmware", "unit", "map"} {
		for _, fault := range []string{"dangling", "duplicate-ref", "empty-ref", "no-ref", "state", "missing-value", "missing-refs", "unavailable-value", "unavailable-refs", "present-alternatives", "empty-detail", "same-alternatives", "alternative-no-ref", "alternative-dangling", "alternative-duplicate-ref", "union", "union-same-size", "one-alternative", "contradiction-value", "empty-value"} {
			t.Run(field+"/"+fault, func(t *testing.T) {
				e := sun2000TestEnvelope()
				p := sun2000TestWrapper(e, field)
				value := p["value"]
				switch fault {
				case "dangling":
					p["refs"] = []any{"unknown-target"}
				case "duplicate-ref":
					p["refs"] = []any{"evidence-a", "evidence-a"}
				case "empty-ref":
					p["refs"] = []any{""}
				case "no-ref":
					p["refs"] = []any{}
				case "state":
					p["state"] = "other"
				case "missing-value":
					p["state"] = "missing"
					p["refs"] = []any{}
				case "missing-refs":
					p["state"] = "missing"
					p["value"] = nil
				case "unavailable-value":
					p["state"] = "unavailable"
				case "unavailable-refs":
					p["state"] = "unavailable"
					p["value"] = nil
					p["refs"] = []any{}
				case "present-alternatives":
					p["alternatives"] = []any{map[string]any{"value": value, "refs": []any{"evidence-a"}}}
				case "empty-detail":
					p["detail"] = ""
				case "empty-value":
					switch v := value.(type) {
					case string:
						p["value"] = ""
					case map[string]any:
						for k := range v {
							v[k] = ""
						}
					}
				default:
					other := any("synthetic-other")
					if field == "unit" {
						other = map[string]any{"context": "synthetic-subject", "binding": "synthetic-other"}
					}
					if field == "map" {
						other = map[string]any{"document": "synthetic-other", "revision": "synthetic-other", "applicability": "synthetic-other"}
					}
					a := map[string]any{"value": value, "refs": []any{"evidence-a"}}
					b := map[string]any{"value": other, "refs": []any{"evidence-b"}}
					p["state"] = "contradictory"
					p["value"] = nil
					p["refs"] = []any{"evidence-a", "evidence-b"}
					p["alternatives"] = []any{a, b}
					switch fault {
					case "same-alternatives":
						b["value"] = value
					case "alternative-no-ref":
						b["refs"] = []any{}
						p["refs"] = []any{"evidence-a"}
					case "alternative-dangling":
						b["refs"] = []any{"unknown-target"}
					case "alternative-duplicate-ref":
						b["refs"] = []any{"evidence-b", "evidence-b"}
					case "union":
						p["refs"] = []any{"evidence-a"}
					case "union-same-size":
						b["refs"] = []any{"evidence-c"}
						e["evidence"] = append(e["evidence"].([]any), map[string]any{"id": "evidence-c", "source": "synthetic", "locator": "synthetic record c", "context": "synthetic-subject", "claim_level": "Unknown"})
					case "one-alternative":
						p["alternatives"] = []any{a}
					case "contradiction-value":
						p["value"] = value
					}
				}
				sun2000TestReject(t, e)
			})
		}
	}
}

func TestSun2000EvidenceInputBudgetsAndSyntax(t *testing.T) {
	data, _ := json.Marshal(sun2000TestEnvelope())
	for _, limits := range []Sun2000EvidenceInputLimits{{}, {0, 100}, {100000, 0}, {-1, 100}, {100000, -1}, {len(data) - 1, 100}, {len(data), 1}} {
		if _, err := ParseSun2000EvidenceInput(data, limits); err == nil {
			t.Fatalf("accepted limits %+v", limits)
		} else if (limits.MaxBytes <= 0 || limits.MaxRecords <= 0) && err.Error() != "sun2000 evidence input: invalid budgets" {
			t.Fatal("invalid budgets not rejected before parsing")
		}
	}
	if _, err := ParseSun2000EvidenceInput(data, Sun2000EvidenceInputLimits{len(data), 256}); err != nil {
		t.Fatal(err)
	}
	// The fixture contains 17 objects: envelope, two evidence, two candidates,
	// ten wrappers, unit and map values, plus no alternative objects.
	var objects int
	var count func(any)
	count = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			objects++
			for _, x := range n {
				count(x)
			}
		case []any:
			for _, x := range n {
				count(x)
			}
		}
	}
	count(sun2000TestEnvelope())
	if objects != 17 {
		t.Fatalf("unexpected independent record count %d", objects)
	}
	for _, delta := range []int{-1, 0} {
		_, err := ParseSun2000EvidenceInput(data, Sun2000EvidenceInputLimits{len(data), objects + delta})
		if (err == nil) != (delta == 0) {
			t.Fatalf("record boundary %d: %v", delta, err)
		}
	}
	for i, b := range [][]byte{nil, []byte(`{`), []byte(`[]`), append(append([]byte{}, data...), []byte(` {}`)...), []byte(`{"x":1,}`), {0xff}, []byte(strings.Repeat("[", 66) + strings.Repeat("]", 66))} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := ParseSun2000EvidenceInput(b, Sun2000EvidenceInputLimits{65536, 256})
			if err == nil || len(err.Error()) > 96 {
				t.Fatal("syntax accepted or error unbounded")
			}
		})
	}
	unsafe := sun2000TestEnvelope()
	unsafe["untrusted-private-text"] = true
	_, err := sun2000TestParse(unsafe)
	if err == nil || strings.Contains(err.Error(), "untrusted-private-text") {
		t.Fatal("unsafe error")
	}
}

func TestSun2000EvidenceInputDetached(t *testing.T) {
	e := sun2000TestEnvelope()
	p := sun2000TestWrapper(e, "model")
	p["state"] = "contradictory"
	p["value"] = nil
	p["refs"] = []any{"evidence-a", "evidence-b"}
	p["alternatives"] = []any{map[string]any{"value": "synthetic-left", "refs": []any{"evidence-a"}}, map[string]any{"value": "synthetic-right", "refs": []any{"evidence-b"}}}
	data, _ := json.Marshal(e)
	s, err := ParseSun2000EvidenceInput(data, Sun2000EvidenceInputLimits{65536, 256})
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 'x'
	}
	evidence := s.Evidence()
	evidence[0] = Sun2000EvidenceReference{}
	candidates := s.Candidates()
	candidates[0].id = "changed"
	candidates[0].family.refs[0] = "changed"
	candidates[0].model.alternatives[0].refs[0] = "changed"
	candidate := s.Candidates()[0]
	family := candidate.Family()
	family.refs[0] = "changed"
	model := candidate.Model()
	model.alternatives[0].refs[0] = "changed"
	for _, refs := range [][]string{candidate.Family().Refs(), candidate.Firmware().Refs(), candidate.Unit().Refs(), candidate.Map().Refs(), candidate.Model().Refs(), candidate.Model().Alternatives()[0].Refs()} {
		refs[0] = "changed"
	}
	alternatives := candidate.Model().Alternatives()
	alternatives[0].value = "changed"
	alternatives[0].refs[0] = "changed"
	if s.Subject() != "synthetic-subject" || s.Evidence()[0].ID() != "evidence-a" || s.Candidates()[0].ID() != "candidate-a" || s.Candidates()[0].Family().Refs()[0] != "evidence-a" || candidate.Family().Refs()[0] != "evidence-a" || candidate.Model().Alternatives()[0].Value() != "synthetic-left" || candidate.Model().Alternatives()[0].Refs()[0] != "evidence-a" {
		t.Fatal("snapshot alias")
	}
	for _, refs := range [][]string{candidate.Firmware().Refs(), candidate.Unit().Refs(), candidate.Map().Refs(), candidate.Model().Refs()} {
		if refs[0] != "evidence-a" {
			t.Fatal("reference alias")
		}
	}
	// Failed next snapshots cannot erase the retained snapshot.
	e["subject"] = "different-subject"
	sun2000TestReject(t, e)
	unit, ok := candidate.Unit().Value()
	if !ok || unit.Context() != "synthetic-subject" || unit.Binding() != "synthetic-binding-a" {
		t.Fatal("unit changed")
	}
	mapping, ok := candidate.Map().Value()
	if !ok || mapping.Document() != "synthetic-document" || mapping.Revision() != "synthetic-revision" || mapping.Applicability() != "synthetic applicability" {
		t.Fatal("map changed")
	}
	if _, ok := candidate.Model().Value(); ok {
		t.Fatal("contradiction exposes present value")
	}
	r := s.Evidence()[0]
	if r.Source() != "synthetic" || r.Locator() != "synthetic record a" || r.Context() != "synthetic-subject" || r.ClaimLevel() != "Hypothesis" || candidate.Family().State() != "present" || candidate.Family().Detail() != "synthetic explanation" {
		t.Fatal("retained metadata changed")
	}
}

func TestSun2000EvidenceInputNoAdmission(t *testing.T) {
	e := sun2000TestEnvelope()
	e["candidates"] = e["candidates"].([]any)[:1]
	s, err := sun2000TestParse(e)
	if err != nil || len(s.Candidates()) != 1 {
		t.Fatalf("structure-valid control: %v", err)
	}
	data, err := os.ReadFile("profiles/vendor/huawei/sun2000-disposition.json")
	if err != nil {
		t.Fatal(err)
	}
	var disposition struct {
		Outcome   string `json:"outcome"`
		Catalog   bool   `json:"catalog_registered"`
		Admission bool   `json:"automatic_runtime_admission"`
		Support   bool   `json:"support_claim"`
		Write     bool   `json:"write_authority"`
	}
	if err := json.Unmarshal(data, &disposition); err != nil {
		t.Fatal(err)
	}
	if disposition.Outcome != "NO_ADMISSIBLE_PROFILE" || disposition.Catalog || disposition.Admission || disposition.Support || disposition.Write {
		t.Fatal("native disposition changed")
	}
}

func TestSun2000EvidenceInputIdentityAxes(t *testing.T) {
	for _, axis := range []string{"candidate-id", "evidence-id", "reference-target", "subject", "unit-binding", "model", "firmware", "map-document", "map-revision", "map-applicability"} {
		t.Run(axis, func(t *testing.T) {
			e := sun2000TestEnvelope()
			c := sun2000TestCandidate(e)
			switch axis {
			case "candidate-id":
				c["id"] = "synthetic-candidate-other"
			case "evidence-id":
				e["evidence"].([]any)[1].(map[string]any)["id"] = "synthetic-reference-other"
			case "reference-target":
				sun2000TestWrapper(e, "model")["refs"] = []any{"evidence-b"}
			case "subject":
				e["subject"] = "synthetic-subject-other"
				sun2000TestWrapper(e, "unit")["value"].(map[string]any)["context"] = "synthetic-subject-other"
			case "unit-binding":
				sun2000TestWrapper(e, "unit")["value"].(map[string]any)["binding"] = "synthetic-binding-other"
			case "model", "firmware":
				sun2000TestWrapper(e, axis)["value"] = "synthetic-token-other"
			case "map-document", "map-revision", "map-applicability":
				sun2000TestWrapper(e, "map")["value"].(map[string]any)[strings.TrimPrefix(axis, "map-")] = "synthetic-map-other"
			}
			s, err := sun2000TestParse(e)
			if err != nil {
				t.Fatal(err)
			}
			parsed := s.Candidates()[0]
			expectedID, expectedEvidence, expectedRef, expectedSubject := "candidate-a", "evidence-b", "evidence-a", "synthetic-subject"
			expectedBinding, expectedModel, expectedFirmware := "synthetic-binding-a", "synthetic-model-a", "synthetic-firmware-a"
			expectedDoc, expectedRevision, expectedApplicability := "synthetic-document", "synthetic-revision", "synthetic applicability"
			switch axis {
			case "candidate-id":
				expectedID = "synthetic-candidate-other"
			case "evidence-id":
				expectedEvidence = "synthetic-reference-other"
			case "reference-target":
				expectedRef = "evidence-b"
			case "subject":
				expectedSubject = "synthetic-subject-other"
			case "unit-binding":
				expectedBinding = "synthetic-binding-other"
			case "model":
				expectedModel = "synthetic-token-other"
			case "firmware":
				expectedFirmware = "synthetic-token-other"
			case "map-document":
				expectedDoc = "synthetic-map-other"
			case "map-revision":
				expectedRevision = "synthetic-map-other"
			case "map-applicability":
				expectedApplicability = "synthetic-map-other"
			}
			family, familyOK := parsed.Family().Value()
			model, modelOK := parsed.Model().Value()
			firmware, firmwareOK := parsed.Firmware().Value()
			unit, unitOK := parsed.Unit().Value()
			mapping, mapOK := parsed.Map().Value()
			if !familyOK || !modelOK || !firmwareOK || !unitOK || !mapOK || family != "SUN2000" || parsed.ID() != expectedID || s.Evidence()[1].ID() != expectedEvidence || parsed.Model().Refs()[0] != expectedRef || s.Subject() != expectedSubject || unit.Context() != expectedSubject || unit.Binding() != expectedBinding || model != expectedModel || firmware != expectedFirmware || mapping.Document() != expectedDoc || mapping.Revision() != expectedRevision || mapping.Applicability() != expectedApplicability {
				t.Fatal("identity axis lost or inherited")
			}
		})
	}
}

func TestSun2000EvidenceInputResourceAndEncoding(t *testing.T) {
	deep := []byte(strings.Repeat("[", 66) + strings.Repeat("]", 66))
	if _, err := ParseSun2000EvidenceInput(deep, Sun2000EvidenceInputLimits{65536, 256}); err == nil || err.Error() != "sun2000 evidence input: JSON nesting exceeded" {
		t.Fatal("nesting resource guard absent")
	}
	data := []byte(`{"contract":"sun2000-detection-input-v1","subject":"synthetic-` + string([]byte{0xff}) + `","evidence":[],"candidates":[]}`)
	if _, err := ParseSun2000EvidenceInput(data, Sun2000EvidenceInputLimits{65536, 256}); err == nil {
		t.Fatal("invalid UTF-8 normalized into accepted identity")
	}
}
