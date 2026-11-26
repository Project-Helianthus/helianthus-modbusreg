package reader

import (
	"sync"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// Slot names one retained payload of a charger.
type Slot string

// The retained payloads. Each persistent-settings and provisional-limit slot
// holds the last payload of that part of the operation; each observation slot
// holds the last payload of that observation. The persistent-settings reply is
// the readback, so that operation has no separate acknowledgement slot.
const (
	SlotPersistentRequest          Slot = "persistent_settings.request"
	SlotPersistentReadback         Slot = "persistent_settings.readback"
	SlotProvisionalRequest         Slot = "provisional_limit.request"
	SlotProvisionalAcknowledgement Slot = "provisional_limit.acknowledgement"
	SlotProvisionalReadback        Slot = "provisional_limit.readback"
	SlotCommonSystemInfo           Slot = "observation.common_system_info"
	SlotVitals                     Slot = "observation.vitals"
	SlotLifetimeStatistics         Slot = "observation.lifetime_statistics"
	SlotSystemInfo                 Slot = "observation.system_info"
	SlotLoadSharingState           Slot = "observation.load_sharing_state"
)

// ReasonNeverReceived is the reason a slot states while no payload for it has
// been received.
const ReasonNeverReceived = "never_received"

type slotSpec struct {
	slot      Slot
	operation tesla.FC100Operation
}

// slotSpecs lists every slot in the order the namespace declares them.
var slotSpecs = []slotSpec{
	{SlotPersistentRequest, tesla.FC100OperationWCConfigureSettings},
	{SlotPersistentReadback, tesla.FC100OperationWCConfigureSettings},
	{SlotProvisionalRequest, tesla.FC100OperationWCSetProvisional},
	{SlotProvisionalAcknowledgement, tesla.FC100OperationWCSetProvisional},
	{SlotProvisionalReadback, tesla.FC100OperationWCGetProvisional},
	{SlotCommonSystemInfo, tesla.FC100OperationCommonSystemInfo},
	{SlotVitals, tesla.FC100OperationWCGetVitals},
	{SlotLifetimeStatistics, tesla.FC100OperationWCGetLifetimeStats},
	{SlotSystemInfo, tesla.FC100OperationWCGetSystemInfo},
	{SlotLoadSharingState, tesla.FC100OperationWCGetLoadSharingState},
}

var observationSlots = map[tesla.FC100Operation]Slot{
	tesla.FC100OperationCommonSystemInfo:      SlotCommonSystemInfo,
	tesla.FC100OperationWCGetVitals:           SlotVitals,
	tesla.FC100OperationWCGetLifetimeStats:    SlotLifetimeStatistics,
	tesla.FC100OperationWCGetSystemInfo:       SlotSystemInfo,
	tesla.FC100OperationWCGetLoadSharingState: SlotLoadSharingState,
}

// Payload is the last payload retained for one slot. Bytes are the exact
// private-function payload of the frame as it was received or sent; nothing in
// them is decoded. A slot that never received one is not Present and states why.
type Payload struct {
	Slot      Slot
	Operation tesla.FC100Operation
	Present   bool
	Reason    string
	Bytes     []byte
	Receipt   Receipt
}

type payloads struct {
	mu   sync.Mutex
	held map[Slot]Payload
}

func newPayloads() *payloads { return &payloads{held: map[Slot]Payload{}} }

// retain keeps a payload unless the slot already holds one whose receipt is
// the same or later, so the last by receipt is kept whatever the arrival order.
func (p *payloads) retain(slot Slot, operation tesla.FC100Operation, frame []byte, receipt Receipt) {
	if frame == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if held, ok := p.held[slot]; ok && !receipt.After(held.Receipt) {
		return
	}
	p.held[slot] = Payload{Slot: slot, Operation: operation, Present: true, Bytes: append([]byte{}, frame...), Receipt: receipt}
}

func (p *payloads) get(spec slotSpec) Payload {
	p.mu.Lock()
	defer p.mu.Unlock()
	if held, ok := p.held[spec.slot]; ok {
		held.Bytes = append([]byte{}, held.Bytes...)
		return held
	}
	return Payload{Slot: spec.slot, Operation: spec.operation, Reason: ReasonNeverReceived}
}

// framePayload is the private-function payload of one frame: the bytes
// between the unit and function bytes and the check bytes.
func framePayload(frame []byte) []byte {
	if len(frame) < 4 {
		return nil
	}
	return append([]byte{}, frame[2:len(frame)-2]...)
}

func terminalFrame(record tesla.Gen3ReadRecord) []byte {
	frames := record.ResponseADUs()
	if len(frames) == 0 {
		return nil
	}
	return framePayload(frames[len(frames)-1])
}

// Payload returns the last payload retained for a slot, or the slot with the
// reason it has none. It sends nothing and works while the transport is closed.
func (r *ChargerReader) Payload(slot Slot) (Payload, bool) {
	for _, spec := range slotSpecs {
		if spec.slot == slot {
			return r.payloads.get(spec), true
		}
	}
	return Payload{}, false
}

// Payloads returns every slot in declaration order.
func (r *ChargerReader) Payloads() []Payload {
	out := make([]Payload, 0, len(slotSpecs))
	for _, spec := range slotSpecs {
		out = append(out, r.payloads.get(spec))
	}
	return out
}

// retainObservation keeps the terminal payload of an acknowledged read of one
// of the five observations of this charger.
func (r *ChargerReader) retainObservation(record tesla.Gen3ReadRecord) {
	slot, ok := observationSlots[record.Operation]
	if !ok || !r.correlated(record, record.Operation) {
		return
	}
	r.payloads.retain(slot, record.Operation, terminalFrame(record), receiptOf(record))
}

// retainReadback keeps the terminal payload of an acknowledged provisional
// readback with the receipt of its own read.
func (r *ChargerReader) retainReadback(record tesla.Gen3ReadRecord) {
	r.payloads.retain(SlotProvisionalReadback, tesla.FC100OperationWCGetProvisional, terminalFrame(record), receiptOf(record))
}

// retainSet keeps the request and the acknowledgement of an acknowledged set.
func (r *ChargerReader) retainSet(record tesla.Gen3ReadRecord) {
	if !r.correlated(record, tesla.FC100OperationWCSetProvisional) {
		return
	}
	receipt := receiptOf(record)
	r.payloads.retain(SlotProvisionalRequest, tesla.FC100OperationWCSetProvisional, framePayload(record.RequestADU()), receipt)
	r.payloads.retain(SlotProvisionalAcknowledgement, tesla.FC100OperationWCSetProvisional, terminalFrame(record), receipt)
}

// retainPersistent keeps the request and the reply of an acknowledged
// persistent-settings exchange.
func (r *ChargerReader) retainPersistent(record tesla.Gen3ReadRecord) {
	receipt := receiptOf(record)
	r.payloads.retain(SlotPersistentRequest, tesla.FC100OperationWCConfigureSettings, framePayload(record.RequestADU()), receipt)
	r.payloads.retain(SlotPersistentReadback, tesla.FC100OperationWCConfigureSettings, terminalFrame(record), receipt)
}
