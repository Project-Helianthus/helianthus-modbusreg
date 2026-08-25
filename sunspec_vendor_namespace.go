package modbusreg

import "fmt"

// SunSpecVendorDecoderNamespace holds definitions supplied explicitly by one
// vendor flavor. It is separate from the standard registry and never extends
// the standard decoder catalog.
type SunSpecVendorDecoderNamespace struct {
	registry SunSpecDecoderRegistry
}

func newSunSpecVendorDecoderNamespace(revision SunSpecSchemaRevision, definitions []sunSpecModelDefinition) (SunSpecVendorDecoderNamespace, error) {
	if len(definitions) == 0 {
		return SunSpecVendorDecoderNamespace{}, fmt.Errorf("vendor SunSpec namespace has no definitions")
	}
	registry := SunSpecDecoderRegistry{revision: revision, definitions: make(map[SunSpecDecoderKey]sunSpecModelDefinition, len(definitions))}
	for _, definition := range definitions {
		if definition.key.SchemaRevision != revision {
			return SunSpecVendorDecoderNamespace{}, fmt.Errorf("vendor SunSpec definition has incompatible revision")
		}
		if _, duplicate := registry.definitions[definition.key]; duplicate {
			return SunSpecVendorDecoderNamespace{}, fmt.Errorf("vendor SunSpec decoder key is duplicated")
		}
		registry.definitions[definition.key] = definition
		registry.keys = append(registry.keys, definition.key)
	}
	return SunSpecVendorDecoderNamespace{registry: registry}, nil
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
