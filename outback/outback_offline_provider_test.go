package outback

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func offlineInput() DetachedRecordInput {
	return DetachedRecordInput{Words: []uint16{7, 8}, DeclaredWords: 2, Present: []WordSpan{{0, 2}}, Source: SourceAssociation{Key: "family-source", Revision: "one", PublicLocator: "public/source", ContentIdentity: "synthetic-content", DeclaredLayout: "opaque", SourceSetRevision: "set-one", SelectorInputs: "synthetic", Available: true}, Occurrences: []DetachedOccurrence{
		{Ordinal: 3, WireKey: modbusreg.SunSpecWireKey{ModelID: 64111, ModelLength: 1}, Words: []uint16{7}, Present: []WordSpan{{0, 1}}},
		{Ordinal: 5, WireKey: modbusreg.SunSpecWireKey{ModelID: 64111, ModelLength: 1}, Offset: 1, Words: []uint16{8}, Present: []WordSpan{{0, 1}}},
	}}
}
func outBackOfflineProvider(t *testing.T, supplier DiagnosticSupplier) *OfflineProvider {
	t.Helper()
	p, err := NewOfflineProvider(offlineInput().Source, 4096, time.Second, supplier)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func offlineNoGo(t *testing.T, r *DetachedRecord) {
	t.Helper()
	if r.Evaluate().Matched() || r.Evaluate().Reason() != AXSReadOnlyFlavorReasonSourceNotQualified || len(r.TypedFacts()) != 0 || len(r.Requests()) != 0 {
		t.Fatal("admission or output")
	}
}
func TestOutbackOfflineProviderBounds(t *testing.T) {
	calls := 0
	supplier := func(context.Context, OfflineAttempt) (DetachedRecordInput, error) {
		calls++
		return offlineInput(), nil
	}
	for _, c := range []struct {
		bytes uint64
		work  time.Duration
		fn    DiagnosticSupplier
	}{{0, time.Second, supplier}, {4096, 0, supplier}, {4096, -1, supplier}, {4096, time.Second, nil}, {1, time.Second, supplier}} {
		if p, err := NewOfflineProvider(offlineInput().Source, c.bytes, c.work, c.fn); err == nil || p != nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	p := outBackOfflineProvider(t, supplier)
	// Deliberately exercise nil-context rejection before the supplier is invoked.
	var nilContext context.Context
	if err := p.Cycle(nilContext, time.Unix(1, 0)); err == nil {
		t.Fatal("nil context accepted")
	}
	if err := p.Cycle(context.Background(), time.Time{}); err == nil {
		t.Fatal("missing receipt time accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Cycle(ctx, time.Unix(1, 0)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("invalid input invoked supplier")
	}
	source := offlineInput().Source
	source.Key = string(make([]byte, 5000))
	if err := p.Replace(source); err == nil {
		t.Fatal("oversize source accepted")
	}
	if err := p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestOutbackOfflineProviderRetention(t *testing.T) {
	input := offlineInput()
	var supplierErr error
	p := outBackOfflineProvider(t, func(_ context.Context, a OfflineAttempt) (DetachedRecordInput, error) {
		if a.Generation != 1 || a.Number == 0 || a.Source != offlineInput().Source {
			t.Error("attempt not frozen")
		}
		return input, supplierErr
	})
	first := time.Unix(10, 0)
	if err := p.Cycle(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	s := p.Snapshot(input.Source)
	if s.Record == nil || !s.Record.Complete() || s.Association != AssociationCurrent || s.Status != OfflineImported || s.RetainedAt != first {
		t.Fatal(s)
	}
	want := offlineInput()
	input.Words[0] = 99
	input.Present[0].Offset = 99
	input.Occurrences[0].Words[0] = 99
	input.Occurrences[0].Present[0].Offset = 99
	input.Occurrences[1].Ordinal = 99
	if !reflect.DeepEqual(s.Record.RawWords(), want.Words) || !reflect.DeepEqual(s.Record.Occurrences(), want.Occurrences) || !reflect.DeepEqual(s.Record.PresentSpans(), want.Present) {
		t.Fatal("aliased supplier slices")
	}
	for _, kind := range []string{"error", "oversize", "incomplete", "malformed"} {
		input = offlineInput()
		supplierErr = nil
		switch kind {
		case "error":
			supplierErr = errors.New("synthetic failure")
		case "oversize":
			input.Words = make([]uint16, 4096)
		case "incomplete":
			input.Missing = []WordSpan{{1, 1}}
		case "malformed":
			input.Present = []WordSpan{{^uint64(0), 2}}
		}
		err := p.Cycle(context.Background(), time.Unix(20, 0))
		if (kind == "error" || kind == "oversize") && err == nil {
			t.Fatal("failure accepted")
		}
		if (kind == "incomplete" || kind == "malformed") && err != nil {
			t.Fatal(err)
		}
		got := p.Snapshot(want.Source)
		if got.Record != s.Record || got.RetainedAt != first || got.RetainedAttempt != s.RetainedAttempt || got.LatestAttempt.Number != s.LatestAttempt.Number+1 {
			t.Fatal("failure/partial replaced record or renewed age", kind, got)
		}
		if (kind == "error" || kind == "oversize") && got.Status != OfflineFailed {
			t.Fatal(got.Status)
		}
		if (kind == "incomplete" || kind == "malformed") && got.Status != OfflineIncomplete {
			t.Fatal(got.Status)
		}
		offlineNoGo(t, got.Record)
		s = got
	}
}
func TestOutbackOfflineProviderDiagnostic(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		i := offlineInput()
		i.DeclaredWords = 3
		i.Missing = []WordSpan{{2, 1}}
		i.Occurrences[0].Missing = []WordSpan{{1, 2}}
		if malformed {
			i.Present = []WordSpan{{^uint64(0), 2}}
		}
		p := outBackOfflineProvider(t, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) { return i, nil })
		if err := p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		s := p.Snapshot(i.Source)
		if s.Status != OfflineIncomplete || s.Record == nil || s.Record.Complete() || !reflect.DeepEqual(s.Record.Occurrences(), i.Occurrences) || !reflect.DeepEqual(s.Record.MissingSpans(), i.Missing) || !reflect.DeepEqual(s.Record.PresentSpans(), i.Present) {
			t.Fatal("diagnostics lost")
		}
		i.Missing[0].Offset = 9
		i.Occurrences[0].Missing[0].Offset = 9
		if s.Record.MissingSpans()[0].Offset != 2 || s.Record.Occurrences()[0].Missing[0].Offset != 1 {
			t.Fatal("aliased missing spans")
		}
		offlineNoGo(t, s.Record)
	}
}
func offlineChanges() map[string]func(*SourceAssociation) {
	return map[string]func(*SourceAssociation){
		"Key": func(s *SourceAssociation) { s.Key = "other" }, "Revision": func(s *SourceAssociation) { s.Revision = "two" }, "PublicLocator": func(s *SourceAssociation) { s.PublicLocator = "public/other" }, "ContentIdentity": func(s *SourceAssociation) { s.ContentIdentity = "different-synthetic-content" }, "DeclaredLayout": func(s *SourceAssociation) { s.DeclaredLayout = "conflict" }, "SourceSetRevision": func(s *SourceAssociation) { s.SourceSetRevision = "set-two" }, "SelectorInputs": func(s *SourceAssociation) { s.SelectorInputs = "other" }, "Available": func(s *SourceAssociation) { s.Available = false },
	}
}
func TestOutbackOfflineProviderAssociation(t *testing.T) {
	for name, change := range offlineChanges() {
		t.Run(name, func(t *testing.T) {
			p := outBackOfflineProvider(t, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) {
				return offlineInput(), nil
			})
			if err := p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
				t.Fatal(err)
			}
			current := offlineInput().Source
			change(&current)
			s := p.Snapshot(current)
			if s.Association != AssociationHistorical || s.Record.SourceAssociation() != offlineInput().Source {
				t.Fatal("relabeled association")
			}
			if err := p.Replace(current); err != nil {
				t.Fatal(err)
			}
			if p.Snapshot(current).LatestAttempt != s.LatestAttempt {
				t.Fatal("replacement relabeled latest attempted context")
			}
			if p.Snapshot(offlineInput().Source).Association != AssociationHistorical {
				t.Fatal("previous descriptor still current after replacement")
			}
			if err := p.Cycle(context.Background(), time.Unix(2, 0)); !errors.Is(err, ErrOutBackOfflineAssociation) {
				t.Fatal("mismatched supplier association accepted", err)
			}
			got := p.Snapshot(current)
			if got.Record != s.Record || got.RetainedAt != s.RetainedAt || got.Status != OfflineFailed {
				t.Fatal("mismatch committed")
			}
		})
	}
	p := outBackOfflineProvider(t, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) {
		return offlineInput(), nil
	})
	if err := p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if p.Snapshot(SourceAssociation{}).Association != AssociationHistorical {
		t.Fatal("missing source current")
	}
}
func TestOutbackOfflineProviderFences(t *testing.T) {
	for _, event := range []string{"generation", "attempt", "retire", "cancel", "deadline"} {
		t.Run(event, func(t *testing.T) {
			entered := make(chan OfflineAttempt, 1)
			release := make(chan struct{})
			done := make(chan error, 1)
			p := outBackOfflineProvider(t, func(_ context.Context, a OfflineAttempt) (DetachedRecordInput, error) {
				if a.Number == 1 {
					entered <- a
					<-release
				}
				return offlineInput(), nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var manual *offlineDeadlineContext
			if event == "deadline" {
				manual = &offlineDeadlineContext{Context: ctx}
				ctx = manual
			}
			go func() { done <- p.Cycle(ctx, time.Unix(1, 0)) }()
			var frozen OfflineAttempt
			select {
			case frozen = <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("supplier not entered")
			}
			if frozen.Generation != 1 || frozen.Number != 1 {
				t.Fatal(frozen)
			}
			switch event {
			case "generation":
				if err := p.Replace(offlineInput().Source); err != nil {
					t.Fatal(err)
				}
			case "attempt":
				if err := p.Cycle(context.Background(), time.Unix(2, 0)); err != nil {
					t.Fatal(err)
				}
			case "retire":
				p.Retire()
			case "cancel":
				cancel()
			case "deadline":
				manual.expired.Store(true)
			}
			before := p.Snapshot(offlineInput().Source)
			close(release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("late completion committed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cycle hung")
			}
			after := p.Snapshot(offlineInput().Source)
			if after.Record != before.Record || after.RetainedAt != before.RetainedAt {
				t.Fatal("fence replaced record")
			}
			if event == "generation" || event == "attempt" || event == "retire" {
				if after.Status != before.Status || after.LatestAttempt != before.LatestAttempt {
					t.Fatal("late completion changed latest status")
				}
			}
			if event == "cancel" || event == "deadline" {
				if after.Status != OfflineCanceled {
					t.Fatal(after.Status)
				}
			}
			if event == "retire" {
				if err := p.Cycle(context.Background(), time.Unix(3, 0)); !errors.Is(err, ErrOutBackOfflineRetired) {
					t.Fatal(err)
				}
				if err := p.Replace(offlineInput().Source); !errors.Is(err, ErrOutBackOfflineRetired) {
					t.Fatal(err)
				}
			}
		})
	}
}

// The barrier orders expiry before the supplier returns, without wall-clock sleep.
type offlineDeadlineContext struct {
	context.Context
	expired atomic.Bool
}

func (c *offlineDeadlineContext) Err() error {
	if c.expired.Load() {
		return context.DeadlineExceeded
	}
	return c.Context.Err()
}

func TestOutbackOfflineProviderSuccessfulReplacement(t *testing.T) {
	input := offlineInput()
	p := outBackOfflineProvider(t, func(ctx context.Context, a OfflineAttempt) (DetachedRecordInput, error) {
		if deadline, ok := ctx.Deadline(); !ok || deadline.After(time.Now().Add(time.Second)) {
			t.Error("missing bounded work deadline")
		}
		input.Source = a.Source
		return input, nil
	})
	if err := p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	old := p.Snapshot(input.Source)
	next := input.Source
	next.Revision = "next"
	if err := p.Replace(next); err != nil {
		t.Fatal(err)
	}
	input.Words = []uint16{9, 10}
	input.Occurrences[0].Words = []uint16{9}
	input.Occurrences[1].Words = []uint16{10}
	if err := p.Cycle(context.Background(), time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	got := p.Snapshot(next)
	if got.Record == old.Record || got.RetainedAt != time.Unix(2, 0) || got.Record.SourceAssociation() != next || got.RetainedAttempt.Generation != 2 || got.RetainedAttempt.Number != 2 || got.LatestAttempt != got.RetainedAttempt || got.Association != AssociationCurrent || !reflect.DeepEqual(got.Record.RawWords(), []uint16{9, 10}) {
		t.Fatal(got)
	}
	p.Retire()
	retired := p.Snapshot(next)
	if retired.Record != got.Record || retired.RetainedAt != got.RetainedAt || retired.Association != AssociationHistorical {
		t.Fatal("retirement lost historical evidence")
	}
	offlineNoGo(t, got.Record)
}
func TestOutbackOfflineProviderUnavailable(t *testing.T) {
	input := offlineInput()
	input.Source.Available = false
	p, err := NewOfflineProvider(input.Source, 4096, time.Second, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) { return input, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Cycle(context.Background(), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	s := p.Snapshot(input.Source)
	if s.Record == nil || s.Association != AssociationHistorical || s.Record.SourceAssociation().Available {
		t.Fatal(s)
	}
	offlineNoGo(t, s.Record)
}
func TestOutbackOfflineProviderZeroAndExhaustion(t *testing.T) {
	var zero OfflineProvider
	if err := zero.Cycle(context.Background(), time.Unix(1, 0)); err == nil {
		t.Fatal("zero cycle accepted")
	}
	if err := zero.Replace(offlineInput().Source); err == nil {
		t.Fatal("zero replacement accepted")
	}
	s := zero.Snapshot(offlineInput().Source)
	if s.Record != nil || s.Association != AssociationHistorical {
		t.Fatal(s)
	}
	offlineNoGo(t, s.Record)
	calls := 0
	p := outBackOfflineProvider(t, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) {
		calls++
		return offlineInput(), nil
	})
	p.attempt = ^uint64(0)
	if err := p.Cycle(context.Background(), time.Unix(1, 0)); err == nil || calls != 0 {
		t.Fatal("attempt wrapped")
	}
	p.generation = ^uint64(0)
	if err := p.Replace(offlineInput().Source); err == nil {
		t.Fatal("generation wrapped")
	}
}

// Models cancellation between validation and publication using logical Err calls.
// No Done watcher or wall-clock race participates in this deterministic context.
type offlinePublicationCancel struct {
	context.Context
	armed  bool
	checks int
}

func (c *offlinePublicationCancel) Err() error {
	if c.armed {
		c.checks++
		if c.checks > 1 {
			return context.Canceled
		}
	}
	return nil
}
func TestOutbackOfflineProviderPublicationCancel(t *testing.T) {
	ctx := &offlinePublicationCancel{Context: context.Background()}
	p := outBackOfflineProvider(t, func(context.Context, OfflineAttempt) (DetachedRecordInput, error) {
		ctx.armed = true
		return offlineInput(), nil
	})
	if err := p.Cycle(ctx, time.Unix(1, 0)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s := p.Snapshot(offlineInput().Source)
	if s.Record != nil || !s.RetainedAt.IsZero() || s.Status != OfflineCanceled {
		t.Fatal("publication ignored cancellation", s)
	}
}
