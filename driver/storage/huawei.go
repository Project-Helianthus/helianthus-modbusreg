package storage

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	hw "github.com/Project-Helianthus/helianthus-modbusreg/huawei"
)

const (
	huaweiProfile  = "huawei.storage.monitoring"
	huaweiProtocol = "modbus_tcp"
	huaweiNative   = "native.huawei.storage.observation"
	huaweiRegistry = "registry.huawei.storage.mapping"

	reasonNoInverterBattery = "the child's documented edition defines no inverter-side battery groups; only the site battery fields are published"
	reasonSignNotStated     = "the documented sign of the power word is not stated"
)

// HuaweiBattery configures one child unit that carries battery groups and the
// operator slot of the battery they belong to.
type HuaweiBattery struct {
	Unit    uint8
	Battery string
}

// HuaweiChild is the decoded battery evidence of one child unit: its typed
// facts and whether its documented edition defines the inverter-side battery
// groups.
type HuaweiChild struct {
	Unit            uint8
	Facts           []hw.SUN2000Fact
	BatteryAdmitted bool
}

// HuaweiObservation is one provider cycle's battery evidence with its receipt:
// the children's typed facts, and the EMMA unit 0 site groups with the site
// power readings derived from them. A fact that comes from a retained reading
// is not published again: the facts published from the original reading stay
// with their receipt and age from it.
type HuaweiObservation struct {
	Children  []HuaweiChild
	Site      []hw.EMMASiteGroup
	SitePower []hw.EMMASitePowerReading
	Receipt   Receipt
}

// HuaweiFromCycle takes the battery evidence of a provider cycle.
func HuaweiFromCycle(c hw.ProviderCycle, receipt Receipt) HuaweiObservation {
	out := HuaweiObservation{Site: c.EMMASite, SitePower: c.EMMASitePower, Receipt: receipt}
	for _, child := range c.Children {
		out.Children = append(out.Children, HuaweiChild{Unit: child.UnitID, Facts: child.Reading.Facts(), BatteryAdmitted: child.Reading.BatteryFactsAdmitted()})
	}
	return out
}

// huaweiUnitRow is a row of a storage unit's group, keyed by the offset of
// storage unit 1; the same row of unit 2 has its own offset.
type huaweiRow struct {
	Offset uint16
	Row
}

var (
	rowVoltage = Row{NativeUnit: "V", Element: "storage.pack.voltage", Dimension: dimPack, Unit: unitVolt, Conv: identity}
	rowCurrent = Row{NativeUnit: "A", Element: "storage.pack.current", Dimension: dimPack, Unit: unitAmpere, Conv: identity}
	rowSOC     = Row{NativeUnit: "%", Element: "storage.state.soc", Dimension: dimPack, Unit: unitRatio, Conv: percent}
	rowTemp    = Row{NativeUnit: "℃", Element: "storage.temperature.pack", Dimension: dimPack, Unit: unitKelvin, Conv: toKelvin}
	rowCharged = Row{NativeUnit: "kWh", Element: "storage.energy.charged", Dimension: dimPack, Unit: unitJoule, Conv: kwhToJoule, Loss: lossCounter}
	rowDischrg = Row{NativeUnit: "kWh", Element: "storage.energy.discharged", Dimension: dimPack, Unit: unitJoule, Conv: kwhToJoule, Loss: lossCounter}
	rowRated   = Row{NativeUnit: "Wh", Element: "storage.energy.nominal_capacity", Dimension: dimPack, Unit: unitJoule, Conv: whToJoule, ZeroAbsent: true}
	rowChgRate = Row{NativeUnit: "W", Element: "storage.limit.charge_power", Dimension: dimInterface, Unit: unitWatt, Conv: identity}
	rowDisRate = Row{NativeUnit: "W", Element: "storage.limit.discharge_power", Dimension: dimInterface, Unit: unitWatt, Conv: identity}
)

func named(r Row, native string) Row { r.Native = native; return r }

