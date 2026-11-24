package sunspec

import (
	"reflect"
	"slices"
	"testing"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func requirePointWrites(t *testing.T, registry DecoderRegistry, snapshot ChainSnapshot, ordinal uint32) []PointWrite {
	t.Helper()
	points, err := PointWrites(registry, snapshot, ordinal)
	if err != nil {
		t.Fatal(err)
	}
	return points
}

func writePoint(t *testing.T, points []PointWrite, field, group string, index uint16) PointWrite {
	t.Helper()
	for _, point := range points {
		if id, g, i := point.Point(); id == field && g == group && i == index {
			return point
		}
	}
	t.Fatalf("no write for %s %s[%d]", field, group, index)
	return PointWrite{}
}

func requireWrite(t *testing.T, point PointWrite, value any, address uint16, words ...uint16) {
	t.Helper()
	request, err := point.Encode(value)
	if err != nil || request.Function != 16 || request.Address != address || request.WordCount != uint16(len(words)) || !slices.Equal(request.Words, words) {
		t.Fatalf("encode %v: request=%+v err=%v, want FC16 at %d with %x", value, request, err, address, words)
	}
}

func requireNoWrite(t *testing.T, point PointWrite, value any) {
	t.Helper()
	request, err := point.Encode(value)
	if err == nil || !reflect.DeepEqual(request, PointWriteRequest{}) {
		t.Fatalf("rejected value %v returned request=%+v err=%v", value, request, err)
	}
}

func TestSunSpecPointWriteControlsUseNativeScale(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	controls := modelBlock(123, 24)
	controls[23] = 0xfffe
	storage := modelBlock(124, 24)
	storage[25] = 0xffff
	snapshot := walkRegisteredGridChain(t, registry, controls, storage, modelBlock(122, 44), modelBlock(120, 26), modelBlock(121, 30))
	points := requirePointWrites(t, registry, snapshot, 1)
	if len(points) != 15 {
		t.Fatalf("immediate-control writes=%d, want 15", len(points))
	}
	limit := writePoint(t, points, "der.control.active_power_limit.percent", "", 0)
	if limit.PointType() != TypeUint16 || limit.Unit() != "% WMax" {
		t.Fatalf("limit type/unit = %v %q", limit.PointType(), limit.Unit())
	}
	requireWrite(t, limit, Decimal{Coefficient: 375, Exponent: -1}, 40007, 3750)
	requireWrite(t, writePoint(t, points, "der.control.active_power_limit.enabled", "", 0), uint16(0), 40011, 0)
	for _, bad := range []any{uint16(3750), Decimal{1, -3}, Decimal{-1, 0}, Decimal{65535, -2}, Decimal{1, 11}, Decimal{9223372036854775807, 10}} {
		requireNoWrite(t, limit, bad)
	}
	points = requirePointWrites(t, registry, snapshot, 2)
	if len(points) != 9 {
		t.Fatalf("storage-control writes=%d, want 9", len(points))
	}
	rate := writePoint(t, points, "storage.control.discharge_rate", "", 0)
	requireWrite(t, rate, Decimal{-15, 0}, 40040, 0xff6a)
	requireNoWrite(t, rate, Decimal{-32768, -1})
	for ordinal := uint32(3); ordinal <= 5; ordinal++ {
		if got := requirePointWrites(t, registry, snapshot, ordinal); len(got) != 0 {
			t.Fatalf("model %d offered a write without a writable point", snapshot.occurrences[ordinal-1].ModelID())
		}
	}
}

func TestSunSpecPointWriteEnterServiceUsesPercentAndExcludesStatus(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	words := modelBlock(703, 17)
	words[17], words[18] = 0xffff, 0xfffd
	points := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, words), 1)
	if len(points) != 8 {
		t.Fatalf("enter-service writes=%d, want 8", len(points))
	}
	for _, tc := range []struct {
		field     string
		addr, raw uint16
	}{
		{"ESVHi", 40005, 1105}, {"ESVLo", 40006, 895},
	} {
		point := writePoint(t, points, "sunspec.der.v2.703."+tc.field, "", 0)
		if point.Unit() != "Pct" {
			t.Fatalf("unit for %s = %q", tc.field, point.Unit())
		}
		requireWrite(t, point, Decimal{int64(tc.raw), -1}, tc.addr, tc.raw)
	}
	es := writePoint(t, points, "sunspec.der.v2.703.ES", "", 0)
	requireWrite(t, es, uint16(1), 40004, 1)
	requireNoWrite(t, es, uint16(2))
	for _, point := range points {
		if id, _, _ := point.Point(); id == "sunspec.der.v2.703.ESDlyRemTms" {
			t.Fatal("remaining-delay status offered a write")
		}
	}
}

