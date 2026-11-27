package pv

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The registry's status dictionaries name one status code differently on each
// card. A code classified under one spelling publishes the same operating,
// derating and fault outcome under every other spelling of that code.
func TestEveryNameOfAClassifiedStatusCodeHasTheSameRow(t *testing.T) {
	raw, err := os.ReadFile("../../profiles/vendors/huawei/catalog/dictionaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Dictionaries []struct {
			Kind    string `json:"kind"`
			Offsets []int  `json:"offsets"`
			Values  []struct {
				Code    string `json:"code"`
				Meaning string `json:"meaning"`
			} `json:"values"`
		} `json:"dictionaries"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	names := map[string]map[string]bool{}
	for _, d := range file.Dictionaries {
		if d.Kind != "status_enum" && d.Kind != "value_enum" {
			continue
		}
		at := false
		for _, o := range d.Offsets {
			at = at || o == 32089
		}
		if !at {
			continue
		}
		for _, v := range d.Values {
			// The HD card names the waiting codes "idle_*"; the ticket maps idle
			// names to idle, so those names are classified by their own name.
			if strings.HasPrefix(v.Meaning, "idle_") {
				continue
			}
			code := strings.ToLower(v.Code)
			if names[code] == nil {
				names[code] = map[string]bool{}
			}
			names[code][v.Meaning] = true
		}
	}
	if len(names) == 0 {
		t.Fatal("no status dictionary read")
	}
	for code, spellings := range names {
		var want *huaweiState
		for name := range spellings {
			if st, ok := huaweiStates[name]; ok {
				if want != nil && *want != st {
					t.Fatalf("code %s: spellings disagree (%s)", code, name)
				}
				st := st
				want = &st
			}
		}
		if want == nil {
			continue
		}
		for name := range spellings {
			got, ok := huaweiStates[name]
			if !ok {
				t.Errorf("code %s: the spelling %q has no row", code, name)
			} else if got != *want {
				t.Errorf("code %s: %q is %+v, another spelling is %+v", code, name, got, *want)
			}
		}
	}
	for _, code := range []string{"0x0307", "0x0309", "0x030a"} {
		for name := range names[code] {
			if _, ok := huaweiStates[name]; !ok {
				t.Errorf("shutdown code %s: %q has no row", code, name)
			}
		}
	}
}

func TestTheOtherCardSpellingsPublishTheSameOutcome(t *testing.T) {
	for _, tc := range []struct {
		state, operating, derating, fault string
	}{
		{"grid_operation", "generating", "clear", "clear"},
		{"grid_power_limited", "generating", "active", "clear"},
		{"grid_derated", "generating", "active", "clear"},
		{"shutdown_fault", "idle", "clear", "present"},
		{"standby_irradiation_check", "standby", "clear", "clear"},
		{"standby_no_irradiation", "standby", "clear", "clear"},
		{"on_grid_off_gridmode_run", "generating", "clear", "clear"},
		{"stopped_onfault", "idle", "clear", "present"},
		{"rapid_shutdown", "idle", "clear", "clear"},
		{"ns_protection_shutdown", "idle", "clear", "present"},
		{"command_rapid_shutdown", "idle", "clear", "clear"},
	} {
		pub := childOf(t, fact("huawei.sun2000.mc.status", "", tc.state, 512, 0))
		inv := string(pub.node(t, kindInverter, "instance.synthetic/1#1").ID)
		requireSymbol(t, pub, "pv.status.operating", inv, tc.operating)
		requireSymbol(t, pub, "pv.status.derating", inv, tc.derating)
		requireSymbol(t, pub, "pv.status.fault", inv, tc.fault)
	}
}
