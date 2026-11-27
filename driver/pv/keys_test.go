package pv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func inverterRef(t *testing.T, cfg Config, blocks ...[]uint16) string {
	t.Helper()
	pub := publish(t, cfg, Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, blocks...)}}})
	return string(pub.inverters()[0].Ref.ID)
}

func std(t *testing.T, mn, md, sn string) [][]uint16 {
	return [][]uint16{commonBlock(mn, md, sn), inverterBlock(t, 103, integerValues(103, nil))}
}

type recordingAllocator struct {
	inner Allocator
	keys  []driver.NodeKey
}

func (r *recordingAllocator) Allocate(k driver.NodeKey) (semreg.ResourceID, error) {
	r.keys = append(r.keys, k)
	return r.inner.Allocate(k)
}

// Each field of the node key is stated by the producer: the installation, the
// driver kind, the instance key, the node kind and the anchor (the slot and the
// parent). The recording allocator sees every field; a production change that
// leaves one out of the key fails the matching comparison. Identity is
// decided by the kind, the slot and the parent, which the real allocator keys.
func TestEveryFieldOfTheNodeKeyIsStatedAndDecidesTheIdentity(t *testing.T) {
	cfg := testConfig()
	rec := &recordingAllocator{inner: testAllocator(t)}
	p, err := New(cfg, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Publish(Input{Template: template(), SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, std(t, "Maker", "Model", "SN-1")...)}}}); err != nil {
		t.Fatal(err)
	}
	var inverter driver.NodeKey
	phases := map[string]bool{}
	for _, k := range rec.keys {
		if k.Installation != cfg.Installation || k.Driver != cfg.Driver || k.Instance != cfg.Instance {
			t.Fatalf("key %+v does not carry the configured installation, driver and instance", k)
		}
		switch k.Kind {
		case kindInverter:
			inverter = k
		case kindPhase:
			if k.Anchor.Parent == "" {
				t.Fatalf("phase anchor %+v: a phase is a native slot under its inverter", k.Anchor)
			}
			phases[k.Anchor.Slot] = true
		}
	}
	if inverter.Anchor.Slot == "" || inverter.Anchor.Parent != "" {
		t.Fatalf("inverter anchor %+v", inverter.Anchor)
	}
	if len(phases) != 3 || !phases["L1"] || !phases["L2"] || !phases["L3"] {
		t.Fatalf("phase slots %v", phases)
	}
	base := inverterRef(t, testConfig(), std(t, "Maker", "Model", "SN-1")...)
	if again := inverterRef(t, testConfig(), std(t, "Maker", "Model", "SN-1")...); again != base {
		t.Fatal("the same key gave two identities")
	}
	// the three strings of the Common model make the slot: each one decides it
	vary := map[string]string{
		"manufacturer": inverterRef(t, testConfig(), std(t, "Maker2", "Model", "SN-1")...),
		"model":        inverterRef(t, testConfig(), std(t, "Maker", "Model2", "SN-1")...),
		"serial":       inverterRef(t, testConfig(), std(t, "Maker", "Model", "SN-2")...),
	}
	seen := map[string]string{base: "base"}
	for field, ref := range vary {
		if other, dup := seen[ref]; dup {
			t.Fatalf("varying %s gave the identity of %s", field, other)
		}
		seen[ref] = field
	}
	// the kind and the parent: the PV system, the inverter and each phase have
	// their own identity although they share the slot or the position
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, std(t, "Maker", "Model", "SN-1")...)}}})
	ids := map[string]string{}
	for _, n := range pub.Nodes {
		if other, dup := ids[string(n.Ref.ID)]; dup {
			t.Fatalf("%s and %s share an identity", n.Key.Kind, other)
		}
		ids[string(n.Ref.ID)] = string(n.Key.Kind) + "/" + n.Key.Anchor.Slot
	}
	// two devices that state no complete identity differ by unit and position
	blank := func(unit uint8, lead [][]uint16) string {
		blocks := append(append([][]uint16{}, lead...), commonBlock(" ", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)))
		pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: unit, Snapshot: chain(t, unit, blocks...)}}})
		for _, n := range pub.inverters() {
			if n.Key.Anchor.Slot == fmt.Sprintf("unit-%d.position-%d", unit, 1+len(lead)) {
				return string(n.Ref.ID)
			}
		}
		t.Fatal("no inverter at the position")
		return ""
	}
	a := blank(1, nil)
	b := blank(2, nil)
	third := blank(1, [][]uint16{commonBlock("Maker", "Model", "SN-9"), inverterBlock(t, 103, integerValues(103, nil))})
	if a == b || a == third || b == third {
		t.Fatal("unit or position did not decide the identity of a device without an identity")
	}
}

