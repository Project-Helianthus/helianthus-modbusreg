package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"sync"
)

const (
	sunSpecPhaseOneProfileID     = "sunspec.phase1"
	sunSpecSignatureFirst        = 0x5375
	sunSpecSignatureSecond       = 0x6e53
	sunSpecEndModel              = 0xffff
	MaxSunSpecPhaseOneChainWords = 512
)

// SunSpecPhaseOneVersions selects the only supported profile and codec forms.
type SunSpecPhaseOneVersions struct{ Profile, Codec Version }

// NewSunSpecPhaseOneProfile returns the vendor-neutral, read-only v1 profile.
func NewSunSpecPhaseOneProfile(versions SunSpecPhaseOneVersions) (ProfileDescriptor, error) {
	if versions.Profile != CurrentSchemaVersion() || versions.Codec != CurrentCodecContractVersion() {
		return ProfileDescriptor{}, fmt.Errorf("unsupported SunSpec phase-one version")
	}
	codec, err := NewCodec(CodecSpec{ID: "sunspec-chain-word", Version: versions.Codec, RawWordCount: 1, WordPermutation: []uint16{0}, IntraWordByteOrder: ByteOrderModbus, Representation: RepresentationUnsignedInteger, Scale: ScaleSpec{Source: ScaleNotApplicable, ApplicationOrder: ScaleOrderNotApplicable}, String: StringSpec{Applicability: StringNotApplicable}, OutputProfileType: "uint16", ValidityBehavior: ValidityPreserveRaw})
	if err != nil {
		return ProfileDescriptor{}, err
	}
	normalization := AddressNormalizationSpec{Version: versions.Profile, SourceLocator: "urn:helianthus:evidence:sunspec-phase-one-v1", DocumentaryNotation: "40001", DocumentaryBase: AddressBaseOneBased, AddressSpaceLabel: string(HoldingRegisters), DocumentaryAddress: 40001, Transformation: TransformSubtractOne, ResolvedPDUOffset: 40000}
	dependency, err := NewDependency(DependencySpec{ID: "sunspec-chain-base", Version: versions.Profile, Table: HoldingRegisters, Normalization: normalization, WordCount: 1, CodecID: codec.ID(), CodecVersion: codec.Version(), CoherenceGroup: "sunspec-chain", EvidenceReferences: []string{"sunspec-phase-one-v1"}, ApplicabilityRefs: []string{"sunspec-standard-v1"}})
	if err != nil {
		return ProfileDescriptor{}, err
	}
	dependencies, err := NewDependencySet(versions.Profile, []Dependency{dependency})
	if err != nil {
		return ProfileDescriptor{}, err
	}
	return NewProfileDescriptor(ProfileDescriptorSpec{SchemaVersion: CurrentSchemaVersion(), ID: sunSpecPhaseOneProfileID, Version: versions.Profile, Kind: ProfileStandardFamily, StandardApplicability: []string{"sunspec-standard-v1"}, ModelApplicability: []string{"model-1", "model-101", "model-102", "model-103"}, KnownExclusions: []string{}, RuntimeContractVersion: PinnedRuntimeContractVersion(), DetectorVersion: versions.Profile, CodecContractVersion: versions.Codec, NormalizationVersion: versions.Profile, CoherenceVersion: versions.Profile, QualificationVersion: versions.Profile, Codecs: []Codec{codec}, Dependencies: dependencies, Coherence: CoherencePolicySpec{Version: versions.Profile, Mode: CoherenceSingleWireResponse, AcquisitionOrder: AcquisitionOrderNotApplicable, RetrySetBehavior: RetrySetNotApplicable}, Evidence: []EvidenceReference{{ID: "sunspec-phase-one-v1", PublicationDisposition: PublicationPublished}}, Maturity: MaturityExperimental, State: ProfileActive})
}

// SunSpecDiscoveryRequest declares the protocol's one read-only base request.
type SunSpecDiscoveryRequest struct{}

