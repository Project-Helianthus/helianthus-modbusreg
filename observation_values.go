package modbusreg

import (
	"fmt"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// FunctionCode is the runtime Modbus operation identity.
type FunctionCode = modbus.FunctionCode

// The read functions a profile dependency may use: FC03 and FC04.
const (
	FunctionReadHoldingRegisters = modbus.FunctionReadHoldingRegisters
	FunctionReadInputRegisters   = modbus.FunctionReadInputRegisters
)

// TransportFamily is the runtime transport-generation identity.
type TransportFamily = modbus.TransportFamily

// The runtime transport families.
const (
	TransportTCP = modbus.TransportTCP
	TransportRTU = modbus.TransportRTU
)

// DependencyReadStatus classifies terminal dependency inputs.
type DependencyReadStatus string

// The dependency read statuses. NewObservation admits only successful
// dependencies with a validated logical view.
const (
	DependencyReadSuccessful DependencyReadStatus = "successful"
	DependencyReadTorn       DependencyReadStatus = "torn"
	DependencyReadMalformed  DependencyReadStatus = "malformed"
	DependencyReadException  DependencyReadStatus = "exception"
)

// SourceValidity records the source's own validity fact.
type SourceValidity string

// The source validity facts; NewObservation rejects any other value.
const (
	SourceValid          SourceValidity = "valid"
	SourceInvalid        SourceValidity = "invalid"
	SourceNotImplemented SourceValidity = "not_implemented"
	SourceReserved       SourceValidity = "reserved"
)

// SourceTimeState distinguishes a real source time from its documented absence.
type SourceTimeState string

// The source-time states. An unavailable source time carries no time value.
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

func canonicalObservedTime(value time.Time) (time.Time, error) {
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

// HasLocalReceiptTime reports whether value is non-zero or present marks a zero receipt time.
func HasLocalReceiptTime(value time.Time, present bool) bool {
	return present || !value.IsZero()
}

// CanonicalRequiredTime returns a recorded local receipt time in canonical UTC
// form; it fails without one or outside the RFC 3339 nanosecond range.
func CanonicalRequiredTime(value time.Time, present bool) (time.Time, error) {
	if !HasLocalReceiptTime(value, present) {
		return time.Time{}, fmt.Errorf("timestamp is outside RFC3339Nano range")
	}
	return canonicalObservedTime(value)
}

// ValidateSourceTime accepts an observed time in the RFC 3339 nanosecond range
// or an unavailable source time without a time, and no other state.
func ValidateSourceTime(sourceTime SourceTimeSpec) error {
	switch sourceTime.State {
	case SourceTimeObservedState:
		if _, err := canonicalObservedTime(sourceTime.Time); err != nil {
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

// ReplayedDependency is one exact raw dependency and its retained provenance.
type ReplayedDependency struct {
	dependencyID               string
	dependencyVersion          Version
	codecID                    string
	codecVersion               Version
	normalization              AddressNormalization
	view                       LogicalViewSnapshot
	runtimeNormalizationBytes  []byte
	runtimeNormalizationFields modbus.RuntimeNormalizationFields
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

// RuntimeNormalizationBytes returns the exact producer-admitted JSON bytes.
// Offline fixture dependencies return nil.
func (dependency ReplayedDependency) RuntimeNormalizationBytes() []byte {
	return append([]byte(nil), dependency.runtimeNormalizationBytes...)
}

// RuntimeNormalizationFields returns the producer-parsed required fields used
// for dependency matching. Unknown extensions remain only in the exact bytes.
func (dependency ReplayedDependency) RuntimeNormalizationFields() modbus.RuntimeNormalizationFields {
	return dependency.runtimeNormalizationFields
}
