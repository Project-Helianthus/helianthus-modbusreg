package meter

import (
	"encoding/json"
	"os"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

type replayExpect struct {
	Element, Node, Scope, Unit, Value string
}

type replayCase struct {
	Name, Kind, Role string
	Unit             uint8
	Model            uint16
	Points           map[string][]int
	Registers        []struct {
		Unit   uint8
		Offset uint16
		Words  []uint16
	}
	Expect  []replayExpect
	Unknown []string
}

func replayCases(t *testing.T) []replayCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/replay_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Cases []replayCase }
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Cases
}

// TestReplayCasesPublishTheirExpectedFacts drives the replay cases that the
// factory replay and the second-host acceptance take over.
func TestReplayCasesPublishTheirExpectedFacts(t *testing.T) {
	for _, c := range replayCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			var pub Publication
			cfg := emmaConfig()
			cfg.Huawei[0].Role = Role(c.Role)
			switch c.Kind {
			case "sunspec":
				r := registry(t)
				values := map[string][]uint16{}
				for id, words := range c.Points {
					for _, w := range words {
						values[id] = append(values[id], uint16(w))
					}
				}
				cfg = sunSpecConfig(SunSpecMeter{Unit: c.Unit, Position: 2, Role: Role(c.Role), GridConnection: "gc-1"})
				var err error
				pub, err = testPublisher(t, cfg).Publish(Input{Template: template(), SunSpec: []SunSpecUnit{{Unit: c.Unit, Snapshot: chain(t, r, c.Unit, commonBlock(), meterBlock(t, r, c.Model, values))}}})
				if err != nil {
					t.Fatal(err)
				}
			case "emma":
				g := newEMMARig(t)
				for _, reg := range c.Registers {
					g.m.set(reg.Unit, reg.Offset, reg.Words...)
				}
				pub = publishHuawei(t, cfg, HuaweiCycle{Path: PathEMMA, Cycle: g.cycle()})
			default:
				t.Fatalf("unknown replay kind %q", c.Kind)
			}
			slot := map[string]string{"sunspec": "unit-" + string(rune('0'+c.Unit)) + ".position-2", "emma": "emma-meter"}[c.Kind]
			for _, e := range c.Expect {
				node := nodeOf(t, pub, kindMeter, slot)
				dim := string(node.ID)
				switch e.Node {
				case "grid":
					node = nodeOf(t, pub, kindGridConnection, "gc-1")
					dim = string(node.ID)
				case "load":
					node = nodeOf(t, pub, kindLoad, "emma-site")
					dim = string(node.ID)
				case "site-meter":
					node = nodeOf(t, pub, kindMeter, "emma-site")
					dim = string(node.ID)
				}
				if e.Scope != "" {
					dim += "." + e.Scope
				}
				requireFact(t, pub, node, dim, want{semreg.DefinitionID(e.Element), "", semreg.DefinitionID(e.Unit), e.Value})
			}
			for _, id := range c.Unknown {
				unknownReason(t, pub, nodeOf(t, pub, kindGridConnection, "gc-1"), semreg.DefinitionID(id))
			}
		})
	}
}
