package modbusreg

import (
	"fmt"
	"sync"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// FunctionCode is the runtime Modbus operation identity.
type FunctionCode = modbus.FunctionCode

const (
	FunctionReadHoldingRegisters = modbus.FunctionReadHoldingRegisters
	FunctionReadInputRegisters   = modbus.FunctionReadInputRegisters
)

// TransportFamily is the runtime transport-generation identity.
type TransportFamily = modbus.TransportFamily

const (
	TransportTCP = modbus.TransportTCP
	TransportRTU = modbus.TransportRTU
)

// LogicalViewRecord is the immutable serialization input for one successful
// runtime logical view. It deliberately retains raw words and exact transport
// provenance without retaining sockets or transport owners.
type LogicalViewRecord struct {
	LogicalViewID       uint64
	WireResponseID      uint64
	PhysicalRequestID   uint64
	Endpoint            string
	ConnectionID        uint64
	Transport           TransportFamily
	TransportGeneration uint64
	UnitID              byte
	RequestedFunction   FunctionCode
	ReceivedFunction    FunctionCode
	Table               LogicalTable
	PhysicalOffset      uint16
	PhysicalWordCount   uint16
	AuthorizationScope  string
	PollGeneration      uint64
	DeadlineIdentity    uint64
	LogicalOffset       uint16
	LogicalWordCount    uint16
	SliceOffset         uint16
	SliceWordCount      uint16
	Words               []uint16
}

// LogicalViewSnapshot is an immutable transport-to-registry input snapshot.
type LogicalViewSnapshot struct {
	record LogicalViewRecord
	valid  bool
}

// CaptureLogicalView consumes the exact exported helianthus-modbus view and
// copies only the immutable facts required by the registry.
func CaptureLogicalView(view modbus.LogicalReadView) (LogicalViewSnapshot, error) {
	provenance := view.Provenance()
	wire := provenance.Wire
	return NewLogicalViewSnapshot(LogicalViewRecord{
		LogicalViewID:       view.LogicalViewID(),
		WireResponseID:      view.WireResponseID(),
		PhysicalRequestID:   provenance.PhysicalRequestID,
		Endpoint:            wire.Endpoint,
		ConnectionID:        wire.ConnectionID,
		Transport:           wire.Transport,
		TransportGeneration: wire.TransportGeneration,
		UnitID:              wire.UnitID,
		RequestedFunction:   wire.RequestedFunction,
		ReceivedFunction:    wire.ReceivedFunction,
		Table:               wire.Table,
		PhysicalOffset:      wire.Offset,
		PhysicalWordCount:   wire.Quantity,
		AuthorizationScope:  provenance.AuthorizationScope,
		PollGeneration:      provenance.PollGeneration,
		DeadlineIdentity:    provenance.DeadlineIdentity,
		LogicalOffset:       view.LogicalOffset(),
		LogicalWordCount:    view.LogicalWordCount(),
		SliceOffset:         view.SliceOffset(),
		SliceWordCount:      view.SliceWordCount(),
		Words:               view.Words(),
	})
}

// NewLogicalViewSnapshot validates checked slice arithmetic and all request,
// generation, endpoint, and operation identities.
func NewLogicalViewSnapshot(
	record LogicalViewRecord,
) (LogicalViewSnapshot, error) {
	record.Words = append([]uint16(nil), record.Words...)
	if record.LogicalViewID == 0 || record.WireResponseID == 0 ||
		record.PhysicalRequestID == 0 || record.Endpoint == "" ||
		record.TransportGeneration == 0 || record.UnitID == 0 ||
		record.UnitID > 247 || record.AuthorizationScope == "" ||
		record.PollGeneration == 0 || record.DeadlineIdentity == 0 ||
		record.PhysicalWordCount == 0 || record.LogicalWordCount == 0 ||
		record.SliceWordCount == 0 ||
		record.PhysicalWordCount > modbus.MaxReadRegisters ||
		record.LogicalWordCount > modbus.MaxReadRegisters {
		return LogicalViewSnapshot{}, fmt.Errorf("logical-view provenance is incomplete")
	}
	if record.Transport != TransportTCP && record.Transport != TransportRTU {
		return LogicalViewSnapshot{}, fmt.Errorf("logical-view transport is unknown")
	}
	if record.Transport == TransportTCP && record.ConnectionID == 0 {
		return LogicalViewSnapshot{}, fmt.Errorf("TCP logical view lacks connection identity")
	}
	expectedTable := LogicalTable("")
	switch record.RequestedFunction {
	case FunctionReadHoldingRegisters:
		expectedTable = HoldingRegisters
	case FunctionReadInputRegisters:
		expectedTable = InputRegisters
	default:
		return LogicalViewSnapshot{}, fmt.Errorf("logical view is not FC03 or FC04")
	}
	if record.ReceivedFunction != record.RequestedFunction ||
		record.Table != expectedTable {
		return LogicalViewSnapshot{}, fmt.Errorf("logical-view operation identity disagrees")
	}
	physicalEnd := uint32(record.PhysicalOffset) + uint32(record.PhysicalWordCount)
	sliceEnd := uint32(record.SliceOffset) + uint32(record.SliceWordCount)
	logicalEnd := uint32(record.LogicalOffset) + uint32(record.LogicalWordCount)
	if physicalEnd > 65536 || logicalEnd > 65536 ||
		sliceEnd > uint32(record.PhysicalWordCount) ||
		record.LogicalWordCount != record.SliceWordCount ||
		len(record.Words) != int(record.LogicalWordCount) ||
		uint32(record.PhysicalOffset)+uint32(record.SliceOffset) !=
			uint32(record.LogicalOffset) {
		return LogicalViewSnapshot{}, fmt.Errorf("logical-view slice is inconsistent")
	}
	return LogicalViewSnapshot{record: record, valid: true}, nil
}

// Record returns an independent serialized snapshot.
func (snapshot LogicalViewSnapshot) Record() LogicalViewRecord {
	record := snapshot.record
	record.Words = append([]uint16(nil), record.Words...)
	return record
}

// DependencyReadStatus classifies terminal dependency inputs.
type DependencyReadStatus string

const (
	DependencyReadSuccessful DependencyReadStatus = "successful"
	DependencyReadTorn       DependencyReadStatus = "torn"
	DependencyReadMalformed  DependencyReadStatus = "malformed"
	DependencyReadException  DependencyReadStatus = "exception"
)

// SourceValidity records the source's own validity fact.
type SourceValidity string

const (
	SourceValid          SourceValidity = "valid"
	SourceInvalid        SourceValidity = "invalid"
	SourceNotImplemented SourceValidity = "not_implemented"
	SourceReserved       SourceValidity = "reserved"
)

// SourceTimeState distinguishes a real source time from its documented absence.
type SourceTimeState string

const (
	SourceTimeObservedState    SourceTimeState = "observed"
	SourceTimeUnavailableState SourceTimeState = "unavailable"
)

// SourceTimeSpec retains source time without treating receipt time as a proxy.
type SourceTimeSpec struct {
	State SourceTimeState
	Time  time.Time
}

// SourceTimeObserved constructs an explicit source observation time.
func SourceTimeObserved(value time.Time) SourceTimeSpec {
	return SourceTimeSpec{State: SourceTimeObservedState, Time: value}
}

// SourceTimeUnavailable constructs an explicit no-source-time fact.
func SourceTimeUnavailable() SourceTimeSpec {
	return SourceTimeSpec{State: SourceTimeUnavailableState}
}

func validateSourceTime(sourceTime SourceTimeSpec) error {
	switch sourceTime.State {
	case SourceTimeObservedState:
		if sourceTime.Time.IsZero() {
			return fmt.Errorf("observed source time is absent")
		}
	case SourceTimeUnavailableState:
		if !sourceTime.Time.IsZero() {
			return fmt.Errorf("unavailable source time contains a guessed time")
		}
	default:
		return fmt.Errorf("source-time state is unknown")
	}
	return nil
}

// DependencyResult is one terminal dependency input for a profile sample.
type DependencyResult struct {
	DependencyID                 string
	DependencyVersion            Version
	CodecID                      string
	CodecVersion                 Version
	NormalizationVersion         Version
	Status                       DependencyReadStatus
	View                         LogicalViewSnapshot
	SourceTime                   SourceTimeSpec
	LocalReceiptTime             time.Time
	DocumentaryConsistencyMarker string
}

// ObservationSpec is the complete successful source-observation envelope.
type ObservationSpec struct {
	SchemaVersion          Version
	RuntimeContractVersion Version
	ProfileID              string
	ProfileVersion         Version
	CodecContractVersion   Version
	DetectorVersion        Version
	NormalizationVersion   Version
	CoherenceVersion       Version
	QualificationVersion   Version
	SampleID               string
	PollGenerationID       uint64
	DependencySetID        string
	DependencySetVersion   Version
	SourceValidity         SourceValidity
	SourceTime             SourceTimeSpec
	LocalReceiptTime       time.Time
	Endpoint               string
	UnitID                 byte
	Dependencies           []DependencyResult
}

// Observation is an immutable coherent profile sample.
type Observation struct {
	spec     ObservationSpec
	replayed []ReplayedDependency
}

// NewObservation validates exact versions, dependency order, provenance, and
// coherence. Any partial or non-success input yields no Observation.
func NewObservation(
	profile ProfileDescriptor,
	spec ObservationSpec,
) (Observation, error) {
	spec.Dependencies = append([]DependencyResult(nil), spec.Dependencies...)
	if profile.ID() == "" || spec.SchemaVersion != SchemaVersionV1 ||
		spec.RuntimeContractVersion != profile.RuntimeContractVersion() ||
		spec.ProfileID != profile.ID() ||
		spec.ProfileVersion != profile.Version() ||
		spec.CodecContractVersion != profile.CodecContractVersion() ||
		spec.DetectorVersion != profile.DetectorVersion() ||
		spec.NormalizationVersion != profile.NormalizationVersion() ||
		spec.CoherenceVersion != profile.CoherenceVersion() ||
		spec.QualificationVersion != profile.QualificationVersion() ||
		!validIdentity(spec.SampleID) || spec.PollGenerationID == 0 ||
		spec.DependencySetID != profile.Dependencies().ID() ||
		spec.DependencySetVersion != profile.Dependencies().Version() ||
		spec.Endpoint == "" || spec.UnitID == 0 || spec.UnitID > 247 ||
		spec.LocalReceiptTime.IsZero() {
		return Observation{}, fmt.Errorf("observation envelope is incomplete or mismatched")
	}
	switch spec.SourceValidity {
	case SourceValid, SourceInvalid, SourceNotImplemented, SourceReserved:
	default:
		return Observation{}, fmt.Errorf("source validity is unknown")
	}
	if err := validateSourceTime(spec.SourceTime); err != nil {
		return Observation{}, err
	}
	declarations := profile.Dependencies().Dependencies()
	if len(spec.Dependencies) != len(declarations) {
		return Observation{}, fmt.Errorf("observation dependency set is incomplete")
	}
	replayed := make([]ReplayedDependency, len(declarations))
	for index, declaration := range declarations {
		result := spec.Dependencies[index]
		if result.DependencyID != declaration.ID() ||
			result.DependencyVersion != declaration.Version() ||
			result.CodecID != declaration.CodecID() ||
			result.CodecVersion != declaration.CodecVersion() ||
			result.NormalizationVersion !=
				declaration.Normalization().Spec().Version ||
			result.Status != DependencyReadSuccessful ||
			!result.View.valid {
			return Observation{}, fmt.Errorf("dependency %d is absent or unsuccessful", index)
		}
		record := result.View.Record()
		expectedFunction := FunctionReadHoldingRegisters
		if declaration.Table() == InputRegisters {
			expectedFunction = FunctionReadInputRegisters
		}
		if record.Endpoint != spec.Endpoint || record.UnitID != spec.UnitID ||
			record.PollGeneration != spec.PollGenerationID ||
			record.Table != declaration.Table() ||
			record.RequestedFunction != expectedFunction ||
			record.LogicalOffset != declaration.Normalization().ResolvedPDUOffset() ||
			record.LogicalWordCount != declaration.WordCount() ||
			record.SliceWordCount != declaration.WordCount() ||
			len(record.Words) != int(declaration.WordCount()) {
			return Observation{}, fmt.Errorf("dependency %d provenance disagrees", index)
		}
		replayed[index] = ReplayedDependency{
			dependencyID:      declaration.ID(),
			dependencyVersion: declaration.Version(),
			codecID:           declaration.CodecID(),
			codecVersion:      declaration.CodecVersion(),
			normalization:     declaration.Normalization(),
			view:              result.View,
		}
	}
	if err := validateObservationCoherence(profile.spec.Coherence, spec, replayed); err != nil {
		return Observation{}, err
	}
	return Observation{spec: cloneObservationSpec(spec), replayed: replayed}, nil
}

func validateObservationCoherence(
	policy CoherencePolicySpec,
	spec ObservationSpec,
	dependencies []ReplayedDependency,
) error {
	if len(dependencies) == 0 {
		return fmt.Errorf("coherent sample is empty")
	}
	switch policy.Mode {
	case CoherenceSingleWireResponse:
		first := dependencies[0].view.record
		for _, dependency := range dependencies[1:] {
			record := dependency.view.record
			if record.WireResponseID != first.WireResponseID ||
				record.PhysicalRequestID != first.PhysicalRequestID ||
				record.Endpoint != first.Endpoint ||
				record.Transport != first.Transport ||
				record.TransportGeneration != first.TransportGeneration ||
				record.UnitID != first.UnitID ||
				record.RequestedFunction != first.RequestedFunction ||
				record.Table != first.Table ||
				record.PhysicalOffset != first.PhysicalOffset ||
				record.PhysicalWordCount != first.PhysicalWordCount {
				return fmt.Errorf("single-wire-response coherence failed")
			}
		}
	case CoherenceBoundedMultiResponse:
		var minimumSource, maximumSource time.Time
		var minimumReceipt, maximumReceipt time.Time
		for index := range dependencies {
			result := spec.Dependencies[index]
			if result.SourceTime.State != SourceTimeObservedState ||
				result.SourceTime.Time.IsZero() ||
				result.LocalReceiptTime.IsZero() {
				return fmt.Errorf("bounded response lacks per-dependency time facts")
			}
			if policy.DocumentaryConsistencyMarker != "" &&
				result.DocumentaryConsistencyMarker !=
					policy.DocumentaryConsistencyMarker {
				return fmt.Errorf("documentary consistency marker disagrees")
			}
			source := result.SourceTime.Time
			receipt := result.LocalReceiptTime
			if minimumSource.IsZero() || source.Before(minimumSource) {
				minimumSource = source
			}
			if maximumSource.IsZero() || source.After(maximumSource) {
				maximumSource = source
			}
			if minimumReceipt.IsZero() || receipt.Before(minimumReceipt) {
				minimumReceipt = receipt
			}
			if maximumReceipt.IsZero() || receipt.After(maximumReceipt) {
				maximumReceipt = receipt
			}
		}
		if maximumSource.Sub(minimumSource) > policy.MaximumSourceSkew ||
			maximumReceipt.Sub(minimumReceipt) > policy.MaximumReceiptSkew {
			return fmt.Errorf("bounded response exceeds declared skew")
		}
		if spec.SourceTime.State != SourceTimeObservedState ||
			!spec.SourceTime.Time.Equal(maximumSource) ||
			!spec.LocalReceiptTime.Equal(maximumReceipt) {
			return fmt.Errorf("bounded response envelope time disagrees")
		}
	default:
		return fmt.Errorf("coherence mode is unknown")
	}
	return nil
}

func cloneObservationSpec(spec ObservationSpec) ObservationSpec {
	spec.Dependencies = append([]DependencyResult(nil), spec.Dependencies...)
	for index := range spec.Dependencies {
		record := spec.Dependencies[index].View.Record()
		spec.Dependencies[index].View, _ = NewLogicalViewSnapshot(record)
	}
	return spec
}

// ReplayedDependency is one exact raw dependency and its retained provenance.
type ReplayedDependency struct {
	dependencyID      string
	dependencyVersion Version
	codecID           string
	codecVersion      Version
	normalization     AddressNormalization
	view              LogicalViewSnapshot
}

// RawWords returns an independent exact wire-order word copy.
func (dependency ReplayedDependency) RawWords() []uint16 {
	return dependency.view.Record().Words
}

// DependencyID returns the stable dependency identity.
func (dependency ReplayedDependency) DependencyID() string {
	return dependency.dependencyID
}

// DependencyVersion returns the exact dependency declaration version.
func (dependency ReplayedDependency) DependencyVersion() Version {
	return dependency.dependencyVersion
}

// CodecID returns the codec used for this dependency.
func (dependency ReplayedDependency) CodecID() string {
	return dependency.codecID
}

// CodecVersion returns the exact codec version used for this dependency.
func (dependency ReplayedDependency) CodecVersion() Version {
	return dependency.codecVersion
}

// LogicalViewID returns the opaque runtime logical-view identity.
func (dependency ReplayedDependency) LogicalViewID() uint64 {
	return dependency.view.record.LogicalViewID
}

// WireResponseID returns the shared physical response identity.
func (dependency ReplayedDependency) WireResponseID() uint64 {
	return dependency.view.record.WireResponseID
}

// LogicalOffset returns the exact zero-based dependent offset.
func (dependency ReplayedDependency) LogicalOffset() uint16 {
	return dependency.view.record.LogicalOffset
}

// LogicalWordCount returns the dependent width.
func (dependency ReplayedDependency) LogicalWordCount() uint16 {
	return dependency.view.record.LogicalWordCount
}

// SliceOffset returns the view start in the physical response.
func (dependency ReplayedDependency) SliceOffset() uint16 {
	return dependency.view.record.SliceOffset
}

// SliceWordCount returns the exact physical slice width.
func (dependency ReplayedDependency) SliceWordCount() uint16 {
	return dependency.view.record.SliceWordCount
}

// Normalization returns the retained documentary address record.
func (dependency ReplayedDependency) Normalization() AddressNormalization {
	return dependency.normalization
}

// Replay returns independent exact dependency observations in declaration order.
func (observation Observation) Replay() []ReplayedDependency {
	result := make([]ReplayedDependency, len(observation.replayed))
	copy(result, observation.replayed)
	return result
}

// Spec returns an independent complete source-observation envelope.
func (observation Observation) Spec() ObservationSpec {
	return cloneObservationSpec(observation.spec)
}

// SampleID returns the opaque coherent sample identity.
func (observation Observation) SampleID() string {
	return observation.spec.SampleID
}

type sampleBinding struct {
	pollGenerationID uint64
	dependencySetID  string
	profileVersion   Version
}

// SampleLedger rejects reuse of a sample ID across retries, generations, or
// dependency sets. It does not publish or assign canonical meaning.
type SampleLedger struct {
	mu      sync.Mutex
	samples map[string]sampleBinding
}

// NewSampleLedger constructs an empty in-memory identity guard.
func NewSampleLedger() *SampleLedger {
	return &SampleLedger{samples: make(map[string]sampleBinding)}
}

// Admit records a sample identity once and rejects every reuse.
func (ledger *SampleLedger) Admit(observation Observation) error {
	if ledger == nil || observation.SampleID() == "" {
		return fmt.Errorf("sample ledger input is invalid")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.samples[observation.spec.SampleID]; exists {
		return fmt.Errorf("sample ID was reused")
	}
	ledger.samples[observation.spec.SampleID] = sampleBinding{
		pollGenerationID: observation.spec.PollGenerationID,
		dependencySetID:  observation.spec.DependencySetID,
		profileVersion:   observation.spec.ProfileVersion,
	}
	return nil
}

// OwnershipBoundary documents that this package records source facts only.
func OwnershipBoundary() string {
	return "records immutable source facts; downstream composition owns consumer semantics"
}
