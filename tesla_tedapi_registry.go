package modbusreg

import (
	"fmt"
	"sort"
	"sync"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

const maxTeslaTEDAPIObservations = 64

// TeslaTEDAPIDirection identifies the wire direction needed to select the
// appropriate opaque envelope contract.
type TeslaTEDAPIDirection string

const (
	TeslaTEDAPIRequest  TeslaTEDAPIDirection = "request"
	TeslaTEDAPIResponse TeslaTEDAPIDirection = "response"
)

// TeslaTEDAPISemanticState describes the deliberately narrow projection level.
type TeslaTEDAPISemanticState string

const (
	TeslaTEDAPIFramingOnly     TeslaTEDAPISemanticState = "framing_only"
	TeslaTEDAPIOpaqueQualified TeslaTEDAPISemanticState = "opaque_qualified"
)

// TeslaTEDAPIObservationSpec is one bounded TEDAPI envelope observation.
type TeslaTEDAPIObservationSpec struct {
	ID        string
	Profile   TeslaHSCProfile
	Direction TeslaTEDAPIDirection
	Function  modbus.PrivateFunctionCode
	Payload   []byte
}

// TeslaTEDAPIRedactedRecord is safe for gateway/MCP projection. Payload never
// contains raw bytes; digest and length preserve deterministic replay identity.
type TeslaTEDAPIRedactedRecord struct {
	ID                 string                    `json:"id"`
	State              TeslaTEDAPISemanticState  `json:"state"`
	OperationAdmission TeslaTEDAPIAdmissionState `json:"operation_admission"`
	Direction          TeslaTEDAPIDirection      `json:"direction"`
	Function           byte                      `json:"function"`
	PayloadLength      int                       `json:"payload_length"`
	PayloadDigest      string                    `json:"payload_digest"`
	OutboundAllowed    bool                      `json:"outbound_allowed"`
}

// TeslaTEDAPISemanticRegistry is a bounded, concurrent registry of opaque
// observations. It does not decode fields or declare a control operation.
type TeslaTEDAPISemanticRegistry struct {
	mu      sync.RWMutex
	records map[string]TeslaTEDAPIRedactedRecord
}

// NewTeslaTEDAPISemanticRegistry constructs an empty bounded registry.
func NewTeslaTEDAPISemanticRegistry() *TeslaTEDAPISemanticRegistry {
	return &TeslaTEDAPISemanticRegistry{records: make(map[string]TeslaTEDAPIRedactedRecord)}
}

// Retain validates the direction-selected contract and atomically retains a
// redacted projection.
func (registry *TeslaTEDAPISemanticRegistry) Retain(spec TeslaTEDAPIObservationSpec) error {
	if registry == nil || !validIdentity(spec.ID) {
		return fmt.Errorf("tesla TEDAPI observation identity is invalid")
	}
	var (
		function modbus.PrivateFunctionCode
		payload  []byte
		err      error
	)
	switch spec.Direction {
	case TeslaTEDAPIRequest:
		var envelope TeslaHSCEnvelope
		envelope, err = DecodeTeslaHSCRequestEnvelope(spec.Function, spec.Payload)
		function, payload = envelope.Function(), envelope.Payload()
	case TeslaTEDAPIResponse:
		var response TeslaHSCResponse
		response, err = DecodeTeslaHSCResponse(spec.Function, spec.Payload)
		function, payload = response.Function(), response.Payload()
	default:
		return fmt.Errorf("tesla TEDAPI observation direction is invalid")
	}
	if err != nil {
		return err
	}
	state := TeslaTEDAPIFramingOnly
	if spec.Profile.Disposition() == TeslaHSCQualifiedReadOnly {
		state = TeslaTEDAPIOpaqueQualified
	}
	provenance := NewTeslaHSCProvenance(
		TeslaHSCCompatibilityV1,
		spec.Profile.Node(),
		function,
		payload,
	)
	admission := spec.Profile.OperationAdmission()
	record := TeslaTEDAPIRedactedRecord{
		ID: spec.ID, State: state, OperationAdmission: admission.State, Direction: spec.Direction, Function: byte(function),
		PayloadLength: provenance.PayloadLength(), PayloadDigest: provenance.PayloadDigest(),
		OutboundAllowed: false,
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.records[spec.ID]; !exists && len(registry.records) >= maxTeslaTEDAPIObservations {
		return fmt.Errorf("tesla TEDAPI observation registry is full")
	}
	registry.records[spec.ID] = record
	return nil
}

// Lookup returns an independent redacted record.
func (registry *TeslaTEDAPISemanticRegistry) Lookup(id string) (TeslaTEDAPIRedactedRecord, bool) {
	if registry == nil {
		return TeslaTEDAPIRedactedRecord{}, false
	}
	registry.mu.RLock()
	record, ok := registry.records[id]
	registry.mu.RUnlock()
	return record, ok
}

// List returns redacted records in deterministic identity order.
func (registry *TeslaTEDAPISemanticRegistry) List() []TeslaTEDAPIRedactedRecord {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	values := make([]TeslaTEDAPIRedactedRecord, 0, len(registry.records))
	for _, record := range registry.records {
		values = append(values, record)
	}
	registry.mu.RUnlock()
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}
