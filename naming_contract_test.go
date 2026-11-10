package modbusreg

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNamingStatusHasNoAlternateRuntimeKey(t *testing.T) {
	p := h05Profile(t, HuaweiSUN2000M1MonitoringProfileID)
	fixture, _ := h05Load(t, h05FixtureFor(p.ID()))
	r := h05Decode(t, p, 1, h05GroupReadings(p, fixture.Readings[0], 7))
	fact := h05FactValue(t, r, "huawei.sun2000.m1.status", 0)
	if fact.State != "grid_connected" || fact.Value.Coefficient != 512 || fact.Value.Exponent != 0 || fact.Unit != "" || fact.Grade != HuaweiGradeHypothesis || fact.Availability != HuaweiFactValue || fact.Cycle != 7 || fact.Retained {
		t.Fatalf("status changed: %+v", fact)
	}
	// Each lookup axis is changed independently: namespace, field and index.
	// These catch suffix-only lookup, old-alias lookup and omitted-index lookup.
	for _, key := range []string{"huawei.sun2000.m3.status", "huawei.sun2000.m1.device_status", "huawei.sun2000.m1.energy_total"} {
		if _, ok := r.Fact(key, 0); ok {
			t.Fatalf("unexpected key %q", key)
		}
	}
	if _, ok := r.Fact(fact.Key, 1); ok {
		t.Fatal("index collision")
	}
	for _, key := range p.DecoderKeys() {
		if strings.HasSuffix(key, ".device_status") || strings.HasSuffix(key, ".energy_total") {
			t.Fatalf("alternate key %q", key)
		}
	}
	if _, ok := r.Fact("huawei.sun2000.m1.energy_accumulated", 0); !ok {
		t.Fatal("provisional energy key changed")
	}
}

func TestNamingStatusRetentionFencesEachReadingIdentityAxis(t *testing.T) {
	p := h05Profile(t, HuaweiSUN2000M1MonitoringProfileID)
	fixture, _ := h05Load(t, h05FixtureFor(p.ID()))
	previous := h05Decode(t, p, 1, h05GroupReadings(p, fixture.Readings[0], 7))
	groups := h05GroupReadings(p, fixture.Readings[0], 8)
	for i := range groups {
		if groups[i].Offset == 32064 {
			groups[i] = HuaweiSUN2000GroupReading{Offset: 32064, Quantity: 32, Outcome: HuaweiSUN2000GroupFailed}
		}
		if groups[i].Offset == 32016 {
			groups[i].Words[0] = 6000
		}
	}
	current := h05Decode(t, p, 1, groups)
	before := h05FactValue(t, previous, "huawei.sun2000.m1.status", 0)
	kept := h05FactValue(t, RetainHuaweiSUN2000Monitoring(previous, current), before.Key, 0)
	before.Retained = true
	if !reflect.DeepEqual(before, kept) {
		t.Fatalf("status retention changed: %+v", kept)
	}
	voltage := h05FactValue(t, RetainHuaweiSUN2000Monitoring(previous, current), "huawei.sun2000.m1.pv_string_voltage", 1)
	if voltage.Value.String() != "600.0" || voltage.Cycle != 8 || voltage.Retained {
		t.Fatalf("fresh sibling changed: %+v", voltage)
	}
	// profileID and unitID are the two retention identity axes. Independent
	// mutations catch removal of either equality check in retention admission.
	for name, prior := range map[string]HuaweiSUN2000MonitoringReading{"profile_id": previous, "unit_id": previous} {
		t.Run(name, func(t *testing.T) {
			if name == "profile_id" {
				prior.profileID = HuaweiSUN2000M3MonitoringProfileID
			} else {
				prior.unitID = 2
			}
			got := h05FactValue(t, RetainHuaweiSUN2000Monitoring(prior, current), before.Key, 0)
			if got.Availability != HuaweiFactUnknown || got.Retained {
				t.Fatalf("identity collision retained status: %+v", got)
			}
		})
	}
}

func TestNamingInventoryMatchesEveryBoundedEmittedField(t *testing.T) {
	data, err := os.ReadFile("naming-inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Rows []struct {
			Current string `json:"current_name"`
			Target  string `json:"target"`
			Status  string `json:"status"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"pv_string_voltage": "provisional", "pv_string_current": "provisional", "energy_accumulated": "provisional", "energy_current_hour": "provisional", "energy_current_day": "provisional", "input_power": "provisional", "active_power": "provisional", "status": "adopted"}
	if len(inventory.Rows) != len(expected) {
		t.Fatal("bounded inventory row count changed")
	}
	seen := map[string]bool{}
	for _, row := range inventory.Rows {
		if seen[row.Current] || expected[row.Current] != row.Status || row.Target != row.Current {
			t.Fatalf("undefined or wrong inventory row: %+v", row)
		}
		seen[row.Current] = true
	}
	emitted := map[string]bool{}
	for _, p := range HuaweiSUN2000MonitoringProfiles() {
		for _, field := range p.fields {
			if !seen[field.name] {
				t.Fatalf("unclassified field %s", field.name)
			}
			emitted[field.name] = true
		}
		if p.CatalogRegistered() || p.AutomaticRuntimeAdmission() || p.LiveQualified() || p.SupportClaim() || p.WriteAuthority() || p.BatteryFactsAdmitted() || p.MeterFactsAdmitted() {
			t.Fatal("naming change granted authority")
		}
	}
	if !reflect.DeepEqual(seen, emitted) {
		t.Fatal("inventory and emitted shape differ")
	}
}
