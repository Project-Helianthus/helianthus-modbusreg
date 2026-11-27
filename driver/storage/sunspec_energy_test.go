package storage

import (
	"context"
	"math/big"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

func publishChain(t *testing.T, revision sunspec.SchemaRevision, blocks ...[]uint16) (*Publisher, *session) {
	t.Helper()
	r := sunRegistry(t, revision)
	all := append([][]uint16{commonBlock(t, r, revision)}, blocks...)
	snap := chain(t, revision, r.DecoderKeys(), all...)
	s := newSession("asset:sunspec-a", "source-a", "epoch-1", 1)
	p := newPublisher(t, sunConfig(), s, nil)
	if err := p.PublishSunSpec(context.Background(), SunSpecObservation{Decisions: decisions(t, snap), Receipt: receipt(1)}); err != nil {
		t.Fatal(err)
	}
	return p, s
}

func requireNativeDecimal(t *testing.T, n Native, want string) {
	t.Helper()
	got, w := decimalRat(n.Value), new(big.Rat)
	if _, ok := w.SetString(want); !ok || got.Cmp(w) != 0 {
		t.Fatalf("native %s e%d, want %s", n.Value.Coefficient, n.Value.Exponent10, want)
	}
}

func requireNoPackEnergy(t *testing.T, k *kernel) {
	t.Helper()
	if n := len(factsOf(k, "storage.energy.charged")) + len(factsOf(k, "storage.energy.discharged")); n != 0 {
		t.Fatalf("pack energy facts %d", n)
	}
}

func requireWithheld(t *testing.T, p *Publisher, pack semreg.ResourceID, reason string) {
	t.Helper()
	for _, element := range []semreg.DefinitionID{"storage.energy.charged", "storage.energy.discharged"} {
		var got []Unknown
		for _, u := range p.Unknown() {
			if u.Key.FactID == element {
				got = append(got, u)
			}
		}
		if len(got) != 1 || got[0].Reason != reason || len(got[0].Key.Dimensions) != 1 || got[0].Key.Dimensions[0].ID != dimPack || got[0].Key.Dimensions[0].Value.Text == nil || *got[0].Key.Dimensions[0].Value.Text != string(pack) {
			t.Fatalf("%s withheld %+v", element, got)
		}
	}
}

// model160 is two modules. DC energy is the two big-endian words at offset 12
// of each module. A label does not choose the element.
func model160(labels []string, wattHours []uint16) []uint16 {
	n := len(labels)
	words := make([]uint16, 10+20*n)
	words[0], words[1], words[8] = 160, uint16(8+20*n), uint16(n)
	for i := range labels {
		base := 10 + 20*i
		words[base] = uint16(i + 1)
		copy(words[base+1:], padded(labels[i], 8))
		words[base+13] = wattHours[i]
	}
	return words
}

func TestSunSpecModel160LifetimeEnergyStaysNativeWithoutARole(t *testing.T) {
	pack := packResource("battery-a", "pack-1")
	labels := []string{"Storage Charge", "Storage Disch"}
	// Occurrence 2 is the model directly after the common block. Hardcoding
	// another occurrence, summing the two watt-hour values, or publishing
	// either one as charged or discharged fails.
	p, s := publishChain(t, sunspec.ModelsRevisionV1, model160(labels, []uint16{10, 25}))
	requireNoPackEnergy(t, s.k)
	requireWithheld(t, p, pack, reasonModuleRoleUnstated)
	requireNativeDecimal(t, memberValue(t, p, "model_160.occurrence_2.group_1.mppt.module.dc_energy"), "10")
	requireNativeDecimal(t, memberValue(t, p, "model_160.occurrence_2.group_2.mppt.module.dc_energy"), "25")
	if memberValue(t, p, "model_160.occurrence_2.group_1.mppt.module.label").Text != labels[0] || memberValue(t, p, "model_160.occurrence_2.group_2.mppt.module.label").Text != labels[1] {
		t.Fatal("module labels are not on their groups")
	}

	// A ratings block before the modules moves the occurrence to 3. The same
	// watt-hours must follow that occurrence, and the ratings energy is not
	// either lifetime total.
	r := sunRegistry(t, sunspec.ModelsRevisionV1)
	later, laterSession := publishChain(t, sunspec.ModelsRevisionV1, ratingsBlock(t, r), model160(labels, []uint16{10, 25}))
	requireNoPackEnergy(t, laterSession.k)
	requireWithheld(t, later, pack, reasonModuleRoleUnstated)
	requireNativeDecimal(t, memberValue(t, later, "model_160.occurrence_3.group_1.mppt.module.dc_energy"), "10")
	requireNativeDecimal(t, memberValue(t, later, "model_160.occurrence_3.group_2.mppt.module.dc_energy"), "25")
	if _, ok := memberLocal(later, "model_160.occurrence_2.group_1.mppt.module.dc_energy"); ok {
		t.Fatal("the later modules kept the earlier occurrence")
	}

	// Swapping the labels leaves each group's watt-hours where they were.
	swapped, swappedSession := publishChain(t, sunspec.ModelsRevisionV1, model160([]string{labels[1], labels[0]}, []uint16{10, 25}))
	requireNoPackEnergy(t, swappedSession.k)
	requireNativeDecimal(t, memberValue(t, swapped, "model_160.occurrence_2.group_1.mppt.module.dc_energy"), "10")
	requireNativeDecimal(t, memberValue(t, swapped, "model_160.occurrence_2.group_2.mppt.module.dc_energy"), "25")
	if memberValue(t, swapped, "model_160.occurrence_2.group_1.mppt.module.label").Text != labels[1] {
		t.Fatal("the swapped label did not move")
	}

	// Swapping the watt-hours moves the namespace values and still publishes
	// neither pack energy element.
	moved, movedSession := publishChain(t, sunspec.ModelsRevisionV1, model160(labels, []uint16{25, 10}))
	requireNoPackEnergy(t, movedSession.k)
	requireNativeDecimal(t, memberValue(t, moved, "model_160.occurrence_2.group_1.mppt.module.dc_energy"), "25")
	requireNativeDecimal(t, memberValue(t, moved, "model_160.occurrence_2.group_2.mppt.module.dc_energy"), "10")
}

type portEnergy struct {
	typ      uint16
	injected uint16
	absorbed uint16
}

// model714 is a fixed aggregate plus one record per port. Injected energy is
// four big-endian words at port offset 13 and absorbed energy at offset 17.
func model714(fixedInjected, fixedAbsorbed uint16, ports []portEnergy) []uint16 {
	n := len(ports)
	words := make([]uint16, 20+25*n)
	words[0], words[1], words[4] = 714, uint16(18+25*n), uint16(n)
	words[10], words[14] = fixedInjected, fixedAbsorbed
	for i, port := range ports {
		base := 20 + 25*i
		words[base] = port.typ
		words[base+16] = port.injected
		words[base+20] = port.absorbed
	}
	return words
}

func memberLocal(p *Publisher, local string) (Native, bool) {
	for _, m := range p.Members() {
		if string(m.Key.Local) == local {
			return m.Native, true
		}
	}
	return Native{}, false
}

func TestSunSpecModel714PortEnergyStaysNativeWithoutAPortType(t *testing.T) {
	pack := packResource("battery-a", "pack-1")
	ports := []portEnergy{{1, 10, 40}, {2, 25, 70}, {0, 3, 4}}
	// Occurrence 2, three port indexes, and the fixed aggregate are separate
	// identities. Treating a raw port type as a storage port, or the fixed
	// aggregate as the pack total, publishes a fact and fails.
	p, s := publishChain(t, sunspec.ModelsRevisionV2, model714(99, 88, ports))
	requireNoPackEnergy(t, s.k)
	requireWithheld(t, p, pack, reasonPortTypeUnnamed)
	requireNativeDecimal(t, memberValue(t, p, "model_714.occurrence_2.der.dc.energy.injected"), "99")
	requireNativeDecimal(t, memberValue(t, p, "model_714.occurrence_2.der.dc.energy.absorbed"), "88")
	for i, port := range ports {
		group := "model_714.occurrence_2.group_" + string(rune('1'+i))
		requireNativeDecimal(t, memberValue(t, p, group+".der.dc.port.energy.injected"), big.NewInt(int64(port.injected)).String())
		requireNativeDecimal(t, memberValue(t, p, group+".der.dc.port.energy.absorbed"), big.NewInt(int64(port.absorbed)).String())
		got := memberValue(t, p, group+".der.dc.port.type")
		if got.Text != "raw_words=["+big.NewInt(int64(port.typ)).String()+"]" {
			t.Fatalf("port %d type %q", i, got.Text)
		}
	}

	r := sunRegistry(t, sunspec.ModelsRevisionV2)
	later, laterSession := publishChain(t, sunspec.ModelsRevisionV2, base802(t, r), model714(99, 88, ports))
	requireNoPackEnergy(t, laterSession.k)
	requireWithheld(t, later, pack, reasonPortTypeUnnamed)
	requireNativeDecimal(t, memberValue(t, later, "model_714.occurrence_3.group_1.der.dc.port.energy.injected"), "10")
	if _, ok := memberLocal(later, "model_714.occurrence_2.group_1.der.dc.port.energy.injected"); ok {
		t.Fatal("the later ports kept the earlier occurrence")
	}

	swapped := []portEnergy{ports[1], ports[0], ports[2]}
	moved, movedSession := publishChain(t, sunspec.ModelsRevisionV2, model714(99, 88, swapped))
	requireNoPackEnergy(t, movedSession.k)
	requireNativeDecimal(t, memberValue(t, moved, "model_714.occurrence_2.group_1.der.dc.port.energy.injected"), "25")
	requireNativeDecimal(t, memberValue(t, moved, "model_714.occurrence_2.group_1.der.dc.port.energy.absorbed"), "70")
	requireNativeDecimal(t, memberValue(t, moved, "model_714.occurrence_2.group_2.der.dc.port.energy.injected"), "10")
	requireNativeDecimal(t, memberValue(t, moved, "model_714.occurrence_2.group_2.der.dc.port.energy.absorbed"), "40")
	if memberValue(t, moved, "model_714.occurrence_2.group_1.der.dc.port.type").Text != "raw_words=[2]" {
		t.Fatal("the swapped port type did not move with its energy")
	}
}
