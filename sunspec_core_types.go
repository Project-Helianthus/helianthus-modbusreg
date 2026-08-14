package modbusreg

import "fmt"

// SunSpecSchemaRevision identifies an exact, caller-selected schema revision.
type SunSpecSchemaRevision string

// SunSpecDecoderKey is a decoder identity. R1 defines it but does not decode.
type SunSpecDecoderKey struct {
	ModelID, ModelLength uint16
	SchemaRevision       SunSpecSchemaRevision
}

// SunSpecReadPurpose describes the grammar item requested by a read intent.
type SunSpecReadPurpose string

const (
	SunSpecReadSignature SunSpecReadPurpose = "signature"
	SunSpecReadHeader    SunSpecReadPurpose = "header"
	SunSpecReadPayload   SunSpecReadPurpose = "payload"
)

// SunSpecChainDisposition distinguishes structural admission from decoder support.
type SunSpecChainDisposition string

const (
	SunSpecChainDispositionAdmitted          SunSpecChainDisposition = "admitted"
	SunSpecChainDispositionUnknown           SunSpecChainDisposition = "unknown"
	SunSpecChainDispositionUnsupportedLength SunSpecChainDisposition = "unsupported_length"
)

// SunSpecSourceSpan identifies an immutable segment in one logical view.
type SunSpecSourceSpan struct {
	LogicalViewID        uint64
	PDUOffset, WordCount uint16
}

// SunSpecModelOccurrence preserves wire order and raw evidence for one model.
type SunSpecModelOccurrence struct {
	Ordinal                                           uint32
	ModelID, ModelLength, HeaderOffset, PayloadOffset uint16
	Disposition                                       SunSpecChainDisposition
	DecoderKey                                        *SunSpecDecoderKey
	words                                             []uint16
	spans                                             []SunSpecSourceSpan
}

func (o SunSpecModelOccurrence) Words() []uint16 { return append([]uint16(nil), o.words...) }
func (o SunSpecModelOccurrence) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), o.spans...)
}
func (o SunSpecModelOccurrence) Key() (SunSpecDecoderKey, bool) {
	if o.DecoderKey == nil {
		return SunSpecDecoderKey{}, false
	}
	return *o.DecoderKey, true
}

// SunSpecChainSnapshot is the immutable completed generic chain.
type SunSpecChainSnapshot struct {
	occurrences []SunSpecModelOccurrence
	raw         []uint16
}

func (s SunSpecChainSnapshot) Occurrences() []SunSpecModelOccurrence {
	return append([]SunSpecModelOccurrence(nil), s.occurrences...)
}
func (s SunSpecChainSnapshot) RawWords() []uint16 { return append([]uint16(nil), s.raw...) }
func (s SunSpecChainSnapshot) ByModelID(id uint16) []SunSpecModelOccurrence {
	out := make([]SunSpecModelOccurrence, 0)
	for _, o := range s.occurrences {
		if o.ModelID == id {
			out = append(out, o)
		}
	}
	return out
}

func validSunSpecRevision(v SunSpecSchemaRevision) bool { return len(v) > 0 && len(v) <= 128 }
func sunSpecEnd(address uint16, words uint16) error {
	if words == 0 || uint32(address)+uint32(words) > 65536 {
		return fmt.Errorf("SunSpec read range exceeds address space")
	}
	return nil
}
