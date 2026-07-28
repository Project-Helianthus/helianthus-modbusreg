package modbusreg

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	if len(record.Words) > MaxRawWords {
		return LogicalViewSnapshot{}, fmt.Errorf("logical-view words exceed runtime maximum")
	}
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "logical-view endpoint", value: record.Endpoint},
		{
			name:  "logical-view authorization scope",
			value: record.AuthorizationScope,
		},
	}
	for _, field := range stringFields {
		if err := validateBoundedString(field.name, field.value, true); err != nil {
			return LogicalViewSnapshot{}, err
		}
	}
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
	if record.Transport == TransportRTU &&
		(record.ConnectionID != 0 ||
			record.PhysicalOffset != record.LogicalOffset ||
			record.PhysicalWordCount != record.LogicalWordCount ||
			record.SliceOffset != 0) {
		return LogicalViewSnapshot{}, fmt.Errorf("RTU logical view contradicts runtime shape")
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

func canonicalTime(value time.Time) (time.Time, error) {
	if value.IsZero() || value.Year() < 1 || value.Year() > 9999 {
		return time.Time{}, fmt.Errorf("timestamp is outside RFC3339Nano range")
	}
	encoded := value.Round(0).UTC().Format(time.RFC3339Nano)
	decoded, err := time.Parse(time.RFC3339Nano, encoded)
	if err != nil || !decoded.Equal(value) {
		return time.Time{}, fmt.Errorf("timestamp is not RFC3339Nano round-trippable")
	}
	return decoded, nil
}

func validateSourceTime(sourceTime SourceTimeSpec) error {
	switch sourceTime.State {
	case SourceTimeObservedState:
		if _, err := canonicalTime(sourceTime.Time); err != nil {
			return fmt.Errorf("observed source time is invalid: %w", err)
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

// RetryAttemptID identifies one complete whole-set read attempt. Zero is the
// explicit not-applicable identity for single-wire observations.
type RetryAttemptID uint64

const RetryAttemptNotApplicable RetryAttemptID = 0

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
	AcquisitionOrdinal           uint32
	RetryAttemptID               RetryAttemptID
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
	RetryAttemptID         RetryAttemptID
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

func buildObservation(
	profile ProfileDescriptor,
	spec ObservationSpec,
) (Observation, error) {
	if err := preflightObservationSpec(spec); err != nil {
		return Observation{}, err
	}
	if err := canonicalizeObservationTimes(&spec); err != nil {
		return Observation{}, err
	}
	spec.Dependencies = append([]DependencyResult(nil), spec.Dependencies...)
	if profile.ID() == "" || spec.SchemaVersion != schemaVersionV1 ||
		spec.RuntimeContractVersion != profile.RuntimeContractVersion() ||
		spec.ProfileID != profile.ID() ||
		spec.ProfileVersion != profile.Version() ||
		spec.CodecContractVersion != profile.CodecContractVersion() ||
		spec.DetectorVersion != profile.DetectorVersion() ||
		spec.NormalizationVersion != profile.NormalizationVersion() ||
		spec.CoherenceVersion != profile.CoherenceVersion() ||
		spec.QualificationVersion != profile.QualificationVersion() ||
		(spec.SampleID != "" && !validIdentity(spec.SampleID)) ||
		spec.PollGenerationID == 0 ||
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
	logicalViewIDs := make(map[uint64]struct{}, len(declarations))
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
		if _, exists := logicalViewIDs[record.LogicalViewID]; exists {
			return Observation{}, fmt.Errorf("logical view identity is duplicated")
		}
		logicalViewIDs[record.LogicalViewID] = struct{}{}
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

func preflightObservationSpec(spec ObservationSpec) error {
	if len(spec.Dependencies) > MaxProfileDependencies {
		return fmt.Errorf("observation dependencies exceed the contract maximum")
	}
	stringFields := []struct {
		name  string
		value string
	}{
		{name: "sample ID", value: spec.SampleID},
		{name: "profile ID", value: spec.ProfileID},
		{name: "dependency-set ID", value: spec.DependencySetID},
		{name: "endpoint", value: spec.Endpoint},
	}
	for _, field := range stringFields {
		if err := validateBoundedString(field.name, field.value, false); err != nil {
			return err
		}
	}
	for _, dependency := range spec.Dependencies {
		dependencyFields := []struct {
			name  string
			value string
		}{
			{name: "dependency result ID", value: dependency.DependencyID},
			{name: "dependency result codec ID", value: dependency.CodecID},
			{
				name:  "consistency marker",
				value: dependency.DocumentaryConsistencyMarker,
			},
		}
		for _, field := range dependencyFields {
			if err := validateBoundedString(field.name, field.value, false); err != nil {
				return err
			}
		}
		if len(dependency.View.record.Words) > MaxRawWords {
			return fmt.Errorf("dependency result words exceed runtime maximum")
		}
	}
	return nil
}

func canonicalizeObservationTimes(spec *ObservationSpec) error {
	if spec == nil {
		return fmt.Errorf("observation is nil")
	}
	var err error
	if spec.SourceTime.State == SourceTimeObservedState {
		spec.SourceTime.Time, err = canonicalTime(spec.SourceTime.Time)
		if err != nil {
			return err
		}
	}
	spec.LocalReceiptTime, err = canonicalTime(spec.LocalReceiptTime)
	if err != nil {
		return err
	}
	for index := range spec.Dependencies {
		dependency := &spec.Dependencies[index]
		if dependency.SourceTime.State == SourceTimeObservedState {
			dependency.SourceTime.Time, err = canonicalTime(
				dependency.SourceTime.Time,
			)
			if err != nil {
				return fmt.Errorf("dependency %d source time: %w", index, err)
			}
		}
		if !dependency.LocalReceiptTime.IsZero() {
			dependency.LocalReceiptTime, err = canonicalTime(
				dependency.LocalReceiptTime,
			)
			if err != nil {
				return fmt.Errorf("dependency %d receipt time: %w", index, err)
			}
		}
	}
	return nil
}

func validateObservationCoherence(
	policy CoherencePolicySpec,
	spec ObservationSpec,
	dependencies []ReplayedDependency,
) error {
	if len(dependencies) == 0 {
		return fmt.Errorf("coherent sample is empty")
	}
	if err := validateWireResponseGroups(dependencies); err != nil {
		return err
	}
	switch policy.Mode {
	case CoherenceSingleWireResponse:
		first := dependencies[0].view.record
		if first.Transport == TransportRTU && len(dependencies) != 1 {
			return fmt.Errorf("RTU response cannot produce multiple logical views")
		}
		if spec.RetryAttemptID != RetryAttemptNotApplicable ||
			spec.Dependencies[0].AcquisitionOrdinal != 0 ||
			spec.Dependencies[0].RetryAttemptID !=
				RetryAttemptNotApplicable {
			return fmt.Errorf("single-wire response has acquisition ordinal")
		}
		for index, dependency := range dependencies[1:] {
			record := dependency.view.record
			if record.WireResponseID != first.WireResponseID {
				return fmt.Errorf("single-wire-response coherence failed")
			}
			result := spec.Dependencies[index+1]
			if result.AcquisitionOrdinal != 0 ||
				result.RetryAttemptID != RetryAttemptNotApplicable {
				return fmt.Errorf("single-wire response has retry/acquisition identity")
			}
		}
	case CoherenceBoundedMultiResponse:
		if spec.RetryAttemptID == RetryAttemptNotApplicable {
			return fmt.Errorf("bounded response lacks retry-set identity")
		}
		var minimumSource, maximumSource time.Time
		var minimumReceipt, maximumReceipt time.Time
		first := dependencies[0].view.record
		type acquisitionFact struct {
			ordinal uint32
			index   int
			source  time.Time
			receipt time.Time
		}
		facts := make([]acquisitionFact, len(dependencies))
		ordinals := make(map[uint32]struct{}, len(dependencies))
		for index := range dependencies {
			result := spec.Dependencies[index]
			record := dependencies[index].view.record
			if result.SourceTime.State != SourceTimeObservedState ||
				result.SourceTime.Time.IsZero() ||
				result.LocalReceiptTime.IsZero() ||
				result.AcquisitionOrdinal == 0 ||
				result.RetryAttemptID != spec.RetryAttemptID {
				return fmt.Errorf("bounded response lacks per-dependency time facts")
			}
			if _, exists := ordinals[result.AcquisitionOrdinal]; exists {
				return fmt.Errorf("bounded response acquisition ordinal is duplicated")
			}
			ordinals[result.AcquisitionOrdinal] = struct{}{}
			if record.Endpoint != first.Endpoint ||
				record.UnitID != first.UnitID ||
				record.Transport != first.Transport ||
				(policy.RequireGenerationEquality &&
					record.TransportGeneration != first.TransportGeneration) {
				return fmt.Errorf("bounded response source identity disagrees")
			}
			if policy.DocumentaryConsistencyMarker != "" &&
				result.DocumentaryConsistencyMarker !=
					policy.DocumentaryConsistencyMarker {
				return fmt.Errorf("documentary consistency marker disagrees")
			}
			source := result.SourceTime.Time
			receipt := result.LocalReceiptTime
			facts[index] = acquisitionFact{
				ordinal: result.AcquisitionOrdinal,
				index:   index,
				source:  source,
				receipt: receipt,
			}
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
		sort.Slice(facts, func(first, second int) bool {
			return facts[first].ordinal < facts[second].ordinal
		})
		for index, fact := range facts {
			if fact.ordinal != uint32(index+1) {
				return fmt.Errorf("bounded response acquisition order has gaps")
			}
		}
		switch policy.AcquisitionOrder {
		case AcquisitionOrderDependencyDeclaration:
			for index, fact := range facts {
				if fact.index != index {
					return fmt.Errorf("dependency acquisition order is reversed")
				}
			}
		case AcquisitionOrderSourceTimeAscending:
			for index := 1; index < len(facts); index++ {
				if facts[index].source.Before(facts[index-1].source) {
					return fmt.Errorf("source-time acquisition order is reversed")
				}
			}
		case AcquisitionOrderReceiptTimeAscending:
			for index := 1; index < len(facts); index++ {
				if facts[index].receipt.Before(facts[index-1].receipt) {
					return fmt.Errorf("receipt-time acquisition order is reversed")
				}
			}
		default:
			return fmt.Errorf("bounded response acquisition order is unknown")
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

func validateWireResponseGroups(dependencies []ReplayedDependency) error {
	type wireGroup struct {
		record LogicalViewRecord
		words  []uint16
		set    []bool
	}
	groups := make(map[uint64]*wireGroup, len(dependencies))
	for _, dependency := range dependencies {
		record := dependency.view.record
		group, exists := groups[record.WireResponseID]
		if !exists {
			group = &wireGroup{
				record: record,
				words:  make([]uint16, record.PhysicalWordCount),
				set:    make([]bool, record.PhysicalWordCount),
			}
			groups[record.WireResponseID] = group
		} else if !sameWireResponseIdentity(group.record, record) {
			return fmt.Errorf("wire response identity was reused incompatibly")
		}
		for wordIndex, word := range record.Words {
			position := int(record.SliceOffset) + wordIndex
			if group.set[position] && group.words[position] != word {
				return fmt.Errorf("wire response has contradictory overlapping words")
			}
			group.words[position] = word
			group.set[position] = true
		}
	}
	return nil
}

func sameWireResponseIdentity(
	first LogicalViewRecord,
	second LogicalViewRecord,
) bool {
	return first.PhysicalRequestID == second.PhysicalRequestID &&
		first.Endpoint == second.Endpoint &&
		first.ConnectionID == second.ConnectionID &&
		first.Transport == second.Transport &&
		first.TransportGeneration == second.TransportGeneration &&
		first.UnitID == second.UnitID &&
		first.RequestedFunction == second.RequestedFunction &&
		first.ReceivedFunction == second.ReceivedFunction &&
		first.Table == second.Table &&
		first.PhysicalOffset == second.PhysicalOffset &&
		first.PhysicalWordCount == second.PhysicalWordCount &&
		first.AuthorizationScope == second.AuthorizationScope &&
		first.PollGeneration == second.PollGeneration &&
		first.DeadlineIdentity == second.DeadlineIdentity
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

// LogicalViewRecord returns the complete immutable replay provenance snapshot.
func (dependency ReplayedDependency) LogicalViewRecord() LogicalViewRecord {
	return dependency.view.Record()
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

// SampleLedgerState is the O(1) explicit restart and CAS boundary.
type SampleLedgerState struct {
	SchemaVersion   Version
	IssuerDomain    string
	DependencySetID string
	Revision        uint64
	HighWater       uint64
}

// EmptySampleLedgerState explicitly creates a new issuer/dependency-set domain.
func EmptySampleLedgerState(
	issuerDomain string,
	dependencySetID string,
) (SampleLedgerState, error) {
	state := SampleLedgerState{
		SchemaVersion:   schemaVersionV1,
		IssuerDomain:    issuerDomain,
		DependencySetID: dependencySetID,
	}
	if err := validateSampleLedgerState(state, 0); err != nil {
		return SampleLedgerState{}, err
	}
	return state, nil
}

// SampleLedger issues monotonic sample identities in one immutable domain.
type SampleLedger struct {
	mu              sync.Mutex
	issuerDomain    string
	dependencySetID string
	revision        uint64
	highWater       uint64
}

// NewSampleLedger restores state only at or above a trusted external revision.
func NewSampleLedger(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
) (*SampleLedger, error) {
	if err := validateSampleLedgerState(
		state,
		trustedMinimumRevision,
	); err != nil {
		return nil, err
	}
	return &SampleLedger{
		issuerDomain:    state.IssuerDomain,
		dependencySetID: state.DependencySetID,
		revision:        state.Revision,
		highWater:       state.HighWater,
	}, nil
}

func validateSampleLedgerState(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
) error {
	if state.SchemaVersion != schemaVersionV1 ||
		!validIssuerDomain(state.IssuerDomain) ||
		!validDependencySetID(state.DependencySetID) ||
		state.Revision != state.HighWater ||
		state.Revision < trustedMinimumRevision {
		return fmt.Errorf("sample ledger state is incomplete, stale, or incompatible")
	}
	return nil
}

func validIssuerDomain(value string) bool {
	if len(value) > MaxSampleIssuerDomainBytes || !validIdentity(value) {
		return false
	}
	for _, character := range value {
		if character == ':' {
			return false
		}
	}
	return true
}

func (ledger *SampleLedger) issue() (
	string,
	uint64,
	SampleLedgerState,
	error,
) {
	if ledger == nil {
		return "", 0, SampleLedgerState{}, fmt.Errorf("sample ledger is invalid")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.revision == ^uint64(0) || ledger.highWater == ^uint64(0) {
		return "", 0, SampleLedgerState{}, fmt.Errorf("sample ledger is exhausted")
	}
	expectedRevision := ledger.revision
	ledger.revision++
	ledger.highWater++
	state := ledger.stateLocked()
	return fmt.Sprintf(
		"%s:%d",
		ledger.issuerDomain,
		ledger.highWater,
	), expectedRevision, state, nil
}

func (ledger *SampleLedger) admitSerialized(
	sampleID string,
) (uint64, SampleLedgerState, error) {
	if ledger == nil {
		return 0, SampleLedgerState{}, fmt.Errorf("sample ledger is invalid")
	}
	separator := strings.LastIndexByte(sampleID, ':')
	if separator <= 0 || sampleID[:separator] != ledger.issuerDomain {
		return 0, SampleLedgerState{}, fmt.Errorf("sample ID issuer domain disagrees")
	}
	sequence, err := strconv.ParseUint(sampleID[separator+1:], 10, 64)
	if err != nil {
		return 0, SampleLedgerState{}, fmt.Errorf("sample ID sequence is invalid")
	}
	if sampleID != fmt.Sprintf("%s:%d", ledger.issuerDomain, sequence) {
		return 0, SampleLedgerState{}, fmt.Errorf("sample ID is not canonical")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.highWater == ^uint64(0) ||
		sequence != ledger.highWater+1 {
		return 0, SampleLedgerState{}, fmt.Errorf("sample ID is reused or out of sequence")
	}
	expectedRevision := ledger.revision
	ledger.highWater = sequence
	ledger.revision++
	return expectedRevision, ledger.stateLocked(), nil
}

func (ledger *SampleLedger) stateLocked() SampleLedgerState {
	return SampleLedgerState{
		SchemaVersion:   schemaVersionV1,
		IssuerDomain:    ledger.issuerDomain,
		DependencySetID: ledger.dependencySetID,
		Revision:        ledger.revision,
		HighWater:       ledger.highWater,
	}
}

// ExportState returns deterministic explicit restart state.
func (ledger *SampleLedger) ExportState() SampleLedgerState {
	if ledger == nil {
		return SampleLedgerState{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.stateLocked()
}

// SampleAdmission binds one observation to the exact external CAS transition
// that must be durably persisted before consumers use it.
type SampleAdmission struct {
	observation      Observation
	expectedRevision uint64
	persistedState   SampleLedgerState
}

// Observation returns the immutable admitted sample.
func (admission SampleAdmission) Observation() Observation {
	return admission.observation
}

// ExpectedRevision returns the external compare-and-swap anchor.
func (admission SampleAdmission) ExpectedRevision() uint64 {
	return admission.expectedRevision
}

// PersistedState returns the exact state to CAS-persist before external use.
func (admission SampleAdmission) PersistedState() SampleLedgerState {
	return admission.persistedState
}

// ObservationFactory makes ledger admission structurally mandatory.
type ObservationFactory struct {
	profile ProfileDescriptor
	ledger  *SampleLedger
}

// NewObservationFactory binds one exact profile to explicit ledger state.
func NewObservationFactory(
	profile ProfileDescriptor,
	ledger *SampleLedger,
) (*ObservationFactory, error) {
	if ledger == nil {
		return nil, fmt.Errorf("observation factory requires a sample ledger")
	}
	copy, err := NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("observation factory profile: %w", err)
	}
	if copy.spec.Kind != ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	state := ledger.ExportState()
	if state.DependencySetID != copy.Dependencies().ID() {
		return nil, fmt.Errorf("observation factory dependency-set domain disagrees")
	}
	return &ObservationFactory{profile: copy, ledger: ledger}, nil
}

// NewObservation validates and atomically admits one successful observation.
func (factory *ObservationFactory) NewObservation(
	spec ObservationSpec,
) (SampleAdmission, error) {
	if factory == nil || factory.ledger == nil {
		return SampleAdmission{}, fmt.Errorf("observation factory is invalid")
	}
	if spec.SampleID != "" {
		return SampleAdmission{}, fmt.Errorf("sample ID must be factory-issued")
	}
	observation, err := buildObservation(factory.profile, spec)
	if err != nil {
		return SampleAdmission{}, err
	}
	sampleID, expectedRevision, state, err := factory.ledger.issue()
	if err != nil {
		return SampleAdmission{}, err
	}
	observation.spec.SampleID = sampleID
	return SampleAdmission{
		observation:      observation,
		expectedRevision: expectedRevision,
		persistedState:   state,
	}, nil
}

func (factory *ObservationFactory) admitSerializedObservation(
	observation Observation,
) (SampleAdmission, error) {
	if factory == nil || factory.ledger == nil ||
		observation.SampleID() == "" {
		return SampleAdmission{}, fmt.Errorf("observation factory is invalid")
	}
	expectedRevision, state, err := factory.ledger.admitSerialized(
		observation.SampleID(),
	)
	if err != nil {
		return SampleAdmission{}, err
	}
	return SampleAdmission{
		observation:      observation,
		expectedRevision: expectedRevision,
		persistedState:   state,
	}, nil
}

// OwnershipBoundary documents that this package records source facts only.
func OwnershipBoundary() string {
	return "records immutable source facts; downstream composition owns consumer semantics"
}
