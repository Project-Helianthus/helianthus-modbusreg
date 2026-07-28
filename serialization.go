package modbusreg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type dependencySetDTO struct {
	ID           string           `json:"id"`
	Version      string           `json:"version"`
	Dependencies []DependencySpec `json:"dependencies"`
}

type coherenceDTO struct {
	Version                      string           `json:"version"`
	Mode                         CoherenceMode    `json:"mode"`
	MaximumSourceSkewNS          int64            `json:"maximum_source_skew_ns"`
	MaximumReceiptSkewNS         int64            `json:"maximum_receipt_skew_ns"`
	RequireGenerationEquality    bool             `json:"require_generation_equality"`
	AcquisitionOrder             AcquisitionOrder `json:"acquisition_order"`
	RetrySetBehavior             RetrySetBehavior `json:"retry_set_behavior"`
	DocumentaryConsistencyMarker string           `json:"documentary_consistency_marker"`
}

type profileDTO struct {
	SchemaVersion          string                   `json:"schema_version"`
	ID                     string                   `json:"profile_id"`
	Version                string                   `json:"profile_version"`
	Kind                   ProfileKind              `json:"profile_kind"`
	StandardApplicability  []string                 `json:"standard_applicability"`
	ModelApplicability     []string                 `json:"model_applicability"`
	VendorApplicability    []string                 `json:"vendor_applicability"`
	KnownExclusions        []string                 `json:"known_exclusions"`
	RuntimeContractVersion string                   `json:"runtime_contract_version"`
	DetectorVersion        string                   `json:"detector_version"`
	CodecContractVersion   string                   `json:"codec_contract_version"`
	NormalizationVersion   string                   `json:"normalization_version"`
	CoherenceVersion       string                   `json:"coherence_version"`
	QualificationVersion   string                   `json:"qualification_version"`
	Codecs                 []CodecSpec              `json:"codecs"`
	DependencySet          dependencySetDTO         `json:"dependency_set"`
	Coherence              coherenceDTO             `json:"coherence"`
	Evidence               []EvidenceReference      `json:"evidence"`
	Maturity               ProfileMaturity          `json:"maturity"`
	DefaultEnabled         bool                     `json:"default_enabled"`
	State                  ProfileState             `json:"state"`
	RefinesProfileID       string                   `json:"refines_profile_id,omitempty"`
	RefinesProfileVersion  *string                  `json:"refines_profile_version,omitempty"`
	SupersededByID         string                   `json:"superseded_by_id,omitempty"`
	SupersededByVersion    *string                  `json:"superseded_by_version,omitempty"`
	OverlayDeltas          []VendorOverlayDeltaSpec `json:"overlay_deltas"`
}

func versionPointer(version Version) *string {
	if !version.valid() {
		return nil
	}
	value := version.String()
	return &value
}

