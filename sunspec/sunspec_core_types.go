package sunspec

import (
	"fmt"
	"slices"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// SchemaRevision names one pinned SunSpec model-definition revision,
// such as ModelsRevisionV1. Plans, decoder keys and registries compare
// it exactly; a plan accepts a non-empty value of at most 128 bytes.
type SchemaRevision string

// WireKey is the model identifier and declared model length exactly as
// read from a model header. ModelLength counts payload words and excludes the
// two header words.
type WireKey struct{ ModelID, ModelLength uint16 }

// DecoderKey selects one decoder definition: a wire key together with
// the schema revision it was admitted under. Decoding requires all three
// fields to agree with the occurrence and the registry.
type DecoderKey struct {
	ModelID, ModelLength uint16
	SchemaRevision       SchemaRevision
}

// ReadPurpose states which step of a chain walk a planned read serves.
// It routes the admitted view inside Chain; it never changes the
// function code, which is always FC03.
type ReadPurpose string

// Read purposes: a two-word signature probe at a base candidate, a two-word
// model header, and up to 125 payload words of the current model.
const (
	ReadSignature ReadPurpose = "signature"
	ReadHeader    ReadPurpose = "header"
	ReadPayload   ReadPurpose = "payload"
)

// ChainDisposition records how a plan classified one complete
// occurrence. Only an admitted occurrence carries a decoder key; every
// occurrence keeps its raw words and source spans.
type ChainDisposition string

// Chain dispositions. Admitted: the plan holds an exact decoder key for the
// wire key. UnsupportedLength: the plan knows the model identifier only at
// other lengths. UnknownModel: the plan does not know the model identifier.
// ChainDispositionUnknown is the same value as
// ChainDispositionUnknownModel, not a separate state.
const (
	ChainDispositionAdmitted          ChainDisposition = "admitted"
	ChainDispositionUnknownModel      ChainDisposition = "unknown_model"
	ChainDispositionUnsupportedLength ChainDisposition = "unsupported_length"
	ChainDispositionUnknown                            = ChainDispositionUnknownModel
)

// SourceSpan ties a run of occurrence words to the logical view that
// delivered them. LogicalViewID names a view retained in the same snapshot;
// WordCount is in 16-bit registers.
type SourceSpan struct {
	LogicalViewID uint64
	// PDUOffset is the zero-based word offset in the physical response retained
	// by the referenced logical view.
	PDUOffset, WordCount uint16
}

// sunSpecStructuralCandidate is private metadata for a complete occurrence
// whose opted-in structure has been validated. It intentionally carries no
// decoder, catalog, or public selection surface.
type sunSpecStructuralCandidate struct {
	modelID uint16
	layout  sunSpecNestedOccurrenceLayout
}

// Occurrence is one complete model block of a walked chain: its header
// and payload words, their source spans and the plan's disposition. Ordinal is
// 1-based in chain order; HeaderOffset and PayloadOffset are zero-based PDU
// register addresses. The words and spans are private and returned as copies,
// so a value is safe to share.
type Occurrence struct {
	Ordinal                     uint32
	WireKey                     WireKey
	SchemaRevision              SchemaRevision
	HeaderOffset, PayloadOffset uint16
	Disposition                 ChainDisposition
	decoderKey                  *DecoderKey
	structuralCandidate         *sunSpecStructuralCandidate
	words                       []uint16
	spans                       []SourceSpan
}

// ModelID returns the model identifier from the occurrence header.
func (o Occurrence) ModelID() uint16 { return o.WireKey.ModelID }

// ModelLength returns the declared payload length in words, excluding the two
// header words.
func (o Occurrence) ModelLength() uint16 { return o.WireKey.ModelLength }

// Words returns a copy of the occurrence words, beginning with the two header
// words.
func (o Occurrence) Words() []uint16 { return append([]uint16(nil), o.words...) }

// SourceSpans returns a copy of the spans that delivered Words, in word order.
func (o Occurrence) SourceSpans() []SourceSpan {
	return append([]SourceSpan(nil), o.spans...)
}

// DecoderKey returns the exact decoder key the plan assigned and true, or
// false when the occurrence was not admitted.
func (o Occurrence) DecoderKey() (DecoderKey, bool) {
	if o.decoderKey == nil {
		return DecoderKey{}, false
	}
	return *o.decoderKey, true
}

// ModelOccurrence is an alias of Occurrence; both names denote
// the same type.
type ModelOccurrence = Occurrence

// ChainSnapshot is the immutable result of a completed chain walk: the
// raw chain image, the complete occurrences in chain order and the logical
// views that delivered them. The zero value, which Chain.AdmitReplay
// returns while the walk continues, has no raw words. Accessors return copies.
// A snapshot assembled by NewChainSnapshot is checked only for valid
// source views; WireChain reports whether it is a consistent terminal chain.
type ChainSnapshot struct {
	occurrences []Occurrence
	raw         []uint16
	sources     []modbusreg.LogicalViewSnapshot
}

// NewChainSnapshot assembles a snapshot from copies of a raw chain
// image, occurrences in chain order and the source views that delivered them.
// Each source view is revalidated; an invalid one, such as the zero value,
// returns an error naming its index. The raw words and occurrences are not
// checked against each other or against the views: WireChain and every
// evaluation of the snapshot do that, and assembly grants no admission.
func NewChainSnapshot(raw []uint16, occurrences []Occurrence, sources []modbusreg.LogicalViewSnapshot) (ChainSnapshot, error) {
	snapshot := ChainSnapshot{raw: append([]uint16(nil), raw...), occurrences: make([]Occurrence, len(occurrences))}
	for index, occurrence := range occurrences {
		snapshot.occurrences[index] = cloneOccurrence(occurrence)
	}
	for index, source := range sources {
		validated, err := modbusreg.NewLogicalViewSnapshot(source.Record())
		if err != nil {
			return ChainSnapshot{}, fmt.Errorf("SunSpec chain snapshot source view %d: %w", index, err)
		}
		snapshot.sources = append(snapshot.sources, validated)
	}
	return snapshot, nil
}

// Clone returns a copy of the snapshot that shares no storage with it.
func (s ChainSnapshot) Clone() ChainSnapshot {
	cloned := ChainSnapshot{raw: s.RawWords(), occurrences: s.Occurrences()}
	for _, source := range s.sources {
		cloned.sources = append(cloned.sources, source.Clone())
	}
	return cloned
}

// RawWordCount returns the number of raw chain words without copying them.
func (s ChainSnapshot) RawWordCount() int { return len(s.raw) }

// OccurrenceCount returns the number of complete occurrences without copying
// them.
func (s ChainSnapshot) OccurrenceCount() int { return len(s.occurrences) }

// SourceViewCount returns the number of retained source views without copying
// them.
func (s ChainSnapshot) SourceViewCount() int { return len(s.sources) }

// WireChain returns the wire key of every occurrence in chain order followed by
// the end marker, and true, only when the snapshot is a consistent terminal
// chain: the raw image starts with the SunSpec signature, holds each
// occurrence's words at contiguous header offsets and ends with the end
// marker; every occurrence's source spans cover its words exactly from
// distinct retained views. Otherwise it returns nil and false.
func (s ChainSnapshot) WireChain() ([]WireKey, bool) {
	raw := s.RawWords()
	if len(raw) < 4 || raw[0] != sunSpecSignatureFirst || raw[1] != sunSpecSignatureSecond {
		return nil, false
	}
	occurrences := s.Occurrences()
	views := s.SourceViews()
	viewByID := make(map[uint64]modbusreg.LogicalViewRecord, len(views))
	for _, view := range views {
		record := view.Record()
		if record.LogicalViewID == 0 {
			return nil, false
		}
		if _, duplicate := viewByID[record.LogicalViewID]; duplicate {
			return nil, false
		}
		viewByID[record.LogicalViewID] = record
	}
	keys := make([]WireKey, 0, len(occurrences)+1)
	offset := 2
	var nextHeader uint32
	for index, occurrence := range occurrences {
		if occurrence.Ordinal != uint32(index+1) || offset+2 > len(raw) {
			return nil, false
		}
		key := WireKey{ModelID: raw[offset], ModelLength: raw[offset+1]}
		end := offset + 2 + int(key.ModelLength)
		if key.ModelID == EndModelID || key.ModelLength == 0 || end > len(raw) || key != occurrence.WireKey || !slices.Equal(raw[offset:end], occurrence.Words()) {
			return nil, false
		}
		if index == 0 {
			if occurrence.HeaderOffset < 2 {
				return nil, false
			}
			nextHeader = uint32(occurrence.HeaderOffset)
		}
		if uint32(occurrence.HeaderOffset) != nextHeader || uint32(occurrence.PayloadOffset) != nextHeader+2 {
			return nil, false
		}
		wordCursor := 0
		for _, span := range occurrence.SourceSpans() {
			record, ok := viewByID[span.LogicalViewID]
			spanEnd := wordCursor + int(span.WordCount)
			if !ok || span.WordCount == 0 || spanEnd > len(occurrence.words) || span.PDUOffset != record.SliceOffset || span.WordCount != record.SliceWordCount || len(record.Words) != int(span.WordCount) || !slices.Equal(record.Words, occurrence.words[wordCursor:spanEnd]) {
				return nil, false
			}
			wordCursor = spanEnd
		}
		if wordCursor != len(occurrence.words) {
			return nil, false
		}
		keys = append(keys, key)
		offset = end
		nextHeader += uint32(len(occurrence.words))
		if nextHeader > 65535 {
			return nil, false
		}
	}
	if offset+2 != len(raw) || raw[offset] != EndModelID || raw[offset+1] != 0 {
		return nil, false
	}
	return append(keys, WireKey{ModelID: EndModelID, ModelLength: 0}), true
}

func cloneOccurrence(o Occurrence) Occurrence {
	o.words = append([]uint16(nil), o.words...)
	o.spans = append([]SourceSpan(nil), o.spans...)
	if o.decoderKey != nil {
		k := *o.decoderKey
		o.decoderKey = &k
	}
	if o.structuralCandidate != nil {
		candidate := *o.structuralCandidate
		candidate.layout = cloneSunSpecNestedOccurrenceLayout(candidate.layout)
		o.structuralCandidate = &candidate
	}
	return o
}

// Occurrences returns copies of the complete occurrences in chain order. The
// end marker is not an occurrence.
func (s ChainSnapshot) Occurrences() []Occurrence {
	out := make([]Occurrence, len(s.occurrences))
	for i, o := range s.occurrences {
		out[i] = cloneOccurrence(o)
	}
	return out
}

// RawWords returns a copy of the raw chain image: the signature, every header
// and payload word, and the end marker, without gaps.
func (s ChainSnapshot) RawWords() []uint16 { return append([]uint16(nil), s.raw...) }

// SourceViews returns copies of every admitted logical view, signature probes
// included, in admission order.
func (s ChainSnapshot) SourceViews() []modbusreg.LogicalViewSnapshot {
	out := make([]modbusreg.LogicalViewSnapshot, len(s.sources))
	for i, source := range s.sources {
		out[i] = source.Clone()
	}
	return out
}

// ByModelID returns copies of the occurrences with model identifier id in
// chain order; the slice is empty when there are none.
func (s ChainSnapshot) ByModelID(id uint16) []Occurrence {
	out := []Occurrence{}
	for _, o := range s.occurrences {
		if o.WireKey.ModelID == id {
			out = append(out, cloneOccurrence(o))
		}
	}
	return out
}
func validSunSpecRevision(v SchemaRevision) bool { return len(v) > 0 && len(v) <= 128 }
func sunSpecEnd(address, words uint16) error {
	if words == 0 || uint32(address)+uint32(words) > 65536 {
		return fmt.Errorf("SunSpec read range exceeds address space")
	}
	return nil
}
