package growatt

import (
	"context"
	"errors"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/profiles"
)

func TestLookupProtocolIIProviderDecodesConfiguredFamily(t *testing.T) {
	entry := profiles.GrowattProtocolIICatalog()
	provider, ok := LookupProtocolIIProvider([]ProtocolIICatalogEntry{growattCatalogEntry(entry)}, entry.RuntimeIdentity)
	if !ok || provider.RuntimeIdentity() != entry.RuntimeIdentity {
		t.Fatal("catalog lookup did not construct the provider")
	}
	admission, err := provider.Admit(ProtocolIIAdmissionInput{ConfiguredFamily: "MID", ConfiguredUnit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !admission.Configured() || admission.Family() != "MID" || admission.ExecutionUnit() != 4 || admission.IdentityEvidence() || admission.AutomaticQualification() != "unknown" {
		t.Fatalf("admission=%#v", admission)
	}
	if _, ok := admission.ObservedDeviceType(); ok {
		t.Fatal("configured admission fabricated a device type")
	}
	words := make([]uint16, protocolIIInputWidth)
	words[0] = 1
	words[3], words[4], words[5], words[6] = 10, 20, 0, 30
	words[33], words[34] = 1, 0
	words[93] = 250
	words[113] = 40
	words[125] = 500
	telemetry, err := provider.Decode(ProtocolIIIdentityObservation{}, admission, ProtocolIIFC04Slice{Words: words})
	if err != nil {
		t.Fatal(err)
	}
	if telemetry.Identity().UnitID() != 0 || telemetry.PVInputs[0].VoltageVolts != 1 || telemetry.PVInputs[0].CurrentAmps != 2 || telemetry.PVInputs[0].PowerWatts != 3 || telemetry.PVInputs[7].PowerWatts != 6553.6 || telemetry.InverterTemperatureCelsius != 25 {
		t.Fatalf("telemetry=%#v", telemetry)
	}
	if _, ok := telemetry.Reading("real_output_power_percent", 113); ok {
		t.Fatal("MID typed word 113")
	}
	raw, ok := telemetry.RawWord(113)
	if !ok || raw.Word != 40 {
		t.Fatalf("word 113 raw=%#v", raw)
	}
	for _, offset := range []uint16{108, 109, 125, 127, 129, 131, 133, 135, 137, 139} {
		word, ok := telemetry.RawWord(offset)
		if !ok || (offset == 125 && word.Word != 500) {
			t.Fatalf("offset %d raw=%#v ok=%t", offset, word, ok)
		}
		if _, typed := telemetry.Reading("pv_positive_to_earth_voltage", offset); typed {
			t.Fatalf("offset %d typed PID", offset)
		}
		if _, typed := telemetry.Reading("flyspan_voltage", offset); typed {
			t.Fatalf("offset %d typed flyspan", offset)
		}
	}
}

func TestLookupProtocolIIProviderRejectsEachCatalogField(t *testing.T) {
	base := growattCatalogEntry(profiles.GrowattProtocolIICatalog())
	if _, ok := LookupProtocolIIProvider([]ProtocolIICatalogEntry{base}, "other"); ok {
		t.Fatal("identity mismatch")
	}
	for name, mutate := range map[string]func(*ProtocolIICatalogEntry){
		"runtime identity": func(entry *ProtocolIICatalogEntry) { entry.RuntimeIdentity = "" },
		"schema":           func(entry *ProtocolIICatalogEntry) { entry.Schema = "other" },
		"family order":     func(entry *ProtocolIICatalogEntry) { entry.Families = []string{"MID", "MAX", "MAC"} },
		"family count":     func(entry *ProtocolIICatalogEntry) { entry.Families = []string{"MAX", "MID"} },
		"function":         func(entry *ProtocolIICatalogEntry) { entry.Function = 3 },
		"offset":           func(entry *ProtocolIICatalogEntry) { entry.Offset = 1 },
		"width":            func(entry *ProtocolIICatalogEntry) { entry.WidthWords = 59 },
	} {
		t.Run(name, func(t *testing.T) {
			entry := base
			entry.Families = append([]string(nil), base.Families...)
			mutate(&entry)
			if _, ok := LookupProtocolIIProvider([]ProtocolIICatalogEntry{entry}, entry.RuntimeIdentity); ok {
				t.Fatal("mutated catalog entry constructed a provider")
			}
		})
	}
	var absent ProtocolIIProvider
	if _, err := absent.Decode(ProtocolIIIdentityObservation{}, ProtocolIIFC04Applicability{}, ProtocolIIFC04Slice{}); err == nil {
		t.Fatal("zero provider decoded")
	}
	words := make([]uint16, protocolIIInputWidth)
	words[0] = 1
	provider, _ := LookupProtocolIIProvider([]ProtocolIICatalogEntry{base}, base.RuntimeIdentity)
	if telemetry, err := provider.Decode(ProtocolIIIdentityObservation{}, ProtocolIIFC04Applicability{}, ProtocolIIFC04Slice{Words: words}); err == nil || telemetry.Identity().UnitID() != 0 || len(telemetry.RawSlice().Words) != 0 {
		t.Fatal("zero applicability decoded")
	}
}

func TestProtocolIIConfiguredAndAutomaticAdmission(t *testing.T) {
	identity, err := DecodeGrowattProtocolIIIdentity(validGrowattProtocolIIIdentityInput())
	if err != nil {
		t.Fatal(err)
	}
	input := ProtocolIIAdmissionInput{ConfiguredFamily: "MAX", ConfiguredUnit: identity.UnitID(), Identity: identity}
	admission, err := NewProtocolIIFC04Applicability(input)
	if err != nil {
		t.Fatal(err)
	}
	device, ok := admission.ObservedDeviceType()
	version, versionOK := admission.ObservedProtocolVersion()
	build, buildOK := admission.ObservedModelBuild()
	if !admission.IdentityEvidence() || !ok || device != identity.DeviceType() || !versionOK || version != identity.ProtocolVersion() || !buildOK || build != identity.ModelBuild() || admission.AutomaticQualification() != "unknown" {
		t.Fatalf("admission did not retain raw identity: %#v", admission)
	}
	automatic := identity
	automatic.protocolVersion = 207
	if _, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{Identity: automatic}); !errors.Is(err, ErrGrowattNoAdmissibleProfile) {
		t.Fatal(err)
	}
	for _, family := range []string{"SPA", "STORAGE_POWER", "MAX_HV"} {
		if _, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: family, ConfiguredUnit: 1}); !errors.Is(err, ErrGrowattNoAdmissibleProfile) {
			t.Fatalf("family %s err=%v", family, err)
		}
	}
	if _, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAX", ConfiguredUnit: 1, ContraryEvidence: true}); !errors.Is(err, ErrGrowattNoAdmissibleProfile) {
		t.Fatal(err)
	}
	other := validGrowattProtocolIIIdentityInput()
	other.Profile.Family = "MID"
	otherIdentity, err := DecodeGrowattProtocolIIIdentity(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAX", ConfiguredUnit: otherIdentity.UnitID(), Identity: otherIdentity}); !errors.Is(err, ErrGrowattNoAdmissibleProfile) {
		t.Fatal(err)
	}
}