func profileToDTO(profile ProfileDescriptor) (profileDTO, error) {
	validated, err := NewProfileDescriptor(profile.Spec())
	if err != nil {
		return profileDTO{}, err
	}
	spec := validated.Spec()
	codecs := make([]CodecSpec, len(spec.Codecs))
	for index, codec := range spec.Codecs {
		codecs[index] = codec.Spec()
	}
	dependencies := spec.Dependencies.Dependencies()
	dependencySpecs := make([]DependencySpec, len(dependencies))
	for index, dependency := range dependencies {
		dependencySpecs[index] = dependency.Spec()
	}
	return profileDTO{
		SchemaVersion:          spec.SchemaVersion.String(),
		ID:                     spec.ID,
		Version:                spec.Version.String(),
		Kind:                   spec.Kind,
		StandardApplicability:  cloneStrings(spec.StandardApplicability),
		ModelApplicability:     cloneStrings(spec.ModelApplicability),
		VendorApplicability:    cloneStrings(spec.VendorApplicability),
		KnownExclusions:        cloneStrings(spec.KnownExclusions),
		RuntimeContractVersion: spec.RuntimeContractVersion.String(),
		DetectorVersion:        spec.DetectorVersion.String(),
		CodecContractVersion:   spec.CodecContractVersion.String(),
		NormalizationVersion:   spec.NormalizationVersion.String(),
		CoherenceVersion:       spec.CoherenceVersion.String(),
		QualificationVersion:   spec.QualificationVersion.String(),
		Codecs:                 codecs,
		DependencySet: dependencySetDTO{
			ID:           spec.Dependencies.ID(),
			Version:      spec.Dependencies.Version().String(),
			Dependencies: dependencySpecs,
		},
		Coherence: coherenceDTO{
			Version:                      spec.Coherence.Version.String(),
			Mode:                         spec.Coherence.Mode,
			MaximumSourceSkewNS:          int64(spec.Coherence.MaximumSourceSkew),
			MaximumReceiptSkewNS:         int64(spec.Coherence.MaximumReceiptSkew),
			RequireGenerationEquality:    spec.Coherence.RequireGenerationEquality,
			AcquisitionOrder:             spec.Coherence.AcquisitionOrder,
			RetrySetBehavior:             spec.Coherence.RetrySetBehavior,
			DocumentaryConsistencyMarker: spec.Coherence.DocumentaryConsistencyMarker,
		},
		Evidence:              append([]EvidenceReference(nil), spec.Evidence...),
		Maturity:              spec.Maturity,
		DefaultEnabled:        spec.DefaultEnabled,
		State:                 spec.State,
		RefinesProfileID:      spec.RefinesProfileID,
		RefinesProfileVersion: versionPointer(spec.RefinesProfileVersion),
		SupersededByID:        spec.SupersededByID,
		SupersededByVersion:   versionPointer(spec.SupersededByVersion),
		OverlayDeltas:         cloneOverlayDeltas(spec.OverlayDeltas),
	}, nil
}

func cloneOverlayDeltas(
	deltas []VendorOverlayDeltaSpec,
) []VendorOverlayDeltaSpec {
	if deltas == nil {
		return nil
	}
	result := make([]VendorOverlayDeltaSpec, len(deltas))
	for index, delta := range deltas {
		result[index] = cloneOverlayDelta(delta)
	}
	return result
}

func parseRequiredVersion(field, value string) (Version, error) {
	version, err := ParseVersion(value)
	if err != nil {
		return Version{}, fmt.Errorf("%s: %w", field, err)
	}
	return version, nil
}

func parseOptionalVersion(field string, value *string) (Version, error) {
	if value == nil {
		return Version{}, nil
	}
	return parseRequiredVersion(field, *value)
}

