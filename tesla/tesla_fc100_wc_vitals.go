package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"unicode/utf8"
)

// TEDAPIOperationWCVitalsV1 identifies the explicitly qualified FC100
// WC vitals operation in this compatibility contract.
const TEDAPIOperationWCVitalsV1 = "tesla.hsc.fc100.wc_vitals.v1"

var teslaFC100WCVitalsRequestPDU = []byte{0x04, 0x32, 0x02, 0x0a, 0x00}

// FC100WCVitalsReplayKind distinguishes the echoed FC100 intermediate
// from the qualified terminal snapshot without exposing field values.
type FC100WCVitalsReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the terminal body.
const (
	FC100WCVitalsIntermediate FC100WCVitalsReplayKind = "intermediate"
	FC100WCVitalsTerminal     FC100WCVitalsReplayKind = "terminal"
)

// FC100WCVitalsReplay describes a bounded echo or terminal. The decoder that
// accepts an identified firmware also retains the terminal body.
type FC100WCVitalsReplay struct {
	Kind           FC100WCVitalsReplayKind
	SnapshotLength int
	SnapshotDigest string
	terminal       *wcVitalsTerminal
}

type wcVitalsTerminal struct {
	body      []byte
	vitals    WCVitals
	hasVitals bool
}

// TypedValueDisposition distinguishes a decoded vitals message from an echo,
// a reply without a vitals member, or an untyped terminal.
func (replay FC100WCVitalsReplay) TypedValueDisposition() string {
	if replay.terminal != nil && replay.terminal.hasVitals {
		return "typed"
	}
	return "unknown"
}

// Vitals returns a value copy, with the boolean indicating message presence.
func (replay FC100WCVitalsReplay) Vitals() (WCVitals, bool) {
	if replay.terminal == nil {
		return WCVitals{}, false
	}
	return replay.terminal.vitals, replay.terminal.hasVitals
}

// TerminalBody returns a detached copy of the terminal retained by the
// firmware-aware decoder, including members not projected into Vitals.
func (replay FC100WCVitalsReplay) TerminalBody() []byte {
	if replay.terminal == nil {
		return nil
	}
	return bytes.Clone(replay.terminal.body)
}

// WCVitalsField retains wire presence separately from the protobuf default.
type WCVitalsField[T any] struct {
	Value   T
	Present bool
}

// WCVitals describes station-reported telemetry (TEDAPI Specification §§6.4.1,
// 6.6.1, 8 and 9.3). For a wrapper, the outer presence determines whether it
// reports a value; the inner presence records whether the scalar was encoded.
type WCVitals struct {
	UptimeSeconds            WCVitalsField[uint32]
	ContactorClosed          WCVitalsField[bool]
	VehicleConnected         WCVitalsField[bool]
	PCBATempC                WCVitalsField[float32]
	HandleTempC              WCVitalsField[float32]
	MCUTempC                 WCVitalsField[float32]
	InputThermopileV         WCVitalsField[float32]
	ProxV                    WCVitalsField[float32]
	PilotHighV               WCVitalsField[float32]
	PilotLowV                WCVitalsField[float32]
	SessionDurationSeconds   WCVitalsField[uint32]
	VehicleAC                WCVitalsField[WCVitalsVehicleAC]
	GroundResistanceOhms     WCVitalsField[float32]
	LineEarthVrms            WCVitalsField[float32]
	NeutralEarthVrms         WCVitalsField[float32]
	FaultStatus              WCVitalsField[int32] // Enumeration not specified.
	VIN                      WCVitalsField[WCVitalsField[string]]
	InstantaneousPowerWatts  WCVitalsField[WCVitalsField[float32]]
	ScheduledChargingStatus  WCVitalsField[int32]                  // Enumeration not specified.
	InstantaneousLineCurrent WCVitalsField[WCVitalsField[float32]] // Amperes (Provisional).
	OCPPStatus               WCVitalsField[int32]
	PowershareSessionState   WCVitalsField[int32] // Enumeration association Provisional.
	DryContactControlState   WCVitalsField[int32] // Enumeration association Provisional.
	SmartChargingReason      WCVitalsField[int32] // Enumeration association Provisional.
}

// WCVitalsVehicleAC projects the specified vehicle-side RMS voltage.
type WCVitalsVehicleAC struct {
	VoltageVrms WCVitalsField[float32]
}

