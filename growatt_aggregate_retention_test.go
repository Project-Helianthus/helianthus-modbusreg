package modbusreg

import (
	"reflect"
	"sync"
	"testing"
)

func retentionInput() GrowattBMSReadOnlyInput {
	return GrowattBMSReadOnlyInput{UnitID: 1, Function: FunctionReadHoldingRegisters,
		Revision: GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		Slices: []GrowattBMSReadOnlySlice{
			{Offset: 1, Words: []uint16{0x0102, 0x0304, 11, 12, 13, 14, 15}},
			{Offset: 13, Words: []uint16{0x0204, 0x0301, 0, 0, 0, 0, 2, 0, 75, 5200, 100, 25, 0, 3200, 5000, 0, 0, 110, 0x0300, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32}},
			{Offset: 256, Words: []uint16{100, 123, 3300, 0, 512, 5, 6, 0, 0, 0x8001, 0, 0}},
			{Offset: 269, Words: []uint16{17, 18}},
		}}
}

func retentionContext() GrowattAggregateContext {
	return GrowattAggregateContext{SourceInstance: "synthetic-source", OwnerEpoch: 1, UnitID: 1,
		Function: FunctionReadHoldingRegisters, Revision: retentionInput().Revision, IdentityKnown: true,
		Identity: GrowattAggregateIdentity{MCUSoftwareVersion: "1.2", BMSCompany: 4, BMSGeneration: 2, PackCompany: 1, PackGeneration: 3, Topology: 0x0300, CellSeriesCount: 32}}
}