func TestSunSpecPointWriteGridAccessMatrix(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	for _, tc := range []struct {
		id, length, countOffset uint16
		fixed, repeated         int
	}{
		{125, 8, 0, 6, 0}, {126, 64, 7, 5, 46}, {127, 10, 0, 6, 0}, {128, 14, 0, 11, 0},
		{129, 60, 7, 5, 42}, {130, 60, 7, 5, 42}, {131, 64, 7, 5, 45}, {132, 64, 7, 5, 46},
		{133, 66, 5, 2, 31}, {134, 68, 7, 5, 50}, {135, 60, 7, 5, 42}, {136, 60, 7, 5, 42},
		{137, 60, 7, 5, 42}, {138, 60, 7, 5, 42}, {139, 60, 7, 5, 42}, {140, 60, 7, 5, 42},
		{141, 60, 7, 5, 42}, {142, 60, 7, 5, 42}, {143, 60, 7, 5, 42}, {144, 60, 7, 5, 42},
		{145, 8, 0, 7, 0},
	} {
		block := modelBlock(tc.id, tc.length)
		if tc.countOffset != 0 {
			block[tc.countOffset] = 1
		}
		points := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, block), 1)
		var fixed, repeated int
		for _, point := range points {
			if _, _, index := point.Point(); index == 0 {
				fixed++
			} else {
				repeated++
			}
		}
		if fixed != tc.fixed || repeated != tc.repeated {
			t.Fatalf("model %d has %d fixed and %d repeated writes; want %d and %d", tc.id, fixed, repeated, tc.fixed, tc.repeated)
		}
	}
}

func TestSunSpecPointWriteGridUsesOwnCurveAccessAndStringSpan(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	block := modelBlock(126, 118)
	block[7], block[65], block[119] = 2, 0, 1
	points := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, block), 1)
	if len(points) != 5+46 {
		t.Fatalf("got %d writes for one writable curve and one read-only curve", len(points))
	}
	for _, point := range points {
		field, _, index := point.Point()
		if index == 2 || field == gridField(126, "ReadOnly") || field == gridField(126, "NPt") {
			t.Fatalf("read-only %s[%d] offered a write", field, index)
		}
	}
	curveName := writePoint(t, points, gridField(126, "CrvNam"), "curve", 1)
	requireWrite(t, curveName, "Aé", 40056, 0x41c3, 0xa900, 0, 0, 0, 0, 0, 0)
	requireWrite(t, curveName, "", 40056, 0x0080, 0, 0, 0, 0, 0, 0, 0)
	requireWrite(t, curveName, "ABCDEFGHIJKLMNOP", 40056, 0x4142, 0x4344, 0x4546, 0x4748, 0x494a, 0x4b4c, 0x4d4e, 0x4f50)
	for _, bad := range []string{"text\x00", "\xff", "ABCDEFGHIJKLMNOPQ"} {
		requireNoWrite(t, curveName, bad)
	}
}

