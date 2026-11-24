package huawei

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Catalogue profile tests vary the card, the kept hand identifier, the string
// count, the power-group offset and quantity, the direct-current counter, the
// status code and its card dictionary, the signed active-power coefficient,
// and the refused group's unit, offset and quantity.
//
// A production mutation this file catches:
//   - a missing card, or one profile reused for every card
//   - a renamed hand identifier, or a hand family bound to the other card
//   - a fixed string count, or channel numbers packed densely
//   - a power quantity other than 33 where the block starts at 32064
//   - a direct-current counter omitted, or invented on a card whose interval is a hole
//   - only the observed status code published, or one card's dictionary used for another
//   - a negative active power withheld
//   - a refused group dropping the sibling groups, or a loss without its unit
//   - status names read from the source tree, or a missing label document published as empty names

func TestCatalogCardProfilesCoverEveryCard(t *testing.T) {
	wantStrings := map[string]int{
		"COMMERCIAL": 3, "HA": 28, "HB": 0, "HD": 36,
		"L-LEGACY": 4, "L-V200": 4, "LB-C00": 4, "LB-C10": 4, "LC": 4,
		"MA-C00": 4, "MA-C20": 8, "MA-V200": 10, "MB02": 6, "MC": 8,
		"SUN-C81": 8, "SUN-V300": 8,
	}
	profiles, err := CatalogMonitoringProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != len(wantStrings) {
		t.Fatalf("profiles %d", len(profiles))
	}
	seen := map[string]bool{}
	for _, profile := range profiles {
		card := profile.Card()
		want, ok := wantStrings[card]
		if !ok || seen[card] {
			t.Fatalf("card %q repeated or unknown", card)
		}
		seen[card] = true
		if profile.StringCount() != want {
			t.Fatalf("%s string count %d", card, profile.StringCount())
		}
		if profile.ID() == "" || profile.Family() == "" || len(profile.Plan(1)) == 0 {
			t.Fatalf("%s profile is empty", card)
		}
	}
}

func TestCatalogHandFamiliesKeepIdentifiers(t *testing.T) {
	series := catalogProfileByCard(t, "MC")
	compact := catalogProfileByCard(t, "MA-C00")
	if series.ID() != SUN2000M3MonitoringProfileID || series.Family() != huaweiSUN2000M3Family {
		t.Fatalf("series id %s family %s", series.ID(), series.Family())
	}
	if compact.ID() != SUN2000M1MonitoringProfileID || compact.Family() != huaweiSUN2000M1Family {
		t.Fatalf("compact id %s family %s", compact.ID(), compact.Family())
	}
	if series.ID() == compact.ID() || series.StringCount() == compact.StringCount() {
		t.Fatal("hand families collapsed")
	}
	other := catalogProfileByCard(t, "SUN-V300")
	if other.ID() == series.ID() || other.ID() == compact.ID() {
		t.Fatal("another card took a hand identifier")
	}
	if other.ID() != "huawei.sun2000.sun-v300.monitoring-readonly@1.0.0" || other.Family() != "SUN2000 SUN-V300" {
		t.Fatalf("other id %s family %s", other.ID(), other.Family())
	}
}

