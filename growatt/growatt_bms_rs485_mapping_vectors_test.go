package growatt

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestGrowattBMSMappingVectorPositiveNative(t *testing.T) {
	data, err := os.ReadFile("../driverdata/growatt/growatt-bms-rs485-v202-storage-scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Scenarios []struct {
			ID          string `json:"id"`
			Observation struct {
				Revision struct {
					Family             string `json:"family"`
					FileRevision       string `json:"file_revision"`
					HeaderVersion      string `json:"header_version"`
					CumulativeRevision string `json:"cumulative_revision"`
				} `json:"revision"`
				Typed struct {
					OperatingState              string  `json:"operating_state"`
					SOCPercent                  uint8   `json:"soc_percent"`
					PackVoltageVolts            float64 `json:"pack_voltage_volts"`
					PackCurrentAmps             float64 `json:"pack_current_amps"`
					TemperatureCelsius          int16   `json:"temperature_celsius"`
					CumulativeChargeAmpHours    float64 `json:"cumulative_charge_amp_hours"`
					CumulativeDischargeAmpHours float64 `json:"cumulative_discharge_amp_hours"`
				} `json:"typed"`
				Slices []struct {
					Function string   `json:"function"`
					Offset   string   `json:"offset"`
					UnitID   byte     `json:"unit_id"`
					Words    []uint16 `json:"words"`
				} `json:"slices"`
			} `json:"observation"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Scenarios) != 16 {
		t.Fatalf("scenarios=%d", len(document.Scenarios))
	}
	index := -1
	for candidate := range document.Scenarios {
		if document.Scenarios[candidate].ID == "positive-native-to-semreg" {
			index = candidate
		}
	}
	if index < 0 {
		t.Fatal("positive scenario is absent")
	}
	scenario := document.Scenarios[index]
	input := BMSReadOnlyInput{
		Function: modbusreg.FunctionReadHoldingRegisters,
		Revision: BMSRevisionTuple{
			Family: scenario.Observation.Revision.Family, FileRevision: scenario.Observation.Revision.FileRevision,
			HeaderVersion: scenario.Observation.Revision.HeaderVersion, CumulativeRevision: scenario.Observation.Revision.CumulativeRevision,
		},
	}
	for _, slice := range scenario.Observation.Slices {
		if slice.Function != "FC03" {
			t.Fatalf("function %s", slice.Function)
		}
		parsed, err := strconv.ParseUint(slice.Offset, 0, 16)
		if err != nil {
			t.Fatal(err)
		}
		if input.UnitID == 0 {
			input.UnitID = slice.UnitID
		}
		input.Slices = append(input.Slices, BMSReadOnlySlice{Offset: uint16(parsed), Words: append([]uint16(nil), slice.Words...)})
	}
	status, err := DecodeGrowattBMSTypedReadOnlyStatus(input)
	if err != nil {
		t.Fatal(err)
	}
	typed := scenario.Observation.Typed
	if status.OperatingState != BMSOperatingState(typed.OperatingState) || status.SOCPercent != typed.SOCPercent ||
		status.PackVoltageVolts != typed.PackVoltageVolts || status.PackCurrentAmps != typed.PackCurrentAmps ||
		status.TemperatureCelsius != typed.TemperatureCelsius || status.CumulativeChargeAmpHours != typed.CumulativeChargeAmpHours ||
		status.CumulativeDischargeAmpHours != typed.CumulativeDischargeAmpHours {
		t.Fatalf("status=%#v typed=%#v", status, typed)
	}
}
