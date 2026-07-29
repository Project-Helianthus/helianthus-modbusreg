package modbusreg

import (
	"fmt"
	"sort"
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
	if value.IsZero() {
		return time.Time{}, fmt.Errorf("timestamp is outside RFC3339Nano range")
	}
	canonical := value.Round(0).UTC()
	if canonical.Year() < 1 || canonical.Year() > 9999 {
		return time.Time{}, fmt.Errorf("timestamp is outside RFC3339Nano range")
	}
	encoded := canonical.Format(time.RFC3339Nano)
	decoded, err := time.Parse(time.RFC3339Nano, encoded)
	if err != nil || !decoded.Equal(canonical) {
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
	RetryOrdinal                 uint32
	claim                        *dependencyResultClaim
	owner                        *observationAttemptState
}

type dependencyResultClaim struct {
	mu       sync.Mutex
	consumed bool
}

// NewDependencyResult creates one capture handle. Copies of the returned value
// share a single-use claim and cannot be rebound to different attempts.
func NewDependencyResult(spec DependencyResult) (DependencyResult, error) {
	if spec.claim != nil || spec.owner != nil {
		return DependencyResult{}, fmt.Errorf("dependency result is already owned")
	}
	if err := preflightDependencyResult(spec); err != nil {
		return DependencyResult{}, err
	}
	spec = cloneDependencyResult(spec)
	spec.claim = &dependencyResultClaim{}
	return spec, nil
}

// AttemptIdentity is the deterministic capture identity for one poll retry.
// RetryOrdinal is zero only when retry identity is not applicable.
type AttemptIdentity struct {
	PollGenerationID uint64
	RetryOrdinal     uint32
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
	RetryOrdinal           uint32
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
	spec = cloneObservationSpec(spec)
	if err := canonicalizeObservationTimes(&spec); err != nil {
		return Observation{}, err
	}
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

func preflightObservationSpec(spec ObservationSpec) error {
	if len(spec.Dependencies) > MaxProfileDependencies {
		return fmt.Errorf("observation dependencies exceed the contract maximum")
	}
	if err := preflightAggregate(spec); err != nil {
		return err
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
		if err := preflightDependencyResult(dependency); err != nil {
			return err
		}
	}
	if spec.DependencySetID != "" && !validDependencySetID(spec.DependencySetID) {
		return fmt.Errorf("dependency-set ID is malformed")
	}
	return nil
}

func preflightDependencyResult(result DependencyResult) error {
	return preflightAggregate(result)
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
		if spec.RetryOrdinal != 0 {
			return fmt.Errorf("single-wire retry identity is not applicable")
		}
		for index, dependency := range dependencies {
			result := spec.Dependencies[index]
			if result.RetryOrdinal != 0 ||
				result.AcquisitionOrdinal != 0 ||
				result.SourceTime.State != SourceTimeUnavailableState ||
				!result.SourceTime.Time.IsZero() ||
				!result.LocalReceiptTime.IsZero() {
				return fmt.Errorf("single-wire dependency time/retry identity is not applicable")
			}
			if index == 0 {
				continue
			}
			record := dependency.view.record
			if record.WireResponseID != first.WireResponseID {
				return fmt.Errorf("single-wire-response coherence failed")
			}
		}
	case CoherenceBoundedMultiResponse:
		if spec.RetryOrdinal == 0 {
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
				result.RetryOrdinal != spec.RetryOrdinal {
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
		if exceedsTimeSkew(minimumSource, maximumSource, policy.MaximumSourceSkew) ||
			exceedsTimeSkew(minimumReceipt, maximumReceipt, policy.MaximumReceiptSkew) {
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

func exceedsTimeSkew(minimum, maximum time.Time, limit time.Duration) bool {
	if limit < 0 || maximum.Before(minimum) {
		return true
	}
	seconds := maximum.Unix() - minimum.Unix()
	nanoseconds := int64(maximum.Nanosecond()) - int64(minimum.Nanosecond())
	if nanoseconds < 0 {
		seconds--
		nanoseconds += int64(time.Second)
	}
	limitSeconds := int64(limit / time.Second)
	limitNanoseconds := int64(limit % time.Second)
	return seconds > limitSeconds ||
		(seconds == limitSeconds && nanoseconds > limitNanoseconds)
}

func validateWireResponseGroups(dependencies []ReplayedDependency) error {
	type physicalRequestKey struct {
		endpoint            string
		connectionID        uint64
		transport           TransportFamily
		transportGeneration uint64
		physicalRequestID   uint64
	}
	type physicalIdentity struct {
		physicalRequestID   uint64
		endpoint            string
		connectionID        uint64
		transport           TransportFamily
		transportGeneration uint64
		unitID              byte
		requestedFunction   FunctionCode
		receivedFunction    FunctionCode
		table               LogicalTable
		physicalOffset      uint16
		physicalWordCount   uint16
		authorizationScope  string
		pollGeneration      uint64
		deadlineIdentity    uint64
	}
	type physicalGroup struct {
		wireResponseID uint64
		words          []uint16
		set            []bool
		logicalViewIDs map[uint64]struct{}
	}
	identityOf := func(record LogicalViewRecord) physicalIdentity {
		return physicalIdentity{
			physicalRequestID:   record.PhysicalRequestID,
			endpoint:            record.Endpoint,
			connectionID:        record.ConnectionID,
			transport:           record.Transport,
			transportGeneration: record.TransportGeneration,
			unitID:              record.UnitID,
			requestedFunction:   record.RequestedFunction,
			receivedFunction:    record.ReceivedFunction,
			table:               record.Table,
			physicalOffset:      record.PhysicalOffset,
			physicalWordCount:   record.PhysicalWordCount,
			authorizationScope:  record.AuthorizationScope,
			pollGeneration:      record.PollGeneration,
			deadlineIdentity:    record.DeadlineIdentity,
		}
	}
	physicalGroups := make(map[physicalIdentity]*physicalGroup, len(dependencies))
	physicalRequests := make(
		map[physicalRequestKey]physicalIdentity,
		len(dependencies),
	)
	wireToPhysical := make(map[uint64]physicalIdentity, len(dependencies))
	rtuWireResponses := make(map[uint64]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		record := dependency.view.record
		identity := identityOf(record)
		if record.Transport == TransportRTU {
			if _, exists := rtuWireResponses[record.WireResponseID]; exists {
				return fmt.Errorf("RTU response maps to multiple logical views")
			}
			rtuWireResponses[record.WireResponseID] = struct{}{}
		}
		requestKey := physicalRequestKey{
			endpoint:            record.Endpoint,
			connectionID:        record.ConnectionID,
			transport:           record.Transport,
			transportGeneration: record.TransportGeneration,
			physicalRequestID:   record.PhysicalRequestID,
		}
		if mapped, exists := physicalRequests[requestKey]; exists &&
			mapped != identity {
			return fmt.Errorf("physical request identity maps to multiple canonical requests")
		}
		physicalRequests[requestKey] = identity
		if mapped, exists := wireToPhysical[record.WireResponseID]; exists &&
			mapped != identity {
			return fmt.Errorf("wire response identity maps to multiple physical identities")
		}
		wireToPhysical[record.WireResponseID] = identity
		group, exists := physicalGroups[identity]
		if !exists {
			group = &physicalGroup{
				wireResponseID: record.WireResponseID,
				words:          make([]uint16, record.PhysicalWordCount),
				set:            make([]bool, record.PhysicalWordCount),
				logicalViewIDs: make(map[uint64]struct{}),
			}
			physicalGroups[identity] = group
		} else if group.wireResponseID != record.WireResponseID {
			return fmt.Errorf("physical identity maps to multiple wire responses")
		}
		if _, exists := group.logicalViewIDs[record.LogicalViewID]; exists {
			return fmt.Errorf("logical view identity is duplicated in one physical group")
		}
		group.logicalViewIDs[record.LogicalViewID] = struct{}{}
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

func cloneObservationSpec(spec ObservationSpec) ObservationSpec {
	spec.Dependencies = append([]DependencyResult(nil), spec.Dependencies...)
	for index := range spec.Dependencies {
		spec.Dependencies[index] = cloneDependencyResult(spec.Dependencies[index])
	}
	return spec
}

func cloneDependencyResult(result DependencyResult) DependencyResult {
	if result.View.valid {
		record := result.View.Record()
		result.View, _ = NewLogicalViewSnapshot(record)
	}
	return result
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
	SchemaVersion        Version
	IssuerDomain         string
	ProfileID            string
	ProfileVersion       Version
	DependencySetID      string
	Revision             uint64
	HighWater            uint64
	LastCommittedAttempt AttemptIdentity
}

// EmptySampleLedgerState creates a new issuer/profile/dependency-set domain.
func EmptySampleLedgerState(
	issuerDomain string,
	profile ProfileDescriptor,
) (SampleLedgerState, error) {
	if profile.ID() == "" {
		return SampleLedgerState{}, fmt.Errorf("sample ledger profile is invalid")
	}
	state := SampleLedgerState{
		SchemaVersion:   schemaVersionV1,
		IssuerDomain:    issuerDomain,
		ProfileID:       profile.ID(),
		ProfileVersion:  profile.Version(),
		DependencySetID: profile.Dependencies().ID(),
	}
	if err := validateSampleLedgerState(state, 0); err != nil {
		return SampleLedgerState{}, err
	}
	return state, nil
}

// SampleLedger issues monotonic sample identities in one immutable domain.
type SampleLedger struct {
	mu                   sync.Mutex
	commitSerial         chan struct{}
	issuerDomain         string
	profileID            string
	profileVersion       Version
	dependencySetID      string
	revision             uint64
	highWater            uint64
	lastCommittedAttempt AttemptIdentity
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
	ledger := &SampleLedger{
		commitSerial:         make(chan struct{}, 1),
		issuerDomain:         state.IssuerDomain,
		profileID:            state.ProfileID,
		profileVersion:       state.ProfileVersion,
		dependencySetID:      state.DependencySetID,
		revision:             state.Revision,
		highWater:            state.HighWater,
		lastCommittedAttempt: state.LastCommittedAttempt,
	}
	ledger.commitSerial <- struct{}{}
	return ledger, nil
}

func validateSampleLedgerState(
	state SampleLedgerState,
	trustedMinimumRevision uint64,
) error {
	initialAttemptState := state.LastCommittedAttempt == (AttemptIdentity{})
	if state.SchemaVersion != schemaVersionV1 ||
		!validIssuerDomain(state.IssuerDomain) ||
		!validIdentity(state.ProfileID) ||
		!state.ProfileVersion.valid() ||
		!validDependencySetID(state.DependencySetID) ||
		state.Revision != state.HighWater ||
		state.Revision < trustedMinimumRevision ||
		(state.Revision == 0 && !initialAttemptState) ||
		(state.Revision != 0 &&
			state.LastCommittedAttempt.PollGenerationID == 0) ||
		(state.LastCommittedAttempt.PollGenerationID == 0 &&
			state.LastCommittedAttempt.RetryOrdinal != 0) {
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

// SampleStateCAS is the consumer-owned atomic persistence boundary. A true
// result means the exact expected state was durably replaced by next.
type SampleStateCAS interface {
	CompareAndSwap(expected, next SampleLedgerState) (bool, error)
}

func (ledger *SampleLedger) commit(
	store SampleStateCAS,
	attempt AttemptIdentity,
) (string, error) {
	if ledger == nil {
		return "", fmt.Errorf("sample ledger is invalid")
	}
	if store == nil {
		return "", fmt.Errorf("sample state CAS is invalid")
	}
	<-ledger.commitSerial
	defer func() {
		ledger.commitSerial <- struct{}{}
	}()
	ledger.mu.Lock()
	if ledger.revision == ^uint64(0) || ledger.highWater == ^uint64(0) {
		ledger.mu.Unlock()
		return "", fmt.Errorf("sample ledger is exhausted")
	}
	if !attemptIdentityAdvances(ledger.lastCommittedAttempt, attempt) {
		ledger.mu.Unlock()
		return "", fmt.Errorf("observation attempt does not advance the ledger")
	}
	expected := ledger.stateLocked()
	next := expected
	next.Revision++
	next.HighWater++
	next.LastCommittedAttempt = attempt
	ledger.mu.Unlock()
	sampleID := fmt.Sprintf(
		"%s:%d",
		ledger.issuerDomain,
		next.HighWater,
	)
	swapped, err := store.CompareAndSwap(expected, next)
	if err != nil {
		return "", fmt.Errorf("persist sample ledger state: %w", err)
	}
	if !swapped {
		return "", fmt.Errorf("sample ledger compare-and-swap conflict")
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.stateLocked() != expected {
		return "", fmt.Errorf("sample ledger changed during compare-and-swap")
	}
	ledger.revision = next.Revision
	ledger.highWater = next.HighWater
	ledger.lastCommittedAttempt = next.LastCommittedAttempt
	return sampleID, nil
}

func attemptIdentityAdvances(previous, next AttemptIdentity) bool {
	if next.PollGenerationID == 0 {
		return false
	}
	if previous.PollGenerationID == 0 {
		return true
	}
	if next.PollGenerationID != previous.PollGenerationID {
		return next.PollGenerationID > previous.PollGenerationID
	}
	return next.RetryOrdinal > previous.RetryOrdinal
}

func (ledger *SampleLedger) stateLocked() SampleLedgerState {
	return SampleLedgerState{
		SchemaVersion:        schemaVersionV1,
		IssuerDomain:         ledger.issuerDomain,
		ProfileID:            ledger.profileID,
		ProfileVersion:       ledger.profileVersion,
		DependencySetID:      ledger.dependencySetID,
		Revision:             ledger.revision,
		HighWater:            ledger.highWater,
		LastCommittedAttempt: ledger.lastCommittedAttempt,
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

// ObservationFactory binds attempts to one profile and persistence domain.
type ObservationFactory struct {
	profile ProfileDescriptor
	ledger  *SampleLedger
	store   SampleStateCAS
}

// NewObservationFactory requires the consumer's atomic persistence boundary.
func NewObservationFactory(
	profile ProfileDescriptor,
	ledger *SampleLedger,
	store SampleStateCAS,
) (*ObservationFactory, error) {
	if ledger == nil || store == nil {
		return nil, fmt.Errorf("observation factory requires a ledger and state CAS")
	}
	copy, err := NewProfileDescriptor(profile.Spec())
	if err != nil {
		return nil, fmt.Errorf("observation factory profile: %w", err)
	}
	if copy.spec.Kind != ProfileStandardFamily {
		return nil, fmt.Errorf("vendor overlay requires M3 resolution")
	}
	state := ledger.ExportState()
	if state.ProfileID != copy.ID() ||
		state.ProfileVersion != copy.Version() ||
		state.DependencySetID != copy.Dependencies().ID() {
		return nil, fmt.Errorf("observation factory persistence domain disagrees")
	}
	return &ObservationFactory{profile: copy, ledger: ledger, store: store}, nil
}

type observationAttemptPhase uint8

const (
	attemptOpen observationAttemptPhase = iota + 1
	attemptPublishing
	attemptPublished
	attemptFailed
)

type observationCaptureMode uint8

const (
	captureUnset observationCaptureMode = iota
	captureDirect
	captureSerialized
)

type observationAttemptState struct {
	mu       sync.Mutex
	factory  *ObservationFactory
	identity AttemptIdentity
	phase    observationAttemptPhase
	capture  observationCaptureMode
}

// ObservationAttempt owns the deterministic retry binding for one captured
// set. Value copies retain the same private lifecycle state.
type ObservationAttempt struct {
	state *observationAttemptState
}

// BeginObservationAttempt starts one isolated capture/admission transaction.
func (factory *ObservationFactory) BeginObservationAttempt(
	identity AttemptIdentity,
) (*ObservationAttempt, error) {
	if factory == nil || factory.ledger == nil || factory.store == nil {
		return nil, fmt.Errorf("observation factory is invalid")
	}
	if identity.PollGenerationID == 0 {
		return nil, fmt.Errorf("attempt poll generation is absent")
	}
	switch factory.profile.spec.Coherence.Mode {
	case CoherenceSingleWireResponse:
		if identity.RetryOrdinal != 0 {
			return nil, fmt.Errorf("single-wire retry identity is not applicable")
		}
	case CoherenceBoundedMultiResponse:
		if identity.RetryOrdinal == 0 {
			return nil, fmt.Errorf("bounded retry identity is absent")
		}
	default:
		return nil, fmt.Errorf("observation coherence mode is invalid")
	}
	return &ObservationAttempt{
		state: &observationAttemptState{
			factory:  factory,
			identity: identity,
			phase:    attemptOpen,
		},
	}, nil
}

// BindDependency captures a result into this attempt without mutating input.
func (attempt *ObservationAttempt) BindDependency(
	result DependencyResult,
) (DependencyResult, error) {
	if attempt == nil || attempt.state == nil {
		return DependencyResult{}, fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.factory == nil || state.phase != attemptOpen {
		return DependencyResult{}, fmt.Errorf("observation attempt is closed")
	}
	if state.capture == captureSerialized {
		return DependencyResult{}, fmt.Errorf("serialized capture cannot accept direct dependencies")
	}
	if result.claim == nil || result.owner != nil {
		return DependencyResult{}, fmt.Errorf("dependency result is not an unbound capture")
	}
	if result.View.valid &&
		result.View.record.PollGeneration != state.identity.PollGenerationID {
		return DependencyResult{}, fmt.Errorf("dependency poll generation disagrees")
	}
	if result.RetryOrdinal != state.identity.RetryOrdinal {
		return DependencyResult{}, fmt.Errorf("dependency retry ordinal disagrees")
	}
	if err := preflightDependencyResult(result); err != nil {
		return DependencyResult{}, err
	}
	result.claim.mu.Lock()
	defer result.claim.mu.Unlock()
	if result.claim.consumed {
		return DependencyResult{}, fmt.Errorf("dependency belongs to another attempt")
	}
	result.claim.consumed = true
	result = cloneDependencyResult(result)
	result.owner = state
	state.capture = captureDirect
	return result, nil
}

func (attempt *ObservationAttempt) openState() (*observationAttemptState, error) {
	if attempt == nil || attempt.state == nil ||
		attempt.state.factory == nil {
		return nil, fmt.Errorf("observation attempt is invalid")
	}
	attempt.state.mu.Lock()
	defer attempt.state.mu.Unlock()
	if attempt.state.phase != attemptOpen {
		return nil, fmt.Errorf("observation attempt is closed")
	}
	return attempt.state, nil
}

func (attempt *ObservationAttempt) prepareSpec(
	spec ObservationSpec,
) (ObservationSpec, error) {
	if attempt == nil || attempt.state == nil {
		return ObservationSpec{}, fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.factory == nil || state.phase != attemptOpen {
		return ObservationSpec{}, fmt.Errorf("observation attempt is closed")
	}
	spec = cloneObservationSpec(spec)
	if spec.PollGenerationID != state.identity.PollGenerationID ||
		spec.RetryOrdinal != state.identity.RetryOrdinal {
		return ObservationSpec{}, fmt.Errorf("observation attempt identity disagrees")
	}
	mode := state.factory.profile.spec.Coherence.Mode
	switch mode {
	case CoherenceSingleWireResponse:
		if spec.RetryOrdinal != 0 {
			return ObservationSpec{}, fmt.Errorf("single-wire retry identity is not applicable")
		}
	case CoherenceBoundedMultiResponse:
		if spec.RetryOrdinal == 0 {
			return ObservationSpec{}, fmt.Errorf("bounded retry identity is absent")
		}
	default:
		return ObservationSpec{}, fmt.Errorf("observation coherence mode is invalid")
	}
	for index := range spec.Dependencies {
		if spec.Dependencies[index].owner != state ||
			spec.Dependencies[index].RetryOrdinal != spec.RetryOrdinal {
			return ObservationSpec{}, fmt.Errorf(
				"dependency %d is not bound to this attempt",
				index,
			)
		}
	}
	return cloneObservationSpec(spec), nil
}

// Publish returns an Observation only after the external CAS succeeds.
func (attempt *ObservationAttempt) Publish(
	spec ObservationSpec,
) (Observation, error) {
	if attempt == nil || attempt.state == nil {
		return Observation{}, fmt.Errorf("observation attempt is invalid")
	}
	state := attempt.state
	state.mu.Lock()
	if state.phase != attemptOpen {
		state.mu.Unlock()
		return Observation{}, fmt.Errorf("observation attempt is closed")
	}
	state.phase = attemptPublishing
	state.mu.Unlock()
	succeeded := false
	defer func() {
		state.mu.Lock()
		if succeeded {
			state.phase = attemptPublished
		} else {
			state.phase = attemptFailed
		}
		state.mu.Unlock()
	}()
	// prepareSpec normally requires an open attempt; publication already owns
	// the lifecycle, so validate against this exact state directly.
	prepared := cloneObservationSpec(spec)
	if prepared.PollGenerationID != state.identity.PollGenerationID ||
		prepared.RetryOrdinal != state.identity.RetryOrdinal {
		return Observation{}, fmt.Errorf("observation attempt identity disagrees")
	}
	for index := range prepared.Dependencies {
		if prepared.Dependencies[index].owner != state ||
			prepared.Dependencies[index].RetryOrdinal != prepared.RetryOrdinal {
			return Observation{}, fmt.Errorf(
				"dependency %d is not bound to this attempt",
				index,
			)
		}
	}
	if prepared.SampleID != "" {
		return Observation{}, fmt.Errorf("sample ID must be factory-issued")
	}
	observation, err := buildObservation(state.factory.profile, prepared)
	if err != nil {
		return Observation{}, err
	}
	sampleID, err := state.factory.ledger.commit(
		state.factory.store,
		state.identity,
	)
	if err != nil {
		return Observation{}, err
	}
	observation.spec.SampleID = sampleID
	succeeded = true
	return observation, nil
}

// OwnershipBoundary documents that this package records source facts only.
func OwnershipBoundary() string {
	return "records immutable source facts; downstream composition owns consumer semantics"
}
