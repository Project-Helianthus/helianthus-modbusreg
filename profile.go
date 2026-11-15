package modbusreg

import (
	"fmt"
	"reflect"
	"slices"
	"sort"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ProfileDescriptorSpec is the complete serialized profile declaration.
type ProfileDescriptorSpec struct {
	SchemaVersion          Version
	ID                     string
	Version                Version
	Kind                   ProfileKind
	StandardApplicability  []string
	ModelApplicability     []string
	VendorApplicability    []string
	KnownExclusions        []string
	RuntimeContractVersion Version
	DetectorVersion        Version
	CodecContractVersion   Version
	NormalizationVersion   Version
	CoherenceVersion       Version
	QualificationVersion   Version
	Codecs                 []Codec
	Dependencies           DependencySet
	Coherence              CoherencePolicySpec
	Evidence               []EvidenceReference
	Maturity               ProfileMaturity
	DefaultEnabled         bool
	State                  ProfileState
	RefinesProfileID       string
	RefinesProfileVersion  Version
	SupersededByID         string
	SupersededByVersion    Version
	OverlayDeltas          []VendorOverlayDeltaSpec
}

// ProfileDescriptor is an immutable validated profile contract.
type ProfileDescriptor struct {
	spec ProfileDescriptorSpec
}

// NewProfileDescriptor validates one complete immutable profile declaration.
func NewProfileDescriptor(spec ProfileDescriptorSpec) (ProfileDescriptor, error) {
	if err := preflightProfileSpec(spec); err != nil {
		return ProfileDescriptor{}, err
	}
	spec = cloneProfileSpec(spec)
	if spec.SchemaVersion != schemaVersionV1 || !ValidIdentity(spec.ID) ||
		!spec.Version.Valid() ||
		spec.RuntimeContractVersion != runtimeContractVersionV1 ||
		!spec.DetectorVersion.Valid() ||
		spec.CodecContractVersion != codecContractVersionV1 ||
		!spec.NormalizationVersion.Valid() ||
		!spec.CoherenceVersion.Valid() || !spec.QualificationVersion.Valid() ||
		len(spec.Evidence) == 0 {
		return ProfileDescriptor{}, fmt.Errorf("profile descriptor is incomplete")
	}
	switch spec.Kind {
	case ProfileStandardFamily:
		if len(spec.VendorApplicability) != 0 ||
			spec.RefinesProfileID != "" || spec.RefinesProfileVersion.Valid() ||
			len(spec.OverlayDeltas) != 0 ||
			!stringsComplete(spec.StandardApplicability) ||
			!stringsDeclared(spec.ModelApplicability) ||
			!stringsDeclared(spec.KnownExclusions) ||
			spec.Dependencies.ID() == "" ||
			!spec.Dependencies.Version().Valid() ||
			len(spec.Codecs) == 0 {
			return ProfileDescriptor{}, fmt.Errorf("standard family contains vendor assumptions")
		}
		if err := validateCoherence(spec.Coherence); err != nil {
			return ProfileDescriptor{}, err
		}
		if spec.Coherence.Version != spec.CoherenceVersion {
			return ProfileDescriptor{}, fmt.Errorf("coherence versions disagree")
		}
	case ProfileVendorOverlay:
		if !stringsComplete(spec.VendorApplicability) ||
			!ValidIdentity(spec.RefinesProfileID) ||
			!spec.RefinesProfileVersion.Valid() ||
			len(spec.StandardApplicability) != 0 ||
			len(spec.ModelApplicability) != 0 ||
			len(spec.KnownExclusions) != 0 ||
			len(spec.Codecs) != 0 || spec.Dependencies.ID() != "" ||
			spec.Dependencies.Version().Valid() ||
			!reflect.DeepEqual(spec.Coherence, CoherencePolicySpec{}) ||
			len(spec.OverlayDeltas) == 0 {
			return ProfileDescriptor{}, fmt.Errorf("vendor overlay lacks qualified base")
		}
	default:
		return ProfileDescriptor{}, fmt.Errorf("profile kind is unknown")
	}
	codecs := make(map[string]Codec, len(spec.Codecs))
	for _, codec := range spec.Codecs {
		if !ValidIdentity(codec.ID()) || !codec.Version().Valid() {
			return ProfileDescriptor{}, fmt.Errorf("profile contains invalid codec")
		}
		if _, exists := codecs[codec.ID()]; exists {
			return ProfileDescriptor{}, fmt.Errorf("profile contains duplicate codec ID")
		}
		codecs[codec.ID()] = codec
	}
	evidenceIDs := make(map[string]struct{}, len(spec.Evidence))
	for _, evidence := range spec.Evidence {
		if !ValidIdentity(evidence.ID) ||
			(evidence.PublicationDisposition != PublicationPublished &&
				evidence.PublicationDisposition != PublicationMetadataOnly) {
			return ProfileDescriptor{}, fmt.Errorf("evidence reference is incomplete")
		}
		if _, exists := evidenceIDs[evidence.ID]; exists {
			return ProfileDescriptor{}, fmt.Errorf("duplicate evidence reference")
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}
	if spec.Kind == ProfileVendorOverlay {
		if err := validateOverlayDeltas(spec.OverlayDeltas, evidenceIDs); err != nil {
			return ProfileDescriptor{}, err
		}
	}
	dependencies := spec.Dependencies.Dependencies()
	dependencyIDs := make(map[string]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		dependencyIDs[dependency.ID()] = struct{}{}
		codec, exists := codecs[dependency.CodecID()]
		if !exists || codec.Version() != dependency.CodecVersion() ||
			codec.RawWordCount() != dependency.WordCount() {
			return ProfileDescriptor{}, fmt.Errorf("dependency codec is absent or mismatched")
		}
		if dependency.Normalization().Spec().Version != spec.NormalizationVersion {
			return ProfileDescriptor{}, fmt.Errorf("normalization versions disagree")
		}
		for _, evidenceID := range dependency.Spec().EvidenceReferences {
			if _, exists := evidenceIDs[evidenceID]; !exists {
				return ProfileDescriptor{}, fmt.Errorf("dependency evidence is not in profile")
			}
		}
	}
	if spec.Coherence.Mode == CoherenceSingleWireResponse {
		if err := validateSingleWireDependencies(dependencies); err != nil {
			return ProfileDescriptor{}, err
		}
	}
	for _, codec := range spec.Codecs {
		scale := codec.Spec().Scale
		if scale.Source == ScaleDependency {
			if _, exists := dependencyIDs[scale.DependencyID]; !exists {
				return ProfileDescriptor{}, fmt.Errorf("codec scale dependency is absent")
			}
		}
	}
	scaleEdges := make(map[string]string)
	for _, dependency := range dependencies {
		scale := codecs[dependency.CodecID()].Spec().Scale
		if scale.Source == ScaleDependency {
			scaleEdges[dependency.ID()] = scale.DependencyID
		}
	}
	if hasDependencyCycle(scaleEdges) {
		return ProfileDescriptor{}, fmt.Errorf("codec scale dependencies are cyclic")
	}
	if spec.Maturity != MaturityExperimental &&
		spec.Maturity != MaturityQualified {
		return ProfileDescriptor{}, fmt.Errorf("profile maturity is unknown")
	}
	if spec.Maturity == MaturityExperimental && spec.DefaultEnabled {
		return ProfileDescriptor{}, fmt.Errorf("experimental profile cannot default on")
	}
	switch spec.State {
	case ProfileActive:
		if spec.SupersededByID != "" || spec.SupersededByVersion.Valid() {
			return ProfileDescriptor{}, fmt.Errorf("active profile has replacement")
		}
	case ProfileRevoked:
		if spec.DefaultEnabled || spec.SupersededByID != "" ||
			spec.SupersededByVersion.Valid() {
			return ProfileDescriptor{}, fmt.Errorf(
				"revoked profile carries activation or successor fields",
			)
		}
	case ProfileSuperseded:
		if !ValidIdentity(spec.SupersededByID) ||
			!spec.SupersededByVersion.Valid() {
			return ProfileDescriptor{}, fmt.Errorf("superseded profile lacks replacement")
		}
		if spec.DefaultEnabled {
			return ProfileDescriptor{}, fmt.Errorf("superseded profile cannot default on")
		}
	default:
		return ProfileDescriptor{}, fmt.Errorf("profile state is unknown")
	}
	return ProfileDescriptor{spec: spec}, nil
}

func preflightProfileSpec(spec ProfileDescriptorSpec) error {
	if len(spec.StandardApplicability) > MaxProfileEvidenceReferences ||
		len(spec.ModelApplicability) > MaxProfileEvidenceReferences ||
		len(spec.VendorApplicability) > MaxProfileEvidenceReferences ||
		len(spec.KnownExclusions) > MaxProfileEvidenceReferences ||
		len(spec.Codecs) > MaxProfileCodecs ||
		len(spec.Evidence) > MaxProfileEvidenceReferences ||
		len(spec.OverlayDeltas) > MaxProfileDependencies {
		return fmt.Errorf("profile exceeds the contract collection boundary")
	}
	if err := PreflightAggregate(spec); err != nil {
		return err
	}
	if err := ValidateBoundedString(
		"coherence documentary marker",
		spec.Coherence.DocumentaryConsistencyMarker,
		false,
	); err != nil {
		return err
	}
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "profile ID", value: spec.ID},
		{name: "refined profile ID", value: spec.RefinesProfileID},
		{name: "superseded profile ID", value: spec.SupersededByID},
	}
	for _, field := range stringFields {
		if err := ValidateBoundedString(field.name, field.value, false); err != nil {
			return err
		}
	}
	stringSlices := []struct {
		name   string
		values []string
	}{
		{name: "standard applicability", values: spec.StandardApplicability},
		{name: "model applicability", values: spec.ModelApplicability},
		{name: "vendor applicability", values: spec.VendorApplicability},
		{name: "known exclusions", values: spec.KnownExclusions},
	}
	for _, field := range stringSlices {
		if err := validateBoundedStrings(
			field.name,
			field.values,
			MaxProfileEvidenceReferences,
			false,
		); err != nil {
			return err
		}
	}
	for _, evidence := range spec.Evidence {
		if err := ValidateBoundedString(
			"profile evidence ID",
			evidence.ID,
			true,
		); err != nil {
			return err
		}
	}
	for _, delta := range spec.OverlayDeltas {
		if err := preflightOverlayDelta(delta); err != nil {
			return err
		}
	}
	return nil
}

