package modbusreg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func i27Input() OutBackDetachedRecordInput {
	return OutBackDetachedRecordInput{
		Words: []uint16{17, 23}, DeclaredWords: 2, Present: []OutBackWordSpan{{0, 2}},
		Source:      OutBackSourceAssociation{Key: "synthetic-family", Revision: "one", PublicLocator: "public/family", ContentIdentity: "synthetic-content", DeclaredLayout: "opaque", SourceSetRevision: "set-one", SelectorInputs: "selector-one", Available: true},
		Occurrences: []OutBackDetachedOccurrence{{Ordinal: 2, WireKey: SunSpecWireKey{64111, 1}, Words: []uint16{17}, Present: []OutBackWordSpan{{0, 1}}}, {Ordinal: 4, WireKey: SunSpecWireKey{64111, 1}, Offset: 1, Words: []uint16{23}, Present: []OutBackWordSpan{{0, 1}}}},
	}
}

func i27Config(i27Supplier OutBackDiagnosticSupplier) I27OutBackConfig {
	return I27OutBackConfig{I27Source: i27Input().Source, I27MaxBytes: 4096, I27MaxWork: time.Second, I27Supplier: i27Supplier}
}

func i27Provider(i27T *testing.T, i27Supplier OutBackDiagnosticSupplier) I27OutBackProvider {
	i27T.Helper()
	i27P, i27Err := I27NewOutBackAdapter(i27Config(i27Supplier))
	if i27Err != nil {
		i27T.Fatal(i27Err)
	}
	return i27P
}

func i27Denied(i27T *testing.T, i27Record *OutBackDetachedRecord) {
	i27T.Helper()
	if i27Record.Evaluate().Matched() || i27Record.Evaluate().Reason() != OutBackAXSReadOnlyFlavorReasonSourceNotQualified || len(i27Record.Requests()) != 0 || len(i27Record.TypedFacts()) != 0 {
		i27T.Fatal("opaque evidence admitted or send/facts exposed")
	}
}

func TestI27Refusals(i27T *testing.T) {
	i27Calls := 0
	i27Supplier := func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error) {
		i27Calls++
		return i27Input(), nil
	}
	for _, i27Kind := range []string{"nil", "bytes", "work", "negative", "source"} {
		i27C := i27Config(i27Supplier)
		switch i27Kind {
		case "nil":
			i27C.I27Supplier = nil
		case "bytes":
			i27C.I27MaxBytes = 0
		case "work":
			i27C.I27MaxWork = 0
		case "negative":
			i27C.I27MaxWork = -1
		case "source":
			i27C.I27Source.Key = string(make([]byte, 4096))
		}
		if i27P, i27Err := I27NewOutBackAdapter(i27C); i27Err == nil || i27P != nil {
			i27T.Fatal("accepted invalid config", i27Kind)
		}
	}
	i27P := i27Provider(i27T, i27Supplier)
	var i27Nil context.Context
	if i27P.I27Cycle(i27Nil, time.Unix(1, 0)) == nil {
		i27T.Fatal("nil context accepted")
	}
	if i27P.I27Cycle(context.Background(), time.Time{}) == nil {
		i27T.Fatal("zero receipt accepted")
	}
	i27Ctx, i27Cancel := context.WithCancel(context.Background())
	i27Cancel()
	if !errors.Is(i27P.I27Cycle(i27Ctx, time.Unix(1, 0)), context.Canceled) {
		i27T.Fatal("canceled context accepted")
	}
	i27Source := i27Input().Source
	i27Source.Key = string(make([]byte, 4096))
	if i27P.I27Replace(i27Source) == nil {
		i27T.Fatal("oversized replacement accepted")
	}
	i27P.I27Retire()
	if !errors.Is(i27P.I27Cycle(context.Background(), time.Unix(1, 0)), ErrOutBackOfflineRetired) {
		i27T.Fatal("retired cycle accepted")
	}
	if !errors.Is(i27P.I27Replace(i27Input().Source), ErrOutBackOfflineRetired) {
		i27T.Fatal("retired replacement accepted")
	}
	if i27Calls != 0 {
		i27T.Fatal("refusal invoked supplier", i27Calls)
	}
	for _, i27Zero := range []*I27OutBackAdapter{nil, {}} {
		if i27Zero.I27Cycle(context.Background(), time.Unix(1, 0)) == nil || i27Zero.I27Replace(i27Input().Source) == nil {
			i27T.Fatal("zero adapter accepted")
		}
		i27Zero.I27Retire()
		i27S := i27Zero.I27Snapshot(i27Input().Source)
		if i27S.Record != nil || i27S.Association != OutBackAssociationHistorical {
			i27T.Fatal(i27S)
		}
		i27Denied(i27T, i27S.Record)
	}
}