func retentionStore(t *testing.T) *GrowattAggregateRetention {
	t.Helper()
	r, err := NewGrowattAggregateRetention(10)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func retentionBegin(t *testing.T, r *GrowattAggregateRetention, c GrowattAggregateContext, cycle, now uint64) {
	t.Helper()
	if err := r.Begin(c, cycle, now, now+10); err != nil {
		t.Fatal(err)
	}
}
func retentionSuccess(t *testing.T, r *GrowattAggregateRetention, c GrowattAggregateContext, cycle, now uint64, in GrowattBMSReadOnlyInput) {
	t.Helper()
	if err := r.Complete(c, cycle, now, in); err != nil {
		t.Fatal(err)
	}
}
func retentionSeed(t *testing.T) (*GrowattAggregateRetention, GrowattAggregateContext) {
	t.Helper()
	r := retentionStore(t)
	c := retentionContext()
	retentionBegin(t, r, c, 1, 1)
	retentionSuccess(t, r, c, 1, 2, retentionInput())
	return r, c
}

func TestGrowattAggregateRetentionReplacementAndFailure(t *testing.T) {
	r := retentionStore(t)
	c := retentionContext()
	if s := r.Snapshot(); s.State != "absent" || s.Value != nil || s.SuccessRevision != 0 {
		t.Fatalf("initial: %+v", s)
	}
	retentionBegin(t, r, c, 1, 1)
	if s := r.Snapshot(); s.State != "absent" || s.Attempt.Outcome != "pending" {
		t.Fatalf("pending: %+v", s)
	}
	retentionSuccess(t, r, c, 1, 2, retentionInput())
	first := r.Snapshot()
	if first.State != "current" || first.SuccessRevision != 1 || first.SuccessCycle != 1 || first.SuccessTick != 2 || first.Value.PackCurrentAmps != 1 {
		t.Fatalf("first: %+v", first)
	}
	retentionBegin(t, r, c, 2, 3)
	if s := r.Snapshot(); s.State != "retained" || s.Attempt.Outcome != "pending" {
		t.Fatalf("pending prior: %+v", s)
	}
	if err := r.Fail(c, 2, 4); err != nil {
		t.Fatal(err)
	}
	s := r.Snapshot()
	if s.State != "retained" || s.Attempt.Cycle != 2 || s.Attempt.Outcome != "failed" || s.SuccessRevision != 1 || s.SuccessTick != 2 || !reflect.DeepEqual(s.Value, first.Value) {
		t.Fatalf("failed: %+v", s)
	}
	retentionBegin(t, r, c, 3, 5)
	in := retentionInput()
	in.Slices[1].Words[10] = 0
	in.Slices[0].Words[6] = 91
	in.Slices[2].Words[11] = 92
	in.Slices[3].Words[1] = 93
	retentionSuccess(t, r, c, 3, 6, in)
	s = r.Snapshot()
	if s.State != "current" || s.SuccessRevision != 2 || s.SuccessCycle != 3 || s.SuccessTick != 6 || s.Value.PackCurrentAmps != 0 || !reflect.DeepEqual(s.Value.NativeObservation().Slices(), in.Slices) {
		t.Fatalf("zero/replacement: %+v", s)
	}
	retentionBegin(t, r, c, 4, 7)
	retentionSuccess(t, r, c, 4, 8, in)
	if s = r.Snapshot(); s.SuccessRevision != 3 || s.SuccessCycle != 4 || s.SuccessTick != 8 {
		t.Fatalf("equal success: %+v", s)
	}
}

func TestGrowattAggregateRetentionMalformedIsAtomic(t *testing.T) {
	for _, kind := range []string{"missing", "short", "encoding", "empty"} {
		t.Run(kind, func(t *testing.T) {
			r, c := retentionSeed(t)
			before := r.Snapshot()
			retentionBegin(t, r, c, 2, 3)
			in := retentionInput()
			in.Slices[1].Words[9] = 1111
			in.Slices[2].Words[0] = 222
			switch kind {
			case "missing":
				in.Slices = in.Slices[:3]
			case "short":
				in.Slices[3].Words = in.Slices[3].Words[:1]
			case "encoding":
				in.Slices[1].Words[8] = 101
			case "empty":
				in = GrowattBMSReadOnlyInput{}
			}
			if err := r.Complete(c, 2, 4, in); err == nil {
				t.Fatal("malformed completion accepted")
			}
			s := r.Snapshot()
			if s.SuccessRevision != 1 || s.SuccessTick != 2 || s.Attempt.Outcome != "failed" || !reflect.DeepEqual(s.Value, before.Value) {
				t.Fatalf("partial merge: %+v", s)
			}
		})
	}
}

func TestGrowattAggregateRetentionContinuityAxes(t *testing.T) {
	axes := map[string]func(*GrowattAggregateContext){
		"source":              func(c *GrowattAggregateContext) { c.SourceInstance = "replacement" },
		"epoch":               func(c *GrowattAggregateContext) { c.OwnerEpoch++ },
		"unit":                func(c *GrowattAggregateContext) { c.UnitID++ },
		"function":            func(c *GrowattAggregateContext) { c.Function = FunctionReadInputRegisters },
		"revision-family":     func(c *GrowattAggregateContext) { c.Revision.Family = "other" },
		"revision-file":       func(c *GrowattAggregateContext) { c.Revision.FileRevision = "other" },
		"revision-header":     func(c *GrowattAggregateContext) { c.Revision.HeaderVersion = "other" },
		"revision-cumulative": func(c *GrowattAggregateContext) { c.Revision.CumulativeRevision = "other" },
		"MCU":                 func(c *GrowattAggregateContext) { c.Identity.MCUSoftwareVersion = "1.3" },
		"BMS-company":         func(c *GrowattAggregateContext) { c.Identity.BMSCompany++ },
		"BMS-generation":      func(c *GrowattAggregateContext) { c.Identity.BMSGeneration++ },
		"pack-company":        func(c *GrowattAggregateContext) { c.Identity.PackCompany++ },
		"pack-generation":     func(c *GrowattAggregateContext) { c.Identity.PackGeneration++ },
		"topology":            func(c *GrowattAggregateContext) { c.Identity.Topology++ },
		"cell-series":         func(c *GrowattAggregateContext) { c.Identity.CellSeriesCount++ },
		"unknown":             func(c *GrowattAggregateContext) { c.IdentityKnown = false },
	}
	for name, mutate := range axes {
		t.Run(name, func(t *testing.T) {
			r, c := retentionSeed(t)
			changed := c
			mutate(&changed)
			retentionBegin(t, r, changed, 2, 3)
			s := r.Snapshot()
			if s.State != "invalidated" || s.Value != nil || s.SuccessRevision != 1 || s.SuccessTick != 2 {
				t.Fatalf("old value exposed: %+v", s)
			}
			if err := r.Fail(changed, 2, 4); err != nil {
				t.Fatal(err)
			}
			if s = r.Snapshot(); s.State != "invalidated" || s.Value != nil {
				t.Fatalf("failure restored old value: %+v", s)
			}
			recovered := c
			if changed.OwnerEpoch > c.OwnerEpoch {
				recovered.OwnerEpoch = changed.OwnerEpoch + 1
			}
			retentionBegin(t, r, recovered, 3, 5)
			if s = r.Snapshot(); s.Value != nil {
				t.Fatal("returning key resurrected invalidated value")
			}
			retentionSuccess(t, r, recovered, 3, 6, retentionInput())
			if s = r.Snapshot(); s.State != "current" || s.SuccessRevision != 2 {
				t.Fatalf("recovery: %+v", s)
			}
		})
	}
}

func TestGrowattAggregateRetentionNativeIdentityEvidence(t *testing.T) {
	axes := map[string]func(*GrowattBMSReadOnlyInput){
		"unit":                func(in *GrowattBMSReadOnlyInput) { in.UnitID++ },
		"function":            func(in *GrowattBMSReadOnlyInput) { in.Function = FunctionReadInputRegisters },
		"revision-family":     func(in *GrowattBMSReadOnlyInput) { in.Revision.Family = "other" },
		"revision-file":       func(in *GrowattBMSReadOnlyInput) { in.Revision.FileRevision = "other" },
		"revision-header":     func(in *GrowattBMSReadOnlyInput) { in.Revision.HeaderVersion = "other" },
		"revision-cumulative": func(in *GrowattBMSReadOnlyInput) { in.Revision.CumulativeRevision = "other" },
		"MCU":                 func(in *GrowattBMSReadOnlyInput) { in.Slices[0].Words[0]++ },
		"BMS-company":         func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[0]++ },
		"BMS-generation":      func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[0] += 0x100 },
		"pack-company":        func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[1]++ },
		"pack-generation":     func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[1] += 0x100 },
		"topology":            func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[18]++ },
		"cell-series":         func(in *GrowattBMSReadOnlyInput) { in.Slices[1].Words[28]++ },
	}
	for name, mutate := range axes {
		t.Run(name, func(t *testing.T) {
			r, c := retentionSeed(t)
			retentionBegin(t, r, c, 2, 3)
			in := retentionInput()
			mutate(&in)
			if err := r.Complete(c, 2, 4, in); err == nil {
				t.Fatal("contradictory native evidence accepted")
			}
			s := r.Snapshot()
			if s.State != "invalidated" || s.Value != nil || s.SuccessRevision != 1 || s.SuccessTick != 2 {
				t.Fatalf("contradiction: %+v", s)
			}
		})
	}
	// A coherent changed identity starts a new lineage only on complete success.
	r, c := retentionSeed(t)
	c.Identity.Topology++
	in := retentionInput()
	in.Slices[1].Words[18]++
	retentionBegin(t, r, c, 2, 3)
	retentionSuccess(t, r, c, 2, 4, in)
	if s := r.Snapshot(); s.State != "current" || s.SuccessRevision != 2 || s.SuccessContext.Identity.Topology != 0x0301 {
		t.Fatalf("new lineage: %+v", s)
	}
}

