package modbusreg

import "fmt"

const (
	teslaLegacyFBE0PayloadLength = 7
	teslaLegacyFDE0PayloadLength = 9
)

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

// TeslaLegacyFBE0ObservationSpec binds an FBE0 request to one explicitly
// selected legacy profile and its complete encoded wire frame.
type TeslaLegacyFBE0ObservationSpec struct {
	Profile        TeslaLegacyWallConnectorProfile
	Direction      TeslaLegacyDirection
	Qualification  TeslaLegacyQualification
	State          TeslaLegacyFDE0State
	AllocatedMaxCA uint16
	Raw            []byte
}

// TeslaLegacyFBE0Observation retains one native allocation request. It does
// not establish that an allocation was applied by a device.
type TeslaLegacyFBE0Observation struct {
	qualification  TeslaLegacyQualification
	sourceID       uint16
	destinationID  uint16
	state          TeslaLegacyFDE0State
	allocatedMaxCA uint16
	raw            []byte
}

// NewTeslaLegacyFBE0Observation validates and parses one complete FBE0
// request. The caller-supplied typed fields are checked against the validated
// wire frame, so no typed observation can be detached from its native bytes.
func NewTeslaLegacyFBE0Observation(spec TeslaLegacyFBE0ObservationSpec) (TeslaLegacyFBE0Observation, error) {
	payload, qualification, err := teslaLegacyDynamicCurrentPayload(
		spec.Profile, spec.Direction, spec.Raw,
		TeslaLegacyWallConnectorRequest, TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, teslaLegacyFBE0PayloadLength, teslaLegacyFDE0PayloadLength,
	)
	if err != nil {
		return TeslaLegacyFBE0Observation{}, err
	}
	state, allocatedMaxCA := teslaLegacyAllocationFields(payload)
	if spec.Qualification != qualification || spec.State != state || spec.AllocatedMaxCA != allocatedMaxCA {
		return TeslaLegacyFBE0Observation{}, fmt.Errorf("legacy Tesla FBE0 typed fields do not match validated raw frame")
	}
	return TeslaLegacyFBE0Observation{
		qualification:  qualification,
		sourceID:       teslaLegacyID(payload[0:2]),
		destinationID:  teslaLegacyID(payload[2:4]),
		state:          state,
		allocatedMaxCA: allocatedMaxCA,
		raw:            append([]byte(nil), spec.Raw...),
	}, nil
}

func (observation TeslaLegacyFBE0Observation) Qualification() TeslaLegacyQualification {
	return observation.qualification
}
func (observation TeslaLegacyFBE0Observation) SourceID() uint16 { return observation.sourceID }
func (observation TeslaLegacyFBE0Observation) DestinationID() uint16 {
	return observation.destinationID
}
func (observation TeslaLegacyFBE0Observation) State() TeslaLegacyFDE0State { return observation.state }
func (observation TeslaLegacyFBE0Observation) AllocatedMaxCA() uint16 {
	return observation.allocatedMaxCA
}

// AbsoluteOfferCA exposes an absolute native allocation only for the two
// documented absolute-offer states. Relative and unknown states stay raw-only.
func (observation TeslaLegacyFBE0Observation) AbsoluteOfferCA() (uint16, bool) {
	if !observation.state.IsAbsoluteOffer() {
		return 0, false
	}
	return observation.allocatedMaxCA, true
}

// Raw returns an independent copy of the complete validated encoded frame.
func (observation TeslaLegacyFBE0Observation) Raw() []byte {
	return append([]byte(nil), observation.raw...)
}

// TeslaLegacyFDE0ObservationSpec binds an FDE0 response to one explicitly
// selected legacy profile and its complete encoded wire frame.
type TeslaLegacyFDE0ObservationSpec struct {
	Profile         TeslaLegacyWallConnectorProfile
	Direction       TeslaLegacyDirection
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
	sourceID        uint16
	destinationID   uint16
	state           TeslaLegacyFDE0State
	allocatedMaxCA  uint16
	actualCurrentCA uint16
	raw             []byte
}

