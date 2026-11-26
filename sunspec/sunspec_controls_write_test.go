package sunspec

import (
	"errors"
	"slices"
	"testing"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Synthetic register layout of the fixture chain: signature at 40000, model
// 121 header at 40002, model 123 header at 40034, model 124 header at 40060.
const (
	fixtureControlsHeader = 40034
	fixtureStorageHeader  = 40060

	fixtureSettingsOrdinal = 1
	fixtureControlsOrdinal = 2
	fixtureStorageOrdinal  = 3
)

type controlsFixture struct {
	wmax, wmaxScale            uint16
	pctScale                   uint16
	winTms, rvrtTms, rmpTms    uint16
	enable                     uint16
	wChaMax, wChaMaxScale      uint16
	rateScale                  uint16
	mode                       uint16
	omitSettings, omitControls bool
	omitStorage                bool
}

func defaultControlsFixture() controlsFixture {
	return controlsFixture{
		wmax: 8000, wmaxScale: 0, pctScale: 0xffff, // -1
		winTms: 7, rvrtTms: 300, rmpTms: 11, enable: 0,
		wChaMax: 5000, wChaMaxScale: 0, rateScale: 0xffff, // -1
		mode: 0,
	}
}

func (f controlsFixture) chain(t *testing.T) (DecoderRegistry, ChainSnapshot) {
	t.Helper()
	registry := mustSunSpecRegistry(t, ModelsRevisionV1)
	settings := modelBlock(121, 30)
	settings[2], settings[22] = f.wmax, f.wmaxScale
	controls := modelBlock(123, 24)
	controls[5], controls[6], controls[7], controls[8], controls[9] = 1234, f.winTms, f.rvrtTms, f.rmpTms, f.enable
	controls[23] = f.pctScale
	storage := modelBlock(124, 24)
	storage[2], storage[5], storage[12], storage[13] = f.wChaMax, f.mode, 111, 222
	storage[14], storage[15], storage[16] = 5, 6, 7
	storage[18], storage[25] = f.wChaMaxScale, f.rateScale
	blocks := [][]uint16{settings, controls, storage}
	if f.omitSettings {
		blocks = [][]uint16{modelBlock(122, 44), controls, storage}
	}
	if f.omitControls {
		blocks = [][]uint16{settings, modelBlock(122, 44), storage}
	}
	if f.omitStorage {
		blocks = [][]uint16{settings, controls, modelBlock(122, 44)}
	}
	return registry, walkRegisteredGridChain(t, registry, blocks...)
}

func requireControlRefusal(t *testing.T, err error, reason ControlWriteReason) {
	t.Helper()
	var refusal *ControlWriteError
	if !errors.As(err, &refusal) || refusal.Reason != reason {
		t.Fatalf("error = %v, want refusal %q", err, reason)
	}
}

func secondsPointer(v uint16) *uint16 { return &v }

func TestSunSpecControlsWriteActivePowerEncodesOneContiguousRange(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	step, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 4000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if step.Request.Function != 16 || step.Request.Address != fixtureControlsHeader+5 || step.Request.WordCount != 5 {
		t.Fatalf("request = %+v, want FC16 at the percent point over five words", step.Request)
	}
	// WMaxLimPct 500 (50.0 % in scale -1), the three time points as read, WMaxLim_Ena 1.
	if want := []uint16{500, 7, 300, 11, 1}; !slices.Equal(step.Request.Words, want) {
		t.Fatalf("words = %v, want %v", step.Request.Words, want)
	}
	withDuration, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 4000}, Duration: secondsPointer(900),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint16{500, 7, 900, 11, 1}; !slices.Equal(withDuration.Request.Words, want) || withDuration.Request.WordCount != 5 {
		t.Fatalf("words with duration = %v, want %v", withDuration.Request.Words, want)
	}
	if _, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 4000}, Duration: secondsPointer(0xffff),
	}); err == nil {
		t.Fatal("the not-implemented duration word was encoded")
	}
}

func TestSunSpecControlsWriteTimePointsFollowTheDevice(t *testing.T) {
	f := defaultControlsFixture()
	f.winTms, f.rvrtTms, f.rmpTms = 0xffff, 0, 65000
	registry, snapshot := f.chain(t)
	step, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 8000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint16{1000, 0xffff, 0, 65000, 1}; !slices.Equal(step.Request.Words, want) {
		t.Fatalf("words = %v, want the time points as read: %v", step.Request.Words, want)
	}
}

