package modbusreg

import (
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"
)

// h06Mutation changes one field of a recorded disposition. field names the
// struct field it covers; production names the mutation of the gate that the
// case catches.
type h06Mutation struct {
	field      string
	production string
	mutate     func(*HuaweiProviderDisposition)
}

func h06DispositionMutations() []h06Mutation {
	return []h06Mutation{
		{"Path", "a gate that ignores the path or admits an unknown path", func(d *HuaweiProviderDisposition) { d.Path = "SmartLoggerX" }},
		{"Path", "a gate that admits the EMMA path with another path's card", func(d *HuaweiProviderDisposition) { d.Path = HuaweiProviderPathEMMA }},
		{"IdentityCard", "a gate that accepts any card", func(d *HuaweiProviderDisposition) { d.IdentityCard = HuaweiEMMAObservedQualificationCardID }},
		{"IdentityStatus", "a gate that skips the identity status", func(d *HuaweiProviderDisposition) { d.IdentityStatus = "INSUFFICIENT_EVIDENCE" }},
		{"IdentityGateRegistered", "a gate that skips the registration of the identity gate", func(d *HuaweiProviderDisposition) { d.IdentityGateRegistered = false }},
		{"IdentityScope", "a gate that widens the observed-tuple scope", func(d *HuaweiProviderDisposition) { d.IdentityScope = "ALL_TUPLES" }},
		{"RecordedStop", "a gate that ignores a stop in force", func(d *HuaweiProviderDisposition) {
			d.RecordedStop = "IN_FORCE"
			d.FurtherLiveIO = huaweiLiveIOOperatorAtTime
		}},
		{"FurtherLiveIO", "a gate that ignores a hard stop on further reads", func(d *HuaweiProviderDisposition) {
			d.RecordedStop = huaweiStopObservedIdentity
			d.FurtherLiveIO = "HARD_STOP"
		}},
		{"ChildProfiles", "a gate that admits without a child profile", func(d *HuaweiProviderDisposition) { d.ChildProfiles = nil }},
		{"ChildProfiles", "a gate that ignores the child outcome", func(d *HuaweiProviderDisposition) { d.ChildProfiles[0].Outcome = "NO_ADMISSIBLE_PROFILE" }},
		{"ChildProfiles", "a gate that admits an unknown profile identifier", func(d *HuaweiProviderDisposition) { d.ChildProfiles[0].ProfileID += "x" }},
		{"ChildProfiles", "a gate that admits a profile of another family", func(d *HuaweiProviderDisposition) { d.ChildProfiles[0].Family = "SUN2000 MAP0 series" }},
		{"ChildProfiles", "a gate that admits the other path's profile", func(d *HuaweiProviderDisposition) {
			for _, profile := range HuaweiSUN2000MonitoringProfiles() {
				if profile.ID() != d.ChildProfiles[0].ProfileID {
					d.ChildProfiles[0].ProfileID, d.ChildProfiles[0].Family = profile.ID(), profile.Family()
					return
				}
			}
		}},
		{"CatalogRegistered", "a gate that ignores catalog registration", func(d *HuaweiProviderDisposition) { d.CatalogRegistered = true }},
		{"AutomaticRuntimeAdmission", "a gate that ignores automatic admission", func(d *HuaweiProviderDisposition) { d.AutomaticRuntimeAdmission = true }},
		{"SupportClaim", "a gate that ignores a support claim", func(d *HuaweiProviderDisposition) { d.SupportClaim = true }},
		{"WriteAuthority", "a gate that ignores write authority", func(d *HuaweiProviderDisposition) { d.WriteAuthority = true }},
	}
}

