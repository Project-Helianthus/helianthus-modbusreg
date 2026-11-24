package growatt

import (
	"errors"
	"fmt"
)

const protocolIIInputWidth = 250

// ErrGrowattNoAdmissibleProfile is the named no-profile result.
var ErrGrowattNoAdmissibleProfile = errors.New("NO_ADMISSIBLE_PROFILE")

// ProtocolIIFC04Slice retains the FC04 input image. The live record is words
// 0 through 249. Words without one applicable row stay in this slice.
type ProtocolIIFC04Slice struct {
	Offset uint16
	Words  []uint16
}

// ProtocolIIAdmissionInput is the one public admission input.
// An empty ConfiguredFamily asks for a documented automatic identity match.
// ConfiguredFamily MAX, MID or MAC is an explicit TL3-X endpoint selection.
// Identity is optional FC03 evidence from one unit. ContraryEvidence is
// source-backed evidence that the endpoint is outside the selected family.
type ProtocolIIAdmissionInput struct {
	ConfiguredFamily string
	ConfiguredUnit   byte
	Identity         ProtocolIIIdentityObservation
	ContraryEvidence bool
}

// ProtocolIIFC04Applicability admits one FC04 decode. The zero value admits
// nothing. NewProtocolIIFC04Applicability does not set the synthetic fixture flag.
type ProtocolIIFC04Applicability struct {
	profile           ProtocolIIIdentityProfile
	syntheticFixture  bool
	configured        bool
	executionUnit     byte
	identityEvidence  bool
	deviceType        uint16
	modelBuild        [2]uint16
	protocolVersion   uint16
	haveIdentityWords bool
}

// ProtocolIICatalogEntry is the data a catalog lookup passes to the provider.
type ProtocolIICatalogEntry struct {
	RuntimeIdentity string
	Schema          string
	Families        []string
	Function        uint16
	Offset          uint16
	WidthWords      uint16
}

