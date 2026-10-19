package modbusreg

import (
	"context"
	"reflect"
	"testing"
)

func i15Input() LunaOptionalInput {
	return LunaOptionalInput{Binding: LunaOptionalBinding{Parent: LunaOptionalIdentity{"parent-family", "parent-firmware", "parent-unit"}, Group: LunaOptionalIdentity{"battery-family", "battery-firmware", "battery-unit"}, Attachment: "attached", ParentEpoch: "parent-epoch", GroupEpoch: "group-epoch", EvidenceRevision: "public-revision"}, ParentIdentified: true, EpochsAssociated: true, ObservationParentEpoch: "parent-epoch", ObservationGroupEpoch: "group-epoch", Started: 9, Now: 10, Event: LunaDocumentaryPresence, Evidence: []LunaOptionalEvidence{{Reference: "synthetic-presence", NativeOutcome: "documentary presence", ObservedAt: 10}}, Policy: LunaOptionalPolicy{MaxRecords: 4, MaxBytes: 2048, Deadline: 50, FreshnessAge: 10, RetentionAge: 30, HistoryRecords: 8, HistoryBytes: 8192}}
}
func i15Denied(i15T *testing.T, i15S LunaOptionalState) {
	i15T.Helper()
	if i15S.Outcome() != "NO_ADMISSIBLE_PROFILE" || i15S.CatalogRegistered() || i15S.AutomaticRuntimeAdmission() || i15S.SupportClaim() || i15S.WriteAuthority() || i15S.Qualified() {
		i15T.Fatal("documentary success admitted")
	}
}
func TestI15CycleRetentionAndDetachment(i15T *testing.T) {
	i15In := i15Input()
	i15Response := i15Input()
	i15Calls := 0
	i15P, i15Err := I15NewLunaProvider("synthetic-generation", i15In, func(i15Ctx context.Context, i15Request LunaOptionalInput) (LunaOptionalInput, error) {
		i15Calls++
		i15Request.Evidence = append(i15Request.Evidence, LunaOptionalEvidence{Reference: "supplier-local"})
		return i15Response, nil
	})
	if i15Err != nil {
		i15T.Fatal(i15Err)
	}
	if i15P.I15Latest().I15HasReceipt || i15P.I15Latest().I15State.State() != LunaUnknown || len(i15P.I15Latest().I15State.History()) != 0 {
		i15T.Fatal("parent/constructor proved presence")
	}
	var i15Port I15LunaProviderPort = i15P
	i15In.Evidence[0].Reference = "caller-edit"
	if i15Err = i15Port.I15Cycle(context.Background(), 9); i15Err != nil {
		i15T.Fatal(i15Err)
	}
	i15Response.Evidence[0].Reference = "supplier-edit"
	i15First := i15Port.I15Latest()
	if i15First.I15State.State() != LunaPresentUnqualified || i15First.I15Receipt.Evidence[0].Reference != "synthetic-presence" || i15First.I15Generation != "synthetic-generation" {
		i15T.Fatal(i15First)
	}
	i15First.I15Receipt.Evidence[0].Reference = "view-edit"
	i15History := i15First.I15State.History()
	i15History[0].Evidence.Reference = "history-edit"
	if i15Port.I15Latest().I15Receipt.Evidence[0].Reference != "synthetic-presence" || i15Port.I15Latest().I15State.History()[0].Evidence.Reference != "synthetic-presence" {
		i15T.Fatal("alias")
	}
	for i15Index, i15Outcome := range []string{"timeout", "illegal-address", "unattempted", "explicit absence"} {
		i15Response = i15Input()
		i15Response.Started = int64(11 + i15Index)
		i15Response.Now = int64(12 + i15Index)
		i15Response.Event = LunaOptionalFailure
		if i15Outcome == "unattempted" {
			i15Response.Event = LunaNoObservation
		}
		if i15Outcome == "explicit absence" {
			i15Response.Event = LunaOptionalEvent("observed absent")
		}
		i15Response.Evidence = []LunaOptionalEvidence{{Reference: "synthetic-result", NativeOutcome: i15Outcome, ObservedAt: i15Response.Now}}
		if i15Err = i15Port.I15Cycle(context.Background(), i15Response.Started); i15Err != nil {
			i15T.Fatal(i15Err)
		}
		i15View := i15Port.I15Latest()
		i15Want := LunaTemporarilyUnreadable
		if i15Outcome == "unattempted" || i15Outcome == "explicit absence" {
			i15Want = LunaUnknown
		}
		if i15View.I15State.State() != i15Want || !i15View.I15State.ParentIdentified() || i15View.I15Receipt.Evidence[0].NativeOutcome != i15Outcome || i15View.I15State.History()[0].Evidence.ObservedAt != 10 {
			i15T.Fatal(i15View)
		}
		i15Denied(i15T, i15View.I15State)
	}
	i15Denied(i15T, i15First.I15State)
	if i15Calls != 5 {
		i15T.Fatal(i15Calls)
	}
}

