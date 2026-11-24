package sunspec

import (
	"fmt"
	"reflect"
	"slices"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

const (
	sunSpecPhaseOneProfileID = "sunspec.phase1"
	sunSpecSignatureFirst    = 0x5375
	sunSpecSignatureSecond   = 0x6e53
	// EndModelID is the model identifier of the end marker that
	// terminates a chain; the marker's declared length is zero.
	EndModelID = 0xffff
	// MaxSunSpecPhaseOneChainWords bounds a phase-one raw chain image,
	// signature and end marker included. The Fronius qualification card uses
	// it as its total word limit.
	MaxSunSpecPhaseOneChainWords = 512
)

// PhaseOneVersions selects the only supported profile and codec forms.
type PhaseOneVersions struct{ Profile, Codec modbusreg.Version }

// NewSunSpecPhaseOneProfile returns the vendor-neutral, read-only v1 profile.
func NewSunSpecPhaseOneProfile(versions PhaseOneVersions) (modbusreg.ProfileDescriptor, error) {
	if versions.Profile != modbusreg.CurrentSchemaVersion() || versions.Codec != modbusreg.CurrentCodecContractVersion() {
		return modbusreg.ProfileDescriptor{}, fmt.Errorf("unsupported SunSpec phase-one version")
	}
	codec, err := modbusreg.NewCodec(modbusreg.CodecSpec{ID: "sunspec-chain-word", Version: versions.Codec, RawWordCount: 1, WordPermutation: []uint16{0}, IntraWordByteOrder: modbusreg.ByteOrderModbus, Representation: modbusreg.RepresentationUnsignedInteger, Scale: modbusreg.ScaleSpec{Source: modbusreg.ScaleNotApplicable, ApplicationOrder: modbusreg.ScaleOrderNotApplicable}, String: modbusreg.StringSpec{Applicability: modbusreg.StringNotApplicable}, OutputProfileType: "uint16", ValidityBehavior: modbusreg.ValidityPreserveRaw})
	if err != nil {
		return modbusreg.ProfileDescriptor{}, err
	}
	normalization := modbusreg.AddressNormalizationSpec{Version: versions.Profile, SourceLocator: "urn:helianthus:evidence:sunspec-phase-one-v1", DocumentaryNotation: "40001", DocumentaryBase: modbusreg.AddressBaseOneBased, AddressSpaceLabel: string(modbusreg.HoldingRegisters), DocumentaryAddress: 40001, Transformation: modbusreg.TransformSubtractOne, ResolvedPDUOffset: 40000}
	dependency, err := modbusreg.NewDependency(modbusreg.DependencySpec{ID: "sunspec-chain-base", Version: versions.Profile, Table: modbusreg.HoldingRegisters, Normalization: normalization, WordCount: 1, CodecID: codec.ID(), CodecVersion: codec.Version(), CoherenceGroup: "sunspec-chain", EvidenceReferences: []string{"sunspec-phase-one-v1"}, ApplicabilityRefs: []string{"sunspec-standard-v1"}})
	if err != nil {
		return modbusreg.ProfileDescriptor{}, err
	}
	dependencies, err := modbusreg.NewDependencySet(versions.Profile, []modbusreg.Dependency{dependency})
	if err != nil {
		return modbusreg.ProfileDescriptor{}, err
	}
	return modbusreg.NewProfileDescriptor(modbusreg.ProfileDescriptorSpec{SchemaVersion: modbusreg.CurrentSchemaVersion(), ID: sunSpecPhaseOneProfileID, Version: versions.Profile, Kind: modbusreg.ProfileStandardFamily, StandardApplicability: []string{"sunspec-standard-v1"}, ModelApplicability: []string{"model-1", "model-101", "model-102", "model-103"}, KnownExclusions: []string{}, RuntimeContractVersion: modbusreg.PinnedRuntimeContractVersion(), DetectorVersion: versions.Profile, CodecContractVersion: versions.Codec, NormalizationVersion: versions.Profile, CoherenceVersion: versions.Profile, QualificationVersion: versions.Profile, Codecs: []modbusreg.Codec{codec}, Dependencies: dependencies, Coherence: modbusreg.CoherencePolicySpec{Version: versions.Profile, Mode: modbusreg.CoherenceSingleWireResponse, AcquisitionOrder: modbusreg.AcquisitionOrderNotApplicable, RetrySetBehavior: modbusreg.RetrySetNotApplicable}, Evidence: []modbusreg.EvidenceReference{{ID: "sunspec-phase-one-v1", PublicationDisposition: modbusreg.PublicationPublished}}, Maturity: modbusreg.MaturityQualified, DefaultEnabled: true, State: modbusreg.ProfileActive})
}

// DiscoveryRequest declares the protocol's one read-only base request.
type DiscoveryRequest struct{}

// Function returns FunctionReadHoldingRegisters.
func (DiscoveryRequest) Function() modbusreg.FunctionCode {
	return modbusreg.FunctionReadHoldingRegisters
}

// Table returns HoldingRegisters.
func (DiscoveryRequest) Table() modbusreg.LogicalTable { return modbusreg.HoldingRegisters }

// Offset returns the zero-based PDU address of the base, 40000 (documentary
// 40001).
func (DiscoveryRequest) Offset() uint16 { return 40000 }

// ReadOnly always reports true.
func (DiscoveryRequest) ReadOnly() bool { return true }

// PhaseOneDecoder validates and decodes the v1 standard family only.
type PhaseOneDecoder struct{ profile modbusreg.ProfileDescriptor }

// NewPhaseOneDecoder binds a decoder to profile, which must equal the
// profile NewSunSpecPhaseOneProfile builds for the current schema and codec
// versions; any other profile returns an error.
func NewPhaseOneDecoder(profile modbusreg.ProfileDescriptor) (PhaseOneDecoder, error) {
	if profile.ID() != sunSpecPhaseOneProfileID || profile.Kind() != modbusreg.ProfileStandardFamily || profile.Version() != modbusreg.CurrentSchemaVersion() || profile.CodecContractVersion() != modbusreg.CurrentCodecContractVersion() {
		return PhaseOneDecoder{}, fmt.Errorf("unsupported SunSpec phase-one profile")
	}
	expected, err := NewSunSpecPhaseOneProfile(PhaseOneVersions{
		Profile: modbusreg.CurrentSchemaVersion(),
		Codec:   modbusreg.CurrentCodecContractVersion(),
	})
	if err != nil || !reflect.DeepEqual(profile.Spec(), expected.Spec()) {
		return PhaseOneDecoder{}, fmt.Errorf("SunSpec phase-one profile is not exact")
	}
	return PhaseOneDecoder{profile: profile}, nil
}

// DiscoveryRequest returns the single read-only base request.
func (PhaseOneDecoder) DiscoveryRequest() DiscoveryRequest {
	return DiscoveryRequest{}
}

// PhaseOneModel records one structural model extent in the raw chain.
type PhaseOneModel struct{ id, offset, length uint16 }

// ID returns the model identifier.
func (model PhaseOneModel) ID() uint16 { return model.id }

// Offset returns the index of the model header in the raw chain image, not a
// register address.
func (model PhaseOneModel) Offset() uint16 { return model.offset }

// Length returns the declared payload length in words, excluding the header.
func (model PhaseOneModel) Length() uint16 { return model.length }

// PhaseOneChain retains semantic and structurally skipped models.
type PhaseOneChain struct {
	models, skipped []PhaseOneModel
	raw             []uint16
	common          *CommonModel
	inverters       map[uint16]InverterModel
	valid           bool
}

// Models returns a copy of the decoded Common and integer inverter models in
// chain order.
func (chain PhaseOneChain) Models() []PhaseOneModel {
	return append([]PhaseOneModel(nil), chain.models...)
}

// SkippedModels returns a copy of the models kept only as structural extents,
// in chain order.
func (chain PhaseOneChain) SkippedModels() []PhaseOneModel {
	return append([]PhaseOneModel(nil), chain.skipped...)
}

// RawWords returns the exact ordered discovery image.
func (chain PhaseOneChain) RawWords() []uint16 { return append([]uint16(nil), chain.raw...) }

// CommonModel exposes the admitted Common model fields.
type CommonModel struct {
	manufacturer, model, options, version, serial string
	deviceAddress                                 uint16
}

// Manufacturer returns the manufacturer string, cut at its first NUL.
func (model CommonModel) Manufacturer() string { return model.manufacturer }

// Model returns the model string, cut at its first NUL.
func (model CommonModel) Model() string { return model.model }

// Options returns the options string, cut at its first NUL.
func (model CommonModel) Options() string { return model.options }

// SerialNumber returns the serial number string, cut at its first NUL.
func (model CommonModel) SerialNumber() string { return model.serial }

// Version returns the version string, cut at its first NUL.
func (model CommonModel) Version() string { return model.version }

// DeviceAddress returns the device address word as read.
func (model CommonModel) DeviceAddress() uint16 { return model.deviceAddress }

// ScaledValue preserves raw integer, scale factor, and scaled value.
type ScaledValue struct {
	raw   int64
	scale int16
	value float64
}

// Raw returns the unscaled integer as read.
func (value ScaledValue) Raw() int64 { return value.raw }

// ScaleFactor returns the power-of-ten exponent, from -10 to 10.
func (value ScaledValue) ScaleFactor() int16 { return value.scale }

// Value returns Raw * 10^ScaleFactor as a float64, which may round.
func (value ScaledValue) Value() float64 { return value.value }

// InverterModel exposes admitted int-plus-scale-factor values.
type InverterModel struct{ power, energy ScaledValue }

// Power returns the scaled AC active power, in W.
func (model InverterModel) Power() ScaledValue { return model.power }

// Energy returns the scaled lifetime AC energy accumulator, in Wh.
func (model InverterModel) Energy() ScaledValue { return model.energy }

// Common returns the Common model and true; it reports false only for a chain
// that Parse did not produce.
func (chain PhaseOneChain) Common() (CommonModel, bool) {
	if chain.common == nil {
		return CommonModel{}, false
	}
	return *chain.common, true
}

// Inverter returns the integer inverter model with identifier id and true when
// the chain contains it.
func (chain PhaseOneChain) Inverter(id uint16) (InverterModel, bool) {
	value, ok := chain.inverters[id]
	return value, ok
}

// Parse validates a bounded raw chain and its required terminal marker.
func (PhaseOneDecoder) Parse(words []uint16) (PhaseOneChain, error) {
	if len(words) > MaxSunSpecPhaseOneChainWords || len(words) < 4 || words[0] != sunSpecSignatureFirst || words[1] != sunSpecSignatureSecond {
		return PhaseOneChain{}, fmt.Errorf("invalid SunSpec chain signature or bounds")
	}
	chain := PhaseOneChain{raw: append([]uint16(nil), words...), inverters: make(map[uint16]InverterModel)}
	for offset := 2; ; {
		if offset+2 > len(words) {
			return PhaseOneChain{}, fmt.Errorf("SunSpec chain is missing end marker")
		}
		id, length := words[offset], words[offset+1]
		if id == EndModelID {
			if length != 0 {
				return PhaseOneChain{}, fmt.Errorf("SunSpec end marker has nonzero length")
			}
			if offset+2 != len(words) {
				return PhaseOneChain{}, fmt.Errorf("SunSpec chain has trailing words")
			}
			if chain.common == nil {
				return PhaseOneChain{}, fmt.Errorf("SunSpec Common model is missing")
			}
			chain.valid = true
			return chain, nil
		}
		if length == 0 {
			return PhaseOneChain{}, fmt.Errorf("SunSpec non-end model has zero length")
		}
		end := offset + 2 + int(length)
		if end > len(words) {
			return PhaseOneChain{}, fmt.Errorf("SunSpec model extent overruns raw chain")
		}
		model := PhaseOneModel{id: id, offset: uint16(offset), length: length}
		if offset == 2 && id != 1 {
			return PhaseOneChain{}, fmt.Errorf("SunSpec Common model must be first")
		}
		switch {
		case id == 1:
			if offset != 2 || chain.common != nil {
				return PhaseOneChain{}, fmt.Errorf("SunSpec Common model is duplicated or reordered")
			}
			if length != 65 && length != 66 {
				return PhaseOneChain{}, fmt.Errorf("SunSpec Common model length is invalid")
			}
			common, err := decodeSunSpecCommon(words[offset+2 : end])
			if err != nil {
				return PhaseOneChain{}, err
			}
			chain.common = &common
			chain.models = append(chain.models, model)
		case id == 101 || id == 102 || id == 103:
			if length != 50 {
				return PhaseOneChain{}, fmt.Errorf("SunSpec inverter model length is invalid")
			}
			inverter, err := decodeSunSpecInverter(words[offset+2 : end])
			if err != nil {
				return PhaseOneChain{}, err
			}
			chain.inverters[id] = inverter
			chain.models = append(chain.models, model)
		case deferredSunSpecModel(id):
			return PhaseOneChain{}, fmt.Errorf("SunSpec model is deferred")
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

func decodeSunSpecCommon(words []uint16) (CommonModel, error) {
	decode := func(offset, width int) (string, error) {
		return PhaseOneDecoder{}.String(words[offset : offset+width])
	}
	manufacturer, err := decode(0, 16)
	if err != nil {
		return CommonModel{}, err
	}
	model, err := decode(16, 16)
	if err != nil {
		return CommonModel{}, err
	}
	options, err := decode(32, 8)
	if err != nil {
		return CommonModel{}, err
	}
	version, err := decode(40, 8)
	if err != nil {
		return CommonModel{}, err
	}
	serial, err := decode(48, 16)
	if err != nil {
		return CommonModel{}, err
	}
	return CommonModel{
		manufacturer:  manufacturer,
		model:         model,
		options:       options,
		version:       version,
		serial:        serial,
		deviceAddress: words[64],
	}, nil
}

func decodeSunSpecInverter(words []uint16) (InverterModel, error) {
	decoder := PhaseOneDecoder{}
	powerRaw, err := decoder.Int16(words[12])
	if err != nil {
		return InverterModel{}, err
	}
	powerScale, err := decoder.ScaleFactor(words[13])
	if err != nil {
		return InverterModel{}, err
	}
	energyRaw, err := decoder.Acc32(words[22], words[23])
	if err != nil {
		return InverterModel{}, err
	}
	energyScale, err := decoder.ScaleFactor(words[24])
	if err != nil {
		return InverterModel{}, err
	}
	return InverterModel{power: ScaledValue{raw: int64(powerRaw), scale: powerScale, value: float64(powerRaw) * decimalScale(powerScale)}, energy: ScaledValue{raw: energyRaw, scale: energyScale, value: float64(energyRaw) * decimalScale(energyScale)}}, nil
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

// Int16 reinterprets word as two's complement. It never fails and gives the
// not-implemented sentinel no special meaning.
func (PhaseOneDecoder) Int16(word uint16) (int16, error) { return int16(word), nil }

// Acc32 joins a high and a low word into an unsigned 32-bit accumulator. It
// never fails and returns zero as a reading.
func (PhaseOneDecoder) Acc32(high, low uint16) (int64, error) {
	return int64(uint32(high)<<16 | uint32(low)), nil
}

// ScaleFactor returns word as a scale-factor exponent. It returns an error for
// the not-implemented sentinel or a value outside -10 to 10.
func (PhaseOneDecoder) ScaleFactor(word uint16) (int16, error) {
	value := int16(word)
	if value == -32768 || value < -10 || value > 10 {
		return 0, fmt.Errorf("invalid SunSpec scale factor")
	}
	return value, nil
}

// String decodes the big-endian bytes of words up to the first NUL. It returns
// an error for more than MaxRawWords words; bytes after the first NUL are
// ignored and UTF-8 is not validated.
func (PhaseOneDecoder) String(words []uint16) (string, error) {
	if len(words) > modbusreg.MaxRawWords {
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

// PhaseOneCapture is the ordered source-view input for one activation.
type PhaseOneCapture struct {
	SourceViews []modbusreg.LogicalViewSnapshot
}

// Views returns defensive copies of the ordered source views.
func (capture PhaseOneCapture) Views() []modbusreg.LogicalViewSnapshot {
	return cloneSunSpecViews(capture.SourceViews)
}

// PhaseOneActivation joins a validated chain to one exact source capture.
type PhaseOneActivation struct {
	Chain       PhaseOneChain
	RawWords    []uint16
	Capture     PhaseOneCapture
	Observation modbusreg.ObservationSpec
}

// PhaseOneObservation retains one admitted observation with its raw chain.
type PhaseOneObservation struct {
	observation modbusreg.Observation
	raw         []uint16
	views       []modbusreg.LogicalViewSnapshot
}

// Spec returns the admitted observation spec.
func (observation PhaseOneObservation) Spec() modbusreg.ObservationSpec {
	return observation.observation.Spec()
}

// RawWords returns a copy of the raw chain image the observation was admitted
// with.
func (observation PhaseOneObservation) RawWords() []uint16 {
	return append([]uint16(nil), observation.raw...)
}

// SourceViews returns copies of the capture's source views in order.
func (observation PhaseOneObservation) SourceViews() []modbusreg.LogicalViewSnapshot {
	return cloneSunSpecViews(observation.views)
}

// Activate admits one observation for a chain produced by Parse. The capture's
// views must be valid and contiguous from the base, share one endpoint, unit,
// poll generation, transport, connection and authorization scope, and carry
// consistent physical-request and wire-response identities. Their words must
// equal the parsed chain, and RawWords when it is set. The observation spec is
// built against the profile and must bind exactly to the first view. Any
// failure returns an error and no observation.
func (decoder PhaseOneDecoder) Activate(activation PhaseOneActivation) (PhaseOneObservation, error) {
	if !activation.Chain.valid {
		return PhaseOneObservation{}, fmt.Errorf("SunSpec chain was not parsed")
	}
	raw, views, err := validateSunSpecCapture(activation.Capture)
	if err != nil {
		return PhaseOneObservation{}, err
	}
	if !slices.Equal(activation.Chain.raw, raw) ||
		(activation.RawWords != nil && !slices.Equal(activation.RawWords, raw)) {
		return PhaseOneObservation{}, fmt.Errorf("SunSpec chain does not match raw words")
	}
	observation, err := modbusreg.NewObservation(decoder.profile, activation.Observation)
	if err != nil {
		return PhaseOneObservation{}, err
	}
	if err := bindSunSpecObservation(decoder.profile, observation.Spec(), views); err != nil {
		return PhaseOneObservation{}, err
	}
	return PhaseOneObservation{
		observation: observation,
		raw:         append([]uint16(nil), raw...),
		views:       cloneSunSpecViews(views),
	}, nil
}

func validateSunSpecCapture(capture PhaseOneCapture) ([]uint16, []modbusreg.LogicalViewSnapshot, error) {
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
		function                                          modbusreg.FunctionCode
		table                                             modbusreg.LogicalTable
		offset, count                                     uint16
	}
	physicalIDs := make(map[uint64]physicalIdentity, len(views))
	type wireIdentity struct {
		physicalRequestID uint64
		responseBytes     []byte
	}
	wireIDs := make(map[uint64]wireIdentity, len(views))
	for index, view := range views {
		if !view.Valid() {
			return nil, nil, fmt.Errorf("SunSpec source view %d is invalid", index)
		}
		record := view.Record()
		if uint32(record.LogicalOffset) != expectedOffset ||
			record.Table != modbusreg.HoldingRegisters ||
			record.RequestedFunction != modbusreg.FunctionReadHoldingRegisters ||
			record.ReceivedFunction != modbusreg.FunctionReadHoldingRegisters ||
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

func bindSunSpecObservation(profile modbusreg.ProfileDescriptor, spec modbusreg.ObservationSpec, views []modbusreg.LogicalViewSnapshot) error {
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

func cloneSunSpecViews(values []modbusreg.LogicalViewSnapshot) []modbusreg.LogicalViewSnapshot {
	result := make([]modbusreg.LogicalViewSnapshot, len(values))
	for index, value := range values {
		result[index], _ = modbusreg.NewLogicalViewSnapshot(value.Record())
	}
	return result
}
