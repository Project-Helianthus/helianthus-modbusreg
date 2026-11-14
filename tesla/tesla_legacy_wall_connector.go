package tesla

import "fmt"

const maxTeslaLegacyWallConnectorMessage = 256

// LegacyWallConnectorCompatibility identifies the evidence tier selected
// for the legacy, pre-Gen3 Wall Connector RS-485 family.
type LegacyWallConnectorCompatibility string

// Evidence tiers. BuildConfirmed: the exact known build label. FamilyCompatible:
// a locally declared legacy family, without a build claim. Unknown: missing,
// contradictory or unknown identity; it inherits no build-specific field
// interpretation. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/tesla/legacy-wall-connector-rs485.md.
const (
	LegacyWallConnectorBuildConfirmed   LegacyWallConnectorCompatibility = "build_confirmed"
	LegacyWallConnectorFamilyCompatible LegacyWallConnectorCompatibility = "family_compatible"
	LegacyWallConnectorUnknown          LegacyWallConnectorCompatibility = "unknown"
)

// LegacyWallConnectorGeneration records the deliberately non-exclusive
// legacy generation classification.
type LegacyWallConnectorGeneration string

// LegacyWallConnectorGenerationCandidate is the only generation value: a
// pre-Gen3 candidate, never an exclusive hardware generation claim.
const LegacyWallConnectorGenerationCandidate LegacyWallConnectorGeneration = "pre_gen3_candidate"

// LegacyWallConnectorProfileConfig selects a legacy protocol tier without
// treating a Tesla name or Gen3 profile as compatibility evidence.
type LegacyWallConnectorProfileConfig struct {
	Enabled       bool
	Compatibility LegacyWallConnectorCompatibility
}

// LegacyWallConnectorProfile is independent from the Gen3 HSC profile.
type LegacyWallConnectorProfile struct {
	config LegacyWallConnectorProfileConfig
}

// NewLegacyWallConnectorProfile validates the explicit legacy selection.
func NewLegacyWallConnectorProfile(
	config LegacyWallConnectorProfileConfig,
) (LegacyWallConnectorProfile, error) {
	switch config.Compatibility {
	case LegacyWallConnectorBuildConfirmed,
		LegacyWallConnectorFamilyCompatible,
		LegacyWallConnectorUnknown:
	default:
		return LegacyWallConnectorProfile{}, fmt.Errorf("tesla legacy Wall Connector compatibility is invalid")
	}
	return LegacyWallConnectorProfile{config: config}, nil
}

// Generation returns the non-exclusive legacy generation classification.
func (LegacyWallConnectorProfile) Generation() LegacyWallConnectorGeneration {
	return LegacyWallConnectorGenerationCandidate
}

// Compatibility returns the immutable evidence tier selected for this profile.
func (profile LegacyWallConnectorProfile) Compatibility() LegacyWallConnectorCompatibility {
	return profile.config.Compatibility
}

// LegacyDirection identifies an offline request or response record.
type LegacyDirection string

// Record directions for offline request and response frames.
const (
	LegacyWallConnectorRequest  LegacyDirection = "request"
	LegacyWallConnectorResponse LegacyDirection = "response"
)

// LegacyCommand is the two-byte legacy command identity.
type LegacyCommand struct {
	Prefix byte
	Opcode byte
}

// LegacyOperation names only the documented link and heartbeat forms.
type LegacyOperation string

// Operations by command: FCE1 primary discovery; FBE2 and FDE2 secondary
// discovery; FBE0 primary heartbeat; FDE0 secondary heartbeat. Every other
// command is Unknown and its payload stays native.
const (
	LegacyOperationUnknown            LegacyOperation = "unknown"
	LegacyOperationPrimaryDiscovery   LegacyOperation = "primary_discovery"
	LegacyOperationSecondaryDiscovery LegacyOperation = "secondary_discovery"
	LegacyOperationPrimaryHeartbeat   LegacyOperation = "primary_heartbeat"
	LegacyOperationSecondaryHeartbeat LegacyOperation = "secondary_heartbeat"
)

// LegacyWallConnectorFrame is a validated, unescaped legacy frame.
type LegacyWallConnectorFrame struct {
	command LegacyCommand
	payload []byte
}

// DecodeTeslaLegacyWallConnectorFrame validates one complete SLIP-like wire
// frame. It neither opens a serial transport nor interprets unknown payloads.
func DecodeTeslaLegacyWallConnectorFrame(
	wire []byte,
) (LegacyWallConnectorFrame, error) {
	if len(wire) < 5 || len(wire) > maxTeslaLegacyWallConnectorMessage*2+2 {
		return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame size is invalid")
	}
	if wire[0] != 0xc0 || wire[len(wire)-1] != 0xc0 {
		return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame delimiters are invalid")
	}
	message := make([]byte, 0, len(wire)-2)
	for index := 1; index < len(wire)-1; index++ {
		value := wire[index]
		switch value {
		case 0xc0:
			return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame contains an unescaped delimiter")
		case 0xdb:
			if index+1 >= len(wire)-1 {
				return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame escape is incomplete")
			}
			index++
			switch wire[index] {
			case 0xdc:
				message = append(message, 0xc0)
			case 0xdd:
				message = append(message, 0xdb)
			default:
				return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame escape is invalid")
			}
		default:
			message = append(message, value)
		}
	}
	if len(message) < 3 || len(message) > maxTeslaLegacyWallConnectorMessage {
		return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector message size is invalid")
	}
	if message[len(message)-1] != teslaLegacyWallConnectorChecksum(message[:len(message)-1]) {
		return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector checksum is invalid")
	}
	command := LegacyCommand{Prefix: message[0], Opcode: message[1]}
	if !teslaLegacyCommandPrefix(command.Prefix) {
		return LegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector command prefix is invalid")
	}
	return LegacyWallConnectorFrame{
		command: command,
		payload: append([]byte(nil), message[2:len(message)-1]...),
	}, nil
}