func TestGrowattAggregateRetentionFencesAndDuplicates(t *testing.T) {
	r, c := retentionSeed(t)
	before := r.Snapshot()
	if err := r.Complete(c, 1, 2, retentionInput()); err != nil {
		t.Fatal("identical retransmission", err)
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("duplicate refreshed success")
	}
	conflict := retentionInput()
	conflict.Slices[1].Words[9] = 1234
	if err := r.Complete(c, 1, 2, conflict); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	if err := r.Fail(c, 1, 2); err == nil {
		t.Fatal("conflicting failure accepted")
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("conflict changed accepted record")
	}
	retentionBegin(t, r, c, 2, 3)
	for _, axis := range []string{"cycle", "source", "epoch", "unit", "function", "revision", "identity", "unknown"} {
		t.Run(axis, func(t *testing.T) {
			cb := c
			cycle := uint64(2)
			switch axis {
			case "cycle":
				cycle = 1
			case "source":
				cb.SourceInstance = "other"
			case "epoch":
				cb.OwnerEpoch++
			case "unit":
				cb.UnitID++
			case "function":
				cb.Function = FunctionReadInputRegisters
			case "revision":
				cb.Revision.HeaderVersion = "other"
			case "identity":
				cb.Identity.CellSeriesCount++
			case "unknown":
				cb.IdentityKnown = false
			}
			snap := r.Snapshot()
			if err := r.Complete(cb, cycle, 4, retentionInput()); err == nil {
				t.Fatal("wrong callback accepted")
			}
			if err := r.Fail(cb, cycle, 4); err == nil {
				t.Fatal("wrong failure accepted")
			}
			if !reflect.DeepEqual(snap, r.Snapshot()) {
				t.Fatal("wrong callback changed state")
			}
		})
	}
	retentionSuccess(t, r, c, 2, 4, retentionInput())
	c.OwnerEpoch++
	retentionBegin(t, r, c, 1, 5)
	old := c
	old.OwnerEpoch--
	if err := r.Complete(old, 1, 6, retentionInput()); err == nil {
		t.Fatal("old epoch accepted")
	}
	if err := r.Begin(old, 99, 6, 16); err == nil {
		t.Fatal("old epoch reopened")
	}
	retentionSuccess(t, r, c, 1, 6, retentionInput())
	if s := r.Snapshot(); s.SuccessRevision != 3 || s.SuccessContext.OwnerEpoch != 2 {
		t.Fatalf("epoch: %+v", s)
	}
	if err := r.Begin(c, 1, 7, 17); err == nil {
		t.Fatal("cycle reused")
	}
}

