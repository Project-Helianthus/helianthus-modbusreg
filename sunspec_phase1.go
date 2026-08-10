package modbusreg

import (
	"fmt"
	"reflect"
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
	normalization := AddressNormalizationSpec{Version: versions.Profile, SourceLocator: "urn:helianthus:evidence:sunspec-phase-one-v1", DocumentaryNotation: "40001-to-40000", DocumentaryBase: AddressBaseZeroBased, AddressSpaceLabel: string(HoldingRegisters), DocumentaryAddress: 40000, Transformation: TransformIdentity, ResolvedPDUOffset: 40000}
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
type SunSpecPhaseOneDecoder struct{ profile ProfileDescriptor }

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
	return SunSpecPhaseOneDecoder{profile: profile}, nil
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
	valid           bool
}

func (chain SunSpecPhaseOneChain) Models() []SunSpecPhaseOneModel {
	return append([]SunSpecPhaseOneModel(nil), chain.models...)
}
func (chain SunSpecPhaseOneChain) SkippedModels() []SunSpecPhaseOneModel {
	return append([]SunSpecPhaseOneModel(nil), chain.skipped...)
}

// Parse validates a bounded raw chain and its required terminal marker.
func (SunSpecPhaseOneDecoder) Parse(words []uint16) (SunSpecPhaseOneChain, error) {
	if len(words) > MaxSunSpecPhaseOneChainWords || len(words) < 4 || words[0] != sunSpecSignatureFirst || words[1] != sunSpecSignatureSecond {
		return SunSpecPhaseOneChain{}, fmt.Errorf("invalid SunSpec chain signature or bounds")
	}
	chain := SunSpecPhaseOneChain{}
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
		case id == 1 || id == 101 || id == 102 || id == 103:
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
	return (id >= 111 && id <= 113) || (id >= 120 && id <= 124) || id == 160 || (id >= 200 && id <= 299) || (id >= 700 && id <= 799)
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
	Observation ObservationSpec
}

func (decoder SunSpecPhaseOneDecoder) Activate(activation SunSpecPhaseOneActivation) (Observation, error) {
	if !activation.Chain.valid {
		return Observation{}, fmt.Errorf("SunSpec chain was not parsed")
	}
	return buildObservation(decoder.profile, activation.Observation)
}