// huaweiUnitRows are the rows of the storage-unit groups: unit 1 at offsets
// 37000 through 37068 and its rated capacity at 37758, and unit 2 at 37741
// through 37755.
func huaweiUnitRows(unit int) []huaweiRow {
	if unit == 2 {
		return []huaweiRow{{37750, named(rowVoltage, "unit_2_bus_voltage")}, {37751, named(rowCurrent, "unit_2_bus_current")},
			{37752, named(rowTemp, "unit_2_pack_temperature")}, {37753, named(rowCharged, "unit_2_charge_energy_total")}, {37755, named(rowDischrg, "unit_2_discharge_energy_total")}}
	}
	return []huaweiRow{{37003, named(rowVoltage, "unit_1_bus_voltage")}, {37004, named(rowSOC, "unit_1_state_of_charge")}, {37021, named(rowCurrent, "unit_1_bus_current")},
		{37022, named(rowTemp, "unit_1_pack_temperature")}, {37066, named(rowCharged, "unit_1_charge_energy_total")}, {37068, named(rowDischrg, "unit_1_discharge_energy_total")},
		{37007, named(rowChgRate, "unit_1_charge_rating")}, {37009, named(rowDisRate, "unit_1_discharge_rating")},
		{37758, named(rowRated, "unit_1_rated_capacity")}}
}

// huaweiPackRows are the measured words of pack n of unit u, counted from the
// pack block origin 38200 + 126(u-1) + 42(n-1): state of charge at +29, voltage
// at +35, current at +36 and the two energy totals at +38 and +40.
func huaweiPackRows(unit, pack int) []huaweiRow {
	origin := uint16(38200 + 126*(unit-1) + 42*(pack-1))
	tag := fmt.Sprintf("unit_%d_pack_%d_", unit, pack)
	return []huaweiRow{{origin + 29, named(rowSOC, tag+"state_of_charge")}, {origin + 35, named(rowVoltage, tag+"voltage")}, {origin + 36, named(rowCurrent, tag+"current")},
		{origin + 38, named(rowCharged, tag+"charge_energy_total")}, {origin + 40, named(rowDischrg, tag+"discharge_energy_total")}}
}

// huaweiStates reduces the running state of a unit to the coarser symbol of the
// element. Offline, fault and a code the card does not name state no symbol.
var huaweiStates = map[string]string{"running": "active", "idle": "idle", "sleep": "standby", "standby": "standby"}

// PublishHuawei publishes the batteries of one provider cycle.
func (p *Publisher) PublishHuawei(ctx context.Context, obs HuaweiObservation) error {
	return p.publish(ctx, func(st stamp) (plan, error) { return p.huaweiPlan(obs) })
}

func decimalOfHuawei(d hw.Decimal) *big.Rat {
	r := new(big.Rat).SetInt64(d.Coefficient)
	switch {
	case d.Exponent > 0:
		r.Mul(r, new(big.Rat).SetInt(pow10(int32(d.Exponent))))
	case d.Exponent < 0:
		r.Quo(r, new(big.Rat).SetInt(pow10(int32(-d.Exponent))))
	}
	return r
}

func (p *Publisher) huaweiPlan(obs HuaweiObservation) (plan, error) {
	pl := plan{profile: huaweiProfile, protocol: huaweiProtocol, nativeKind: huaweiNative, registryKind: huaweiRegistry,
		profileID: huaweiProfile, receipt: obs.Receipt, identity: -1}
	byUnit := map[uint8]HuaweiChild{}
	for _, c := range obs.Children {
		byUnit[c.Unit] = c
	}
	for _, cfg := range p.cfg.Huawei {
		child, ok := byUnit[cfg.Unit]
		if !ok {
			continue
		}
		if err := p.huaweiChild(&pl, cfg, child); err != nil {
			return plan{}, err
		}
	}
	if p.cfg.EMMASite != "" && len(obs.Site) > 0 {
		if err := p.emmaSite(&pl, obs); err != nil {
			return plan{}, err
		}
	}
	return pl, nil
}

func (p *Publisher) battery(pl *plan, slot string) int {
	if slot == "" {
		slot = p.cfg.Battery
	}
	b := pl.node(nodeDecl{kind: kindBattery, slot: slot, parent: -1, operator: true})
	if p.cfg.System != "" {
		pl.link(relSystemBattery, pl.node(nodeDecl{kind: kindSystem, slot: p.cfg.System, parent: -1, operator: true}), b)
	}
	return b
}

func (p *Publisher) packNode(pl *plan, battery int, slot string) int {
	pack := pl.node(nodeDecl{kind: kindPack, slot: slot, parent: battery})
	pl.link(relBatteryPack, battery, pack)
	pl.service(pack, svcPack)
	if pl.identity < 0 {
		pl.identity = pack
	}
	return pack
}

