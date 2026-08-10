package modbusreg

import (
	"fmt"
	"reflect"
	"slices"
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
type SunSpecCommonModel struct {
	manufacturer, model, options, version, serial string
	deviceAddress                                 uint16
}

func (model SunSpecCommonModel) Manufacturer() string  { return model.manufacturer }
func (model SunSpecCommonModel) Model() string         { return model.model }
func (model SunSpecCommonModel) Options() string       { return model.options }
func (model SunSpecCommonModel) SerialNumber() string  { return model.serial }
func (model SunSpecCommonModel) Version() string       { return model.version }
func (model SunSpecCommonModel) DeviceAddress() uint16 { return model.deviceAddress }

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
			if offset+2 != len(words) {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec chain has trailing words")
			}
			if chain.common == nil {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec Common model is missing")
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
		if offset == 2 && id != 1 {
			return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec Common model must be first")
		}
		switch {
		case id == 1:
			if offset != 2 || chain.common != nil {
				return SunSpecPhaseOneChain{}, fmt.Errorf("SunSpec Common model is duplicated or reordered")
			}
			if length != 65 && length != 66 {
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
	return (id >= 111 && id <= 113) || (id >= 120 && id <= 124) || id == 160 ||
		(id >= 200 && id <= 219) || (id >= 700 && id <= 799)
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
	options, err := decode(32, 8)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	version, err := decode(40, 8)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	serial, err := decode(48, 16)
	if err != nil {
		return SunSpecCommonModel{}, err
	}
	return SunSpecCommonModel{
		manufacturer:  manufacturer,
		model:         model,
		options:       options,
		version:       version,
		serial:        serial,
		deviceAddress: words[64],
	}, nil
}

func decodeSunSpecInverter(words []uint16) (SunSpecInverterModel, error) {
	decoder := SunSpecPhaseOneDecoder{}
	powerRaw, err := decoder.Int16(words[12])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	powerScale, err := decoder.ScaleFactor(words[13])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	energyRaw, err := decoder.Acc32(words[22], words[23])
	if err != nil {
		return SunSpecInverterModel{}, err
	}
	energyScale, err := decoder.ScaleFactor(words[24])
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

// SunSpecPhaseOneCapture is the ordered source-view input for one activation.
type SunSpecPhaseOneCapture struct {
	SourceViews []LogicalViewSnapshot
}

// Views returns defensive copies of the ordered source views.
func (capture SunSpecPhaseOneCapture) Views() []LogicalViewSnapshot {
	return cloneSunSpecViews(capture.SourceViews)
}

// SunSpecPhaseOneActivation joins a validated chain to one exact source capture.
type SunSpecPhaseOneActivation struct {
	Chain       SunSpecPhaseOneChain
	RawWords    []uint16
	Capture     SunSpecPhaseOneCapture
	Observation ObservationSpec
}

// SunSpecPhaseOneObservation retains one admitted observation with its raw chain.
type SunSpecPhaseOneObservation struct {
	observation Observation
	raw         []uint16
	views       []LogicalViewSnapshot
}

func (observation SunSpecPhaseOneObservation) Spec() ObservationSpec {
	return observation.observation.Spec()
}
func (observation SunSpecPhaseOneObservation) RawWords() []uint16 {
	return append([]uint16(nil), observation.raw...)
}
func (observation SunSpecPhaseOneObservation) SourceViews() []LogicalViewSnapshot {
	return cloneSunSpecViews(observation.views)
}

func (decoder SunSpecPhaseOneDecoder) Activate(activation SunSpecPhaseOneActivation) (SunSpecPhaseOneObservation, error) {
	if !activation.Chain.valid {
		return SunSpecPhaseOneObservation{}, fmt.Errorf("SunSpec chain was not parsed")
	}
	raw, views, err := validateSunSpecCapture(activation.Capture)
	if err != nil {
		return SunSpecPhaseOneObservation{}, err
	}
	if !slices.Equal(activation.Chain.raw, raw) ||
		(activation.RawWords != nil && !slices.Equal(activation.RawWords, raw)) {
		return SunSpecPhaseOneObservation{}, fmt.Errorf("SunSpec chain does not match raw words")
	}
	observation, err := buildObservation(decoder.profile, activation.Observation)
	if err != nil {
		return SunSpecPhaseOneObservation{}, err
	}
	if err := bindSunSpecObservation(decoder.profile, observation.Spec(), views); err != nil {
		return SunSpecPhaseOneObservation{}, err
	}
	return SunSpecPhaseOneObservation{
		observation: observation,
		raw:         append([]uint16(nil), raw...),
		views:       cloneSunSpecViews(views),
	}, nil
}

func validateSunSpecCapture(capture SunSpecPhaseOneCapture) ([]uint16, []LogicalViewSnapshot, error) {
	if len(capture.SourceViews) == 0 || len(capture.SourceViews) > MaxSunSpecPhaseOneChainWords {
		return nil, nil, fmt.Errorf("SunSpec source-view count is invalid")
	}
	views := cloneSunSpecViews(capture.SourceViews)
	first := views[0].Record()
	expectedOffset := uint32(40000)
	raw := make([]uint16, 0, MaxSunSpecPhaseOneChainWords)
	logicalIDs := make(map[uint64]struct{}, len(views))
	type physicalIdentity struct {
		wireResponseID, connectionID, transportGeneration uint64
		function                                          FunctionCode
		table                                             LogicalTable
		offset, count                                     uint16
	}
	physicalIDs := make(map[uint64]physicalIdentity, len(views))
	type wireIdentity struct {
		physicalRequestID uint64
		responseBytes     []byte
	}
	wireIDs := make(map[uint64]wireIdentity, len(views))
	for index, view := range views {
		if !view.valid {
			return nil, nil, fmt.Errorf("SunSpec source view %d is invalid", index)
		}
		record := view.Record()
		if uint32(record.LogicalOffset) != expectedOffset ||
			record.Table != HoldingRegisters ||
			record.RequestedFunction != FunctionReadHoldingRegisters ||
			record.ReceivedFunction != FunctionReadHoldingRegisters ||
			record.Endpoint != first.Endpoint || record.UnitID != first.UnitID ||
			record.PollGeneration != first.PollGeneration ||
			record.Transport != first.Transport ||
			record.TransportGeneration != first.TransportGeneration ||
			record.ConnectionID != first.ConnectionID ||
			record.AuthorizationScope != first.AuthorizationScope {
			return nil, nil, fmt.Errorf("SunSpec source views are detached or incoherent")
		}
		if _, duplicate := logicalIDs[record.LogicalViewID]; duplicate {
			return nil, nil, fmt.Errorf("SunSpec logical-view identity is duplicated")
		}
		logicalIDs[record.LogicalViewID] = struct{}{}
		physical := physicalIdentity{
			wireResponseID: record.WireResponseID, connectionID: record.ConnectionID,
			transportGeneration: record.TransportGeneration, function: record.RequestedFunction,
			table: record.Table, offset: record.PhysicalOffset, count: record.PhysicalWordCount,
		}
		if prior, exists := physicalIDs[record.PhysicalRequestID]; exists && prior != physical {
			return nil, nil, fmt.Errorf("SunSpec physical-request identity is contradictory")
		}
		physicalIDs[record.PhysicalRequestID] = physical
		if prior, exists := wireIDs[record.WireResponseID]; exists {
			if prior.physicalRequestID != record.PhysicalRequestID ||
				!slices.Equal(prior.responseBytes, record.WireResponseBytes) {
				return nil, nil, fmt.Errorf("SunSpec wire-response identity is contradictory")
			}
		}
		wireIDs[record.WireResponseID] = wireIdentity{
			physicalRequestID: record.PhysicalRequestID,
			responseBytes:     append([]byte(nil), record.WireResponseBytes...),
		}
		if len(raw)+len(record.Words) > MaxSunSpecPhaseOneChainWords {
			return nil, nil, fmt.Errorf("SunSpec source views exceed the raw bound")
		}
		raw = append(raw, record.Words...)
		expectedOffset += uint32(record.LogicalWordCount)
	}
	return raw, views, nil
}

func bindSunSpecObservation(profile ProfileDescriptor, spec ObservationSpec, views []LogicalViewSnapshot) error {
	if len(spec.Dependencies) != 1 || len(views) == 0 {
		return fmt.Errorf("SunSpec observation has no exact base source view")
	}
	dependency := profile.Dependencies().Dependencies()[0]
	result := spec.Dependencies[0]
	if result.DependencyID != dependency.ID() ||
		result.DependencyVersion != dependency.Version() ||
		result.CodecID != dependency.CodecID() ||
		result.CodecVersion != dependency.CodecVersion() ||
		result.NormalizationVersion != dependency.Normalization().Spec().Version ||
		!reflect.DeepEqual(result.View.Record(), views[0].Record()) {
		return fmt.Errorf("SunSpec observation base view is detached from capture")
	}
	first := views[0].Record()
	if spec.Endpoint != first.Endpoint || spec.UnitID != first.UnitID ||
		spec.PollGenerationID != first.PollGeneration ||
		first.LogicalOffset != dependency.Normalization().ResolvedPDUOffset() ||
		first.LogicalWordCount != dependency.WordCount() {
		return fmt.Errorf("SunSpec observation provenance disagrees with capture")
	}
	return nil
}

func cloneSunSpecViews(values []LogicalViewSnapshot) []LogicalViewSnapshot {
	result := make([]LogicalViewSnapshot, len(values))
	for index, value := range values {
		result[index], _ = NewLogicalViewSnapshot(value.Record())
	}
	return result
}
