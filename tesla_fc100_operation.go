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

// TeslaFC100Operation names one version-scoped FC100 request/response pair.
// It does not assign meanings to an opaque request or response body.
type TeslaFC100Operation string

const (
	TeslaFC100OperationCommonSystemInfo              TeslaFC100Operation = "common.system_info"
	TeslaFC100OperationCommonPerformUpdate           TeslaFC100Operation = "common.perform_update"
	TeslaFC100OperationCommonFactoryReset            TeslaFC100Operation = "common.factory_reset"
	TeslaFC100OperationCommonWifiScan                TeslaFC100Operation = "common.wifi_scan"
	TeslaFC100OperationCommonConfigureWifi           TeslaFC100Operation = "common.configure_wifi"
	TeslaFC100OperationCommonCheckForUpdate          TeslaFC100Operation = "common.check_for_update"
	TeslaFC100OperationCommonClearUpdate             TeslaFC100Operation = "common.clear_update"
	TeslaFC100OperationCommonPrepareRegistration     TeslaFC100Operation = "common.prepare_registration"
	TeslaFC100OperationWCGetVitals                   TeslaFC100Operation = "wc.get_vitals"
	TeslaFC100OperationWCGetLifetimeStats            TeslaFC100Operation = "wc.get_lifetime_stats"
	TeslaFC100OperationWCGetConfig                   TeslaFC100Operation = "wc.get_config"
	TeslaFC100OperationWCConfigureSettings           TeslaFC100Operation = "wc.configure_settings"
	TeslaFC100OperationWCGetSystemInfo               TeslaFC100Operation = "wc.get_system_info"
	TeslaFC100OperationWCGetLoadSharingState         TeslaFC100Operation = "wc.get_load_sharing_state"
	TeslaFC100OperationWCSetLoadSharingOperation     TeslaFC100Operation = "wc.set_load_sharing_operation"
	TeslaFC100OperationWCConfigureLoadSharing        TeslaFC100Operation = "wc.configure_load_sharing"
	TeslaFC100OperationWCConfigurePPU                TeslaFC100Operation = "wc.configure_ppu"
	TeslaFC100OperationWCGetPPU                      TeslaFC100Operation = "wc.get_ppu"
	TeslaFC100OperationWCSetProvisional              TeslaFC100Operation = "wc.set_provisional"
	TeslaFC100OperationWCGetProvisional              TeslaFC100Operation = "wc.get_provisional"
	TeslaFC100OperationWCGetAccessControl            TeslaFC100Operation = "wc.get_access_control"
	TeslaFC100OperationWCConfigureAccessControl      TeslaFC100Operation = "wc.configure_access_control"
	TeslaFC100OperationWCGetRecentVehicles           TeslaFC100Operation = "wc.get_recent_vehicles"
	TeslaFC100OperationWCPushPPUAuthorization        TeslaFC100Operation = "wc.push_ppu_authorization"
	TeslaFC100OperationWCConfigureChargeSchedule     TeslaFC100Operation = "wc.configure_charge_schedule"
	TeslaFC100OperationWCPushChargeCommand           TeslaFC100Operation = "wc.push_charge_command"
	TeslaFC100OperationWCConfigureThirdPartyVehicle  TeslaFC100Operation = "wc.configure_third_party_vehicle"
	TeslaFC100OperationWCConfigureHomeSiteController TeslaFC100Operation = "wc.configure_home_site_controller"
	TeslaFC100OperationWCConfigureOCPP               TeslaFC100Operation = "wc.configure_ocpp"
	TeslaFC100OperationWCSetOCPPSecurity             TeslaFC100Operation = "wc.set_ocpp_security"
	TeslaFC100OperationWCGetOCPPSecurity             TeslaFC100Operation = "wc.get_ocpp_security"
	TeslaFC100OperationWCConfigureOperational        TeslaFC100Operation = "wc.configure_operational"
	TeslaFC100OperationWCGetOperational              TeslaFC100Operation = "wc.get_operational"
	TeslaFC100OperationWCConfigureCountryCode        TeslaFC100Operation = "wc.configure_country_code"
	TeslaFC100OperationNeurioConfigureCTs            TeslaFC100Operation = "neurio.configure_cts"
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
	TeslaFC100OperationCommonSystemInfo:              {"CommonAPIGetSystemInfo", "CommonSystemInfo", []string{"device_id", "din", "firmware_version", "system_update", "device_type"}},
	TeslaFC100OperationCommonWifiScan:                {"CommonAPIWifiScan", "WifiScanResponse", []string{"wifi_networks", "ssid", "rssi_value", "rssi", "security_type"}},
	TeslaFC100OperationWCGetVitals:                   {"GetVitals", "WCVitals", nil},
	TeslaFC100OperationWCGetLifetimeStats:            {"GetLifetimeStats", "WCLifetimeStats", []string{"uptime_s", "alert_count", "contactor_cycles", "contactor_cycles_loaded", "connector_cycles", "thermal_foldbacks", "avg_startup_temp_c", "charge_starts", "charging_time_s", "charging_energy"}},
	TeslaFC100OperationWCGetConfig:                   {"GetConfig", "WCConfig", []string{"settings", "wifi_config", "wifi", "meters", "charge_schedule", "ocpp_settings", "vehicle_to_home"}},
	TeslaFC100OperationWCGetSystemInfo:               {"GetSystemInfo", "WCGenealogy", []string{"region", "handle_type", "hardware_features"}},
	TeslaFC100OperationWCGetLoadSharingState:         {"GetLoadSharingNetworkState", "WCLoadSharingNetworkState", []string{"devices", "leader_state", "settings", "status", "limits"}},
	TeslaFC100OperationWCGetPPU:                      {"GetPpuSettings", "WCPpuConfig", []string{"session_reporting_mode"}},
	TeslaFC100OperationWCGetProvisional:              {"GetProvisionalOperationalParams", "WCProvisionalOperationalParams", []string{"limit_current_max_amps", "limit_timeout_s", "inhibit_charging", "configured_current_limit_amps"}},
	TeslaFC100OperationWCGetOperational:              {"GetOperationalSettings", "WCOperationalSettingsConfig", []string{"operational_mode", "emit_increased_telemetry"}},
	TeslaFC100OperationWCGetAccessControl:            {"GetAccessControlSettings", "WCAccessControlEntry", []string{"vin", "name", "model", "model_year", "drive_type"}},
	TeslaFC100OperationWCConfigureAccessControl:      {"ConfigureAccessControlSettings", "WCAccessControlEntry", []string{"operation", "vin", "name"}},
	TeslaFC100OperationWCGetRecentVehicles:           {"GetRecentVehicles", "RecentVehicles", []string{"recent_vehicles", "vin"}},
	TeslaFC100OperationWCGetOCPPSecurity:             {"GetOcppSecurityParameter", "OcppSecurityParameter", []string{"security_parameter_type", "security_parameter"}},
	TeslaFC100OperationNeurioConfigureCTs:            {"NeurioMeterAPIConfigureCts", "ConfigureCtsResponse", []string{"serial", "ct_config", "location", "real_power_scale_factor"}},
	TeslaFC100OperationCommonPerformUpdate:           {"CommonAPIPerformUpdate", "PerformUpdateResponse", nil},
	TeslaFC100OperationCommonFactoryReset:            {"CommonAPIFactoryReset", "FactoryResetResponse", nil},
	TeslaFC100OperationCommonConfigureWifi:           {"CommonAPIConfigureWifi", "ConfigureWifiResponse", []string{"enabled", "wifi_config", "ssid", "password", "value", "security_type"}},
	TeslaFC100OperationCommonCheckForUpdate:          {"CommonAPICheckForUpdate", "CheckForUpdateResponse", []string{"download_if_available"}},
	TeslaFC100OperationCommonPrepareRegistration:     {"CommonAPIPrepareRegistrationPayload", "PrepareRegistrationPayloadResponse", []string{"customer_registration_info"}},
	TeslaFC100OperationWCConfigureSettings:           {"ConfigureSettings", "ConfigureSettingsResponse", []string{"settings"}},
	TeslaFC100OperationWCSetLoadSharingOperation:     {"SetLoadSharingNetworkOperation", "SetLoadSharingNetworkOperationResponse", []string{"charging_enabled"}},
	TeslaFC100OperationWCConfigurePPU:                {"ConfigurePpuSettings", "ConfigurePpuSettingsResponse", []string{"ppu_config"}},
	TeslaFC100OperationWCPushPPUAuthorization:        {"PushPpuAuthorizationState", "PushPpuAuthorizationStateResponse", []string{"authorized", "auth_uuid"}},
	TeslaFC100OperationWCConfigureChargeSchedule:     {"ConfigureChargeSchedule", "ConfigureChargeScheduleResponse", []string{"config", "time_zone"}},
	TeslaFC100OperationWCPushChargeCommand:           {"PushChargeCommand", "PushChargeCommandResponse", []string{"charge_command"}},
	TeslaFC100OperationWCConfigureHomeSiteController: {"ConfigureHomeSiteController", "ConfigureHomeSiteControllerResponse", []string{"din", "modbus_node_id", "vehicle_to_home"}},
	TeslaFC100OperationWCSetOCPPSecurity:             {"SetOcppSecurityParameter", "SetOcppSecurityParameterResponse", []string{"security_parameter_type", "security_parameter"}},
	TeslaFC100OperationWCConfigureCountryCode:        {"ConfigureCountryCodeSettings", "ConfigureCountryCodeSettingsResponse", []string{"country"}},
}

