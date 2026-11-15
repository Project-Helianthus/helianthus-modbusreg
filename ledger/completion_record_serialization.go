package ledger

import "fmt"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

type completionRecordDTO struct {
	SchemaID                      string                `json:"schema_id"`
	SchemaVersion                 uint16                `json:"schema_version"`
	Disposition                   CompletionDisposition `json:"disposition"`
	DocsEvidenceSHA               string                `json:"docs_evidence_sha"`
	M302MergeSHA                  string                `json:"m3_02_merge_sha"`
	OfficialModelsSHA             string                `json:"official_models_sha"`
	ReadOnly                      bool                  `json:"read_only"`
	TransportNeutral              bool                  `json:"transport_neutral"`
	OverlayPresent                bool                  `json:"overlay_present"`
	WriteCapable                  bool                  `json:"write_capable"`
	AutomaticProductQualification bool                  `json:"automatic_product_qualification"`
	StandardProfileID             string                `json:"standard_profile_id"`
	StandardProfileVersion        string                `json:"standard_profile_version"`
	Applicability                 []string              `json:"applicability"`
	Limitations                   []string              `json:"limitations"`
	InvalidationRollback          []string              `json:"invalidation_rollback"`
}

func completionRecordToDTO(record CompletionRecord) completionRecordDTO {
	spec := record.Spec()
	return completionRecordDTO{
		SchemaID:                      spec.SchemaID,
		SchemaVersion:                 spec.SchemaVersion,
		Disposition:                   spec.Disposition,
		DocsEvidenceSHA:               spec.Evidence.DocsEvidenceSHA,
		M302MergeSHA:                  spec.Evidence.M302MergeSHA,
		OfficialModelsSHA:             spec.Evidence.OfficialModelsSHA,
		ReadOnly:                      spec.ReadOnly,
		TransportNeutral:              spec.TransportNeutral,
		OverlayPresent:                spec.OverlayPresent,
		WriteCapable:                  spec.WriteCapable,
		AutomaticProductQualification: spec.AutomaticProductQualification,
		StandardProfileID:             spec.StandardProfileID,
		StandardProfileVersion:        spec.StandardProfileVersion,
		Applicability:                 spec.Applicability,
		Limitations:                   spec.Limitations,
		InvalidationRollback:          spec.InvalidationRollback,
	}
}

func completionRecordFromDTO(record completionRecordDTO) (CompletionRecord, error) {
	return NewFMV3M303CompletionRecord(CompletionRecordSpec{
		SchemaID:                      record.SchemaID,
		SchemaVersion:                 record.SchemaVersion,
		Disposition:                   record.Disposition,
		Evidence:                      CompletionEvidenceRefs{DocsEvidenceSHA: record.DocsEvidenceSHA, M302MergeSHA: record.M302MergeSHA, OfficialModelsSHA: record.OfficialModelsSHA},
		ReadOnly:                      record.ReadOnly,
		TransportNeutral:              record.TransportNeutral,
		OverlayPresent:                record.OverlayPresent,
		WriteCapable:                  record.WriteCapable,
		AutomaticProductQualification: record.AutomaticProductQualification,
		StandardProfileID:             record.StandardProfileID,
		StandardProfileVersion:        record.StandardProfileVersion,
		Applicability:                 record.Applicability,
		Limitations:                   record.Limitations,
		InvalidationRollback:          record.InvalidationRollback,
	})
}

// MarshalFMV3M303CompletionRecord emits deterministic validated JSON.
func MarshalFMV3M303CompletionRecord(record CompletionRecord) ([]byte, error) {
	if _, err := NewFMV3M303CompletionRecord(record.Spec()); err != nil {
		return nil, err
	}
	return modbusreg.MarshalBounded(completionRecordToDTO(record))
}

// UnmarshalFMV3M303CompletionRecord rejects unknown fields and incompatible
// schemas before constructing an immutable record.
func UnmarshalFMV3M303CompletionRecord(data []byte) (CompletionRecord, error) {
	var dto completionRecordDTO
	if err := modbusreg.DecodeStrict(data, &dto); err != nil {
		return CompletionRecord{}, err
	}
	return completionRecordFromDTO(dto)
}