func TestProtocolIIWordSelectionAndPV(t *testing.T) {
	maxAdmission, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAX", ConfiguredUnit: 3})
	if err != nil {
		t.Fatal(err)
	}
	midAdmission, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MID", ConfiguredUnit: 3})
	if err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, protocolIIInputWidth)
	words[0] = 1
	words[3], words[4], words[5], words[6] = 331, 12, 0, 50
	words[31], words[32], words[33], words[34] = 1, 2, 0x0001, 0x0002
	words[93] = 501
	words[112], words[113] = 0, 500
	words[108], words[109] = 7, 8
	for _, offset := range []uint16{125, 127, 129, 131, 133, 135, 137, 139} {
		words[offset] = 500
	}
	maxTelemetry, err := DecodeGrowattProtocolIIFC04Telemetry(ProtocolIIIdentityObservation{}, maxAdmission, ProtocolIIFC04Slice{Words: words})
	if err != nil {
		t.Fatal(err)
	}
	reading, ok := maxTelemetry.Reading("real_output_power_percent", 113)
	if !ok || reading.Value != 500 || reading.Unit != "percent" {
		t.Fatalf("MAX percent %#v", reading)
	}
	if _, ok := maxTelemetry.Reading("flyspan_voltage", 125); ok || maxTelemetry.RawWordMust(125) != 500 {
		t.Fatal("plain MAX typed flyspan")
	}
	midTelemetry, err := DecodeGrowattProtocolIIFC04Telemetry(ProtocolIIIdentityObservation{}, midAdmission, ProtocolIIFC04Slice{Words: words})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := midTelemetry.Reading("real_output_power_percent", 113); ok || midTelemetry.RawWordMust(113) != 500 {
		t.Fatal("MID typed word 113")
	}
	if midTelemetry.RawWordMust(108) != 7 || midTelemetry.RawWordMust(109) != 8 {
		t.Fatal("SPA words were typed for MID")
	}
	pv := midTelemetry.PVInputs[0]
	if pv.VoltageVolts != 33.1 || pv.CurrentAmps != 1.2 || pv.PowerWatts != 5 || pv.VoltageWord != 331 || pv.PowerWords != [2]uint16{0, 50} {
		t.Fatalf("pv1 %#v", pv)
	}
	eighth := midTelemetry.PVInputs[7]
	if eighth.VoltageVolts != 0.1 || eighth.CurrentAmps != 0.2 || eighth.PowerWatts != 6553.8 || eighth.PowerWords != [2]uint16{1, 2} || midTelemetry.InverterTemperatureCelsius != 50.1 || midTelemetry.InverterTemperatureWord != 501 {
		t.Fatalf("eighth input %#v temp=%v", eighth, midTelemetry.InverterTemperatureCelsius)
	}
	for _, family := range []ProtocolIIFC04Applicability{maxAdmission, midAdmission} {
		mac, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAC", ConfiguredUnit: 3})
		if err != nil {
			t.Fatal(err)
		}
		_ = family
		telemetry, err := DecodeGrowattProtocolIIFC04Telemetry(ProtocolIIIdentityObservation{}, mac, ProtocolIIFC04Slice{Words: words})
		if err != nil || telemetry.RawWordMust(125) != 500 {
			t.Fatal("MAC did not keep word 125 raw")
		}
		if _, ok := telemetry.Reading("pv_positive_to_earth_voltage", 125); ok {
			t.Fatal("MAC typed PID")
		}
	}
	for _, offset := range []uint16{125, 127, 129, 131, 133, 135, 137, 139} {
		pid, ok := growattProtocolIIApplyRow(words, offset, "pv_positive_to_earth_voltage")
		fly, flyOK := growattProtocolIIApplyRow(words, offset, "flyspan_voltage")
		if !ok || !flyOK || pid.Value != 50 || pid.Unit != "V" || fly.Value != 50 || fly.Unit != "V" || fly.Qualifier != "MAX_HV" {
			t.Fatalf("offset %d pid=%#v fly=%#v", offset, pid, fly)
		}
	}
	storage, ok := growattProtocolIIApplyRow(words, 112, "daily_ac_charge_energy")
	if !ok || storage.Value != 50 || storage.Unit != "kWh" {
		t.Fatalf("storage %#v", storage)
	}
	ambiguous, _, _, raw, _ := growattProtocolIIProjectAllow(words, func(row protocolIITableRow) bool {
		return row.qualifier == "MAX" || row.qualifier == "STORAGE_POWER" || row.qualifier == ""
	})
	for _, reading := range ambiguous {
		if reading.Offset == 113 || reading.Name == "daily_ac_charge_energy" {
			t.Fatalf("ambiguous row typed %#v", reading)
		}
	}
	if !rawHas(raw, 113, 500) || !rawHas(raw, 112, 0) {
		t.Fatal("ambiguous selection dropped the raw words")
	}
}

