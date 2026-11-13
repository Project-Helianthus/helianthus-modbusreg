package growatt

import "fmt"

// BMSOperatingState is the decoded operating state of the typed BMS
// status. Decoding uses the two low bits of the state word and rejects a word
// whose high byte is nonzero; field meaning is owned by
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/growatt/bms-rs485-1xsxxp-v202.md.
type BMSOperatingState string

// Operating states for the bit values 0 to 3, in that order.
const (
	BMSStateSoftStarting BMSOperatingState = "soft_starting"
	BMSStateStandby      BMSOperatingState = "standby"
	BMSStateCharging     BMSOperatingState = "charging"
	BMSStateDischarging  BMSOperatingState = "discharging"
)

// BMSTypedReadOnlyStatus is the bounded, observation-only projection
// of the exact 1xSxxP ESS V2.02 four-slice input. All unlisted words remain
// available only through the separate opaque read-only observation.
type BMSTypedReadOnlyStatus struct {
	nativeObservation           *BMSReadOnlyObservation
	Revision                    BMSRevisionTuple
	MCUSoftwareVersion          string
	GaugeVersion                string
	BMSCompany                  uint8
	BMSGeneration               uint8
	PackCompany                 uint8
	PackGeneration              uint8
	OperatingState              BMSOperatingState
	SOCPercent                  uint8
	PackVoltageVolts            float64
	PackCurrentAmps             float64
	TemperatureCelsius          int16
	RemainingCapacityAmpHours   float64
	FullChargeCapacityAmpHours  float64
	CycleCount                  uint16
	ContinuousChargeSeconds     uint16
	CurrentCycleChargeAmpHours  float64
	AverageCellVoltageVolts     float64
	FloatingPackVoltageVolts    float64
	CumulativeChargeAmpHours    float64
	CumulativeDischargeAmpHours float64
}

// NativeObservation returns the validated caller-selected unit, revision, and
// exact FC03 slices that produced this typed status. The returned value owns
// independent slice storage.
func (status BMSTypedReadOnlyStatus) NativeObservation() BMSReadOnlyObservation {
	if status.nativeObservation == nil {
		return BMSReadOnlyObservation{}
	}
	return BMSReadOnlyObservation{
		unitID:   status.nativeObservation.unitID,
		revision: status.nativeObservation.revision,
		slices:   cloneGrowattBMSReadOnlySlices(status.nativeObservation.slices),
	}
}

// OutboundAllowed is false for this typed observation. A separate exact
// operation contract owns any BMS request or control decision.
func (BMSTypedReadOnlyStatus) OutboundAllowed() bool { return false }

// DecodeGrowattBMSTypedReadOnlyStatus decodes only the field definitions
// fixed by the exact Growatt BMS V2.02 contract. It has no transport or
// detection behavior and returns no partial status on malformed input.
func DecodeGrowattBMSTypedReadOnlyStatus(input BMSReadOnlyInput) (BMSTypedReadOnlyStatus, error) {
	observation, err := DecodeGrowattBMSReadOnlyObservation(input)
	if err != nil {
		return BMSTypedReadOnlyStatus{}, err
	}
	wordsByOffset := make(map[uint16][]uint16, len(observation.Slices()))
	for _, slice := range observation.Slices() {
		wordsByOffset[slice.Offset] = slice.Words
	}
	identity := wordsByOffset[0x0001]
	status := wordsByOffset[0x000d]
	extension := wordsByOffset[0x0100]
	if len(identity) != 7 || len(status) != 29 || len(extension) != 12 {
		return BMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed status slices are invalid")
	}
	stateWord, socWord, temperatureWord := status[6], status[8], status[11]
	if stateWord>>8 != 0 || socWord>>8 != 0 || uint8(socWord) > 100 {
		return BMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed status byte encoding is invalid")
	}
	temperature := int16(temperatureWord)
	if temperature < -127 || temperature > 127 {
		return BMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed temperature is invalid")
	}
	operatingState, ok := growattBMSOperatingState(uint8(stateWord))
	if !ok {
		return BMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed operating state is invalid")
	}
	return BMSTypedReadOnlyStatus{
		nativeObservation:           &observation,
		Revision:                    observation.Revision(),
		MCUSoftwareVersion:          growattBMSByteVersion(identity[0]),
		GaugeVersion:                growattBMSByteVersion(identity[1]),
		BMSCompany:                  uint8(status[0]),
		BMSGeneration:               uint8(status[0] >> 8),
		PackCompany:                 uint8(status[1]),
		PackGeneration:              uint8(status[1] >> 8),
		OperatingState:              operatingState,
		SOCPercent:                  uint8(socWord),
		PackVoltageVolts:            float64(status[9]) / 100,
		PackCurrentAmps:             float64(int16(status[10])) / 100,
		TemperatureCelsius:          temperature,
		RemainingCapacityAmpHours:   float64(status[13]) / 100,
		FullChargeCapacityAmpHours:  float64(status[14]) / 100,
		CycleCount:                  status[17],
		ContinuousChargeSeconds:     extension[0],
		CurrentCycleChargeAmpHours:  float64(extension[1]) / 10,
		AverageCellVoltageVolts:     float64(extension[2]) / 1000,
		FloatingPackVoltageVolts:    float64(extension[4]) / 10,
		CumulativeChargeAmpHours:    float64(extension[5]) / 10,
		CumulativeDischargeAmpHours: float64(extension[6]) / 10,
	}, nil
}

func growattBMSByteVersion(word uint16) string {
	return fmt.Sprintf("%d.%d", word>>8, uint8(word))
}

func growattBMSOperatingState(bits uint8) (BMSOperatingState, bool) {
	switch bits & 0x03 {
	case 0:
		return BMSStateSoftStarting, true
	case 1:
		return BMSStateStandby, true
	case 2:
		return BMSStateCharging, true
	case 3:
		return BMSStateDischarging, true
	default:
		return "", false
	}
}
