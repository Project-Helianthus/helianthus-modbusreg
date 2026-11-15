package sunspec

func immediateControlsSunSpecPoints() []sunSpecPointDefinition {
	return []sunSpecPointDefinition{
		extendedSunSpecPoint("ID", "", TypeUint16, 1, "", "", true),
		extendedSunSpecPoint("L", "", TypeUint16, 1, "", "", true),
		extendedSunSpecPoint("Conn_WinTms", "der.control.connection.window_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("Conn_RvrtTms", "der.control.connection.revert_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("Conn", "der.control.connection.command", true, map[uint64]string{0: "DISCONNECT", 1: "CONNECT"}),
		extendedSunSpecPoint("WMaxLimPct", "der.control.active_power_limit.percent", TypeUint16, 1, "% WMax", "WMaxLimPct_SF", true),
		extendedSunSpecPoint("WMaxLimPct_WinTms", "der.control.active_power_limit.window_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("WMaxLimPct_RvrtTms", "der.control.active_power_limit.revert_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("WMaxLimPct_RmpTms", "der.control.active_power_limit.ramp_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("WMaxLim_Ena", "der.control.active_power_limit.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("OutPFSet", "der.control.power_factor.setpoint", TypeInt16, 1, "cos()", "OutPFSet_SF", true),
		extendedSunSpecPoint("OutPFSet_WinTms", "der.control.power_factor.window_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("OutPFSet_RvrtTms", "der.control.power_factor.revert_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("OutPFSet_RmpTms", "der.control.power_factor.ramp_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("OutPFSet_Ena", "der.control.power_factor.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("VArWMaxPct", "der.control.reactive_power.percent_wmax", TypeInt16, 1, "% WMax", "VArPct_SF", false),
		extendedSunSpecPoint("VArMaxPct", "der.control.reactive_power.percent_var_max", TypeInt16, 1, "% VArMax", "VArPct_SF", false),
		extendedSunSpecPoint("VArAvalPct", "der.control.reactive_power.percent_var_available", TypeInt16, 1, "% VArAval", "VArPct_SF", false),
		extendedSunSpecPoint("VArPct_WinTms", "der.control.reactive_power.window_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("VArPct_RvrtTms", "der.control.reactive_power.revert_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecPoint("VArPct_RmpTms", "der.control.reactive_power.ramp_seconds", TypeUint16, 1, "Secs", "", false),
		extendedSunSpecEnum("VArPct_Mod", "der.control.reactive_power.mode", false, map[uint64]string{0: "NONE", 1: "WMax", 2: "VArMax", 3: "VArAval"}),
		extendedSunSpecEnum("VArPct_Ena", "der.control.reactive_power.enabled", true, map[uint64]string{0: "DISABLED", 1: "ENABLED"}),
		extendedSunSpecPoint("WMaxLimPct_SF", "der.control.scale.active_power_limit", TypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("OutPFSet_SF", "der.control.scale.power_factor", TypeScaleFactor, 1, "", "", true),
		extendedSunSpecPoint("VArPct_SF", "der.control.scale.reactive_power", TypeScaleFactor, 1, "", "", false),
	}
}