func TestH06ANonAdmissibleDispositionMakesConstructionRefuse(t *testing.T) {
	clock := newH06Clock()
	waits := newH06Waits(clock)
	covered := map[string]bool{}
	for _, path := range []HuaweiProviderPath{HuaweiProviderPathSDongle, HuaweiProviderPathSmartLogger} {
		recorded, ok := HuaweiRecordedProviderDisposition(path)
		if !ok || !EvaluateHuaweiProviderEnablement(recorded).Admitted {
			t.Fatalf("%s: the recorded disposition is not admitted: %+v", path, EvaluateHuaweiProviderEnablement(recorded))
		}
		construct := func(d HuaweiProviderDisposition) error {
			if path == HuaweiProviderPathSDongle {
				_, err := newHuaweiSDongleReadOnlyProvider(h06Config(clock, waits, []uint8{1}), d)
				return err
			}
			_, err := newHuaweiSmartLoggerReadOnlyProvider(h06Config(clock, waits, []uint8{1}), d)
			return err
		}
		if err := construct(recorded.clone()); err != nil {
			t.Fatalf("%s: the recorded disposition does not construct: %v", path, err)
		}
		for _, mutation := range h06DispositionMutations() {
			d := recorded.clone()
			mutation.mutate(&d)
			if EvaluateHuaweiProviderEnablement(d).Admitted {
				t.Fatalf("%s %s: admitted; catches %s", path, mutation.field, mutation.production)
			}
			if err := construct(d); !errors.Is(err, ErrHuaweiProviderNotAdmitted) {
				t.Fatalf("%s %s: construction %v; catches %s", path, mutation.field, err, mutation.production)
			}
			covered[mutation.field] = true
		}
	}
	kind := reflect.TypeOf(HuaweiProviderDisposition{})
	for index := 0; index < kind.NumField(); index++ {
		if !covered[kind.Field(index).Name] {
			t.Fatalf("field %s is not varied", kind.Field(index).Name)
		}
	}
	// The S-Dongle allowlist must name exactly the observed child unit.
	for _, allowed := range [][]uint8{nil, {2}, {1, 2}, {100}} {
		if _, err := NewHuaweiSDongleReadOnlyProvider(h06Config(clock, waits, allowed)); !errors.Is(err, ErrHuaweiProviderConfig) {
			t.Fatalf("allowlist %v: %v", allowed, err)
		}
	}
	for _, config := range []HuaweiProviderConfig{
		{Source: "", Wait: waits.Wait},
		{Source: "endpoint\x00", Wait: waits.Wait},
		{Source: string(make([]byte, 65)), Wait: waits.Wait},
		{Source: "endpoint-a", Wait: waits.Wait, AllowedUnits: []uint8{0}},
		{Source: "endpoint-a", Wait: waits.Wait, AllowedUnits: []uint8{248}},
		{Source: "endpoint-a", Wait: waits.Wait, AllowedUnits: []uint8{2, 2}},
	} {
		if _, err := NewHuaweiSmartLoggerReadOnlyProvider(config); !errors.Is(err, ErrHuaweiProviderConfig) {
			t.Fatalf("config %+v: %v", config, err)
		}
	}
}

func TestH06EMMAPathHasNoProviderAndKeepsTheMAP0Reason(t *testing.T) {
	recorded, ok := HuaweiRecordedProviderDisposition(HuaweiProviderPathEMMA)
	if !ok {
		t.Fatal("no recorded EMMA disposition")
	}
	enablement := EvaluateHuaweiProviderEnablement(recorded)
	if enablement.Admitted || enablement.Reason != HuaweiProviderNoAdmittedChildProfile ||
		!reflect.DeepEqual(enablement.Detail, []string{"SUN2000 MAP0 series: no_consulted_edition"}) {
		t.Fatalf("EMMA enablement: %+v", enablement)
	}
	// Even an identity card of the observed SmartLogger kind would not admit
	// the path while its only child row admits nothing.
	recorded.IdentityCard, recorded.IdentityScope = HuaweiSmartLoggerObservedQualificationCardID, huaweiIdentityObservedOnly
	if EvaluateHuaweiProviderEnablement(recorded).Admitted {
		t.Fatal("an EMMA disposition without an admitted child profile was admitted")
	}
	if _, ok := HuaweiProviderPacingFor(HuaweiProviderPathEMMA); ok {
		t.Fatal("the EMMA path has a provider pacing envelope")
	}
	if _, ok := HuaweiRecordedProviderDisposition("Other"); ok {
		t.Fatal("an unknown path has a recorded disposition")
	}
}

