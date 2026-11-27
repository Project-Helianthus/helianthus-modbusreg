package pv

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Project-Helianthus/helianthus-modbusreg/fronius"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

func zeros(model, length uint16) []uint16 {
	b := make([]uint16, int(length)+2)
	b[0], b[1] = model, length
	return b
}

// froniusBlocks is the observed model chain of the recorded flavor, with the
// Common model of the given firmware.
func froniusBlocks(t *testing.T, manufacturer, firmware string) [][]uint16 {
	t.Helper()
	common := commonBlock(manufacturer, "Symo GEN24 10.0", "SN-F")
	copy(common[42:50], stringWords(firmware, 8))
	mppt := zeros(160, 88)
	mppt[8] = 4
	return [][]uint16{common, inverterBlock(t, 113, floatValues(113, nil)), zeros(120, 26), zeros(121, 30), zeros(122, 44), zeros(123, 24), mppt, zeros(124, 24)}
}

// shape lists every published fact without the identifiers that name its subject.
func shape(p Publication) []string {
	var out []string
	for _, f := range p.Facts {
		v := f.Candidate.Value
		switch {
		case v.Quantity != nil:
			out = append(out, fmt.Sprint(f.Candidate.Key.FactID, "|", v.Quantity.Unit, "|", v.Quantity.Number.Coefficient, "e", v.Quantity.Number.Exponent10))
		case v.Symbol != nil:
			out = append(out, fmt.Sprint(f.Candidate.Key.FactID, "|", v.Symbol.Token))
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestACapabilityAdmittedChainPublishesTheSameFactsForAnyManufacturer(t *testing.T) {
	fr := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, froniusBlocks(t, "Fronius", "1.41.11-1")...)}}})
	other := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, froniusBlocks(t, "Other Maker", "1.41.11-1")...)}}})
	if len(shape(fr)) == 0 || !reflect.DeepEqual(shape(fr), shape(other)) {
		t.Fatalf("manufacturer changed the facts:\n%v\n%v", shape(fr), shape(other))
	}
	level := func(p Publication) string {
		for _, d := range p.Decisions {
			if d.Capability == "sunspec.inverter.monitoring" {
				if !d.Go {
					t.Fatalf("decision %+v", d)
				}
				return d.Level
			}
		}
		t.Fatal("no inverter decision")
		return ""
	}
	if got := level(fr); got != fronius.SunSpecFroniusObservedFlavorV11ID {
		t.Fatalf("recorded flavor level %q", got)
	}
	if got := level(other); got != "" {
		t.Fatalf("level %q for a chain that is not Fronius", got)
	}
	// a Fronius chain at an unrecorded firmware publishes; the level is empty and decides nothing
	unrecorded := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, froniusBlocks(t, "Fronius", "9.9.9-9")...)}}})
	if !reflect.DeepEqual(shape(fr), shape(unrecorded)) || level(unrecorded) != "" {
		t.Fatal("an unrecorded firmware changed what is published")
	}
}

func TestAChainTheRegistryDoesNotAdmitIsNoGoWithItsReason(t *testing.T) {
	bad := inverterBlock(t, 103, integerValues(103, map[string][]uint16{"inverter.scale.current": {0x8000}}))
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN"), bad)}}})
	if len(pub.inverters()) != 0 || len(pub.Facts) != 0 {
		t.Fatalf("a rejected source published: %d inverters %d facts", len(pub.inverters()), len(pub.Facts))
	}
	if len(pub.Decisions) == 0 {
		t.Fatal("no decision recorded")
	}
	if d := pub.Decisions[0]; d.Go || d.Reason != "INVALID_REQUIRED_FACT" {
		t.Fatalf("decision %+v", d)
	}
}

func nodeKeys(p Publication) map[string]driver.NodeKey {
	out := map[string]driver.NodeKey{}
	for _, n := range p.Nodes {
		out[string(n.Ref.ID)] = n.Key
	}
	return out
}

func linkKeys(p Publication) []string {
	var out []string
	for _, l := range append(append([]driver.LinkClaim{}, p.Links...), p.DongleLinks...) {
		out = append(out, fmt.Sprint(l.Key.Type.ID, "|", l.Key.From.ID, "|", l.Key.To.ID))
	}
	sortStrings(out)
	return out
}

