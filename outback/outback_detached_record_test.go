package outback

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func detachedInput() DetachedRecordInput {
	return DetachedRecordInput{
		Words: []uint16{11, 22, 33}, DeclaredWords: 5,
		Present: []WordSpan{{0, 3}}, Missing: []WordSpan{{3, 2}},
		Occurrences: []DetachedOccurrence{
			{Ordinal: 7, WireKey: modbusreg.SunSpecWireKey{ModelID: 64110, ModelLength: 282}, Offset: 0, Words: []uint16{11, 22}, Present: []WordSpan{{0, 2}}, Missing: []WordSpan{{2, 280}}},
			{Ordinal: 9, WireKey: modbusreg.SunSpecWireKey{ModelID: 64110, ModelLength: 420}, Offset: 2, Words: []uint16{33}, Present: []WordSpan{{0, 1}}, Missing: []WordSpan{{1, 419}}},
		},
		Source: SourceAssociation{Key: "synthetic-source", Revision: "R1", PublicLocator: "https://example.org/public-source", ContentIdentity: fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic source"))), DeclaredLayout: "candidate-A", Available: true, SourceSetRevision: "set-A", SelectorInputs: "unknown applicability"},
	}
}

func TestOutbackDetachedRecordCopies(t *testing.T) {
	input := detachedInput()
	want := detachedInput()
	r, err := NewDetachedRecord(input, 10000)
	if err != nil {
		t.Fatal(err)
	}
	input.Words[0] = 999
	input.Present[0].WordCount = 999
	input.Missing[0].WordCount = 999
	input.Occurrences[0].Words[0] = 999
	input.Occurrences[0].Present[0].WordCount = 999
	input.Occurrences[0].Missing[0].WordCount = 999
	input.Occurrences[1].Ordinal = 999
	input.Source.Key = "changed"
	assert := func() {
		t.Helper()
		if !reflect.DeepEqual(r.RawWords(), want.Words) || !reflect.DeepEqual(r.PresentSpans(), want.Present) || !reflect.DeepEqual(r.MissingSpans(), want.Missing) || !reflect.DeepEqual(r.Occurrences(), want.Occurrences) || r.SourceAssociation() != want.Source {
			t.Fatal("retained evidence changed or occurrences were reordered/deduplicated")
		}
	}
	assert()
	words := r.RawWords()
	words[1] = 888
	present := r.PresentSpans()
	present[0].Offset = 888
	missing := r.MissingSpans()
	missing[0].Offset = 888
	occ := r.Occurrences()
	occ[0].Words[1] = 888
	occ[0].Present[0].Offset = 888
	occ[0].Missing[0].Offset = 888
	occ[1].Ordinal = 888
	source := r.SourceAssociation()
	source.Revision = "changed"
	assert()
	if r.Evaluate().Reason() != AXSReadOnlyFlavorReasonSourceNotQualified || len(r.TypedFacts()) != 0 || len(r.Requests()) != 0 {
		t.Fatal("fragment admitted or emitted facts/requests")
	}
	if r.Complete() || r.DeclaredWords() != 5 || len(r.RawWords()) != 3 {
		t.Fatal("fragment padded or labelled complete")
	}
}

func TestOutbackDetachedRecordBounds(t *testing.T) {
	input := detachedInput()
	r, err := NewDetachedRecord(input, 10000)
	if err != nil {
		t.Fatal(err)
	}
	// Independent expected size for this fixture: 97 fixed record/source bytes,
	// 80 occurrence bytes, 12 word bytes and 96 span bytes, plus source text.
	wantBytes := uint64(285 + len(input.Source.Key) + len(input.Source.Revision) + len(input.Source.PublicLocator) + len(input.Source.ContentIdentity) + len(input.Source.DeclaredLayout) + len(input.Source.SourceSetRevision) + len(input.Source.SelectorInputs))
	if r.AccountedBytes() != wantBytes {
		t.Fatalf("accounted bytes %d, want %d", r.AccountedBytes(), wantBytes)
	}
	for _, bound := range []uint64{0, 1, r.AccountedBytes() - 1} {
		got, err := NewDetachedRecord(input, bound)
		if err == nil || got != nil {
			t.Fatalf("bound %d accepted partial record", bound)
		}
	}
	exact, err := NewDetachedRecord(input, r.AccountedBytes())
	if err != nil || exact == nil {
		t.Fatalf("exact bound rejected: %v", err)
	}
	for name, change := range map[string]func(*DetachedRecordInput){
		"raw":     func(i *DetachedRecordInput) { i.Words = append(i.Words, 1) },
		"present": func(i *DetachedRecordInput) { i.Present = append(i.Present, WordSpan{}) },
		"missing": func(i *DetachedRecordInput) { i.Missing = append(i.Missing, WordSpan{}) },
		"occurrence": func(i *DetachedRecordInput) {
			i.Occurrences = append(i.Occurrences, DetachedOccurrence{})
		},
		"nested words": func(i *DetachedRecordInput) { i.Occurrences[0].Words = append(i.Occurrences[0].Words, 1) },
		"nested present": func(i *DetachedRecordInput) {
			i.Occurrences[0].Present = append(i.Occurrences[0].Present, WordSpan{})
		},
		"nested missing": func(i *DetachedRecordInput) {
			i.Occurrences[0].Missing = append(i.Occurrences[0].Missing, WordSpan{})
		},
		"source metadata": func(i *DetachedRecordInput) { i.Source.PublicLocator += "x" },
	} {
		t.Run(name, func(t *testing.T) {
			i := detachedInput()
			change(&i)
			if got, err := NewDetachedRecord(i, r.AccountedBytes()); err == nil || got != nil {
				t.Fatal("unaccounted data accepted")
			}
		})
	}
}

func TestOutbackDetachedRecordAssociation(t *testing.T) {
	input := detachedInput()
	r, err := NewDetachedRecord(input, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if r.Association(input.Source) != AssociationCurrent {
		t.Fatal("unchanged association historical")
	}
	changes := map[string]func(*SourceAssociation){
		"key":      func(s *SourceAssociation) { s.Key = "other" },
		"revision": func(s *SourceAssociation) { s.Revision = "R2" },
		"content identity": func(s *SourceAssociation) {
			s.ContentIdentity = fmt.Sprintf("%x", sha256.Sum256([]byte("other synthetic source")))
		},
		"declared layout":     func(s *SourceAssociation) { s.DeclaredLayout = "candidate-B" },
		"availability":        func(s *SourceAssociation) { s.Available = false },
		"public locator":      func(s *SourceAssociation) { s.PublicLocator = "https://example.org/replacement" },
		"source set revision": func(s *SourceAssociation) { s.SourceSetRevision = "set-B" },
		"selector inputs":     func(s *SourceAssociation) { s.SelectorInputs = "changed applicability" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := input.Source
			change(&s)
			if r.Association(s) != AssociationHistorical {
				t.Fatal("changed association remains current")
			}
			i := detachedInput()
			i.Source = s
			other, err := NewDetachedRecord(i, 10000)
			if err != nil {
				t.Fatal(err)
			}
			if other.SourceAssociation() == r.SourceAssociation() || !reflect.DeepEqual(other.RawWords(), r.RawWords()) {
				t.Fatal("same bytes lost distinct source association")
			}
		})
	}
	unavailable := input
	unavailable.Source.Available = false
	u, err := NewDetachedRecord(unavailable, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if u.Association(input.Source) != AssociationHistorical {
		t.Fatal("original unavailable evidence current")
	}
	if u.Association(unavailable.Source) != AssociationHistorical {
		t.Fatal("unavailable evidence current")
	}
	var zero DetachedRecord
	if zero.Association(SourceAssociation{}) != AssociationHistorical {
		t.Fatal("zero record current")
	}
}

func TestOutbackDetachedRecordNoGo(t *testing.T) {
	for _, key := range []modbusreg.SunSpecWireKey{{ModelID: 64110, ModelLength: 282}, {ModelID: 64110, ModelLength: 420}, {ModelID: 64111, ModelLength: 23}, {ModelID: 64111, ModelLength: 26}} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			words := make([]uint16, key.ModelLength)
			for i := range words {
				words[i] = uint16(i + 1)
			}
			input := DetachedRecordInput{Words: words, DeclaredWords: uint64(len(words)), Present: []WordSpan{{0, uint64(len(words))}}, Source: detachedInput().Source, Occurrences: []DetachedOccurrence{{Ordinal: 1, WireKey: key, Words: words, Present: []WordSpan{{0, uint64(len(words))}}}}}
			r, err := NewDetachedRecord(input, 10000)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Complete() {
				t.Fatal("complete opaque geometry labelled incomplete")
			}
			check := func(r *DetachedRecord) {
				t.Helper()
				d := r.Evaluate()
				if d.Reason() != AXSReadOnlyFlavorReasonSourceNotQualified || d.Matched() || len(d.Chain()) != 0 {
					t.Fatal("geometry selected a layout")
				}
				if _, ok := d.InterfaceModel(); ok {
					t.Fatal("typed facts emitted")
				}
				if _, ok := d.InterfaceOccurrence(); ok {
					t.Fatal("opaque occurrence selected")
				}
				if len(r.TypedFacts()) != 0 || len(r.Requests()) != 0 {
					t.Fatal("facts or requests emitted")
				}
			}
			check(r)
			check(&DetachedRecord{})
			var absent *DetachedRecord
			check(absent)
		})
	}
}