// The claim states the manufacturer and the serial in their own fields, never
// the model, and its member is the inverter of its own device.
func TestTheClaimCarriesTheManufacturerAndTheSerialNotTheModel(t *testing.T) {
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, std(t, "Maker-X", "Model-Y", "Serial-Z")...)}}})
	c := pub.Claims[0]
	if c.Manufacturer != "Maker-X" || c.Serial != "Serial-Z" || c.Member != pub.inverters()[0].Ref {
		t.Fatalf("claim %+v", c)
	}
	other := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, std(t, "Maker-X", "Model-Y", "Serial-Q")...)}}})
	if other.Claims[0].Proof[0].Digest == c.Proof[0].Digest {
		t.Fatal("the proof does not cover the serial")
	}
}

func TestAPhaseValueNeverFeedsATotalAndATotalNeverFeedsAPhase(t *testing.T) {
	phaseOnly := derBlock(t, 701, 153, map[string][]uint16{"ACType": {2}, "WL1": word(11), "WL2": {0x8000}, "WL3": {0x8000}, "W": {0x8000}, "W_SF": word(0)})
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, derCommonBlock("Maker", "Model", "SN-1"), phaseOnly)}}})
	pub.absent(t, "pv.ac.aggregate_active_power")
	if got := len(pub.factsOf("pv.ac.active_power")); got != 1 {
		t.Fatalf("%d phase powers", got)
	}
	totalOnly := derBlock(t, 701, 153, map[string][]uint16{"ACType": {2}, "W": word(30), "WL1": {0x8000}, "WL2": {0x8000}, "WL3": {0x8000}, "W_SF": word(0)})
	pub = publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, derCommonBlock("Maker", "Model", "SN-1"), totalOnly)}}})
	pub.absent(t, "pv.ac.active_power")
	if got := len(pub.factsOf("pv.ac.aggregate_active_power")); got != 1 {
		t.Fatalf("%d total powers", got)
	}
}

// The lifetime DC energy of a model 160 module has no element whose dimension
// admits an input, and a wrong mapping must not publish it elsewhere.
func TestTheModuleEnergyIsNotMappedToAnotherElement(t *testing.T) {
	mppt := zeros(160, 28)
	mppt[8] = 1
	mppt[22], mppt[23] = 0, 500
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)), mppt)}}})
	pub.absent(t, "pv.dc.energy")
	listed := false
	for _, w := range pub.Withheld {
		listed = listed || w.Reason == "pv.dc.energy_dimension_is_string_not_input"
	}
	if !listed {
		t.Fatal("the module energy is not listed with its reason")
	}
}

// A grade of evidence rejects nothing: a hypothesis-graded dongle-family
// reading publishes like any other.
func TestAnEvidenceGradeDoesNotRejectAPublication(t *testing.T) {
	f := fact("huawei.sun2000.mc.frequency", "Hz", "", 5000, -2)
	f.Grade = huawei.GradeHypothesis
	pub := childOf(t, f)
	requireQuantity(t, pub, "pv.ac.frequency", string(pub.node(t, kindInverter, "instance.synthetic/1#1").ID), "unit.hertz", "50")
}

// A reading whose unit is not the row's unit is not converted by guess.
func TestAReadingInAnotherUnitIsNotConverted(t *testing.T) {
	pub := childOf(t, fact("huawei.sun2000.mc.frequency", "kHz", "", 5, -2))
	pub.absent(t, "pv.ac.frequency")
}

// A device that states a complete Common identity keeps the anchor the earlier
// publisher gave it: the inverter and its own PV system share the slot that is
// the digest of the three strings under their domain, and a phase is a native
// slot under the inverter.
func TestADeviceKeepsTheAnchorOfTheEarlierPublisher(t *testing.T) {
	cfg := testConfig()
	cfg.System = ""
	pub := publish(t, cfg, Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, std(t, "Maker", "Model", "SN-1")...)}}})
	sum := sha256.Sum256([]byte("pv-operator-slot\x00Maker\x00Model\x00SN-1"))
	slot := hex.EncodeToString(sum[:])[:32]
	inverter := pub.node(t, kindInverter, slot)
	system := pub.node(t, kindSystem, slot)
	if inverter == system {
		t.Fatal("the inverter and its system share one identity")
	}
	for _, n := range pub.Nodes {
		if n.Key.Kind == kindPhase && n.Key.Anchor.Parent != inverter.ID {
			t.Fatalf("phase %s is not under the inverter", n.Key.Anchor.Slot)
		}
	}
}
