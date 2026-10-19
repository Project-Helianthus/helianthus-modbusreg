package modbusreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func i13Identity() I13Identity {
	return I13Identity{"synthetic-subject", "synthetic-candidate", "synthetic-evidence", "synthetic-source", 1}
}
func i13Bytes() []byte {
	return []byte(`{"contract":"sun2000-detection-input-v1","subject":"synthetic-subject","evidence":[{"id":"synthetic-evidence","source":"synthetic","locator":"synthetic record","context":"synthetic-subject","claim_level":"Hypothesis"}],"candidates":[{"id":"synthetic-candidate","family":{"state":"present","value":"SUN2000","refs":["synthetic-evidence"],"alternatives":[]},"model":{"state":"missing","value":null,"refs":[],"alternatives":[]},"firmware":{"state":"missing","value":null,"refs":[],"alternatives":[]},"unit":{"state":"missing","value":null,"refs":[],"alternatives":[]},"map":{"state":"missing","value":null,"refs":[],"alternatives":[]}}]}`)
}
func i13Limits() Sun2000EvidenceInputLimits { return Sun2000EvidenceInputLimits{4096, 32} }
func i13New(t *testing.T, supplier I13Supplier) *I13Provider {
	t.Helper()
	p, err := I13NewProvider(i13Identity(), i13Limits(), supplier)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func i13Denied(t *testing.T, result I13Result) {
	t.Helper()
	if result.I13Outcome != "NO_ADMISSIBLE_PROFILE" {
		t.Fatal("documentary input changed admission", result.I13Outcome)
	}
}

func TestI13Refusals(t *testing.T) {
	calls := 0
	supplier := func(context.Context, I13Request) (I13Evidence, error) { calls++; return I13Evidence{}, nil }
	for _, axis := range []string{"subject", "candidate", "evidence", "source", "generation"} {
		t.Run(axis, func(t *testing.T) {
			id := i13Identity()
			switch axis {
			case "subject":
				id.I13Subject = ""
			case "candidate":
				id.I13Candidate = ""
			case "evidence":
				id.I13Evidence = ""
			case "source":
				id.I13Source = ""
			case "generation":
				id.I13Generation = 0
			}
			if _, err := I13NewProvider(id, i13Limits(), supplier); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	for _, limits := range []Sun2000EvidenceInputLimits{{}, {-1, 32}, {4096, 0}} {
		if _, err := I13NewProvider(i13Identity(), limits, supplier); err == nil {
			t.Fatal("invalid bounds")
		}
	}
	if _, err := I13NewProvider(i13Identity(), i13Limits(), nil); err == nil {
		t.Fatal("nil supplier")
	}
	p := i13New(t, supplier)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx} {
		result, err := p.I13Cycle(ctx)
		i13Denied(t, result)
		if err == nil {
			t.Fatal("refusal accepted")
		}
	}
	p.I13Retire()
	if _, err := p.I13Cycle(context.Background()); err == nil {
		t.Fatal("retired cycle")
	}
	var zero I13Provider
	if _, err := zero.I13Cycle(context.Background()); err == nil {
		t.Fatal("zero provider")
	}
	if calls != 0 {
		t.Fatal("refusal called supplier")
	}
}

func TestI13IdentityJoins(t *testing.T) {
	for _, axis := range []string{"subject", "candidate", "evidence", "source", "generation", "receipt", "native-subject", "native-candidate", "native-evidence", "candidate-reference"} {
		t.Run(axis, func(t *testing.T) {
			p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
				e := I13Evidence{r, i13Bytes()}
				switch axis {
				case "subject":
					e.I13Request.I13Identity.I13Subject = "other"
				case "candidate":
					e.I13Request.I13Identity.I13Candidate = "other"
				case "evidence":
					e.I13Request.I13Identity.I13Evidence = "other"
				case "source":
					e.I13Request.I13Identity.I13Source = "other"
				case "generation":
					e.I13Request.I13Identity.I13Generation = 2
				case "receipt":
					e.I13Request.I13Receipt = 99
				case "native-subject":
					e.I13Bytes = bytes.ReplaceAll(e.I13Bytes, []byte("synthetic-subject"), []byte("other"))
				case "native-candidate":
					e.I13Bytes = bytes.ReplaceAll(e.I13Bytes, []byte("synthetic-candidate"), []byte("other"))
				case "native-evidence":
					e.I13Bytes = bytes.ReplaceAll(e.I13Bytes, []byte("synthetic-evidence"), []byte("other"))
				case "candidate-reference":
					e.I13Bytes = bytes.Replace(e.I13Bytes, []byte(`"state":"present","value":"SUN2000","refs":["synthetic-evidence"]`), []byte(`"state":"missing","value":null,"refs":[]`), 1)
				}
				return e, nil
			})
			result, err := p.I13Cycle(context.Background())
			i13Denied(t, result)
			if err == nil || result.I13Current != nil || result.I13Retained != nil {
				t.Fatal("identity join accepted")
			}
		})
	}
}

func TestI13RetentionAndDetachment(t *testing.T) {
	original := i13Bytes()
	supplied := append([]byte(nil), original...)
	calls := 0
	p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
		calls++
		if calls == 1 {
			return I13Evidence{r, supplied}, nil
		}
		return I13Evidence{r, []byte("{")}, nil
	})
	var port I13Port = p
	first, err := port.I13Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i13Denied(t, first)
	if first.I13Current == nil || first.I13Retained != nil || first.I13Current.I13Request.I13Receipt != 1 {
		t.Fatal("first receipt")
	}
	supplied[0] = 'x'
	first.I13Current.I13Bytes[1] = 'x'
	first.I13Current.I13Input.candidates[0].family.refs[0] = "changed"
	first.I13Current.I13Input.evidence[0] = Sun2000EvidenceReference{}
	second, err := port.I13Cycle(context.Background())
	i13Denied(t, second)
	if err == nil || second.I13Current != nil || second.I13Rejected == nil || second.I13Rejected.I13Receipt != 2 || second.I13Retained == nil || second.I13Retained.I13Request.I13Receipt != 1 || !bytes.Equal(second.I13Retained.I13Bytes, original) || second.I13Retained.I13Input.Evidence()[0].ID() != "synthetic-evidence" || second.I13Retained.I13Input.Candidates()[0].Family().Refs()[0] != "synthetic-evidence" {
		t.Fatal("retention or detachment")
	}
	second.I13Retained.I13Bytes[0] = 'x'
	second.I13Retained.I13Input.candidates[0].family.refs[0] = "changed"
	third, _ := port.I13Cycle(context.Background())
	if third.I13Rejected.I13Receipt != 3 || third.I13Retained.I13Request.I13Receipt != 1 || !bytes.Equal(third.I13Retained.I13Bytes, original) || third.I13Retained.I13Input.Candidates()[0].Family().Refs()[0] != "synthetic-evidence" {
		t.Fatal("retained record redated or aliased")
	}
}