func profileFromDTO(record profileDTO) (ProfileDescriptor, error) {
	schema, err := parseRequiredVersion("schema_version", record.SchemaVersion)
	if err != nil || schema != schemaVersionV1 {
		return ProfileDescriptor{}, fmt.Errorf("profile schema is incompatible")
	}
	profileVersion, err := parseRequiredVersion("profile_version", record.Version)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	runtimeVersion, err := parseRequiredVersion(
		"runtime_contract_version",
		record.RuntimeContractVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	detectorVersion, err := parseRequiredVersion(
		"detector_version",
		record.DetectorVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	codecContractVersion, err := parseRequiredVersion(
		"codec_contract_version",
		record.CodecContractVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	normalizationVersion, err := parseRequiredVersion(
		"normalization_version",
		record.NormalizationVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	coherenceVersion, err := parseRequiredVersion(
		"coherence_version",
		record.CoherenceVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	qualificationVersion, err := parseRequiredVersion(
		"qualification_version",
		record.QualificationVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	var dependencySetVersion Version
	var coherencePolicyVersion Version
	if record.Kind == ProfileVendorOverlay &&
		record.DependencySet.Version == "" &&
		record.Coherence.Version == "" {
		dependencySetVersion = Version{}
		coherencePolicyVersion = Version{}
	} else {
		dependencySetVersion, err = parseRequiredVersion(
			"dependency_set.version",
			record.DependencySet.Version,
		)
		if err != nil {
			return ProfileDescriptor{}, err
		}
		coherencePolicyVersion, err = parseRequiredVersion(
			"coherence.version",
			record.Coherence.Version,
		)
		if err != nil {
			return ProfileDescriptor{}, err
		}
	}
	refinesVersion, err := parseOptionalVersion(
		"refines_profile_version",
		record.RefinesProfileVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	supersededVersion, err := parseOptionalVersion(
		"superseded_by_version",
		record.SupersededByVersion,
	)
	if err != nil {
		return ProfileDescriptor{}, err
	}
	codecs := make([]Codec, len(record.Codecs))
	for index, codecSpec := range record.Codecs {
		codecs[index], err = NewCodec(codecSpec)
		if err != nil {
			return ProfileDescriptor{}, fmt.Errorf("codec %d: %w", index, err)
		}
	}
	dependencies := make([]Dependency, len(record.DependencySet.Dependencies))
	for index, dependencySpec := range record.DependencySet.Dependencies {
		dependencies[index], err = NewDependency(dependencySpec)
		if err != nil {
			return ProfileDescriptor{}, fmt.Errorf("dependency %d: %w", index, err)
		}
	}
	var dependencySet DependencySet
	if record.Kind == ProfileVendorOverlay {
		if record.DependencySet.ID != "" || len(dependencies) != 0 {
			return ProfileDescriptor{}, fmt.Errorf("vendor overlay copied a dependency set")
		}
	} else {
		dependencySet, err = NewDependencySet(
			dependencySetVersion,
			dependencies,
		)
		if err != nil {
			return ProfileDescriptor{}, err
		}
		if dependencySet.ID() != record.DependencySet.ID {
			return ProfileDescriptor{}, fmt.Errorf("dependency-set identity changed")
		}
	}
	return NewProfileDescriptor(ProfileDescriptorSpec{
		SchemaVersion:          schema,
		ID:                     record.ID,
		Version:                profileVersion,
		Kind:                   record.Kind,
		StandardApplicability:  cloneStrings(record.StandardApplicability),
		ModelApplicability:     cloneStrings(record.ModelApplicability),
		VendorApplicability:    cloneStrings(record.VendorApplicability),
		KnownExclusions:        cloneStrings(record.KnownExclusions),
		RuntimeContractVersion: runtimeVersion,
		DetectorVersion:        detectorVersion,
		CodecContractVersion:   codecContractVersion,
		NormalizationVersion:   normalizationVersion,
		CoherenceVersion:       coherenceVersion,
		QualificationVersion:   qualificationVersion,
		Codecs:                 codecs,
		Dependencies:           dependencySet,
		Coherence: CoherencePolicySpec{
			Version:                      coherencePolicyVersion,
			Mode:                         record.Coherence.Mode,
			MaximumSourceSkew:            time.Duration(record.Coherence.MaximumSourceSkewNS),
			MaximumReceiptSkew:           time.Duration(record.Coherence.MaximumReceiptSkewNS),
			RequireGenerationEquality:    record.Coherence.RequireGenerationEquality,
			AcquisitionOrder:             record.Coherence.AcquisitionOrder,
			RetrySetBehavior:             record.Coherence.RetrySetBehavior,
			DocumentaryConsistencyMarker: record.Coherence.DocumentaryConsistencyMarker,
		},
		Evidence:              append([]EvidenceReference(nil), record.Evidence...),
		Maturity:              record.Maturity,
		DefaultEnabled:        record.DefaultEnabled,
		State:                 record.State,
		RefinesProfileID:      record.RefinesProfileID,
		RefinesProfileVersion: refinesVersion,
		SupersededByID:        record.SupersededByID,
		SupersededByVersion:   supersededVersion,
		OverlayDeltas:         cloneOverlayDeltas(record.OverlayDeltas),
	})
}

func decodeStrict(data []byte, target any) error {
	if err := preflightJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("serialized record contains multiple values")
		}
		return err
	}
	return nil
}

func marshalBounded(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaxSerializedContractBytes {
		return nil, fmt.Errorf("serialized contract exceeds the byte boundary")
	}
	return encoded, nil
}

// MarshalProfileDescriptor emits deterministic validated profile bytes.
func MarshalProfileDescriptor(profile ProfileDescriptor) ([]byte, error) {
	record, err := profileToDTO(profile)
	if err != nil {
		return nil, err
	}
	return marshalBounded(record)
}

// UnmarshalProfileDescriptor rejects unknown fields and incompatible schemas.
func UnmarshalProfileDescriptor(data []byte) (ProfileDescriptor, error) {
	var record profileDTO
	if err := decodeStrict(data, &record); err != nil {
		return ProfileDescriptor{}, err
	}
	return profileFromDTO(record)
}

// MarshalJSON gives construction specs a lossless validated representation.
func (spec ProfileDescriptorSpec) MarshalJSON() ([]byte, error) {
	profile, err := NewProfileDescriptor(spec)
	if err != nil {
		return nil, err
	}
	return MarshalProfileDescriptor(profile)
}

// UnmarshalJSON reconstructs a validated profile construction spec.
func (spec *ProfileDescriptorSpec) UnmarshalJSON(data []byte) error {
	profile, err := UnmarshalProfileDescriptor(data)
	if err != nil {
		return err
	}
	*spec = profile.Spec()
	return nil
}

// MarshalJSON serializes an immutable profile through its validated DTO.
func (profile ProfileDescriptor) MarshalJSON() ([]byte, error) {
	return MarshalProfileDescriptor(profile)
}

// UnmarshalJSON reconstructs an immutable validated profile.
func (profile *ProfileDescriptor) UnmarshalJSON(data []byte) error {
	decoded, err := UnmarshalProfileDescriptor(data)
	if err != nil {
		return err
	}
	*profile = decoded
	return nil
}

type sourceTimeDTO struct {
	State SourceTimeState `json:"state"`
	Time  string          `json:"time,omitempty"`
}

type dependencyResultDTO struct {
	DependencyID                 string               `json:"dependency_id"`
	DependencyVersion            string               `json:"dependency_version"`
	CodecID                      string               `json:"codec_id"`
	CodecVersion                 string               `json:"codec_version"`
	NormalizationVersion         string               `json:"normalization_version"`
	Status                       DependencyReadStatus `json:"status"`
	View                         LogicalViewRecord    `json:"logical_view"`
	SourceTime                   sourceTimeDTO        `json:"source_time"`
	LocalReceiptTime             string               `json:"local_receipt_time,omitempty"`
	DocumentaryConsistencyMarker string               `json:"documentary_consistency_marker"`
	AcquisitionOrdinal           uint32               `json:"acquisition_ordinal"`
	RetryAttemptID               RetryAttemptID       `json:"retry_attempt_id"`
}

type observationDTO struct {
	SchemaVersion          string                `json:"schema_version"`
	RuntimeContractVersion string                `json:"runtime_contract_version"`
	ProfileID              string                `json:"profile_id"`
	ProfileVersion         string                `json:"profile_version"`
	CodecContractVersion   string                `json:"codec_contract_version"`
	DetectorVersion        string                `json:"detector_version"`
	NormalizationVersion   string                `json:"normalization_version"`
	CoherenceVersion       string                `json:"coherence_version"`
	QualificationVersion   string                `json:"qualification_version"`
	SampleID               string                `json:"sample_id"`
	RetryAttemptID         RetryAttemptID        `json:"retry_attempt_id"`
	PollGenerationID       uint64                `json:"poll_generation_id"`
	DependencySetID        string                `json:"dependency_set_id"`
	DependencySetVersion   string                `json:"dependency_set_version"`
	SourceValidity         SourceValidity        `json:"source_validity"`
	SourceTime             sourceTimeDTO         `json:"source_time"`
	LocalReceiptTime       string                `json:"local_receipt_time"`
	Endpoint               string                `json:"endpoint"`
	UnitID                 byte                  `json:"unit_id"`
	Dependencies           []dependencyResultDTO `json:"dependencies"`
}

func sourceTimeToDTO(source SourceTimeSpec) (sourceTimeDTO, error) {
	if err := validateSourceTime(source); err != nil {
		return sourceTimeDTO{}, err
	}
	record := sourceTimeDTO{State: source.State}
	if source.State == SourceTimeObservedState {
		value, err := canonicalTime(source.Time)
		if err != nil {
			return sourceTimeDTO{}, err
		}
		record.Time = value.Format(time.RFC3339Nano)
	}
	return record, nil
}

func sourceTimeFromDTO(record sourceTimeDTO) (SourceTimeSpec, error) {
	switch record.State {
	case SourceTimeObservedState:
		if record.Time == "" {
			return SourceTimeSpec{}, fmt.Errorf("observed source time is absent")
		}
		value, err := time.Parse(time.RFC3339Nano, record.Time)
		if err != nil {
			return SourceTimeSpec{}, err
		}
		return SourceTimeObserved(value), nil
	case SourceTimeUnavailableState:
		if record.Time != "" {
			return SourceTimeSpec{}, fmt.Errorf("unavailable source time has a value")
		}
		return SourceTimeUnavailable(), nil
	default:
		return SourceTimeSpec{}, fmt.Errorf("source-time state is unknown")
	}
}

func formatRequiredTime(value time.Time) (string, error) {
	canonical, err := canonicalTime(value)
	if err != nil {
		return "", err
	}
	return canonical.Format(time.RFC3339Nano), nil
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return canonicalTime(parsed)
}

func observationSpecToDTO(spec ObservationSpec) (observationDTO, error) {
	if err := preflightObservationSpec(spec); err != nil {
		return observationDTO{}, err
	}
	if err := canonicalizeObservationTimes(&spec); err != nil {
		return observationDTO{}, err
	}
	sourceTime, err := sourceTimeToDTO(spec.SourceTime)
	if err != nil {
		return observationDTO{}, err
	}
	receipt, err := formatRequiredTime(spec.LocalReceiptTime)
	if err != nil {
		return observationDTO{}, err
	}
	dependencies := make([]dependencyResultDTO, len(spec.Dependencies))
	for index, result := range spec.Dependencies {
		if !result.View.valid {
			return observationDTO{}, fmt.Errorf("dependency %d snapshot is invalid", index)
		}
		dependencySourceTime, err := sourceTimeToDTO(result.SourceTime)
		if err != nil {
			if result.SourceTime.State == "" && result.LocalReceiptTime.IsZero() {
				dependencySourceTime = sourceTimeDTO{}
			} else {
				return observationDTO{}, fmt.Errorf("dependency %d source time: %w", index, err)
			}
		}
		localReceipt := ""
		if !result.LocalReceiptTime.IsZero() {
			localReceipt = result.LocalReceiptTime.UTC().Format(time.RFC3339Nano)
		}
		dependencies[index] = dependencyResultDTO{
			DependencyID:                 result.DependencyID,
			DependencyVersion:            result.DependencyVersion.String(),
			CodecID:                      result.CodecID,
			CodecVersion:                 result.CodecVersion.String(),
			NormalizationVersion:         result.NormalizationVersion.String(),
			Status:                       result.Status,
			View:                         result.View.Record(),
			SourceTime:                   dependencySourceTime,
			LocalReceiptTime:             localReceipt,
			DocumentaryConsistencyMarker: result.DocumentaryConsistencyMarker,
			AcquisitionOrdinal:           result.AcquisitionOrdinal,
			RetryAttemptID:               result.RetryAttemptID,
		}
	}
	return observationDTO{
		SchemaVersion:          spec.SchemaVersion.String(),
		RuntimeContractVersion: spec.RuntimeContractVersion.String(),
		ProfileID:              spec.ProfileID,
		ProfileVersion:         spec.ProfileVersion.String(),
		CodecContractVersion:   spec.CodecContractVersion.String(),
		DetectorVersion:        spec.DetectorVersion.String(),
		NormalizationVersion:   spec.NormalizationVersion.String(),
		CoherenceVersion:       spec.CoherenceVersion.String(),
		QualificationVersion:   spec.QualificationVersion.String(),
		SampleID:               spec.SampleID,
		RetryAttemptID:         spec.RetryAttemptID,
		PollGenerationID:       spec.PollGenerationID,
		DependencySetID:        spec.DependencySetID,
		DependencySetVersion:   spec.DependencySetVersion.String(),
		SourceValidity:         spec.SourceValidity,
		SourceTime:             sourceTime,
		LocalReceiptTime:       receipt,
		Endpoint:               spec.Endpoint,
		UnitID:                 spec.UnitID,
		Dependencies:           dependencies,
	}, nil
}

func observationSpecFromDTO(record observationDTO) (ObservationSpec, error) {
	if record.SchemaVersion != schemaVersionV1.String() {
		return ObservationSpec{}, fmt.Errorf("observation schema is incompatible")
	}
	parse := func(field, value string) (Version, error) {
		return parseRequiredVersion(field, value)
	}
	schema, err := parse("schema_version", record.SchemaVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	runtimeVersion, err := parse("runtime_contract_version", record.RuntimeContractVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	profileVersion, err := parse("profile_version", record.ProfileVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	codecVersion, err := parse("codec_contract_version", record.CodecContractVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	detectorVersion, err := parse("detector_version", record.DetectorVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	normalizationVersion, err := parse(
		"normalization_version",
		record.NormalizationVersion,
	)
	if err != nil {
		return ObservationSpec{}, err
	}
	coherenceVersion, err := parse("coherence_version", record.CoherenceVersion)
	if err != nil {
		return ObservationSpec{}, err
	}
	qualificationVersion, err := parse(
		"qualification_version",
		record.QualificationVersion,
	)
	if err != nil {
		return ObservationSpec{}, err
	}
	dependencySetVersion, err := parse(
		"dependency_set_version",
		record.DependencySetVersion,
	)
	if err != nil {
		return ObservationSpec{}, err
	}
	sourceTime, err := sourceTimeFromDTO(record.SourceTime)
	if err != nil {
		return ObservationSpec{}, err
	}
	receipt, err := parseOptionalTime(record.LocalReceiptTime)
	if err != nil || receipt.IsZero() {
		return ObservationSpec{}, fmt.Errorf("observation receipt time is invalid")
	}
	dependencies := make([]DependencyResult, len(record.Dependencies))
	for index, dependency := range record.Dependencies {
		dependencyVersion, err := parse(
			"dependency_version",
			dependency.DependencyVersion,
		)
		if err != nil {
			return ObservationSpec{}, err
		}
		dependencyCodecVersion, err := parse("codec_version", dependency.CodecVersion)
		if err != nil {
			return ObservationSpec{}, err
		}
		dependencyNormalizationVersion, err := parse(
			"dependency_normalization_version",
			dependency.NormalizationVersion,
		)
		if err != nil {
			return ObservationSpec{}, err
		}
		snapshot, err := NewLogicalViewSnapshot(dependency.View)
		if err != nil {
			return ObservationSpec{}, fmt.Errorf("dependency %d view: %w", index, err)
		}
		var dependencySourceTime SourceTimeSpec
		if dependency.SourceTime.State != "" || dependency.SourceTime.Time != "" {
			dependencySourceTime, err = sourceTimeFromDTO(dependency.SourceTime)
			if err != nil {
				return ObservationSpec{}, err
			}
		}
		dependencyReceipt, err := parseOptionalTime(dependency.LocalReceiptTime)
		if err != nil {
			return ObservationSpec{}, err
		}
		dependencies[index] = DependencyResult{
			DependencyID:                 dependency.DependencyID,
			DependencyVersion:            dependencyVersion,
			CodecID:                      dependency.CodecID,
			CodecVersion:                 dependencyCodecVersion,
			NormalizationVersion:         dependencyNormalizationVersion,
			Status:                       dependency.Status,
			View:                         snapshot,
			SourceTime:                   dependencySourceTime,
			LocalReceiptTime:             dependencyReceipt,
			DocumentaryConsistencyMarker: dependency.DocumentaryConsistencyMarker,
			AcquisitionOrdinal:           dependency.AcquisitionOrdinal,
			RetryAttemptID:               dependency.RetryAttemptID,
		}
	}
	return ObservationSpec{
		SchemaVersion:          schema,
		RuntimeContractVersion: runtimeVersion,
		ProfileID:              record.ProfileID,
		ProfileVersion:         profileVersion,
		CodecContractVersion:   codecVersion,
		DetectorVersion:        detectorVersion,
		NormalizationVersion:   normalizationVersion,
		CoherenceVersion:       coherenceVersion,
		QualificationVersion:   qualificationVersion,
		SampleID:               record.SampleID,
		RetryAttemptID:         record.RetryAttemptID,
		PollGenerationID:       record.PollGenerationID,
		DependencySetID:        record.DependencySetID,
		DependencySetVersion:   dependencySetVersion,
		SourceValidity:         record.SourceValidity,
		SourceTime:             sourceTime,
		LocalReceiptTime:       receipt,
		Endpoint:               record.Endpoint,
		UnitID:                 record.UnitID,
		Dependencies:           dependencies,
	}, nil
}

// MarshalJSON makes logical-view snapshots lossless inside ObservationSpec.
func (spec ObservationSpec) MarshalJSON() ([]byte, error) {
	record, err := observationSpecToDTO(spec)
	if err != nil {
		return nil, err
	}
	return marshalBounded(record)
}

// UnmarshalJSON reconstructs validated logical-view snapshots.
func (spec *ObservationSpec) UnmarshalJSON(data []byte) error {
	var record observationDTO
	if err := decodeStrict(data, &record); err != nil {
		return err
	}
	decoded, err := observationSpecFromDTO(record)
	if err != nil {
		return err
	}
	*spec = decoded
	return nil
}

// MarshalObservation emits deterministic lossless observation bytes.
func MarshalObservation(observation Observation) ([]byte, error) {
	if observation.SampleID() == "" {
		return nil, fmt.Errorf("observation is invalid")
	}
	return marshalBounded(observation.Spec())
}

// MarshalJSON serializes an immutable admitted observation.
func (observation Observation) MarshalJSON() ([]byte, error) {
	return MarshalObservation(observation)
}

// UnmarshalObservation validates bytes and atomically admits the sample.
func (factory *ObservationFactory) UnmarshalObservation(
	data []byte,
) (SampleAdmission, error) {
	if factory == nil {
		return SampleAdmission{}, fmt.Errorf("observation factory is invalid")
	}
	var spec ObservationSpec
	if err := decodeStrict(data, &spec); err != nil {
		return SampleAdmission{}, err
	}
	observation, err := buildObservation(factory.profile, spec)
	if err != nil {
		return SampleAdmission{}, err
	}
	return factory.admitSerializedObservation(observation)
}

type sampleLedgerDTO struct {
	SchemaVersion   string `json:"schema_version"`
	IssuerDomain    string `json:"issuer_domain"`
	DependencySetID string `json:"dependency_set_id"`
	Revision        uint64 `json:"revision"`
	HighWater       uint64 `json:"high_water"`
}

// MarshalSampleLedgerState emits deterministic explicit restart state.
func MarshalSampleLedgerState(state SampleLedgerState) ([]byte, error) {
	if err := validateSampleLedgerState(state, 0); err != nil {
		return nil, err
	}
	return marshalBounded(sampleLedgerDTO{
		SchemaVersion:   state.SchemaVersion.String(),
		IssuerDomain:    state.IssuerDomain,
		DependencySetID: state.DependencySetID,
		Revision:        state.Revision,
		HighWater:       state.HighWater,
	})
}

// UnmarshalSampleLedgerState validates persisted restart state.
func UnmarshalSampleLedgerState(data []byte) (SampleLedgerState, error) {
	var record sampleLedgerDTO
	if err := decodeStrict(data, &record); err != nil {
		return SampleLedgerState{}, err
	}
	if record.SchemaVersion != schemaVersionV1.String() {
		return SampleLedgerState{}, fmt.Errorf("sample ledger schema is incompatible")
	}
	state := SampleLedgerState{
		SchemaVersion:   schemaVersionV1,
		IssuerDomain:    record.IssuerDomain,
		DependencySetID: record.DependencySetID,
		Revision:        record.Revision,
		HighWater:       record.HighWater,
	}
	if err := validateSampleLedgerState(state, 0); err != nil {
		return SampleLedgerState{}, err
	}
	return state, nil
}