func TestCatalogPowerGroupDecodesSignedQuantities(t *testing.T) {
	for _, card := range []string{"MC", "MA-C00", "COMMERCIAL", "SUN-V300"} {
		card := card
		t.Run(card, func(t *testing.T) {
			profile := catalogProfileByCard(t, card)
			step := catalogStepAt(t, profile, 32064)
			if step.Offset != 32064 || step.Quantity != 33 {
				t.Fatalf("power step %d/%d", step.Offset, step.Quantity)
			}
		})
	}
	without := catalogProfileByCard(t, "HB")
	for _, step := range without.Plan(1) {
		if step.Offset == 32064 {
			t.Fatalf("card without that word still plans %d/%d", step.Offset, step.Quantity)
		}
	}
	series := catalogProfileByCard(t, "MC")
	compact := catalogProfileByCard(t, "MA-C00")
	seriesReading := catalogDecodePower(t, series, 4, -1500, 0x0100)
	compactReading := catalogDecodePower(t, compact, 9, -2750, 0x0203)
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "reactive_power", 0, -3200, -3, "kVar")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "line_voltage_ab", 0, 4001, -1, "V")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "phase_voltage_a", 0, 2302, -1, "V")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "phase_current_a", 0, 1503, -3, "A")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "frequency", 0, 5004, -2, "Hz")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "power_factor", 0, -805, -3, "")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "efficiency", 0, 9856, -2, "%")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "internal_temperature", 0, -17, -1, "°C")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "insulation_resistance", 0, 2007, -3, "MΩ")
	catalogWantSigned(t, seriesReading, series.IDPrefix(), "active_power", 0, -1500, -3, "kW")
	catalogWantSigned(t, compactReading, compact.IDPrefix(), "active_power", 0, -2750, -3, "kW")
	catalogWantSigned(t, compactReading, compact.IDPrefix(), "reactive_power", 0, -3200, -3, "kVar")
	if got, ok := seriesReading.Fact(series.IDPrefix()+"rated_power", 0); !ok || got.Availability != FactValue || got.Value.Coefficient != 50000 || got.Value.Exponent != -3 {
		t.Fatalf("rating %+v %v", got, ok)
	}
	if _, ok := compactReading.Fact(compact.IDPrefix()+"mppt_power", 1); ok {
		t.Fatal("compact card invented tracker power")
	}
	if _, ok := seriesReading.Fact(series.IDPrefix()+"mppt_energy", 1); !ok {
		t.Fatal("series card dropped tracker energy")
	}
}

