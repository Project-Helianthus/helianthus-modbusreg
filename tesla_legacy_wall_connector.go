package modbusreg

import "fmt"

const maxTeslaLegacyWallConnectorMessage = 256

// TeslaLegacyWallConnectorCompatibility identifies the evidence tier selected
// for the legacy, pre-Gen3 Wall Connector RS-485 family.
type TeslaLegacyWallConnectorCompatibility string

const (
	TeslaLegacyWallConnectorBuildConfirmed   TeslaLegacyWallConnectorCompatibility = "build_confirmed"
	TeslaLegacyWallConnectorFamilyCompatible TeslaLegacyWallConnectorCompatibility = "family_compatible"
	TeslaLegacyWallConnectorUnknown          TeslaLegacyWallConnectorCompatibility = "unknown"
)

// TeslaLegacyWallConnectorGeneration records the deliberately non-exclusive
// legacy generation classification.
type TeslaLegacyWallConnectorGeneration string

const TeslaLegacyWallConnectorGenerationCandidate TeslaLegacyWallConnectorGeneration = "pre_gen3_candidate"

// TeslaLegacyWallConnectorProfileConfig selects a legacy protocol tier without
// treating a Tesla name or Gen3 profile as compatibility evidence.
type TeslaLegacyWallConnectorProfileConfig struct {
	Enabled       bool
	Compatibility TeslaLegacyWallConnectorCompatibility
}

// TeslaLegacyWallConnectorProfile is independent from the Gen3 HSC profile.
type TeslaLegacyWallConnectorProfile struct {
	config TeslaLegacyWallConnectorProfileConfig
}

// NewTeslaLegacyWallConnectorProfile validates the explicit legacy selection.
func NewTeslaLegacyWallConnectorProfile(
	config TeslaLegacyWallConnectorProfileConfig,
) (TeslaLegacyWallConnectorProfile, error) {
	switch config.Compatibility {
	case TeslaLegacyWallConnectorBuildConfirmed,
		TeslaLegacyWallConnectorFamilyCompatible,
		TeslaLegacyWallConnectorUnknown:
	default:
		return TeslaLegacyWallConnectorProfile{}, fmt.Errorf("tesla legacy Wall Connector compatibility is invalid")
	}
	return TeslaLegacyWallConnectorProfile{config: config}, nil
}

// Generation returns the non-exclusive legacy generation classification.
func (TeslaLegacyWallConnectorProfile) Generation() TeslaLegacyWallConnectorGeneration {
	return TeslaLegacyWallConnectorGenerationCandidate
}

// Compatibility returns the immutable evidence tier selected for this profile.
func (profile TeslaLegacyWallConnectorProfile) Compatibility() TeslaLegacyWallConnectorCompatibility {
	return profile.config.Compatibility
}

// TeslaLegacyDirection identifies an offline request or response record.
type TeslaLegacyDirection string

const (
	TeslaLegacyWallConnectorRequest  TeslaLegacyDirection = "request"
	TeslaLegacyWallConnectorResponse TeslaLegacyDirection = "response"
)

// TeslaLegacyCommand is the two-byte legacy command identity.
type TeslaLegacyCommand struct {
	Prefix byte
	Opcode byte
}

// TeslaLegacyOperation names only the documented link and heartbeat forms.
type TeslaLegacyOperation string

const (
	TeslaLegacyOperationUnknown            TeslaLegacyOperation = "unknown"
	TeslaLegacyOperationPrimaryDiscovery   TeslaLegacyOperation = "primary_discovery"
	TeslaLegacyOperationSecondaryDiscovery TeslaLegacyOperation = "secondary_discovery"
	TeslaLegacyOperationPrimaryHeartbeat   TeslaLegacyOperation = "primary_heartbeat"
	TeslaLegacyOperationSecondaryHeartbeat TeslaLegacyOperation = "secondary_heartbeat"
)

// TeslaLegacyWallConnectorFrame is a validated, unescaped legacy frame.
type TeslaLegacyWallConnectorFrame struct {
	command TeslaLegacyCommand
	payload []byte
}

