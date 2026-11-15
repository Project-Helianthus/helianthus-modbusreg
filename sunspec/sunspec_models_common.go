package sunspec

func commonSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		sunSpecPoint("ID", "", TypeUint16, 1, "", "", true, nil, 0),
		sunSpecPoint("L", "", TypeUint16, 1, "", "", true, nil, 0),
		sunSpecPoint("Mn", "device.manufacturer", TypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("Md", "device.model", TypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("Opt", "device.options", TypeString, 8, "", "", false, nil, 0),
		sunSpecPoint("Vr", "device.version", TypeString, 8, "", "", false, nil, 0),
		sunSpecPoint("SN", "device.serial", TypeString, 16, "", "", true, nil, 0),
		sunSpecPoint("DA", "device.address", TypeUint16, 1, "", "", false, nil, 0),
		sunSpecPoint("Pad", "device.pad", TypePad, 1, "", "", false, nil, 0),
	}
}
