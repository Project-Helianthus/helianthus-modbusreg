package modbusreg

import (
	"bytes"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaHSCFC100OperationCompatibilityV1 identifies the firmware-scoped
// operation inventory. It is intentionally distinct from the generic HSC
// profile compatibility identifier.
const TeslaHSCFC100OperationCompatibilityV1 = "wc3_24_44_3"

// TeslaFC100Operation names one version-scoped FC100 EVSE request/response
// pair. The retained set and public evidence basis are recorded in
// docs/tesla-fc100-evse-scope.md. A catalog entry is not operation admission
// or a qualified live setter; unrelated settings remain outside EVSE scope.
type TeslaFC100Operation string

const (
	TeslaFC100OperationCommonSystemInfo      TeslaFC100Operation = "common.system_info"
	TeslaFC100OperationWCGetVitals           TeslaFC100Operation = "wc.get_vitals"
	TeslaFC100OperationWCGetLifetimeStats    TeslaFC100Operation = "wc.get_lifetime_stats"
	TeslaFC100OperationWCConfigureSettings   TeslaFC100Operation = "wc.configure_settings"
	TeslaFC100OperationWCGetSystemInfo       TeslaFC100Operation = "wc.get_system_info"
	TeslaFC100OperationWCGetLoadSharingState TeslaFC100Operation = "wc.get_load_sharing_state"
	TeslaFC100OperationWCSetProvisional      TeslaFC100Operation = "wc.set_provisional"
	TeslaFC100OperationWCGetProvisional      TeslaFC100Operation = "wc.get_provisional"
)

type teslaFC100OperationSpec struct{ family, requestTag, responseTag uint64 }

// TeslaFC100OperationNames provides recovered 24.44.3 message names and the
// field names established for each terminal body. Unknown wire fields remain
// retained in TeslaFC100OperationResult.Body.
type TeslaFC100OperationNames struct {
	Request  string
	Response string
	Fields   []string
}

var teslaFC100OperationNames = map[TeslaFC100Operation]TeslaFC100OperationNames{
	TeslaFC100OperationCommonSystemInfo:      {"CommonAPIGetSystemInfo", "CommonSystemInfo", []string{"device_id", "din", "firmware_version", "device_type"}},
	TeslaFC100OperationWCGetVitals:           {"GetVitals", "WCVitals", nil},
	TeslaFC100OperationWCGetLifetimeStats:    {"GetLifetimeStats", "WCLifetimeStats", []string{"uptime_s", "alert_count", "contactor_cycles", "contactor_cycles_loaded", "connector_cycles", "thermal_foldbacks", "avg_startup_temp_c", "charge_starts", "charging_time_s", "charging_energy"}},
	TeslaFC100OperationWCGetSystemInfo:       {"GetSystemInfo", "WCGenealogy", []string{"region", "handle_type", "hardware_features"}},
	TeslaFC100OperationWCGetLoadSharingState: {"GetLoadSharingNetworkState", "WCLoadSharingNetworkState", []string{"devices", "leader_state", "settings", "status", "limits"}},
	TeslaFC100OperationWCGetProvisional:      {"GetProvisionalOperationalParams", "WCProvisionalOperationalParams", []string{"limit_current_max_amps", "limit_timeout_s", "inhibit_charging", "configured_current_limit_amps"}},
	TeslaFC100OperationWCConfigureSettings:   {"ConfigureSettings", "ConfigureSettingsResponse", []string{"settings"}},
	TeslaFC100OperationWCSetProvisional:      {"SetProvisionalOperationalParams", "SetProvisionalOperationalParamsResponse", []string{"prov_op_params"}},
}

// TeslaFC100RecoveredNames returns independent recovered names for one
// retained FC100 EVSE operation. It returns false for unsupported operations.
func TeslaFC100RecoveredNames(operation TeslaFC100Operation) (TeslaFC100OperationNames, bool) {
	names, ok := teslaFC100OperationNames[operation]
	names.Fields = append([]string(nil), names.Fields...)
	return names, ok
}

var teslaFC100OperationSpecs = map[TeslaFC100Operation]teslaFC100OperationSpec{
	TeslaFC100OperationCommonSystemInfo:      {4, 2, 3},
	TeslaFC100OperationWCGetVitals:           {6, 1, 2},
	TeslaFC100OperationWCGetLifetimeStats:    {6, 3, 4},
	TeslaFC100OperationWCConfigureSettings:   {6, 7, 8},
	TeslaFC100OperationWCGetSystemInfo:       {6, 9, 10},
	TeslaFC100OperationWCGetLoadSharingState: {6, 11, 12},
	TeslaFC100OperationWCSetProvisional:      {6, 25, 26},
	TeslaFC100OperationWCGetProvisional:      {6, 27, 28},
}

// TeslaFC100OperationResultKind distinguishes FC100 phase, terminal data, and
// a TEDAPI application error carried in a normal FC100 response.
type TeslaFC100OperationResultKind string

const (
	TeslaFC100OperationIntermediate     TeslaFC100OperationResultKind = "intermediate"
	TeslaFC100OperationTerminal         TeslaFC100OperationResultKind = "terminal"
	TeslaFC100OperationApplicationError TeslaFC100OperationResultKind = "application_error"
)

// TeslaFC100OperationResult retains a complete bounded opaque response body.
// Status is set only for the Common ErrorResponse shape.
type TeslaFC100OperationResult struct {
	Kind   TeslaFC100OperationResultKind
	Body   []byte
	Status uint64
}

// BuildTeslaFC100OperationRequest constructs the FC100 length envelope for a
// matrix-confirmed request pair. The caller supplies opaque body bytes when a
// field-level schema is not fixed by the compatibility contract. Construction
// does not grant send authority or validate safe current-only mutation scope;
// the current-limit operation admission contract must be enforced separately.
func BuildTeslaFC100OperationRequest(version string, operation TeslaFC100Operation, body []byte) (modbus.PrivateFunctionRequest, error) {
	if version != TeslaHSCFC100OperationCompatibilityV1 {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("tesla FC100 operation compatibility is unsupported")
	}
	spec, ok := teslaFC100OperationSpecs[operation]
	if !ok {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("tesla FC100 operation is unsupported")
	}
	inner := appendTeslaFC100Varint(nil, spec.requestTag<<3|2)
	inner = appendTeslaFC100Varint(inner, uint64(len(body)))
	inner = append(inner, body...)
	message := appendTeslaFC100Varint(nil, spec.family<<3|2)
	message = appendTeslaFC100Varint(message, uint64(len(inner)))
	message = append(message, inner...)
	if len(message) > 251 {
		return modbus.PrivateFunctionRequest{}, fmt.Errorf("tesla FC100 operation message exceeds bound")
	}
	payload := append([]byte{byte(len(message))}, message...)
	return modbus.NewPrivateFunctionRequest(teslaHSCFunction100, payload)
}

// DecodeTeslaFC100OperationSequence classifies one bounded FC100 normal-response sequence. Generic Modbus exceptions are deliberately not converted into TEDAPI application errors.
func DecodeTeslaFC100OperationSequence(version string, operation TeslaFC100Operation, requestPayload []byte, payloads [][]byte) ([]TeslaFC100OperationResult, error) {
	if version != TeslaHSCFC100OperationCompatibilityV1 {
		return nil, fmt.Errorf("tesla FC100 operation compatibility is unsupported")
	}
	spec, ok := teslaFC100OperationSpecs[operation]
	if !ok || len(payloads) == 0 || len(payloads) > 8 {
		return nil, fmt.Errorf("tesla FC100 operation sequence is invalid")
	}
	request, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, requestPayload)
	if err != nil {
		return nil, err
	}
	results := make([]TeslaFC100OperationResult, 0, len(payloads))
	terminal := false
	for _, payload := range payloads {
		response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
		if err != nil {
			return nil, err
		}
		message := response.Payload()
		if bytes.Equal(message, request.Payload()) {
			if terminal {
				return nil, fmt.Errorf("tesla FC100 operation echo follows terminal")
			}
			results = append(results, TeslaFC100OperationResult{Kind: TeslaFC100OperationIntermediate})
			continue
		}
		if terminal {
			return nil, fmt.Errorf("tesla FC100 operation has multiple terminals")
		}
		if status, ok := decodeTeslaFC100ApplicationError(message); ok {
			results = append(results, TeslaFC100OperationResult{Kind: TeslaFC100OperationApplicationError, Status: status})
			terminal = true
			continue
		}
		family, err := decodeExactLengthDelimitedField(message, spec.family)
		if err != nil {
			return nil, fmt.Errorf("tesla FC100 operation envelope: %w", err)
		}
		body, err := decodeExactLengthDelimitedField(family, spec.responseTag)
		if err != nil {
			return nil, fmt.Errorf("tesla FC100 operation terminal: %w", err)
		}
		results = append(results, TeslaFC100OperationResult{Kind: TeslaFC100OperationTerminal, Body: body})
		terminal = true
	}
	if !terminal {
		return nil, fmt.Errorf("tesla FC100 operation has no terminal")
	}
	return results, nil
}

func decodeTeslaFC100ApplicationError(message []byte) (uint64, bool) {
	common, err := decodeExactLengthDelimitedField(message, 4)
	if err != nil {
		return 0, false
	}
	body, err := decodeExactLengthDelimitedField(common, 1)
	if err != nil {
		return 0, false
	}
	key, width, err := decodeTeslaFC100Varint(body)
	if err != nil || key != 8 {
		return 0, false
	}
	status, consumed, err := decodeTeslaFC100Varint(body[width:])
	if err != nil || width+consumed != len(body) {
		return 0, false
	}
	return status, true
}

func appendTeslaFC100Varint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}
