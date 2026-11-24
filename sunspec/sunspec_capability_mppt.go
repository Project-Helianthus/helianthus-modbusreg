package sunspec

import "strings"

func projectMPPT(model DecodedModel) (capabilityProjection, bool) {
	return projectCapabilityPoints(model, "mppt.module.index", "unknown", func(point Fact) (string, string, MeterSignConvention) {
		if point.FieldID == "mppt.events" || (strings.HasPrefix(point.FieldID, "mppt.module.") && point.Repeated && point.PointName != "Tms") {
			return point.FieldID, point.Unit, ""
		}
		return "", "", ""
	})
}
