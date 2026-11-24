package sunspec

import "strings"

func projectDERState(model DecodedModel) (capabilityProjection, bool) {
	return projectCapabilityPoints(model, "", "", func(point Fact) (string, string, MeterSignConvention) {
		if !strings.HasPrefix(point.FieldID, "der.") {
			return "", "", ""
		}
		unit := point.Unit
		if point.FieldID == "der.status.available_active_power" {
			unit = "W"
		}
		return point.FieldID, unit, ""
	})
}