// EncodeTeslaLegacyWallConnectorFrame constructs one bounded legacy wire frame
// for offline testing or a future separately authorized transport dispatch.
func EncodeTeslaLegacyWallConnectorFrame(command LegacyCommand, payload []byte) ([]byte, error) {
	if !teslaLegacyCommandPrefix(command.Prefix) {
		return nil, fmt.Errorf("tesla legacy Wall Connector command prefix is invalid")
	}
	if len(payload)+3 > maxTeslaLegacyWallConnectorMessage {
		return nil, fmt.Errorf("tesla legacy Wall Connector message exceeds bound")
	}
	message := make([]byte, 0, len(payload)+3)
	message = append(message, command.Prefix, command.Opcode)
	message = append(message, payload...)
	message = append(message, teslaLegacyWallConnectorChecksum(message))
	wire := make([]byte, 0, len(message)*2+2)
	wire = append(wire, 0xc0)
	for _, value := range message {
		switch value {
		case 0xc0:
			wire = append(wire, 0xdb, 0xdc)
		case 0xdb:
			wire = append(wire, 0xdb, 0xdd)
		default:
			wire = append(wire, value)
		}
	}
	wire = append(wire, 0xc0)
	return wire, nil
}

func teslaLegacyWallConnectorChecksum(message []byte) byte {
	var sum byte
	for _, value := range message[1:] {
		sum += value
	}
	return sum
}

func teslaLegacyCommandPrefix(prefix byte) bool {
	return prefix == 0xfb || prefix == 0xfc || prefix == 0xfd
}

// Command returns the two-byte native command identity.
func (frame LegacyWallConnectorFrame) Command() LegacyCommand {
	return frame.command
}

// Payload returns an independent copy of the complete native payload.
func (frame LegacyWallConnectorFrame) Payload() []byte {
	return append([]byte(nil), frame.payload...)
}

// Operation returns a name only for the documented link and heartbeat forms.
func (frame LegacyWallConnectorFrame) Operation() LegacyOperation {
	switch frame.command {
	case LegacyCommand{Prefix: 0xfc, Opcode: 0xe1}:
		return LegacyOperationPrimaryDiscovery
	case LegacyCommand{Prefix: 0xfb, Opcode: 0xe2}, LegacyCommand{Prefix: 0xfd, Opcode: 0xe2}:
		return LegacyOperationSecondaryDiscovery
	case LegacyCommand{Prefix: 0xfb, Opcode: 0xe0}:
		return LegacyOperationPrimaryHeartbeat
	case LegacyCommand{Prefix: 0xfd, Opcode: 0xe0}:
		return LegacyOperationSecondaryHeartbeat
	default:
		return LegacyOperationUnknown
	}
}

// LegacyWallConnectorRecord is a native offline request or response
// record. It intentionally keeps its payload rather than reducing it to a
// digest or inferred field set.
type LegacyWallConnectorRecord struct {
	compatibility LegacyWallConnectorCompatibility
	direction     LegacyDirection
	command       LegacyCommand
	operation     LegacyOperation
	payload       []byte
}

// NativeRecord retains one validated legacy frame under the selected profile.
func (profile LegacyWallConnectorProfile) NativeRecord(
	direction LegacyDirection,
	frame LegacyWallConnectorFrame,
) (LegacyWallConnectorRecord, error) {
	if !profile.config.Enabled {
		return LegacyWallConnectorRecord{}, fmt.Errorf("tesla legacy Wall Connector profile is disabled")
	}
	if direction != LegacyWallConnectorRequest && direction != LegacyWallConnectorResponse {
		return LegacyWallConnectorRecord{}, fmt.Errorf("tesla legacy Wall Connector direction is invalid")
	}
	return LegacyWallConnectorRecord{
		compatibility: profile.Compatibility(), direction: direction, command: frame.Command(),
		operation: frame.Operation(), payload: frame.Payload(),
	}, nil
}

// Compatibility returns the selected legacy evidence tier.
func (record LegacyWallConnectorRecord) Compatibility() LegacyWallConnectorCompatibility {
	return record.compatibility
}

// Direction returns whether the native record is a request or response.
func (record LegacyWallConnectorRecord) Direction() LegacyDirection {
	return record.direction
}

// Command returns the complete native command identity.
func (record LegacyWallConnectorRecord) Command() LegacyCommand {
	return record.command
}

// Operation returns a named operation only when its command form is documented.
func (record LegacyWallConnectorRecord) Operation() LegacyOperation {
	return record.operation
}

// Payload returns an independent copy of the exact native payload.
func (record LegacyWallConnectorRecord) Payload() []byte {
	return append([]byte(nil), record.payload...)
}