func TestSunSpecPointWriteScheduleUsesLocalScaleAndSignedWords(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	block := modelBlock(133, 126)
	block[5], block[14], block[74] = 2, 0xffff, 0xfffe
	points := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, block), 1)
	requireWrite(t, writePoint(t, points, gridField(133, "X1"), "repeating", 1), Decimal{-0x10203, -1}, 40019, 0xfffe, 0xfdfd)
	requireWrite(t, writePoint(t, points, gridField(133, "X1"), "repeating", 2), Decimal{-0x10203, -2}, 40079, 0xfffe, 0xfdfd)
	exponent := writePoint(t, points, gridField(133, "X_SF"), "repeating", 2)
	requireWrite(t, exponent, int16(-10), 40076, 0xfff6)
	requireNoWrite(t, exponent, int16(-11))
	active := writePoint(t, points, gridField(133, "ActSchd"), "", 0)
	requireWrite(t, active, uint32(0x80000001), 40004, 0x8000, 1)
	requireNoWrite(t, active, uint32(0xffffffff))

	block[74] = 0x8000
	points = requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, block), 1)
	for _, point := range points {
		if id, _, index := point.Point(); id == gridField(133, "X1") && index == 2 {
			t.Fatal("a scaled coordinate offered a write with an unavailable scale factor")
		}
	}
	requireWrite(t, writePoint(t, points, gridField(133, "X_SF"), "repeating", 2), int16(-2), 40076, 0xfffe)
}

func TestSunSpecPointWriteUsesRetainedStructuralLayout(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV2)
	snapshot, _ := runV2StructuralCandidateChain(t, []uint16{707})
	for _, ordinal := range []uint32{1, 2} {
		points := requirePointWrites(t, registry, snapshot, ordinal)
		if len(points) != 20 {
			t.Fatalf("structural occurrence %d offers %d writes, want 20", ordinal, len(points))
		}
		base := uint16(40002 + (ordinal-1)*35)
		requireWrite(t, writePoint(t, points, tripField(707, "Ena"), "", 0), uint16(1), base+2, 1)
		var target PointWrite
		for _, point := range points {
			path, ok := point.NestedPath()
			if !ok {
				t.Fatal("structural point has no path")
			}
			if slices.Equal(path.Segments(), []FactPathSegment{{Name: "Crv", Index: 2, Indexed: true}, {Name: "MayTrip"}, {Name: "Pt", Index: 1, Indexed: true}, {Name: "Tms"}}) {
				target = point
			}
		}
		requireWrite(t, target, Decimal{0x123456, 0}, base+29, 0x0012, 0x3456)
	}
	plain, _ := runV2StructuralCandidateChain(t, nil)
	if points, err := PointWrites(registry, plain, 1); err == nil || len(points) != 0 {
		t.Fatalf("plain unknown occurrence offered writes (%d, %v)", len(points), err)
	}
}

func TestSunSpecPointWriteRejectsContradictoryOccurrence(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	snapshot := walkRegisteredGridChain(t, registry, modelBlock(125, 8))
	for _, tc := range []struct {
		name   string
		mutate func(*Occurrence)
	}{
		{"ordinal", func(o *Occurrence) { o.Ordinal++ }},
		{"wire model", func(o *Occurrence) { o.WireKey.ModelID = 127 }},
		{"wire length", func(o *Occurrence) { o.WireKey.ModelLength++ }},
		{"occurrence revision", func(o *Occurrence) { o.SchemaRevision = ModelsRevisionV2 }},
		{"key model", func(o *Occurrence) { o.decoderKey.ModelID = 127 }},
		{"key length", func(o *Occurrence) { o.decoderKey.ModelLength++ }},
		{"key revision", func(o *Occurrence) { o.decoderKey.SchemaRevision = ModelsRevisionV2 }},
		{"key absent", func(o *Occurrence) { o.decoderKey = nil }},
		{"disposition", func(o *Occurrence) { o.Disposition = ChainDispositionUnknownModel }},
		{"shifted span", func(o *Occurrence) { o.HeaderOffset++; o.PayloadOffset++ }},
		{"mismatched payload", func(o *Occurrence) { o.PayloadOffset++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := snapshot.Clone()
			tc.mutate(&changed.occurrences[0])
			if points, err := PointWrites(registry, changed, 1); err == nil || len(points) != 0 {
				t.Fatalf("contradictory occurrence offered writes (%d, %v)", len(points), err)
			}
		})
	}
	for _, ordinal := range []uint32{0, 2} {
		if points, err := PointWrites(registry, snapshot, ordinal); err == nil || len(points) != 0 {
			t.Fatalf("absent occurrence offered writes (%d, %v)", len(points), err)
		}
	}
	malformed := modelBlock(126, 64) // The one-curve length contradicts NCrv=0.
	if points, err := PointWrites(registry, walkRegisteredGridChain(t, registry, malformed), 1); err == nil || len(points) != 0 {
		t.Fatalf("malformed curve geometry offered writes (%d, %v)", len(points), err)
	}
	requireNoWrite(t, PointWrite{}, uint16(1))
}

