package write

import (
	"context"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

var _ driver.Executor = (*Writer)(nil)

// Execute answers one dispatch of the operation. The amperes argument is one
// native set or pause; the boolean false is the typed release. The boolean
// true, a request that carries anything else or more than one argument, and a
// route or connector with no registered owner send nothing and are answered
// with their reason.
func (w *Writer) Execute(ctx context.Context, d driver.Dispatch) (driver.Completion, error) {
	amps, release, reason := w.decode(d)
	if reason != "" {
		return w.completion(d, refused(reason)), nil
	}
	if release {
		return w.completion(d, w.release(ctx, d.Guard)), nil
	}
	return w.completion(d, w.ApplyAllocatedCurrent(ctx, d.Guard, amps)), nil
}

func (w *Writer) decode(d driver.Dispatch) (amps semreg.Quantity, release bool, reason string) {
	capability := w.cfg.Publication.CapabilityInstanceIdentifier(w.capability())
	switch {
	case d.Request.Operation != definition(OperationID):
		return amps, false, ReasonForeignRoute
	case !scopedAs(capability, d.Request.CapabilityInstanceID) || !scopedAs(capability, d.Route.CapabilityInstance):
		return amps, false, ReasonForeignRoute
	case !w.connectorMatches(d.ExpectedEffect.Fact):
		return amps, false, ReasonForeignRoute
	case len(d.Request.Arguments) != 1 || d.Request.Arguments[0].ID != ArgumentID:
		return amps, false, ReasonArguments
	}
	value := d.Request.Arguments[0].Value
	switch {
	case value.Kind == semreg.ValueQuantity && value.Quantity != nil:
		return *value.Quantity, false, ""
	case value.Kind == semreg.ValueBoolean && value.Boolean != nil && !*value.Boolean:
		return amps, true, ""
	case value.Kind == semreg.ValueBoolean && value.Boolean != nil:
		return amps, false, ReasonReleaseTrue
	}
	return amps, false, ReasonArguments
}

// connectorMatches accepts an expected effect with no fact key, as a release
// has, and one whose connector dimension is this writer's connector.
func (w *Writer) connectorMatches(key semreg.FactKey) bool {
	if key.FactID == "" {
		return true
	}
	for _, dimension := range key.Dimensions {
		if dimension.ID == DimensionID {
			return dimension.Value.Kind == semreg.ValueText && dimension.Value.Text != nil && *dimension.Value.Text == w.cfg.Publication.Connector
		}
	}
	return false
}
