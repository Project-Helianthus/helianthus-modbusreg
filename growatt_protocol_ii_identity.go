package modbusreg

import "fmt"

const growattProtocolIIIdentitySchema = "Protocol II v1.24 TL3-X"

// GrowattProtocolIIIdentityProfile is supplied by the caller's bounded
// fixture. The decoder never discovers a family or interprets another
// Growatt profile from these words.
type GrowattProtocolIIIdentityProfile struct {
	Schema          string
	Family          string
	DeviceType      uint16
	ModelBuild      [2]uint16
	ProtocolVersion uint16
}

// GrowattProtocolIIIdentitySlice is one exact FC03 PDU slice. It has no
// endpoint, session, request, or write authority.
type GrowattProtocolIIIdentitySlice struct {
	Offset uint16
	Words  []uint16
}

// GrowattProtocolIIIdentityInput is a caller-supplied, offline identity
// snapshot for the native Growatt Protocol II v1.24 TL3-X contract.
type GrowattProtocolIIIdentityInput struct {
	UnitID   byte
	Function FunctionCode
	Profile  GrowattProtocolIIIdentityProfile
	Slices   []GrowattProtocolIIIdentitySlice
}

// GrowattProtocolIIIdentityObservation retains a validated raw identity
// snapshot. It does not create a detector, standard registry match, catalog
// entry, runtime activation, telemetry publication, or write authority.
type GrowattProtocolIIIdentityObservation struct {
	unitID          byte
	profile         GrowattProtocolIIIdentityProfile
	slices          []GrowattProtocolIIIdentitySlice
	firmwareText    string
	deviceType      uint16
	modelBuild      [2]uint16
	protocolVersion uint16
}

func (o GrowattProtocolIIIdentityObservation) UnitID() byte { return o.unitID }
func (o GrowattProtocolIIIdentityObservation) Profile() GrowattProtocolIIIdentityProfile {
	return o.profile
}
func (o GrowattProtocolIIIdentityObservation) Slices() []GrowattProtocolIIIdentitySlice {
	return cloneGrowattProtocolIIIdentitySlices(o.slices)
}
func (o GrowattProtocolIIIdentityObservation) FirmwareText() string    { return o.firmwareText }
func (o GrowattProtocolIIIdentityObservation) DeviceType() uint16      { return o.deviceType }
func (o GrowattProtocolIIIdentityObservation) ModelBuild() [2]uint16   { return o.modelBuild }
func (o GrowattProtocolIIIdentityObservation) ProtocolVersion() uint16 { return o.protocolVersion }

// OutboundAllowed is permanently false: identity evidence cannot authorize a
// control operation or another Modbus request.
func (GrowattProtocolIIIdentityObservation) OutboundAllowed() bool { return false }

// DecodeGrowattProtocolIIIdentity validates the four individually bounded
// FC03 identity slices allowed by the Protocol II v1.24 TL3-X contract.
func DecodeGrowattProtocolIIIdentity(input GrowattProtocolIIIdentityInput) (GrowattProtocolIIIdentityObservation, error) {
	if input.UnitID == 0 || input.UnitID == 255 || input.Function != FunctionReadHoldingRegisters ||
		!validGrowattProtocolIIIdentityProfile(input.Profile) {
		return GrowattProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity is invalid")
	}

	want := [...]struct{ offset, words uint16 }{{9, 6}, {43, 1}, {82, 2}, {88, 1}}
	if len(input.Slices) != len(want) {
		return GrowattProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity slice count is invalid")
	}
	for index, expected := range want {
		slice := input.Slices[index]
		if slice.Offset != expected.offset || len(slice.Words) != int(expected.words) {
			return GrowattProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity slice %d is invalid", index)
		}
	}
	if input.Slices[1].Words[0] != input.Profile.DeviceType ||
		[2]uint16{input.Slices[2].Words[0], input.Slices[2].Words[1]} != input.Profile.ModelBuild ||
		input.Slices[3].Words[0] != input.Profile.ProtocolVersion {
		return GrowattProtocolIIIdentityObservation{}, fmt.Errorf("growatt Protocol II identity tuple disagrees with caller profile")
	}
	firmware, err := growattProtocolIIASCII(input.Slices[0].Words)
	if err != nil {
		return GrowattProtocolIIIdentityObservation{}, err
	}

	return GrowattProtocolIIIdentityObservation{
		unitID:          input.UnitID,
		profile:         input.Profile,
		slices:          cloneGrowattProtocolIIIdentitySlices(input.Slices),
		firmwareText:    firmware,
		deviceType:      input.Slices[1].Words[0],
		modelBuild:      [2]uint16{input.Slices[2].Words[0], input.Slices[2].Words[1]},
		protocolVersion: input.Slices[3].Words[0],
	}, nil
}

func validGrowattProtocolIIIdentityProfile(profile GrowattProtocolIIIdentityProfile) bool {
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

func cloneGrowattProtocolIIIdentitySlices(slices []GrowattProtocolIIIdentitySlice) []GrowattProtocolIIIdentitySlice {
	cloned := make([]GrowattProtocolIIIdentitySlice, len(slices))
	for index, slice := range slices {
		cloned[index] = GrowattProtocolIIIdentitySlice{Offset: slice.Offset, Words: append([]uint16(nil), slice.Words...)}
	}
	return cloned
}
