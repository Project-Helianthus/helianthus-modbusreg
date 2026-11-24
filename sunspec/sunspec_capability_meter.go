package sunspec

import "strings"

// MeterSignConvention describes a native meter fact without changing its sign.
// Open means that no directional sign is established for the quantity. None
// applies to quantities such as voltage, frequency and event words.
type MeterSignConvention string

// Meter sign conventions: direction, reactive behavior, or counter class.
const (
	MeterSignNone          MeterSignConvention = "none"
	MeterSignOpen          MeterSignConvention = "open"
	MeterSignTowardLoad    MeterSignConvention = "positive_toward_load"
	MeterSignInductive     MeterSignConvention = "positive_inductive"
	MeterSignLagging       MeterSignConvention = "positive_lagging"
	MeterSignImportCounter MeterSignConvention = "import_counter"
	MeterSignExportCounter MeterSignConvention = "export_counter"
)

// SignConvention returns the native meter sign convention, or empty for a
// non-meter fact. Counter direction is not a signed instantaneous reading.
func (f CapabilityFact) SignConvention() MeterSignConvention { return f.sign }

func projectMeter(model DecodedModel) (capabilityProjection, bool) {
	return projectCapabilityPoints(model, "", "", func(point Fact) (string, string, MeterSignConvention) {
		return point.FieldID, point.Unit, meterSignConvention(point.FieldID)
	})
}

var meterSignConventions = []struct {
	prefix string
	sign   MeterSignConvention
}{
	{"meter.ac.current.", MeterSignTowardLoad},
	{"meter.ac.power.active.", MeterSignTowardLoad},
	{"meter.ac.power.reactive.", MeterSignInductive},
	{"meter.ac.power_factor.", MeterSignLagging},
	{"meter.ac.power.apparent.", MeterSignOpen},
	{"meter.energy.import.", MeterSignImportCounter},
	{"meter.energy.apparent_import.", MeterSignImportCounter},
	{"meter.energy.reactive_import.", MeterSignImportCounter},
	{"meter.energy.export.", MeterSignExportCounter},
	{"meter.energy.apparent_export.", MeterSignExportCounter},
	{"meter.energy.reactive_export.", MeterSignExportCounter},
}

func meterSignConvention(id string) MeterSignConvention {
	for _, row := range meterSignConventions {
		if strings.HasPrefix(id, row.prefix) {
			return row.sign
		}
	}
	return MeterSignNone
}