func (SunSpecDiscoveryRequest) Function() FunctionCode { return FunctionReadHoldingRegisters }
func (SunSpecDiscoveryRequest) Table() LogicalTable    { return HoldingRegisters }
func (SunSpecDiscoveryRequest) Offset() uint16         { return 40000 }
func (SunSpecDiscoveryRequest) ReadOnly() bool         { return true }

// SunSpecPhaseOneDecoder validates and decodes the v1 standard family only.
type SunSpecPhaseOneDecoder struct {
	profile  ProfileDescriptor
	bindings *sunSpecBindingStore
}

type sunSpecBindingStore struct {
	mu     sync.Mutex
	values map[[32]byte][]byte
}

func NewSunSpecPhaseOneDecoder(profile ProfileDescriptor) (SunSpecPhaseOneDecoder, error) {
	if profile.ID() != sunSpecPhaseOneProfileID || profile.Kind() != ProfileStandardFamily || profile.Version() != CurrentSchemaVersion() || profile.CodecContractVersion() != CurrentCodecContractVersion() {
		return SunSpecPhaseOneDecoder{}, fmt.Errorf("unsupported SunSpec phase-one profile")
	}
	expected, err := NewSunSpecPhaseOneProfile(SunSpecPhaseOneVersions{
		Profile: CurrentSchemaVersion(),
		Codec:   CurrentCodecContractVersion(),
	})
	if err != nil || !reflect.DeepEqual(profile.Spec(), expected.Spec()) {
		return SunSpecPhaseOneDecoder{}, fmt.Errorf("SunSpec phase-one profile is not exact")
	}
	return SunSpecPhaseOneDecoder{profile: profile, bindings: &sunSpecBindingStore{values: make(map[[32]byte][]byte)}}, nil
}
func (SunSpecPhaseOneDecoder) DiscoveryRequest() SunSpecDiscoveryRequest {
	return SunSpecDiscoveryRequest{}
}

// SunSpecPhaseOneModel records one structural model extent in the raw chain.
type SunSpecPhaseOneModel struct{ id, offset, length uint16 }

func (model SunSpecPhaseOneModel) ID() uint16     { return model.id }
func (model SunSpecPhaseOneModel) Offset() uint16 { return model.offset }
func (model SunSpecPhaseOneModel) Length() uint16 { return model.length }

// SunSpecPhaseOneChain retains semantic and structurally skipped models.
type SunSpecPhaseOneChain struct {
	models, skipped []SunSpecPhaseOneModel
	raw             []uint16
	common          *SunSpecCommonModel
	inverters       map[uint16]SunSpecInverterModel
	valid           bool
}

func (chain SunSpecPhaseOneChain) Models() []SunSpecPhaseOneModel {
	return append([]SunSpecPhaseOneModel(nil), chain.models...)
}
func (chain SunSpecPhaseOneChain) SkippedModels() []SunSpecPhaseOneModel {
	return append([]SunSpecPhaseOneModel(nil), chain.skipped...)
}

// RawWords returns the exact ordered discovery image.
func (chain SunSpecPhaseOneChain) RawWords() []uint16 { return append([]uint16(nil), chain.raw...) }

// SunSpecCommonModel exposes the admitted Common model fields.
type SunSpecCommonModel struct{ manufacturer, model, serial, version string }

func (model SunSpecCommonModel) Manufacturer() string { return model.manufacturer }
func (model SunSpecCommonModel) Model() string        { return model.model }
func (model SunSpecCommonModel) SerialNumber() string { return model.serial }
func (model SunSpecCommonModel) Version() string      { return model.version }

// SunSpecScaledValue preserves raw integer, scale factor, and scaled value.
type SunSpecScaledValue struct {
	raw   int64
	scale int16
	value float64
}

func (value SunSpecScaledValue) Raw() int64         { return value.raw }
func (value SunSpecScaledValue) ScaleFactor() int16 { return value.scale }
func (value SunSpecScaledValue) Value() float64     { return value.value }

