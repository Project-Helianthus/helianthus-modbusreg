package modbusreg

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
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
	allowed := make(map[TeslaFC100Operation]bool, len(retained))
	for operation := range retained {
		allowed[operation] = true
	}
	for _, finding := range teslaFC100SourceScopeFindings(".", allowed) {
		t.Error(finding)
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

// teslaFC100SourceScopeFindings is shared by the real-package guard and the
// mutation tests, so the probes exercise the same source inspection.
func teslaFC100SourceScopeFindings(directory string, allowed map[TeslaFC100Operation]bool) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return []string{err.Error()}
	}
	fileSet := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			return []string{err.Error()}
		}
		if file.Name.Name == "modbusreg" {
			files = append(files, file)
		}
	}
	// Local contextual types catch explicit declarations, inferred conversion
	// forms, aliases, and strings inside operation-typed composite literals.
	// Import resolution is deliberately unnecessary: unrelated unresolved
	// imports/types do not stop go/types from recording local operation values.
	// This scope guard supplements, rather than replaces, the normal compiler.
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	config := types.Config{Error: func(error) {}}
	pkg, _ := config.Check("modbusreg", fileSet, files, info)
	if pkg == nil || pkg.Scope().Lookup("TeslaFC100Operation") == nil {
		return []string{"package has no TeslaFC100Operation type to inspect"}
	}
	operationType := pkg.Scope().Lookup("TeslaFC100Operation").Type()
	var findings []string
	for _, file := range files {
		// The empty operation is an opaque FC101/FC102 context sentinel, not
		// a catalog entry. Permit its comparison, but not a declared value.
		sentinelComparisons := make(map[ast.Expr]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			comparison, ok := node.(*ast.BinaryExpr)
			if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
				return true
			}
			for _, operand := range []ast.Expr{comparison.X, comparison.Y} {
				value := info.Types[operand]
				if value.Value != nil && value.Value.Kind() == constant.String && constant.StringVal(value.Value) == "" {
					sentinelComparisons[operand] = true
				}
			}
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			expression, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			value, ok := info.Types[expression]
			if !ok || sentinelComparisons[expression] || value.Value == nil || value.Value.Kind() != constant.String || !types.Identical(value.Type, operationType) {
				return true
			}
			operation := constant.StringVal(value.Value)
			if !allowed[TeslaFC100Operation(operation)] {
				findings = append(findings, fmt.Sprintf("%s: operation %q is outside the retained EVSE scope", fileSet.Position(expression.Pos()), operation))
			}
			return true
		})
	}
	return findings
}

func TestTeslaFC100SourceScopeGuardRejectsMutationEscapes(t *testing.T) {
	allowed := map[TeslaFC100Operation]bool{
		"common.system_info": true, "wc.get_vitals": true,
		"wc.get_lifetime_stats": true, "wc.configure_settings": true,
		"wc.get_system_info": true, "wc.get_load_sharing_state": true,
		"wc.set_provisional": true, "wc.get_provisional": true,
	}
	for _, mutation := range []struct {
		name, declaration string
	}{
		{"typed constant in another file", `const ReviewEscape TeslaFC100Operation = "wc.get_config"`},
		{"conversion-form constant", `const ReviewEscape = TeslaFC100Operation("wc.get_config")`},
		{"exported typed variable", `var ReviewEscape TeslaFC100Operation = "wc.get_config"`},
		{"exported inferred variable", `var ReviewEscape = TeslaFC100Operation("wc.get_config")`},
		{"exported variable assigned in init", `var ReviewEscape TeslaFC100Operation; func init() { ReviewEscape = "wc.get_config" }`},
		{"operation type alias", `type ReviewOperation = TeslaFC100Operation; const ReviewEscape ReviewOperation = "wc.get_config"`},
		{"slice composite", `var reviewEscape = []TeslaFC100Operation{"wc.get_config"}`},
		{"map composite key", `var reviewEscape = map[TeslaFC100Operation]bool{"wc.get_config": true}`},
		{"map composite value", `var reviewEscape = map[string]TeslaFC100Operation{"operation": "wc.get_config"}`},
		{"struct composite", `var reviewEscape = TeslaGen3HSCNativeOperationRecordConfig{Operation: "wc.get_config"}`},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			directory := t.TempDir()
			files, err := filepath.Glob("*.go")
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if strings.HasSuffix(file, "_test.go") {
					continue
				}
				contents, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, file), contents, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if findings := teslaFC100SourceScopeFindings(directory, allowed); len(findings) != 0 {
				t.Fatalf("unmutated package rejected: %v", findings)
			}
			if err := os.WriteFile(filepath.Join(directory, "review_scope_escape.go"), []byte("package modbusreg\n"+mutation.declaration+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			findings := teslaFC100SourceScopeFindings(directory, allowed)
			if report := strings.Join(findings, "\n"); !strings.Contains(report, "review_scope_escape.go") || !strings.Contains(report, `operation "wc.get_config" is outside the retained EVSE scope`) {
				t.Fatalf("out-of-scope mutation escaped the package guard: %v", findings)
			}
			retained := strings.ReplaceAll(mutation.declaration, "wc.get_config", "wc.get_vitals")
			if err := os.WriteFile(filepath.Join(directory, "review_scope_escape.go"), []byte("package modbusreg\n"+retained+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if findings := teslaFC100SourceScopeFindings(directory, allowed); len(findings) != 0 {
				t.Fatalf("retained operation rejected in the same declaration form: %v", findings)
			}
		})
	}
}
