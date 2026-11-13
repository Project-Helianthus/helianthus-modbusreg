package growatt

import (
	"fmt"
)

// ProtocolIIFC04Slice retains the sole bounded FC04 acquisition. Words
// outside the typed fields remain native evidence only.
type ProtocolIIFC04Slice struct {
	Offset uint16
	Words  []uint16
}

// ProtocolIIFC04Applicability is an opaque typed-FC04 admission value.
// The pinned Protocol II v1.24 source does not establish an admissible exact
// device-type/model-build/protocol tuple, so this package intentionally exposes
// no public successful constructor. Its zero value and every externally
// constructible form fail closed. A raw FC03 identity observation cannot create
// an admission implicitly.
type ProtocolIIFC04Applicability struct {
	profile          ProtocolIIIdentityProfile
	syntheticFixture bool
}

func (a ProtocolIIFC04Applicability) matches(identity ProtocolIIIdentityObservation) bool {
	return a.syntheticFixture && a.profile == identity.Profile() &&
		a.profile.DeviceType == identity.DeviceType() && a.profile.ModelBuild == identity.ModelBuild() &&
		a.profile.ProtocolVersion == identity.ProtocolVersion()
}

// ProtocolIIInverterState is the decoded inverter state of FC04 word 0.
// Only the values 0, 1 and 3 decode; any other value rejects the whole
// telemetry record. Meaning is owned by
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/protocol-ii-readonly-v1.md.
type ProtocolIIInverterState string

// Inverter states for word values 0 (Waiting), 1 (Normal) and 3 (Fault).
const (
	ProtocolIIStateWaiting ProtocolIIInverterState = "waiting"
	ProtocolIIStateNormal  ProtocolIIInverterState = "normal"
	ProtocolIIStateFault   ProtocolIIInverterState = "fault"
)

// ProtocolIIFC04Telemetry is an immutable, read-only subset from the
// exact selected identity. It deliberately omits fields whose signedness or
// interpretation is not established by the pinned source.
type ProtocolIIFC04Telemetry struct {
	identity                                                          ProtocolIIIdentityObservation
	raw                                                               ProtocolIIFC04Slice
	InverterState                                                     ProtocolIIInverterState
	PVInputPowerWatts, OutputPowerWatts, GridFrequencyHz              float64
	Phase1VoltageVolts, Phase1CurrentAmps                             float64
	Phase2VoltageVolts, Phase2CurrentAmps                             float64
	Phase3VoltageVolts, Phase3CurrentAmps                             float64
	TodayGeneratedEnergyKWh, TotalGeneratedEnergyKWh, WorkTimeSeconds float64
}

// Identity returns the FC03 identity observation the telemetry was admitted for.
func (t ProtocolIIFC04Telemetry) Identity() ProtocolIIIdentityObservation {
	return t.identity
}

// RawSlice returns a copy of the complete raw FC04 slice, including untyped words.
func (t ProtocolIIFC04Telemetry) RawSlice() ProtocolIIFC04Slice {
	return ProtocolIIFC04Slice{Offset: t.raw.Offset, Words: append([]uint16(nil), t.raw.Words...)}
}

// OutboundAllowed is permanently false: telemetry never authorizes a further request or control.
func (ProtocolIIFC04Telemetry) OutboundAllowed() bool { return false }

// DecodeGrowattProtocolIIFC04Telemetry accepts exactly offsets 0 through 58
// only when an opaque applicability value admits the FC03 identity. No public
// admitted value currently exists; a caller-selected raw identity alone is
// insufficient.
func DecodeGrowattProtocolIIFC04Telemetry(identity ProtocolIIIdentityObservation, applicability ProtocolIIFC04Applicability, raw ProtocolIIFC04Slice) (ProtocolIIFC04Telemetry, error) {
	if !validGrowattProtocolIIIdentityProfile(identity.Profile()) || !applicability.matches(identity) || identity.UnitID() == 0 || raw.Offset != 0 || len(raw.Words) != 59 {
		return ProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 telemetry is invalid")
	}
	state, ok := growattProtocolIIState(raw.Words[0])
	if !ok {
		return ProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 inverter state is invalid")
	}
	u32 := func(high, low uint16) uint32 { return uint32(high)<<16 | uint32(low) }
	return ProtocolIIFC04Telemetry{
		identity: identity, raw: ProtocolIIFC04Slice{Offset: raw.Offset, Words: append([]uint16(nil), raw.Words...)}, InverterState: state,
		PVInputPowerWatts: float64(u32(raw.Words[1], raw.Words[2])) / 10, OutputPowerWatts: float64(u32(raw.Words[35], raw.Words[36])) / 10, GridFrequencyHz: float64(raw.Words[37]) / 100,
		Phase1VoltageVolts: float64(raw.Words[38]) / 10, Phase1CurrentAmps: float64(raw.Words[39]) / 10, Phase2VoltageVolts: float64(raw.Words[42]) / 10, Phase2CurrentAmps: float64(raw.Words[43]) / 10, Phase3VoltageVolts: float64(raw.Words[46]) / 10, Phase3CurrentAmps: float64(raw.Words[47]) / 10,
		TodayGeneratedEnergyKWh: float64(u32(raw.Words[53], raw.Words[54])) / 10, TotalGeneratedEnergyKWh: float64(u32(raw.Words[55], raw.Words[56])) / 10, WorkTimeSeconds: float64(u32(raw.Words[57], raw.Words[58])) / 2,
	}, nil
}
func growattProtocolIIState(word uint16) (ProtocolIIInverterState, bool) {
	switch word {
	case 0:
		return ProtocolIIStateWaiting, true
	case 1:
		return ProtocolIIStateNormal, true
	case 3:
		return ProtocolIIStateFault, true
	default:
		return "", false
	}
}
