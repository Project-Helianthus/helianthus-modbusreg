package storage

import (
	"context"
	"errors"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/growatt"
)

// ErrNativeReaderInvalid reports a reader without a session, receipt source or
// valid unit.
var ErrNativeReaderInvalid = errors.New("storage: the RS-485 reader is invalid")

// GrowattReader is the native acquisition boundary of one running RS-485
// instance: the session of its transport, the unit and revision it reads, and
// the receipt of a completed base observation. It owns no pacing and no
// session; the transport does.
type GrowattReader struct {
	Session  growatt.BMSRS485RTUObserverSession
	Unit     byte
	Revision growatt.BMSRevisionTuple
	Receipt  func() Receipt
}

// diagnosticWindows are the three optional windows read after the four base
// windows, in this order.
var diagnosticWindows = [...]struct{ offset, quantity uint16 }{{0x0070, 1}, {0x0071, 16}, {0x0081, 16}}

// NativeDiagnostic is the separate native view of the diagnostic: the raw
// group word, and each window with its offset, length and every word. The
// concatenation of the words is not a substitute for the windows.
type NativeDiagnostic struct {
	ReportedGroupWord uint16
	Windows           []growatt.BMSReadOnlySlice
	Typed             growatt.BMSMultiGroupDiagnostic
}

// NativeStatus is what one acquisition returns: the typed base members, the
// four native slices it came from, its receipt, and the diagnostic when its
// three windows were answered. The diagnostic is nil when any window failed.
type NativeStatus struct {
	Status     growatt.BMSTypedReadOnlyStatus
	Slices     []growatt.BMSReadOnlySlice
	Receipt    Receipt
	Diagnostic *NativeDiagnostic
}

func (r GrowattReader) valid() bool {
	return r.Session != nil && r.Receipt != nil && r.Unit != 0 && r.Unit <= 247
}

// Acquire reads the four base windows and then the three diagnostic windows.
// A failed base read is an error and returns no status. A failed diagnostic
// window leaves the base result and no diagnostic.
func (r GrowattReader) Acquire(ctx context.Context) (NativeStatus, error) {
	if !r.valid() || ctx == nil {
		return NativeStatus{}, ErrNativeReaderInvalid
	}
	observer, err := growatt.NewBMSRS485RTUObserver(r.Revision, r.Unit, r.Session)
	if err != nil {
		return NativeStatus{}, err
	}
	status, err := observer.Observe(ctx)
	if err != nil {
		return NativeStatus{}, err
	}
	out := NativeStatus{Status: status, Slices: status.NativeObservation().Slices(), Receipt: r.Receipt()}
	windows := make([]growatt.BMSReadOnlySlice, 0, len(diagnosticWindows))
	for _, w := range diagnosticWindows {
		request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, w.offset, w.quantity)
		if err != nil {
			return out, nil
		}
		response, err := r.Session.ReadHolding(ctx, r.Unit, request)
		if err != nil || response.Provenance != (modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: w.offset, Quantity: w.quantity}) ||
			len(response.Words) != int(w.quantity) {
			return out, nil
		}
		windows = append(windows, growatt.BMSReadOnlySlice{Offset: w.offset, Words: append([]uint16(nil), response.Words...)})
	}
	typed, err := growatt.DecodeGrowattBMSMultiGroupDiagnostic(status, growatt.BMSMultiGroupDiagnosticInput{
		UnitID: r.Unit, Function: modbusreg.FunctionReadHoldingRegisters, Revision: r.Revision, Slices: windows})
	if err != nil {
		return out, nil
	}
	out.Diagnostic = &NativeDiagnostic{ReportedGroupWord: windows[0].Words[0], Windows: windows, Typed: typed}
	return out, nil
}

// NativeStatus is the on-demand native read. It performs the contract reads
// when called, serves the typed members with the cell voltages and balance
// states, and publishes nothing: only ObserveGrowatt commits a cut.
func (p *Publisher) NativeStatus(ctx context.Context, r GrowattReader) (NativeStatus, error) {
	return r.Acquire(ctx)
}

// ObserveGrowatt is the ordinary semantic observation: it acquires and
// publishes the result. The diagnostic is read after every complete base
// observation.
func (p *Publisher) ObserveGrowatt(ctx context.Context, r GrowattReader) error {
	got, err := r.Acquire(ctx)
	if err != nil {
		return err
	}
	obs := GrowattObservation{Status: got.Status, Receipt: got.Receipt}
	if got.Diagnostic != nil {
		typed := got.Diagnostic.Typed
		obs.Diagnostic = &typed
	}
	return p.PublishGrowatt(ctx, obs)
}