func TestH06RecordedDispositionsAgreeWithTheDispositionFiles(t *testing.T) {
	sun := decodeHuaweiObject(t, "profiles/vendor/huawei/sun2000-disposition.json")
	childRows := func(path string) []HuaweiProviderChildDisposition {
		var rows []HuaweiProviderChildDisposition
		for _, value := range h03Array(t, sun["monitoring_profiles"]) {
			row := huaweiObject(t, value)
			if row["gateway_path"] != path {
				continue
			}
			child := HuaweiProviderChildDisposition{Family: row["family"].(string), Outcome: row["outcome"].(string)}
			if id, ok := row["profile_id"].(string); ok {
				child.ProfileID = id
			}
			if reason, ok := row["reason"].(string); ok {
				child.Reason = reason
			}
			rows = append(rows, child)
		}
		return rows
	}
	flag := func(record map[string]any, key string) bool {
		value, ok := record[key]
		if !ok {
			return false
		}
		return value.(bool)
	}
	for _, test := range []struct {
		path HuaweiProviderPath
		file string
	}{
		{HuaweiProviderPathSmartLogger, "smartlogger-disposition.json"},
		{HuaweiProviderPathSDongle, "sdongle-disposition.json"},
		{HuaweiProviderPathEMMA, "emma-disposition.json"},
	} {
		t.Run(string(test.path), func(t *testing.T) {
			record := decodeHuaweiObject(t, "profiles/vendor/huawei/"+test.file)
			want := HuaweiProviderDisposition{
				Path: test.path, ChildProfiles: childRows(string(test.path)),
				CatalogRegistered: flag(record, "catalog_registered"), AutomaticRuntimeAdmission: flag(record, "automatic_runtime_admission"),
				SupportClaim: flag(record, "support_claim"), WriteAuthority: flag(record, "write_authority"),
			}
			observed := huaweiObject(t, record["observed_identity"])
			want.IdentityCard, want.IdentityStatus = observed["qualification_card"].(string), observed["status"].(string)
			want.IdentityGateRegistered, want.IdentityScope = observed["gate_registered"].(bool), observed["scope"].(string)
			if stop, ok := observed["hard_stop"]; ok {
				want.RecordedStop = huaweiObject(t, stop)["superseded_for"].(string)
				want.FurtherLiveIO = huaweiObject(t, stop)["further_live_io"].(string)
			}
			got, _ := HuaweiRecordedProviderDisposition(test.path)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("recorded=%+v\nfile=%+v", got, want)
			}
		})
	}
}

