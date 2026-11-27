package storage

import (
	"strings"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/sunspec"
)

func sunRegistry(t *testing.T, revision sunspec.SchemaRevision) sunspec.DecoderRegistry {
	t.Helper()
	r, err := sunspec.NewStandardSunSpecDecoderRegistry(revision)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// block builds the words of one model occurrence from named points, each a
// field identifier with its words. Every unnamed word is zero.
func block(t *testing.T, r sunspec.DecoderRegistry, id, length uint16, revision sunspec.SchemaRevision, points map[string][]uint16) []uint16 {
	t.Helper()
	key := sunspec.DecoderKey{ModelID: id, ModelLength: length, SchemaRevision: revision}
	words := make([]uint16, int(length)+2)
	words[0], words[1] = id, length
	for field, value := range points {
		if base, isScale := strings.CutSuffix(field, "#sf"); isScale {
			_, scale, ok := r.FixedPointLayout(key, base)
			if !ok || scale.WordWidth != 1 || len(value) != 1 {
				t.Fatalf("model %d scale of %s: %+v", id, base, scale)
			}
			words[scale.WordOffset] = value[0]
			continue
		}
		point, _, ok := r.FixedPointLayout(key, field)
		if !ok || int(point.WordWidth) != len(value) {
			t.Fatalf("model %d point %s: layout %+v ok=%v for %d words", id, field, point, ok, len(value))
		}
		copy(words[point.WordOffset:], value)
	}
	return words
}

func padded(text string, words int) []uint16 {
	out := make([]uint16, words)
	for i := 0; i < len(text) && i/2 < words; i++ {
		out[i/2] |= uint16(text[i]) << (8 * (1 - uint(i%2)))
	}
	return out
}

func commonBlock(t *testing.T, r sunspec.DecoderRegistry, revision sunspec.SchemaRevision) []uint16 {
	length := uint16(65)
	if revision == sunspec.ModelsRevisionV2 {
		length = 66
	}
	return block(t, r, 1, length, revision, map[string][]uint16{"device.manufacturer": padded("Synthetic maker", 16), "device.model": padded("Storage family", 16), "device.version": padded("1.0", 8), "device.serial": padded("sanitized", 16)})
}

// chain walks a chain plan over a word image without a socket: every request
// is answered from the image as one logical view.
func chain(t *testing.T, revision sunspec.SchemaRevision, keys []sunspec.DecoderKey, blocks ...[]uint16) sunspec.ChainSnapshot {
	t.Helper()
	cells := []uint16{0x5375, 0x6e53}
	for _, b := range blocks {
		cells = append(cells, b...)
	}
	cells = append(cells, 0xffff, 0)
	plan, err := sunspec.NewChainPlan(sunspec.ChainPlanSpec{BaseCandidates: []uint16{0}, SchemaRevision: revision, DecoderKeys: keys,
		Limits: sunspec.ChainLimits{MaxTotalWords: uint32(len(cells)), MaxOccurrences: uint32(len(blocks))}})
	if err != nil {
		t.Fatal(err)
	}
	c := sunspec.NewChain(plan)
	var snap sunspec.ChainSnapshot
	var id uint64
	for next := c.NextRequests(); len(next) > 0; next = c.NextRequests() {
		for _, request := range next {
			id++
			words := append([]uint16(nil), cells[request.Address():int(request.Address())+int(request.WordCount())]...)
			view, err := reg.NewLogicalViewSnapshot(reg.LogicalViewRecord{LogicalViewID: id, WireResponseID: id + 100, PhysicalRequestID: id + 200,
				Endpoint: "fixture-endpoint", ConnectionID: 4, Transport: reg.TransportTCP, TransportGeneration: 5, UnitID: 1,
				RequestedFunction: reg.FunctionReadHoldingRegisters, ReceivedFunction: reg.FunctionReadHoldingRegisters, Table: reg.HoldingRegisters,
				PhysicalOffset: request.Address(), PhysicalWordCount: request.WordCount(), AuthorizationScope: "fixture-read-only", PollGeneration: 6, DeadlineIdentity: 7,
				LogicalOffset: request.Address(), LogicalWordCount: request.WordCount(), SliceWordCount: request.WordCount(), Words: words, WireResponseBytes: []byte{0, byte(id)}})
			if err != nil {
				t.Fatal(err)
			}
			if snap, err = c.AdmitReplay(request, view); err != nil {
				t.Fatalf("admit: %v", err)
			}
		}
	}
	return snap
}

func decisions(t *testing.T, snap sunspec.ChainSnapshot) []sunspec.CapabilityDecision {
	t.Helper()
	d, err := sunspec.EvaluateCapabilities(snap)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
