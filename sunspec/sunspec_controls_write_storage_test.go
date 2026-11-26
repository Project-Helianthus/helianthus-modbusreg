package sunspec

import (
	"fmt"
	"slices"
	"testing"
)

// Model 124 register addresses of the fixture (header at 40060).
const (
	storageModeAddress   = fixtureStorageHeader + 5
	storageOutAddress    = fixtureStorageHeader + 12
	storageInAddress     = fixtureStorageHeader + 13
	storageWinAddress    = fixtureStorageHeader + 14
	storageRvrtAddress   = fixtureStorageHeader + 15
	storageHeaderWords   = 26
	storageUnrelatedMode = uint16(0x8004)
)

// storageImage applies function 16 requests to a copy of the model 124 words
// and records which register addresses were written.
type storageImage struct {
	words   []uint16
	written map[uint16]bool
	frames  []PointWriteRequest
}

func newStorageImage(t *testing.T, f controlsFixture) *storageImage {
	t.Helper()
	_, snapshot := f.chain(t)
	return &storageImage{words: snapshot.occurrences[fixtureStorageOrdinal-1].Words(), written: map[uint16]bool{}}
}

func (m *storageImage) apply(t *testing.T, step ControlWriteStep) {
	t.Helper()
	q := step.Request
	start := int(q.Address) - fixtureStorageHeader
	if q.Function != 16 || q.WordCount == 0 || len(q.Words) != int(q.WordCount) || start < 0 || start+int(q.WordCount) > len(m.words) {
		t.Fatalf("frame %+v is not a function 16 write inside model 124", q)
	}
	copy(m.words[start:], q.Words)
	for i := 0; i < int(q.WordCount); i++ {
		m.written[q.Address+uint16(i)] = true
	}
	m.frames = append(m.frames, q)
}

func (m *storageImage) requireOnlyWritten(t *testing.T, addresses ...uint16) {
	t.Helper()
	want := map[uint16]bool{}
	for _, a := range addresses {
		want[a] = true
	}
	for a := range m.written {
		if !want[a] {
			t.Errorf("register %d (offset %d) was written but is not part of the command", a, a-fixtureStorageHeader)
		}
	}
	for a := range want {
		if !m.written[a] {
			t.Errorf("register %d (offset %d) was not written", a, a-fixtureStorageHeader)
		}
	}
}

func (m *storageImage) requireFrames(t *testing.T, starts []uint16, widths []uint16) {
	t.Helper()
	var gotStarts, gotWidths []uint16
	for _, frame := range m.frames {
		gotStarts, gotWidths = append(gotStarts, frame.Address), append(gotWidths, frame.WordCount)
	}
	if !slices.Equal(gotStarts, starts) || !slices.Equal(gotWidths, widths) {
		t.Fatalf("frame starts %v widths %v, want starts %v widths %v", gotStarts, gotWidths, starts, widths)
	}
}

func runStorageCommand(t *testing.T, f controlsFixture, q StorageLimitRequest) (*storageImage, StorageLimitWrite) {
	t.Helper()
	registry, snapshot := f.chain(t)
	plan, err := EncodeStorageLimit(registry, snapshot, q)
	if err != nil {
		t.Fatal(err)
	}
	image := newStorageImage(t, f)
	for _, step := range plan.Steps {
		image.apply(t, step)
	}
	mode, err := plan.Mode.Request(f.mode)
	if err != nil {
		t.Fatal(err)
	}
	image.apply(t, mode)
	return image, plan
}

func TestSunSpecControlsWriteStorageChargeSetsOnlyTheChargeBit(t *testing.T) {
	f := defaultControlsFixture()
	f.mode = storageUnrelatedMode
	image, plan := runStorageCommand(t, f, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 2500}})
	// 2500 W of WChaMax 5000 W is 50 percent, word 500 in InOutWRte_SF -1.
	if want := []uint16{500}; len(plan.Steps) != 1 || !slices.Equal(plan.Steps[0].Request.Words, want) {
		t.Fatalf("steps = %+v, want one InWRte request %v", plan.Steps, want)
	}
	image.requireFrames(t, []uint16{storageInAddress, storageModeAddress}, []uint16{1, 1})
	image.requireOnlyWritten(t, storageInAddress, storageModeAddress)
	if image.words[13] != 500 || image.words[5] != storageUnrelatedMode|storageChargeBit {
		t.Fatalf("image InWRte=%d mode=%#x, want 500 and %#x", image.words[13], image.words[5], storageUnrelatedMode|storageChargeBit)
	}
	if image.words[12] != 111 || image.words[14] != 5 || image.words[15] != 6 || image.words[16] != 7 || image.words[2] != 5000 {
		t.Fatalf("an untouched register changed: %v", image.words)
	}
}

