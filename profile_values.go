package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// LogicalTable is the runtime's FC03/FC04 table identity.
type LogicalTable = modbus.LogicalTable

// The logical tables: holding registers are read with FC03 and input registers
// with FC04.
const (
	HoldingRegisters = modbus.HoldingRegisters
	InputRegisters   = modbus.InputRegisters
)

// AddressBase identifies one documentary address basis.
type AddressBase string

// The documentary address bases. Each base admits exactly one
// AddressTransformation.
const (
	AddressBaseZeroBased        AddressBase = "zero_based_pdu"
	AddressBaseOneBased         AddressBase = "one_based_register"
	AddressBaseHoldingReference AddressBase = "holding_reference_4xxxx"
	AddressBaseInputReference   AddressBase = "input_reference_3xxxx"
)

// AddressTransformation is the explicit documentary-to-PDU operation.
type AddressTransformation string

// The documentary-to-PDU transformations: identity for zero-based PDU
// addresses, minus one for one-based registers, minus 40001 for 4xxxx holding
// references and minus 30001 for 3xxxx input references.
const (
	TransformIdentity     AddressTransformation = "identity"
	TransformSubtractOne  AddressTransformation = "subtract_one"
	TransformHoldingToPDU AddressTransformation = "subtract_40001"
	TransformInputToPDU   AddressTransformation = "subtract_30001"
)

// AddressNormalizationSpec is the serialized documentary normalization record.
type AddressNormalizationSpec struct {
	Version             Version
	SourceLocator       string
	DocumentaryNotation string
	DocumentaryBase     AddressBase
	AddressSpaceLabel   string
	DocumentaryAddress  uint32
	Transformation      AddressTransformation
	ResolvedPDUOffset   uint16
}

// AddressNormalization is an immutable validated normalization record.
type AddressNormalization struct {
	spec AddressNormalizationSpec
}

// NewAddressNormalization validates the exact declared arithmetic.
func NewAddressNormalization(
	spec AddressNormalizationSpec,
) (AddressNormalization, error) {
	if err := preflightAddressNormalizationSpec(spec); err != nil {
		return AddressNormalization{}, err
	}
	if !spec.Version.Valid() || spec.SourceLocator == "" ||
		spec.DocumentaryNotation == "" || spec.AddressSpaceLabel == "" {
		return AddressNormalization{}, fmt.Errorf("normalization metadata is incomplete")
	}
	if !validNormalizationSourceLocator(spec.SourceLocator) {
		return AddressNormalization{}, fmt.Errorf("normalization source locator is unstable")
	}
	var resolved uint32
	switch {
	case spec.DocumentaryBase == AddressBaseZeroBased &&
		spec.Transformation == TransformIdentity:
		resolved = spec.DocumentaryAddress
	case spec.DocumentaryBase == AddressBaseOneBased &&
		spec.Transformation == TransformSubtractOne:
		if spec.DocumentaryAddress == 0 {
			return AddressNormalization{}, fmt.Errorf("one-based address is zero")
		}
		resolved = spec.DocumentaryAddress - 1
	case spec.DocumentaryBase == AddressBaseHoldingReference &&
		spec.Transformation == TransformHoldingToPDU:
		if spec.DocumentaryAddress < 40001 || spec.DocumentaryAddress > 49999 {
			return AddressNormalization{}, fmt.Errorf("holding reference is outside 4xxxx")
		}
		resolved = spec.DocumentaryAddress - 40001
	case spec.DocumentaryBase == AddressBaseInputReference &&
		spec.Transformation == TransformInputToPDU:
		if spec.DocumentaryAddress < 30001 || spec.DocumentaryAddress > 39999 {
			return AddressNormalization{}, fmt.Errorf("input reference is outside 3xxxx")
		}
		resolved = spec.DocumentaryAddress - 30001
	default:
		return AddressNormalization{}, fmt.Errorf("address basis and transformation disagree")
	}
	if resolved > 65535 || uint16(resolved) != spec.ResolvedPDUOffset {
		return AddressNormalization{}, fmt.Errorf("resolved PDU offset is inconsistent")
	}
	return AddressNormalization{spec: spec}, nil
}

func validNormalizationSourceLocator(locator string) bool {
	const evidencePrefix = "urn:helianthus:evidence:"
	if strings.HasPrefix(locator, evidencePrefix) {
		suffix := strings.TrimPrefix(locator, evidencePrefix)
		return suffix != "" && ValidIdentity(suffix)
	}
	parsed, err := url.Parse(locator)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return parsed.Path != "" && parsed.Path != "/"
}