func i27Changes() map[string]func(*OutBackSourceAssociation) {
	return map[string]func(*OutBackSourceAssociation){
		"Key":               func(i27S *OutBackSourceAssociation) { i27S.Key = "other" },
		"Revision":          func(i27S *OutBackSourceAssociation) { i27S.Revision = "two" },
		"PublicLocator":     func(i27S *OutBackSourceAssociation) { i27S.PublicLocator = "public/other" },
		"ContentIdentity":   func(i27S *OutBackSourceAssociation) { i27S.ContentIdentity = "other-content" },
		"DeclaredLayout":    func(i27S *OutBackSourceAssociation) { i27S.DeclaredLayout = "other-layout" },
		"SourceSetRevision": func(i27S *OutBackSourceAssociation) { i27S.SourceSetRevision = "set-two" },
		"SelectorInputs":    func(i27S *OutBackSourceAssociation) { i27S.SelectorInputs = "selector-two" },
		"Available":         func(i27S *OutBackSourceAssociation) { i27S.Available = false },
	}
}

func TestI27AssociationAxes(i27T *testing.T) {
	for i27Name, i27Change := range i27Changes() {
		i27T.Run(i27Name, func(i27T *testing.T) {
			i27InputValue := i27Input()
			i27Calls := 0
			i27P := i27Provider(i27T, func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error) {
				i27Calls++
				return i27InputValue, nil
			})
			if i27Err := i27P.I27Cycle(context.Background(), time.Unix(1, 0)); i27Err != nil {
				i27T.Fatal(i27Err)
			}
			i27Before := i27P.I27Snapshot(i27Input().Source)
			i27Current := i27Input().Source
			i27Change(&i27Current)
			if i27Before.Record.Association(i27Current) != OutBackAssociationHistorical {
				i27T.Fatal("retained native comparison omitted axis")
			}
			if i27S := i27P.I27Snapshot(i27Current); i27S.Association != OutBackAssociationHistorical || i27S.Record.SourceAssociation() != i27Input().Source {
				i27T.Fatal("comparison omitted axis or relabeled", i27S)
			}
			i27Change(&i27InputValue.Source)
			if i27Err := i27P.I27Cycle(context.Background(), time.Unix(2, 0)); !errors.Is(i27Err, ErrOutBackOfflineAssociation) {
				i27T.Fatal("supplier axis accepted", i27Err)
			}
			i27After := i27P.I27Snapshot(i27Input().Source)
			if i27After.Record != i27Before.Record || i27After.RetainedAt != time.Unix(1, 0) || i27After.RetainedAttempt != i27Before.RetainedAttempt || i27After.Status != OutBackOfflineFailed || i27After.Association != OutBackAssociationCurrent || i27Calls != 2 {
				i27T.Fatal("failed same-source attempt changed retention", i27After)
			}
			if i27Err := i27P.I27Replace(i27Current); i27Err != nil {
				i27T.Fatal(i27Err)
			}
			if i27P.I27Snapshot(i27Input().Source).Association != OutBackAssociationHistorical {
				i27T.Fatal("old descriptor restored current history")
			}
			if i27Err := i27P.I27Cycle(context.Background(), time.Unix(3, 0)); i27Err != nil {
				i27T.Fatal(i27Err)
			}
			i27Replacement := i27P.I27Snapshot(i27Current)
			i27Expected := OutBackAssociationCurrent
			if !i27Current.Available {
				i27Expected = OutBackAssociationHistorical
			}
			if i27Replacement.Record.SourceAssociation() != i27Current || i27Replacement.RetainedAttempt.Generation != 2 || i27Replacement.RetainedAt != time.Unix(3, 0) || i27Replacement.Association != i27Expected || i27Replacement.Record == i27Before.Record || i27Calls != 3 {
				i27T.Fatal("replacement import lost identity", i27Replacement)
			}
			if i27P.I27Snapshot(OutBackSourceAssociation{}).Association != OutBackAssociationHistorical {
				i27T.Fatal("missing descriptor current")
			}
			if !i27Current.Available {
				if i27Replacement.Record.Association(i27Current) != OutBackAssociationHistorical {
					i27T.Fatal("unavailable original/current native association admitted")
				}
				i27Current.Available = true
				if i27P.I27Snapshot(i27Current).Association != OutBackAssociationHistorical {
					i27T.Fatal("unavailable original restored by current availability")
				}
			}
			if i27Before.Record.SourceAssociation() != i27Input().Source || i27Before.RetainedAt != time.Unix(1, 0) {
				i27T.Fatal("replacement mutated detached history")
			}
			i27Denied(i27T, i27Replacement.Record)
			i27Denied(i27T, i27After.Record)
		})
	}
}