// OCPPStatusName returns the specified enumeration name when the wire code
// is present and listed. This is not a claim about behavior on a given build.
func (vitals WCVitals) OCPPStatusName() (string, bool) {
	names := [...]string{"INVALID", "UNCONFIGURED", "INCOMPLETE_CONFIGURATION", "CONFIGURED_DISABLED", "CONFIGURED_ENABLED", "CONFIGURED_ENABLED_CONNECTED", "CONFIGURED_ENABLED_INVALID_CONFIGURATION"}
	code := vitals.OCPPStatus.Value
	if !vitals.OCPPStatus.Present || code < 0 || code >= int32(len(names)) {
		return "", false
	}
	return names[code], true
}

// OperationAdmissionFor applies the profile gate and the operation-specific
// exact version gate. Unknown operations remain no-send.
func (profile HSCProfile) OperationAdmissionFor(operation string) TEDAPIOperationAdmission {
	if profile.Disposition() != HSCQualifiedReadOnly {
		return TEDAPIOperationAdmission{State: TEDAPIAdmissionBlockedProfile}
	}
	switch operation {
	case TEDAPIOperationWCVitalsV1:
		if profile.config.WCVitalsOperationVersion == HSCWCVitalsCompatibilityV1 {
			return TEDAPIOperationAdmission{
				State:           TEDAPIAdmissionAllowedWCVitals,
				OutboundAllowed: true,
			}
		}
	case TEDAPIOperationCommonSystemInfoV1:
		if profile.config.SystemInfoOperationVersion == HSCSystemInfoCompatibilityV1 {
			return TEDAPIOperationAdmission{
				State:           TEDAPIAdmissionAllowedCommonSystemInfo,
				OutboundAllowed: true,
			}
		}
	case TEDAPIOperationWCLifetimeV1:
		if profile.config.WCLifetimeOperationVersion == HSCWCLifetimeCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCLifetime, OutboundAllowed: true}
		}
	case TEDAPIOperationWCSystemInfoV1:
		if profile.config.WCSystemInfoOperationVersion == HSCWCSystemInfoCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCSystemInfo, OutboundAllowed: true}
		}
	case TEDAPIOperationWCProvisionalReadback:
		if profile.config.WCProvisionalReadbackOperationVersion == HSCFC100OperationCompatibilityV1 && profile.config.WCProvisionalReadbackQualified {
			return TEDAPIOperationAdmission{State: "allowed_wc_provisional_readback", OutboundAllowed: true}
		}
	case TEDAPIOperationWCLoadSharingStateV1:
		if profile.config.WCLoadSharingStateOperationVersion == HSCWCLoadSharingStateCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCLoadSharingState, OutboundAllowed: true}
		}
	}
	return TEDAPIOperationAdmission{State: TEDAPIAdmissionBlockedNoAdmissibleOperation}
}

// DecodeTeslaFC100WCVitalsReplay decodes the exact FC100 WC vitals operation
// shape. It accepts the byte-exact echo or one fully bounded terminal response.
func DecodeTeslaFC100WCVitalsReplay(payload []byte) (FC100WCVitalsReplay, error) {
	return decodeWCVitalsReplay(payload, "", false)
}

// DecodeWCVitalsReplayForFirmware retains the exact terminal and types its
// vitals only when the identified firmware has a specified RS-485 vitals layout.
func DecodeWCVitalsReplayForFirmware(payload []byte, firmware string) (FC100WCVitalsReplay, error) {
	return decodeWCVitalsReplay(payload, firmware, true)
}

func decodeWCVitalsReplay(payload []byte, firmware string, retainBody bool) (FC100WCVitalsReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return FC100WCVitalsReplay{}, err
	}
	message := response.Payload()
	requestMessage := teslaFC100WCVitalsRequestPDU[1:]
	if bytes.Equal(message, requestMessage) {
		return FC100WCVitalsReplay{Kind: FC100WCVitalsIntermediate}, nil
	}
	wcMessages, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return FC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals envelope: %w", err)
	}
	vitalsResponse, err := decodeExactLengthDelimitedField(wcMessages, 2)
	if err != nil {
		return FC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals message: %w", err)
	}
	digest := sha256.Sum256(vitalsResponse)
	replay := FC100WCVitalsReplay{
		Kind:           FC100WCVitalsTerminal,
		SnapshotLength: len(vitalsResponse),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}
	if retainBody {
		vitals, present, err := decodeWCVitals(vitalsResponse, firmware)
		if err != nil {
			return FC100WCVitalsReplay{}, err
		}
		replay.terminal = &wcVitalsTerminal{body: vitalsResponse, vitals: vitals, hasVitals: present}
	}
	return replay, nil
}