func TestI13BoundsAndMalformed(t *testing.T) {
	for _, axis := range []string{"bytes", "records", "syntax", "dangling", "supplier"} {
		t.Run(axis, func(t *testing.T) {
			data := i13Bytes()
			limits := i13Limits()
			if axis == "bytes" {
				limits.MaxBytes = len(data) - 1
			}
			if axis == "records" {
				limits.MaxRecords = 7
			}
			if axis == "syntax" {
				data = []byte("{")
			}
			if axis == "dangling" {
				data = bytes.Replace(data, []byte(`"refs":["synthetic-evidence"]`), []byte(`"refs":["missing"]`), 1)
			}
			p, err := I13NewProvider(i13Identity(), limits, func(_ context.Context, r I13Request) (I13Evidence, error) {
				if axis == "supplier" {
					return I13Evidence{}, errors.New("unsafe supplier detail")
				}
				return I13Evidence{r, data}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.I13Cycle(context.Background())
			i13Denied(t, result)
			if err == nil || strings.Contains(err.Error(), "unsafe") || result.I13Current != nil {
				t.Fatal("invalid input published")
			}
		})
	}
	// Eight objects: envelope, evidence, candidate and five wrappers.
	p, err := I13NewProvider(i13Identity(), Sun2000EvidenceInputLimits{len(i13Bytes()), 8}, func(_ context.Context, r I13Request) (I13Evidence, error) { return I13Evidence{r, i13Bytes()}, nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.I13Cycle(context.Background())
	if err != nil || result.I13Current == nil {
		t.Fatal("exact bounds", err)
	}
	i13Denied(t, result)
}

func TestI13Fences(t *testing.T) {
	for _, axis := range []string{"cancel", "retire", "generation", "source"} {
		t.Run(axis, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var p *I13Provider
			p = i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
				switch axis {
				case "cancel":
					cancel()
				case "retire":
					p.I13Retire()
				case "generation", "source":
					id := i13Identity()
					id.I13Generation = 2
					if axis == "source" {
						id.I13Source = "synthetic-replacement"
					}
					if err := p.I13Replace(id); err != nil {
						t.Fatal(err)
					}
				}
				return I13Evidence{r, i13Bytes()}, nil
			})
			result, err := p.I13Cycle(ctx)
			i13Denied(t, result)
			if err == nil || result.I13Current != nil || result.I13Retained != nil {
				t.Fatal("stale callback published")
			}
		})
	}
}

func TestI13Replacement(t *testing.T) {
	calls := 0
	p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
		calls++
		return I13Evidence{r, i13Bytes()}, nil
	})
	if _, err := p.I13Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := i13Identity()
	if err := p.I13Replace(id); err == nil {
		t.Fatal("same generation accepted")
	}
	id.I13Generation = 2
	id.I13Source = "synthetic-replacement"
	if err := p.I13Replace(id); err != nil {
		t.Fatal(err)
	}
	result, err := p.I13Cycle(context.Background())
	if err != nil || result.I13Retained != nil || result.I13Current.I13Request.I13Identity != id || calls != 2 {
		t.Fatal("replacement inherited prior source")
	}
	i13Denied(t, result)
}

