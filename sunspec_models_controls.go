package modbusreg

func immediateControlsSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("L", "", SunSpecTypeUint16, 1, "", "", true),
		extendedSunSpecPoint("Conn_WinTms", "der.control.connection.window_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("Conn_RvrtTms", "der.control.connection.revert_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("Conn", "der.control.connection.command", true, map[uint64]string{0: "DISCONNECT", 1: "CONNECT"}),
		extendedSunSpecPoint("WMaxLimPct", "der.control.active_power_limit.percent", SunSpecTypeUint16, 1, "% WMax", "WMaxLimPct_SF", true),
		extendedSunSpecPoint("WMaxLimPct_WinTms", "der.control.active_power_limit.window_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("WMaxLimPct_RvrtTms", "der.control.active_power_limit.revert_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("WMaxLimPct_RmpTms", "der.control.active_power_limit.ramp_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("WMaxLim_Ena", "der.control.active_power_limit.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("OutPFSet", "der.control.power_factor.setpoint", SunSpecTypeInt16, 1, "cos()", "OutPFSet_SF", true),
		extendedSunSpecPoint("OutPFSet_WinTms", "der.control.power_factor.window_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("OutPFSet_RvrtTms", "der.control.power_factor.revert_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("OutPFSet_RmpTms", "der.control.power_factor.ramp_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("OutPFSet_Ena", "der.control.power_factor.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("VArWMaxPct", "der.control.reactive_power.percent_wmax", SunSpecTypeInt16, 1, "% WMax", "VArPct_SF", false),
		extendedSunSpecPoint("VArMaxPct", "der.control.reactive_power.percent_var_max", SunSpecTypeInt16, 1, "% VArMax", "VArPct_SF", false),
		extendedSunSpecPoint("VArAvalPct", "der.control.reactive_power.percent_var_available", SunSpecTypeInt16, 1, "% VArAval", "VArPct_SF", false),
		extendedSunSpecPoint("VArPct_WinTms", "der.control.reactive_power.window_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("VArPct_RvrtTms", "der.control.reactive_power.revert_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("VArPct_RmpTms", "der.control.reactive_power.ramp_seconds", SunSpecTypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("VArPct_Mod", "der.control.reactive_power.mode", false, map[uint64]string{0: "NONE", 1: "WMax", 2: "VArMax", 3: "VArAval"}),
		extendedSunSpecEnum("VArPct_Ena", "der.control.reactive_power.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("WMaxLimPct_SF", "der.control.scale.active_power_limit", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("OutPFSet_SF", "der.control.scale.power_factor", SunSpecTypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("VArPct_SF", "der.control.scale.reactive_power", SunSpecTypeScaleFactor, 1, "", "", false),
	}
}