func TestI15FrozenAxes(i15T *testing.T) {
	i15Changes := map[string]func(*LunaOptionalInput){}
	for _, i15Side := range []string{"Parent", "Group"} {
		for _, i15Field := range []string{"Model", "Firmware", "Unit"} {
			i15Side, i15Field := i15Side, i15Field
			i15Changes[i15Side+"."+i15Field] = func(i15In *LunaOptionalInput) {
				reflect.ValueOf(&i15In.Binding).Elem().FieldByName(i15Side).FieldByName(i15Field).SetString("changed")
			}
		}
	}
	for _, i15Field := range []string{"Attachment", "ParentEpoch", "GroupEpoch", "EvidenceRevision"} {
		i15Field := i15Field
		i15Changes[i15Field] = func(i15In *LunaOptionalInput) {
			reflect.ValueOf(&i15In.Binding).Elem().FieldByName(i15Field).SetString("changed")
		}
	}
	for _, i15Field := range []string{"MaxRecords", "MaxBytes", "Deadline", "FreshnessAge", "RetentionAge", "HistoryRecords", "HistoryBytes"} {
		i15Field := i15Field
		i15Changes["Policy."+i15Field] = func(i15In *LunaOptionalInput) {
			i15V := reflect.ValueOf(&i15In.Policy).Elem().FieldByName(i15Field)
			i15V.SetInt(i15V.Int() + 1)
		}
	}
	i15Changes["ParentIdentified"] = func(i15In *LunaOptionalInput) { i15In.ParentIdentified = false }
	i15Changes["EpochsAssociated"] = func(i15In *LunaOptionalInput) { i15In.EpochsAssociated = false }
	i15Changes["ObservationParentEpoch"] = func(i15In *LunaOptionalInput) { i15In.ObservationParentEpoch = "retired" }
	i15Changes["ObservationGroupEpoch"] = func(i15In *LunaOptionalInput) { i15In.ObservationGroupEpoch = "retired" }
	for i15Name, i15Change := range i15Changes {
		i15T.Run(i15Name, func(i15T *testing.T) {
			i15Response := i15Input()
			i15P, i15Err := I15NewLunaProvider("generation", i15Input(), func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) { return i15Response, nil })
			if i15Err != nil {
				i15T.Fatal(i15Err)
			}
			if i15Err = i15P.I15Cycle(context.Background(), 9); i15Err != nil {
				i15T.Fatal(i15Err)
			}
			i15Before := i15P.I15Latest()
			i15Response.Started = 11
			i15Response.Now = 12
			i15Change(&i15Response)
			if i15Name == "ParentEpoch" {
				i15Response.ObservationParentEpoch = i15Response.Binding.ParentEpoch
			}
			if i15Name == "GroupEpoch" {
				i15Response.ObservationGroupEpoch = i15Response.Binding.GroupEpoch
			}
			if i15P.I15Cycle(context.Background(), 11) == nil || !reflect.DeepEqual(i15Before, i15P.I15Latest()) {
				i15T.Fatal("changed frozen axis published")
			}
		})
	}
}

