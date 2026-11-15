package modbusreg_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// All profiles, views and normalization records below are synthetic.

func TestLogicalViewSnapshotValidReportsConstructorValidation(t *testing.T) {
	var zero reg.LogicalViewSnapshot
	if zero.Valid() || !reflect.DeepEqual(zero.Record(), reg.LogicalViewRecord{}) {
		t.Fatalf("zero snapshot: Valid=%v Record=%+v", zero.Valid(), zero.Record())
	}
	record := logicalViewRecord(1001, 100, 0, []uint16{0x0102, 0x0304})
	snapshot, err := reg.NewLogicalViewSnapshot(record)
	if err != nil {
		t.Fatalf("NewLogicalViewSnapshot: %v", err)
	}
	if !snapshot.Valid() || !reflect.DeepEqual(snapshot.Record(), record) {
		t.Fatalf("validated snapshot: Valid=%v Record=%+v", snapshot.Valid(), snapshot.Record())
	}
}

func TestNewObservationValidatesSpecAgainstProfile(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	observation, err := reg.NewObservation(profile, spec)
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	if observation.SampleID() != "" || len(observation.Replay()) != 2 {
		t.Fatalf("observation: sample=%q replay=%d", observation.SampleID(), len(observation.Replay()))
	}
	for index, dependency := range observation.Replay() {
		if dependency.DependencyID() != spec.Dependencies[index].DependencyID ||
			dependency.RuntimeNormalizationBytes() != nil {
			t.Fatalf("dependency %d: id=%q runtime=%q", index, dependency.DependencyID(), dependency.RuntimeNormalizationBytes())
		}
	}
	mismatched := spec
	mismatched.ProfileID = "example.standard.other"
	if _, err := reg.NewObservation(profile, mismatched); err == nil {
		t.Fatal("observation with another profile identity was accepted")
	}
	if _, err := reg.NewObservation(reg.ProfileDescriptor{}, spec); err == nil {
		t.Fatal("observation for the zero profile was accepted")
	}
}

// syntheticNormalization returns synthetic producer bytes and the fields they
// decode to for one dependency of profileFixture.
func syntheticNormalization(address uint32) ([]byte, modbus.RuntimeNormalizationFields) {
	encoded := fmt.Sprintf(
		`{"schema_version":1,"source_kind":"runtime","source_evidence_id":"urn:helianthus:evidence:example-register-map","documentary_notation":"one-based input register","documentary_address":%d,"documentary_address_base":"one_based_register","function_code":4,"logical_table":"input_registers","normalized_zero_based_pdu_offset":%d,"word_count":2}`,
		address,
		address-1,
	)
	return []byte(encoded), modbus.RuntimeNormalizationFields{
		SchemaVersion:                1,
		SourceKind:                   modbus.RuntimeAcquisitionSourceRuntime,
		SourceEvidenceID:             "urn:helianthus:evidence:example-register-map",
		DocumentaryNotation:          "one-based input register",
		DocumentaryAddress:           address,
		DocumentaryAddressBase:       "one_based_register",
		FunctionCode:                 modbus.FunctionReadInputRegisters,
		LogicalTable:                 modbus.InputRegisters,
		NormalizedZeroBasedPDUOffset: uint16(address - 1),
		WordCount:                    2,
	}
}

func syntheticNormalizationPair() ([][]byte, []modbus.RuntimeNormalizationFields) {
	firstBytes, firstFields := syntheticNormalization(101)
	secondBytes, secondFields := syntheticNormalization(102)
	return [][]byte{firstBytes, secondBytes}, []modbus.RuntimeNormalizationFields{firstFields, secondFields}
}

