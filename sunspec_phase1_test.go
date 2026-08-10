package modbusreg_test

import (
	"reflect"
	"testing"
	"time"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestSunSpecPhaseOneProfileIsVersionGatedAndReadOnly(t *testing.T) {
	profile, err := reg.NewSunSpecPhaseOneProfile(
		reg.SunSpecPhaseOneVersions{Profile: version(t, "1.0.0"), Codec: version(t, "1.0.0")},
	)
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneProfile(v1): %v", err)
	}
	if profile.Kind() != reg.ProfileStandardFamily || len(profile.Spec().VendorApplicability) != 0 {
		t.Fatalf("profile leaked a vendor assumption: %#v", profile.Spec())
	}
	if profile.CodecContractVersion() != version(t, "1.0.0") {
		t.Fatalf("codec contract version = %s", profile.CodecContractVersion())
	}
	for _, unsupported := range []reg.SunSpecPhaseOneVersions{
		{Profile: version(t, "2.0.0"), Codec: version(t, "1.0.0")},
		{Profile: version(t, "1.0.0"), Codec: version(t, "2.0.0")},
	} {
		if _, err := reg.NewSunSpecPhaseOneProfile(unsupported); err == nil {
			t.Fatalf("unsupported versions accepted: %#v", unsupported)
		}
	}
	altered := profile.Spec()
	altered.ModelApplicability = []string{"model-1"}
	otherProfile, err := reg.NewProfileDescriptor(altered)
	if err != nil {
		t.Fatalf("NewProfileDescriptor(altered): %v", err)
	}
	if _, err := reg.NewSunSpecPhaseOneDecoder(otherProfile); err == nil {
		t.Fatal("decoder accepted a same-ID profile with altered applicability")
	}

	decoder, err := reg.NewSunSpecPhaseOneDecoder(profile)
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneDecoder: %v", err)
	}
	request := decoder.DiscoveryRequest()
	if request.Function() != reg.FunctionReadHoldingRegisters || request.Table() != reg.HoldingRegisters ||
		request.Offset() != 40000 || !request.ReadOnly() {
		t.Fatalf("discovery request = %#v", request)
	}
	for _, value := range []any{decoder, request} {
		typ := reflect.TypeOf(value)
		for method := 0; method < typ.NumMethod(); method++ {
			name := typ.Method(method).Name
			if name == "Write" || name == "Control" || name == "Set" {
				t.Fatalf("phase-one surface exposes %s on %s", name, typ)
			}
		}
	}
}

func TestSunSpecPhaseOneChainIsBoundedAndSupportsOnlyIntScaleModels(t *testing.T) {
	profile, _ := reg.NewSunSpecPhaseOneProfile(reg.SunSpecPhaseOneVersions{
		Profile: version(t, "1.0.0"), Codec: version(t, "1.0.0"),
	})
	decoder, err := reg.NewSunSpecPhaseOneDecoder(profile)
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneDecoder: %v", err)
	}
	valid := sunSpecWords(1, 65, 101, 50, 666, 3, 102, 50, 103, 50, 0xffff, 0)
	chain, err := decoder.Parse(valid)
	if err != nil {
		t.Fatalf("Parse(valid chain): %v", err)
	}
	if got := chain.Models(); !reflect.DeepEqual(modelIDs(got), []uint16{1, 101, 102, 103}) {
		t.Fatalf("published models = %#v", got)
	}
	if skipped := chain.SkippedModels(); !reflect.DeepEqual(modelIDs(skipped), []uint16{666}) {
		t.Fatalf("structurally skipped models = %#v", skipped)
	}
	for _, words := range [][]uint16{
		{0, 0, 1, 1, 0, 0xffff, 0},                                     // invalid signature
		sunSpecWords(1, 0, 0xffff, 0),                                  // zero non-end length
		append([]uint16{0x5375, 0x6e53, 1, 65}, make([]uint16, 64)...), // declared extent overruns
		sunSpecWords(1, 65),                                            // missing end
		sunSpecWords(1, 65, 0xffff, 1),                                 // nonzero end length
		sunSpecWords(200, 50, 0xffff, 0),                               // deferred model
		sunSpecWords(777, 3, 0xffff, 0),                                // deferred 7xx model
	} {
		if _, err := decoder.Parse(words); err == nil {
			t.Fatalf("invalid or deferred chain accepted: %v", words)
		}
	}
}