func TestSunSpecControlsWriteStorageDischargeSetsOnlyTheDischargeBit(t *testing.T) {
	f := defaultControlsFixture()
	f.mode = storageUnrelatedMode | storageChargeBit
	image, _ := runStorageCommand(t, f, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageDischarge, Watts: Decimal{Coefficient: 2500}})
	image.requireFrames(t, []uint16{storageOutAddress, storageModeAddress}, []uint16{1, 1})
	image.requireOnlyWritten(t, storageOutAddress, storageModeAddress)
	if image.words[12] != 500 || image.words[13] != 222 || image.words[5] != storageUnrelatedMode|storageChargeBit|storageDischargeBit {
		t.Fatalf("image OutWRte=%d InWRte=%d mode=%#x", image.words[12], image.words[13], image.words[5])
	}
}

func TestSunSpecControlsWriteStorageDurationIsItsOwnRequestBetweenRateAndMode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direction StorageDirection
		rate      uint16
		untouched uint16
	}{
		{"charge", StorageCharge, storageInAddress, storageOutAddress},
		{"discharge", StorageDischarge, storageOutAddress, storageInAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			image, plan := runStorageCommand(t, defaultControlsFixture(), StorageLimitRequest{
				Ordinal: fixtureStorageOrdinal, Direction: tc.direction, Watts: Decimal{Coefficient: 2500}, Duration: secondsPointer(1800),
			})
			if len(plan.Steps) != 2 || !slices.Equal(plan.Steps[1].Request.Words, []uint16{1800}) {
				t.Fatalf("steps = %+v, want the rate then the revert time", plan.Steps)
			}
			image.requireFrames(t, []uint16{tc.rate, storageRvrtAddress, storageModeAddress}, []uint16{1, 1, 1})
			image.requireOnlyWritten(t, tc.rate, storageRvrtAddress, storageModeAddress)
			if image.written[tc.untouched] || image.written[storageWinAddress] || image.written[storageWinAddress+2] {
				t.Fatal("the opposite rate or an intervening window or ramp word was written")
			}
			if image.words[14] != 5 || image.words[16] != 7 {
				t.Fatalf("window or ramp changed: %v", image.words)
			}
		})
	}
}

func TestSunSpecControlsWriteStorageRoundsDownInTheirBounds(t *testing.T) {
	eight := Decimal{Coefficient: 8000}
	for _, tc := range []struct {
		name         string
		direction    StorageDirection
		wChaMax      uint16
		maxDischarge *Decimal
		field        uint16
	}{
		{"charge of WChaMax", StorageCharge, 8000, nil, storageInAddress},
		{"discharge of a reported maximum", StorageDischarge, 5000, &eight, storageOutAddress},
		{"discharge of WChaMax without a separate maximum", StorageDischarge, 8000, nil, storageOutAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := defaultControlsFixture()
			f.wChaMax = tc.wChaMax
			registry, snapshot := f.chain(t)
			plan, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{
				Ordinal: fixtureStorageOrdinal, Direction: tc.direction, Watts: Decimal{Coefficient: 3333}, MaxDischarge: tc.maxDischarge,
			})
			if err != nil || plan.Steps[0].Request.Address != tc.field || !slices.Equal(plan.Steps[0].Request.Words, []uint16{416}) {
				t.Fatalf("plan = %+v err = %v, want 416 (41.6625 percent rounded down) at %d", plan.Steps, err, tc.field)
			}
		})
	}
	// The reported discharge maximum applies over WChaMax; a charge ignores it.
	f := defaultControlsFixture()
	registry, snapshot := f.chain(t)
	half := Decimal{Coefficient: 2500}
	discharge, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageDischarge, Watts: Decimal{Coefficient: 2500}, MaxDischarge: &half})
	if err != nil || discharge.Steps[0].Request.Words[0] != 1000 {
		t.Fatalf("discharge of a smaller reported maximum = %+v err %v, want 1000", discharge.Steps, err)
	}
	charge, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 2500}, MaxDischarge: &half})
	if err != nil || charge.Steps[0].Request.Words[0] != 500 {
		t.Fatalf("charge with a reported discharge maximum = %+v err %v, want 500", charge.Steps, err)
	}
}