func TestSunSpecControlsWriteRoundsDownInTheScale(t *testing.T) {
	for _, tc := range []struct {
		name            string
		wmax, wmaxScale uint16
		pctScale        uint16
		watts           Decimal
		want            uint16
	}{
		{"41.6625 percent in tenths", 8000, 0, 0xffff, Decimal{Coefficient: 3333}, 416},
		{"whole percent scale", 8000, 0, 0, Decimal{Coefficient: 3333}, 41},
		{"hundredths", 8000, 0, 0xfffe, Decimal{Coefficient: 3333}, 4166},
		{"rating in kilowatts", 8, 3, 0xffff, Decimal{Coefficient: 3333}, 416},
		{"fractional watts", 8000, 0, 0xffff, Decimal{Coefficient: 33339, Exponent: -1}, 416},
		{"exponent watts", 8000, 0, 0xffff, Decimal{Coefficient: 4, Exponent: 3}, 500},
		{"zero", 8000, 0, 0xffff, Decimal{}, 0},
		{"full rating", 8000, 0, 0xffff, Decimal{Coefficient: 8000}, 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := defaultControlsFixture()
			f.wmax, f.wmaxScale, f.pctScale = tc.wmax, tc.wmaxScale, tc.pctScale
			registry, snapshot := f.chain(t)
			step, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
				SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: tc.watts,
			})
			if err != nil || step.Request.Words[0] != tc.want {
				t.Fatalf("percent word = %v (err %v), want %d", step.Request.Words, err, tc.want)
			}
		})
	}
}

func TestSunSpecControlsWriteAnswersBeforeEncoding(t *testing.T) {
	request := ActivePowerLimitRequest{SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 100}}
	for _, tc := range []struct {
		name   string
		mutate func(*controlsFixture, *ActivePowerLimitRequest)
		reason ControlWriteReason
	}{
		{"no model 121", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.omitSettings = true }, ControlReasonModelAbsent},
		{"no model 123", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.omitControls = true }, ControlReasonModelAbsent},
		{"settings ordinal zero", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.SettingsOrdinal = 0 }, ControlReasonModelAbsent},
		{"controls ordinal past the chain", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.ControlsOrdinal = 9 }, ControlReasonModelAbsent},
		{"controls ordinal names another model", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.ControlsOrdinal = fixtureStorageOrdinal }, ControlReasonModelAbsent},
		{"WMax not implemented", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.wmax = 0xffff }, ControlReasonBoundAbsent},
		{"WMax scale not implemented", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.wmaxScale = 0x8000 }, ControlReasonBoundAbsent},
		{"WMax zero", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.wmax = 0 }, ControlReasonBoundAbsent},
		{"percent scale not implemented", func(f *controlsFixture, _ *ActivePowerLimitRequest) { f.pctScale = 0x8000 }, ControlReasonPointUnavailable},
		{"above WMax", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.Watts = Decimal{Coefficient: 8001} }, ControlReasonArgumentOutOfBound},
		{"negative", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.Watts = Decimal{Coefficient: -1} }, ControlReasonArgumentOutOfBound},
		{"watts exponent", func(_ *controlsFixture, q *ActivePowerLimitRequest) { q.Watts = Decimal{Coefficient: 1, Exponent: 11} }, ControlReasonUnencodable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, q := defaultControlsFixture(), request
			tc.mutate(&f, &q)
			registry, snapshot := f.chain(t)
			step, err := EncodeActivePowerLimit(registry, snapshot, q)
			requireControlRefusal(t, err, tc.reason)
			if step.Request.WordCount != 0 || len(step.Request.Words) != 0 {
				t.Fatalf("refused request built %+v", step.Request)
			}
		})
	}
}

