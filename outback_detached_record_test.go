package modbusreg

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"
)

func detachedInput() OutBackDetachedRecordInput {
	return OutBackDetachedRecordInput{
		Words: []uint16{11, 22, 33}, DeclaredWords: 5,
		Present: []OutBackWordSpan{{0, 3}}, Missing: []OutBackWordSpan{{3, 2}},
		Occurrences: []OutBackDetachedOccurrence{
			{Ordinal: 7, WireKey: SunSpecWireKey{64110, 282}, Offset: 0, Words: []uint16{11, 22}, Present: []OutBackWordSpan{{0, 2}}, Missing: []OutBackWordSpan{{2, 280}}},
			{Ordinal: 9, WireKey: SunSpecWireKey{64110, 420}, Offset: 2, Words: []uint16{33}, Present: []OutBackWordSpan{{0, 1}}, Missing: []OutBackWordSpan{{1, 419}}},
		},
		Source: OutBackSourceAssociation{Key: "synthetic-source", Revision: "R1", PublicLocator: "https://example.org/public-source", ContentIdentity: fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic source"))), DeclaredLayout: "candidate-A", Available: true, SourceSetRevision: "set-A", SelectorInputs: "unknown applicability"},
	}
}

func TestOutbackDetachedRecordCopies(t *testing.T) {
	input := detachedInput()
	want := detachedInput()
	r, err := NewOutBackDetachedRecord(input, 10000)
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
	if r.Evaluate().Reason() != OutBackAXSReadOnlyFlavorReasonSourceNotQualified || len(r.TypedFacts()) != 0 || len(r.Requests()) != 0 {
		t.Fatal("fragment admitted or emitted facts/requests")
	}
	if r.Complete() || r.DeclaredWords() != 5 || len(r.RawWords()) != 3 {
		t.Fatal("fragment padded or labelled complete")
	}
}

func TestOutbackDetachedRecordBounds(t *testing.T) {
	input := detachedInput()
	r, err := NewOutBackDetachedRecord(input, 10000)
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
		got, err := NewOutBackDetachedRecord(input, bound)
		if err == nil || got != nil {
			t.Fatalf("bound %d accepted partial record", bound)
		}
	}
	exact, err := NewOutBackDetachedRecord(input, r.AccountedBytes())
	if err != nil || exact == nil {
		t.Fatalf("exact bound rejected: %v", err)
	}
	for name, change := range map[string]func(*OutBackDetachedRecordInput){
		"raw":     func(i *OutBackDetachedRecordInput) { i.Words = append(i.Words, 1) },
		"present": func(i *OutBackDetachedRecordInput) { i.Present = append(i.Present, OutBackWordSpan{}) },
		"missing": func(i *OutBackDetachedRecordInput) { i.Missing = append(i.Missing, OutBackWordSpan{}) },
		"occurrence": func(i *OutBackDetachedRecordInput) {
			i.Occurrences = append(i.Occurrences, OutBackDetachedOccurrence{})
		},
		"nested words": func(i *OutBackDetachedRecordInput) { i.Occurrences[0].Words = append(i.Occurrences[0].Words, 1) },
		"nested present": func(i *OutBackDetachedRecordInput) {
			i.Occurrences[0].Present = append(i.Occurrences[0].Present, OutBackWordSpan{})
		},
		"nested missing": func(i *OutBackDetachedRecordInput) {
			i.Occurrences[0].Missing = append(i.Occurrences[0].Missing, OutBackWordSpan{})
		},
		"source metadata": func(i *OutBackDetachedRecordInput) { i.Source.PublicLocator += "x" },
	} {
		t.Run(name, func(t *testing.T) {
			i := detachedInput()
			change(&i)
			if got, err := NewOutBackDetachedRecord(i, r.AccountedBytes()); err == nil || got != nil {
				t.Fatal("unaccounted data accepted")
			}
		})
	}
}

