package modbusreg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// This independent allowlist follows docs/tesla-fc100-evse-scope.md. Extending
// the implementation's maps alone must not extend the public EVSE catalog.
func TestTeslaFC100CatalogStaysWithinEVSEScope(t *testing.T) {
	retained := map[TeslaFC100Operation]struct {
		spec  teslaFC100OperationSpec
		empty bool
	}{
		"common.system_info":        {teslaFC100OperationSpec{4, 2, 3}, true},
		"wc.get_vitals":             {teslaFC100OperationSpec{6, 1, 2}, true},
		"wc.get_lifetime_stats":     {teslaFC100OperationSpec{6, 3, 4}, true},
		"wc.configure_settings":     {teslaFC100OperationSpec{6, 7, 8}, false},
		"wc.get_system_info":        {teslaFC100OperationSpec{6, 9, 10}, true},
		"wc.get_load_sharing_state": {teslaFC100OperationSpec{6, 11, 12}, true},
		"wc.set_provisional":        {teslaFC100OperationSpec{6, 25, 26}, false},
		"wc.get_provisional":        {teslaFC100OperationSpec{6, 27, 28}, true},
	}
	// Guard the exported enumeration too: an unused out-of-scope constant
	// must not escape just because it was omitted from the runtime maps.
	catalog, err := parser.ParseFile(token.NewFileSet(), "tesla_fc100_operation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range catalog.Decls {
		constants, ok := declaration.(*ast.GenDecl)
		if !ok || constants.Tok != token.CONST {
			continue
		}
		for _, entry := range constants.Specs {
			constant := entry.(*ast.ValueSpec)
			kind, ok := constant.Type.(*ast.Ident)
			if !ok || kind.Name != "TeslaFC100Operation" {
				continue
			}
			if len(constant.Values) != 1 {
				t.Errorf("operation constant %s must have an explicit retained value", constant.Names[0])
				continue
			}
			literal, ok := constant.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("operation constant %s must have an explicit retained string", constant.Names[0])
				continue
			}
			value, err := strconv.Unquote(literal.Value)
			if _, ok := retained[TeslaFC100Operation(value)]; err != nil || !ok {
				t.Errorf("exported operation constant %s is outside the retained EVSE scope", constant.Names[0])
			}
		}
	}
	if len(teslaFC100OperationSpecs) != len(retained) {
		t.Errorf("catalog contains %d operations, want %d retained EVSE operations", len(teslaFC100OperationSpecs), len(retained))
	}
	for operation := range teslaFC100OperationSpecs {
		if _, ok := retained[operation]; !ok {
			t.Errorf("catalog operation %q is outside the retained EVSE scope", operation)
		}
	}
	for operation := range teslaFC100OperationNames {
		if _, ok := retained[operation]; !ok {
			t.Errorf("named operation %q is outside the retained EVSE scope", operation)
		}
	}
	for operation := range teslaFC100EmptyRequestOperations {
		if expected, ok := retained[operation]; !ok || !expected.empty {
			t.Errorf("fixed request operation %q is outside the retained empty-request set", operation)
		}
	}
	for operation, expected := range retained {
		if spec, ok := teslaFC100OperationSpecs[operation]; !ok || spec != expected.spec {
			t.Errorf("operation %q spec = %+v, present %v, want %+v", operation, spec, ok, expected.spec)
		}
		_, empty := teslaFC100EmptyRequestOperations[operation]
		if empty != expected.empty {
			t.Errorf("operation %q empty request = %v, want %v", operation, empty, expected.empty)
		}
		if names, ok := TeslaFC100RecoveredNames(operation); !ok || names.Request == "" || names.Response == "" {
			t.Errorf("operation %q lacks retained request/terminal names", operation)
		}
	}
}

func TestTeslaFC100RejectsOperationsOutsideEVSEScope(t *testing.T) {
	for _, operation := range []TeslaFC100Operation{
		"common.perform_update", "common.factory_reset", "common.wifi_scan",
		"common.configure_wifi", "common.check_for_update", "common.clear_update",
		"common.prepare_registration", "wc.get_config", "wc.set_load_sharing_operation",
		"wc.configure_load_sharing", "wc.configure_ppu", "wc.get_ppu",
		"wc.get_access_control", "wc.configure_access_control", "wc.get_recent_vehicles",
		"wc.push_ppu_authorization", "wc.configure_charge_schedule", "wc.push_charge_command",
		"wc.configure_third_party_vehicle", "wc.configure_home_site_controller",
		"wc.configure_ocpp", "wc.set_ocpp_security", "wc.get_ocpp_security",
		"wc.configure_operational", "wc.get_operational", "wc.configure_country_code",
		"neurio.configure_cts", "unknown",
	} {
		t.Run(string(operation), func(t *testing.T) {
			version := TeslaHSCFC100OperationCompatibilityV1
			if _, err := BuildTeslaFC100OperationRequest(version, operation, nil); err == nil {
				t.Error("out-of-scope request was constructed")
			}
			if _, err := ValidateTeslaFC100RequestMatrix(version, operation, []byte{0x08, 0x01}); err == nil {
				t.Error("out-of-scope request matrix was accepted")
			}
			// A valid application-error terminal must not bypass the operation allowlist.
			if _, err := DecodeTeslaFC100OperationSequence(version, operation,
				[]byte{0x04, 0x32, 0x02, 0x0a, 0x00},
				[][]byte{{0x06, 0x22, 0x04, 0x0a, 0x02, 0x08, 0x0e}}); err == nil {
				t.Error("out-of-scope operation sequence was accepted")
			}
			if _, ok := TeslaFC100RecoveredNames(operation); ok {
				t.Error("out-of-scope operation has public recovered names")
			}
			if _, err := NewTeslaGen3HSCNativeOperationRecord(TeslaGen3HSCNativeOperationRecordConfig{
				Profile: TeslaHSCProfileName, ProfileVersion: TeslaHSCCompatibilityV1,
				OperationVersion: version, Operation: operation,
				Function: modbus.PrivateFunctionCode(100), Direction: TeslaGen3HSCResponseDirection,
				Outcome: TeslaGen3HSCNormalOutcome,
			}); err == nil {
				t.Error("out-of-scope named native operation record was accepted")
			}
		})
	}
}
