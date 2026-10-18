package modbusreg

import (
	"errors"
	"strings"
)

// OutBackWordSpan uses zero-based word coordinates relative to the raw record,
// or relative to an occurrence payload when nested inside an occurrence.
// Missing spans describe absent words; they never allocate or synthesize them.
type OutBackWordSpan struct {
	Offset, WordCount uint64
}

// OutBackSourceAssociation is caller-supplied evidence metadata, not a verified
// source or qualification claim. Empty strings explicitly retain unavailable
// metadata. DeclaredLayout is an opaque declaration, never selected by geometry.
type OutBackSourceAssociation struct {
	Key, Revision, PublicLocator, ContentIdentity     string
	DeclaredLayout, SourceSetRevision, SelectorInputs string
	Available                                         bool
}

// OutBackDetachedOccurrence retains payload words and diagnostic extents in
// caller order, including duplicate IDs and incomplete or malformed extents.
// Offset is the payload's logical word offset within the record, not a request.
type OutBackDetachedOccurrence struct {
	Ordinal          uint32
	WireKey          SunSpecWireKey
	Offset           uint64
	Words            []uint16
	Present, Missing []OutBackWordSpan
}

// OutBackDetachedRecordInput describes opaque words supplied by the caller.
// It contains no transport coordinates, inferred reads or typed decoding.
type OutBackDetachedRecordInput struct {
	Words            []uint16
	DeclaredWords    uint64
	Present, Missing []OutBackWordSpan
	Occurrences      []OutBackDetachedOccurrence
	Source           OutBackSourceAssociation
}

// OutBackDetachedRecord is immutable after successful construction. It is one
// diagnostic record, not a retention store, provider or admission mechanism.
// Its zero value is incomplete, historical and NO_GO.
type OutBackDetachedRecord struct {
	input          OutBackDetachedRecordInput
	accountedBytes uint64
	valid          bool
}

var ErrOutBackDetachedRecordBound = errors.New("OutBack detached record: positive byte bound exceeded or absent")

// NewOutBackDetachedRecord checks the entire record against maxBytes before
// copying anything. The positive finite bound counts deterministic encoded
// evidence bytes (including lengths and metadata), not Go heap overhead.
// Failure returns nil and leaves all caller evidence untouched.
func NewOutBackDetachedRecord(input OutBackDetachedRecordInput, maxBytes uint64) (*OutBackDetachedRecord, error) {
	if maxBytes == 0 {
		return nil, ErrOutBackDetachedRecordBound
	}
	remaining := maxBytes
	charge := func(count, width uint64) bool {
		if count > remaining/width {
			return false
		}
		remaining -= count * width
		return true
	}
	// Record: declared extent, four collection lengths; source: availability
	// and seven string lengths. Occurrence: ordinal, wire key, offset and
	// three collection lengths. Spans contain two uint64 coordinates.
	if !charge(1, 97) || !charge(uint64(len(input.Words)), 2) || !charge(uint64(len(input.Present)), 16) || !charge(uint64(len(input.Missing)), 16) || !charge(uint64(len(input.Occurrences)), 40) {
		return nil, ErrOutBackDetachedRecordBound
	}
	for _, s := range []string{input.Source.Key, input.Source.Revision, input.Source.PublicLocator, input.Source.ContentIdentity, input.Source.DeclaredLayout, input.Source.SourceSetRevision, input.Source.SelectorInputs} {
		if !charge(uint64(len(s)), 1) {
			return nil, ErrOutBackDetachedRecordBound
		}
	}
	for _, o := range input.Occurrences {
		if !charge(uint64(len(o.Words)), 2) || !charge(uint64(len(o.Present)), 16) || !charge(uint64(len(o.Missing)), 16) {
			return nil, ErrOutBackDetachedRecordBound
		}
	}
	input.Words = append([]uint16(nil), input.Words...)
	input.Present = append([]OutBackWordSpan(nil), input.Present...)
	input.Missing = append([]OutBackWordSpan(nil), input.Missing...)
	input.Occurrences = cloneOutBackOccurrences(input.Occurrences)
	// Detach string storage too, so short substrings cannot retain an
	// unrelated large caller buffer beyond the accounted evidence bytes.
	input.Source.Key = strings.Clone(input.Source.Key)
	input.Source.Revision = strings.Clone(input.Source.Revision)
	input.Source.PublicLocator = strings.Clone(input.Source.PublicLocator)
	input.Source.ContentIdentity = strings.Clone(input.Source.ContentIdentity)
	input.Source.DeclaredLayout = strings.Clone(input.Source.DeclaredLayout)
	input.Source.SourceSetRevision = strings.Clone(input.Source.SourceSetRevision)
	input.Source.SelectorInputs = strings.Clone(input.Source.SelectorInputs)

	return &OutBackDetachedRecord{input: input, accountedBytes: maxBytes - remaining, valid: true}, nil
}