func TestNewRuntimeObservationAttachesMatchedNormalizationRecords(t *testing.T) {
	profile := profileFixture(t)
	spec := successfulObservationSpec(t, profile)
	encoded, fields := syntheticNormalizationPair()
	observation, err := reg.NewRuntimeObservation(profile, spec, encoded, fields)
	if err != nil {
		t.Fatalf("NewRuntimeObservation: %v", err)
	}
	encoded[0][0] = '['
	for index, dependency := range observation.Replay() {
		want, wantFields := syntheticNormalization(uint32(101 + index))
		if !bytes.Equal(dependency.RuntimeNormalizationBytes(), want) ||
			dependency.RuntimeNormalizationFields() != wantFields {
			t.Fatalf("dependency %d runtime record was not retained exactly", index)
		}
	}
	published, err := observation.WithSampleID("synthetic-issuer:1")
	if err != nil {
		t.Fatalf("WithSampleID: %v", err)
	}
	data, err := reg.MarshalObservation(published)
	if err != nil {
		t.Fatalf("MarshalObservation: %v", err)
	}
	decoded, err := reg.UnmarshalObservation(profile, data)
	if err != nil || !reflect.DeepEqual(decoded.Replay(), published.Replay()) || decoded.SampleID() != "synthetic-issuer:1" {
		t.Fatalf("durable round trip: err=%v sample=%q", err, decoded.SampleID())
	}
	cases := map[string]func([][]byte, []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields){
		"missing_bytes": func(b [][]byte, f []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields) {
			return b[:1], f
		},
		"missing_fields": func(b [][]byte, f []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields) {
			return b, f[:1]
		},
		"empty_bytes": func(b [][]byte, f []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields) {
			b[1] = nil
			return b, f
		},
		"oversized_bytes": func(b [][]byte, f []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields) {
			b[1] = make([]byte, reg.MaxSerializedContractBytes+1)
			return b, f
		},
		"disagreeing_fields": func(b [][]byte, f []modbus.RuntimeNormalizationFields) ([][]byte, []modbus.RuntimeNormalizationFields) {
			f[1].WordCount = 3
			return b, f
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, fields := mutate(syntheticNormalizationPair())
			if _, err := reg.NewRuntimeObservation(profile, spec, encoded, fields); err == nil {
				t.Fatal("invalid runtime normalization records were accepted")
			}
		})
	}
}

func TestObservationWithSampleIDRequiresIdentityAndValidObservation(t *testing.T) {
	profile := profileFixture(t)
	encoded, fields := syntheticNormalizationPair()
	observation, err := reg.NewRuntimeObservation(profile, successfulObservationSpec(t, profile), encoded, fields)
	if err != nil {
		t.Fatalf("NewRuntimeObservation: %v", err)
	}
	for _, sampleID := range []string{"", "synthetic issuer:1", "synthetic-issuer:\u00e9", strings.Repeat("a", reg.MaxContractStringBytes+1)} {
		if _, err := observation.WithSampleID(sampleID); err == nil {
			t.Fatalf("sample identity %q was accepted", sampleID)
		}
	}
	if _, err := (reg.Observation{}).WithSampleID("synthetic-issuer:1"); err == nil {
		t.Fatal("zero observation accepted a sample identity")
	}
	fixtureOnly, err := reg.NewObservation(profile, successfulObservationSpec(t, profile))
	if err != nil {
		t.Fatalf("NewObservation: %v", err)
	}
	if _, err := fixtureOnly.WithSampleID("synthetic-issuer:1"); err == nil ||
		!strings.Contains(err.Error(), "durable observation envelope") {
		t.Fatalf("observation without runtime records: err=%v", err)
	}
	identified, err := observation.WithSampleID("synthetic-issuer:7")
	if err != nil || identified.SampleID() != "synthetic-issuer:7" || observation.SampleID() != "" {
		t.Fatalf("WithSampleID: err=%v copy=%q original=%q", err, identified.SampleID(), observation.SampleID())
	}
}