func TestH06ProductionProviderRowsMatchTheProviders(t *testing.T) {
	for _, test := range []struct {
		path       HuaweiProviderPath
		file       string
		providerID string
		profile    string
		page       string
		requires   []string
		issues     map[string]int
	}{
		{HuaweiProviderPathSmartLogger, "smartlogger-disposition.json", HuaweiSmartLoggerReadOnlyProviderID, HuaweiSUN2000M3MonitoringProfileID,
			"architecture/smartlogger-provider-port.md",
			[]string{"observed_identity_offline_admitted", "child_monitoring_profile_offline_admitted", "authority_fields_false"},
			map[string]int{"slice": 206, "parent": 201}},
		{HuaweiProviderPathSDongle, "sdongle-disposition.json", HuaweiSDongleReadOnlyProviderID, HuaweiSUN2000M1MonitoringProfileID,
			"architecture/sdongle-provider-port.md",
			[]string{"observed_identity_offline_admitted", "child_monitoring_profile_offline_admitted", "recorded_stop_superseded_for_observed_identity", "authority_fields_false"},
			map[string]int{"parent": 201}},
	} {
		t.Run(string(test.path), func(t *testing.T) {
			record := decodeHuaweiObject(t, "profiles/vendor/huawei/"+test.file)
			row := huaweiObject(t, record["production_provider"])
			keys := []string{"issue", "provider_id", "enablement", "scope", "requires", "identity_card", "child_profiles", "read_functions",
				"units", "never_read", "pacing", "docs_contract", "issues", "support_claim", "live_qualified"}
			if test.path == HuaweiProviderPathSDongle {
				keys = append(keys, "recorded_stop", "further_live_io")
			}
			requireHuaweiKeys(t, row, keys...)
			recorded, _ := HuaweiRecordedProviderDisposition(test.path)
			enablement := EvaluateHuaweiProviderEnablement(recorded)
			if huaweiInt(t, row["issue"]) != 349 || row["provider_id"] != test.providerID || row["enablement"] != string(enablement.Reason) ||
				row["scope"] != huaweiIdentityObservedOnly || row["identity_card"] != recorded.IdentityCard ||
				!reflect.DeepEqual(huaweiStringSlice(t, row["requires"]), test.requires) ||
				!reflect.DeepEqual(huaweiStringSlice(t, row["child_profiles"]), []string{test.profile}) ||
				!reflect.DeepEqual(huaweiStringSlice(t, row["child_profiles"]), enablement.Profiles) ||
				!reflect.DeepEqual(huaweiStringSlice(t, row["read_functions"]), []string{"FC03", "FC2B_MEI_0E"}) ||
				row["support_claim"] != false || row["live_qualified"] != false {
				t.Fatalf("production provider row: %#v", row)
			}
			pacing, _ := HuaweiProviderPacingFor(test.path)
			block := huaweiObject(t, row["pacing"])
			requireHuaweiKeys(t, block, "gap_ms", "cycle_ms", "attempt_deadline_ms", "max_attempts", "backoff_ms", "first_request_extra_attempt")
			var backoff []time.Duration
			for _, value := range h03Array(t, block["backoff_ms"]) {
				backoff = append(backoff, time.Duration(huaweiInt(t, value))*time.Millisecond)
			}
			if time.Duration(huaweiInt(t, block["gap_ms"]))*time.Millisecond != pacing.Gap ||
				time.Duration(huaweiInt(t, block["cycle_ms"]))*time.Millisecond != pacing.Cycle ||
				time.Duration(huaweiInt(t, block["attempt_deadline_ms"]))*time.Millisecond != pacing.AttemptDeadline ||
				huaweiInt(t, block["max_attempts"]) != pacing.MaxAttempts || !reflect.DeepEqual(backoff, pacing.Backoff) ||
				block["first_request_extra_attempt"] != pacing.FirstRequestExtraAttempt {
				t.Fatalf("pacing row %#v differs from %+v", block, pacing)
			}
			docs := huaweiObject(t, row["docs_contract"])
			if docs["repository"] != "helianthus-docs-modbus" || huaweiInt(t, docs["issue"]) != 242 || huaweiInt(t, docs["pull_request"]) != 253 ||
				!reflect.DeepEqual(huaweiStringSlice(t, docs["pages"]), []string{"protocols/huawei/monitoring-readonly-v1.md", "protocols/huawei/read-pacing-v1.md", test.page}) {
				t.Fatalf("docs contract: %#v", docs)
			}
			issues := huaweiObject(t, row["issues"])
			got := map[string]int{}
			for key, value := range issues {
				got[key] = huaweiInt(t, value)
			}
			if !reflect.DeepEqual(got, test.issues) {
				t.Fatalf("issues=%v want %v", got, test.issues)
			}
			// The disposition outcome of the gateway's own profile is unchanged.
			if record["catalog_registered"] != false || record["automatic_runtime_admission"] != false || record["support_claim"] != false || record["write_authority"] != false {
				t.Fatalf("an authority field changed: %#v", record)
			}
		})
	}
}

// The documentary SmartLogger commit owner and offline cycle validate the
// documentary tuples only. The observed tuple is refused there, which is why
// the provider is built on the observed card and the SUN2000 retention
// instead of widening that owner.
func TestH06DocumentaryOfflineOwnersRefuseTheObservedTuple(t *testing.T) {
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	context := SmartloggerInventoryContext{OwnerToken: "owner", ConnectionGeneration: 1, Identity: SmartloggerInventoryIdentity{
		RoutingContext: "route", UnitID: 0, ParentProfile: "parent", SourceRevision: "source", SelfModel: card.Model(), Firmware: card.Firmware(),
	}}
	if _, err := NewSmartloggerInventoryCommitOwner(context, time.Now); !errors.Is(err, ErrSmartloggerInventoryContext) {
		t.Fatalf("documentary commit owner accepted the observed tuple: %v", err)
	}
	if _, err := ParseHuaweiSmartLoggerOfflineInventory(HuaweiSmartLoggerOfflineInventoryInput{}); err == nil {
		t.Fatal("the documentary parser accepted an empty inventory")
	}
	// The providers' read plans are the SUN2000 profiles' own plans.
	for _, profile := range HuaweiSUN2000MonitoringProfiles() {
		var offsets []int
		for _, step := range profile.Plan(1) {
			offsets = append(offsets, int(step.Offset))
		}
		if !sort.IntsAreSorted(offsets) || len(offsets) < 3 {
			t.Fatalf("%s plan %v", profile.ID(), offsets)
		}
	}
}