func preflightAddressNormalizationSpec(spec AddressNormalizationSpec) error {
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "normalization source locator", value: spec.SourceLocator},
		{
			name:  "normalization documentary notation",
			value: spec.DocumentaryNotation,
		},
		{
			name:  "normalization address-space label",
			value: spec.AddressSpaceLabel,
		},
	}
	for _, field := range stringFields {
		if err := ValidateBoundedString(field.name, field.value, true); err != nil {
			return err
		}
	}
	return nil
}

// Spec returns the complete normalization record.
func (normalization AddressNormalization) Spec() AddressNormalizationSpec {
	return normalization.spec
}

// ResolvedPDUOffset returns the exact zero-based PDU offset.
func (normalization AddressNormalization) ResolvedPDUOffset() uint16 {
	return normalization.spec.ResolvedPDUOffset
}

// DependencySpec declares one exact profile input.
type DependencySpec struct {
	ID                 string
	Version            Version
	Table              LogicalTable
	Normalization      AddressNormalizationSpec
	WordCount          uint16
	CodecID            string
	CodecVersion       Version
	CoherenceGroup     string
	EvidenceReferences []string
	ApplicabilityRefs  []string
}

// Dependency is an immutable register dependency.
type Dependency struct {
	spec          DependencySpec
	normalization AddressNormalization
}

// NewDependency validates a dependency without performing any transport work.
func NewDependency(spec DependencySpec) (Dependency, error) {
	if err := preflightDependencySpec(spec); err != nil {
		return Dependency{}, err
	}
	spec.EvidenceReferences = slices.Clone(spec.EvidenceReferences)
	spec.ApplicabilityRefs = slices.Clone(spec.ApplicabilityRefs)
	normalization, err := NewAddressNormalization(spec.Normalization)
	if err != nil {
		return Dependency{}, err
	}
	if !ValidIdentity(spec.ID) || !spec.Version.Valid() ||
		(spec.Table != HoldingRegisters && spec.Table != InputRegisters) ||
		spec.WordCount == 0 || spec.WordCount > modbus.MaxReadRegisters ||
		!ValidIdentity(spec.CodecID) ||
		!spec.CodecVersion.Valid() || !ValidIdentity(spec.CoherenceGroup) ||
		!stringsComplete(spec.EvidenceReferences) ||
		!stringsComplete(spec.ApplicabilityRefs) {
		return Dependency{}, fmt.Errorf("dependency declaration is incomplete")
	}
	if spec.Normalization.AddressSpaceLabel != string(spec.Table) ||
		(spec.Table == HoldingRegisters &&
			spec.Normalization.DocumentaryBase == AddressBaseInputReference) ||
		(spec.Table == InputRegisters &&
			spec.Normalization.DocumentaryBase == AddressBaseHoldingReference) {
		return Dependency{}, fmt.Errorf("dependency table and address space disagree")
	}
	end := uint32(normalization.ResolvedPDUOffset()) + uint32(spec.WordCount)
	if end > 65536 {
		return Dependency{}, fmt.Errorf("dependency range overflows PDU space")
	}
	return Dependency{spec: spec, normalization: normalization}, nil
}

func preflightDependencySpec(spec DependencySpec) error {
	if spec.WordCount == 0 || spec.WordCount > modbus.MaxReadRegisters {
		return fmt.Errorf("dependency width exceeds the runtime boundary")
	}
	if len(spec.EvidenceReferences) > MaxProfileEvidenceReferences ||
		len(spec.ApplicabilityRefs) > MaxProfileEvidenceReferences {
		return fmt.Errorf("dependency exceeds the contract collection boundary")
	}
	if err := PreflightAggregate(spec); err != nil {
		return err
	}
	if err := preflightAddressNormalizationSpec(spec.Normalization); err != nil {
		return err
	}
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "dependency ID", value: spec.ID},
		{name: "dependency codec ID", value: spec.CodecID},
		{name: "dependency coherence group", value: spec.CoherenceGroup},
	}
	for _, field := range stringFields {
		if err := ValidateBoundedString(field.name, field.value, true); err != nil {
			return err
		}
	}
	if err := validateBoundedStrings(
		"dependency evidence references",
		spec.EvidenceReferences,
		MaxProfileEvidenceReferences,
		true,
	); err != nil {
		return err
	}
	return validateBoundedStrings(
		"dependency applicability references",
		spec.ApplicabilityRefs,
		MaxProfileEvidenceReferences,
		true,
	)
}

// Spec returns an independent complete dependency declaration.
func (dependency Dependency) Spec() DependencySpec {
	spec := dependency.spec
	spec.EvidenceReferences = slices.Clone(spec.EvidenceReferences)
	spec.ApplicabilityRefs = slices.Clone(spec.ApplicabilityRefs)
	return spec
}

// ID returns the stable dependency identifier.
func (dependency Dependency) ID() string {
	return dependency.spec.ID
}