func TestSunSpecControlsWriteStorageAnswersBeforeEncoding(t *testing.T) {
	zero, wide := Decimal{}, Decimal{Coefficient: 1, Exponent: 11}
	for _, tc := range []struct {
		name   string
		mutate func(*controlsFixture, *StorageLimitRequest)
		reason ControlWriteReason
	}{
		{"no model 124", func(f *controlsFixture, _ *StorageLimitRequest) { f.omitStorage = true }, ControlReasonModelAbsent},
		{"ordinal zero", func(_ *controlsFixture, q *StorageLimitRequest) { q.Ordinal = 0 }, ControlReasonModelAbsent},
		{"ordinal names model 123", func(_ *controlsFixture, q *StorageLimitRequest) { q.Ordinal = fixtureControlsOrdinal }, ControlReasonModelAbsent},
		{"charge without WChaMax", func(f *controlsFixture, _ *StorageLimitRequest) { f.wChaMax = 0xffff }, ControlReasonBoundAbsent},
		{"charge with WChaMax scale not implemented", func(f *controlsFixture, _ *StorageLimitRequest) { f.wChaMaxScale = 0x8000 }, ControlReasonBoundAbsent},
		{"charge with zero WChaMax", func(f *controlsFixture, _ *StorageLimitRequest) { f.wChaMax = 0 }, ControlReasonBoundAbsent},
		{"discharge without any bound", func(f *controlsFixture, q *StorageLimitRequest) { f.wChaMax = 0xffff; q.Direction = StorageDischarge }, ControlReasonBoundAbsent},
		{"discharge with a zero reported maximum", func(_ *controlsFixture, q *StorageLimitRequest) {
			q.Direction, q.MaxDischarge = StorageDischarge, &zero
		}, ControlReasonBoundAbsent},
		{"discharge with an out-of-range reported maximum", func(_ *controlsFixture, q *StorageLimitRequest) {
			q.Direction, q.MaxDischarge = StorageDischarge, &wide
		}, ControlReasonBoundAbsent},
		{"rate scale not implemented", func(f *controlsFixture, _ *StorageLimitRequest) { f.rateScale = 0x8000 }, ControlReasonPointUnavailable},
		{"above the bound", func(_ *controlsFixture, q *StorageLimitRequest) { q.Watts = Decimal{Coefficient: 5001} }, ControlReasonArgumentOutOfBound},
		{"negative", func(_ *controlsFixture, q *StorageLimitRequest) { q.Watts = Decimal{Coefficient: -1} }, ControlReasonArgumentOutOfBound},
		{"unknown direction", func(_ *controlsFixture, q *StorageLimitRequest) { q.Direction = 0 }, ControlReasonUnencodable},
		{"unusable duration", func(_ *controlsFixture, q *StorageLimitRequest) { q.Duration = secondsPointer(0xffff) }, ControlReasonUnencodable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := defaultControlsFixture()
			q := StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 100}}
			tc.mutate(&f, &q)
			registry, snapshot := f.chain(t)
			plan, err := EncodeStorageLimit(registry, snapshot, q)
			requireControlRefusal(t, err, tc.reason)
			if len(plan.Steps) != 0 || plan.Mode.bit != 0 {
				t.Fatalf("a refused command built %+v", plan)
			}
		})
	}
}

func TestSunSpecControlsWriteStorageReleaseClearsOnlyItsBit(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	for _, tc := range []struct {
		name      string
		direction StorageDirection
		bit       uint16
	}{
		{"charge", StorageCharge, storageChargeBit},
		{"discharge", StorageDischarge, storageDischargeBit},
	} {
		release, err := EncodeStorageRelease(registry, snapshot, fixtureStorageOrdinal, tc.direction)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []uint16{0, storageChargeBit | storageDischargeBit, storageUnrelatedMode | tc.bit, 0x7ffd | tc.bit} {
			step, err := release.Request(current)
			if err != nil {
				t.Fatal(err)
			}
			if step.Request.Function != 16 || step.Request.Address != storageModeAddress || step.Request.WordCount != 1 || step.Request.Words[0] != current&^tc.bit {
				t.Fatalf("%s release of %#x = %+v, want only bit %#x cleared at %d", tc.name, current, step.Request, tc.bit, storageModeAddress)
			}
		}
	}
	if _, err := EncodeStorageRelease(registry, snapshot, fixtureStorageOrdinal, 0); err == nil {
		t.Fatal("release of an unknown direction was encoded")
	}
	_, err := EncodeStorageRelease(registry, snapshot, fixtureControlsOrdinal, StorageCharge)
	requireControlRefusal(t, err, ControlReasonModelAbsent)
}

func TestSunSpecControlsWriteStorageModeRefusesAnUnestablishedRead(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	release, err := EncodeStorageRelease(registry, snapshot, fixtureStorageOrdinal, StorageCharge)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 100}})
	if err != nil {
		t.Fatal(err)
	}
	// Clearing a bit of the not-implemented word would otherwise look like a valid 0xfffe.
	for name, mode := range map[string]StorageModeWrite{"release": release, "set": plan.Mode} {
		if step, err := mode.Request(0xffff); err == nil || step.Request.WordCount != 0 {
			t.Fatalf("%s built a mode write from the not-implemented word: %+v %v", name, step, err)
		}
	}
	if _, err := (StorageModeWrite{}).Request(0); err == nil {
		t.Fatal("the zero mode write built a request")
	}
}

