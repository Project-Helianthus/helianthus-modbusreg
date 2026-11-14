package tesla

import (
	"fmt"
	"sort"
	"sync"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

const maxTeslaTEDAPIObservations = 64

// TEDAPIDirection identifies the wire direction needed to select the
// appropriate opaque envelope contract.
type TEDAPIDirection string

// Directions: Request selects request-envelope validation, Response selects
// normal-response validation.
const (
	TEDAPIRequest  TEDAPIDirection = "request"
	TEDAPIResponse TEDAPIDirection = "response"
)

// TEDAPISemanticState describes the deliberately narrow projection level.
type TEDAPISemanticState string

// Projection levels. FramingOnly: the profile is not qualified read-only.
// OpaqueQualified: the profile is qualified read-only; the payload stays opaque
// and grants no outbound permission. See
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/tesla/tedapi.md.
const (
	TEDAPIFramingOnly     TEDAPISemanticState = "framing_only"
	TEDAPIOpaqueQualified TEDAPISemanticState = "opaque_qualified"
)

// TEDAPIObservationSpec is one bounded TEDAPI envelope observation.
type TEDAPIObservationSpec struct {
	ID        string
	Profile   HSCProfile
	Direction TEDAPIDirection
	Function  modbus.PrivateFunctionCode
	Payload   []byte
}

// TEDAPINativeRecord retains one bounded native TEDAPI payload.
type TEDAPINativeRecord struct {
	ID                 string               `json:"id"`
	State              TEDAPISemanticState  `json:"state"`
	OperationAdmission TEDAPIAdmissionState `json:"operation_admission"`
	Direction          TEDAPIDirection      `json:"direction"`
	Function           byte                 `json:"function"`
	PayloadLength      int                  `json:"payload_length"`
	OutboundAllowed    bool                 `json:"outbound_allowed"`
	payload            []byte
}

// TEDAPISemanticRegistry is a bounded, concurrent registry of opaque
// observations. It does not decode fields or declare a control operation.
type TEDAPISemanticRegistry struct {
	mu      sync.RWMutex
	records map[string]TEDAPINativeRecord
}

// NewTEDAPISemanticRegistry constructs an empty bounded registry.
func NewTEDAPISemanticRegistry() *TEDAPISemanticRegistry {
	return &TEDAPISemanticRegistry{records: make(map[string]TEDAPINativeRecord)}
}

// Retain validates the direction-selected contract and atomically retains the
// complete bounded native payload.
func (registry *TEDAPISemanticRegistry) Retain(spec TEDAPIObservationSpec) error {
	if registry == nil || !modbusreg.ValidIdentity(spec.ID) {
		return fmt.Errorf("tesla TEDAPI observation identity is invalid")
	}
	var (
		function modbus.PrivateFunctionCode
		payload  []byte
		err      error
	)
	switch spec.Direction {
	case TEDAPIRequest:
		var envelope HSCEnvelope
		envelope, err = DecodeTeslaHSCRequestEnvelope(spec.Function, spec.Payload)
		function, payload = envelope.Function(), envelope.Payload()
	case TEDAPIResponse:
		var response HSCResponse
		response, err = DecodeTeslaHSCResponse(spec.Function, spec.Payload)
		function, payload = response.Function(), response.Payload()
	default:
		return fmt.Errorf("tesla TEDAPI observation direction is invalid")
	}
	if err != nil {
		return err
	}
	state := TEDAPIFramingOnly
	if spec.Profile.Disposition() == HSCQualifiedReadOnly {
		state = TEDAPIOpaqueQualified
	}
	admission := spec.Profile.OperationAdmission()
	record := TEDAPINativeRecord{
		ID: spec.ID, State: state, OperationAdmission: admission.State, Direction: spec.Direction, Function: byte(function),
		PayloadLength: len(payload), OutboundAllowed: false, payload: append([]byte(nil), payload...),
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.records[spec.ID]; !exists && len(registry.records) >= maxTeslaTEDAPIObservations {
		return fmt.Errorf("tesla TEDAPI observation registry is full")
	}
	registry.records[spec.ID] = record
	return nil
}

// Lookup returns an independent native record.
func (registry *TEDAPISemanticRegistry) Lookup(id string) (TEDAPINativeRecord, bool) {
	if registry == nil {
		return TEDAPINativeRecord{}, false
	}
	registry.mu.RLock()
	record, ok := registry.records[id]
	registry.mu.RUnlock()
	return cloneTeslaTEDAPINativeRecord(record), ok
}

// List returns native records in deterministic identity order.
func (registry *TEDAPISemanticRegistry) List() []TEDAPINativeRecord {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	values := make([]TEDAPINativeRecord, 0, len(registry.records))
	for _, record := range registry.records {
		values = append(values, cloneTeslaTEDAPINativeRecord(record))
	}
	registry.mu.RUnlock()
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}

// Payload returns an independent copy of the exact native payload.
func (record TEDAPINativeRecord) Payload() []byte { return append([]byte(nil), record.payload...) }

func cloneTeslaTEDAPINativeRecord(record TEDAPINativeRecord) TEDAPINativeRecord {
	record.payload = append([]byte(nil), record.payload...)
	return record
}
