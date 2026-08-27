package modbusreg

const (
	OutBackAXSModelInterface      uint16 = 64110
	OutBackAXSModelChargeControl  uint16 = 64111
	OutBackAXSInterfaceModelWords uint16 = 282
	OutBackAXSChargeModelWords    uint16 = 23
)

// OutBackAXSReadOnlyDecoder is explicitly supplied by an OutBack caller. It
// exposes observed state only; it has no profile discovery or send surface.
type OutBackAXSReadOnlyDecoder struct{ namespace SunSpecVendorDecoderNamespace }

func NewOutBackAXSReadOnlyDecoder() (OutBackAXSReadOnlyDecoder, error) {
	definitions, err := outBackAXSDefinitions()
	if err != nil {
		return OutBackAXSReadOnlyDecoder{}, err
	}
	namespace, err := newSunSpecVendorDecoderNamespace(SunSpecModelsRevisionV1, definitions)
	if err != nil {
		return OutBackAXSReadOnlyDecoder{}, err
	}
	return OutBackAXSReadOnlyDecoder{namespace: namespace}, nil
}

func (d OutBackAXSReadOnlyDecoder) Decode(words []uint16) (SunSpecDecodedModel, error) {
	return d.namespace.Decode(words)
}

func outBackAXSDefinitions() ([]sunSpecModelDefinition, error) {
	interfacePoints := []sunSpecPointDefinition{
		sunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("FirmwareMajor", "outback.axs.firmware.major", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("FirmwareMid", "outback.axs.firmware.mid", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("FirmwareMinor", "outback.axs.firmware.minor", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("Excluded", "", SunSpecTypePad, 273, "", "", false, nil, 0),
		sunSpecPoint("BatteryTemperature", "outback.axs.temperature.battery", SunSpecTypeInt16, 1, "C", "TemperatureSF", false, nil, 0),
		sunSpecPoint("AmbientTemperature", "outback.axs.temperature.ambient", SunSpecTypeInt16, 1, "C", "TemperatureSF", false, nil, 0),
		sunSpecPoint("TemperatureSF", "outback.axs.temperature.scale", SunSpecTypeScaleFactor, 1, "", "", false, nil, 0),
		sunSpecPoint("Error", "outback.axs.error", SunSpecTypeBitfield16, 1, "", "", false, nil, 0),
		sunSpecPoint("Status", "outback.axs.status", SunSpecTypeBitfield16, 1, "", "", false, nil, 0),
		sunSpecPoint("Spare", "", SunSpecTypePad, 1, "", "", false, nil, 0),
	}
	chargePoints := []sunSpecPointDefinition{
		sunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		// The public contract fixes the block's exact geometry but not a point
		// map. Keep its payload vendor-scoped and raw rather than invent facts.
		sunSpecPoint("VendorPayload", "", SunSpecTypePad, 23, "", "", false, nil, 0),
	}
	definitions, err := appendSunSpecDefinition(nil, SunSpecModelsRevisionV1, OutBackAXSModelInterface, OutBackAXSInterfaceModelWords, SunSpecTopologyNone, false, interfacePoints)
	if err != nil {
		return nil, err
	}
	return appendSunSpecDefinition(definitions, SunSpecModelsRevisionV1, OutBackAXSModelChargeControl, OutBackAXSChargeModelWords, SunSpecTopologyNone, false, chargePoints)
}