func TestOutbackDetachedRecordAssociation(t *testing.T) {
	input := detachedInput()
	r, err := NewOutBackDetachedRecord(input, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if r.Association(input.Source) != OutBackAssociationCurrent {
		t.Fatal("unchanged association historical")
	}
	changes := map[string]func(*OutBackSourceAssociation){
		"key":      func(s *OutBackSourceAssociation) { s.Key = "other" },
		"revision": func(s *OutBackSourceAssociation) { s.Revision = "R2" },
		"content identity": func(s *OutBackSourceAssociation) {
			s.ContentIdentity = fmt.Sprintf("%x", sha256.Sum256([]byte("other synthetic source")))
		},
		"declared layout":     func(s *OutBackSourceAssociation) { s.DeclaredLayout = "candidate-B" },
		"availability":        func(s *OutBackSourceAssociation) { s.Available = false },
		"public locator":      func(s *OutBackSourceAssociation) { s.PublicLocator = "https://example.org/replacement" },
		"source set revision": func(s *OutBackSourceAssociation) { s.SourceSetRevision = "set-B" },
		"selector inputs":     func(s *OutBackSourceAssociation) { s.SelectorInputs = "changed applicability" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := input.Source
			change(&s)
			if r.Association(s) != OutBackAssociationHistorical {
				t.Fatal("changed association remains current")
			}
			i := detachedInput()
			i.Source = s
			other, err := NewOutBackDetachedRecord(i, 10000)
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
	u, err := NewOutBackDetachedRecord(unavailable, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if u.Association(input.Source) != OutBackAssociationHistorical {
		t.Fatal("original unavailable evidence current")
	}
	if u.Association(unavailable.Source) != OutBackAssociationHistorical {
		t.Fatal("unavailable evidence current")
	}
	var zero OutBackDetachedRecord
	if zero.Association(OutBackSourceAssociation{}) != OutBackAssociationHistorical {
		t.Fatal("zero record current")
	}
}

func TestOutbackDetachedRecordNoGo(t *testing.T) {
	for _, key := range []SunSpecWireKey{{64110, 282}, {64110, 420}, {64111, 23}, {64111, 26}} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			words := make([]uint16, key.ModelLength)
			for i := range words {
				words[i] = uint16(i + 1)
			}
			input := OutBackDetachedRecordInput{Words: words, DeclaredWords: uint64(len(words)), Present: []OutBackWordSpan{{0, uint64(len(words))}}, Source: detachedInput().Source, Occurrences: []OutBackDetachedOccurrence{{Ordinal: 1, WireKey: key, Words: words, Present: []OutBackWordSpan{{0, uint64(len(words))}}}}}
			r, err := NewOutBackDetachedRecord(input, 10000)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Complete() {
				t.Fatal("complete opaque geometry labelled incomplete")
			}
			check := func(r *OutBackDetachedRecord) {
				t.Helper()
				d := r.Evaluate()
				if d.Reason() != OutBackAXSReadOnlyFlavorReasonSourceNotQualified || d.Matched() || len(d.Chain()) != 0 {
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
			check(&OutBackDetachedRecord{})
			var absent *OutBackDetachedRecord
			check(absent)
		})
	}
}

func TestOutbackDetachedRecordCompleteness(t *testing.T) {
	base := func() OutBackDetachedRecordInput {
		return OutBackDetachedRecordInput{Words: []uint16{4, 5}, DeclaredWords: 2, Present: []OutBackWordSpan{{0, 1}, {1, 1}}, Occurrences: []OutBackDetachedOccurrence{{WireKey: SunSpecWireKey{64111, 2}, Words: []uint16{4, 5}, Present: []OutBackWordSpan{{0, 2}}}}}
	}
	for name, change := range map[string]func(*OutBackDetachedRecordInput){
		"declared extent":    func(i *OutBackDetachedRecordInput) { i.DeclaredWords = 3 },
		"missing":            func(i *OutBackDetachedRecordInput) { i.Missing = []OutBackWordSpan{{1, 1}} },
		"gap":                func(i *OutBackDetachedRecordInput) { i.Present = []OutBackWordSpan{{0, 1}} },
		"overlap":            func(i *OutBackDetachedRecordInput) { i.Present = []OutBackWordSpan{{0, 1}, {0, 1}} },
		"overflow":           func(i *OutBackDetachedRecordInput) { i.Present = []OutBackWordSpan{{^uint64(0), 2}} },
		"occurrence extent":  func(i *OutBackDetachedRecordInput) { i.Occurrences[0].WireKey.ModelLength = 3 },
		"occurrence missing": func(i *OutBackDetachedRecordInput) { i.Occurrences[0].Missing = []OutBackWordSpan{{1, 1}} },
		"occurrence gap":     func(i *OutBackDetachedRecordInput) { i.Occurrences[0].Present = nil },
		"occurrence offset":  func(i *OutBackDetachedRecordInput) { i.Occurrences[0].Offset = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			i := base()
			change(&i)
			r, err := NewOutBackDetachedRecord(i, 10000)
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