func TestSunSpecControlsWriteStorageTwoDirectionsAccumulateThroughTheMode(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	charge, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 100}})
	if err != nil {
		t.Fatal(err)
	}
	discharge, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{Ordinal: fixtureStorageOrdinal, Direction: StorageDischarge, Watts: Decimal{Coefficient: 100}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := charge.Mode.Request(storageUnrelatedMode)
	if err != nil {
		t.Fatal(err)
	}
	second, err := discharge.Mode.Request(first.Request.Words[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := storageUnrelatedMode | storageChargeBit | storageDischargeBit; second.Request.Words[0] != want {
		t.Fatalf("mode after both directions = %#x, want %#x", second.Request.Words[0], want)
	}
}

func TestSunSpecControlsWriteStorageEachPredicateAdmitsItsOwnSpanOnly(t *testing.T) {
	f := defaultControlsFixture()
	registry, snapshot := f.chain(t)
	plan, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{
		Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 100}, Duration: secondsPointer(60),
	})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := plan.Mode.Request(0)
	if err != nil {
		t.Fatal(err)
	}
	spans := []PointWriteRequest{plan.Steps[0].Request, plan.Steps[1].Request, mode.Request}
	for i, span := range spans {
		requireAdmitsOnlyItsSpan(t, fmt.Sprintf("span %d", i), span)
		for j, other := range spans {
			if i != j && span.Admission()(admissionRequest(other)) {
				t.Fatalf("span %d admitted span %d", i, j)
			}
		}
	}
	modeSpan := plan.Mode.Span()
	if modeSpan.Address != storageModeAddress || modeSpan.WordCount != 1 || !plan.Mode.Admission()(admissionRequest(mode.Request)) {
		t.Fatalf("mode admission span = %+v", modeSpan)
	}
	for _, other := range []PointWriteRequest{plan.Steps[0].Request, plan.Steps[1].Request} {
		if plan.Mode.Admission()(admissionRequest(other)) {
			t.Fatal("the mode predicate admitted a rate or revert span")
		}
	}
	// The release admits the same mode word and the opposite rate is outside every predicate.
	release, err := EncodeStorageRelease(registry, snapshot, fixtureStorageOrdinal, StorageCharge)
	if err != nil {
		t.Fatal(err)
	}
	if !release.Admission()(admissionRequest(mode.Request)) {
		t.Fatal("the release predicate does not admit the mode word")
	}
	opposite := PointWriteRequest{Function: 16, Address: storageOutAddress, WordCount: 1, Words: []uint16{0}}
	for _, span := range spans {
		if span.Admission()(admissionRequest(opposite)) {
			t.Fatal("a charge predicate admitted the discharge rate")
		}
	}
	for _, hand := range []string{"storage.control.rate_window", "storage.control.rate_ramp", "storage.control.rate_revert", "storage.control.charge_rate"} {
		request := requireHandRequest(t, registry, snapshot, fixtureStorageOrdinal, hand)
		requireAdmitsOnlyItsSpan(t, hand, request)
	}
}

func TestSunSpecControlsWriteStorageReadbackEchoOfTheFirstStepIsNotEnough(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	plan, err := EncodeStorageLimit(registry, snapshot, StorageLimitRequest{
		Ordinal: fixtureStorageOrdinal, Direction: StorageCharge, Watts: Decimal{Coefficient: 2500}, Duration: secondsPointer(60),
	})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := plan.Mode.Request(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		reads [][]uint16
		want  ReadbackState
	}{
		{"every point re-read as written", [][]uint16{{500}, {60}, {storageChargeBit}}, ReadbackMatch},
		{"only the first step re-read", [][]uint16{{500}}, ReadbackUnknown},
		{"no step re-read", nil, ReadbackUnknown},
		{"mode not re-read", [][]uint16{{500}, {60}, nil}, ReadbackUnknown},
		{"mode re-read without the bit", [][]uint16{{500}, {60}, {0}}, ReadbackMismatch},
		{"revert time not implemented", [][]uint16{{500}, {0xffff}, {storageChargeBit}}, ReadbackUnknown},
		{"rate differs", [][]uint16{{501}, {60}, {storageChargeBit}}, ReadbackMismatch},
		{"a re-read beyond the plan", [][]uint16{{500}, {60}, {storageChargeBit}, {0}}, ReadbackUnknown},
	} {
		if got := plan.CompareReadback(mode, tc.reads...); got != tc.want {
			t.Errorf("%s: readback = %s, want %s", tc.name, got, tc.want)
		}
	}
}