func preflightOverlayDelta(delta VendorOverlayDeltaSpec) error {
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "overlay delta ID", value: delta.ID},
		{name: "overlay delta target", value: delta.TargetID},
		{
			name:  "overlay delta applicability value",
			value: delta.ApplicabilityValue,
		},
	}
	for _, field := range stringFields {
		if err := ValidateBoundedString(field.name, field.value, false); err != nil {
			return err
		}
	}
	if err := validateBoundedStrings(
		"overlay delta evidence",
		delta.EvidenceReferences,
		MaxProfileEvidenceReferences,
		true,
	); err != nil {
		return err
	}
	if delta.Codec != nil {
		if err := preflightCodecSpec(*delta.Codec); err != nil {
			return fmt.Errorf("overlay codec preflight: %w", err)
		}
	}
	if delta.Dependency != nil {
		if err := preflightDependencySpec(*delta.Dependency); err != nil {
			return fmt.Errorf("overlay dependency preflight: %w", err)
		}
	}
	if delta.Coherence != nil {
		if err := ValidateBoundedString(
			"overlay coherence marker",
			delta.Coherence.DocumentaryConsistencyMarker,
			false,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateOverlayDeltas(
	deltas []VendorOverlayDeltaSpec,
	evidenceIDs map[string]struct{},
) error {
	seen := make(map[string]struct{}, len(deltas))
	seenTargets := make(map[string]struct{}, len(deltas))
	for _, delta := range deltas {
		if !ValidIdentity(delta.ID) || !delta.Version.Valid() {
			return fmt.Errorf("overlay delta identity is incomplete")
		}
		if _, exists := seen[delta.ID]; exists {
			return fmt.Errorf("overlay delta identity is duplicated")
		}
		seen[delta.ID] = struct{}{}
		if !stringsComplete(delta.EvidenceReferences) {
			return fmt.Errorf("overlay delta evidence is incomplete")
		}
		for _, evidenceID := range delta.EvidenceReferences {
			if _, exists := evidenceIDs[evidenceID]; !exists {
				return fmt.Errorf("overlay delta evidence is absent from profile")
			}
		}
		switch delta.Kind {
		case OverlayDeltaModelApplicability, OverlayDeltaKnownExclusion:
			if (delta.Operation != OverlayDeltaAdd &&
				delta.Operation != OverlayDeltaRemove) ||
				delta.ApplicabilityValue == "" || delta.TargetID != "" ||
				delta.Codec != nil || delta.Dependency != nil ||
				delta.Coherence != nil {
				return fmt.Errorf("overlay applicability delta is malformed")
			}
			target := string(delta.Kind) + ":" + delta.ApplicabilityValue
			if _, exists := seenTargets[target]; exists {
				return fmt.Errorf("overlay delta target is duplicated")
			}
			seenTargets[target] = struct{}{}
		case OverlayDeltaCodec:
			if (delta.Operation != OverlayDeltaAdd &&
				delta.Operation != OverlayDeltaReplace) ||
				!ValidIdentity(delta.TargetID) ||
				delta.ApplicabilityValue != "" || delta.Codec == nil ||
				delta.Dependency != nil || delta.Coherence != nil {
				return fmt.Errorf("overlay codec delta is malformed")
			}
			target := string(delta.Kind) + ":" + delta.TargetID
			if _, exists := seenTargets[target]; exists {
				return fmt.Errorf("overlay delta target is duplicated")
			}
			seenTargets[target] = struct{}{}
			codec, err := NewCodec(*delta.Codec)
			if err != nil || codec.ID() != delta.TargetID {
				return fmt.Errorf("overlay codec delta is invalid")
			}
		case OverlayDeltaDependency:
			if !ValidIdentity(delta.TargetID) ||
				delta.ApplicabilityValue != "" || delta.Codec != nil ||
				delta.Coherence != nil {
				return fmt.Errorf("overlay dependency delta is malformed")
			}
			target := string(delta.Kind) + ":" + delta.TargetID
			if _, exists := seenTargets[target]; exists {
				return fmt.Errorf("overlay delta target is duplicated")
			}
			seenTargets[target] = struct{}{}
			switch delta.Operation {
			case OverlayDeltaAdd, OverlayDeltaReplace:
				if delta.Dependency == nil {
					return fmt.Errorf("overlay dependency delta lacks payload")
				}
				dependency, err := NewDependency(*delta.Dependency)
				if err != nil || dependency.ID() != delta.TargetID {
					return fmt.Errorf("overlay dependency delta is invalid")
				}
				for _, evidenceID := range dependency.Spec().EvidenceReferences {
					if !slices.Contains(delta.EvidenceReferences, evidenceID) {
						return fmt.Errorf("overlay dependency payload lacks delta evidence")
					}
				}
			case OverlayDeltaRemove:
				if delta.Dependency != nil {
					return fmt.Errorf("overlay dependency removal has payload")
				}
			default:
				return fmt.Errorf("overlay dependency operation is unknown")
			}
		case OverlayDeltaCoherence:
			if delta.Operation != OverlayDeltaReplace ||
				delta.TargetID != "coherence-policy" ||
				delta.ApplicabilityValue != "" || delta.Codec != nil ||
				delta.Dependency != nil || delta.Coherence == nil ||
				validateCoherence(*delta.Coherence) != nil {
				return fmt.Errorf("overlay coherence delta is malformed")
			}
			target := string(delta.Kind) + ":" + delta.TargetID
			if _, exists := seenTargets[target]; exists {
				return fmt.Errorf("overlay delta target is duplicated")
			}
			seenTargets[target] = struct{}{}
		default:
			return fmt.Errorf("overlay delta kind is unknown")
		}
	}
	return nil
}

func validateSingleWireDependencies(dependencies []Dependency) error {
	if len(dependencies) == 0 {
		return fmt.Errorf("single-wire dependency set is empty")
	}
	table := dependencies[0].Table()
	minOffset := uint32(dependencies[0].Normalization().ResolvedPDUOffset())
	maxEnd := minOffset + uint32(dependencies[0].WordCount())
	maxStart := minOffset
	minEnd := maxEnd
	for _, dependency := range dependencies[1:] {
		if dependency.Table() != table {
			return fmt.Errorf("single-wire dependencies span FC03 and FC04")
		}
		start := uint32(dependency.Normalization().ResolvedPDUOffset())
		end := start + uint32(dependency.WordCount())
		if start < minOffset {
			minOffset = start
		}
		if end > maxEnd {
			maxEnd = end
		}
		if start > maxStart {
			maxStart = start
		}
		if end < minEnd {
			minEnd = end
		}
	}
	if maxEnd-minOffset > modbus.MaxReadRegisters {
		return fmt.Errorf("single-wire physical union exceeds runtime maximum")
	}
	if len(dependencies) > 1 && maxStart >= minEnd {
		return fmt.Errorf("single-wire dependencies lack a common intersection")
	}
	return nil
}

func hasDependencyCycle(edges map[string]string) bool {
	const (
		visiting = 1
		visited  = 2
	)
	states := make(map[string]byte, len(edges))
	var visit func(string) bool
	visit = func(dependencyID string) bool {
		switch states[dependencyID] {
		case visiting:
			return true
		case visited:
			return false
		}
		states[dependencyID] = visiting
		if next, exists := edges[dependencyID]; exists && visit(next) {
			return true
		}
		states[dependencyID] = visited
		return false
	}
	for dependencyID := range edges {
		if visit(dependencyID) {
			return true
		}
	}
	return false
}

func cloneProfileSpec(spec ProfileDescriptorSpec) ProfileDescriptorSpec {
	spec.StandardApplicability = slices.Clone(spec.StandardApplicability)
	spec.ModelApplicability = slices.Clone(spec.ModelApplicability)
	spec.VendorApplicability = slices.Clone(spec.VendorApplicability)
	spec.KnownExclusions = slices.Clone(spec.KnownExclusions)
	spec.Codecs = append([]Codec(nil), spec.Codecs...)
	for index, codec := range spec.Codecs {
		spec.Codecs[index], _ = NewCodec(codec.Spec())
	}
	spec.Dependencies, _ = NewDependencySet(
		spec.Dependencies.Version(),
		spec.Dependencies.Dependencies(),
	)
	spec.Evidence = append([]EvidenceReference(nil), spec.Evidence...)
	overlayDeltas := spec.OverlayDeltas
	spec.OverlayDeltas = make(
		[]VendorOverlayDeltaSpec,
		len(overlayDeltas),
	)
	for index, delta := range overlayDeltas {
		spec.OverlayDeltas[index] = cloneOverlayDelta(delta)
	}
	return spec
}

func cloneOverlayDelta(delta VendorOverlayDeltaSpec) VendorOverlayDeltaSpec {
	delta.EvidenceReferences = slices.Clone(delta.EvidenceReferences)
	if delta.Codec != nil {
		codec := cloneCodecSpec(*delta.Codec)
		delta.Codec = &codec
	}
	if delta.Dependency != nil {
		dependency := *delta.Dependency
		dependency.EvidenceReferences = slices.Clone(
			dependency.EvidenceReferences,
		)
		dependency.ApplicabilityRefs = slices.Clone(
			dependency.ApplicabilityRefs,
		)
		delta.Dependency = &dependency
	}
	if delta.Coherence != nil {
		coherence := *delta.Coherence
		delta.Coherence = &coherence
	}
	return delta
}

// Spec returns an independent complete profile declaration.
func (profile ProfileDescriptor) Spec() ProfileDescriptorSpec {
	return cloneProfileSpec(profile.spec)
}

// ID returns the stable profile identity.
func (profile ProfileDescriptor) ID() string {
	return profile.spec.ID
}

// Version returns the immutable profile version.
func (profile ProfileDescriptor) Version() Version {
	return profile.spec.Version
}

// Kind returns whether this immutable profile is a standard family or overlay.
func (profile ProfileDescriptor) Kind() ProfileKind {
	return profile.spec.Kind
}

// Coherence returns the profile's sample coherence policy: how its dependency
// responses form one sample. A vendor overlay declares the zero policy; the
// standard family it refines carries the policy.
func (profile ProfileDescriptor) Coherence() CoherencePolicySpec {
	return profile.spec.Coherence
}

// Maturity returns the profile's declared maturity, experimental or
// qualified. Maturity is documentary; an experimental profile never defaults
// on.
func (profile ProfileDescriptor) Maturity() ProfileMaturity {
	return profile.spec.Maturity
}

// DefaultEnabled reports whether the profile takes part in detection without
// an explicit opt-in. Only an active qualified profile can default on: the
// descriptor rejects default activation for any other.
func (profile ProfileDescriptor) DefaultEnabled() bool {
	return profile.spec.DefaultEnabled
}

// State returns the profile's lifecycle state: active, superseded or revoked.
// A superseded profile names its replacement; an active or revoked profile
// names none.
func (profile ProfileDescriptor) State() ProfileState {
	return profile.spec.State
}

// RuntimeContractVersion returns the required transport contract version.
func (profile ProfileDescriptor) RuntimeContractVersion() Version {
	return profile.spec.RuntimeContractVersion
}

// DetectorVersion returns the detector contract identity without implementing detection.
func (profile ProfileDescriptor) DetectorVersion() Version {
	return profile.spec.DetectorVersion
}

// CodecContractVersion returns the complete codec declaration contract version.
func (profile ProfileDescriptor) CodecContractVersion() Version {
	return profile.spec.CodecContractVersion
}

// NormalizationVersion returns the documentary normalization version.
func (profile ProfileDescriptor) NormalizationVersion() Version {
	return profile.spec.NormalizationVersion
}

// CoherenceVersion returns the sample coherence version.
func (profile ProfileDescriptor) CoherenceVersion() Version {
	return profile.spec.CoherenceVersion
}

// QualificationVersion returns the referenced qualification contract version.
func (profile ProfileDescriptor) QualificationVersion() Version {
	return profile.spec.QualificationVersion
}

// Dependencies returns the exact ordered dependency set.
func (profile ProfileDescriptor) Dependencies() DependencySet {
	set, _ := NewDependencySet(
		profile.spec.Dependencies.Version(),
		profile.spec.Dependencies.Dependencies(),
	)
	return set
}

// Catalog is a deterministic immutable profile collection.
type Catalog struct {
	profiles []ProfileDescriptor
}

// NewCatalog rejects duplicate IDs, including a second profile version.
func NewCatalog(profiles ...ProfileDescriptor) (Catalog, error) {
	if len(profiles) == 0 || len(profiles) > MaxProfileDependencies {
		return Catalog{}, fmt.Errorf("catalog is empty or exceeds the contract maximum")
	}
	if err := PreflightAggregate(profiles); err != nil {
		return Catalog{}, err
	}
	seen := make(map[string]struct{}, len(profiles))
	result := make([]ProfileDescriptor, len(profiles))
	for index, profile := range profiles {
		if !ValidIdentity(profile.ID()) || !profile.Version().Valid() {
			return Catalog{}, fmt.Errorf("catalog contains invalid profile")
		}
		if _, exists := seen[profile.ID()]; exists {
			return Catalog{}, fmt.Errorf("catalog contains duplicate profile ID")
		}
		seen[profile.ID()] = struct{}{}
		copy, err := NewProfileDescriptor(profile.Spec())
		if err != nil {
			return Catalog{}, err
		}
		result[index] = copy
	}
	sort.Slice(result, func(first, second int) bool {
		if result[first].ID() != result[second].ID() {
			return result[first].ID() < result[second].ID()
		}
		return result[first].Version().String() < result[second].Version().String()
	})
	byID := make(map[string]ProfileDescriptor, len(result))
	for _, profile := range result {
		byID[profile.ID()] = profile
	}
	for _, profile := range result {
		spec := profile.spec
		if spec.Kind == ProfileVendorOverlay {
			base, exists := byID[spec.RefinesProfileID]
			if !exists || base.Version() != spec.RefinesProfileVersion ||
				base.spec.Kind != ProfileStandardFamily ||
				base.spec.State != ProfileActive ||
				base.spec.Maturity != MaturityQualified {
				return Catalog{}, fmt.Errorf("vendor overlay base is not active and qualified")
			}
			if spec.RuntimeContractVersion != base.spec.RuntimeContractVersion ||
				spec.DetectorVersion != base.spec.DetectorVersion ||
				spec.CodecContractVersion != base.spec.CodecContractVersion ||
				spec.NormalizationVersion != base.spec.NormalizationVersion ||
				spec.QualificationVersion != base.spec.QualificationVersion {
				return Catalog{}, fmt.Errorf("vendor overlay changes inherited contract versions")
			}
			if err := validateOverlayAgainstBase(spec, base.spec); err != nil {
				return Catalog{}, err
			}
			baseEvidence := make(map[string]struct{}, len(base.spec.Evidence))
			for _, evidence := range base.spec.Evidence {
				baseEvidence[evidence.ID] = struct{}{}
			}
			for _, evidence := range spec.Evidence {
				if _, copied := baseEvidence[evidence.ID]; copied {
					return Catalog{}, fmt.Errorf("vendor overlay copied base evidence")
				}
			}
		}
		if spec.State != ProfileSuperseded {
			continue
		}
		target, exists := byID[spec.SupersededByID]
		if !exists || target.ID() == profile.ID() ||
			target.Version() != spec.SupersededByVersion ||
			target.spec.State != ProfileActive ||
			target.spec.Kind != spec.Kind {
			return Catalog{}, fmt.Errorf("supersession target is absent or incoherent")
		}
		if spec.Kind == ProfileVendorOverlay &&
			(target.spec.RefinesProfileID != spec.RefinesProfileID ||
				target.spec.RefinesProfileVersion != spec.RefinesProfileVersion ||
				!slices.Equal(
					target.spec.VendorApplicability,
					spec.VendorApplicability,
				)) {
			return Catalog{}, fmt.Errorf("vendor supersession changes overlay identity")
		}
	}
	return Catalog{profiles: result}, nil
}

func validateOverlayAgainstBase(
	overlay ProfileDescriptorSpec,
	base ProfileDescriptorSpec,
) error {
	models := slices.Clone(base.ModelApplicability)
	exclusions := slices.Clone(base.KnownExclusions)
	codecs := make([]CodecSpec, len(base.Codecs))
	for index, codec := range base.Codecs {
		codecs[index] = codec.Spec()
	}
	baseDependencies := base.Dependencies.Dependencies()
	dependencies := make([]DependencySpec, len(baseDependencies))
	for index, dependency := range baseDependencies {
		dependencies[index] = dependency.Spec()
	}
	coherence := base.Coherence
	coherenceChanged := false
	for _, delta := range overlay.OverlayDeltas {
		switch delta.Kind {
		case OverlayDeltaModelApplicability:
			exists := slices.Contains(models, delta.ApplicabilityValue)
			if (delta.Operation == OverlayDeltaAdd && exists) ||
				(delta.Operation == OverlayDeltaRemove && !exists) {
				return fmt.Errorf("overlay model-applicability delta is a no-op")
			}
			if delta.Operation == OverlayDeltaAdd {
				models = append(models, delta.ApplicabilityValue)
			} else {
				models = removeString(models, delta.ApplicabilityValue)
			}
		case OverlayDeltaKnownExclusion:
			exists := slices.Contains(exclusions, delta.ApplicabilityValue)
			if (delta.Operation == OverlayDeltaAdd && exists) ||
				(delta.Operation == OverlayDeltaRemove && !exists) {
				return fmt.Errorf("overlay exclusion delta is a no-op")
			}
			if delta.Operation == OverlayDeltaAdd {
				exclusions = append(exclusions, delta.ApplicabilityValue)
			} else {
				exclusions = removeString(exclusions, delta.ApplicabilityValue)
			}
		case OverlayDeltaCodec:
			index := codecSpecIndex(codecs, delta.TargetID)
			if delta.Operation == OverlayDeltaAdd && index >= 0 {
				return fmt.Errorf("overlay codec addition already exists")
			}
			if delta.Operation == OverlayDeltaAdd {
				codecs = append(codecs, cloneCodecSpec(*delta.Codec))
				continue
			}
			if index < 0 {
				return fmt.Errorf("overlay codec replacement target is absent")
			}
			current := codecs[index]
			if !codecSemanticsChanged(current, *delta.Codec) {
				return fmt.Errorf("overlay codec replacement is semantically unchanged")
			}
			if CompareVersions(delta.Codec.Version, current.Version) <= 0 {
				return fmt.Errorf("overlay codec replacement did not advance version")
			}
			codecs[index] = cloneCodecSpec(*delta.Codec)
		case OverlayDeltaDependency:
			index := dependencySpecIndex(dependencies, delta.TargetID)
			switch delta.Operation {
			case OverlayDeltaAdd:
				if index >= 0 {
					return fmt.Errorf("overlay dependency addition already exists")
				}
				dependencies = append(
					dependencies,
					cloneDependencySpec(*delta.Dependency),
				)
			case OverlayDeltaReplace:
				if index < 0 {
					return fmt.Errorf("overlay dependency replacement target is absent")
				}
				current := dependencies[index]
				if !dependencySemanticsChanged(current, *delta.Dependency) {
					return fmt.Errorf(
						"overlay dependency replacement is semantically unchanged",
					)
				}
				if CompareVersions(delta.Dependency.Version, current.Version) <= 0 {
					return fmt.Errorf(
						"overlay dependency replacement did not advance version",
					)
				}
				dependencies[index] = cloneDependencySpec(*delta.Dependency)
			case OverlayDeltaRemove:
				if index < 0 {
					return fmt.Errorf("overlay dependency removal target is absent")
				}
				dependencies = append(
					dependencies[:index],
					dependencies[index+1:]...,
				)
			}
		case OverlayDeltaCoherence:
			if !coherenceSemanticsChanged(coherence, *delta.Coherence) {
				return fmt.Errorf("overlay coherence replacement is semantically unchanged")
			}
			if CompareVersions(delta.Coherence.Version, coherence.Version) <= 0 {
				return fmt.Errorf("overlay coherence replacement did not advance version")
			}
			coherence = *delta.Coherence
			coherenceChanged = true
		}
	}
	if coherenceChanged {
		if overlay.CoherenceVersion != coherence.Version {
			return fmt.Errorf("overlay coherence contract version disagrees with delta")
		}
	} else if overlay.CoherenceVersion != base.CoherenceVersion {
		return fmt.Errorf("overlay changes inherited coherence version")
	}
	effectiveCodecs := make([]Codec, len(codecs))
	for index, codec := range codecs {
		validated, err := NewCodec(codec)
		if err != nil {
			return fmt.Errorf("effective overlay codec %d: %w", index, err)
		}
		effectiveCodecs[index] = validated
	}
	effectiveDependencies := make([]Dependency, len(dependencies))
	for index, dependency := range dependencies {
		validated, err := NewDependency(dependency)
		if err != nil {
			return fmt.Errorf("effective overlay dependency %d: %w", index, err)
		}
		effectiveDependencies[index] = validated
	}
	dependencySet, err := NewDependencySet(
		base.Dependencies.Version(),
		effectiveDependencies,
	)
	if err != nil {
		return fmt.Errorf("effective overlay dependency set: %w", err)
	}
	evidence := append([]EvidenceReference(nil), base.Evidence...)
	evidence = append(evidence, overlay.Evidence...)
	_, err = NewProfileDescriptor(ProfileDescriptorSpec{
		SchemaVersion:          overlay.SchemaVersion,
		ID:                     overlay.ID,
		Version:                overlay.Version,
		Kind:                   ProfileStandardFamily,
		StandardApplicability:  slices.Clone(base.StandardApplicability),
		ModelApplicability:     models,
		KnownExclusions:        exclusions,
		RuntimeContractVersion: overlay.RuntimeContractVersion,
		DetectorVersion:        overlay.DetectorVersion,
		CodecContractVersion:   overlay.CodecContractVersion,
		NormalizationVersion:   overlay.NormalizationVersion,
		CoherenceVersion:       coherence.Version,
		QualificationVersion:   overlay.QualificationVersion,
		Codecs:                 effectiveCodecs,
		Dependencies:           dependencySet,
		Coherence:              coherence,
		Evidence:               evidence,
		Maturity:               base.Maturity,
		DefaultEnabled:         false,
		State:                  ProfileActive,
	})
	if err != nil {
		return fmt.Errorf("effective overlay graph: %w", err)
	}
	return nil
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values)-1)
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func codecSpecIndex(codecs []CodecSpec, target string) int {
	for index := range codecs {
		if codecs[index].ID == target {
			return index
		}
	}
	return -1
}

func dependencySpecIndex(dependencies []DependencySpec, target string) int {
	for index := range dependencies {
		if dependencies[index].ID == target {
			return index
		}
	}
	return -1
}

func cloneDependencySpec(spec DependencySpec) DependencySpec {
	spec.EvidenceReferences = slices.Clone(spec.EvidenceReferences)
	spec.ApplicabilityRefs = slices.Clone(spec.ApplicabilityRefs)
	return spec
}

func codecSemanticsChanged(first, second CodecSpec) bool {
	first.Version = Version{}
	second.Version = Version{}
	return !reflect.DeepEqual(first, second)
}

func dependencySemanticsChanged(first, second DependencySpec) bool {
	first.Version = Version{}
	second.Version = Version{}
	return !reflect.DeepEqual(first, second)
}

func coherenceSemanticsChanged(first, second CoherencePolicySpec) bool {
	first.Version = Version{}
	second.Version = Version{}
	return !reflect.DeepEqual(first, second)
}

// Profiles returns the deterministic catalog order.
func (catalog Catalog) Profiles() []ProfileDescriptor {
	result := make([]ProfileDescriptor, len(catalog.profiles))
	for index, profile := range catalog.profiles {
		result[index], _ = NewProfileDescriptor(profile.Spec())
	}
	return result
}

// Lookup returns an independent exact profile by stable ID.
func (catalog Catalog) Lookup(profileID string) (ProfileDescriptor, bool) {
	index := sort.Search(len(catalog.profiles), func(index int) bool {
		return catalog.profiles[index].ID() >= profileID
	})
	if index >= len(catalog.profiles) || catalog.profiles[index].ID() != profileID {
		return ProfileDescriptor{}, false
	}
	profile, err := NewProfileDescriptor(catalog.profiles[index].Spec())
	if err != nil {
		return ProfileDescriptor{}, false
	}
	return profile, true
}
