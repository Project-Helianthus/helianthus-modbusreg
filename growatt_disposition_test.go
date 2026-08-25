package modbusreg

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestGrowattProtocolIIDispositionAdmitsOnlyOfflineIdentity(t *testing.T) {
	data, err := os.ReadFile("profiles/vendor/growatt/disposition.json")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Schema                        string `json:"schema"`
		Profile                       string `json:"profile"`
		ProfileVersion                string `json:"profile_version"`
		Outcome                       string `json:"outcome"`
		OfflineIdentityGateRegistered bool   `json:"offline_identity_gate_registered"`
		AutomaticRuntimeAdmission     bool   `json:"automatic_runtime_admission"`
		Evidence                      struct {
			PacketID          string `json:"packet_id"`
			DocsMergeSHA      string `json:"docs_merge_sha"`
			SourceIdentity    string `json:"source_identity"`
			SourceLicense     string `json:"source_license"`
			PacketDisposition string `json:"packet_disposition"`
			Eligible          bool   `json:"eligible"`
		}
		CandidateOperations []struct {
			Function         uint8
			Offset, Quantity uint16
			Purpose          string
			Executable       bool
		} `json:"candidate_operations"`
		Unresolved        []string            `json:"unresolved"`
		DecoderKeys       []SunSpecDecoderKey `json:"decoder_keys"`
		CatalogRegistered bool                `json:"catalog_registered"`
		SupportClaim      bool                `json:"support_claim"`
		ReopenRequires    []string            `json:"reopen_requires"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.Schema != "helianthus-modbusreg-profile-disposition/v1" ||
		record.Profile != "growatt" ||
		record.ProfileVersion != "1.0.0" ||
		record.Outcome != "OFFLINE_IDENTITY_ADMITTED" ||
		!record.OfflineIdentityGateRegistered || record.AutomaticRuntimeAdmission {
		t.Fatalf("unexpected disposition identity: %#v", record)
	}
	if record.Evidence.PacketID != "GROWATT-CANDIDATE-V1" ||
		record.Evidence.DocsMergeSHA != "6654837a4ec29a3f226a29f701574ee84495ff2f" ||
		record.Evidence.SourceIdentity != "sha256:fac88d609d74ff6b3c9c31ed65370d166d1fb17461e91b4b4855018fe232a320" ||
		record.Evidence.SourceLicense != "vendor-copyright-inspection-only" ||
		record.Evidence.PacketDisposition != "DOCUMENTED_OFFLINE_IDENTITY" ||
		!record.Evidence.Eligible {
		t.Fatalf("unexpected evidence lock: %#v", record.Evidence)
	}
	wantOperations := []struct {
		function         uint8
		offset, quantity uint16
		purpose          string
	}{{3, 9, 6, "firmware_tuple"}, {3, 43, 1, "device_type_code"}, {3, 82, 2, "model_build_code"}, {3, 88, 1, "protocol_version"}}
	if len(record.CandidateOperations) != len(wantOperations) {
		t.Fatalf("candidate operations=%d", len(record.CandidateOperations))
	}
	for index, operation := range record.CandidateOperations {
		want := wantOperations[index]
		if operation.Executable || operation.Function != want.function || operation.Offset != want.offset || operation.Quantity != want.quantity || operation.Purpose != want.purpose {
			t.Fatalf("candidate operation %d=%#v want=%#v", index, operation, want)
		}
	}
	if len(record.DecoderKeys) != 0 || record.CatalogRegistered || record.SupportClaim {
		t.Fatalf("non-admission leaked support: keys=%v catalog=%v claim=%v", record.DecoderKeys, record.CatalogRegistered, record.SupportClaim)
	}
	wantUnresolved := []string{"automatic_detector_uniqueness", "exact_fc04_telemetry_schema", "runtime_admission_contract"}
	wantReopen := []string{"mutually_exclusive_identity_tuple", "exact_fc04_telemetry_schema", "explicit_runtime_admission_contract"}
	if !reflect.DeepEqual(record.Unresolved, wantUnresolved) || !reflect.DeepEqual(record.ReopenRequires, wantReopen) {
		t.Fatalf("closure conditions unresolved=%v reopen=%v", record.Unresolved, record.ReopenRequires)
	}
	if strings.Contains(strings.ToLower(string(data)), "serial") {
		t.Fatal("disposition contains a sensitive identity field")
	}
}