func TestSunSpecPhaseOneDecodingAndObservationActivationPreserveProvenance(t *testing.T) {
	profile, _ := reg.NewSunSpecPhaseOneProfile(reg.SunSpecPhaseOneVersions{
		Profile: version(t, "1.0.0"), Codec: version(t, "1.0.0"),
	})
	decoder, err := reg.NewSunSpecPhaseOneDecoder(profile)
	if err != nil {
		t.Fatalf("NewSunSpecPhaseOneDecoder: %v", err)
	}
	if got, err := decoder.Int16(0xff85); err != nil || got != -123 {
		t.Fatalf("Int16 = %d, %v", got, err)
	}
	if got, err := decoder.Acc32(0x0001, 0xd4c0); err != nil || got != 120000 {
		t.Fatalf("Acc32 = %d, %v", got, err)
	}
	for raw, want := range map[uint16]int16{0xfff6: -10, 0x000a: 10} {
		if got, err := decoder.ScaleFactor(raw); err != nil || got != want {
			t.Fatalf("ScaleFactor(%#x) = %d, %v", raw, got, err)
		}
	}
	if _, err := decoder.ScaleFactor(0x8000); err == nil {
		t.Fatal("sunssf sentinel was accepted")
	}
	if got, err := decoder.String([]uint16{0x4142, 0x0043, 0x4445}); err != nil || got != "AB" {
		t.Fatalf("first-NUL string = %q, %v", got, err)
	}
	if got, err := decoder.String([]uint16{0x4142, 0x4344}); err != nil || got != "ABCD" {
		t.Fatalf("full-width string = %q, %v", got, err)
	}

	raw := sunSpecWords(1, 65, 102, 50, 0xffff, 0)
	chain, err := decoder.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	capture, observationSpec := reviewCapture(t, profile, raw, 77, 5000, reg.TransportTCP)
	activation := reg.SunSpecPhaseOneActivation{Chain: chain, RawWords: raw, Capture: capture, Observation: observationSpec}
	observation, err := decoder.Activate(activation)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got := observation.Spec()
	want := activation.Observation
	if got.SampleID != want.SampleID || got.PollGenerationID != want.PollGenerationID ||
		got.ProfileVersion != want.ProfileVersion || got.CodecContractVersion != want.CodecContractVersion ||
		got.Endpoint != want.Endpoint || got.UnitID != want.UnitID ||
		!got.LocalReceiptTime.Equal(want.LocalReceiptTime) || len(got.Dependencies) != len(want.Dependencies) {
		t.Fatalf("activation did not preserve exact observation provenance: %#v", got)
	}
}

func sunSpecWords(headers ...uint16) []uint16 {
	words := []uint16{0x5375, 0x6e53}
	for index := 0; index < len(headers); index += 2 {
		words = append(words, headers[index], headers[index+1])
		if headers[index] != 0xffff {
			words = append(words, make([]uint16, headers[index+1])...)
		}
	}
	return words
}

func modelIDs(models []reg.SunSpecPhaseOneModel) []uint16 {
	ids := make([]uint16, len(models))
	for index, model := range models {
		ids[index] = model.ID()
	}
	return ids
}

func sunSpecObservation(t *testing.T, profile reg.ProfileDescriptor) reg.ObservationSpec {
	t.Helper()
	dependencies := profile.Dependencies().Dependencies()
	results := make([]reg.DependencyResult, 0, len(dependencies))
	for index, dependency := range dependencies {
		record := logicalViewRecord(uint64(900+index), dependency.Normalization().ResolvedPDUOffset(), 0, make([]uint16, dependency.WordCount()))
		record.RequestedFunction, record.ReceivedFunction = reg.FunctionReadHoldingRegisters, reg.FunctionReadHoldingRegisters
		record.Table, record.PhysicalOffset, record.PhysicalWordCount = reg.HoldingRegisters, record.LogicalOffset, record.LogicalWordCount
		record.Endpoint, record.UnitID, record.PollGeneration = "fixture:transport-neutral", 7, 77
		view := snapshotFromRecord(t, record)
		result, err := reg.NewDependencyResult(reg.DependencyResult{DependencyID: dependency.ID(), DependencyVersion: dependency.Version(), CodecID: dependency.CodecID(), CodecVersion: dependency.CodecVersion(), NormalizationVersion: dependency.Normalization().Spec().Version, Status: reg.DependencyReadSuccessful, View: view, SourceTime: reg.SourceTimeUnavailable()})
		if err != nil {
			t.Fatalf("NewDependencyResult: %v", err)
		}
		results = append(results, result)
	}
	return reg.ObservationSpec{SchemaVersion: version(t, "1.0.0"), RuntimeContractVersion: profile.RuntimeContractVersion(), ProfileID: profile.ID(), ProfileVersion: profile.Version(), CodecContractVersion: profile.CodecContractVersion(), DetectorVersion: profile.DetectorVersion(), NormalizationVersion: profile.NormalizationVersion(), CoherenceVersion: profile.CoherenceVersion(), QualificationVersion: profile.QualificationVersion(), SampleID: "sunspec-sample-102", PollGenerationID: 77, DependencySetID: profile.Dependencies().ID(), DependencySetVersion: profile.Dependencies().Version(), SourceValidity: reg.SourceValid, SourceTime: reg.SourceTimeUnavailable(), LocalReceiptTime: time.Unix(1_700_000_000, 0).UTC(), Endpoint: "fixture:transport-neutral", UnitID: 7, Dependencies: results}
}
