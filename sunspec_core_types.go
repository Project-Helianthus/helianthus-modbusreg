package modbusreg

import "fmt"

// SunSpecSchemaRevision names one pinned SunSpec model-definition revision,
// such as SunSpecModelsRevisionV1. Plans, decoder keys and registries compare
// it exactly; a plan accepts a non-empty value of at most 128 bytes.
type SunSpecSchemaRevision string

// SunSpecWireKey is the model identifier and declared model length exactly as
// read from a model header. ModelLength counts payload words and excludes the
// two header words.
type SunSpecWireKey struct{ ModelID, ModelLength uint16 }

// SunSpecDecoderKey selects one decoder definition: a wire key together with
// the schema revision it was admitted under. Decoding requires all three
// fields to agree with the occurrence and the registry.
type SunSpecDecoderKey struct {
	ModelID, ModelLength uint16
	SchemaRevision       SunSpecSchemaRevision
}

// SunSpecReadPurpose states which step of a chain walk a planned read serves.
// It routes the admitted view inside SunSpecChain; it never changes the
// function code, which is always FC03.
type SunSpecReadPurpose string

// Read purposes: a two-word signature probe at a base candidate, a two-word
// model header, and up to 125 payload words of the current model.
const (
	SunSpecReadSignature SunSpecReadPurpose = "signature"
	SunSpecReadHeader    SunSpecReadPurpose = "header"
	SunSpecReadPayload   SunSpecReadPurpose = "payload"
)

// SunSpecChainDisposition records how a plan classified one complete
// occurrence. Only an admitted occurrence carries a decoder key; every
// occurrence keeps its raw words and source spans.
type SunSpecChainDisposition string

// Chain dispositions. Admitted: the plan holds an exact decoder key for the
// wire key. UnsupportedLength: the plan knows the model identifier only at
// other lengths. UnknownModel: the plan does not know the model identifier.
// SunSpecChainDispositionUnknown is the same value as
// SunSpecChainDispositionUnknownModel, not a separate state.
const (
	SunSpecChainDispositionAdmitted          SunSpecChainDisposition = "admitted"
	SunSpecChainDispositionUnknownModel      SunSpecChainDisposition = "unknown_model"
	SunSpecChainDispositionUnsupportedLength SunSpecChainDisposition = "unsupported_length"
	SunSpecChainDispositionUnknown                                   = SunSpecChainDispositionUnknownModel
)

// SunSpecSourceSpan ties a run of occurrence words to the logical view that
// delivered them. LogicalViewID names a view retained in the same snapshot;
// WordCount is in 16-bit registers.
type SunSpecSourceSpan struct {
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

// SunSpecOccurrence is one complete model block of a walked chain: its header
// and payload words, their source spans and the plan's disposition. Ordinal is
// 1-based in chain order; HeaderOffset and PayloadOffset are zero-based PDU
// register addresses. The words and spans are private and returned as copies,
// so a value is safe to share.
type SunSpecOccurrence struct {
	Ordinal                     uint32
	WireKey                     SunSpecWireKey
	SchemaRevision              SunSpecSchemaRevision
	HeaderOffset, PayloadOffset uint16
	Disposition                 SunSpecChainDisposition
	decoderKey                  *SunSpecDecoderKey
	structuralCandidate         *sunSpecStructuralCandidate
	words                       []uint16
	spans                       []SunSpecSourceSpan
}

// ModelID returns the model identifier from the occurrence header.
func (o SunSpecOccurrence) ModelID() uint16 { return o.WireKey.ModelID }

// ModelLength returns the declared payload length in words, excluding the two
// header words.
func (o SunSpecOccurrence) ModelLength() uint16 { return o.WireKey.ModelLength }

// Words returns a copy of the occurrence words, beginning with the two header
// words.
func (o SunSpecOccurrence) Words() []uint16 { return append([]uint16(nil), o.words...) }

// SourceSpans returns a copy of the spans that delivered Words, in word order.
func (o SunSpecOccurrence) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), o.spans...)
}

// DecoderKey returns the exact decoder key the plan assigned and true, or
// false when the occurrence was not admitted.
func (o SunSpecOccurrence) DecoderKey() (SunSpecDecoderKey, bool) {
	if o.decoderKey == nil {
		return SunSpecDecoderKey{}, false
	}
	return *o.decoderKey, true
}

// SunSpecModelOccurrence is an alias of SunSpecOccurrence; both names denote
// the same type.
type SunSpecModelOccurrence = SunSpecOccurrence

// SunSpecChainSnapshot is the immutable result of a completed chain walk: the
// raw chain image, the complete occurrences in chain order and the logical
// views that delivered them. The zero value, which SunSpecChain.AdmitReplay
// returns while the walk continues, has no raw words. Accessors return copies.
type SunSpecChainSnapshot struct {
	occurrences []SunSpecOccurrence
	raw         []uint16
	sources     []LogicalViewRecord
}

func cloneOccurrence(o SunSpecOccurrence) SunSpecOccurrence {
	o.words = append([]uint16(nil), o.words...)
	o.spans = append([]SunSpecSourceSpan(nil), o.spans...)
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
func (s SunSpecChainSnapshot) Occurrences() []SunSpecOccurrence {
	out := make([]SunSpecOccurrence, len(s.occurrences))
	for i, o := range s.occurrences {
		out[i] = cloneOccurrence(o)
	}
	return out
}

// RawWords returns a copy of the raw chain image: the signature, every header
// and payload word, and the end marker, without gaps.
func (s SunSpecChainSnapshot) RawWords() []uint16 { return append([]uint16(nil), s.raw...) }

// SourceViews returns copies of every admitted logical view, signature probes
// included, in admission order.
func (s SunSpecChainSnapshot) SourceViews() []LogicalViewSnapshot {
	out := make([]LogicalViewSnapshot, len(s.sources))
	for i, source := range s.sources {
		out[i] = LogicalViewSnapshot{record: cloneSunSpecLogicalViewRecord(source), valid: true}
	}
	return out
}

// ByModelID returns copies of the occurrences with model identifier id in
// chain order; the slice is empty when there are none.
func (s SunSpecChainSnapshot) ByModelID(id uint16) []SunSpecOccurrence {
	out := []SunSpecOccurrence{}
	for _, o := range s.occurrences {
		if o.WireKey.ModelID == id {
			out = append(out, cloneOccurrence(o))
		}
	}
	return out
}
func cloneSunSpecLogicalViewRecord(record LogicalViewRecord) LogicalViewRecord {
	record.Words = append([]uint16(nil), record.Words...)
	record.WireResponseBytes = append([]byte(nil), record.WireResponseBytes...)
	return record
}
func validSunSpecRevision(v SunSpecSchemaRevision) bool { return len(v) > 0 && len(v) <= 128 }
func sunSpecEnd(address, words uint16) error {
	if words == 0 || uint32(address)+uint32(words) > 65536 {
		return fmt.Errorf("SunSpec read range exceeds address space")
	}
	return nil
}