// Version returns the immutable dependency version.
func (dependency Dependency) Version() Version {
	return dependency.spec.Version
}

// Table returns the FC03 or FC04 logical table.
func (dependency Dependency) Table() LogicalTable {
	return dependency.spec.Table
}

// WordCount returns the exact dependency width.
func (dependency Dependency) WordCount() uint16 {
	return dependency.spec.WordCount
}

// CodecID returns the exact codec identity.
func (dependency Dependency) CodecID() string {
	return dependency.spec.CodecID
}

// CodecVersion returns the exact codec version.
func (dependency Dependency) CodecVersion() Version {
	return dependency.spec.CodecVersion
}

// Normalization returns the immutable documentary address record.
func (dependency Dependency) Normalization() AddressNormalization {
	return dependency.normalization
}

// DependencySet is an exact ordered and versioned dependency declaration.
type DependencySet struct {
	id           string
	version      Version
	dependencies []Dependency
}

// NewDependencySet derives identity from exact ordered dependency declarations.
func NewDependencySet(
	version Version,
	dependencies []Dependency,
) (DependencySet, error) {
	if !version.Valid() || len(dependencies) == 0 ||
		len(dependencies) > MaxProfileDependencies {
		return DependencySet{}, fmt.Errorf("dependency set is incomplete")
	}
	if err := PreflightAggregate(version, dependencies); err != nil {
		return DependencySet{}, err
	}
	specs := make([]DependencySpec, len(dependencies))
	copies := make([]Dependency, len(dependencies))
	seen := make(map[string]struct{}, len(dependencies))
	for index, dependency := range dependencies {
		if !ValidIdentity(dependency.ID()) || !dependency.Version().Valid() {
			return DependencySet{}, fmt.Errorf("dependency set contains an invalid entry")
		}
		if _, exists := seen[dependency.ID()]; exists {
			return DependencySet{}, fmt.Errorf("dependency set contains duplicate IDs")
		}
		seen[dependency.ID()] = struct{}{}
		specs[index] = dependency.Spec()
		copy, err := NewDependency(specs[index])
		if err != nil {
			return DependencySet{}, err
		}
		copies[index] = copy
	}
	encoded, err := MarshalBounded(struct {
		Schema       string           `json:"schema"`
		Version      Version          `json:"version"`
		Dependencies []DependencySpec `json:"dependencies"`
	}{
		Schema:       "helianthus.modbusreg.dependency-set/v1",
		Version:      version,
		Dependencies: specs,
	})
	if err != nil {
		return DependencySet{}, fmt.Errorf("encode dependency set identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return DependencySet{
		id:           "sha256:" + hex.EncodeToString(digest[:]),
		version:      version,
		dependencies: copies,
	}, nil
}

// ID returns the content-bound ordered dependency-set identity.
func (set DependencySet) ID() string {
	return set.id
}

// Version returns the immutable dependency-set version.
func (set DependencySet) Version() Version {
	return set.version
}

// Dependencies returns independent immutable dependency values.
func (set DependencySet) Dependencies() []Dependency {
	result := make([]Dependency, len(set.dependencies))
	for index, dependency := range set.dependencies {
		result[index], _ = NewDependency(dependency.Spec())
	}
	return result
}

// ProfileKind distinguishes standard families from evidence-backed overlays.
type ProfileKind string

// The profile kinds.
const (
	ProfileStandardFamily ProfileKind = "standard_family"
	ProfileVendorOverlay  ProfileKind = "vendor_overlay"
)

// PublicationDisposition records how public evidence may be represented.
type PublicationDisposition string

// The publication dispositions: published evidence, or metadata only.
const (
	PublicationPublished    PublicationDisposition = "published"
	PublicationMetadataOnly PublicationDisposition = "metadata_only"
)

// EvidenceReference binds a profile to one public evidence record.
type EvidenceReference struct {
	ID                     string
	PublicationDisposition PublicationDisposition
}

// OverlayDeltaKind identifies one base-relative vendor deviation.
type OverlayDeltaKind string

// The overlay delta kinds; each names the kind of base-profile element a delta
// targets.
const (
	OverlayDeltaModelApplicability OverlayDeltaKind = "model_applicability"
	OverlayDeltaKnownExclusion     OverlayDeltaKind = "known_exclusion"
	OverlayDeltaCodec              OverlayDeltaKind = "codec"
	OverlayDeltaDependency         OverlayDeltaKind = "dependency"
	OverlayDeltaCoherence          OverlayDeltaKind = "coherence"
)

// OverlayDeltaOperation describes the base-relative change.
type OverlayDeltaOperation string

// The overlay delta operations.
const (
	OverlayDeltaAdd     OverlayDeltaOperation = "add"
	OverlayDeltaReplace OverlayDeltaOperation = "replace"
	OverlayDeltaRemove  OverlayDeltaOperation = "remove"
)

// VendorOverlayDeltaSpec is one typed, evidence-backed deviation from a
// qualified standard-family profile. Exactly one payload form is permitted.
type VendorOverlayDeltaSpec struct {
	ID                 string                `json:"ID"`
	Version            Version               `json:"Version"`
	Kind               OverlayDeltaKind      `json:"Kind"`
	Operation          OverlayDeltaOperation `json:"Operation"`
	TargetID           string                `json:"TargetID"`
	ApplicabilityValue string                `json:"ApplicabilityValue"`
	Codec              *CodecSpec            `json:"Codec,omitempty"`
	Dependency         *DependencySpec       `json:"Dependency,omitempty"`
	Coherence          *CoherencePolicySpec  `json:"Coherence,omitempty"`
	EvidenceReferences []string              `json:"EvidenceReferences"`
}

// ProfileMaturity is documentary support maturity, not activation policy.
type ProfileMaturity string

// The documentary profile maturities.
const (
	MaturityExperimental ProfileMaturity = "experimental"
	MaturityQualified    ProfileMaturity = "qualified"
)

// ProfileState records independent revocation or supersession.
type ProfileState string

// The profile states. Revocation and supersession are independent of maturity.
const (
	ProfileActive     ProfileState = "active"
	ProfileRevoked    ProfileState = "revoked"
	ProfileSuperseded ProfileState = "superseded"
)

// CoherenceMode declares how dependency responses form one sample.
type CoherenceMode string

// The coherence modes: every dependency from one wire response, or a bounded
// set of responses within declared skew limits.
const (
	CoherenceSingleWireResponse   CoherenceMode = "single_wire_response"
	CoherenceBoundedMultiResponse CoherenceMode = "bounded_multi_response"
)

// RetrySetBehavior makes retry ownership explicit.
type RetrySetBehavior string

// The retry behaviors. A single-response policy uses RetrySetNotApplicable; a
// bounded multi-response policy retries the whole set.
const (
	RetrySetNotApplicable RetrySetBehavior = "not_applicable"
	RetryWholeSet         RetrySetBehavior = "whole_set"
)

// AcquisitionOrder declares how bounded response facts are ordered.
type AcquisitionOrder string

// The acquisition orders. A single-response policy uses
// AcquisitionOrderNotApplicable; a bounded multi-response policy uses one of
// the other three.
const (
	AcquisitionOrderNotApplicable         AcquisitionOrder = "not_applicable"
	AcquisitionOrderDependencyDeclaration AcquisitionOrder = "dependency_declaration"
	AcquisitionOrderSourceTimeAscending   AcquisitionOrder = "source_time_ascending"
	AcquisitionOrderReceiptTimeAscending  AcquisitionOrder = "receipt_time_ascending"
)

// CoherencePolicySpec is the complete policy for one dependency set.
type CoherencePolicySpec struct {
	Version                      Version
	Mode                         CoherenceMode
	MaximumSourceSkew            time.Duration
	MaximumReceiptSkew           time.Duration
	RequireGenerationEquality    bool
	AcquisitionOrder             AcquisitionOrder
	RetrySetBehavior             RetrySetBehavior
	DocumentaryConsistencyMarker string
}

func validateCoherence(spec CoherencePolicySpec) error {
	if !spec.Version.Valid() {
		return fmt.Errorf("coherence version is missing")
	}
	switch spec.Mode {
	case CoherenceSingleWireResponse:
		if spec.MaximumSourceSkew != 0 || spec.MaximumReceiptSkew != 0 ||
			spec.RequireGenerationEquality ||
			spec.AcquisitionOrder != AcquisitionOrderNotApplicable ||
			spec.RetrySetBehavior != RetrySetNotApplicable ||
			spec.DocumentaryConsistencyMarker != "" {
			return fmt.Errorf("single-response policy carries multi-response fields")
		}
	case CoherenceBoundedMultiResponse:
		if spec.MaximumSourceSkew <= 0 || spec.MaximumReceiptSkew <= 0 ||
			spec.MaximumSourceSkew > MaxDeclaredCoherenceSkew ||
			spec.MaximumReceiptSkew > MaxDeclaredCoherenceSkew ||
			!spec.RequireGenerationEquality ||
			(spec.AcquisitionOrder != AcquisitionOrderDependencyDeclaration &&
				spec.AcquisitionOrder != AcquisitionOrderSourceTimeAscending &&
				spec.AcquisitionOrder != AcquisitionOrderReceiptTimeAscending) ||
			spec.RetrySetBehavior != RetryWholeSet {
			return fmt.Errorf("bounded multi-response policy is incomplete")
		}
	default:
		return fmt.Errorf("coherence mode is unknown")
	}
	return nil
}