func TestGrowattAggregateRetentionDeadlineAndBounds(t *testing.T) {
	if _, err := NewGrowattAggregateRetention(0); err == nil {
		t.Fatal("unbounded deadline accepted")
	}
	r := retentionStore(t)
	c := retentionContext()
	for _, d := range []uint64{0, 1, 12} {
		if err := r.Begin(c, 1, 1, d); err == nil {
			t.Fatal("invalid deadline accepted", d)
		}
	}
	retentionBegin(t, r, c, 1, 1)
	if err := r.Advance(10); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().Attempt.Outcome != "pending" {
		t.Fatal("closed before deadline")
	}
	if err := r.Complete(c, 1, 11, retentionInput()); err == nil {
		t.Fatal("deadline completion accepted")
	}
	if s := r.Snapshot(); s.State != "absent" || s.Attempt.Outcome != "incomplete" {
		t.Fatalf("deadline: %+v", s)
	}
	retentionBegin(t, r, c, 2, 12)
	retentionSuccess(t, r, c, 2, 13, retentionInput())
	retentionBegin(t, r, c, 3, 14)
	if err := r.Advance(24); err != nil {
		t.Fatal(err)
	}
	if s := r.Snapshot(); s.State != "retained" || s.SuccessTick != 13 || s.SuccessRevision != 1 || s.Attempt.Outcome != "incomplete" {
		t.Fatalf("expiry refreshed age: %+v", s)
	}
	if err := r.Advance(23); err == nil {
		t.Fatal("logical time reversal accepted")
	}
	retentionBegin(t, r, c, 4, 25)
	retentionBegin(t, r, c, 5, 26)
	if err := r.Complete(c, 4, 27, retentionInput()); err == nil {
		t.Fatal("superseded cycle accepted")
	}
	for cycle := uint64(6); cycle < 106; cycle++ {
		retentionBegin(t, r, c, cycle, cycle+30)
		retentionSuccess(t, r, c, cycle, cycle+31, retentionInput())
	}
	if s := r.Snapshot(); s.SuccessRevision != 101 || s.SuccessCycle != 105 {
		t.Fatalf("bounded long sequence: %+v", s)
	}
}