// DecodeTeslaLegacyWallConnectorFrame validates one complete SLIP-like wire
// frame. It neither opens a serial transport nor interprets unknown payloads.
func DecodeTeslaLegacyWallConnectorFrame(
	wire []byte,
) (TeslaLegacyWallConnectorFrame, error) {
	if len(wire) < 5 || len(wire) > maxTeslaLegacyWallConnectorMessage+2 {
		return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame size is invalid")
	}
	if wire[0] != 0xc0 || wire[len(wire)-1] != 0xc0 {
		return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame delimiters are invalid")
	}
	message := make([]byte, 0, len(wire)-2)
	for index := 1; index < len(wire)-1; index++ {
		value := wire[index]
		switch value {
		case 0xc0:
			return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame contains an unescaped delimiter")
		case 0xdb:
			if index+1 >= len(wire)-1 {
				return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame escape is incomplete")
			}
			index++
			switch wire[index] {
			case 0xdc:
				message = append(message, 0xc0)
			case 0xdd:
				message = append(message, 0xdb)
			default:
				return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector frame escape is invalid")
			}
		default:
			message = append(message, value)
		}
	}
	if len(message) < 3 || len(message) > maxTeslaLegacyWallConnectorMessage {
		return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector message size is invalid")
	}
	if message[len(message)-1] != teslaLegacyWallConnectorChecksum(message[:len(message)-1]) {
		return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector checksum is invalid")
	}
	command := TeslaLegacyCommand{Prefix: message[0], Opcode: message[1]}
	if !teslaLegacyCommandPrefix(command.Prefix) {
		return TeslaLegacyWallConnectorFrame{}, fmt.Errorf("tesla legacy Wall Connector command prefix is invalid")
	}
	return TeslaLegacyWallConnectorFrame{
		command: command,
		payload: append([]byte(nil), message[2:len(message)-1]...),
	}, nil
}

// EncodeTeslaLegacyWallConnectorFrame constructs one bounded legacy wire frame
// for offline testing or a future separately authorized transport dispatch.
func EncodeTeslaLegacyWallConnectorFrame(command TeslaLegacyCommand, payload []byte) ([]byte, error) {
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
func (frame TeslaLegacyWallConnectorFrame) Command() TeslaLegacyCommand {
	return frame.command
}

// Payload returns an independent copy of the complete native payload.
func (frame TeslaLegacyWallConnectorFrame) Payload() []byte {
	return append([]byte(nil), frame.payload...)
}

// Operation returns a name only for the documented link and heartbeat forms.
func (frame TeslaLegacyWallConnectorFrame) Operation() TeslaLegacyOperation {
	switch frame.command {
	case TeslaLegacyCommand{Prefix: 0xfc, Opcode: 0xe1}:
		return TeslaLegacyOperationPrimaryDiscovery
	case TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe2}, TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe2}:
		return TeslaLegacyOperationSecondaryDiscovery
	case TeslaLegacyCommand{Prefix: 0xfb, Opcode: 0xe0}:
		return TeslaLegacyOperationPrimaryHeartbeat
	case TeslaLegacyCommand{Prefix: 0xfd, Opcode: 0xe0}:
		return TeslaLegacyOperationSecondaryHeartbeat
	default:
		return TeslaLegacyOperationUnknown
	}
}

// TeslaLegacyWallConnectorRecord is a native offline request or response
// record. It intentionally keeps its payload rather than reducing it to a
// digest or inferred field set.
type TeslaLegacyWallConnectorRecord struct {
	compatibility TeslaLegacyWallConnectorCompatibility
	direction     TeslaLegacyDirection
	command       TeslaLegacyCommand
	operation     TeslaLegacyOperation
	payload       []byte
}

// NativeRecord retains one validated legacy frame under the selected profile.
func (profile TeslaLegacyWallConnectorProfile) NativeRecord(
	direction TeslaLegacyDirection,
	frame TeslaLegacyWallConnectorFrame,
) (TeslaLegacyWallConnectorRecord, error) {
	if !profile.config.Enabled {
		return TeslaLegacyWallConnectorRecord{}, fmt.Errorf("tesla legacy Wall Connector profile is disabled")
	}
	if direction != TeslaLegacyWallConnectorRequest && direction != TeslaLegacyWallConnectorResponse {
		return TeslaLegacyWallConnectorRecord{}, fmt.Errorf("tesla legacy Wall Connector direction is invalid")
	}
	return TeslaLegacyWallConnectorRecord{
		compatibility: profile.Compatibility(), direction: direction, command: frame.Command(),
		operation: frame.Operation(), payload: frame.Payload(),
	}, nil
}

// Compatibility returns the selected legacy evidence tier.
func (record TeslaLegacyWallConnectorRecord) Compatibility() TeslaLegacyWallConnectorCompatibility {
	return record.compatibility
}

// Direction returns whether the native record is a request or response.
func (record TeslaLegacyWallConnectorRecord) Direction() TeslaLegacyDirection {
	return record.direction
}

// Command returns the complete native command identity.
func (record TeslaLegacyWallConnectorRecord) Command() TeslaLegacyCommand {
	return record.command
}

// Operation returns a named operation only when its command form is documented.
func (record TeslaLegacyWallConnectorRecord) Operation() TeslaLegacyOperation {
	return record.operation
}

// Payload returns an independent copy of the exact native payload.
func (record TeslaLegacyWallConnectorRecord) Payload() []byte {
	return append([]byte(nil), record.payload...)
}
