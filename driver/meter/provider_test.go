package meter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbus"
	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
)

// image answers the requests of a provider from register images and
// identification pages, by address.
type image struct {
	mu      sync.Mutex
	words   map[string][]uint16
	pages   map[string]*huawei.IdentificationPage
	asked   []string
	clock   *time.Time
	closes  int
	exclude map[string]bool
}

func newImage() *image {
	return &image{words: map[string][]uint16{}, pages: map[string]*huawei.IdentificationPage{}}
}

func addr(unit uint8, offset, quantity uint16) string {
	return fmt.Sprintf("%d/%d/%d", unit, offset, quantity)
}

func (m *image) set(unit uint8, offset uint16, words ...uint16) {
	m.words[addr(unit, offset, uint16(len(words)))] = words
}

func (m *image) ReadHolding(_ context.Context, unit byte, r modbus.ReadRegistersRequest) (modbus.ReadRegistersResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := addr(unit, r.Offset(), r.Quantity())
	m.asked = append(m.asked, key)
	*m.clock = m.clock.Add(20 * time.Millisecond)
	words, ok := m.words[key]
	if !ok || m.exclude[key] {
		return modbus.ReadRegistersResponse{}, &modbus.ProtocolError{Kind: modbus.ErrorExceptionResponse, RequestedFunction: r.Function(), ReceivedFunction: r.Function() | 0x80, ExceptionCode: 2, Offset: -1}
	}
	return modbus.ReadRegistersResponse{Words: append([]uint16(nil), words...), Provenance: modbus.ReadProvenance{Function: r.Function(), Table: r.Table(), Offset: r.Offset(), Quantity: r.Quantity()}}, nil
}

func (m *image) ReadDeviceIdentification(_ context.Context, unit byte, r huawei.IdentificationRequest) (huawei.IdentificationPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d/2b/%d/%d", unit, r.ReadDeviceIDCode, r.ObjectID)
	m.asked = append(m.asked, key)
	*m.clock = m.clock.Add(20 * time.Millisecond)
	if p, ok := m.pages[key]; ok {
		return *p, nil
	}
	return huawei.IdentificationPage{}, errors.New("no page")
}

func (m *image) Close() error { m.closes++; return nil }

func textWords(text string, words uint16) []uint16 {
	b := make([]byte, 2*int(words))
	copy(b, text)
	out := make([]uint16, words)
	for i := range out {
		out[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return out
}

// emmaRig runs the EMMA provider against a register image.
type emmaRig struct {
	t        *testing.T
	m        *image
	now      time.Time
	provider *huawei.EMMAReadOnlyProvider
}

func words32(v uint32) []uint16 { return []uint16{uint16(v >> 16), uint16(v)} }
func words64(v uint64) []uint16 {
	return []uint16{uint16(v >> 48), uint16(v >> 32), uint16(v >> 16), uint16(v)}
}

func cat(parts ...[]uint16) []uint16 {
	var out []uint16
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func newEMMARig(t *testing.T) *emmaRig {
	t.Helper()
	g := &emmaRig{t: t, m: newImage(), now: time.Unix(1_000_000, 0).UTC()}
	g.m.clock = &g.now
	g.m.set(0, 30000, textWords("SmartHEMS", 15)...)
	g.m.set(0, 30222, textWords("EMMA-A01", 20)...)
	g.m.set(0, 30035, textWords("SmartHEMS V100R025C00SPC102", 15)...)
	self, child := "1=EMMA-A01;2=V100R025C00SPC102;5=0;8=HEMS", "1=SUN2000-SYNTHETIC;5=3;8=SUN2000"
	g.m.pages["0/2b/3/135"] = &huawei.IdentificationPage{ReadDeviceIDCode: 3, Conformity: 3, DeclaredObjects: 3, ResponseBytes: 7 + 7 + len(self) + len(child),
		Objects: []huawei.GatewayInventoryObject{{ObjectID: 0x87, Value: []byte{2}}, {ObjectID: 0x88, Value: []byte(self)}, {ObjectID: 0x89, Value: []byte(child)}}}
	p, err := huawei.NewEMMAReadOnlyProvider(huawei.ProviderConfig{Source: "endpoint-a", AllowedUnits: []uint8{3}, Clock: func() time.Time { return g.now },
		Wait: func(ctx context.Context, d time.Duration) error { g.now = g.now.Add(d); return ctx.Err() }})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(g.m); err != nil {
		t.Fatal(err)
	}
	g.provider = p
	return g
}

// site sets the household, feed-in and supply counters (kWh, gain 100) and the
// power words (kW, gain 1000) of the EMMA site groups.
func (g *emmaRig) site(day, total [3]uint64, pv, load uint32, exchange int32) {
	w := make([]uint16, 46)
	copy(w[18:], cat(words32(uint32(day[0])), words64(total[0]), words32(uint32(day[1])), words64(total[1]), words32(uint32(day[2])), words64(total[2])))
	g.m.set(0, 30306, w...)
	g.m.set(0, 30354, cat(words32(pv), words32(load), words32(uint32(exchange)), words32(0), words32(0), words32(0))...)
	g.m.set(0, 30368, 0)
}

func (g *emmaRig) meter(power int32) {
	g.m.set(0, 31639, cat(words32(23010), words32(23120), words32(23230), words32(0), words32(0), words32(0), words32(125), words32(130), words32(135), words32(uint32(power)))...)
	g.m.set(0, 31661, cat(words32(980), words32(0), words32(100), words32(200), words32(300))...)
	g.m.set(0, 31671, words64(5000)...)
	g.m.set(0, 31679, words64(1000)...)
	g.m.set(0, 31687, words64(4000)...)
}

func (g *emmaRig) cycle() huawei.ProviderCycle {
	g.t.Helper()
	g.now = g.now.Add(time.Hour)
	c, err := g.provider.Refresh(context.Background())
	if err != nil || c.Outcome != huawei.ProviderCycleComplete {
		g.t.Fatalf("cycle %s (%s): %v; asked %v", c.Outcome, c.Reason, err, g.m.asked)
	}
	return c
}
