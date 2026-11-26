package meter

import (
	"math/big"
	"strings"

	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// feeder is a configured meter whose readings are being published.
type feeder struct {
	r    *run
	slot string
	node driver.NodeRef
	grid *driver.NodeRef
}

// meter publishes the resource of a configured meter and, for the role
// grid_connection, its relation. grid is nil unless the meter is bound to a
// declared grid connection.
func (r *run) meter(slot string, role Role, gcSlot string) (*feeder, error) {
	node, err := r.node(kindMeter, slot, nil)
	if err != nil {
		return nil, err
	}
	f := &feeder{r: r, slot: slot, node: node}
	if role != RoleGridConnection {
		return f, nil
	}
	gc, ok, err := r.gridConnection(gcSlot)
	if err != nil || !ok {
		return f, err
	}
	f.grid = &gc
	r.link("infrastructure.relation.grid_connection_meter", gc, node, gcSlot, slot, string(role))
	return f, nil
}

func (f *feeder) member(rd reading, d semreg.Decimal) {
	f.r.member(f.slot, rd.field, rd.native(d))
}

// feed publishes one reading: the native value always, the element of the table
// when there is one.
func (f *feeder) feed(rd reading) error {
	d, err := decimalOf(rd.value)
	if err != nil {
		return err
	}
	f.member(rd, d)
	if rd.kind == "power.active.total" {
		return f.activePower(rd)
	}
	if rd.kind == "power.reactive.total" {
		return f.reactivePower(rd)
	}
	row, ok := rows[rd.kind]
	if !ok {
		return nil
	}
	conv, ok := row.units[rd.unit]
	if !ok {
		return nil
	}
	target, dim := f.node, string(f.node.ID)
	switch row.scope {
	case onGrid:
		if f.grid == nil {
			return nil
		}
		target, dim = *f.grid, string(f.grid.ID)
	case onPhase:
		dim = string(f.node.ID) + ".phase_" + row.suffix
	case onLinePair:
		dim = string(f.node.ID) + ".line_" + row.suffix
	}
	return f.publish(target, row.el, dim, rd, conv)
}

func (f *feeder) publish(target driver.NodeRef, el element, dim string, rd reading, conv factor) error {
	v, err := quantity(conv.apply(rd.value), el.unit)
	if err != nil {
		return err
	}
	d, _ := decimalOf(rd.value)
	_, err = f.r.emit(target, el, dim, v, source{path: joinPath(f.slot, rd.path), native: rd.native(d), retained: rd.retained})
	return err
}

func (f *feeder) activePower(rd reading) error {
	conv, ok := activeTotal.units[rd.unit]
	if !ok || f.grid == nil {
		return nil
	}
	gc := *f.grid
	if err := f.publish(gc, activeTotal.el, string(gc.ID), rd, conv); err != nil {
		return err
	}
	return f.split(gc, rd, conv, importAct, exportAct, rd.polarity)
}

func (f *feeder) reactivePower(rd reading) error {
	conv, ok := map[string]factor{"var": one}[rd.unit]
	if !ok || f.grid == nil {
		return nil
	}
	return f.split(*f.grid, rd, conv, importReact, exportReact, rd.polarity)
}

// split publishes the two directions of a signed value, or records that the
// sign is open and withholds both.
func (f *feeder) split(gc driver.NodeRef, rd reading, conv factor, in, out element, p polarity) error {
	if p == signOpen {
		f.r.unknown(gc, in, string(gc.ID), openSign)
		f.r.unknown(gc, out, string(gc.ID), openSign)
		return nil
	}
	positive, negative := in, out
	if p == signExportPositive {
		positive, negative = out, in
	}
	value := conv.apply(rd.value)
	zero := new(big.Rat)
	pos, neg := new(big.Rat), new(big.Rat)
	switch value.Cmp(zero) {
	case 1:
		pos = value
	case -1:
		neg = new(big.Rat).Neg(value)
	}
	for _, side := range []struct {
		el element
		v  *big.Rat
	}{{positive, pos}, {negative, neg}} {
		v, err := quantity(side.v, side.el.unit)
		if err != nil {
			return err
		}
		d, _ := decimalOf(rd.value)
		if _, err := f.r.emit(gc, side.el, string(gc.ID), v, source{path: joinPath(f.slot, rd.path, string(side.el.id)), native: rd.native(d), retained: rd.retained}); err != nil {
			return err
		}
	}
	return nil
}

func (f *feeder) fault(present bool, rd reading) error {
	token := "clear"
	if present {
		token = "present"
	}
	if f.r.p.cfg.Site.Slot == "" {
		return nil
	}
	site, err := f.r.node(kindSite, f.r.p.cfg.Site.Slot, nil)
	if err != nil {
		return err
	}
	v := semreg.Value{Kind: semreg.ValueSymbol, Symbol: &semreg.Symbol{Namespace: faultEl.id, Token: token, Known: true}}
	d, err := decimalOf(rd.value)
	if err != nil {
		return err
	}
	_, err = f.r.emit(site, faultEl, string(site.ID), v, source{path: joinPath(f.slot, rd.path), native: rd.native(d), retained: rd.retained})
	return err
}

func joinPath(parts ...string) string { return strings.Join(parts, ".") }

func bitsRat(bits uint64) *big.Rat { return new(big.Rat).SetInt(new(big.Int).SetUint64(bits)) }
