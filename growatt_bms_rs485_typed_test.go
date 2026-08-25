package modbusreg

import "testing"

func TestDecodeGrowattBMSTypedReadOnlyStatusProjectsOnlyExactV202Fields(t *testing.T) {
	status, err := DecodeGrowattBMSTypedReadOnlyStatus(validGrowattBMSTypedReadOnlyInput())
	if err != nil {
		t.Fatal(err)
	}
	if status.MCUSoftwareVersion != "1.2" || status.GaugeVersion != "3.4" ||
		status.BMSCompany != 4 || status.BMSGeneration != 2 ||
		status.PackCompany != 1 || status.PackGeneration != 3 ||
		status.OperatingState != GrowattBMSStateCharging || status.SOCPercent != 75 ||
		status.PackVoltageVolts != 52 || status.PackCurrentAmps != -1 ||
		status.TemperatureCelsius != 25 || status.RemainingCapacityAmpHours != 32 ||
		status.FullChargeCapacityAmpHours != 50 || status.CycleCount != 110 ||
		status.ContinuousChargeSeconds != 100 || status.CurrentCycleChargeAmpHours != 12.3 ||
		status.AverageCellVoltageVolts != 3.3 || status.FloatingPackVoltageVolts != 51.2 ||
		status.CumulativeChargeAmpHours != 0.5 || status.CumulativeDischargeAmpHours != 0.6 ||
		status.OutboundAllowed() {
		t.Fatalf("status=%#v", status)
	}
}

func TestDecodeGrowattBMSTypedReadOnlyStatusRejectsInvalidTypedEncoding(t *testing.T) {
	for name, mutate := range map[string]func(*GrowattBMSReadOnlyInput){
		"status high byte": func(input *GrowattBMSReadOnlyInput) { input.Slices[1].Words[6] = 0x0102 },
		"SOC high byte":    func(input *GrowattBMSReadOnlyInput) { input.Slices[1].Words[8] = 0x0101 },
		"SOC range":        func(input *GrowattBMSReadOnlyInput) { input.Slices[1].Words[8] = 101 },
		"temperature":      func(input *GrowattBMSReadOnlyInput) { input.Slices[1].Words[11] = 128 },
	} {
		t.Run(name, func(t *testing.T) {
			input := validGrowattBMSTypedReadOnlyInput()
			mutate(&input)
			if _, err := DecodeGrowattBMSTypedReadOnlyStatus(input); err == nil {
				t.Fatal("accepted invalid typed encoding")
			}
		})
	}
}

func validGrowattBMSTypedReadOnlyInput() GrowattBMSReadOnlyInput {
	input := validGrowattBMSReadOnlyInput()
	input.Slices[0].Words = []uint16{0x0102, 0x0304, 0, 0, 0, 0, 0}
	input.Slices[1].Words = []uint16{
		0x0204, 0x0301, 0, 0,
		0, 0, 0x0002, 0,
		75, 5200, 0xff9c, 25,
		0, 3200, 5000, 0,
		0, 110, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0, 0,
	}
	input.Slices[2].Words = []uint16{100, 123, 3300, 0, 512, 5, 6, 0, 0, 0, 0, 0}
	return input
}