func TestI15RefusalsAndCompletionFence(i15T *testing.T) {
	for _, i15Case := range []string{"cancelled", "retired", "late", "backward", "supplier error", "busy"} {
		i15T.Run(i15Case, func(i15T *testing.T) {
			i15Calls := 0
			i15Ctx, i15Cancel := context.WithCancel(context.Background())
			defer i15Cancel()
			var i15P *I15LunaProvider
			i15P, _ = I15NewLunaProvider("generation", i15Input(), func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) {
				i15Calls++
				i15Response := i15Input()
				switch i15Case {
				case "cancelled":
					i15Cancel()
				case "retired":
					i15P.I15Retire()
				case "late":
					i15Response.Now = 50
				case "backward":
					i15Response.Now = 8
				case "supplier error":
					return i15Response, context.Canceled
				case "busy":
					if i15P.I15Cycle(context.Background(), 9) == nil {
						i15T.Fatal("recursive cycle accepted")
					}
					return i15Response, context.Canceled
				}
				return i15Response, nil
			})
			i15Before := i15P.I15Latest()
			if i15P.I15Cycle(i15Ctx, 9) == nil || !reflect.DeepEqual(i15Before, i15P.I15Latest()) || i15Calls != 1 {
				i15T.Fatal("completion escaped fence")
			}
		})
	}
	i15Calls := 0
	i15Supplier := func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) {
		i15Calls++
		return i15Input(), nil
	}
	for _, i15Case := range []string{"generation", "binding", "policy", "parent", "association", "epochs", "bytes", "time", "evidence"} {
		i15In := i15Input()
		i15Generation := "generation"
		switch i15Case {
		case "generation":
			i15Generation = ""
		case "binding":
			i15In.Binding.Group.Unit = ""
		case "policy":
			i15In.Policy.FreshnessAge = 0
		case "parent":
			i15In.ParentIdentified = false
		case "association":
			i15In.EpochsAssociated = false
		case "epochs":
			i15In.ObservationParentEpoch = "retired"
		case "bytes":
			i15In.Policy.MaxBytes = 1
		case "time":
			i15In.Now = 50
		case "evidence":
			i15In.Evidence[0].Reference = ""
		}
		if i15P, i15Err := I15NewLunaProvider(i15Generation, i15In, i15Supplier); i15Err == nil || i15P != nil {
			i15T.Fatal(i15Case)
		}
	}
	if i15P, i15Err := I15NewLunaProvider("generation", i15Input(), nil); i15Err == nil || i15P != nil {
		i15T.Fatal("nil supplier")
	}
	i15P, _ := I15NewLunaProvider("generation", i15Input(), i15Supplier)
	i15Ctx, i15Cancel := context.WithCancel(context.Background())
	i15Cancel()
	var i15NilContext context.Context
	if i15P.I15Cycle(i15Ctx, 9) == nil || i15P.I15Cycle(i15NilContext, 9) == nil || i15P.I15Cycle(context.Background(), 50) == nil {
		i15T.Fatal("invalid request accepted")
	}
	i15P.I15Retire()
	if i15P.I15Cycle(context.Background(), 9) == nil {
		i15T.Fatal("retired request accepted")
	}
	if i15Calls != 0 {
		i15T.Fatal("refusal called supplier")
	}
}

func TestI15InvalidResultsKeepOriginalReceipt(i15T *testing.T) {
	i15Changes := map[string]func(*LunaOptionalInput){
		"future":           func(i15In *LunaOptionalInput) { i15In.Evidence[0].ObservedAt = 13 },
		"negative receipt": func(i15In *LunaOptionalInput) { i15In.Evidence[0].ObservedAt = -1 },
		"reference":        func(i15In *LunaOptionalInput) { i15In.Evidence[0].Reference = "" },
		"outcome":          func(i15In *LunaOptionalInput) { i15In.Evidence[0].NativeOutcome = "" },
		"records":          func(i15In *LunaOptionalInput) { i15In.Evidence = make([]LunaOptionalEvidence, 5) },
		"bytes":            func(i15In *LunaOptionalInput) { i15In.Evidence[0].Reference = string(make([]byte, 2048)) },
		"started":          func(i15In *LunaOptionalInput) { i15In.Started = 8 },
	}
	for i15Name, i15Change := range i15Changes {
		i15T.Run(i15Name, func(i15T *testing.T) {
			i15Response := i15Input()
			i15P, _ := I15NewLunaProvider("generation", i15Input(), func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) { return i15Response, nil })
			if i15P.I15Cycle(context.Background(), 9) != nil {
				i15T.Fatal("initial cycle")
			}
			i15Before := i15P.I15Latest()
			i15Response.Started = 11
			i15Response.Now = 12
			i15Change(&i15Response)
			if i15P.I15Cycle(context.Background(), 11) == nil || !reflect.DeepEqual(i15P.I15Latest(), i15Before) {
				i15T.Fatal("invalid result replaced original receipt")
			}
		})
	}
}

