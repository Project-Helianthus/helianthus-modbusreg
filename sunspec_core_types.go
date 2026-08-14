package modbusreg

import "fmt"

type SunSpecSchemaRevision string
type SunSpecWireKey struct{ ModelID, ModelLength uint16 }
type SunSpecDecoderKey struct {
	ModelID, ModelLength uint16
	SchemaRevision       SunSpecSchemaRevision
}
type SunSpecReadPurpose string

const (
	SunSpecReadSignature SunSpecReadPurpose = "signature"
	SunSpecReadHeader    SunSpecReadPurpose = "header"
	SunSpecReadPayload   SunSpecReadPurpose = "payload"
)

type SunSpecChainDisposition string

const (
	SunSpecChainDispositionAdmitted          SunSpecChainDisposition = "admitted"
	SunSpecChainDispositionUnknownModel      SunSpecChainDisposition = "unknown_model"
	SunSpecChainDispositionUnsupportedLength SunSpecChainDisposition = "unsupported_length"
	SunSpecChainDispositionUnknown                                   = SunSpecChainDispositionUnknownModel
)

type SunSpecSourceSpan struct {
	LogicalViewID        uint64
	PDUOffset, WordCount uint16
}
type SunSpecOccurrence struct {
	Ordinal                     uint32
	WireKey                     SunSpecWireKey
	HeaderOffset, PayloadOffset uint16
	Disposition                 SunSpecChainDisposition
	decoderKey                  *SunSpecDecoderKey
	words                       []uint16
	spans                       []SunSpecSourceSpan
}

func (o SunSpecOccurrence) ModelID() uint16     { return o.WireKey.ModelID }
func (o SunSpecOccurrence) ModelLength() uint16 { return o.WireKey.ModelLength }
func (o SunSpecOccurrence) Words() []uint16     { return append([]uint16(nil), o.words...) }
func (o SunSpecOccurrence) SourceSpans() []SunSpecSourceSpan {
	return append([]SunSpecSourceSpan(nil), o.spans...)
}
func (o SunSpecOccurrence) DecoderKey() (SunSpecDecoderKey, bool) {
	if o.decoderKey == nil {
		return SunSpecDecoderKey{}, false
	}
	return *o.decoderKey, true
}

type SunSpecModelOccurrence = SunSpecOccurrence
type SunSpecChainSnapshot struct {
	occurrences []SunSpecOccurrence
	raw         []uint16
}

func cloneOccurrence(o SunSpecOccurrence) SunSpecOccurrence {
	o.words = append([]uint16(nil), o.words...)
	o.spans = append([]SunSpecSourceSpan(nil), o.spans...)
	if o.decoderKey != nil {
		k := *o.decoderKey
		o.decoderKey = &k
	}
	return o
}
func (s SunSpecChainSnapshot) Occurrences() []SunSpecOccurrence {
	out := make([]SunSpecOccurrence, len(s.occurrences))
	for i, o := range s.occurrences {
		out[i] = cloneOccurrence(o)
	}
	return out
}
func (s SunSpecChainSnapshot) RawWords() []uint16 { return append([]uint16(nil), s.raw...) }
func (s SunSpecChainSnapshot) ByModelID(id uint16) []SunSpecOccurrence {
	out := []SunSpecOccurrence{}
	for _, o := range s.occurrences {
		if o.WireKey.ModelID == id {
			out = append(out, cloneOccurrence(o))
		}
	}
	return out
}
func validSunSpecRevision(v SunSpecSchemaRevision) bool { return len(v) > 0 && len(v) <= 128 }
func sunSpecEnd(address, words uint16) error {
	if words == 0 || uint32(address)+uint32(words) > 65536 {
		return fmt.Errorf("SunSpec read range exceeds address space")
	}
	return nil
}
