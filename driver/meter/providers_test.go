package meter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/huawei"
)

// The logger and S-Dongle tests run the real read-only providers over a
// register image, as the EMMA tests do. The identity pages come from the
// committed synthetic identity fixtures of the huawei package.

const identityFixtures = "../../testdata/huawei/identity/"

type fixtureObject struct {
	ID     string `json:"object_id"`
	Count  *uint8 `json:"count"`
	Entry  string `json:"entry"`
	PadTo  int    `json:"pad_to"`
	Text   string `json:"text"`
	Opaque int    `json:"opaque_bytes"`
}

type fixturePage struct {
	Request    string          `json:"request_object_id"`
	Conformity string          `json:"conformity"`
	More       string          `json:"more_follows"`
	Next       string          `json:"next_object_id"`
	Declared   int             `json:"declared_objects"`
	Objects    []fixtureObject `json:"objects"`
}

func hexByte(t *testing.T, s string) uint8 {
	t.Helper()
	v, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	return uint8(v)
}

// fixturePages rebuilds the identification pages of an identity fixture, after
// the edit applied to every text.
func fixturePages(t *testing.T, name string, edit func(string) string) (uint8, []fixturePageOut) {
	t.Helper()
	raw, err := os.ReadFile(identityFixtures + name)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Read struct {
			Code uint8 `json:"read_device_id_code"`
		} `json:"read"`
		Pages []fixturePage `json:"pages"`
		Reply *fixturePage  `json:"reply"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	all := f.Pages
	if f.Reply != nil {
		all = []fixturePage{*f.Reply}
	}
	var pages []fixturePageOut
	for _, p := range all {
		out := fixturePageOut{page: huawei.IdentificationPage{ReadDeviceIDCode: f.Read.Code, Conformity: hexByte(t, p.Conformity), DeclaredObjects: p.Declared,
			More: hexByte(t, p.More) != 0, NextObjectID: hexByte(t, p.Next)}}
		if p.Request != "" {
			out.request = hexByte(t, p.Request)
		}
		size := 7
		for _, o := range p.Objects {
			var value []byte
			switch {
			case o.Count != nil:
				value = []byte{*o.Count}
			case o.Entry != "":
				value = make([]byte, o.PadTo)
				copy(value, edit(o.Entry))
			case o.Text != "":
				value = []byte(edit(o.Text))
			default:
				value = make([]byte, o.Opaque)
			}
			out.page.Objects = append(out.page.Objects, huawei.GatewayInventoryObject{ObjectID: hexByte(t, o.ID), Value: value})
			size += 2 + len(value)
		}
		out.page.ResponseBytes = size
		pages = append(pages, out)
	}
	return f.Read.Code, pages
}

type fixturePageOut struct {
	request uint8
	page    huawei.IdentificationPage
}

func (m *image) addPages(unit uint8, code uint8, pages []fixturePageOut) {
	for _, p := range pages {
		page := p.page
		m.pages[fmt.Sprintf("%d/2b/%d/%d", unit, code, p.request)] = &page
	}
}

func (m *image) refuseID(unit uint8, code uint8, object uint8, exception uint8) {
	if m.idRefs == nil {
		m.idRefs = map[string]uint8{}
	}
	m.idRefs[fmt.Sprintf("%d/2b/%d/%d", unit, code, object)] = exception
}

// deviceRig runs one real provider against a register image.
type deviceRig struct {
	t       *testing.T
	m       *image
	now     time.Time
	refresh func(context.Context) (huawei.ProviderCycle, error)
	unit    uint8
}

func newDeviceRig(t *testing.T) *deviceRig {
	t.Helper()
	g := &deviceRig{t: t, m: newImage(), now: time.Unix(1_000_000, 0).UTC()}
	g.m.clock = &g.now
	return g
}

func (g *deviceRig) config(allowed ...uint8) huawei.ProviderConfig {
	return huawei.ProviderConfig{Source: "endpoint-a", AllowedUnits: allowed, Clock: func() time.Time { return g.now },
		Wait: func(ctx context.Context, d time.Duration) error { g.now = g.now.Add(d); return ctx.Err() }}
}

func (g *deviceRig) cycle() huawei.ProviderCycle {
	g.t.Helper()
	g.now = g.now.Add(time.Hour)
	c, err := g.refresh(context.Background())
	if err != nil || c.Outcome != huawei.ProviderCycleComplete {
		g.t.Fatalf("cycle %s (%s): %v; asked %v", c.Outcome, c.Reason, err, g.m.asked)
	}
	return c
}

// block builds a register block of the given length with words placed at their
// register offsets.
func block(base, length uint16, at map[uint16][]uint16) []uint16 {
	out := make([]uint16, length)
	for offset, w := range at {
		copy(out[offset-base:], w)
	}
	return out
}

func int32Words(v int32) []uint16 { return words32(uint32(v)) }

// newSDongleRig starts the S-Dongle provider with the configured child inverter
// at the given routed unit and answers its identity and groups from the image.
func newSDongleRig(t *testing.T, unit uint8) *deviceRig {
	t.Helper()
	g := newDeviceRig(t)
	m := g.m
	m.set(100, 37410, 4, 0, 7)
	m.set(100, 37411, 0)
	m.set(100, 30068, 50, 0)
	m.set(100, 37429, 10)
	m.set(100, 30050, 22066, 12336, 21040, 12850, 17201, 12371, 20547, 12336, 12544, 0, 0, 0, 0, 0, 0)
	m.refuseID(100, 1, 0, 1)
	model := make([]byte, 30)
	copy(model, "SUN2000-8KTL-M1")
	copy(model[16:], "00000000-000")
	words := make([]uint16, 15)
	for i := range words {
		words[i] = uint16(model[2*i])<<8 | uint16(model[2*i+1])
	}
	m.set(unit, 30000, words...)
	m.set(unit, 30070, 428)
	m.set(unit, 30050, textWords("V100R001C00SPC100", 15)...)
	m.set(unit, 30035, textWords("X000X000-00", 15)...)
	route := func(s string) string { return strings.ReplaceAll(s, ";5=1;", ";5="+strconv.Itoa(int(unit))+";") }
	for _, name := range []string{"sdongle_child_basic_reply.json", "sdongle_child_extended_inventory.json"} {
		code, pages := fixturePages(t, name, route)
		m.addPages(unit, code, pages)
	}
	p, err := huawei.NewSDongleReadOnlyProvider(g.config(unit))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(m); err != nil {
		t.Fatal(err)
	}
	g.refresh, g.unit = p.Refresh, unit
	g.plant(0, 0)
	g.meter(0, 980)
	return g
}

// plant sets the plant power group of the gateway unit: consumption and grid
// exchange in watts (kW, gain 1000).
func (g *deviceRig) plant(consumption uint32, exchange int32) {
	g.m.set(100, 37498, cat(words32(0), words32(consumption), int32Words(exchange), words32(0))...)
	g.m.set(100, 37516, 0, 0)
}

// meter sets the inverter-side meter groups of the routed child unit: the
// active power in watts (gain 1) and the power factor (gain 1000).
func (g *deviceRig) meter(power int32, pf uint16) {
	u := g.unit
	g.m.set(u, 37100, 84)
	g.m.set(u, 37101, block(37101, 24, map[uint16][]uint16{
		37101: int32Words(2300), 37103: int32Words(2310), 37105: int32Words(2320),
		37107: int32Words(410), 37109: int32Words(420), 37111: int32Words(430),
		37113: int32Words(power), 37117: {pf}, 37119: int32Words(1234), 37121: int32Words(567),
	})...)
	g.m.set(u, 37125, 85)
	g.m.set(u, 37126, block(37126, 13, map[uint16][]uint16{37132: int32Words(200), 37134: int32Words(150), 37136: int32Words(150)})...)
}

// loggerBuild is the gateway build a logger rig reports; the edition decides
// whether the meter power sign is stated.
const (
	loggerObservedBuild = "V300R023C10SPC620"
	loggerSignedBuild   = "V300R025C10SPC620"
)

// newLoggerRig starts the SmartLogger provider on the fixture listing, which
// lists the meters at units 11 and 19, with the given gateway build.
func newLoggerRig(t *testing.T, build string) *deviceRig {
	t.Helper()
	g := newDeviceRig(t)
	g.m.set(0, 65521, 7)
	edit := func(s string) string { return strings.ReplaceAll(s, loggerObservedBuild, build) }
	for _, name := range []string{"smartlogger_extended_inventory.json", "smartlogger_basic_reply.json"} {
		code, pages := fixturePages(t, name, edit)
		g.m.addPages(0, code, pages)
	}
	p, err := huawei.NewSmartLoggerReadOnlyProvider(g.config(11, 19))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(g.m); err != nil {
		t.Fatal(err)
	}
	g.refresh = p.Refresh
	return g
}

// loggerMeter sets the meter groups of a listed meter unit: the total active
// power in watts (kW, gain 1000) and the power factor (gain 1000). The phase
// A voltage is 230.01 V and the energy counters are 1000.00, 200.00 and 800.00 kWh.
func (g *deviceRig) loggerMeter(unit uint8, power int32, pf uint16) {
	g.m.set(unit, 32260, block(32260, 33, map[uint16][]uint16{
		32260: words32(23001), 32272: int32Words(114), 32278: int32Words(power), 32284: {pf},
	})...)
	g.m.set(unit, 32335, block(32335, 30, map[uint16][]uint16{
		32335: int32Words(250), 32341: words64(100000), 32349: words64(20000), 32357: words64(80000),
	})...)
}