func TestI15MissingConstructorAxes(i15T *testing.T) {
	i15Calls := 0
	i15Supplier := func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) {
		i15Calls++
		return i15Input(), nil
	}
	for _, i15Side := range []string{"Parent", "Group"} {
		for _, i15Field := range []string{"Model", "Firmware", "Unit"} {
			i15T.Run(i15Side+"."+i15Field, func(i15T *testing.T) {
				i15In := i15Input()
				reflect.ValueOf(&i15In.Binding).Elem().FieldByName(i15Side).FieldByName(i15Field).SetString("")
				if i15P, i15Err := I15NewLunaProvider("generation", i15In, i15Supplier); i15Err == nil || i15P != nil {
					i15T.Fatal("missing identity accepted")
				}
			})
		}
	}
	for _, i15Field := range []string{"Attachment", "ParentEpoch", "GroupEpoch", "EvidenceRevision"} {
		i15T.Run(i15Field, func(i15T *testing.T) {
			i15In := i15Input()
			reflect.ValueOf(&i15In.Binding).Elem().FieldByName(i15Field).SetString("")
			if i15P, i15Err := I15NewLunaProvider("generation", i15In, i15Supplier); i15Err == nil || i15P != nil {
				i15T.Fatal("missing binding accepted")
			}
		})
	}
	for _, i15Field := range []string{"MaxRecords", "MaxBytes", "Deadline", "FreshnessAge", "RetentionAge", "HistoryRecords", "HistoryBytes"} {
		i15T.Run(i15Field, func(i15T *testing.T) {
			i15In := i15Input()
			reflect.ValueOf(&i15In.Policy).Elem().FieldByName(i15Field).SetInt(0)
			if i15P, i15Err := I15NewLunaProvider("generation", i15In, i15Supplier); i15Err == nil || i15P != nil {
				i15T.Fatal("missing policy accepted")
			}
		})
	}
	if i15Calls != 0 {
		i15T.Fatal("constructor invoked supplier")
	}
}

func TestI15ConcurrentRetirementAndCancellation(i15T *testing.T) {
	for _, i15Case := range []string{"retirement", "cancellation"} {
		i15T.Run(i15Case, func(i15T *testing.T) {
			i15Entered := make(chan struct{})
			i15Release := make(chan struct{})
			i15Done := make(chan error, 1)
			i15Ctx, i15Cancel := context.WithCancel(context.Background())
			defer i15Cancel()
			i15Calls := 0
			i15P, _ := I15NewLunaProvider("generation", i15Input(), func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) {
				i15Calls++
				if i15Calls == 2 {
					close(i15Entered)
					<-i15Release
					i15Result := i15Input()
					i15Result.Started = 10
					i15Result.Now = 11
					return i15Result, nil
				}
				return i15Input(), nil
			})
			if i15P.I15Cycle(context.Background(), 9) != nil {
				i15T.Fatal("initial cycle")
			}
			i15Before := i15P.I15Latest()
			go func() { i15Done <- i15P.I15Cycle(i15Ctx, 10) }()
			<-i15Entered
			if i15P.I15Cycle(context.Background(), 10) == nil {
				i15T.Fatal("busy cycle accepted")
			}
			if i15Case == "retirement" {
				i15P.I15Retire()
			} else {
				i15Cancel()
			}
			close(i15Release)
			if <-i15Done == nil || !reflect.DeepEqual(i15Before, i15P.I15Latest()) || i15Calls != 2 {
				i15T.Fatal("late completion replaced original receipt")
			}
		})
	}
}

func TestI15RetentionAgesWithoutRedating(i15T *testing.T) {
	i15Response := i15Input()
	i15P, _ := I15NewLunaProvider("generation", i15Input(), func(context.Context, LunaOptionalInput) (LunaOptionalInput, error) { return i15Response, nil })
	if i15P.I15Cycle(context.Background(), 9) != nil {
		i15T.Fatal("initial cycle")
	}
	i15Original := i15P.I15Latest()
	i15Response.Started = 20
	i15Response.Now = 21
	i15Response.Event = LunaOptionalFailure
	i15Response.Evidence = []LunaOptionalEvidence{{Reference: "synthetic-timeout", NativeOutcome: "timeout", ObservedAt: 21}}
	if i15P.I15Cycle(context.Background(), 20) != nil {
		i15T.Fatal("failure cycle")
	}
	i15History := i15P.I15Latest().I15State.History()
	if len(i15History) != 2 || i15History[0].Evidence.Reference != "synthetic-presence" || i15History[0].Evidence.ObservedAt != 10 || i15History[0].Fresh || !i15History[1].Fresh || i15Original.I15Receipt.Now != 10 || !i15Original.I15State.History()[0].Fresh {
		i15T.Fatal("retained receipt age changed", i15History)
	}
	i15Response.Started = 40
	i15Response.Now = 41
	i15Response.Evidence = nil
	if i15P.I15Cycle(context.Background(), 40) != nil {
		i15T.Fatal("aging cycle")
	}
	i15History = i15P.I15Latest().I15State.History()
	if len(i15History) != 1 || i15History[0].Evidence.Reference != "synthetic-timeout" || i15History[0].Evidence.ObservedAt != 21 || i15History[0].Fresh {
		i15T.Fatal("retention or freshness extended", i15History)
	}
	i15Denied(i15T, i15P.I15Latest().I15State)
}