type huaweiFacts map[uint16]hw.SUN2000Fact

func indexFacts(facts []hw.SUN2000Fact) huaweiFacts {
	out := huaweiFacts{}
	for _, f := range facts {
		i := strings.LastIndex(f.Key, "storage.")
		if i < 0 || f.Index != 0 {
			continue
		}
		if n, err := strconv.ParseUint(f.Key[i+len("storage."):], 10, 16); err == nil {
			out[uint16(n)] = f
		}
	}
	return out
}

func (f huaweiFacts) value(offset uint16) (*big.Rat, hw.SUN2000Fact, bool) {
	fact, ok := f[offset]
	if !ok || fact.Availability != hw.FactValue {
		return nil, fact, false
	}
	return decimalOfHuawei(fact.Value), fact, true
}

// present is the zero-placeholder rule: a unit or pack whose measured words
// are all zero, or whose running-state word is zero, is no battery.
func (f huaweiFacts) present(offsets ...uint16) bool {
	for _, o := range offsets {
		if r, _, ok := f.value(o); ok && r.Sign() != 0 {
			return true
		}
	}
	return false
}

func (p *Publisher) huaweiChild(pl *plan, cfg HuaweiBattery, child HuaweiChild) error {
	battery := p.battery(pl, cfg.Battery)
	if !child.BatteryAdmitted {
		pl.members = append(pl.members, memberDecl{battery, fmt.Sprintf("unit_%d_site_only", child.Unit), Native{Source: "battery_groups", Open: reasonNoInverterBattery}})
		return nil
	}
	facts := indexFacts(child.Facts)
	consumed := map[uint16]bool{}
	for unit := 1; unit <= 2; unit++ {
		state := uint16(37000)
		if unit == 2 {
			state = 37741
		}
		sf, hasState := facts[state]
		if !hasState || sf.Availability != hw.FactValue || sf.Value.Coefficient == 0 {
			continue
		}
		node := p.packNode(pl, battery, fmt.Sprintf("unit-%d", unit))
		if !sf.Retained {
			if token, ok := huaweiStates[sf.State]; ok {
				pl.facts = append(pl.facts, factDecl{node, elementOperating, dimPack, symbol(elementOperating, token), Native{Source: "running_state", Unit: "none", Text: sf.State}, lossSymbolCoarse})
			}
		}
		if err := p.huaweiRows(pl, facts, node, huaweiUnitRows(unit)); err != nil {
			return err
		}
		power := uint16(37001)
		if unit == 2 {
			power = 37743
		}
		p.signOpen(pl, node, facts, power, fmt.Sprintf("unit_%d_power", unit))
		consumed[state], consumed[power] = true, true
		for _, row := range huaweiUnitRows(unit) {
			consumed[row.Offset] = true
		}
		for n := 1; n <= 3; n++ {
			rows := huaweiPackRows(unit, n)
			for _, row := range rows {
				consumed[row.Offset] = true
			}
			consumed[uint16(38200+126*(unit-1)+42*(n-1)+33)] = true
			if !facts.present(rows[0].Offset, rows[1].Offset) {
				continue
			}
			packNode := p.packNode(pl, battery, fmt.Sprintf("unit-%d-pack-%d", unit, n))
			if err := p.huaweiRows(pl, facts, packNode, rows); err != nil {
				return err
			}
			origin := uint16(38200 + 126*(unit-1) + 42*(n-1))
			p.signOpen(pl, packNode, facts, origin+33, fmt.Sprintf("unit_%d_pack_%d_power", unit, n))
		}
	}
	p.unmapped(pl, battery, facts, consumed)
	return nil
}

// unmapped keeps every battery word the rows do not take in the namespace,
// under the offset it was read at: the system group, the modes, the
// temperatures and every word the pack has no element for.
func (p *Publisher) unmapped(pl *plan, battery int, facts huaweiFacts, consumed map[uint16]bool) {
	offsets := make([]int, 0, len(facts))
	for offset := range facts {
		if !consumed[offset] {
			offsets = append(offsets, int(offset))
		}
	}
	sort.Ints(offsets)
	for _, offset := range offsets {
		f := facts[uint16(offset)]
		if f.Availability != hw.FactValue {
			continue
		}
		pl.members = append(pl.members, memberDecl{battery, "storage_" + strconv.Itoa(offset),
			Native{Source: f.Key, Unit: f.Unit, Value: decimalOf(decimalOfHuawei(f.Value), 6), Text: f.State}})
	}
}