func TestSunSpecPointWriteRejectsMixedSources(t *testing.T) {
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	snapshot := walkRegisteredGridChain(t, registry, modelBlock(125, 8))
	for _, tc := range []struct {
		name   string
		mutate func(*modbusreg.LogicalViewRecord)
	}{
		{"endpoint", func(r *modbusreg.LogicalViewRecord) { r.Endpoint += "-other" }},
		{"unit", func(r *modbusreg.LogicalViewRecord) { r.UnitID++ }},
		{"connection", func(r *modbusreg.LogicalViewRecord) { r.ConnectionID++ }},
		{"transport", func(r *modbusreg.LogicalViewRecord) { r.Transport = modbusreg.TransportRTU; r.ConnectionID = 0 }},
		{"transport generation", func(r *modbusreg.LogicalViewRecord) { r.TransportGeneration++ }},
		{"poll generation", func(r *modbusreg.LogicalViewRecord) { r.PollGeneration++ }},
		{"deadline", func(r *modbusreg.LogicalViewRecord) { r.DeadlineIdentity++ }},
		{"scope", func(r *modbusreg.LogicalViewRecord) { r.AuthorizationScope += "-other" }},
		{"address", func(r *modbusreg.LogicalViewRecord) { r.PhysicalOffset++; r.LogicalOffset++ }},
		{"register table", func(r *modbusreg.LogicalViewRecord) {
			r.RequestedFunction, r.ReceivedFunction, r.Table = modbusreg.FunctionReadInputRegisters, modbusreg.FunctionReadInputRegisters, modbusreg.InputRegisters
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := snapshot.Clone()
			record := changed.sources[2].Record()
			tc.mutate(&record)
			var err error
			changed.sources[2], err = modbusreg.NewLogicalViewSnapshot(record)
			if err != nil {
				t.Fatal(err)
			}
			if points, err := PointWrites(registry, changed, 1); err == nil || len(points) != 0 {
				t.Fatalf("mixed source offered writes (%d, %v)", len(points), err)
			}
		})
	}
}

func TestSunSpecPointWriteTripWordsAreMostSignificantFirst(t *testing.T) {
	for _, tc := range []struct {
		name, field        string
		id, length, offset uint16
		exponent           int16
	}{
		{"high-voltage", "Crv.MustTrip.Pt.Tms", 708, 51, 15, -3},
		{"low-frequency", "Crv.MustTrip.Pt.Hz", 709, 63, 15, -2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := mustSunSpecRegistry(t, ModelsRevisionV2)
			words := modelBlock(tc.id, tc.length)
			words[5], words[6], words[7], words[8] = 2, 2, 0xfffe, 0xfffd
			points := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, words), 1)
			if len(points) != 32 {
				t.Fatalf("got %d writes for two writable curves", len(points))
			}
			point := writePoint(t, points, tripField(tc.id, tc.field), "curve.1.MustTrip", 2)
			requireWrite(t, point, Decimal{0x123456, tc.exponent}, 40002+tc.offset, 0x0012, 0x3456)
			requireNoWrite(t, point, Decimal{0xffffffff, tc.exponent})
			words[9+(tc.length-7)/2] = 1
			if got := requirePointWrites(t, registry, walkRegisteredGridChain(t, registry, words), 1); len(got) != 17 {
				t.Fatalf("got %d writes after marking the second curve read-only", len(got))
			}
		})
	}
}