func decodeWCVitals(body []byte, firmware string) (WCVitals, bool, error) {
	version, ok := teslaReadFirmware(firmware)
	if !ok || (version != [3]uint64{26, 34, 0} && version != [3]uint64{26, 42, 0}) {
		return WCVitals{}, false, nil
	}
	response, err := readWCVitalsFields(body, map[uint64]uint8{1: 2})
	if err != nil || !response[1].Present {
		return WCVitals{}, false, err
	}
	fields, err := readWCVitalsFields(response[1].Value, map[uint64]uint8{
		1: 0, 3: 0, 4: 0, 5: 5, 6: 5, 7: 5, 8: 5, 9: 5, 10: 5, 11: 5, 12: 0,
		13: 2, 14: 2, 15: 2, 16: 5, 17: 5, 18: 5, 19: 0, 21: 2, 22: 2, 23: 0, 24: 2, 25: 0, 27: 0, 28: 0, 29: 0,
	})
	if err != nil {
		return WCVitals{}, false, err
	}
	for _, number := range [...]uint64{1, 12} {
		if wcVitalsVarint(fields[number]) > math.MaxUint32 {
			return WCVitals{}, false, fmt.Errorf("WC vitals field %d is not a uint32", number)
		}
	}
	for _, number := range [...]uint64{19, 23, 25, 27, 28, 29} {
		value := wcVitalsVarint(fields[number])
		if value > math.MaxInt32 && value < math.MaxUint64-math.MaxInt32 {
			return WCVitals{}, false, fmt.Errorf("WC vitals field %d is not an int32", number)
		}
	}
	for _, number := range [...]uint64{3, 4} {
		if wcVitalsVarint(fields[number]) > 1 {
			return WCVitals{}, false, fmt.Errorf("WC vitals field %d is not a bool", number)
		}
	}
	vin, err := decodeWCVitalsMember(fields[21], map[uint64]uint8{1: 2}, func(field WCVitalsField[[]byte]) (WCVitalsField[string], error) {
		if !utf8.Valid(field.Value) {
			return WCVitalsField[string]{}, fmt.Errorf("WC vitals VIN value is not UTF-8")
		}
		return WCVitalsField[string]{Value: string(field.Value), Present: field.Present}, nil
	})
	if err != nil {
		return WCVitals{}, false, err
	}
	power, err := decodeWCVitalsFloatValue(fields[22])
	if err != nil {
		return WCVitals{}, false, err
	}
	current, err := decodeWCVitalsFloatValue(fields[24])
	if err != nil {
		return WCVitals{}, false, err
	}
	vehicleAC, err := decodeWCVitalsMember(fields[15], map[uint64]uint8{1: 5, 2: 5, 3: 2, 4: 2, 5: 2, 6: 2}, func(field WCVitalsField[[]byte]) (WCVitalsVehicleAC, error) {
		return WCVitalsVehicleAC{VoltageVrms: wcVitalsFloat(field)}, nil
	})
	if err != nil {
		return WCVitals{}, false, err
	}
	vitals := WCVitals{
		UptimeSeconds:    wcVitalsInteger[uint32](fields[1]),
		ContactorClosed:  WCVitalsField[bool]{Value: wcVitalsVarint(fields[3]) != 0, Present: fields[3].Present},
		VehicleConnected: WCVitalsField[bool]{Value: wcVitalsVarint(fields[4]) != 0, Present: fields[4].Present},
		PCBATempC:        wcVitalsFloat(fields[5]), HandleTempC: wcVitalsFloat(fields[6]), MCUTempC: wcVitalsFloat(fields[7]),
		InputThermopileV: wcVitalsFloat(fields[8]), ProxV: wcVitalsFloat(fields[9]),
		PilotHighV: wcVitalsFloat(fields[10]), PilotLowV: wcVitalsFloat(fields[11]),
		SessionDurationSeconds: wcVitalsInteger[uint32](fields[12]), VehicleAC: vehicleAC,
		GroundResistanceOhms: wcVitalsFloat(fields[16]), LineEarthVrms: wcVitalsFloat(fields[17]), NeutralEarthVrms: wcVitalsFloat(fields[18]),
		FaultStatus: wcVitalsInteger[int32](fields[19]), VIN: vin, InstantaneousPowerWatts: power,
		ScheduledChargingStatus: wcVitalsInteger[int32](fields[23]), InstantaneousLineCurrent: current,
		OCPPStatus: wcVitalsInteger[int32](fields[25]), DryContactControlState: wcVitalsInteger[int32](fields[28]), SmartChargingReason: wcVitalsInteger[int32](fields[29]),
	}
	if version == [3]uint64{26, 34, 0} {
		vitals.PowershareSessionState = wcVitalsInteger[int32](fields[27])
	}
	return vitals, true, nil
}

