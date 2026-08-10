package modbusreg

import (
	"fmt"
	"slices"
)

const (
	// FMV3M303CompletionSchemaID identifies the immutable phase-one decision record.
	FMV3M303CompletionSchemaID = "helianthus.fmv3-m3-03-completion.v2"
	// FMV3M303CompletionSchemaVersion is the only compatible completion schema.
	FMV3M303CompletionSchemaVersion uint16 = 2

	fmv3M303StandardProfileID      = "sunspec.phase1"
	fmv3M303StandardProfileVersion = "1.0.0"
	fmv3M303DocsEvidenceSHA        = "59218d21163acb868687ed3d8196f0aa1496aab7"
	fmv3M303M302MergeSHA           = "867c8275c090d3c703a9638548b48ea6846e8c56"
	fmv3M303OfficialModelsSHA      = "7abdf8982d5364f8ae916deee18aac86c11be36d"
)

var (
	fmv3M303CanonicalApplicability = []string{
		"qualified documentary GEN24 Primo/Symo ROW int+SF boundary requires runtime chain discovery",
	}
	fmv3M303CanonicalLimitations = []string{
		"Verto: UNKNOWN",
		"Tauro: UNKNOWN",
		"older Datamanager: UNKNOWN",
		"SnapINverter: UNKNOWN",
		"live installations: UNKNOWN",
	}
	fmv3M303CanonicalInvalidationRollback = []string{
		"evidence change invalidates decision",
		"retain standard SunSpec/raw access",
		"no automatic side effect",
	}
)

// CompletionDisposition is the closed decision set for a completion record.
type CompletionDisposition string

const (
	CompletionDispositionStandardOnly    CompletionDisposition = "STANDARD_ONLY"
	CompletionDispositionOverlayRequired CompletionDisposition = "OVERLAY_REQUIRED"
)

// CompletionEvidenceRefs identifies the immutable public evidence supporting a
// completion decision.
type CompletionEvidenceRefs struct {
	DocsEvidenceSHA   string
	M302MergeSHA      string
	OfficialModelsSHA string
}

// CompletionRecordSpec is the validated construction input for a completion
// record. The constructor retains defensive copies of all collection fields.
type CompletionRecordSpec struct {
	SchemaID                      string
	SchemaVersion                 uint16
	Disposition                   CompletionDisposition
	Evidence                      CompletionEvidenceRefs
	ReadOnly                      bool
	TransportNeutral              bool
	OverlayPresent                bool
	WriteCapable                  bool
	AutomaticProductQualification bool
	StandardProfileID             string
	StandardProfileVersion        string
	Applicability                 []string
	Limitations                   []string
	InvalidationRollback          []string
}

// CompletionRecord is an immutable public record of an evidence-qualified
// profile disposition.
type CompletionRecord struct{ spec CompletionRecordSpec }

// NewFMV3M303CompletionRecord validates one immutable completion record.
func NewFMV3M303CompletionRecord(spec CompletionRecordSpec) (CompletionRecord, error) {
	if err := validateFMV3M303CompletionRecord(spec); err != nil {
		return CompletionRecord{}, err
	}
	spec.Applicability = cloneStrings(spec.Applicability)
	spec.Limitations = cloneStrings(spec.Limitations)
	spec.InvalidationRollback = cloneStrings(spec.InvalidationRollback)
	return CompletionRecord{spec: spec}, nil
}

// NewCurrentFMV3M303CompletionRecord returns the canonical STANDARD_ONLY
// record for the current evidence-qualified phase-one boundary.
func NewCurrentFMV3M303CompletionRecord() (CompletionRecord, error) {
	return NewFMV3M303CompletionRecord(CompletionRecordSpec{
		SchemaID:      FMV3M303CompletionSchemaID,
		SchemaVersion: FMV3M303CompletionSchemaVersion,
		Disposition:   CompletionDispositionStandardOnly,
		Evidence: CompletionEvidenceRefs{
			DocsEvidenceSHA:   fmv3M303DocsEvidenceSHA,
			M302MergeSHA:      fmv3M303M302MergeSHA,
			OfficialModelsSHA: fmv3M303OfficialModelsSHA,
		},
		ReadOnly:                      true,
		TransportNeutral:              true,
		OverlayPresent:                false,
		WriteCapable:                  false,
		AutomaticProductQualification: false,
		StandardProfileID:             fmv3M303StandardProfileID,
		StandardProfileVersion:        fmv3M303StandardProfileVersion,
		Applicability:                 cloneStrings(fmv3M303CanonicalApplicability),
		Limitations:                   cloneStrings(fmv3M303CanonicalLimitations),
		InvalidationRollback:          cloneStrings(fmv3M303CanonicalInvalidationRollback),
	})
}

