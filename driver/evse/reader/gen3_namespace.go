package reader

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

const (
	namespaceNodeKind semreg.DefinitionID = "resource.charging_station"
	namespaceContract driver.SchemaRef    = "modbus.evse.charger/v1"
	namespaceSchema   driver.SchemaRef    = "modbus.evse.charger.payload/v1"
	namespaceMode     driver.AccessMode   = "read"
	namespaceScope    driver.ScopeID      = "diagnostic"
)

// NamespaceReceipt is the receipt of a served payload.
type NamespaceReceipt struct {
	ReceivedAt  time.Time `json:"received_at"`
	ClockEpoch  string    `json:"clock_epoch"`
	MonotonicNS int64     `json:"monotonic_ns"`
	Lifecycle   uint64    `json:"lifecycle"`
	Instance    uint64    `json:"instance"`
	Generation  uint64    `json:"generation"`
	RequestID   uint64    `json:"request_id"`
}

// NamespaceMember is the encoding of one member of the driver namespace. A
// slot with no payload has no payload and no receipt members; it states its
// reason instead.
type NamespaceMember struct {
	Slot      string            `json:"slot"`
	Operation string            `json:"operation"`
	Present   bool              `json:"present"`
	Reason    string            `json:"reason,omitempty"`
	Payload   *[]byte           `json:"payload,omitempty"`
	Receipt   *NamespaceReceipt `json:"receipt,omitempty"`
}

func namespaceMember(p Payload) NamespaceMember {
	member := NamespaceMember{Slot: string(p.Slot), Operation: string(p.Operation), Present: p.Present, Reason: p.Reason}
	if p.Present {
		bytes := p.Bytes
		member.Payload = &bytes
		member.Receipt = &NamespaceReceipt{ReceivedAt: p.Receipt.Wall.UTC(), ClockEpoch: p.Receipt.ClockEpoch, MonotonicNS: p.Receipt.Mono.Nanoseconds(),
			Lifecycle: p.Receipt.Lifecycle, Instance: p.Receipt.Instance, Generation: p.Receipt.Generation, RequestID: p.Receipt.RequestID}
	}
	return member
}

// Register declares this charger's node and one read member per slot in the
// driver namespace. A read answers from the retained payload and sends no
// request, so it works while the transport is closed.
func (r *ChargerReader) Register(registry driver.NativeRegistry) error {
	if registry == nil {
		return errors.New("charger reader requires a native registry")
	}
	node, err := registry.Node(driver.NodeSpec{Kind: namespaceNodeKind, Local: driver.NodeID(r.cfg.Endpoint), Contract: namespaceContract})
	if err != nil {
		return err
	}
	for _, spec := range slotSpecs {
		spec := spec
		member := driver.MemberSpec{Local: driver.MemberID(spec.slot), Schema: namespaceSchema, Mode: namespaceMode, Scope: namespaceScope}
		err := registry.ReadMember(node, member, func(context.Context) ([]byte, error) {
			return json.Marshal(namespaceMember(r.payloads.get(spec)))
		})
		if err != nil {
			return err
		}
	}
	return nil
}
