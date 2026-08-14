package modbusreg

func commonSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		sunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true, nil, 0),
		sunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true, nil, 0),
		sunSpecPoint("Mn", "device.manufacturer", SunSpecTypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("Md", "device.model", SunSpecTypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("Opt", "device.options", SunSpecTypeString, 8, "", "", false, nil, 0),
		sunSpecPoint("Vr", "device.version", SunSpecTypeString, 8, "", "", false, nil, 0),
		sunSpecPoint("SN", "device.serial", SunSpecTypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("DA", "device.address", SunSpecTypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("Pad", "device.pad", SunSpecTypePad, 1, "", "", false, nil, 0),
	}
}