func TestI13DocumentaryFields(t *testing.T) {
	for _, axis := range []string{"control", "model", "firmware", "unit-context", "unit-binding", "map-document", "map-revision", "map-applicability"} {
		t.Run(axis, func(t *testing.T) {
			var envelope map[string]any
			if err := json.Unmarshal(i13Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			candidate := envelope["candidates"].([]any)[0].(map[string]any)
			model, firmware, binding, document, revision, applicability := "synthetic-model", "synthetic-firmware", "synthetic-binding", "synthetic-document", "synthetic-revision", "synthetic-applicability"
			switch axis {
			case "model":
				model = "other-model"
			case "firmware":
				firmware = "other-firmware"
			case "unit-binding":
				binding = "other-binding"
			case "map-document":
				document = "other-document"
			case "map-revision":
				revision = "other-revision"
			case "map-applicability":
				applicability = "other-applicability"
			}
			unitContext := "synthetic-subject"
			if axis == "unit-context" {
				unitContext = "other-subject"
			}
			for field, value := range map[string]any{"model": model, "firmware": firmware, "unit": map[string]any{"context": unitContext, "binding": binding}, "map": map[string]any{"document": document, "revision": revision, "applicability": applicability}} {
				candidate[field] = map[string]any{"state": "present", "value": value, "refs": []string{"synthetic-evidence"}, "alternatives": []any{}}
			}
			data, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) { return I13Evidence{r, data}, nil })
			result, err := p.I13Cycle(context.Background())
			i13Denied(t, result)
			if axis == "unit-context" {
				if err == nil || result.I13Current != nil {
					t.Fatal("native subject validation weakened")
				}
				return
			}
			if err != nil || result.I13Current == nil {
				t.Fatal(err)
			}
			c := result.I13Current.I13Input.Candidates()[0]
			m, _ := c.Model().Value()
			f, _ := c.Firmware().Value()
			u, _ := c.Unit().Value()
			mapping, _ := c.Map().Value()
			if m != model || f != firmware || u.Context() != "synthetic-subject" || u.Binding() != binding || mapping.Document() != document || mapping.Revision() != revision || mapping.Applicability() != applicability || !bytes.Equal(result.I13Current.I13Bytes, data) {
				t.Fatal("native documentary field lost")
			}
		})
	}
}

