package modbusreg

import "fmt"

// TeslaLegacyQualification states the evidence tier for a legacy Tesla record.
type TeslaLegacyQualification string

const (
	TeslaLegacyFamilyCompatible TeslaLegacyQualification = "family_compatible"
	TeslaLegacyBuildConfirmed   TeslaLegacyQualification = "build_confirmed"
)

// TeslaLegacyFDE0State is the native legacy dynamic-current state byte.
type TeslaLegacyFDE0State byte

const (
	TeslaLegacyFDE0StatePreCharge TeslaLegacyFDE0State = 0x05
	TeslaLegacyFDE0StateCharging  TeslaLegacyFDE0State = 0x09
)

// IsAbsoluteOffer distinguishes the two documented absolute-offer states from
// native-only relative and unknown state values.
func (state TeslaLegacyFDE0State) IsAbsoluteOffer() bool {
	return state == TeslaLegacyFDE0StatePreCharge || state == TeslaLegacyFDE0StateCharging
}

// TeslaLegacyFDE0ObservationSpec contains a bounded legacy FDE0 observation.
type TeslaLegacyFDE0ObservationSpec struct {
	Qualification   TeslaLegacyQualification
	State           TeslaLegacyFDE0State
	AllocatedMaxCA  uint16
	ActualCurrentCA uint16
	Raw             []byte
}

// TeslaLegacyFDE0Observation retains candidate native current roles without
// inferring device limits, EVSE control, or a protocol-neutral fact.
type TeslaLegacyFDE0Observation struct {
	qualification   TeslaLegacyQualification
	state           TeslaLegacyFDE0State
	allocatedMaxCA  uint16
	actualCurrentCA uint16
	raw             []byte
}

// NewTeslaLegacyFDE0Observation validates and defensively retains a bounded
// native FDE0 observation.
func NewTeslaLegacyFDE0Observation(spec TeslaLegacyFDE0ObservationSpec) (TeslaLegacyFDE0Observation, error) {
	if spec.Qualification != TeslaLegacyFamilyCompatible && spec.Qualification != TeslaLegacyBuildConfirmed {
		return TeslaLegacyFDE0Observation{}, fmt.Errorf("legacy Tesla qualification is unsupported")
	}
	if len(spec.Raw) == 0 || len(spec.Raw) > 256 {
		return TeslaLegacyFDE0Observation{}, fmt.Errorf("legacy Tesla FDE0 raw observation is out of bounds")
	}
	return TeslaLegacyFDE0Observation{qualification: spec.Qualification, state: spec.State, allocatedMaxCA: spec.AllocatedMaxCA, actualCurrentCA: spec.ActualCurrentCA, raw: append([]byte(nil), spec.Raw...)}, nil
}

func (observation TeslaLegacyFDE0Observation) Qualification() TeslaLegacyQualification {
	return observation.qualification
}
func (observation TeslaLegacyFDE0Observation) State() TeslaLegacyFDE0State { return observation.state }
func (observation TeslaLegacyFDE0Observation) AllocatedMaxCA() uint16 {
	return observation.allocatedMaxCA
}
func (observation TeslaLegacyFDE0Observation) ActualCurrentCA() uint16 {
	return observation.actualCurrentCA
}
func (observation TeslaLegacyFDE0Observation) Raw() []byte {
	return append([]byte(nil), observation.raw...)
}