func TestCatalogDirectCurrentCounterFollowsTheCard(t *testing.T) {
	for _, card := range []string{"MC", "MA-C00", "MA-V200"} {
		profile := catalogProfileByCard(t, card)
		step := catalogStepAt(t, profile, 32108)
		if card != "MA-V200" && (step.Offset != 32106 || step.Quantity != 14) {
			t.Fatalf("%s energy step %d/%d", card, step.Offset, step.Quantity)
		}
		words := make([]uint16, step.Quantity)
		catalogPutWide(words, step.Offset, 32108, 23456)
		reading, err := profile.Decode(3, []SUN2000GroupReading{catalogAnswered(step, words)})
		if err != nil {
			t.Fatal(err)
		}
		fact, ok := reading.Fact(profile.IDPrefix()+"energy_dc_input", 0)
		if !ok || fact.Availability != FactValue || fact.Value != (Decimal{Coefficient: 23456, Exponent: -2}) || fact.Unit != "kWh" || fact.Grade != GradeHypothesis {
			t.Fatalf("%s counter %+v %v", card, fact, ok)
		}
	}
	commercial := catalogProfileByCard(t, "COMMERCIAL")
	reading, err := commercial.Decode(3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reading.Fact(commercial.IDPrefix()+"energy_dc_input", 0); ok {
		t.Fatal("hole interval became a field")
	}
}

func TestCatalogStatusNamesComeFromTheCard(t *testing.T) {
	seriesProfile := catalogProfileByCard(t, "MC")
	series := catalogDecodePower(t, seriesProfile, 4, 10, 0x0100)
	if fact, ok := series.Fact(seriesProfile.IDPrefix()+"status", 0); !ok || fact.State != "starting" || fact.Availability != FactValue || fact.Value.Coefficient != 0x0100 {
		t.Fatalf("series starting %+v %v", fact, ok)
	}
	series = catalogDecodePower(t, seriesProfile, 4, 10, 0x0200)
	if fact, _ := series.Fact(seriesProfile.IDPrefix()+"status", 0); fact.State != "grid_operation" {
		t.Fatalf("series connected name %q", fact.State)
	}
	series = catalogDecodePower(t, seriesProfile, 4, 10, 0x0203)
	if fact, ok := series.Fact(seriesProfile.IDPrefix()+"status", 0); !ok || fact.Availability != FactUnknown || fact.State != "" {
		t.Fatalf("unlisted series code %+v %v", fact, ok)
	}
	compactProfile := catalogProfileByCard(t, "MA-C00")
	compact := catalogDecodePower(t, compactProfile, 9, 10, 0x0200)
	if fact, _ := compact.Fact(compactProfile.IDPrefix()+"status", 0); fact.State != "running_mode_dependent" {
		t.Fatalf("compact running name %q", fact.State)
	}
	compact = catalogDecodePower(t, compactProfile, 9, 10, 0x0203)
	if fact, _ := compact.Fact(compactProfile.IDPrefix()+"status", 0); fact.State != "off_grid_running" || fact.Availability != FactValue {
		t.Fatalf("compact off-grid %+v", fact)
	}
	compact = catalogDecodePower(t, compactProfile, 9, 10, 0x0100)
	if fact, _ := compact.Fact(compactProfile.IDPrefix()+"status", 0); fact.State != "starting" {
		t.Fatalf("compact starting %q", fact.State)
	}
}

func TestCatalogRefusedGroupLeavesSiblingFacts(t *testing.T) {
	profile := catalogProfileByCard(t, "MC")
	power := catalogStepAt(t, profile, 32064)
	energy := catalogStepAt(t, profile, 32106)
	words := make([]uint16, energy.Quantity)
	catalogPutWide(words, energy.Offset, 32106, 12345)
	reading, err := profile.Decode(5, []SUN2000GroupReading{
		{Offset: power.Offset, Quantity: power.Quantity, Outcome: SUN2000GroupException, ExceptionCode: huaweiSUN2000RefusalCode},
		catalogAnswered(energy, words),
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogWantLoss(t, reading.Losses(), 5, power.Offset, power.Quantity)
	if fact, ok := reading.Fact(profile.IDPrefix()+"energy_accumulated", 0); !ok || fact.Availability != FactValue || fact.Value.Coefficient != 12345 {
		t.Fatalf("sibling energy %+v %v", fact, ok)
	}
	if fact, ok := reading.Fact(profile.IDPrefix()+"active_power", 0); !ok || fact.Availability != FactUnknown || fact.Value != (Decimal{}) {
		t.Fatalf("refused power %+v %v", fact, ok)
	}
	strings := catalogStepAt(t, profile, 32016)
	powerWords := make([]uint16, power.Quantity)
	catalogPutWide(powerWords, power.Offset, 32080, 1800)
	other, err := profile.Decode(6, []SUN2000GroupReading{
		{Offset: strings.Offset, Quantity: strings.Quantity, Outcome: SUN2000GroupException, ExceptionCode: huaweiSUN2000RefusalCode},
		catalogAnswered(power, powerWords),
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogWantLoss(t, other.Losses(), 6, strings.Offset, strings.Quantity)
	if fact, ok := other.Fact(profile.IDPrefix()+"active_power", 0); !ok || fact.Availability != FactValue || fact.Value.Coefficient != 1800 {
		t.Fatalf("sibling power %+v %v", fact, ok)
	}
	if fact, ok := other.Fact(profile.IDPrefix()+"pv_string_voltage", 1); !ok || fact.Availability != FactUnknown {
		t.Fatalf("refused string %+v %v", fact, ok)
	}
}

func TestCatalogStringChannelKeepsItsNumber(t *testing.T) {
	profile := catalogProfileByCard(t, "COMMERCIAL")
	if profile.StringCount() != 3 {
		t.Fatalf("count %d", profile.StringCount())
	}
	step := catalogStepAt(t, profile, 32062)
	words := make([]uint16, step.Quantity)
	catalogPutWord(words, step.Offset, 32062, 321)
	reading, err := profile.Decode(2, []SUN2000GroupReading{catalogAnswered(step, words)})
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := reading.Fact(profile.IDPrefix()+"pv_string_voltage", 24)
	if !ok || fact.Availability != FactValue || fact.Value.Coefficient != 321 || fact.Index != 24 {
		t.Fatalf("channel %+v %v", fact, ok)
	}
	if _, packed := reading.Fact(profile.IDPrefix()+"pv_string_voltage", 3); packed {
		t.Fatal("channel number was packed")
	}
}

func TestCatalogAlarmBitWithoutALabelStaysNative(t *testing.T) {
	profile := catalogProfileByCard(t, "COMMERCIAL")
	step := catalogStepAt(t, profile, 32009)
	words := make([]uint16, step.Quantity)
	catalogPutWord(words, step.Offset, 32009, 1<<8|1)
	reading, err := profile.Decode(8, []SUN2000GroupReading{catalogAnswered(step, words)})
	if err != nil {
		t.Fatal(err)
	}
	named, ok := reading.Fact(profile.IDPrefix()+"alarm.faultleakageamps", 0)
	if !ok || named.Availability != FactValue || named.State != "alarm.faultleakageamps" || named.Grade != GradeHypothesis {
		t.Fatalf("named bit %+v %v", named, ok)
	}
	native, ok := reading.Fact(profile.IDPrefix()+"bit_32009_8", 8)
	if !ok || native.Availability != FactValue || native.Grade != GradeUnknown || native.State != "" {
		t.Fatalf("native bit %+v %v", native, ok)
	}
}

func TestCatalogLabelsAreCompiledIn(t *testing.T) {
	if _, _, err := catalogDictionaries(nil); err == nil {
		t.Fatal("missing label document returned empty maps")
	}
	if _, _, err := catalogDictionaries([]byte("{")); err == nil {
		t.Fatal("unreadable label document returned empty maps")
	}
	saved := catalogLabelDocument
	catalogLabelDocument = nil
	t.Cleanup(func() { catalogLabelDocument = saved })
	missing := catalogLoad()
	if missing.labelErr == nil || len(missing.profiles) != 0 {
		t.Fatal("a missing label document still built profiles")
	}
	catalogLabelDocument = []byte("{")
	broken := catalogLoad()
	if broken.labelErr == nil || len(broken.profiles) != 0 {
		t.Fatal("an unreadable label document still built profiles")
	}
	catalogLabelDocument = saved
	data, err := os.ReadFile(filepath.Join("..", "profiles", "vendors", "huawei", "catalog", "dictionaries.json"))
	if err != nil {
		t.Fatal(err)
	}
	fromFile, fileAlarms, err := catalogDictionaries(data)
	if err != nil {
		t.Fatal(err)
	}
	fromModule, moduleAlarms, err := catalogDictionaries(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromFile, fromModule) || !reflect.DeepEqual(fileAlarms, moduleAlarms) {
		t.Fatal("compiled labels differ from the catalogue document")
	}
	if fromModule["MC"][0x0100] != "starting" {
		t.Fatalf("compiled starting name %q", fromModule["MC"][0x0100])
	}
}

func TestCatalogProfileLossDropsFilledHoles(t *testing.T) {
	for _, card := range []string{"MC", "MA-C00"} {
		profile := catalogProfileByCard(t, card)
		for _, loss := range CatalogProfileLosses(profile) {
			if loss.Offset == 32108 || loss.Kind == ProviderLossUnconsumedWords {
				t.Fatalf("%s still lists %+v", card, loss)
			}
		}
		var battery, meter bool
		for _, loss := range CatalogProfileLosses(profile) {
			if loss.Kind == ProviderLossBattery && loss.Detail == "battery offsets are never read; a zero word is never a battery value" {
				battery = true
			}
			if loss.Kind == ProviderLossMeter && loss.Detail == "every meter value stays Unknown; no meter class contract" {
				meter = true
			}
		}
		if !battery || !meter {
			t.Fatalf("%s battery %v meter %v", card, battery, meter)
		}
	}
	for _, profile := range SUN2000MonitoringProfiles() {
		var hole bool
		for _, loss := range huaweiChildProfileLoss(profile) {
			if loss.Offset == 32108 && loss.Quantity == 4 && loss.Kind == ProviderLossUnconsumedWords {
				hole = true
			}
		}
		if !hole {
			t.Fatalf("%s hand loss dropped the hole", profile.ID())
		}
	}
}

func catalogProfileByCard(t *testing.T, card string) CatalogMonitoringProfile {
	t.Helper()
	profiles, err := CatalogMonitoringProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		if profile.Card() == card {
			return profile
		}
	}
	t.Fatalf("no profile for %s", card)
	return CatalogMonitoringProfile{}
}

func catalogStepAt(t *testing.T, profile CatalogMonitoringProfile, offset uint16) SUN2000ReadStep {
	t.Helper()
	for _, step := range profile.Plan(1) {
		if step.Offset <= offset && offset < step.Offset+step.Quantity {
			return step
		}
	}
	t.Fatalf("%s has no step covering %d", profile.Card(), offset)
	return SUN2000ReadStep{}
}

func catalogAnswered(step SUN2000ReadStep, words []uint16) SUN2000GroupReading {
	return SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Outcome: SUN2000GroupAnswered, Words: words}
}

func catalogPutWord(words []uint16, group, field, value uint16) {
	words[field-group] = value
}

func catalogSignedWord(value int16) uint16 {
	return uint16(value)
}

func catalogPutWide(words []uint16, group, field uint16, value int32) {
	raw := uint32(value)
	words[field-group] = uint16(raw >> 16)
	words[field-group+1] = uint16(raw)
}

func catalogDecodePower(t *testing.T, profile CatalogMonitoringProfile, unit uint8, active int32, status uint16) CatalogMonitoringReading {
	t.Helper()
	power := catalogStepAt(t, profile, 32064)
	words := make([]uint16, power.Quantity)
	catalogPutWide(words, power.Offset, 32064, 1100)
	catalogPutWord(words, power.Offset, 32066, 4001)
	catalogPutWord(words, power.Offset, 32069, 2302)
	catalogPutWide(words, power.Offset, 32072, 1503)
	catalogPutWide(words, power.Offset, 32080, active)
	catalogPutWide(words, power.Offset, 32082, -3200)
	catalogPutWord(words, power.Offset, 32084, catalogSignedWord(-805))
	catalogPutWord(words, power.Offset, 32085, 5004)
	catalogPutWord(words, power.Offset, 32086, 9856)
	catalogPutWord(words, power.Offset, 32087, catalogSignedWord(-17))
	catalogPutWord(words, power.Offset, 32088, 2007)
	catalogPutWord(words, power.Offset, 32089, status)
	ratings := catalogStepAt(t, profile, 30073)
	ratingWords := make([]uint16, ratings.Quantity)
	catalogPutWide(ratingWords, ratings.Offset, 30073, 50000)
	var tracker SUN2000GroupReading
	var hasTracker bool
	if step, ok := catalogOptionalStep(profile, 32212); ok {
		trackerWords := make([]uint16, step.Quantity)
		catalogPutWide(trackerWords, step.Offset, 32212, 42)
		tracker = catalogAnswered(step, trackerWords)
		hasTracker = true
	}
	readings := []SUN2000GroupReading{catalogAnswered(power, words), catalogAnswered(ratings, ratingWords)}
	if hasTracker {
		readings = append(readings, tracker)
	}
	reading, err := profile.Decode(unit, readings)
	if err != nil {
		t.Fatal(err)
	}
	return reading
}

func catalogOptionalStep(profile CatalogMonitoringProfile, offset uint16) (SUN2000ReadStep, bool) {
	for _, step := range profile.Plan(1) {
		if step.Offset <= offset && offset < step.Offset+step.Quantity {
			return step, true
		}
	}
	return SUN2000ReadStep{}, false
}

func catalogWantSigned(t *testing.T, reading CatalogMonitoringReading, prefix, name string, index int, coefficient int64, exponent int16, unit string) {
	t.Helper()
	fact, ok := reading.Fact(prefix+name, index)
	if !ok || fact.Availability != FactValue || fact.Grade != GradeHypothesis || fact.Value != (Decimal{Coefficient: coefficient, Exponent: exponent}) || fact.Unit != unit {
		t.Fatalf("%s %+v %v", name, fact, ok)
	}
}

func catalogWantLoss(t *testing.T, losses []ProviderLoss, unit uint8, offset, quantity uint16) {
	t.Helper()
	want := ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: unit, Offset: offset, Quantity: quantity,
		Detail: "refused with exception 3 at runtime; not sent again"}
	for _, loss := range losses {
		if loss == want {
			return
		}
	}
	t.Fatalf("losses %+v", losses)
}
