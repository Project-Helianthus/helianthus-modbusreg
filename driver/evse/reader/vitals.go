package reader

import (
	"sync"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// ElementConnection is the connector's connection state, fed from the vitals.
const ElementConnection = "evse.status.connection"

// The symbols of the connection state.
const (
	SymbolConnectionConnected    = "evse.status.connection.connected"
	SymbolConnectionDisconnected = "evse.status.connection.disconnected"
)

// VitalsObservation is one acknowledged vitals reply of identified firmware
// with the receipt of the read that returned it.
type VitalsObservation struct {
	Vitals  tesla.WCVitals
	Receipt Receipt
}

// VitalsFeed turns acknowledged vitals replies into the connector's
// connection state. It is safe for concurrent use.
//
// A reply that carries the vitals member states the connection: the vehicle
// flag is true while a vehicle is connected and absent otherwise (TEDAPI
// Specification §6.6.1), so an omitted flag reads as not connected. A reply
// without the vitals member is never fed and leaves the state as it was.
type VitalsFeed struct {
	mu      sync.Mutex
	current *VitalsObservation
}

// NewVitalsFeed returns an empty feed.
func NewVitalsFeed() *VitalsFeed { return &VitalsFeed{} }

// Vitals feeds one reply. A reply later than the one held replaces it, whole;
// a replay, a reply of an older lifecycle, instance or generation, and a late
// reply of the generation held change nothing and report false.
func (f *VitalsFeed) Vitals(o VitalsObservation) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current != nil && !o.Receipt.After(f.current.Receipt) {
		return false
	}
	held := o
	f.current = &held
	return true
}

// Last returns the reply held, exactly as it arrived.
func (f *VitalsFeed) Last() (VitalsObservation, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current == nil {
		return VitalsObservation{}, false
	}
	return *f.current, true
}

// View is the facts of the reply held; with none held it is empty.
func (f *VitalsFeed) View() View {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current == nil {
		return View{}
	}
	symbol := SymbolConnectionDisconnected
	if f.current.Vitals.VehicleConnected.Present && f.current.Vitals.VehicleConnected.Value {
		symbol = SymbolConnectionConnected
	}
	return View{Facts: []Fact{{Element: ElementConnection, Role: RoleConnector, Symbol: symbol, Receipt: f.current.Receipt}}}
}

// NativeVitalsField is a vitals member that stays native: no admitted mapping
// gives it an EVSE fact, so it is served only in the native record.
type NativeVitalsField struct {
	// Field is the member of tesla.WCVitals.
	Field string
	// Unit is the unit the specification states for the member, empty where it
	// states none.
	Unit   string
	Reason string
}

const (
	reasonNoMapping    = "no admitted mapping assigns the member an EVSE fact"
	reasonPower        = "power stays native in the current-limit mapping"
	reasonNoPhase      = "the specification assigns no phase to the member"
	reasonNoSession    = "no charging session is published"
	reasonNoEnumerated = "the enumeration of the member is not specified or its association is provisional"
	reasonNoCharging   = "the specification states the contactor state, not a charging state"
)

// NativeOnlyVitals lists every member of the vitals that is not projected into
// a fact. Together with ElementConnection's source it covers the whole of
// tesla.WCVitals.
func NativeOnlyVitals() []NativeVitalsField {
	return []NativeVitalsField{
		{"UptimeSeconds", "s", reasonNoMapping},
		{"ContactorClosed", "", reasonNoCharging},
		{"PCBATempC", "°C", reasonNoMapping},
		{"HandleTempC", "°C", reasonNoMapping},
		{"MCUTempC", "°C", reasonNoMapping},
		{"InputThermopileV", "V", reasonNoMapping},
		{"ProxV", "V", reasonNoMapping},
		{"PilotHighV", "V", reasonNoMapping},
		{"PilotLowV", "V", reasonNoMapping},
		{"SessionDurationSeconds", "s", reasonNoSession},
		{"VehicleAC", "V (rms)", reasonNoPhase},
		{"GroundResistanceOhms", "Ω", reasonNoMapping},
		{"LineEarthVrms", "V (rms)", reasonNoPhase},
		{"NeutralEarthVrms", "V (rms)", reasonNoPhase},
		{"FaultStatus", "", reasonNoEnumerated},
		{"VIN", "", reasonNoMapping},
		{"InstantaneousPowerWatts", "W", reasonPower},
		{"ScheduledChargingStatus", "", reasonNoEnumerated},
		{"InstantaneousLineCurrent", "A (Provisional)", reasonNoPhase},
		{"OCPPStatus", "", reasonNoMapping},
		{"PowershareSessionState", "", reasonNoEnumerated},
		{"DryContactControlState", "", reasonNoEnumerated},
		{"SmartChargingReason", "", reasonNoEnumerated},
	}
}

// ingestVitals feeds the typed vitals of one acknowledged read of this
// charger. A read of another operation, endpoint or node, an uncorrelated read
// and a read of a build without typed vitals have no effect.
func (r *ChargerReader) ingestVitals(record tesla.Gen3ReadRecord) bool {
	if !r.correlated(record, tesla.FC100OperationWCGetVitals) {
		return false
	}
	vitals, ok := record.Vitals()
	if !ok {
		return false
	}
	return r.vitals.Vitals(VitalsObservation{Vitals: vitals, Receipt: receiptOf(record)})
}

// Vitals is the vitals state the reader publishes from.
func (r *ChargerReader) Vitals() *VitalsFeed { return r.vitals }

func (r *ChargerReader) view(at At) View {
	view := r.feed.View(at)
	extra := r.vitals.View()
	view.Facts = append(view.Facts, extra.Facts...)
	view.Dispositions = append(view.Dispositions, extra.Dispositions...)
	return view
}