func TestGrowattAggregateRetentionDetachedStorage(t *testing.T) {
	r := retentionStore(t)
	c := retentionContext()
	in := retentionInput()
	retentionBegin(t, r, c, 1, 1)
	retentionSuccess(t, r, c, 1, 2, in)
	control := retentionInput()
	for i := range in.Slices {
		in.Slices[i].Offset++
		for j := range in.Slices[i].Words {
			in.Slices[i].Words[j]++
		}
	}
	s := r.Snapshot()
	if !reflect.DeepEqual(s.Value.NativeObservation().Slices(), control.Slices) {
		t.Fatal("input aliases stored evidence")
	}
	s.Value.PackCurrentAmps = 999
	s.Value.Revision.Family = "mutated"
	raw := s.Value.NativeObservation().Slices()
	for i := range raw {
		raw[i].Words[0]++
	}
	// Same-package access also probes the storage behind the exported typed value.
	for i := range s.Value.nativeObservation.slices {
		s.Value.nativeObservation.slices[i].Words[0]++
	}
	after := r.Snapshot()
	if after.Value.PackCurrentAmps != 1 || after.Value.Revision.Family != "1xSxxP ESS" || !reflect.DeepEqual(after.Value.NativeObservation().Slices(), control.Slices) {
		t.Fatal("snapshot aliases retained record")
	}
}

func TestGrowattAggregateRetentionAdmissionBoundary(t *testing.T) {
	for _, axis := range []string{"family", "file", "header", "cumulative", "function", "unit", "unknown"} {
		t.Run(axis, func(t *testing.T) {
			r, c := retentionSeed(t)
			in := retentionInput()
			switch axis {
			case "family":
				in.Revision.Family = "unsupported"
				c.Revision = in.Revision
			case "file":
				in.Revision.FileRevision = "unsupported"
				c.Revision = in.Revision
			case "header":
				in.Revision.HeaderVersion = "unsupported"
				c.Revision = in.Revision
			case "cumulative":
				in.Revision.CumulativeRevision = "unsupported"
				c.Revision = in.Revision
			case "function":
				in.Function = FunctionReadInputRegisters
				c.Function = in.Function
			case "unit":
				in.UnitID = 0
				c.UnitID = 0
			case "unknown":
				c.IdentityKnown = false
			}
			retentionBegin(t, r, c, 2, 3)
			if err := r.Complete(c, 2, 4, in); err == nil {
				t.Fatal("unsupported/unknown context admitted")
			}
			if s := r.Snapshot(); s.Value != nil || s.SuccessRevision != 1 || s.SuccessTick != 2 {
				t.Fatalf("admission: %+v", s)
			}
		})
	}
	// Native identity contradiction dominates even a simultaneous typed error.
	r, c := retentionSeed(t)
	retentionBegin(t, r, c, 2, 3)
	in := retentionInput()
	in.Slices[1].Words[28]++
	in.Slices[1].Words[8] = 101
	if err := r.Complete(c, 2, 4, in); err == nil {
		t.Fatal("contradiction accepted")
	}
	if s := r.Snapshot(); s.State != "invalidated" || s.Value != nil {
		t.Fatalf("contradiction hidden by telemetry error: %+v", s)
	}
}