func TestSunSpecControlsWriteReleaseWritesOnlyTheEnable(t *testing.T) {
	f := defaultControlsFixture()
	f.enable = 1
	registry, snapshot := f.chain(t)
	step, err := EncodeActivePowerRelease(registry, snapshot, fixtureControlsOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	if step.Request.Function != 16 || step.Request.Address != fixtureControlsHeader+9 || step.Request.WordCount != 1 || !slices.Equal(step.Request.Words, []uint16{0}) {
		t.Fatalf("release = %+v, want FC16 of WMaxLim_Ena = 0 alone", step.Request)
	}
	if !slices.Equal(step.Fields, []string{"der.control.active_power_limit.enabled"}) {
		t.Fatalf("release fields = %v", step.Fields)
	}
	for _, ordinal := range []uint32{0, fixtureSettingsOrdinal, 9} {
		if _, err := EncodeActivePowerRelease(registry, snapshot, ordinal); err == nil {
			t.Fatalf("release accepted ordinal %d", ordinal)
		}
	}
	omitted := defaultControlsFixture()
	omitted.omitControls = true
	registry, snapshot = omitted.chain(t)
	_, err = EncodeActivePowerRelease(registry, snapshot, fixtureControlsOrdinal)
	requireControlRefusal(t, err, ControlReasonModelAbsent)
}

func admissionRequest(q PointWriteRequest) modbus.RegisterWriteAdmissionRequest {
	return modbus.RegisterWriteAdmissionRequest{
		Function:  modbus.FunctionWriteMultipleRegisters,
		Address:   q.Address,
		Quantity:  q.WordCount,
		Registers: append([]uint16(nil), q.Words...),
	}
}

// Each row of the table moves one field of the admission key: function,
// address, quantity and the register payload width. Mutating the predicate to
// ignore any one of them admits the row that moves it.
func requireAdmitsOnlyItsSpan(t *testing.T, name string, q PointWriteRequest) {
	t.Helper()
	admit := q.Admission()
	if !admit(admissionRequest(q)) {
		t.Fatalf("%s: its own span is not admitted", name)
	}
	for _, tc := range []struct {
		field  string
		mutate func(*modbus.RegisterWriteAdmissionRequest)
	}{
		{"function 6", func(r *modbus.RegisterWriteAdmissionRequest) { r.Function = modbus.FunctionWriteSingleRegister }},
		{"function 15", func(r *modbus.RegisterWriteAdmissionRequest) { r.Function = modbus.FunctionWriteMultipleCoils }},
		{"address before", func(r *modbus.RegisterWriteAdmissionRequest) { r.Address-- }},
		{"address after", func(r *modbus.RegisterWriteAdmissionRequest) { r.Address++ }},
		{"quantity narrower", func(r *modbus.RegisterWriteAdmissionRequest) {
			r.Quantity--
			r.Registers = r.Registers[:len(r.Registers)-1]
		}},
		{"quantity wider", func(r *modbus.RegisterWriteAdmissionRequest) { r.Quantity++; r.Registers = append(r.Registers, 0) }},
		{"quantity only", func(r *modbus.RegisterWriteAdmissionRequest) { r.Quantity++ }},
		{"payload width only", func(r *modbus.RegisterWriteAdmissionRequest) { r.Registers = append(r.Registers, 0) }},
	} {
		request := admissionRequest(q)
		tc.mutate(&request)
		if admit(request) {
			t.Fatalf("%s: admitted a request that differs in %s", name, tc.field)
		}
	}
	if (PointWriteRequest{}).Admission()(modbus.RegisterWriteAdmissionRequest{Function: modbus.FunctionWriteMultipleRegisters}) {
		t.Fatalf("%s: the zero request admitted a write", name)
	}
}

func TestSunSpecControlsWriteEachPredicateAdmitsItsOwnSpanOnly(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	set, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 4000},
	})
	if err != nil {
		t.Fatal(err)
	}
	release, err := EncodeActivePowerRelease(registry, snapshot, fixtureControlsOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	requireAdmitsOnlyItsSpan(t, "set", set.Request)
	requireAdmitsOnlyItsSpan(t, "release", release.Request)
	if release.Admission()(admissionRequest(set.Request)) || set.Admission()(admissionRequest(release.Request)) {
		t.Fatal("the set and the release admit each other's span")
	}
	// A by-hand call admits exactly the range of the point it writes.
	for _, field := range []string{
		"der.control.active_power_limit.window_seconds", "der.control.active_power_limit.revert_seconds",
		"der.control.active_power_limit.ramp_seconds", "der.control.active_power_limit.percent",
	} {
		hand := requireHandRequest(t, registry, snapshot, fixtureControlsOrdinal, field)
		requireAdmitsOnlyItsSpan(t, field, hand)
		if set.Admission()(admissionRequest(hand)) && hand.WordCount != set.Request.WordCount {
			t.Fatalf("the set range admitted the single point %s", field)
		}
		if hand.Admission()(admissionRequest(set.Request)) {
			t.Fatalf("the single point %s admitted the set range", field)
		}
	}
}