func TestRelationsAndTheirBasis(t *testing.T) {
	blocks := [][]uint16{commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)), zeros(160, 28)}
	blocks[2][8] = 1
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, blocks...)}}})
	inv := pub.inverters()[0].Ref
	counts := map[string]int{}
	for _, l := range pub.Links {
		counts[string(l.Key.Type.ID)]++
		if len(l.Basis) != 1 {
			t.Fatalf("link %v has %d basis items", l.Key.Type.ID, len(l.Basis))
		}
		switch l.Key.Type.ID {
		case "relation.relays":
			if l.Key.From != pub.node(t, kindEquipment, "endpoint-1") || l.Key.To != inv || l.Basis[0].Kind != "evidence.native_observation" {
				t.Fatalf("relays %+v", l)
			}
		default:
			if l.Basis[0].Kind != "evidence.gate_rule" {
				t.Fatalf("%s basis %s", l.Key.Type.ID, l.Basis[0].Kind)
			}
		}
	}
	for id, want := range map[string]int{"pv.relation.system_inverter": 1, "pv.relation.inverter_phase": 3, "pv.relation.inverter_input": 1, "relation.relays": 1} {
		if counts[id] != want {
			t.Fatalf("%s: %d, want %d (%v)", id, counts[id], want, counts)
		}
	}
}

func TestWithoutAnEndpointTheRelaysRelationStaysUnknown(t *testing.T) {
	cfg := testConfig()
	cfg.Endpoint = ""
	pub := publish(t, cfg, Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)))}}})
	for _, l := range pub.Links {
		if l.Key.Type.ID == "relation.relays" {
			t.Fatal("a relays link was claimed without an endpoint")
		}
	}
	if len(pub.UnknownLinks) != 1 || pub.UnknownLinks[0].Reason != "endpoint_not_configured" {
		t.Fatalf("unknown links %+v", pub.UnknownLinks)
	}
}

// Two lifecycles that see the units in reverse discovery order give the same
// node ids and link keys, because every anchor is a native position or a
// device-reported identity.
func TestTwoLifecyclesInReverseDiscoveryOrderGiveTheSameIdentities(t *testing.T) {
	one := SunSpecUnit{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-A"), inverterBlock(t, 103, integerValues(103, nil)))}
	two := SunSpecUnit{Unit: 2, Snapshot: chain(t, 2, commonBlock(" ", "Model", "SN-B"), inverterBlock(t, 103, integerValues(103, nil)),
		commonBlock("Maker", "Model", "SN-C"), inverterBlock(t, 103, integerValues(103, nil)))}
	first := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{one, two}})
	second := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{two, one}})
	if !reflect.DeepEqual(nodeKeys(first), nodeKeys(second)) || !reflect.DeepEqual(linkKeys(first), linkKeys(second)) {
		t.Fatal("the order of discovery changed an identity")
	}
	if len(first.inverters()) != 3 {
		t.Fatalf("%d inverters", len(first.inverters()))
	}
	a, b := fmt.Sprint(first.Claims), fmt.Sprint(second.Claims)
	if len(first.Claims) != 2 || a != b {
		t.Fatalf("claims differ: %s / %s", a, b)
	}
	var positional int
	for _, n := range first.Nodes {
		if n.Key.Kind == kindInverter && n.Key.Anchor.Slot == "unit-2.position-1" {
			positional++
		}
	}
	if positional != 1 {
		t.Fatal("a device without a complete identity is anchored on its native chain position")
	}
}

// TestSunSpecManufacturerSerialClaim is the producer side of the SunSpec plus
// remote eeBUS inverter case. The manufacturer and serial of the test are
// synthetic strings, not a pair of any real device.
func TestSunSpecManufacturerSerialClaim(t *testing.T) {
	complete := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock("Maker", "Model", "SN-1"), inverterBlock(t, 103, integerValues(103, nil)))}}})
	if len(complete.Claims) != 1 {
		t.Fatalf("%d claims", len(complete.Claims))
	}
	c := complete.Claims[0]
	if c.Member != complete.inverters()[0].Ref || c.Manufacturer != "Maker" || c.Serial != "SN-1" || len(c.Proof) != 1 || c.Proof[0].Kind != "evidence.common_model" {
		t.Fatalf("claim %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ mn, sn, reason string }{
		{" ", "SN-1", "manufacturer_absent"}, {"Maker", " ", "serial_absent"}, {" ", " ", "manufacturer_and_serial_absent"},
	} {
		pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1, commonBlock(tc.mn, "Model", tc.sn), inverterBlock(t, 103, integerValues(103, nil)))}}})
		if len(pub.Claims) != 0 || len(pub.Unclaimed) != 1 || pub.Unclaimed[0].Reason != tc.reason {
			t.Fatalf("%q/%q: claims %d unclaimed %+v", tc.mn, tc.sn, len(pub.Claims), pub.Unclaimed)
		}
	}
}