// readWCVitalsFields validates the wire framing and the listed singular fields.
// Unlisted members are skipped; the caller retains the complete terminal body.
func readWCVitalsFields(body []byte, singular map[uint64]uint8) (map[uint64]WCVitalsField[[]byte], error) {
	fields := make(map[uint64]WCVitalsField[[]byte], len(singular))
	for len(body) != 0 {
		field, size, err := decodeTeslaFC100RequestField(body)
		if err != nil {
			return nil, fmt.Errorf("WC vitals protobuf: %w", err)
		}
		if wire, known := singular[field.number]; known {
			if field.wireType != wire || fields[field.number].Present {
				return nil, fmt.Errorf("WC vitals field %d has the wrong wire type or is duplicated", field.number)
			}
			value := field.value
			if wire != 2 {
				_, keySize, _ := decodeTeslaFC100Varint(body)
				value = body[keySize:size]
			}
			fields[field.number] = WCVitalsField[[]byte]{Value: value, Present: true}
		}
		body = body[size:]
	}
	return fields, nil
}

func decodeWCVitalsMember[T any](field WCVitalsField[[]byte], singular map[uint64]uint8, convert func(WCVitalsField[[]byte]) (T, error)) (WCVitalsField[T], error) {
	if !field.Present {
		return WCVitalsField[T]{}, nil
	}
	members, err := readWCVitalsFields(field.Value, singular)
	if err != nil {
		return WCVitalsField[T]{}, err
	}
	value, err := convert(members[1])
	return WCVitalsField[T]{Value: value, Present: true}, err
}

func decodeWCVitalsFloatValue(field WCVitalsField[[]byte]) (WCVitalsField[WCVitalsField[float32]], error) {
	return decodeWCVitalsMember(field, map[uint64]uint8{1: 5}, func(value WCVitalsField[[]byte]) (WCVitalsField[float32], error) {
		return wcVitalsFloat(value), nil
	})
}

func wcVitalsVarint(field WCVitalsField[[]byte]) uint64 {
	if !field.Present {
		return 0
	}
	value, _, _ := decodeTeslaFC100Varint(field.Value)
	return value
}

func wcVitalsInteger[T uint32 | int32](field WCVitalsField[[]byte]) WCVitalsField[T] {
	return WCVitalsField[T]{Value: T(wcVitalsVarint(field)), Present: field.Present}
}

func wcVitalsFloat(field WCVitalsField[[]byte]) WCVitalsField[float32] {
	if !field.Present {
		return WCVitalsField[float32]{}
	}
	return WCVitalsField[float32]{Value: math.Float32frombits(binary.LittleEndian.Uint32(field.Value)), Present: true}
}

func decodeExactLengthDelimitedField(message []byte, fieldNumber uint64) ([]byte, error) {
	key, width, err := decodeTeslaFC100Varint(message)
	if err != nil || key != fieldNumber<<3|2 {
		return nil, fmt.Errorf("expected one length-delimited field")
	}
	length, lengthWidth, err := decodeTeslaFC100Varint(message[width:])
	if err != nil || length > uint64(len(message)-width-lengthWidth) ||
		int(length) != len(message)-width-lengthWidth {
		return nil, fmt.Errorf("field length is invalid")
	}
	return append([]byte(nil), message[width+lengthWidth:]...), nil
}

func decodeTeslaFC100Varint(input []byte) (uint64, int, error) {
	var value uint64
	for index, octet := range input {
		if index == 10 || index == 9 && octet > 1 {
			return 0, 0, fmt.Errorf("varint overflows")
		}
		value |= uint64(octet&0x7f) << (7 * index)
		if octet&0x80 == 0 {
			return value, index + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("varint is truncated")
}