func TestWithLocalReceiptTimeRecordsZeroTimeOnlyWhenPresent(t *testing.T) {
	profile := profileFixture(t)
	base := successfulObservationSpec(t, profile)
	receipt := time.Unix(1_700_000_001, 0).UTC()
	for _, tc := range []struct {
		name    string
		value   time.Time
		present bool
		valid   bool
		json    string
	}{
		{"absent_zero", time.Time{}, false, false, ""},
		{"present_zero", time.Time{}, true, true, `"local_receipt_time":"0001-01-01T00:00:00Z"`},
		{"nonzero_without_flag", receipt, false, true, `"local_receipt_time":"2023-11-14T22:13:21Z"`},
	} {
		t.Run("observation_"+tc.name, func(t *testing.T) {
			spec := base.WithLocalReceiptTime(tc.value, tc.present)
			if !spec.LocalReceiptTime.Equal(tc.value) {
				t.Fatalf("receipt time = %v", spec.LocalReceiptTime)
			}
			_, err := reg.NewObservation(profile, spec)
			if (err == nil) != tc.valid {
				t.Fatalf("NewObservation err=%v, want valid=%v", err, tc.valid)
			}
			if data, err := reg.MarshalFixtureSpec(spec); tc.valid && (err != nil || !bytes.Contains(data, []byte(tc.json))) {
				t.Fatalf("MarshalFixtureSpec=%s err=%v, want %s", data, err, tc.json)
			}
		})
	}
	for _, tc := range []struct {
		name    string
		present bool
		valid   bool
	}{
		{"dependency_absent_zero", false, true},
		{"dependency_present_zero", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := reg.CloneObservationSpec(base)
			spec.Dependencies[0] = spec.Dependencies[0].WithLocalReceiptTime(time.Time{}, tc.present)
			if _, err := reg.NewObservation(profile, spec); (err == nil) != tc.valid {
				t.Fatalf("single-wire dependency receipt: err=%v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestCloneObservationSpecSharesNoDependencyState(t *testing.T) {
	profile := profileFixture(t)
	original := successfulObservationSpec(t, profile)
	clone := reg.CloneObservationSpec(original)
	if !reflect.DeepEqual(clone, original) {
		t.Fatal("clone differs from its source")
	}
	clone.Dependencies[0].DependencyID = "changed"
	if original.Dependencies[0].DependencyID == "changed" {
		t.Fatal("clone shares the dependency collection")
	}
	if reg.CloneObservationSpec(reg.ObservationSpec{}).Dependencies != nil {
		t.Fatal("clone of a spec without dependencies allocated a collection")
	}
}

func TestReceiptAndSourceTimeRules(t *testing.T) {
	zone := time.FixedZone("synthetic", 2*60*60)
	local := time.Date(2024, 1, 2, 5, 4, 5, 6, zone)
	for _, tc := range []struct {
		value   time.Time
		present bool
		has     bool
	}{
		{time.Time{}, false, false},
		{time.Time{}, true, true},
		{local, false, true},
	} {
		if got := reg.HasLocalReceiptTime(tc.value, tc.present); got != tc.has {
			t.Fatalf("HasLocalReceiptTime(%v, %v)=%v", tc.value, tc.present, got)
		}
		canonical, err := reg.CanonicalRequiredTime(tc.value, tc.present)
		if (err == nil) != tc.has || (tc.has && (!canonical.Equal(tc.value) || canonical.Location() != time.UTC)) {
			t.Fatalf("CanonicalRequiredTime(%v, %v)=(%v, %v)", tc.value, tc.present, canonical, err)
		}
	}
	if _, err := reg.CanonicalRequiredTime(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), true); err == nil {
		t.Fatal("receipt time after year 9999 was accepted")
	}
	for _, tc := range []struct {
		name  string
		spec  reg.SourceTimeSpec
		valid bool
	}{
		{"observed", reg.SourceTimeObserved(local), true},
		{"unavailable", reg.SourceTimeUnavailable(), true},
		{"unavailable_with_time", reg.SourceTimeSpec{State: reg.SourceTimeUnavailableState, Time: local}, false},
		{"observed_out_of_range", reg.SourceTimeObserved(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)), false},
		{"unknown_state", reg.SourceTimeSpec{State: "guessed"}, false},
	} {
		if err := reg.ValidateSourceTime(tc.spec); (err == nil) != tc.valid {
			t.Fatalf("%s: ValidateSourceTime err=%v, want valid=%v", tc.name, err, tc.valid)
		}
	}
}

// Each case changes one of the ten required fields; the production comparison
// of that field is what rejects it.
func TestMatchRuntimeNormalizationRejectsEveryDisagreeingField(t *testing.T) {
	declaration := profileFixture(t).Dependencies().Dependencies()[0]
	_, fields := syntheticNormalization(101)
	if err := reg.MatchRuntimeNormalization(declaration, fields); err != nil {
		t.Fatalf("matching fields were rejected: %v", err)
	}
	for name, mutate := range map[string]func(*modbus.RuntimeNormalizationFields){
		"schema_version":                   func(f *modbus.RuntimeNormalizationFields) { f.SchemaVersion = 2 },
		"source_kind":                      func(f *modbus.RuntimeNormalizationFields) { f.SourceKind = "fixture" },
		"source_evidence_id":               func(f *modbus.RuntimeNormalizationFields) { f.SourceEvidenceID += "-other" },
		"documentary_notation":             func(f *modbus.RuntimeNormalizationFields) { f.DocumentaryNotation += " other" },
		"documentary_address":              func(f *modbus.RuntimeNormalizationFields) { f.DocumentaryAddress++ },
		"documentary_address_base":         func(f *modbus.RuntimeNormalizationFields) { f.DocumentaryAddressBase = "zero_based_register" },
		"function_code":                    func(f *modbus.RuntimeNormalizationFields) { f.FunctionCode = modbus.FunctionReadHoldingRegisters },
		"logical_table":                    func(f *modbus.RuntimeNormalizationFields) { f.LogicalTable = modbus.HoldingRegisters },
		"normalized_zero_based_pdu_offset": func(f *modbus.RuntimeNormalizationFields) { f.NormalizedZeroBasedPDUOffset++ },
		"word_count":                       func(f *modbus.RuntimeNormalizationFields) { f.WordCount++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := fields
			mutate(&changed)
			if err := reg.MatchRuntimeNormalization(declaration, changed); err == nil {
				t.Fatal("disagreeing field was accepted")
			}
		})
	}
}
