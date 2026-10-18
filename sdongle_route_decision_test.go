package modbusreg

import (
	"reflect"
	"testing"
	"time"
)

func routePlan() SDongleRoutePlan {
	key := SDongleRouteKey{Family: "S-Dongle", Model: "S-DongleB-03", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, Role: SDongleGateway, Unit: 100, Request: SDongleBasicMEI, Scope: "basic-identity", Revision: "synthetic-rA"}
	ref := SDongleRouteReference{Revision: key.Revision, Reference: "synthetic-evidence"}
	return SDongleRoutePlan{Key: key, Observation: HuaweiSDongleOfflineObservation{UnitID: 100, Model: key.Model, Firmware: key.Firmware, ProtocolMajor: 5, Capacity: 2, ChildCount: 1, ChangeSequenceBefore: 4, ChangeSequenceAfter: 4}, Evidence: SDongleRouteEvidence{Key: key, Current: true, Identity: ref, Protocol: ref, Allowlist: ref, References: []string{"synthetic-native-observation"}}, Bounds: SDongleRouteBounds{Deadline: time.Second, RetryBounded: true, MaxPages: 1, MaxObjects: 1, MaxBytes: 128}}
}

func childRoutePlan() SDongleRoutePlan {
	p := routePlan()
	parent := SDongleRouteSelection{Family: "S-Dongle", Model: "S-DongleB-03", Firmware: "V200R025C00SPC120", ProtocolMajor: 5, Role: SDongleGateway, Unit: 100, Request: SDongleBasicMEI, Scope: "basic-identity", Revision: "synthetic-rA"}
	p.Key.Model = "synthetic-child-product"
	p.Key.Firmware = "synthetic-child-firmware"
	p.Key.Role = SDongleChild
	p.Key.Unit = 7
	p.Key.Request = SDongleExtendedMEI
	p.Key.Scope = "extended-inventory"
	p.Key.Child = SDongleChildRoute{Parent: parent, EvidenceRevision: p.Key.Revision, Qualification: "qualified", GuardBefore: 4, GuardAfter: 4}
	p.Evidence.Key = p.Key
	p.Evidence.Route = SDongleRouteReference{Revision: p.Key.Revision, Reference: "synthetic-independent-child-route"}
	return p
}

func TestSdongleRouteDecisionPlanned(t *testing.T) {
	for _, p := range []SDongleRoutePlan{routePlan(), childRoutePlan()} {
		d := DecideSDongleRoute(p)
		if d.Outcome != SDonglePlanned || !d.DefaultDenied() || d.EvaluatorReason != HuaweiSDongleOfflineMatched {
			t.Fatalf("%+v", d)
		}
		if !reflect.DeepEqual(d.Key, p.Key) || !reflect.DeepEqual(d.Evidence, p.Evidence) {
			t.Fatal("native evidence lost")
		}
		p.Evidence.References[0] = "changed"
		if d.Evidence.References[0] != "synthetic-native-observation" {
			t.Fatal("aliased evidence")
		}
	}
	for _, scope := range []string{"protocol-baseline", "gateway-status", "capacity"} {
		p := routePlan()
		p.Key.Request = SDongleGatewayFC03
		p.Key.Scope = scope
		p.Evidence.Key = p.Key
		if d := DecideSDongleRoute(p); d.Outcome != SDonglePlanned {
			t.Fatalf("%s: %+v", scope, d)
		}
	}
}

