// Package write is the executor of the allocated current operation of a
// configured Gen3 Wall Connector. It calls the native provisional setter of the
// tesla package once per dispatch, feeds the returned t28 to the reader and
// answers the dispatch with delivery, acknowledgement and readback evidence.
package write

import (
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/roweval"
)

// The operation, its capability, service, argument and effect in the EVSE pack.
const (
	OperationID  semreg.DefinitionID = "evse.operation.set_allocated_current"
	CapabilityID semreg.DefinitionID = "evse.capability.set_allocated_current"
	ServiceID    semreg.DefinitionID = "evse.service.connector"
	ArgumentID   semreg.DefinitionID = "evse.limit.allocated_current"
	EffectID     semreg.DefinitionID = "evse.effect.set_allocated_current"
	DimensionID  semreg.DefinitionID = "evse.dimension.connector"
	UnitAmpere   semreg.DefinitionID = "unit.ampere"
)

// The native rows of the operation: the t25 request with its t26
// acknowledgement, and the t27 request with its t28 readback.
const (
	TargetRow   = "tesla.wc_set_provisional.t25_t26"
	ReadbackRow = "tesla.wc_get_provisional.t27_t28"
	SubjectRole = "connector"
)

// The values of the native request.
const (
	// FloorAmps is the lowest current the control pilot duty cycle signals
	// (IEC 61851-1 Annex A).
	FloorAmps = 6
	// LimitTimeoutSeconds is the lifetime of the t25 request of an initial set,
	// a pause and a release. The field range of the request is 0 to 86399
	// seconds (TEDAPI Specification 6.4.10); this value is the one of the
	// operation, not a bound of the field.
	LimitTimeoutSeconds = 900
	// RenewPeriod is the period at which a limit in force is sent again; the
	// timeout of the request is reloaded by a repeated valid request (TEDAPI
	// Specification 7.1.1, rule P5).
	RenewPeriod = 300 * time.Second
)

var evsePack = semreg.PackRef{ID: "helianthus.pack.evse", Version: "1.0.0"}

func definition(id semreg.DefinitionID) semreg.DefinitionRef {
	return semreg.DefinitionRef{Pack: evsePack, ID: id, Version: "1.0.0"}
}

// OperationRow is the row of the operation: the amperes case and, under the
// same operation, capability and service, the typed release case.
type OperationRow struct {
	Binding roweval.BindingRow
	Release roweval.WriteRow
}

// Row declares the operation. The amperes case writes t25 and reads back t27 →
// t28; the lowest value is the floor of the control pilot and the step is one
// whole ampere; the highest value is the configured current of the latest t28,
// which the row does not state. The release case carries the boolean false
// under the operation's argument. A mutation is not retried.
func Row() OperationRow {
	one, floor := semreg.Decimal{Coefficient: "1"}, semreg.Decimal{Coefficient: "6"}
	set := roweval.WriteRow{
		Operation: definition(OperationID), Argument: ArgumentID, TargetRow: TargetRow, ReadbackRow: ReadbackRow,
		NativeBounds: roweval.Bounds{Min: &floor, Step: &one},
	}
	return OperationRow{
		Binding: roweval.BindingRow{
			NativeID: TargetRow, SubjectRole: SubjectRole, SpecRef: "TEDAPI Specification 6.4.10, 6.4.11, 7.1; IEC 61851-1 Annex A",
			Service: definition(ServiceID), Capability: definition(CapabilityID), Write: &set,
		},
		Release: roweval.WriteRow{Operation: definition(OperationID), Argument: ArgumentID, TargetRow: TargetRow, ReadbackRow: ReadbackRow},
	}
}
