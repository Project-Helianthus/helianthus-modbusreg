package modbusreg

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
	return marshalBounded(completionRecordToDTO(record))
}

// UnmarshalFMV3M303CompletionRecord rejects unknown fields and incompatible
// schemas before constructing an immutable record.
func UnmarshalFMV3M303CompletionRecord(data []byte) (CompletionRecord, error) {
	var dto completionRecordDTO
	if err := decodeStrict(data, &dto); err != nil {
		return CompletionRecord{}, err
	}
	return completionRecordFromDTO(dto)
}

func (record CompletionRecord) MarshalJSON() ([]byte, error) {
	return MarshalFMV3M303CompletionRecord(record)
}

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