// MarshalJSON implements json.Marshaler through
// MarshalFMV3M303CompletionRecord, so the record is revalidated and its
// snake_case encoding is deterministic.
func (record CompletionRecord) MarshalJSON() ([]byte, error) {
	return MarshalFMV3M303CompletionRecord(record)
}

// UnmarshalJSON implements json.Unmarshaler through
// UnmarshalFMV3M303CompletionRecord. A nil target, an unknown field or a
// noncanonical record returns an error and leaves the target unchanged.
func (record *CompletionRecord) UnmarshalJSON(data []byte) error {
	if record == nil {
		return errNilCompletionRecordTarget
	}
	decoded, err := UnmarshalFMV3M303CompletionRecord(data)
	if err != nil {
		return err
	}
	*record = decoded
	return nil
}

var errNilCompletionRecordTarget = completionRecordError("completion record target is nil")

type completionRecordError string

func (errorValue completionRecordError) Error() string { return string(errorValue) }

type attemptIdentityDTO struct {
	PollGenerationID uint64 `json:"poll_generation_id"`
	RetryOrdinal     uint32 `json:"retry_ordinal"`
}

type sampleLedgerDTO struct {
	SchemaVersion        string             `json:"schema_version"`
	IssuerDomain         string             `json:"issuer_domain"`
	ProfileID            string             `json:"profile_id"`
	ProfileVersion       string             `json:"profile_version"`
	DependencySetID      string             `json:"dependency_set_id"`
	Revision             uint64             `json:"revision"`
	HighWater            uint64             `json:"high_water"`
	LastCommittedAttempt attemptIdentityDTO `json:"last_committed_attempt"`
}

// MarshalSampleLedgerState emits deterministic explicit restart state.
func MarshalSampleLedgerState(state SampleLedgerState) ([]byte, error) {
	if err := validateSampleLedgerState(state, 0); err != nil {
		return nil, err
	}
	return modbusreg.MarshalBounded(sampleLedgerDTO{
		SchemaVersion:   state.SchemaVersion.String(),
		IssuerDomain:    state.IssuerDomain,
		ProfileID:       state.ProfileID,
		ProfileVersion:  state.ProfileVersion.String(),
		DependencySetID: state.DependencySetID,
		Revision:        state.Revision,
		HighWater:       state.HighWater,
		LastCommittedAttempt: attemptIdentityDTO{
			PollGenerationID: state.LastCommittedAttempt.PollGenerationID,
			RetryOrdinal:     state.LastCommittedAttempt.RetryOrdinal,
		},
	})
}

// UnmarshalSampleLedgerState validates persisted restart state.
func UnmarshalSampleLedgerState(data []byte) (SampleLedgerState, error) {
	var record sampleLedgerDTO
	if err := modbusreg.DecodeStrict(data, &record); err != nil {
		return SampleLedgerState{}, err
	}
	if record.SchemaVersion != modbusreg.CurrentSchemaVersion().String() {
		return SampleLedgerState{}, fmt.Errorf("sample ledger schema is incompatible")
	}
	profileVersion, err := modbusreg.ParseRequiredVersion(
		"profile_version",
		record.ProfileVersion,
	)
	if err != nil {
		return SampleLedgerState{}, err
	}
	state := SampleLedgerState{
		SchemaVersion:   modbusreg.CurrentSchemaVersion(),
		IssuerDomain:    record.IssuerDomain,
		ProfileID:       record.ProfileID,
		ProfileVersion:  profileVersion,
		DependencySetID: record.DependencySetID,
		Revision:        record.Revision,
		HighWater:       record.HighWater,
		LastCommittedAttempt: modbusreg.AttemptIdentity{
			PollGenerationID: record.LastCommittedAttempt.PollGenerationID,
			RetryOrdinal:     record.LastCommittedAttempt.RetryOrdinal,
		},
	}
	if err := validateSampleLedgerState(state, 0); err != nil {
		return SampleLedgerState{}, err
	}
	return state, nil
}
