package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func sunConfig() Config {
	c := testConfig()
	c.Battery, c.System = "battery-a", "system-a"
	c.SunSpec = []SunSpecStorage{{Position: 1, Pack: "pack-1"}}
	return c
}

func ratingsBlock(t *testing.T, r sunspec.DecoderRegistry) []uint16 {
	return block(t, r, 120, 26, sunspec.ModelsRevisionV1, map[string][]uint16{
		"storage.rating.energy": {10}, "storage.scale.energy_rating": {3},
		"storage.rating.max_discharge_power": {3000}, "storage.scale.max_discharge_power": {0},
	})
}

func controlsBlock(t *testing.T, r sunspec.DecoderRegistry, charge, discharge uint16) []uint16 {
	return block(t, r, 124, 24, sunspec.ModelsRevisionV1, map[string][]uint16{
		"storage.control.max_charge_power": {6000}, "storage.scale.max_charge_power": {0},
		"storage.control.charge_gradient": {1}, "storage.control.discharge_gradient": {1}, "storage.scale.charge_discharge_gradient": {0},
		"storage.control.mode":           {0},
		"storage.status.state_of_charge": {7500}, "storage.scale.state_of_charge": {0xfffe},
		"storage.status.internal_battery_voltage": {5120}, "storage.scale.internal_battery_voltage": {0xffff},
		"storage.status.charge_state": {4},
		"storage.control.charge_rate": {charge}, "storage.control.discharge_rate": {discharge}, "storage.scale.charge_discharge_rate": {0xffff},
	})
}

func basicChain(t *testing.T, blocks ...func(sunspec.DecoderRegistry) []uint16) sunspec.ChainSnapshot {
	t.Helper()
	r := sunRegistry(t, sunspec.ModelsRevisionV1)
	all := [][]uint16{commonBlock(t, r, sunspec.ModelsRevisionV1)}
	for _, b := range blocks {
		all = append(all, b(r))
	}
	return chain(t, sunspec.ModelsRevisionV1, r.DecoderKeys(), all...)
}

func TestSunSpecBasicStorageFactsLimitsAndInterface(t *testing.T) {
	snap := basicChain(t,
		func(r sunspec.DecoderRegistry) []uint16 { return ratingsBlock(t, r) },
		func(r sunspec.DecoderRegistry) []uint16 { return controlsBlock(t, r, 500, 400) })
	s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	k := s.k
	requireQuantity(t, k, "storage.state.soc", "75", -2, unitRatio)
	requireQuantity(t, k, "storage.pack.voltage", "512", 0, unitVolt)
	requireQuantity(t, k, "storage.energy.nominal_capacity", "36", 6, unitJoule)
	if got := symbolOf(t, only(t, k, elementOperating)); got != "storage.status.operating.active" {
		t.Errorf("operating %s", got)
	}
	// Each limit against its own reference: 50 % of WChaMax 6000 and 40 % of MaxDisChaRte 3000.
	requireQuantity(t, k, "storage.limit.charge_power", "3", 3, unitWatt)
	requireQuantity(t, k, "storage.limit.discharge_power", "12", 2, unitWatt)
	for _, element := range []semreg.DefinitionID{"storage.limit.charge_power", "storage.limit.discharge_power"} {
		c := only(t, k, element)
		if c.Key.Dimensions[0].ID != dimInterface {
			t.Errorf("%s subject dimension %s", element, c.Key.Dimensions[0].ID)
		}
	}
	// The interface service sits on the same resource as the limits, under the owner's binding and generation.
	iface := *only(t, k, "storage.limit.charge_power").Key.Dimensions[0].Value.Text
	var service *semreg.ServiceInstance
	for _, sv := range k.services {
		sv := sv
		if sv.Definition.ID == svcInterface {
			service = &sv
		}
	}
	if service == nil || service.Resource == nil || string(*service.Resource) != iface || service.DriverGeneration != "1" || service.SourceEpochID != "epoch-1" {
		t.Fatalf("interface service: %+v", service)
	}
	if _, ok := k.bindings[service.BindingID]; !ok {
		t.Fatal("the interface service names no published binding")
	}
	if k.resources[semreg.ResourceID(iface)].Kind != kindInterface {
		t.Fatalf("the limits name a %s", k.resources[semreg.ResourceID(iface)].Kind)
	}
	for _, def := range []semreg.DefinitionID{relSystemBattery, relBatteryPack, relPackInterface} {
		if len(relationsOf(k, def)) != 1 || len(claimsOf(k, def)) != 1 {
			t.Errorf("%s: %d relations %d claims", def, len(relationsOf(k, def)), len(claimsOf(k, def)))
		}
	}
	// Native values with no element stay readable, with their model occurrence.
	var found bool
	for _, m := range p.Members() {
		found = found || m.Key.Local == "model_124.occurrence_3.storage.control.max_charge_power"
	}
	if !found {
		t.Errorf("the native control point is not in the namespace")
	}
}

func TestSunSpecDischargeFallsBackToTheChargeReference(t *testing.T) {
	snap := basicChain(t, func(r sunspec.DecoderRegistry) []uint16 { return controlsBlock(t, r, 500, 400) })
	s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	requireQuantity(t, s.k, "storage.limit.charge_power", "3", 3, unitWatt)
	// No MaxDisChaRte: 40 % of WChaMax 6000.
	requireQuantity(t, s.k, "storage.limit.discharge_power", "24", 2, unitWatt)
}

func TestSunSpecNegativeRateStatesNoLimit(t *testing.T) {
	snap := basicChain(t, func(r sunspec.DecoderRegistry) []uint16 { return controlsBlock(t, r, 0xfe0c, 400) })
	s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	if len(factsOf(s.k, "storage.limit.charge_power")) != 0 {
		t.Fatal("a negative charge rate stated a limit")
	}
	if len(factsOf(s.k, "storage.limit.discharge_power")) != 1 {
		t.Fatal("the discharge limit was lost with it")
	}
}

func TestSunSpecChargeStateTable(t *testing.T) {
	for name, want := range map[string]string{"4": "active", "3": "active", "6": "idle", "5": "idle", "1": "standby", "2": "standby"} {
		code := uint16(name[0] - '0')
		snap := basicChain(t, func(r sunspec.DecoderRegistry) []uint16 {
			b := controlsBlock(t, r, 500, 400)
			point, _, _ := r.FixedPointLayout(sunspec.DecoderKey{ModelID: 124, ModelLength: 24, SchemaRevision: sunspec.ModelsRevisionV1}, "storage.status.charge_state")
			b[point.WordOffset] = code
			return b
		})
		s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
		p := newPublisher(t, sunConfig(), s, nil)
		if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
			t.Fatal(err)
		}
		if got := symbolOf(t, only(t, s.k, elementOperating)); !strings.HasSuffix(got, "."+want) {
			t.Errorf("state %s: %s, want %s", name, got, want)
		}
	}
	// TESTING has no counterpart: the state stays native.
	snap := basicChain(t, func(r sunspec.DecoderRegistry) []uint16 {
		b := controlsBlock(t, r, 500, 400)
		point, _, _ := r.FixedPointLayout(sunspec.DecoderKey{ModelID: 124, ModelLength: 24, SchemaRevision: sunspec.ModelsRevisionV1}, "storage.status.charge_state")
		b[point.WordOffset] = 7
		return b
	})
	s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	if len(factsOf(s.k, elementOperating)) != 0 {
		t.Fatal("TESTING was mapped")
	}
}