// SunSpecInverterModel exposes admitted int-plus-scale-factor values.
type SunSpecInverterModel struct{ power, energy SunSpecScaledValue }

func (model SunSpecInverterModel) Power() SunSpecScaledValue  { return model.power }
func (model SunSpecInverterModel) Energy() SunSpecScaledValue { return model.energy }
func (chain SunSpecPhaseOneChain) Common() (SunSpecCommonModel, bool) {
	if chain.common == nil {
		return SunSpecCommonModel{}, false
	}
	return *chain.common, true
}
func (chain SunSpecPhaseOneChain) Inverter(id uint16) (SunSpecInverterModel, bool) {
	value, ok := chain.inverters[id]
	return value, ok
}

// Parse validates a bounded raw chain and its required terminal marker.
func (SunSpecPhaseOneDecoder) Parse(words []uint16) (SunSpecPhaseOneChain, error) {
	if len(words) > MaxSunSpecPhaseOneChainWords || len(words) < 4 || words[0] != sunSpecSignatureFirst || words[1] != sunSpecSignatureSecond {
		return SunSpecPhaseOneChain{}, fmt.Errorf("invalid SunSpec chain signature or bounds")
	}
	chain := SunSpecPhaseOneChain{raw: append([]uint16(nil), words...), inverters: make(map[uint16]SunSpecInverterModel)}
	for offset := 2; ; {
		if offset+2 > len(words) {
			return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec chain is missing end marker")
		}
		id, length := words[offset], words[offset+1]
		if id == sunSpecEndModel {
			if length != 0 {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec end marker has nonzero length")
			}
			chain.valid = true
			return chain, nil
		}
		if length == 0 {
			return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec non-end model has zero length")
		}
		end := offset + 2 + int(length)
		if end > len(words) {
			return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec model extent overruns raw chain")
		}
		model := SunSpecPhaseOneModel{id: id, offset: uint16(offset), length: length}
		switch {
		case id == 1:
			if length != 65 {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec Common model length is invalid")
			}
			common, err := decodeSunSpecCommon(words[offset+2 : end])
			if err != nil {
				return SunSpecPhaseOneChain{}, err
			}
			chain.common = &common
			chain.models = append(chain.models, model)
		case id == 101 || id == 102 || id == 103:
			if length != 50 {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec inverter model length is invalid")
			}
			inverter, err := decodeSunSpecInverter(words[offset+2 : end])
			if err != nil {
				return SunSpecPhaseOneChain{}, err
			}
			chain.inverters[id] = inverter
			chain.models = append(chain.models, model)
		case deferredSunSpecModel(id):
			return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec model is deferred")
		default:
			chain.skipped = append(chain.skipped, model)
		}
		offset = end
	}
}
func deferredSunSpecModel(id uint16) bool {
	return (id >= 200 && id <= 219) || (id >= 700 && id <= 799)
}

func decodeSunSpecCommon(words []uint16) (SunSpecCommonModel, error) {
	decode := func(offset, width int) (string, error) {
		return SunSpecPhaseOneDecoder{}.String(words[offset : offset+width])
	}
	manufacturer, err := decode(0, 16)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	model, err := decode(16, 16)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	serial, err := decode(32, 16)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	version, err := decode(48, 8)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	return SunSpecCommonModel{manufacturer: manufacturer, model: model, serial: serial, version: version}, nil
}

func decodeSunSpecInverter(words []uint16) (SunSpecInverterModel, error) {
	decoder := SunSpecPhaseOneDecoder{}
	powerRaw, err := decoder.Int16(words[8])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	powerScale, err := decoder.ScaleFactor(words[9])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	energyRaw, err := decoder.Acc32(words[16], words[17])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	energyScale, err := decoder.ScaleFactor(words[18])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	return SunSpecInverterModel{power: SunSpecScaledValue{raw: int64(powerRaw), scale: powerScale, value: float64(powerRaw) * decimalScale(powerScale)}, energy: SunSpecScaledValue{raw: energyRaw, scale: energyScale, value: float64(energyRaw) * decimalScale(energyScale)}}, nil
}