func TestSdongleRouteDecisionMissingAndConflicting(t *testing.T) {
	cases := map[string]func(*SDongleRoutePlan){
		"family-other":          func(p *SDongleRoutePlan) { p.Key.Family = "other-family" },
		"model-other":           func(p *SDongleRoutePlan) { p.Key.Model = "other-product" },
		"firmware-other":        func(p *SDongleRoutePlan) { p.Key.Firmware = "other-firmware" },
		"gateway-child-context": func(p *SDongleRoutePlan) { p.Key.Child.GuardBefore = 1 },
		"empty-reference-entry": func(p *SDongleRoutePlan) { p.Evidence.References = []string{""} },
		"family":                func(p *SDongleRoutePlan) { p.Key.Family = "" },
		"model":                 func(p *SDongleRoutePlan) { p.Key.Model = "" },
		"firmware":              func(p *SDongleRoutePlan) { p.Key.Firmware = "" },
		"protocol-major":        func(p *SDongleRoutePlan) { p.Key.ProtocolMajor = 0 },
		"protocol-minor":        func(p *SDongleRoutePlan) { p.Key.ProtocolMinor = 1 },
		"role":                  func(p *SDongleRoutePlan) { p.Key.Role = "" },
		"unit":                  func(p *SDongleRoutePlan) { p.Key.Unit = 0 },
		"request":               func(p *SDongleRoutePlan) { p.Key.Request = "" },
		"scope":                 func(p *SDongleRoutePlan) { p.Key.Scope = "" },
		"revision":              func(p *SDongleRoutePlan) { p.Key.Revision = "" },
		"stale":                 func(p *SDongleRoutePlan) { p.Evidence.Current = false },
		"collision":             func(p *SDongleRoutePlan) { p.Evidence.Conflict = true },
		"identity-reference":    func(p *SDongleRoutePlan) { p.Evidence.Identity.Reference = "" },
		"protocol-reference":    func(p *SDongleRoutePlan) { p.Evidence.Protocol.Reference = "" },
		"allowlist-reference":   func(p *SDongleRoutePlan) { p.Evidence.Allowlist.Reference = "" },
		"identity-revision":     func(p *SDongleRoutePlan) { p.Evidence.Identity.Revision = "other" },
		"protocol-revision":     func(p *SDongleRoutePlan) { p.Evidence.Protocol.Revision = "other" },
		"allowlist-revision":    func(p *SDongleRoutePlan) { p.Evidence.Allowlist.Revision = "other" },
		"deadline":              func(p *SDongleRoutePlan) { p.Bounds.Deadline = 0 },
		"deadline-negative":     func(p *SDongleRoutePlan) { p.Bounds.Deadline = -time.Second },
		"deadline-excess":       func(p *SDongleRoutePlan) { p.Bounds.Deadline = 16 * time.Second },
		"retry-bound":           func(p *SDongleRoutePlan) { p.Bounds.RetryBounded = false },
		"pages":                 func(p *SDongleRoutePlan) { p.Bounds.MaxPages = 0 },
		"objects":               func(p *SDongleRoutePlan) { p.Bounds.MaxObjects = 0 },
		"bytes":                 func(p *SDongleRoutePlan) { p.Bounds.MaxBytes = 0 },
		"references":            func(p *SDongleRoutePlan) { p.Evidence.References = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := routePlan()
			change(&p)
			p.Evidence.Key = p.Key
			if d := DecideSDongleRoute(p); d.Outcome != SDongleWithheld {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestSdongleRouteDecisionKeyEquality(t *testing.T) {
	// Each scalar field of the complete key is independently changed in evidence.
	var vary func(*testing.T, reflect.Value, string)
	vary = func(t *testing.T, v reflect.Value, path string) {
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			name := path + v.Type().Field(i).Name
			if f.Kind() == reflect.Struct {
				vary(t, f, name+".")
				continue
			}
			t.Run(name, func(t *testing.T) {
				p := childRoutePlan()
				target := reflect.ValueOf(&p.Evidence.Key).Elem()
				for _, idx := range fieldPath(reflect.TypeOf(p.Key), name) {
					target = target.Field(idx)
				}
				switch target.Kind() {
				case reflect.String:
					target.SetString(target.String() + "-other")
				case reflect.Uint8, reflect.Uint16:
					target.SetUint(target.Uint() + 1)
				default:
					t.Fatal("uncovered key field")
				}
				if d := DecideSDongleRoute(p); d.Outcome != SDongleWithheld {
					t.Fatalf("%+v", d)
				}
			})
		}
	}
	vary(t, reflect.ValueOf(childRoutePlan().Key), "")
}

func fieldPath(typ reflect.Type, path string) []int {
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if path == f.Name {
			return []int{i}
		}
		if f.Type.Kind() == reflect.Struct && len(path) > len(f.Name)+1 && path[:len(f.Name)+1] == f.Name+"." {
			return append([]int{i}, fieldPath(f.Type, path[len(f.Name)+1:])...)
		}
	}
	panic("unknown field")
}

func TestSdongleRouteDecisionChild(t *testing.T) {
	cases := map[string]func(*SDongleRoutePlan){
		"parent-family":         func(p *SDongleRoutePlan) { p.Key.Child.Parent.Family = "" },
		"parent-model":          func(p *SDongleRoutePlan) { p.Key.Child.Parent.Model = "other" },
		"parent-firmware":       func(p *SDongleRoutePlan) { p.Key.Child.Parent.Firmware = "other" },
		"parent-major":          func(p *SDongleRoutePlan) { p.Key.Child.Parent.ProtocolMajor = 0 },
		"parent-minor":          func(p *SDongleRoutePlan) { p.Key.Child.Parent.ProtocolMinor = 1 },
		"parent-role":           func(p *SDongleRoutePlan) { p.Key.Child.Parent.Role = SDongleChild },
		"parent-unit":           func(p *SDongleRoutePlan) { p.Key.Child.Parent.Unit = 7 },
		"parent-request":        func(p *SDongleRoutePlan) { p.Key.Child.Parent.Request = "" },
		"parent-scope":          func(p *SDongleRoutePlan) { p.Key.Child.Parent.Scope = "" },
		"parent-revision":       func(p *SDongleRoutePlan) { p.Key.Child.Parent.Revision = "other" },
		"child-identity":        func(p *SDongleRoutePlan) { p.Key.Model = "" },
		"child-firmware":        func(p *SDongleRoutePlan) { p.Key.Firmware = "" },
		"child-revision":        func(p *SDongleRoutePlan) { p.Key.Child.EvidenceRevision = "other" },
		"qualification":         func(p *SDongleRoutePlan) { p.Key.Child.Qualification = "unknown" },
		"guard-before":          func(p *SDongleRoutePlan) { p.Key.Child.GuardBefore++ },
		"guard-after":           func(p *SDongleRoutePlan) { p.Key.Child.GuardAfter++ },
		"guard-both":            func(p *SDongleRoutePlan) { p.Key.Child.GuardBefore++; p.Key.Child.GuardAfter++ },
		"unit-100":              func(p *SDongleRoutePlan) { p.Key.Unit = 100 },
		"unit-zero":             func(p *SDongleRoutePlan) { p.Key.Unit = 0 },
		"route-not-independent": func(p *SDongleRoutePlan) { p.Evidence.Route.Reference = p.Evidence.Identity.Reference },
		"unit-high":             func(p *SDongleRoutePlan) { p.Key.Unit = 248 },
		"route-reference":       func(p *SDongleRoutePlan) { p.Evidence.Route.Reference = "" },
		"route-revision":        func(p *SDongleRoutePlan) { p.Evidence.Route.Revision = "other" },
		"pages-excess":          func(p *SDongleRoutePlan) { p.Bounds.MaxPages = 122 },
		"objects-excess":        func(p *SDongleRoutePlan) { p.Bounds.MaxObjects = 122 },
		"bytes-excess":          func(p *SDongleRoutePlan) { p.Bounds.MaxBytes = 32769 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := childRoutePlan()
			change(&p)
			p.Evidence.Key = p.Key
			want := SDongleWithheld
			if name == "unit-100" {
				want = SDongleNoSend
			}
			if d := DecideSDongleRoute(p); d.Outcome != want {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestSdongleRouteDecisionProhibitionsAndEvaluator(t *testing.T) {
	for name, change := range map[string]func(*SDongleRoutePlan){
		"gateway-extended":  func(p *SDongleRoutePlan) { p.Key.Request = SDongleExtendedMEI },
		"private":           func(p *SDongleRoutePlan) { p.Key.Request = SDonglePrivateFunction },
		"timeout":           func(p *SDongleRoutePlan) { p.TimedOut = true },
		"stopped":           func(p *SDongleRoutePlan) { p.QualificationStopped = true },
		"outside-allowlist": func(p *SDongleRoutePlan) { p.Key.Scope = "wildcard" },
	} {
		t.Run(name, func(t *testing.T) {
			p := routePlan()
			change(&p)
			p.Evidence.Key = p.Key
			d := DecideSDongleRoute(p)
			if d.Outcome != SDongleNoSend || d.EvaluatorReason != HuaweiSDongleOfflineMatched || !d.DefaultDenied() {
				t.Fatalf("%+v", d)
			}
		})
	}
	p := routePlan()
	p.Observation.Model = "S-DongleA-05"
	p.Observation.Firmware = "V200R022C10SPC312"
	p.Key.Model = p.Observation.Model
	p.Key.Firmware = p.Observation.Firmware
	p.Evidence.Key = p.Key
	if d := DecideSDongleRoute(p); d.Outcome != SDongleWithheld || d.EvaluatorReason != HuaweiSDongleOfflineMatched {
		t.Fatalf("documentary tuple: %+v", d)
	}
	for _, reason := range []HuaweiSDongleOfflineReason{HuaweiSDongleOfflineUnitMismatch, HuaweiSDongleOfflineModelMismatch, HuaweiSDongleOfflineFirmwareMismatch, HuaweiSDongleOfflineProtocolMismatch, HuaweiSDongleOfflineSearchIncomplete, HuaweiSDongleOfflineSequenceMismatch, HuaweiSDongleOfflineCapacityMismatch} {
		t.Run(string(reason), func(t *testing.T) {
			p := routePlan()
			switch reason {
			case HuaweiSDongleOfflineUnitMismatch:
				p.Observation.UnitID = 7
			case HuaweiSDongleOfflineModelMismatch:
				p.Observation.Model = "unknown"
			case HuaweiSDongleOfflineFirmwareMismatch:
				p.Observation.Firmware = "unknown"
			case HuaweiSDongleOfflineProtocolMismatch:
				p.Observation.ProtocolMajor = 4
			case HuaweiSDongleOfflineSearchIncomplete:
				p.Observation.SearchState = 1
			case HuaweiSDongleOfflineSequenceMismatch:
				p.Observation.ChangeSequenceAfter++
			case HuaweiSDongleOfflineCapacityMismatch:
				p.Observation.ChildCount = 3
			}
			d := DecideSDongleRoute(p)
			if d.Outcome != SDongleWithheld || d.EvaluatorReason != reason {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestSdongleRouteDecisionDetachedAndDeterministic(t *testing.T) {
	p := childRoutePlan()
	original := childRoutePlan()
	a := DecideSDongleRoute(p)
	b := DecideSDongleRoute(p)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(p, original) {
		t.Fatal("non-deterministic or changed input")
	}
	a.Evidence.References[0] = "changed-result"
	a.Key.Child.Parent.Model = "changed-parent"
	a.Observation.Model = "changed-observation"
	if !reflect.DeepEqual(p, original) || b.Evidence.References[0] != "synthetic-native-observation" {
		t.Fatal("result aliases input or another decision")
	}
	if !(SDongleRouteDecision{}).DefaultDenied() {
		t.Fatal("zero decision admitted")
	}
	p.Evidence.Current = false
	p.TimedOut = true
	if d := DecideSDongleRoute(p); d.Outcome != SDongleNoSend {
		t.Fatalf("stopped stale evidence: %+v", d)
	}
}