func TestGrowattAggregateRetentionDuplicateEvidence(t *testing.T) {
	for _, kind := range []string{"unit", "function", "revision", "count", "offset", "length", "slice0", "slice1", "slice2", "slice3"} {
		t.Run(kind, func(t *testing.T) {
			r, c := retentionSeed(t)
			control := r.Snapshot()
			in := retentionInput()
			switch kind {
			case "unit":
				in.UnitID++
			case "function":
				in.Function = FunctionReadInputRegisters
			case "revision":
				in.Revision.FileRevision = "other"
			case "count":
				in.Slices = in.Slices[:3]
			case "offset":
				in.Slices[0].Offset++
			case "length":
				in.Slices[0].Words = in.Slices[0].Words[:6]
			case "slice0":
				in.Slices[0].Words[6]++
			case "slice1":
				in.Slices[1].Words[9]++
			case "slice2":
				in.Slices[2].Words[11]++
			case "slice3":
				in.Slices[3].Words[1]++
			}
			if err := r.Complete(c, 1, 3, in); err == nil {
				t.Fatal("conflicting duplicate accepted")
			}
			if !reflect.DeepEqual(control, r.Snapshot()) {
				t.Fatal("conflicting duplicate changed accepted evidence")
			}
		})
	}
}

func TestGrowattAggregateRetentionConcurrentSnapshots(t *testing.T) {
	r, c := retentionSeed(t)
	var readers sync.WaitGroup
	for n := 0; n < 4; n++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for i := 0; i < 100; i++ {
				s := r.Snapshot()
				if s.Value != nil {
					s.Value.PackCurrentAmps = 999
					s.Value.nativeObservation.slices[0].Words[0]++
				}
			}
		}()
	}
	for cycle := uint64(2); cycle <= 30; cycle++ {
		retentionBegin(t, r, c, cycle, cycle*2)
		retentionSuccess(t, r, c, cycle, cycle*2+1, retentionInput())
	}
	readers.Wait()
	s := r.Snapshot()
	if s.SuccessRevision != 30 || s.SuccessCycle != 30 || s.Value.PackCurrentAmps != 1 || s.Value.NativeObservation().Slices()[0].Words[0] != 0x0102 {
		t.Fatalf("concurrent snapshots changed stored values: %+v", s)
	}
}

func TestGrowattAggregateRetentionAllTelemetryZero(t *testing.T) {
	r, c := retentionSeed(t)
	in := retentionInput()
	for i := range in.Slices[1].Words {
		in.Slices[1].Words[i] = 0
	}
	in.Slices[1].Words[0] = 0x0204
	in.Slices[1].Words[1] = 0x0301
	in.Slices[1].Words[18] = 0x0300
	in.Slices[1].Words[28] = 32
	for i := 2; i < 4; i++ {
		for j := range in.Slices[i].Words {
			in.Slices[i].Words[j] = 0
		}
	}
	retentionBegin(t, r, c, 2, 3)
	retentionSuccess(t, r, c, 2, 4, in)
	s := r.Snapshot()
	actual := *s.Value
	actual.nativeObservation = nil
	expected := GrowattBMSTypedReadOnlyStatus{
		Revision:           GrowattBMSRevisionTuple{Family: "1xSxxP ESS", FileRevision: "Rev2.01", HeaderVersion: "V2.0", CumulativeRevision: "2.02"},
		MCUSoftwareVersion: "1.2", GaugeVersion: "3.4", BMSCompany: 4, BMSGeneration: 2, PackCompany: 1, PackGeneration: 3, OperatingState: GrowattBMSStateSoftStarting,
	}
	if !reflect.DeepEqual(actual, expected) || !reflect.DeepEqual(s.Value.NativeObservation().Slices(), in.Slices) || s.SuccessRevision != 2 {
		t.Fatalf("validated zeros did not replace every telemetry field: %+v", actual)
	}
}
