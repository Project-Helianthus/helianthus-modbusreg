package tesla

import "fmt"

const (
	teslaLegacyHeartbeatDataProtocol1Length = 7
	teslaLegacyHeartbeatDataProtocol2Length = 9
	teslaLegacyAddressContextLength         = 4
	teslaLegacyDynamicCurrentPayload1Length = teslaLegacyAddressContextLength + teslaLegacyHeartbeatDataProtocol1Length
	teslaLegacyDynamicCurrentPayload2Length = teslaLegacyAddressContextLength + teslaLegacyHeartbeatDataProtocol2Length
)

// LegacyQualification states the evidence tier for a legacy Tesla record.
type LegacyQualification string

// Qualification tiers, taken from the selected profile's compatibility:
// FamilyCompatible claims no firmware build; BuildConfirmed is the only
// exact-build tier. A profile with unknown compatibility yields no typed
// record. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/tesla/legacy-wall-connector-rs485.md.
const (
	LegacyFamilyCompatible LegacyQualification = "family_compatible"
	LegacyBuildConfirmed   LegacyQualification = "build_confirmed"
)

// LegacyFDE0State is the native legacy dynamic-current state byte.
type LegacyFDE0State byte

// The two absolute-offer states: 0x05 pre-charge and 0x09 charging. Every
// other state byte stays native-only.
const (
	LegacyFDE0StatePreCharge LegacyFDE0State = 0x05
	LegacyFDE0StateCharging  LegacyFDE0State = 0x09
)

// IsAbsoluteOffer distinguishes the two documented absolute-offer states from
// native-only relative and unknown state values.
func (state LegacyFDE0State) IsAbsoluteOffer() bool {
	return state == LegacyFDE0StatePreCharge || state == LegacyFDE0StateCharging
}

// LegacyFBE0ObservationSpec binds an FBE0 request to one explicitly
// selected legacy profile and its complete encoded wire frame.
type LegacyFBE0ObservationSpec struct {
	Profile        LegacyWallConnectorProfile
	Direction      LegacyDirection
	Qualification  LegacyQualification
	State          LegacyFDE0State
	AllocatedMaxCA uint16
	Raw            []byte
}

// LegacyFBE0Observation retains one native allocation request. It does
// not establish that an allocation was applied by a device.
type LegacyFBE0Observation struct {
	qualification  LegacyQualification
	sourceID       uint16
	destinationID  uint16
	state          LegacyFDE0State
	allocatedMaxCA uint16
	raw            []byte
}

// NewLegacyFBE0Observation validates and parses one complete FBE0
// request. The caller-supplied typed fields are checked against the validated
// wire frame, so no typed observation can be detached from its native bytes.
func NewLegacyFBE0Observation(spec LegacyFBE0ObservationSpec) (LegacyFBE0Observation, error) {
	payload, qualification, err := teslaLegacyDynamicCurrentPayload(
		spec.Profile, spec.Direction, spec.Raw,
		LegacyWallConnectorRequest, LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}, teslaLegacyDynamicCurrentPayload1Length, teslaLegacyDynamicCurrentPayload2Length,
	)
	if err != nil {
		return LegacyFBE0Observation{}, err
	}
	state, allocatedMaxCA := teslaLegacyAllocationFields(payload)
	if spec.Qualification != qualification || spec.State != state || spec.AllocatedMaxCA != allocatedMaxCA {
		return LegacyFBE0Observation{}, fmt.Errorf("legacy Tesla FBE0 typed fields do not match validated raw frame")
	}
	return LegacyFBE0Observation{
		qualification:  qualification,
		sourceID:       teslaLegacyID(payload[0:2]),
		destinationID:  teslaLegacyID(payload[2:4]),
		state:          state,
		allocatedMaxCA: allocatedMaxCA,
		raw:            append([]byte(nil), spec.Raw...),
	}, nil
}

// Qualification returns the evidence tier of the profile that validated the frame.
func (observation LegacyFBE0Observation) Qualification() LegacyQualification {
	return observation.qualification
}

// SourceID returns the big-endian source ID from payload bytes 0-1.
func (observation LegacyFBE0Observation) SourceID() uint16 { return observation.sourceID }

// DestinationID returns the big-endian destination ID from payload bytes 2-3.
func (observation LegacyFBE0Observation) DestinationID() uint16 {
	return observation.destinationID
}

// State returns the raw state byte, payload byte 4, whatever its value.
func (observation LegacyFBE0Observation) State() LegacyFDE0State { return observation.state }

// AllocatedMaxCA returns the raw big-endian allocated current from payload bytes
// 5-6, in centiamperes, for every state; use AbsoluteOfferCA for an absolute value.
func (observation LegacyFBE0Observation) AllocatedMaxCA() uint16 {
	return observation.allocatedMaxCA
}

// AbsoluteOfferCA exposes an absolute native allocation only for the two
// documented absolute-offer states. Relative and unknown states stay raw-only.
func (observation LegacyFBE0Observation) AbsoluteOfferCA() (uint16, bool) {
	if !observation.state.IsAbsoluteOffer() {
		return 0, false
	}
	return observation.allocatedMaxCA, true
}

