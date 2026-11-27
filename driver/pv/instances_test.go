package pv

import (
	"fmt"
	"sync"
	"testing"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// An inverter segment of an inverter-plus-meter chain publishes; the meter
// segment is left to its own producer.
func TestAnInverterSegmentOfAnInverterAndMeterChain(t *testing.T) {
	pub := publish(t, testConfig(), Input{SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: chain(t, 1,
		commonBlock("Maker", "Model", "SN-A"), inverterBlock(t, 103, integerValues(103, nil)),
		commonBlock("Maker", "Meter", "SN-M"), zeros(203, 105))}}})
	if len(pub.inverters()) != 1 || len(pub.Claims) != 1 {
		t.Fatalf("%d inverters, %d claims", len(pub.inverters()), len(pub.Claims))
	}
	for _, n := range pub.Nodes {
		if n.Key.Kind != kindInverter && n.Key.Kind != kindSystem && n.Key.Kind != kindPhase && n.Key.Kind != kindEquipment {
			t.Fatalf("a %s node was published for the meter segment", n.Key.Kind)
		}
	}
}

// Several instances run side by side with nothing global.
func TestSeveralInstancesRunSideBySide(t *testing.T) {
	blocks := [][]uint16{commonBlock("Maker", "Model", "SN-A"), inverterBlock(t, 103, integerValues(103, nil))}
	snapshot := chain(t, 1, blocks...)
	var wg sync.WaitGroup
	counts := make([]int, 4)
	for i := range counts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := testConfig()
			cfg.Instance = driver.InstanceKey(fmt.Sprint("instance.", i))
			pub, err := testPublisher(t, cfg).Publish(Input{Template: template(), SunSpec: []SunSpecUnit{{Unit: 1, Snapshot: snapshot}}})
			if err != nil {
				t.Error(err)
				return
			}
			counts[i] = len(pub.Facts)
		}(i)
	}
	wg.Wait()
	for _, n := range counts {
		if n == 0 || n != counts[0] {
			t.Fatalf("counts %v", counts)
		}
	}
}

func TestARetainedReadingIsPublishedDegraded(t *testing.T) {
	f := fact("huawei.sun2000.mc.frequency", "Hz", "", 5000, -2)
	f.Retained = true
	pub := childOf(t, f)
	got, ok := pub.find("pv.ac.frequency", string(pub.node(t, kindInverter, "instance.synthetic/1#1").ID))
	if !ok || got.Candidate.Quality.Availability != semreg.AvailabilityDegraded {
		t.Fatalf("retained reading: %+v", got.Candidate.Quality)
	}
}