func TestProtocolIIObserverUsesConfiguredUnit(t *testing.T) {
	other := validGrowattProtocolIIIdentityInput()
	other.UnitID = 9
	identity, err := DecodeGrowattProtocolIIIdentity(other)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAC", ConfiguredUnit: 4, Identity: identity})
	if err != nil || admission.IdentityEvidence() {
		t.Fatal(err)
	}
	session := &growattProtocolIIFC04Fake{words: make([]uint16, protocolIIInputWidth)}
	session.words[0] = 1
	observer, err := NewProtocolIIFC04RTUObserver(identity, admission, session)
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(session.calls) != 2 || session.calls[0].unit != 4 || session.calls[1].unit != 4 || telemetry.Identity().UnitID() != 0 {
		t.Fatalf("calls=%#v identity=%d", session.calls, telemetry.Identity().UnitID())
	}
	bare, err := NewProtocolIIFC04Applicability(ProtocolIIAdmissionInput{ConfiguredFamily: "MAC", ConfiguredUnit: 4})
	if err != nil {
		t.Fatal(err)
	}
	observer, err = NewProtocolIIFC04RTUObserver(ProtocolIIIdentityObservation{}, bare, session)
	if err != nil {
		t.Fatal(err)
	}
	session.calls = nil
	if _, err := observer.Observe(context.Background()); err != nil || len(session.calls) != 2 {
		t.Fatalf("calls=%#v err=%v", session.calls, err)
	}
}

func (t ProtocolIIFC04Telemetry) RawWordMust(offset uint16) uint16 {
	word, _ := t.RawWord(offset)
	return word.Word
}

func rawHas(words []ProtocolIITableRawWord, offset, value uint16) bool {
	for _, word := range words {
		if word.Offset == offset && word.Word == value && word.Name == "" {
			return true
		}
	}
	return false
}

func growattCatalogEntry(entry profiles.GrowattProtocolIICatalogEntry) ProtocolIICatalogEntry {
	return ProtocolIICatalogEntry{
		RuntimeIdentity: entry.RuntimeIdentity, Schema: entry.Schema, Families: append([]string(nil), entry.Families...),
		Function: entry.Function, Offset: entry.Offset, WidthWords: entry.WidthWords,
	}
}