// ProtocolIIProvider is the reusable Protocol II FC04 profile.
type ProtocolIIProvider struct {
	entry ProtocolIICatalogEntry
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

// ProtocolIIPVInput is one documented PV input, n = 1 through 8, at index n-1.
type ProtocolIIPVInput struct {
	VoltageVolts float64
	CurrentAmps  float64
	PowerWatts   float64
	VoltageWord  uint16
	CurrentWord  uint16
	PowerWords   [2]uint16
}

// ProtocolIITableReading is one uniquely applicable scaled row.
type ProtocolIITableReading struct {
	Offset    uint16
	Width     uint16
	Name      string
	Unit      string
	Value     float64
	Qualifier string
}

// ProtocolIITableCode is one uniquely applicable named code.
type ProtocolIITableCode struct {
	Offset uint16
	Name   string
	Code   string
	Raw    uint16
}

// ProtocolIITableFlag is one documented bit.
type ProtocolIITableFlag struct {
	Offset uint16
	Bit    uint8
	Name   string
	Set    bool
}

// ProtocolIITableRawWord is a word kept native. Name is empty when the table
// does not name it, or when more than one row applies.
type ProtocolIITableRawWord struct {
	Offset uint16
	Name   string
	Word   uint16
}

// ProtocolIITableRawBit is an undefined bit kept with its position.
type ProtocolIITableRawBit struct {
	Offset uint16
	Bit    uint8
	Word   uint16
}

// ProtocolIIFC04Telemetry is the read-only FC04 image for one admitted profile.
type ProtocolIIFC04Telemetry struct {
	identity                                                          ProtocolIIIdentityObservation
	raw                                                               ProtocolIIFC04Slice
	InverterState                                                     ProtocolIIInverterState
	PVInputPowerWatts, OutputPowerWatts, GridFrequencyHz              float64
	Phase1VoltageVolts, Phase1CurrentAmps                             float64
	Phase2VoltageVolts, Phase2CurrentAmps                             float64
	Phase3VoltageVolts, Phase3CurrentAmps                             float64
	TodayGeneratedEnergyKWh, TotalGeneratedEnergyKWh, WorkTimeSeconds float64
	PVInputs                                                          [8]ProtocolIIPVInput
	InverterTemperatureCelsius                                        float64
	InverterTemperatureWord                                           uint16
	Readings                                                          []ProtocolIITableReading
	Codes                                                             []ProtocolIITableCode
	Flags                                                             []ProtocolIITableFlag
	NamedRaw                                                          []ProtocolIITableRawWord
	RawBits                                                           []ProtocolIITableRawBit
}

// NewProtocolIIFC04Applicability admits a configured TL3-X family or reports
// why automatic matching has no profile. Edition v1.24 and the example value
// 207 establish no numeric device-type or protocol-version selector.
func NewProtocolIIFC04Applicability(in ProtocolIIAdmissionInput) (ProtocolIIFC04Applicability, error) {
	if in.ConfiguredFamily == "" {
		return ProtocolIIFC04Applicability{}, fmt.Errorf("%w: no documented device-type and protocol-version mapping", ErrGrowattNoAdmissibleProfile)
	}
	switch in.ConfiguredFamily {
	case "MAX", "MID", "MAC":
	default:
		return ProtocolIIFC04Applicability{}, fmt.Errorf("%w: unsupported selected family", ErrGrowattNoAdmissibleProfile)
	}
	if in.ConfiguredUnit == 0 || in.ConfiguredUnit == 255 {
		return ProtocolIIFC04Applicability{}, fmt.Errorf("%w: configured unit is not response-bearing", ErrGrowattNoAdmissibleProfile)
	}
	if in.ContraryEvidence {
		return ProtocolIIFC04Applicability{}, fmt.Errorf("%w: contrary identity evidence", ErrGrowattNoAdmissibleProfile)
	}
	out := ProtocolIIFC04Applicability{
		configured:    true,
		executionUnit: in.ConfiguredUnit,
		profile:       ProtocolIIIdentityProfile{Schema: growattProtocolIIIdentitySchema, Family: in.ConfiguredFamily},
	}
	if in.Identity.UnitID() != in.ConfiguredUnit {
		return out, nil
	}
	family := in.Identity.Profile().Family
	if family != "" && family != in.ConfiguredFamily {
		return ProtocolIIFC04Applicability{}, fmt.Errorf("%w: contrary identity evidence", ErrGrowattNoAdmissibleProfile)
	}
	if family == in.ConfiguredFamily {
		out.identityEvidence = true
		out.haveIdentityWords = true
		out.deviceType = in.Identity.DeviceType()
		out.modelBuild = in.Identity.ModelBuild()
		out.protocolVersion = in.Identity.ProtocolVersion()
		out.profile.DeviceType = out.deviceType
		out.profile.ModelBuild = out.modelBuild
		out.profile.ProtocolVersion = out.protocolVersion
	}
	return out, nil
}

// Configured reports explicit TL3-X endpoint admission.
func (a ProtocolIIFC04Applicability) Configured() bool { return a.configured }

// Family is the admitted TL3-X family, or empty when nothing is admitted.
func (a ProtocolIIFC04Applicability) Family() string { return a.profile.Family }

// ExecutionUnit is the configured endpoint unit. FC04 reads use this unit.
func (a ProtocolIIFC04Applicability) ExecutionUnit() byte { return a.executionUnit }

// IdentityEvidence reports whether FC03 evidence from this unit is associated.
func (a ProtocolIIFC04Applicability) IdentityEvidence() bool { return a.identityEvidence }

// AutomaticQualification is unknown for every configured admission. No
// documented numeric map qualifies the raw device type or protocol version.
func (a ProtocolIIFC04Applicability) AutomaticQualification() string {
	if a.configured {
		return "unknown"
	}
	return ""
}

// ObservedDeviceType returns the raw FC03 device type when evidence from the
// configured unit is associated.
func (a ProtocolIIFC04Applicability) ObservedDeviceType() (uint16, bool) {
	if !a.haveIdentityWords {
		return 0, false
	}
	return a.deviceType, true
}

// ObservedProtocolVersion returns the raw FC03 protocol word when evidence
// from the configured unit is associated.
func (a ProtocolIIFC04Applicability) ObservedProtocolVersion() (uint16, bool) {
	if !a.haveIdentityWords {
		return 0, false
	}
	return a.protocolVersion, true
}

// ObservedModelBuild returns the raw FC03 model-build words, in register
// order, when evidence from the configured unit is associated.
func (a ProtocolIIFC04Applicability) ObservedModelBuild() ([2]uint16, bool) {
	if !a.haveIdentityWords {
		return [2]uint16{}, false
	}
	return a.modelBuild, true
}

func (a ProtocolIIFC04Applicability) matches(identity ProtocolIIIdentityObservation) bool {
	return a.syntheticFixture && a.profile == identity.Profile() &&
		a.profile.DeviceType == identity.DeviceType() && a.profile.ModelBuild == identity.ModelBuild() &&
		a.profile.ProtocolVersion == identity.ProtocolVersion()
}

func (a ProtocolIIFC04Applicability) admits(identity ProtocolIIIdentityObservation) bool {
	if a.syntheticFixture {
		return identity.UnitID() != 0 && validGrowattProtocolIIIdentityProfile(identity.Profile()) && a.matches(identity)
	}
	if !a.configured || !validGrowattProtocolIIIdentityProfile(a.profile) {
		return false
	}
	return a.executionUnit != 0 && a.executionUnit != 255
}

func (a ProtocolIIFC04Applicability) boundIdentity(identity ProtocolIIIdentityObservation) ProtocolIIIdentityObservation {
	if a.syntheticFixture {
		return identity
	}
	if a.identityEvidence && identity.UnitID() == a.executionUnit && identity.Profile().Family == a.profile.Family {
		return identity
	}
	return ProtocolIIIdentityObservation{}
}

// Identity returns the FC03 evidence associated with this telemetry.
// A configured decode without same-unit FC03 evidence returns the zero observation.
func (t ProtocolIIFC04Telemetry) Identity() ProtocolIIIdentityObservation {
	return t.identity
}

// RawSlice returns a copy of the complete raw FC04 slice, including untyped words.
func (t ProtocolIIFC04Telemetry) RawSlice() ProtocolIIFC04Slice {
	return ProtocolIIFC04Slice{Offset: t.raw.Offset, Words: append([]uint16(nil), t.raw.Words...)}
}

// Reading returns one typed row by name and offset.
func (t ProtocolIIFC04Telemetry) Reading(name string, offset uint16) (ProtocolIITableReading, bool) {
	for _, reading := range t.Readings {
		if reading.Name == name && reading.Offset == offset {
			return reading, true
		}
	}
	return ProtocolIITableReading{}, false
}

// RawWord returns a word the decoder did not give a scaled value.
func (t ProtocolIIFC04Telemetry) RawWord(offset uint16) (ProtocolIITableRawWord, bool) {
	for _, word := range t.NamedRaw {
		if word.Offset == offset {
			return word, true
		}
	}
	return ProtocolIITableRawWord{}, false
}

// OutboundAllowed is permanently false: telemetry never authorizes a further request or control.
func (ProtocolIIFC04Telemetry) OutboundAllowed() bool { return false }

// DecodeGrowattProtocolIIFC04Telemetry decodes words 0 through 249 when
// applicability admits the image. A zero applicability value yields no telemetry.
func DecodeGrowattProtocolIIFC04Telemetry(identity ProtocolIIIdentityObservation, applicability ProtocolIIFC04Applicability, raw ProtocolIIFC04Slice) (ProtocolIIFC04Telemetry, error) {
	if !applicability.admits(identity) || raw.Offset != 0 || len(raw.Words) != protocolIIInputWidth {
		return ProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 telemetry is invalid")
	}
	state, ok := growattProtocolIIState(raw.Words[0])
	if !ok {
		return ProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 inverter state is invalid")
	}
	words := append([]uint16(nil), raw.Words...)
	u32 := func(high, low uint16) uint32 { return uint32(high)<<16 | uint32(low) }
	telemetry := ProtocolIIFC04Telemetry{
		identity: applicability.boundIdentity(identity), raw: ProtocolIIFC04Slice{Offset: 0, Words: append([]uint16(nil), words...)}, InverterState: state,
		PVInputPowerWatts: float64(u32(words[1], words[2])) / 10, OutputPowerWatts: float64(u32(words[35], words[36])) / 10, GridFrequencyHz: float64(words[37]) / 100,
		Phase1VoltageVolts: float64(words[38]) / 10, Phase1CurrentAmps: float64(words[39]) / 10, Phase2VoltageVolts: float64(words[42]) / 10, Phase2CurrentAmps: float64(words[43]) / 10, Phase3VoltageVolts: float64(words[46]) / 10, Phase3CurrentAmps: float64(words[47]) / 10,
		TodayGeneratedEnergyKWh: float64(u32(words[53], words[54])) / 10, TotalGeneratedEnergyKWh: float64(u32(words[55], words[56])) / 10, WorkTimeSeconds: float64(u32(words[57], words[58])) / 2,
		InverterTemperatureWord: words[93], InverterTemperatureCelsius: float64(words[93]) / 10,
	}
	growattProtocolIIFillPV(&telemetry, words)
	telemetry.Readings, telemetry.Flags, telemetry.Codes, telemetry.NamedRaw, telemetry.RawBits = growattProtocolIIProject(applicability.profile.Family, words)
	return telemetry, nil
}

func growattProtocolIIFillPV(telemetry *ProtocolIIFC04Telemetry, words []uint16) {
	for index := 0; index < 8; index++ {
		base := 3 + 4*index
		high, low := words[base+2], words[base+3]
		telemetry.PVInputs[index] = ProtocolIIPVInput{
			VoltageVolts: float64(words[base]) / 10, CurrentAmps: float64(words[base+1]) / 10,
			PowerWatts: float64(uint32(high)<<16|uint32(low)) / 10, VoltageWord: words[base], CurrentWord: words[base+1], PowerWords: [2]uint16{high, low},
		}
	}
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

// LookupProtocolIIProvider constructs the public provider from one documented catalog entry.
func LookupProtocolIIProvider(entries []ProtocolIICatalogEntry, runtimeIdentity string) (ProtocolIIProvider, bool) {
	var found *ProtocolIICatalogEntry
	for index := range entries {
		if entries[index].RuntimeIdentity != runtimeIdentity {
			continue
		}
		if found != nil {
			return ProtocolIIProvider{}, false
		}
		copy := entries[index]
		found = &copy
	}
	if found == nil || !protocolIICatalogDocumented(*found) {
		return ProtocolIIProvider{}, false
	}
	return ProtocolIIProvider{entry: *found}, true
}

func protocolIICatalogDocumented(entry ProtocolIICatalogEntry) bool {
	if entry.RuntimeIdentity == "" || entry.Schema != growattProtocolIIIdentitySchema || entry.Function != 4 || entry.Offset != 0 || entry.WidthWords != protocolIIInputWidth {
		return false
	}
	return len(entry.Families) == 3 && entry.Families[0] == "MAX" && entry.Families[1] == "MID" && entry.Families[2] == "MAC"
}

// RuntimeIdentity is the catalog identity this provider was constructed from.
func (p ProtocolIIProvider) RuntimeIdentity() string { return p.entry.RuntimeIdentity }

// Admit resolves applicability for this documented profile.
func (p ProtocolIIProvider) Admit(in ProtocolIIAdmissionInput) (ProtocolIIFC04Applicability, error) {
	if p.entry.RuntimeIdentity == "" {
		return ProtocolIIFC04Applicability{}, fmt.Errorf("growatt Protocol II FC04 provider is absent")
	}
	return NewProtocolIIFC04Applicability(in)
}

// Decode decodes one admitted 250-word image through this provider.
func (p ProtocolIIProvider) Decode(identity ProtocolIIIdentityObservation, applicability ProtocolIIFC04Applicability, raw ProtocolIIFC04Slice) (ProtocolIIFC04Telemetry, error) {
	if p.entry.RuntimeIdentity == "" {
		return ProtocolIIFC04Telemetry{}, fmt.Errorf("growatt Protocol II FC04 provider is absent")
	}
	return DecodeGrowattProtocolIIFC04Telemetry(identity, applicability, raw)
}