func TestOutbackDetachedRecordCompleteness(t *testing.T) {
	base := func() DetachedRecordInput {
		return DetachedRecordInput{Words: []uint16{4, 5}, DeclaredWords: 2, Present: []WordSpan{{0, 1}, {1, 1}}, Occurrences: []DetachedOccurrence{{WireKey: modbusreg.SunSpecWireKey{ModelID: 64111, ModelLength: 2}, Words: []uint16{4, 5}, Present: []WordSpan{{0, 2}}}}}
	}
	for name, change := range map[string]func(*DetachedRecordInput){
		"declared extent":    func(i *DetachedRecordInput) { i.DeclaredWords = 3 },
		"missing":            func(i *DetachedRecordInput) { i.Missing = []WordSpan{{1, 1}} },
		"gap":                func(i *DetachedRecordInput) { i.Present = []WordSpan{{0, 1}} },
		"overlap":            func(i *DetachedRecordInput) { i.Present = []WordSpan{{0, 1}, {0, 1}} },
		"overflow":           func(i *DetachedRecordInput) { i.Present = []WordSpan{{^uint64(0), 2}} },
		"occurrence extent":  func(i *DetachedRecordInput) { i.Occurrences[0].WireKey.ModelLength = 3 },
		"occurrence missing": func(i *DetachedRecordInput) { i.Occurrences[0].Missing = []WordSpan{{1, 1}} },
		"occurrence gap":     func(i *DetachedRecordInput) { i.Occurrences[0].Present = nil },
		"occurrence offset":  func(i *DetachedRecordInput) { i.Occurrences[0].Offset = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			i := base()
			change(&i)
			r, err := NewDetachedRecord(i, 10000)
			if err != nil {
				t.Fatal(err)
			}
			if r.Complete() {
				t.Fatal("diagnostic malformed extent labelled complete")
			}
			if !reflect.DeepEqual(r.RawWords(), i.Words) || !reflect.DeepEqual(r.Occurrences(), i.Occurrences) {
				t.Fatal("diagnostic data rewritten")
			}
		})
	}
}