func cloneOutBackOccurrences(in []OutBackDetachedOccurrence) []OutBackDetachedOccurrence {
	if in == nil {
		return nil
	}
	out := make([]OutBackDetachedOccurrence, len(in))
	for i, o := range in {
		o.Words = append([]uint16(nil), o.Words...)
		o.Present = append([]OutBackWordSpan(nil), o.Present...)
		o.Missing = append([]OutBackWordSpan(nil), o.Missing...)
		out[i] = o
	}
	return out
}

func (r OutBackDetachedRecord) RawWords() []uint16    { return append([]uint16(nil), r.input.Words...) }
func (r OutBackDetachedRecord) DeclaredWords() uint64 { return r.input.DeclaredWords }
func (r OutBackDetachedRecord) PresentSpans() []OutBackWordSpan {
	return append([]OutBackWordSpan(nil), r.input.Present...)
}
func (r OutBackDetachedRecord) MissingSpans() []OutBackWordSpan {
	return append([]OutBackWordSpan(nil), r.input.Missing...)
}
func (r OutBackDetachedRecord) Occurrences() []OutBackDetachedOccurrence {
	return cloneOutBackOccurrences(r.input.Occurrences)
}
func (r OutBackDetachedRecord) SourceAssociation() OutBackSourceAssociation { return r.input.Source }
func (r OutBackDetachedRecord) AccountedBytes() uint64                      { return r.accountedBytes }

// Complete means only that supplied extents have contiguous ordered coverage.
// Invalid, gapped, overlapping, empty and missing extents remain diagnostic.
// This does not assert that words match a source, layout or wire response.
func (r OutBackDetachedRecord) Complete() bool {
	if !r.valid || r.input.DeclaredWords != uint64(len(r.input.Words)) || len(r.input.Missing) != 0 || !outBackCovered(r.input.Present, r.input.DeclaredWords) {
		return false
	}
	for _, o := range r.input.Occurrences {
		n := uint64(len(o.Words))
		if n != uint64(o.WireKey.ModelLength) || len(o.Missing) != 0 || !outBackCovered(o.Present, n) || o.Offset > r.input.DeclaredWords || n > r.input.DeclaredWords-o.Offset {
			return false
		}
	}
	return true
}

func outBackCovered(spans []OutBackWordSpan, n uint64) bool {
	if n == 0 {
		return false
	}
	var end uint64
	for _, s := range spans {
		if s.Offset != end || s.WordCount == 0 || s.WordCount > n-end {
			return false
		}
		end += s.WordCount
	}
	return end == n
}

type OutBackAssociationDisposition string

const (
	OutBackAssociationCurrent    OutBackAssociationDisposition = "current"
	OutBackAssociationHistorical OutBackAssociationDisposition = "historical"
)

// Association is a pure comparison with the caller's current descriptor. It
// never relabels retained evidence. Unavailable sources and zero records are
// historical even when their metadata matches; current never implies admission.
func (r OutBackDetachedRecord) Association(current OutBackSourceAssociation) OutBackAssociationDisposition {
	original := r.input.Source
	if r.valid && original.Available && current.Available &&
		original.Key == current.Key && original.Revision == current.Revision &&
		original.ContentIdentity == current.ContentIdentity && original.DeclaredLayout == current.DeclaredLayout &&
		original.PublicLocator == current.PublicLocator && original.SourceSetRevision == current.SourceSetRevision &&
		original.SelectorInputs == current.SelectorInputs {
		return OutBackAssociationCurrent
	}
	return OutBackAssociationHistorical
}

// Evaluate is unconditionally NO_GO, including for nil and zero records. Raw
// completeness and association cannot admit a layout or qualify an operation.
func (*OutBackDetachedRecord) Evaluate() OutBackAXSReadOnlyFlavorDecision {
	return OutBackAXSReadOnlyFlavorDecision{reason: OutBackAXSReadOnlyFlavorReasonSourceNotQualified}
}

func (*OutBackDetachedRecord) TypedFacts() []SunSpecDecodedModel { return nil }
func (*OutBackDetachedRecord) Requests() []SunSpecReadRequest    { return nil }
