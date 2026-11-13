package growatt

import "fmt"

import modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"

const growattProtocolIIIdentitySchema = "Protocol II v1.24 TL3-X"

// ProtocolIIIdentityProfile is supplied by the caller's bounded
// fixture. The decoder never discovers a family or interprets another
// Growatt profile from these words.
type ProtocolIIIdentityProfile struct {
	Schema          string
	Family          string
	DeviceType      uint16
	ModelBuild      [2]uint16
	ProtocolVersion uint16
}

// ProtocolIIIdentitySlice is one exact FC03 PDU slice. It has no
// endpoint, session, request, or write authority.
type ProtocolIIIdentitySlice struct {
	Offset uint16
	Words  []uint16
}

// ProtocolIIIdentityInput is a caller-supplied, offline identity
// snapshot for the native Growatt Protocol II v1.24 TL3-X contract.
type ProtocolIIIdentityInput struct {
	UnitID   byte
	Function modbusreg.FunctionCode
	Profile  ProtocolIIIdentityProfile
	Slices   []ProtocolIIIdentitySlice
}

// ProtocolIIIdentityObservation retains a validated raw identity
// snapshot. It does not create a detector, standard registry match, catalog
// entry, runtime activation, telemetry publication, or write authority.
type ProtocolIIIdentityObservation struct {
	unitID          byte
	profile         ProtocolIIIdentityProfile
	slices          []ProtocolIIIdentitySlice
	firmwareText    string
	serialText      string
	deviceType      uint16
	modelBuild      [2]uint16
	protocolVersion uint16
}

// UnitID returns the validated Modbus unit identifier.
func (o ProtocolIIIdentityObservation) UnitID() byte { return o.unitID }

// Profile returns the caller-supplied profile the raw words were checked against.
func (o ProtocolIIIdentityObservation) Profile() ProtocolIIIdentityProfile {
	return o.profile
}

// Slices returns a deep copy of the five raw FC03 slices in contract order.
func (o ProtocolIIIdentityObservation) Slices() []ProtocolIIIdentitySlice {
	return cloneGrowattProtocolIIIdentitySlices(o.slices)
}

// FirmwareText returns the printable ASCII of the six words at offset 9, high
// byte first, with trailing NUL and space bytes removed.
func (o ProtocolIIIdentityObservation) FirmwareText() string { return o.firmwareText }

// SerialText returns the printable ASCII of the five words at offset 23, decoded
// like FirmwareText.
func (o ProtocolIIIdentityObservation) SerialText() string { return o.serialText }

// DeviceType returns the raw word at offset 43.
func (o ProtocolIIIdentityObservation) DeviceType() uint16 { return o.deviceType }

// ModelBuild returns the raw words at offsets 82 and 83, in register order.
func (o ProtocolIIIdentityObservation) ModelBuild() [2]uint16 { return o.modelBuild }

// ProtocolVersion returns the raw word at offset 88.
func (o ProtocolIIIdentityObservation) ProtocolVersion() uint16 { return o.protocolVersion }

// OutboundAllowed is false for this identity observation. It cannot authorize
// a separate Modbus request; an operation-specific contract owns that decision.
func (ProtocolIIIdentityObservation) OutboundAllowed() bool { return false }

// DecodeGrowattProtocolIIIdentity validates the five individually bounded
// FC03 identity slices allowed by the Protocol II v1.24 TL3-X contract.
func DecodeGrowattProtocolIIIdentity(input ProtocolIIIdentityInput) (ProtocolIIIdentityObservation, error) {
	if input.UnitID == 0 || input.UnitID == 255 || input.Function != modbusreg.FunctionReadHoldingRegisters ||
		!validGrowattProtocolIIIdentityProfile(input.Profile) {
		return ProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity is invalid")
	}

	want := [...]struct{ offset, words uint16 }{{9, 6}, {23, 5}, {43, 1}, {82, 2}, {88, 1}}
	if len(input.Slices) != len(want) {
		return ProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity slice count is invalid")
	}
	for index, expected := range want {
		slice := input.Slices[index]
		if slice.Offset != expected.offset || len(slice.Words) != int(expected.words) {
			return ProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity slice %d is invalid", index)
		}
	}
	if input.Slices[2].Words[0] != input.Profile.DeviceType ||
		[2]uint16{input.Slices[3].Words[0], input.Slices[3].Words[1]} != input.Profile.ModelBuild ||
		input.Slices[4].Words[0] != input.Profile.ProtocolVersion {
		return ProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity tuple disagrees with caller profile")
	}
	firmware, err := growattProtocolIIASCII(input.Slices[0].Words)
	if err != nil {
		return ProtocolIIIdentityObservation{}, err
	}
	serial, err := growattProtocolIIASCII(input.Slices[1].Words)
	if err != nil {
		return ProtocolIIIdentityObservation{}, err
	}

	return ProtocolIIIdentityObservation{
		unitID:          input.UnitID,
		profile:         input.Profile,
		slices:          cloneGrowattProtocolIIIdentitySlices(input.Slices),
		firmwareText:    firmware,
		serialText:      serial,
		deviceType:      input.Slices[2].Words[0],
		modelBuild:      [2]uint16{input.Slices[3].Words[0], input.Slices[3].Words[1]},
		protocolVersion: input.Slices[4].Words[0],
	}, nil
}

func validGrowattProtocolIIIdentityProfile(profile ProtocolIIIdentityProfile) bool {
	if profile.Schema != growattProtocolIIIdentitySchema {
		return false
	}
	switch profile.Family {
	case "MAX", "MID", "MAC":
		return true
	default:
		return false
	}
}

func growattProtocolIIASCII(words []uint16) (string, error) {
	bytes := make([]byte, 0, len(words)*2)
	for _, word := range words {
		bytes = append(bytes, byte(word>>8), byte(word))
	}
	end := len(bytes)
	for end > 0 && (bytes[end-1] == 0 || bytes[end-1] == ' ') {
		end--
	}
	if end == 0 {
		return "", fmt.Errorf("growatt Protocol II firmware text is empty")
	}
	for _, value := range bytes[:end] {
		if value < 0x20 || value > 0x7e {
			return "", fmt.Errorf("growatt Protocol II firmware text is malformed")
		}
	}
	return string(bytes[:end]), nil
}

func cloneGrowattProtocolIIIdentitySlices(slices []ProtocolIIIdentitySlice) []ProtocolIIIdentitySlice {
	cloned := make([]ProtocolIIIdentitySlice, len(slices))
	for index, slice := range slices {
		cloned[index] = ProtocolIIIdentitySlice{Offset: slice.Offset, Words: append([]uint16(nil), slice.Words...)}
	}
	return cloned
}