func validateFMV3M303CompletionRecord(spec CompletionRecordSpec) error {
	if spec.SchemaID != FMV3M303CompletionSchemaID ||
		spec.SchemaVersion != FMV3M303CompletionSchemaVersion {
		return fmt.Errorf("unsupported completion record schema")
	}
	if spec.Disposition != CompletionDispositionStandardOnly &&
		spec.Disposition != CompletionDispositionOverlayRequired {
		return fmt.Errorf("unknown completion disposition")
	}
	if spec.Disposition == CompletionDispositionOverlayRequired {
		return fmt.Errorf("completion disposition is reserved by the current evidence contract")
	}
	if !spec.ReadOnly || !spec.TransportNeutral || spec.WriteCapable ||
		spec.AutomaticProductQualification {
		return fmt.Errorf("completion record exceeds read-only neutral boundary")
	}
	if spec.OverlayPresent {
		return fmt.Errorf("completion disposition and overlay presence disagree")
	}
	if spec.StandardProfileID != fmv3M303StandardProfileID ||
		spec.StandardProfileVersion != fmv3M303StandardProfileVersion {
		return fmt.Errorf("completion record standard profile identity is incompatible")
	}
	if spec.Evidence.DocsEvidenceSHA != fmv3M303DocsEvidenceSHA ||
		spec.Evidence.M302MergeSHA != fmv3M303M302MergeSHA ||
		spec.Evidence.OfficialModelsSHA != fmv3M303OfficialModelsSHA {
		return fmt.Errorf("completion record evidence is not canonical")
	}
	if !slices.Equal(spec.Applicability, fmv3M303CanonicalApplicability) ||
		!slices.Equal(spec.Limitations, fmv3M303CanonicalLimitations) ||
		!slices.Equal(spec.InvalidationRollback, fmv3M303CanonicalInvalidationRollback) {
		return fmt.Errorf("completion record conclusion content is not canonical")
	}
	return nil
}

// Spec returns a defensive-copy construction view of the record.
func (record CompletionRecord) Spec() CompletionRecordSpec {
	spec := record.spec
	spec.Applicability = cloneStrings(spec.Applicability)
	spec.Limitations = cloneStrings(spec.Limitations)
	spec.InvalidationRollback = cloneStrings(spec.InvalidationRollback)
	return spec
}

func (record CompletionRecord) SchemaID() string      { return record.spec.SchemaID }
func (record CompletionRecord) SchemaVersion() uint16 { return record.spec.SchemaVersion }
func (record CompletionRecord) Disposition() CompletionDisposition {
	return record.spec.Disposition
}
func (record CompletionRecord) Evidence() CompletionEvidenceRefs { return record.spec.Evidence }
func (record CompletionRecord) ReadOnly() bool                   { return record.spec.ReadOnly }
func (record CompletionRecord) TransportNeutral() bool           { return record.spec.TransportNeutral }
func (record CompletionRecord) OverlayPresent() bool             { return record.spec.OverlayPresent }
func (record CompletionRecord) WriteCapable() bool               { return record.spec.WriteCapable }
func (record CompletionRecord) AutomaticProductQualification() bool {
	return record.spec.AutomaticProductQualification
}
func (record CompletionRecord) StandardProfileID() string { return record.spec.StandardProfileID }
func (record CompletionRecord) StandardProfileVersion() string {
	return record.spec.StandardProfileVersion
}
func (record CompletionRecord) Applicability() []string {
	return cloneStrings(record.spec.Applicability)
}
func (record CompletionRecord) Limitations() []string { return cloneStrings(record.spec.Limitations) }
func (record CompletionRecord) InvalidationRollback() []string {
	return cloneStrings(record.spec.InvalidationRollback)
}