func decimalScale(exponent int16) float64 {
	value := 1.0
	for exponent < 0 {
		value /= 10
		exponent++
	}
	for exponent > 0 {
		value *= 10
		exponent--
	}
	return value
}
func (SunSpecPhaseOneDecoder) Int16(word uint16) (int16, error) { return int16(word), nil }
func (SunSpecPhaseOneDecoder) Acc32(high, low uint16) (int64, error) {
	return int64(uint32(high)<<16 | uint32(low)), nil
}
func (SunSpecPhaseOneDecoder) ScaleFactor(word uint16) (int16, error) {
	value := int16(word)
	if value == -32768 || value < -10 || value > 10 {
		return 0, fmt.Errorf("invalid SunSpec scale factor")
	}
	return value, nil
}
func (SunSpecPhaseOneDecoder) String(words []uint16) (string, error) {
	if len(words) > MaxRawWords {
		return "", fmt.Errorf("SunSpec string exceeds word bound")
	}
	bytes := make([]byte, 0, len(words)*2)
	for _, word := range words {
		for _, value := range []byte{byte(word >> 8), byte(word)} {
			if value == 0 {
				return string(bytes), nil
			}
			bytes = append(bytes, value)
		}
	}
	return string(bytes), nil
}

// SunSpecPhaseOneActivation joins a validated chain to the existing source envelope.
type SunSpecPhaseOneActivation struct {
	Chain       SunSpecPhaseOneChain
	RawWords    []uint16
	Observation ObservationSpec
}

// SunSpecPhaseOneObservation retains one admitted observation with its raw chain.
type SunSpecPhaseOneObservation struct {
	observation Observation
	raw         []uint16
}

func (observation SunSpecPhaseOneObservation) Spec() ObservationSpec {
	return observation.observation.Spec()
}
func (observation SunSpecPhaseOneObservation) RawWords() []uint16 {
	return append([]uint16(nil), observation.raw...)
}

func (decoder SunSpecPhaseOneDecoder) Activate(activation SunSpecPhaseOneActivation) (SunSpecPhaseOneObservation, error) {
	raw := activation.RawWords
	if raw == nil {
		raw = activation.Chain.raw
	}
	if !activation.Chain.valid || !bytes.Equal(wordsToBytes(activation.Chain.raw), wordsToBytes(raw)) {
		return SunSpecPhaseOneObservation{}, fmt.Errorf("SunSpec chain does not match raw words")
	}
	observation, err := buildObservation(decoder.profile, activation.Observation)
	if err != nil {
		return SunSpecPhaseOneObservation{}, err
	}
	encoded := []byte(fmt.Sprintf("%#v", observation.Spec()))
	key := sha256.Sum256(wordsToBytes(raw))
	decoder.bindings.mu.Lock()
	defer decoder.bindings.mu.Unlock()
	if prior, exists := decoder.bindings.values[key]; exists {
		if !bytes.Equal(prior, encoded) {
			return SunSpecPhaseOneObservation{}, fmt.Errorf("SunSpec raw chain provenance is mismatched")
		}
		return SunSpecPhaseOneObservation{observation: observation, raw: append([]uint16(nil), raw...)}, nil
	}
	if len(decoder.bindings.values) >= 64 {
		return SunSpecPhaseOneObservation{}, fmt.Errorf("SunSpec retained binding limit reached")
	}
	decoder.bindings.values[key] = append([]byte(nil), encoded...)
	return SunSpecPhaseOneObservation{observation: observation, raw: append([]uint16(nil), raw...)}, nil
}

func wordsToBytes(words []uint16) []byte {
	result := make([]byte, len(words)*2)
	for index, word := range words {
		result[index*2], result[index*2+1] = byte(word>>8), byte(word)
	}
	return result
}
