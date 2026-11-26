package storage

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"github.com/Project-Helianthus/helianthus-modbusreg/growatt"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

const (
	growattProfile   = "growatt.bms.rs485.1xsxxp.v2_02.readonly.v1"
	growattProtocol  = "modbus_rtu"
	growattNative    = "native.growatt.bms.rs485.v202.observation"
	growattRegistry  = "registry.growatt.bms.rs485.v202.mapping"
	growattNativeKey = "native.growatt.bms.rs485.v202"
)

// GrowattObservation is one complete base observation of the RS-485 family
// with its receipt. Diagnostic is the multi-group diagnostic read after the
// base observation; it is nil when it was not read or did not qualify, and a
// nil diagnostic leaves the base publication unchanged.
type GrowattObservation struct {
	Status     growatt.BMSTypedReadOnlyStatus
	Diagnostic *growatt.BMSMultiGroupDiagnostic
	Receipt    Receipt
}

// PublishGrowatt publishes one observation of the RS-485 family.
func (p *Publisher) PublishGrowatt(ctx context.Context, obs GrowattObservation) error {
	return p.publish(ctx, func(st stamp) (plan, error) { return p.growattPlan(obs) })
}

func nativeOf(source, unit string, r *big.Rat) Native {
	return Native{Source: source, Unit: unit, Value: decimalOf(r, 6)}
}

func (p *Publisher) growattPlan(obs GrowattObservation) (plan, error) {
	s := obs.Status
	pl := plan{profile: growattProfile, protocol: growattProtocol, nativeKind: growattNative, registryKind: growattRegistry,
		profileID: growattProfile, receipt: obs.Receipt, legacyIDs: true}
	pack := pl.node(nodeDecl{kind: kindPack, parent: -1, legacy: true, operator: true})
	pl.identity = pack
	pl.service(pack, svcPack)
	p.declare(&pl, pack)

	values := map[string]float64{
		"pack_voltage_volts": s.PackVoltageVolts, "pack_current_amps": s.PackCurrentAmps, "soc_percent": float64(s.SOCPercent),
		"temperature_celsius": float64(s.TemperatureCelsius), "cumulative_charge_amp_hours": s.CumulativeChargeAmpHours,
		"cumulative_discharge_amp_hours": s.CumulativeDischargeAmpHours, "remaining_capacity_amp_hours": s.RemainingCapacityAmpHours,
		"full_charge_capacity_amp_hours": s.FullChargeCapacityAmpHours, "cycle_count": float64(s.CycleCount),
	}
	for _, row := range growattRows {
		r, err := floatRat(values[row.Native])
		if err != nil {
			return plan{}, err
		}
		if err := pl.fact(pack, row, r, ""); err != nil {
			return plan{}, err
		}
	}

	state, ok := growattStates[string(s.OperatingState)]
	if !ok {
		pl.members = append(pl.members, memberDecl{pack, "operating_state", Native{Source: "operating_state", Text: string(s.OperatingState)}})
	} else {
		native := Native{Source: "operating_state", Unit: "none", Text: string(s.OperatingState)}
		if state.Coarse != "" {
			pl.facts = append(pl.facts, factDecl{pack, elementOperating, dimPack, symbol(elementOperating, state.Coarse), native, lossSymbolCoarse})
		} else {
			pl.withdrawals = append(pl.withdrawals, withdrawal{pack, elementOperating, dimPack})
		}
		if state.Direction != "" {
			pl.facts = append(pl.facts, factDecl{pack, elementDirection, dimPack, symbol(elementDirection, state.Direction), native, ""})
		} else {
			pl.withdrawals = append(pl.withdrawals, withdrawal{pack, elementDirection, dimPack})
		}
	}

	for _, m := range []struct {
		local, unit string
		v           float64
	}{
		{"average_cell_voltage_volts", "V", s.AverageCellVoltageVolts}, {"floating_pack_voltage_volts", "V", s.FloatingPackVoltageVolts},
		{"continuous_charge_seconds", "s", float64(s.ContinuousChargeSeconds)}, {"current_cycle_charge_amp_hours", "Ah", s.CurrentCycleChargeAmpHours},
	} {
		r, err := floatRat(m.v)
		if err != nil {
			return plan{}, err
		}
		pl.members = append(pl.members, memberDecl{pack, m.local, nativeOf(m.local, m.unit, r)})
	}

	if obs.Diagnostic != nil {
		if err := p.growattDiagnostic(&pl, pack, *obs.Diagnostic); err != nil {
			return plan{}, err
		}
	}
	return pl, nil
}

// declare links the pack to its configured battery and the battery to its
// configured system. A relation with no configured end stays unknown.
func (p *Publisher) declare(pl *plan, pack int) {
	if p.cfg.Battery == "" {
		return
	}
	battery := pl.node(nodeDecl{kind: kindBattery, slot: p.cfg.Battery, parent: -1, operator: true})
	pl.link(relBatteryPack, battery, pack)
	if p.cfg.System != "" {
		system := pl.node(nodeDecl{kind: kindSystem, slot: p.cfg.System, parent: -1, operator: true})
		pl.link(relSystemBattery, system, battery)
	}
}

// fact converts one native value through its row and declares the fact.
func (pl *plan) fact(node int, row Row, native *big.Rat, text string) error {
	v, err := row.Convert(native)
	if err != nil {
		return err
	}
	n := nativeOf(row.Native, row.NativeUnit, native)
	n.Text = text
	pl.facts = append(pl.facts, factDecl{node, row.Element, row.Dimension, v, n, row.Loss})
	return nil
}

// growattDiagnostic publishes the minimum and maximum of the valid cell
// voltages, one cell each, with the cell index as evidence. A zero placeholder
// word is not a voltage.
func (p *Publisher) growattDiagnostic(pl *plan, pack int, d growatt.BMSMultiGroupDiagnostic) error {
	type cell struct {
		index int
		mv    uint16
	}
	var valid []cell
	for g, group := range d.CellVoltageGroups {
		for i, mv := range group.Millivolts {
			index := g*len(group.Millivolts) + i + 1
			if mv == 0 {
				continue
			}
			valid = append(valid, cell{index, mv})
			pl.members = append(pl.members, memberDecl{pack, "cell_voltage_millivolts_" + strconv.Itoa(index),
				Native{Source: "cell_voltage", Unit: "mV", Value: semreg.Decimal{Coefficient: strconv.Itoa(int(mv))}}})
		}
	}
	for i, state := range d.NativeBalanceStates {
		on := "off"
		if state {
			on = "on"
		}
		pl.members = append(pl.members, memberDecl{pack, "cell_balance_state_" + strconv.Itoa(i+1), Native{Source: "cell_balance_state", Text: on}})
	}
	pl.supersede = append(pl.supersede, growattCellRows[0].Element, growattCellRows[1].Element)
	if len(valid) == 0 {
		return nil
	}
	min, max := valid[0], valid[0]
	for _, c := range valid[1:] {
		if c.mv < min.mv {
			min = c
		}
		if c.mv > max.mv {
			max = c
		}
	}
	for i, c := range []cell{min, max} {
		node := pl.node(nodeDecl{kind: kindCell, slot: "cell-" + strconv.Itoa(c.index), parent: pack})
		if err := pl.fact(node, growattCellRows[i], new(big.Rat).SetInt64(int64(c.mv)), fmt.Sprintf("cell_index=%d", c.index)); err != nil {
			return err
		}
	}
	return nil
}
