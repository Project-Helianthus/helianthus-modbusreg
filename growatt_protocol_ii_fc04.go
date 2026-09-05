package modbusreg

import (
	"fmt"
	"strings"
)

// GrowattProtocolIIFC04Slice retains the sole bounded FC04 acquisition. Words
// outside the typed fields remain native evidence only.
type GrowattProtocolIIFC04Slice struct {
	Offset uint16
	Words  []uint16
}

// GrowattProtocolIIFC04Applicability is an immutable, caller-supplied exact
// mapping admission for typed FC04 monitoring. Protocol II v1.24 names the
// MAX/MID/MAC family but does not publish a device/model-build/protocol tuple
// mapping, so this package deliberately contains no built-in tuple allowlist.
// The caller must obtain a mapping from its own accepted source-backed evidence
// before constructing this value; a raw FC03 identity observation cannot create
// one implicitly.
type GrowattProtocolIIFC04Applicability struct {
	profile           GrowattProtocolIIIdentityProfile
	evidenceReference string
}

// NewGrowattProtocolIIFC04Applicability seals one already-accepted exact
// mapping for the FC04 schema. evidenceReference identifies the caller-owned
// source-backed mapping record; it is not inferred from the Protocol II manual
// revision or from an observed identity tuple.
func NewGrowattProtocolIIFC04Applicability(profile GrowattProtocolIIIdentityProfile, evidenceReference string) (GrowattProtocolIIFC04Applicability, error) {
	if !validGrowattProtocolIIIdentityProfile(profile) || profile.DeviceType == 0 ||
		profile.ModelBuild == [2]uint16{} || profile.ProtocolVersion == 0 || strings.TrimSpace(evidenceReference) == "" {
		return GrowattProtocolIIFC04Applicability{}, fmt.Errorf("growatt Protocol II FC04 applicability is invalid")
	}
	return GrowattProtocolIIFC04Applicability{profile: profile, evidenceReference: evidenceReference}, nil
}

func (a GrowattProtocolIIFC04Applicability) matches(identity GrowattProtocolIIIdentityObservation) bool {
	return a.evidenceReference != "" && a.profile == identity.Profile() &&
		a.profile.DeviceType == identity.DeviceType() && a.profile.ModelBuild == identity.ModelBuild() &&
		a.profile.ProtocolVersion == identity.ProtocolVersion()
}

type GrowattProtocolIIInverterState string

const (
	GrowattProtocolIIStateWaiting GrowattProtocolIIInverterState = "waiting"
	GrowattProtocolIIStateNormal  GrowattProtocolIIInverterState = "normal"
	GrowattProtocolIIStateFault   GrowattProtocolIIInverterState = "fault"
)

// GrowattProtocolIIFC04Telemetry is an immutable, read-only subset from the
// exact selected identity. It deliberately omits fields whose signedness or
// interpretation is not established by the pinned source.
type GrowattProtocolIIFC04Telemetry struct {
	identity                                                          GrowattProtocolIIIdentityObservation
	raw                                                               GrowattProtocolIIFC04Slice
	InverterState                                                     GrowattProtocolIIInverterState
	PVInputPowerWatts, OutputPowerWatts, GridFrequencyHz              float64
	Phase1VoltageVolts, Phase1CurrentAmps                             float64
	Phase2VoltageVolts, Phase2CurrentAmps                             float64
	Phase3VoltageVolts, Phase3CurrentAmps                             float64
	TodayGeneratedEnergyKWh, TotalGeneratedEnergyKWh, WorkTimeSeconds float64
}

func (t GrowattProtocolIIFC04Telemetry) Identity() GrowattProtocolIIIdentityObservation {
	return t.identity
}
func (t GrowattProtocolIIFC04Telemetry) RawSlice() GrowattProtocolIIFC04Slice {
	return GrowattProtocolIIFC04Slice{Offset: t.raw.Offset, Words: append([]uint16(nil), t.raw.Words...)}
}
func (GrowattProtocolIIFC04Telemetry) OutboundAllowed() bool { return false }

// DecodeGrowattProtocolIIFC04Telemetry accepts exactly offsets 0 through 58
// only when an independent exact applicability selection admits the FC03
// identity. A caller-selected raw identity alone is insufficient.
func DecodeGrowattProtocolIIFC04Telemetry(identity GrowattProtocolIIIdentityObservation, applicability GrowattProtocolIIFC04Applicability, raw GrowattProtocolIIFC04Slice) (GrowattProtocolIIFC04Telemetry, error) {
	if !validGrowattProtocolIIIdentityProfile(identity.Profile()) || !applicability.matches(identity) || identity.UnitID() == 0 || raw.Offset != 0 || len(raw.Words) != 59 {
		return GrowattProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 telemetry is invalid")
	}
	state, ok := growattProtocolIIState(raw.Words[0])
	if !ok {
		return GrowattProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 inverter state is invalid")
	}
	u32 := func(high, low uint16) uint32 { return uint32(high)<<16 | uint32(low) }
	return GrowattProtocolIIFC04Telemetry{
		identity: identity, raw: GrowattProtocolIIFC04Slice{Offset: raw.Offset, Words: append([]uint16(nil), raw.Words...)}, InverterState: state,
		PVInputPowerWatts: float64(u32(raw.Words[1], raw.Words[2])) / 10, OutputPowerWatts: float64(u32(raw.Words[35], raw.Words[36])) / 10, GridFrequencyHz: float64(raw.Words[37]) / 100,
		Phase1VoltageVolts: float64(raw.Words[38]) / 10, Phase1CurrentAmps: float64(raw.Words[39]) / 10, Phase2VoltageVolts: float64(raw.Words[42]) / 10, Phase2CurrentAmps: float64(raw.Words[43]) / 10, Phase3VoltageVolts: float64(raw.Words[46]) / 10, Phase3CurrentAmps: float64(raw.Words[47]) / 10,
		TodayGeneratedEnergyKWh: float64(u32(raw.Words[53], raw.Words[54])) / 10, TotalGeneratedEnergyKWh: float64(u32(raw.Words[55], raw.Words[56])) / 10, WorkTimeSeconds: float64(u32(raw.Words[57], raw.Words[58])) / 2,
	}, nil
}
func growattProtocolIIState(word uint16) (GrowattProtocolIIInverterState, bool) {
	switch word {
	case 0:
		return GrowattProtocolIIStateWaiting, true
	case 1:
		return GrowattProtocolIIStateNormal, true
	case 3:
		return GrowattProtocolIIStateFault, true
	default:
		return "", false
	}
}
