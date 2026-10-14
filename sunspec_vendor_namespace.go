package modbusreg

import "fmt"

// SunSpecVendorDecoderNamespace holds definitions supplied explicitly by one
// vendor flavor. It is separate from the standard registry and never extends
// the standard decoder catalog.
type SunSpecVendorDecoderNamespace struct {
	registry SunSpecDecoderRegistry
}

func (n SunSpecVendorDecoderNamespace) Decode(words []uint16) (SunSpecDecodedModel, error) {
	if len(words) < 2 {
		return SunSpecDecodedModel{}, fmt.Errorf("vendor SunSpec occurrence is incomplete")
	}
	key := SunSpecDecoderKey{ModelID: words[0], ModelLength: words[1], SchemaRevision: n.registry.revision}
	if _, ok := n.registry.definitions[key]; !ok {
		return SunSpecDecodedModel{}, fmt.Errorf("vendor SunSpec decoder key is unsupported")
	}
	copyKey := key
	decoded, err := n.registry.DecodeOccurrence(SunSpecOccurrence{
		WireKey:        SunSpecWireKey{ModelID: key.ModelID, ModelLength: key.ModelLength},
		SchemaRevision: key.SchemaRevision,
		Disposition:    SunSpecChainDispositionAdmitted,
		decoderKey:     &copyKey,
		words:          append([]uint16(nil), words...),
	})
	if err != nil {
		return SunSpecDecodedModel{}, err
	}
	filtered := decoded.facts[:0]
	for _, fact := range decoded.facts {
		if fact.FieldID != "" {
			filtered = append(filtered, fact)
		}
	}
	decoded.facts = append([]SunSpecFact(nil), filtered...)
	return decoded, nil
}
