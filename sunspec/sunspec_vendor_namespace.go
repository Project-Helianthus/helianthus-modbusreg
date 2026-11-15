package sunspec

import "fmt"

// VendorDecoderNamespace holds definitions supplied explicitly by one
// vendor flavor. It is separate from the standard registry and never extends
// the standard decoder catalog.
type VendorDecoderNamespace struct {
	registry DecoderRegistry
}

// Decode decodes one complete vendor occurrence, header words included. The
// model identifier and length in words[0] and words[1] must name a definition
// of this namespace under its revision; otherwise Decode returns an error.
// Facts without a field identifier are dropped. The input is copied and the
// result carries no source spans.
func (n VendorDecoderNamespace) Decode(words []uint16) (DecodedModel, error) {
	if len(words) < 2 {
		return DecodedModel{}, fmt.Errorf("vendor SunSpec occurrence is incomplete")
	}
	key := DecoderKey{ModelID: words[0], ModelLength: words[1], SchemaRevision: n.registry.revision}
	if _, ok := n.registry.definitions[key]; !ok {
		return DecodedModel{}, fmt.Errorf("vendor SunSpec decoder key is unsupported")
	}
	copyKey := key
	decoded, err := n.registry.DecodeOccurrence(Occurrence{
		WireKey:        WireKey{ModelID: key.ModelID, ModelLength: key.ModelLength},
		SchemaRevision: key.SchemaRevision,
		Disposition:    ChainDispositionAdmitted,
		decoderKey:     &copyKey,
		words:          append([]uint16(nil), words...),
	})
	if err != nil {
		return DecodedModel{}, err
	}
	filtered := decoded.facts[:0]
	for _, fact := range decoded.facts {
		if fact.FieldID != "" {
			filtered = append(filtered, fact)
		}
	}
	decoded.facts = append([]Fact(nil), filtered...)
	return decoded, nil
}
