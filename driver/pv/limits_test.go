package pv

import (
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

// extBlock builds a block of a V1 extended model from its field identifiers.
func extBlock(t *testing.T, model, length uint16, values map[string][]uint16) []uint16 {
	t.Helper()
	r := registry(t)
	key := sunspec.DecoderKey{ModelID: model, ModelLength: length, SchemaRevision: sunspec.ModelsRevisionV1}
	b := zeros(model, length)
	for id, v := range values {
		p, _, ok := r.FixedPointLayout(key, id)
		if !ok || int(p.WordWidth) != len(v) {
			t.Fatalf("model %d point %s: layout %v ok=%v words=%d", model, id, p, ok, len(v))
		}
		copy(b[p.WordOffset:], v)
	}
	return b
}

func limitChain(t *testing.T, enabled uint16, controls uint32, state uint16) Publication {
	t.Helper()
	blocks := [][]uint16{
		commonBlock("Maker", "Model", "SN-1"),
		inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.operating_state": {state}})),
		zeros(120, 26),
		extBlock(t, 121, 30, map[string][]uint16{"der.settings.max_active_power": {5000}}),
		extBlock(t, 122, 44, map[string][]uint16{"der.status.active_controls": accumulator(controls)}),
		extBlock(t, 123, 24, map[string][]uint16{"der.control.active_power_limit.percent": {50}, "der.control.active_power_limit.enabled": {enabled}}),
	}
	return publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, blocks...)}}})
}

func TestTheActivePowerLimitPublishesOnlyWhenEnabled(t *testing.T) {
	pub := limitChain(t, 1, 0, 4)
	inv := string(pub.inverters()[0].Ref.ID)
	requireQuantity(t, pub, "pv.limit.active_power_ratio", inv, "unit.ratio", "0.5")
	requireQuantity(t, pub, "pv.limit.active_power", inv, "unit.watt", "2500")
	requireSymbol(t, pub, "pv.status.derating", inv, "clear")

	off := limitChain(t, 0, 0, 4)
	off.absent(t, "pv.limit.active_power_ratio")
	off.absent(t, "pv.limit.active_power")
	listed := false
	for _, w := range off.Withheld {
		listed = listed || w.Reason == "limit_not_enabled"
	}
	if !listed {
		t.Fatal("a limit that is not enabled is not listed with its reason")
	}
}

func TestAnActiveControlIsDerating(t *testing.T) {
	pub := limitChain(t, 1, 1, 4)
	requireSymbol(t, pub, "pv.status.derating", string(pub.inverters()[0].Ref.ID), "active")
}