// Raw returns an independent copy of the complete validated encoded frame.
func (observation LegacyFBE0Observation) Raw() []byte {
	return append([]byte(nil), observation.raw...)
}

// LegacyFDE0ObservationSpec binds an FDE0 response to one explicitly
// selected legacy profile and its complete encoded wire frame.
type LegacyFDE0ObservationSpec struct {
	Profile         LegacyWallConnectorProfile
	Direction       LegacyDirection
	Qualification   LegacyQualification
	State           LegacyFDE0State
	AllocatedMaxCA  uint16
	ActualCurrentCA uint16
	Raw             []byte
}

// LegacyFDE0Observation retains candidate native current roles without
// inferring device limits, EVSE control, or a protocol-neutral fact.
type LegacyFDE0Observation struct {
	qualification   LegacyQualification
	sourceID        uint16
	destinationID   uint16
	state           LegacyFDE0State
	allocatedMaxCA  uint16
	actualCurrentCA uint16
	raw             []byte
}

// NewLegacyFDE0Observation validates and parses one complete FDE0
// response. The caller-supplied typed fields are checked against the validated
// wire frame, so no typed observation can be detached from its native bytes.
func NewLegacyFDE0Observation(spec LegacyFDE0ObservationSpec) (LegacyFDE0Observation, error) {
	payload, qualification, err := teslaLegacyDynamicCurrentPayload(
		spec.Profile, spec.Direction, spec.Raw,
		LegacyWallConnectorResponse, LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}, teslaLegacyDynamicCurrentPayload1Length, teslaLegacyDynamicCurrentPayload2Length,
	)
	if err != nil {
		return LegacyFDE0Observation{}, err
	}
	state, allocatedMaxCA := teslaLegacyAllocationFields(payload)
	actualCurrentCA := teslaLegacyID(payload[7:9])
	if spec.Qualification != qualification || spec.State != state || spec.AllocatedMaxCA != allocatedMaxCA || spec.ActualCurrentCA != actualCurrentCA {
		return LegacyFDE0Observation{}, fmt.Errorf("legacy Tesla FDE0 typed fields do not match validated raw frame")
	}
	return LegacyFDE0Observation{
		qualification:   qualification,
		sourceID:        teslaLegacyID(payload[0:2]),
		destinationID:   teslaLegacyID(payload[2:4]),
		state:           state,
		allocatedMaxCA:  allocatedMaxCA,
		actualCurrentCA: actualCurrentCA,
		raw:             append([]byte(nil), spec.Raw...),
	}, nil
}

// Qualification returns the evidence tier of the profile that validated the frame.
func (observation LegacyFDE0Observation) Qualification() LegacyQualification {
	return observation.qualification
}

// SourceID returns the big-endian source ID from payload bytes 0-1.
func (observation LegacyFDE0Observation) SourceID() uint16 { return observation.sourceID }

// DestinationID returns the big-endian destination ID from payload bytes 2-3.
func (observation LegacyFDE0Observation) DestinationID() uint16 {
	return observation.destinationID
}

// State returns the raw state byte, payload byte 4, whatever its value.
func (observation LegacyFDE0Observation) State() LegacyFDE0State { return observation.state }

// AllocatedMaxCA returns the raw big-endian allocated current from payload bytes
// 5-6, in centiamperes, for every state; use AbsoluteOfferCA for an absolute value.
func (observation LegacyFDE0Observation) AllocatedMaxCA() uint16 {
	return observation.allocatedMaxCA
}

// ActualCurrentCA returns the raw big-endian actual current from payload bytes
// 7-8, in centiamperes.
func (observation LegacyFDE0Observation) ActualCurrentCA() uint16 {
	return observation.actualCurrentCA
}

// AbsoluteOfferCA exposes an absolute native allocation only for the two
// documented absolute-offer states. Relative and unknown states stay raw-only.
func (observation LegacyFDE0Observation) AbsoluteOfferCA() (uint16, bool) {
	if !observation.state.IsAbsoluteOffer() {
		return 0, false
	}
	return observation.allocatedMaxCA, true
}

// Raw returns an independent copy of the complete validated encoded frame.
func (observation LegacyFDE0Observation) Raw() []byte {
	return append([]byte(nil), observation.raw...)
}

func teslaLegacyDynamicCurrentPayload(
	profile LegacyWallConnectorProfile,
	direction LegacyDirection,
	raw []byte,
	wantDirection LegacyDirection,
	wantCommand LegacyCommand,
	wantLengths ...int,
) ([]byte, LegacyQualification, error) {
	if !profile.config.Enabled {
		return nil, "", fmt.Errorf("tesla legacy Wall Connector profile is disabled")
	}
	qualification := LegacyQualification(profile.Compatibility())
	if qualification != LegacyFamilyCompatible && qualification != LegacyBuildConfirmed {
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

func teslaLegacyAllocationFields(payload []byte) (LegacyFDE0State, uint16) {
	return LegacyFDE0State(payload[4]), teslaLegacyID(payload[5:7])
}

func teslaLegacyID(bytes []byte) uint16 {
	return uint16(bytes[0])<<8 | uint16(bytes[1])
}