func TestI27RetentionAndReplacement(i27T *testing.T) {
	i27Value := i27Input()
	var i27Failure error
	i27Calls := 0
	i27P := i27Provider(i27T, func(i27Ctx context.Context, i27Attempt OutBackOfflineAttempt) (OutBackDetachedRecordInput, error) {
		i27Calls++
		if _, i27OK := i27Ctx.Deadline(); !i27OK {
			i27T.Error("no work deadline")
		}
		if i27Attempt.Number != uint64(i27Calls) {
			i27T.Error("attempt not frozen")
		}
		return i27Value, i27Failure
	})
	if i27Err := i27P.I27Cycle(context.Background(), time.Unix(1, 0)); i27Err != nil {
		i27T.Fatal(i27Err)
	}
	i27Before := i27P.I27Snapshot(i27Value.Source)
	if i27Before.Association != OutBackAssociationCurrent || !i27Before.Record.Complete() || i27Before.Status != OutBackOfflineImported {
		i27T.Fatal(i27Before)
	}
	for _, i27Kind := range []string{"failed", "oversize", "partial", "malformed"} {
		i27Value = i27Input()
		i27Failure = nil
		switch i27Kind {
		case "failed":
			i27Failure = errors.New("synthetic supplier failure")
		case "oversize":
			i27Value.Words = make([]uint16, 4096)
		case "partial":
			i27Value.Missing = []OutBackWordSpan{{1, 1}}
		case "malformed":
			i27Value.Present = []OutBackWordSpan{{9, 2}}
		}
		i27Err := i27P.I27Cycle(context.Background(), time.Unix(2, 0))
		i27ExpectedStatus := OutBackOfflineIncomplete
		if i27Kind == "failed" || i27Kind == "oversize" {
			i27ExpectedStatus = OutBackOfflineFailed
			if i27Err == nil {
				i27T.Fatal("failure accepted")
			}
		} else if i27Err != nil {
			i27T.Fatal(i27Err)
		}
		i27S := i27P.I27Snapshot(i27Input().Source)
		if i27S.Record != i27Before.Record || i27S.RetainedAt != time.Unix(1, 0) || i27S.RetainedAttempt != i27Before.RetainedAttempt || i27S.Status != i27ExpectedStatus || i27S.Association != OutBackAssociationCurrent {
			i27T.Fatal("retained record renewed/lost", i27Kind, i27S)
		}
	}
	if i27Calls != 5 {
		i27T.Fatal("automatic retry", i27Calls)
	}
	i27Value = i27Input()
	i27Failure = nil
	for _, i27Source := range []OutBackSourceAssociation{i27Input().Source, {}, i27Input().Source} {
		if i27Err := i27P.I27Replace(i27Source); i27Err != nil {
			i27T.Fatal(i27Err)
		}
		i27S := i27P.I27Snapshot(i27Input().Source)
		if i27S.Association != OutBackAssociationHistorical || i27S.Record != i27Before.Record || i27S.RetainedAt != time.Unix(1, 0) || i27S.RetainedAttempt != i27Before.RetainedAttempt {
			i27T.Fatal("replacement resurrected/redated history", i27S)
		}
	}
	if i27Err := i27P.I27Cycle(context.Background(), time.Unix(3, 0)); i27Err != nil {
		i27T.Fatal(i27Err)
	}
	i27After := i27P.I27Snapshot(i27Input().Source)
	if i27After.Association != OutBackAssociationCurrent || i27After.RetainedAttempt.Generation != 4 || i27After.RetainedAt != time.Unix(3, 0) || i27After.Record == i27Before.Record {
		i27T.Fatal(i27After)
	}
	i27P.I27Retire()
	if i27S := i27P.I27Snapshot(i27Input().Source); i27S.Association != OutBackAssociationHistorical || i27S.Record != i27After.Record || i27S.RetainedAt != time.Unix(3, 0) {
		i27T.Fatal(i27S)
	}
	i27Denied(i27T, i27After.Record)
}

