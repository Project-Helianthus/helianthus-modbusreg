package modbusreg

import "fmt"

type GrowattBMSOperatingState string

const (
	GrowattBMSStateSoftStarting GrowattBMSOperatingState = "soft_starting"
	GrowattBMSStateStandby      GrowattBMSOperatingState = "standby"
	GrowattBMSStateCharging     GrowattBMSOperatingState = "charging"
	GrowattBMSStateDischarging  GrowattBMSOperatingState = "discharging"
)

// GrowattBMSTypedReadOnlyStatus is the bounded, observation-only projection
// of the exact 1xSxxP ESS V2.02 four-slice input. All unlisted words remain
// available only through the separate opaque read-only observation.
type GrowattBMSTypedReadOnlyStatus struct {
	nativeObservation           *GrowattBMSReadOnlyObservation
	Revision                    GrowattBMSRevisionTuple
	MCUSoftwareVersion          string
	GaugeVersion                string
	BMSCompany                  uint8
	BMSGeneration               uint8
	PackCompany                 uint8
	PackGeneration              uint8
	OperatingState              GrowattBMSOperatingState
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
func (status GrowattBMSTypedReadOnlyStatus) NativeObservation() GrowattBMSReadOnlyObservation {
	if status.nativeObservation == nil {
		return GrowattBMSReadOnlyObservation{}
	}
	return GrowattBMSReadOnlyObservation{
		unitID:   status.nativeObservation.unitID,
		revision: status.nativeObservation.revision,
		slices:   cloneGrowattBMSReadOnlySlices(status.nativeObservation.slices),
	}
}

// OutboundAllowed is false for this typed observation. A separate exact
// operation contract owns any BMS request or control decision.
func (GrowattBMSTypedReadOnlyStatus) OutboundAllowed() bool { return false }

// DecodeGrowattBMSTypedReadOnlyStatus decodes only the field definitions
// fixed by the exact Growatt BMS V2.02 contract. It has no transport or
// detection behavior and returns no partial status on malformed input.
func DecodeGrowattBMSTypedReadOnlyStatus(input GrowattBMSReadOnlyInput) (GrowattBMSTypedReadOnlyStatus, error) {
	observation, err := DecodeGrowattBMSReadOnlyObservation(input)
	if err != nil {
		return GrowattBMSTypedReadOnlyStatus{}, err
	}
	wordsByOffset := make(map[uint16][]uint16, len(observation.Slices()))
	for _, slice := range observation.Slices() {
		wordsByOffset[slice.Offset] = slice.Words
	}
	identity := wordsByOffset[0x0001]
	status := wordsByOffset[0x000d]
	extension := wordsByOffset[0x0100]
	if len(identity) != 7 || len(status) != 29 || len(extension) != 12 {
		return GrowattBMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed status slices are invalid")
	}
	stateWord, socWord, temperatureWord := status[6], status[8], status[11]
	if stateWord>>8 != 0 || socWord>>8 != 0 || uint8(socWord) > 100 {
		return GrowattBMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed status byte encoding is invalid")
	}
	temperature := int16(temperatureWord)
	if temperature < -127 || temperature > 127 {
		return GrowattBMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed temperature is invalid")
	}
	operatingState, ok := growattBMSOperatingState(uint8(stateWord))
	if !ok {
		return GrowattBMSTypedReadOnlyStatus{}, fmt.Errorf("growatt BMS typed operating state is invalid")
	}
	return GrowattBMSTypedReadOnlyStatus{
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

func growattBMSOperatingState(bits uint8) (GrowattBMSOperatingState, bool) {
	switch bits & 0x03 {
	case 0:
		return GrowattBMSStateSoftStarting, true
	case 1:
		return GrowattBMSStateStandby, true
	case 2:
		return GrowattBMSStateCharging, true
	case 3:
		return GrowattBMSStateDischarging, true
	default:
		return "", false
	}
}