func TestI13CancelledRetention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return I13Evidence{r, i13Bytes()}, nil
	})
	if _, err := p.I13Cycle(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := p.I13Cycle(ctx)
	i13Denied(t, result)
	if !errors.Is(err, context.Canceled) || result.I13Current != nil || result.I13Retained == nil || result.I13Retained.I13Request.I13Receipt != 1 || result.I13Rejected.I13Receipt != 2 {
		t.Fatal("cancelled attempt replaced retained receipt")
	}
}

func TestI13BusyRefusal(t *testing.T) {
	calls := 0
	var p *I13Provider
	p = i13New(t, func(ctx context.Context, r I13Request) (I13Evidence, error) {
		calls++
		if calls > 1 {
			return I13Evidence{}, errors.New("unexpected reentry")
		}
		result, err := p.I13Cycle(ctx)
		i13Denied(t, result)
		if err == nil {
			t.Fatal("reentrant cycle accepted")
		}
		return I13Evidence{r, i13Bytes()}, nil
	})
	if _, err := p.I13Cycle(context.Background()); err != nil || calls != 1 {
		t.Fatal("busy refusal called supplier", err, calls)
	}
}

func TestI13ReplacementFailureClearsSource(t *testing.T) {
	for _, axis := range []string{"generation", "source"} {
		t.Run(axis, func(t *testing.T) {
			calls := 0
			p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
				calls++
				if calls == 1 {
					return I13Evidence{r, i13Bytes()}, nil
				}
				return I13Evidence{r, []byte("{")}, nil
			})
			if _, err := p.I13Cycle(context.Background()); err != nil {
				t.Fatal(err)
			}
			id := i13Identity()
			id.I13Generation = 2
			if axis == "source" {
				id.I13Source = "synthetic-replacement"
			}
			if err := p.I13Replace(id); err != nil {
				t.Fatal(err)
			}
			result, err := p.I13Cycle(context.Background())
			i13Denied(t, result)
			if err == nil || result.I13Retained != nil || result.I13Current != nil || result.I13Rejected.I13Identity != id || result.I13Rejected.I13Receipt != 2 {
				t.Fatal("replaced source retained old record")
			}
		})
	}
}

func TestI13AlternativesDetached(t *testing.T) {
	var envelope map[string]any
	if err := json.Unmarshal(i13Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	candidate := envelope["candidates"].([]any)[0].(map[string]any)
	candidate["family"] = map[string]any{"state": "contradictory", "value": nil, "refs": []string{"synthetic-evidence"}, "alternatives": []any{map[string]any{"value": "SUN2000", "refs": []string{"synthetic-evidence"}}, map[string]any{"value": "synthetic-other-family", "refs": []string{"synthetic-evidence"}}}}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	p := i13New(t, func(_ context.Context, r I13Request) (I13Evidence, error) {
		calls++
		if calls == 1 {
			return I13Evidence{r, data}, nil
		}
		return I13Evidence{}, errors.New("synthetic refusal")
	})
	result, err := p.I13Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i13Denied(t, result)
	result.I13Current.I13Input.candidates[0].family.alternatives[0].refs[0] = "changed"
	result.I13Current.I13Input.candidates[0].family.alternatives[1].value = "changed"
	retained, err := p.I13Cycle(context.Background())
	i13Denied(t, retained)
	if err == nil || retained.I13Retained == nil {
		t.Fatal("missing retained contradiction")
	}
	alts := retained.I13Retained.I13Input.Candidates()[0].Family().Alternatives()
	if alts[0].Value() != "SUN2000" || alts[0].Refs()[0] != "synthetic-evidence" || alts[1].Value() != "synthetic-other-family" {
		t.Fatal("nested alternatives aliased")
	}
}