// signOpen keeps a power word the documentation gives no sign for in the
// namespace; it states no fact.
func (p *Publisher) signOpen(pl *plan, node int, facts huaweiFacts, offset uint16, local string) {
	r, f, ok := facts.value(offset)
	if !ok {
		return
	}
	pl.members = append(pl.members, memberDecl{node, local, Native{Source: f.Key, Unit: f.Unit, Value: decimalOf(r, 6), Open: reasonSignNotStated}})
}

func (p *Publisher) huaweiRows(pl *plan, facts huaweiFacts, node int, rows []huaweiRow) error {
	for _, row := range rows {
		r, f, ok := facts.value(row.Offset)
		if !ok || f.Unit != row.NativeUnit || (row.ZeroAbsent && r.Sign() == 0) {
			continue
		}
		target := node
		if row.Dimension == dimInterface {
			target = pl.node(nodeDecl{kind: kindInterface, slot: "dc", parent: node})
			pl.link(relPackInterface, node, target)
			pl.service(target, svcInterface)
		}
		if f.Retained {
			continue
		}
		if err := pl.fact(target, row.Row, r, strconv.Itoa(int(row.Offset))); err != nil {
			return err
		}
	}
	return nil
}

// emmaSite publishes the site battery of the EMMA unit 0 groups: its state of
// charge, its two cumulative energy totals, and the power the site power
// reading derived from the same battery's charge and discharge counters.
func (p *Publisher) emmaSite(pl *plan, obs HuaweiObservation) error {
	battery := p.battery(pl, p.cfg.EMMASite)
	node := p.packNode(pl, battery, "site")
	type siteRow struct {
		offset uint16
		row    Row
	}
	rows := []siteRow{
		{30368, named(rowSOC, "site_state_of_charge")},
		{30308, named(rowCharged, "site_charge_energy_total")},
		{30314, named(rowDischrg, "site_discharge_energy_total")},
	}
	for _, g := range obs.Site {
		for _, f := range g.Facts {
			native := Native{Source: "site_" + strconv.Itoa(int(f.Offset)), Unit: f.Unit}
			if f.Availability != hw.FactValue {
				continue
			}
			r := siteValue(f)
			native.Value = decimalOf(r, 6)
			pl.members = append(pl.members, memberDecl{node, "site_" + strconv.Itoa(int(f.Offset)), native})
			if g.Retained {
				continue
			}
			for _, row := range rows {
				if row.offset == f.Offset && row.row.NativeUnit == f.Unit {
					if err := pl.fact(node, row.row, r, strconv.Itoa(int(f.Offset))); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, power := range obs.SitePower {
		if power.Offset != 30360 {
			continue
		}
		if !power.Derived {
			pl.unknown = append(pl.unknown, unknownDecl{node, "storage.pack.power", dimPack, power.Reason})
			continue
		}
		w := new(big.Rat).Mul(decimalOfHuawei(power.Value), big.NewRat(1000, 1))
		pl.facts = append(pl.facts, factDecl{node, "storage.pack.power", dimPack, quantity(decimalOf(w, 6), unitWatt),
			Native{Source: "site_30360", Unit: power.Unit, Value: decimalOf(decimalOfHuawei(power.Value), 6),
				Text: fmt.Sprintf("%s;counters=%d,%d;previous=%d,%d;current=%d,%d", power.Direction, power.CounterOffsets[0], power.CounterOffsets[1],
					power.Previous.Totals[0], power.Previous.Totals[1], power.Current.Totals[0], power.Current.Totals[1])}, ""})
	}
	return nil
}

func siteValue(f hw.EMMASiteFact) *big.Rat {
	var n *big.Int
	if f.Type == "I32" {
		n = big.NewInt(int64(f.Signed))
	} else {
		n = new(big.Int).SetUint64(f.Unsigned)
	}
	r := new(big.Rat).SetInt(n)
	switch {
	case f.Exponent > 0:
		r.Mul(r, new(big.Rat).SetInt(pow10(int32(f.Exponent))))
	case f.Exponent < 0:
		r.Quo(r, new(big.Rat).SetInt(pow10(int32(-f.Exponent))))
	}
	return r
}
