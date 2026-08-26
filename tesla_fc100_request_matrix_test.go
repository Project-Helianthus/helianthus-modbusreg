package modbusreg

import "testing"

func TestTeslaFC100RequestMatrixRecognizesFixedAndStructuredRequests(t *testing.T) {
	if _, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationWCGetConfig, nil); err != nil { t.Fatal(err) }
	if _, err := ValidateTeslaFC100RequestMatrix(TeslaHSCFC100OperationCompatibilityV1, TeslaFC100OperationCommonWifiScan, nil); err == nil { t.Fatal("empty C10 accepted") }
	if status, ok := TeslaFC100DeterministicApplicationStatus(TeslaFC100OperationWCPushPPUAuthorization); !ok || status != 7 { t.Fatalf("W35 status = %d, %v", status, ok) }
}