func requireHandRequest(t *testing.T, registry DecoderRegistry, snapshot ChainSnapshot, ordinal uint32, field string) PointWriteRequest {
	t.Helper()
	writes := requirePointWrites(t, registry, snapshot, ordinal)
	point := writePoint(t, writes, field, "", 0)
	var value any = uint16(5)
	if point.scale != nil {
		value = Decimal{Coefficient: 50, Exponent: -1}
	}
	request, err := point.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestSunSpecControlsWriteReadbackStatesMatchMismatchOrUnknown(t *testing.T) {
	registry, snapshot := defaultControlsFixture().chain(t)
	set, err := EncodeActivePowerLimit(registry, snapshot, ActivePowerLimitRequest{
		SettingsOrdinal: fixtureSettingsOrdinal, ControlsOrdinal: fixtureControlsOrdinal, Watts: Decimal{Coefficient: 4000},
	})
	if err != nil {
		t.Fatal(err)
	}
	written := set.Request.Words
	for _, tc := range []struct {
		name string
		got  []uint16
		want ReadbackState
	}{
		{"as written", written, ReadbackMatch},
		{"percent differs", []uint16{499, 7, 300, 11, 1}, ReadbackMismatch},
		{"window differs", []uint16{500, 8, 300, 11, 1}, ReadbackMismatch},
		{"revert differs", []uint16{500, 7, 301, 11, 1}, ReadbackMismatch},
		{"ramp differs", []uint16{500, 7, 300, 12, 1}, ReadbackMismatch},
		{"enable still off", []uint16{500, 7, 300, 11, 0}, ReadbackMismatch},
		{"enable not implemented", []uint16{500, 7, 300, 11, 0xffff}, ReadbackUnknown},
		{"percent not implemented", []uint16{0xffff, 7, 300, 11, 1}, ReadbackUnknown},
		{"a mismatch outranks an unknown point", []uint16{499, 7, 300, 11, 0xffff}, ReadbackMismatch},
		{"too short", written[:4], ReadbackUnknown},
		{"too long", append(slices.Clone(written), 0), ReadbackUnknown},
		{"none", nil, ReadbackUnknown},
	} {
		if got := set.CompareReadback(tc.got); got != tc.want {
			t.Errorf("%s: readback = %s, want %s", tc.name, got, tc.want)
		}
	}
	release, err := EncodeActivePowerRelease(registry, snapshot, fixtureControlsOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		got  []uint16
		want ReadbackState
	}{
		{"enable reads 0", []uint16{0}, ReadbackMatch},
		{"enable reads 1", []uint16{1}, ReadbackMismatch},
		{"enable reads another value", []uint16{2}, ReadbackMismatch},
		{"enable not implemented", []uint16{0xffff}, ReadbackUnknown},
		{"no re-read", nil, ReadbackUnknown},
		{"re-read of the wrong width", []uint16{0, 0}, ReadbackUnknown},
	} {
		if got := release.CompareReadback(tc.got); got != tc.want {
			t.Errorf("release, %s: readback = %s, want %s", tc.name, got, tc.want)
		}
	}
	if (ControlWriteStep{}).CompareReadback([]uint16{0}) != ReadbackUnknown {
		t.Error("the zero step established a readback")
	}
}

func TestSunSpecControlsWriteCombineReadback(t *testing.T) {
	for _, tc := range []struct {
		states []ReadbackState
		want   ReadbackState
	}{
		{nil, ReadbackUnknown},
		{[]ReadbackState{ReadbackMatch}, ReadbackMatch},
		{[]ReadbackState{ReadbackMatch, ReadbackMatch}, ReadbackMatch},
		{[]ReadbackState{ReadbackMatch, ReadbackUnknown}, ReadbackUnknown},
		{[]ReadbackState{ReadbackUnknown, ReadbackMismatch}, ReadbackMismatch},
		{[]ReadbackState{ReadbackMismatch, ReadbackMatch}, ReadbackMismatch},
		{[]ReadbackState{ReadbackState("other")}, ReadbackUnknown},
	} {
		if got := CombineReadback(tc.states...); got != tc.want {
			t.Errorf("CombineReadback(%v) = %s, want %s", tc.states, got, tc.want)
		}
	}
}