func TestI27Detachment(i27T *testing.T) {
	for _, i27Partial := range []bool{false, true} {
		i27Value := i27Input()
		if i27Partial {
			i27Value.Missing = []OutBackWordSpan{{2, 1}}
			i27Value.Occurrences[0].Missing = []OutBackWordSpan{{1, 1}}
		}
		i27Want := i27Input()
		if i27Partial {
			i27Want.Missing = []OutBackWordSpan{{2, 1}}
			i27Want.Occurrences[0].Missing = []OutBackWordSpan{{1, 1}}
		}
		i27C := i27Config(func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error) { return i27Value, nil })
		i27P, i27Err := I27NewOutBackAdapter(i27C)
		if i27Err != nil {
			i27T.Fatal(i27Err)
		}
		i27C.I27Source.Revision = "changed-config"
		if i27Err = i27P.I27Cycle(context.Background(), time.Unix(1, 0)); i27Err != nil {
			i27T.Fatal(i27Err)
		}
		i27Value.Words[0] = 99
		i27Value.Present[0].Offset = 99
		i27Value.Occurrences[0].Words[0] = 99
		i27Value.Occurrences[0].Present[0].Offset = 99
		i27Value.Occurrences[1].Ordinal = 99
		i27Value.Source.Key = "changed-input"
		if i27Partial {
			i27Value.Missing[0].Offset = 99
			i27Value.Occurrences[0].Missing[0].Offset = 99
		}
		i27S := i27P.I27Snapshot(i27Want.Source)
		i27S.Record.RawWords()[0] = 88
		i27S.Record.PresentSpans()[0].Offset = 88
		i27Views := i27S.Record.Occurrences()
		i27Views[0].Words[0] = 88
		i27Views[0].Present[0].Offset = 88
		i27Views[1].Ordinal = 88
		if i27Partial {
			i27S.Record.MissingSpans()[0].Offset = 88
			i27Views[0].Missing[0].Offset = 88
		}
		i27S.RetainedAttempt.Source.Key = "changed-output"
		i27S.RetainedAt = time.Time{}
		i27Got := i27P.I27Snapshot(i27Want.Source)
		if !reflect.DeepEqual(i27Got.Record.RawWords(), i27Want.Words) || !reflect.DeepEqual(i27Got.Record.PresentSpans(), i27Want.Present) || !reflect.DeepEqual(i27Got.Record.MissingSpans(), i27Want.Missing) || !reflect.DeepEqual(i27Got.Record.Occurrences(), i27Want.Occurrences) || i27Got.Record.SourceAssociation() != i27Want.Source || i27Got.RetainedAttempt.Source != i27Want.Source || i27Got.RetainedAt != time.Unix(1, 0) || i27Got.Record.Complete() == i27Partial {
			i27T.Fatal("input/output alias or repaired diagnostics", i27Got)
		}
		i27Denied(i27T, i27Got.Record)
	}
}

func TestI27CallbackFences(i27T *testing.T) {
	for _, i27Event := range []string{"replace", "retire", "cancel"} {
		i27T.Run(i27Event, func(i27T *testing.T) {
			i27Entered := make(chan struct{})
			i27Release := make(chan struct{})
			i27Done := make(chan error, 1)
			i27Calls := 0
			i27P := i27Provider(i27T, func(context.Context, OutBackOfflineAttempt) (OutBackDetachedRecordInput, error) {
				i27Calls++
				if i27Calls == 2 {
					close(i27Entered)
					<-i27Release
				}
				return i27Input(), nil
			})
			if i27Err := i27P.I27Cycle(context.Background(), time.Unix(1, 0)); i27Err != nil {
				i27T.Fatal(i27Err)
			}
			i27Before := i27P.I27Snapshot(i27Input().Source)
			i27Ctx, i27Cancel := context.WithCancel(context.Background())
			defer i27Cancel()
			go func() { i27Done <- i27P.I27Cycle(i27Ctx, time.Unix(2, 0)) }()
			select {
			case <-i27Entered:
			case <-time.After(2 * time.Second):
				i27T.Fatal("callback entry watchdog")
			}
			switch i27Event {
			case "replace":
				if i27Err := i27P.I27Replace(i27Input().Source); i27Err != nil {
					i27T.Fatal(i27Err)
				}
			case "retire":
				i27P.I27Retire()
			case "cancel":
				i27Cancel()
			}
			if i27Err := i27P.I27Cycle(context.Background(), time.Unix(3, 0)); !errors.Is(i27Err, ErrI27OutBackBusy) {
				i27T.Fatal("overlapping supplier allowed", i27Err)
			}
			close(i27Release)
			select {
			case i27Err := <-i27Done:
				if i27Err == nil {
					i27T.Fatal("late callback published")
				}
			case <-time.After(2 * time.Second):
				i27T.Fatal("callback completion watchdog")
			}
			i27After := i27P.I27Snapshot(i27Input().Source)
			i27Status := OutBackOfflineCanceled
			if i27Event == "replace" {
				i27Status = OutBackOfflineIdle
			}
			if i27Event == "retire" {
				i27Status = OutBackOfflineRetired
			}
			if i27After.Record != i27Before.Record || i27After.RetainedAt != time.Unix(1, 0) || i27After.RetainedAttempt != i27Before.RetainedAttempt || i27After.Status != i27Status || i27Calls != 2 {
				i27T.Fatal("late result changed history/status", i27After)
			}
			i27Denied(i27T, i27After.Record)
		})
	}
}