// NewTeslaLegacyFDE0Observation validates and parses one complete FDE0
// response. The caller-supplied typed fields are checked against the validated
// wire frame, so no typed observation can be detached from its native bytes.
func NewTeslaLegacyFDE0Observation(spec TeslaLegacyFDE0ObservationSpec) (TeslaLegacyFDE0Observation, error) {
	payload, qualification, err := teslaLegacyDynamicCurrentPayload(
		spec.Profile, spec.Direction, spec.Raw,
		TeslaLegacyWallConnectorResponse, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, teslaLegacyFDE0PayloadLength,
	)
	if err != nil {
		return TeslaLegacyFDE0Observation{}, err
	}
	state, allocatedMaxCA := teslaLegacyAllocationFields(payload)
	actualCurrentCA := teslaLegacyID(payload[7:9])
	if spec.Qualification != qualification || spec.State != state || spec.AllocatedMaxCA != allocatedMaxCA || spec.ActualCurrentCA != actualCurrentCA {
		return TeslaLegacyFDE0Observation{}, fmt.Errorf("legacy Tesla FDE0 typed fields do not match validated raw frame")
	}
	return TeslaLegacyFDE0Observation{
		qualification:   qualification,
		sourceID:        teslaLegacyID(payload[0:2]),
		destinationID:   teslaLegacyID(payload[2:4]),
		state:           state,
		allocatedMaxCA:  allocatedMaxCA,
		actualCurrentCA: actualCurrentCA,
		raw:             append([]byte(nil), spec.Raw...),
	}, nil
}

func (observation TeslaLegacyFDE0Observation) Qualification() TeslaLegacyQualification {
	return observation.qualification
}
func (observation TeslaLegacyFDE0Observation) SourceID() uint16 { return observation.sourceID }
func (observation TeslaLegacyFDE0Observation) DestinationID() uint16 {
	return observation.destinationID
}
func (observation TeslaLegacyFDE0Observation) State() TeslaLegacyFDE0State { return observation.state }
func (observation TeslaLegacyFDE0Observation) AllocatedMaxCA() uint16 {
	return observation.allocatedMaxCA
}
func (observation TeslaLegacyFDE0Observation) ActualCurrentCA() uint16 {
	return observation.actualCurrentCA
}

// AbsoluteOfferCA exposes an absolute native allocation only for the two
// documented absolute-offer states. Relative and unknown states stay raw-only.
func (observation TeslaLegacyFDE0Observation) AbsoluteOfferCA() (uint16, bool) {
	if !observation.state.IsAbsoluteOffer() {
		return 0, false
	}
	return observation.allocatedMaxCA, true
}

// Raw returns an independent copy of the complete validated encoded frame.
func (observation TeslaLegacyFDE0Observation) Raw() []byte {
	return append([]byte(nil), observation.raw...)
}

func teslaLegacyDynamicCurrentPayload(
	profile TeslaLegacyWallConnectorProfile,
	direction TeslaLegacyDirection,
	raw []byte,
	wantDirection TeslaLegacyDirection,
	wantCommand TeslaLegacyCommand,
	wantLengths ...int,
) ([]byte, TeslaLegacyQualification, error) {
	if !profile.config.Enabled {
		return nil, "", fmt.Errorf("tesla legacy Wall Connector profile is disabled")
	}
	qualification := TeslaLegacyQualification(profile.Compatibility())
	if qualification != TeslaLegacyFamilyCompatible && qualification != TeslaLegacyBuildConfirmed {
		return nil, "", fmt.Errorf("legacy Tesla qualification is unsupported")
	}
	if direction != wantDirection {
		return nil, "", fmt.Errorf("legacy Tesla dynamic-current direction is invalid")
	}
	frame, err := DecodeTeslaLegacyWallConnectorFrame(raw)
	if err != nil {
		return nil, "", err
	}
	record, err := profile.NativeRecord(direction, frame)
	if err != nil {
		return nil, "", err
	}
	if record.Command() != wantCommand {
		return nil, "", fmt.Errorf("legacy Tesla dynamic-current command is invalid")
	}
	payload := record.Payload()
	for _, wantLength := range wantLengths {
		if len(payload) == wantLength {
			return payload, qualification, nil
		}
	}
	return nil, "", fmt.Errorf("legacy Tesla dynamic-current payload length is unsupported")
}

func teslaLegacyAllocationFields(payload []byte) (TeslaLegacyFDE0State, uint16) {
	return TeslaLegacyFDE0State(payload[4]), teslaLegacyID(payload[5:7])
}

func teslaLegacyID(bytes []byte) uint16 {
	return uint16(bytes[0])<<8 | uint16(bytes[1])
}
