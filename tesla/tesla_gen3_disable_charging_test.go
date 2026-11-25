package tesla

import "testing"

func TestTeslaNativeCommandDisableCharging(t *testing.T) {
	testTeslaNativeChargeCommand(t, 2, "wc.disable_charging", EncodeTeslaDisableCharging, DecodeTeslaDisableCharging, (*Gen3ReadAcquisition).DisableCharging)
}