// TeslaFC100RecoveredNames returns independent recovered names for one
// complete FC100 operation. It returns false for pairs with no field names yet.
func TeslaFC100RecoveredNames(operation TeslaFC100Operation) (TeslaFC100OperationNames, bool) {
	names, ok := teslaFC100OperationNames[operation]
	names.Fields = append([]string(nil), names.Fields...)
	return names, ok
}

var teslaFC100OperationSpecs = map[TeslaFC100Operation]teslaFC100OperationSpec{
	TeslaFC100OperationCommonSystemInfo: {4, 2, 3}, TeslaFC100OperationCommonPerformUpdate: {4, 6, 7}, TeslaFC100OperationCommonFactoryReset: {4, 8, 9}, TeslaFC100OperationCommonWifiScan: {4, 10, 11}, TeslaFC100OperationCommonConfigureWifi: {4, 12, 13}, TeslaFC100OperationCommonCheckForUpdate: {4, 14, 15}, TeslaFC100OperationCommonClearUpdate: {4, 16, 17}, TeslaFC100OperationCommonPrepareRegistration: {4, 36, 37},
	TeslaFC100OperationWCGetVitals: {6, 1, 2}, TeslaFC100OperationWCGetLifetimeStats: {6, 3, 4}, TeslaFC100OperationWCGetConfig: {6, 5, 6}, TeslaFC100OperationWCConfigureSettings: {6, 7, 8}, TeslaFC100OperationWCGetSystemInfo: {6, 9, 10}, TeslaFC100OperationWCGetLoadSharingState: {6, 11, 12}, TeslaFC100OperationWCSetLoadSharingOperation: {6, 17, 18}, TeslaFC100OperationWCConfigureLoadSharing: {6, 19, 20}, TeslaFC100OperationWCConfigurePPU: {6, 21, 22}, TeslaFC100OperationWCGetPPU: {6, 23, 24}, TeslaFC100OperationWCSetProvisional: {6, 25, 26}, TeslaFC100OperationWCGetProvisional: {6, 27, 28}, TeslaFC100OperationWCGetAccessControl: {6, 29, 30}, TeslaFC100OperationWCConfigureAccessControl: {6, 31, 32}, TeslaFC100OperationWCGetRecentVehicles: {6, 33, 34}, TeslaFC100OperationWCPushPPUAuthorization: {6, 35, 36}, TeslaFC100OperationWCConfigureChargeSchedule: {6, 37, 38}, TeslaFC100OperationWCPushChargeCommand: {6, 39, 40}, TeslaFC100OperationWCConfigureThirdPartyVehicle: {6, 41, 42}, TeslaFC100OperationWCConfigureHomeSiteController: {6, 43, 44}, TeslaFC100OperationWCConfigureOCPP: {6, 45, 46}, TeslaFC100OperationWCSetOCPPSecurity: {6, 47, 48}, TeslaFC100OperationWCGetOCPPSecurity: {6, 49, 50}, TeslaFC100OperationWCConfigureOperational: {6, 51, 52}, TeslaFC100OperationWCGetOperational: {6, 53, 54}, TeslaFC100OperationWCConfigureCountryCode: {6, 55, 56},
	TeslaFC100OperationNeurioConfigureCTs: {9, 5, 6},
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
// field-level schema is not fixed by the compatibility contract.
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
