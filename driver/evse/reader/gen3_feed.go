package reader

import (
	"sync"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

// Role names the subject an element is published on.
type Role string

// The subjects of the readback facts.
const (
	RoleEVSE      Role = "evse"
	RoleConnector Role = "connector"
)

// The elements fed from the readback.
const (
	ElementConfiguredCurrent = "evse.limit.configured_current"
	ElementAllocatedCurrent  = "evse.limit.allocated_current"
	ElementAllocatedExpiry   = "evse.limit.allocated_current_expiry"
	ElementSupply            = "evse.status.supply"
)

// SymbolSupplyDisabled is the supply symbol an inhibit-true readback publishes.
const SymbolSupplyDisabled = "evse.status.supply.disabled"

// The dispositions a withheld or unresolved element carries.
const (
	ReasonProvisionalExpired = "withheld_provisional_expired"
	ReasonProvisionalMissing = "withheld_provisional_missing"
	ReasonZeroCurrent        = "unresolved_zero_current"
	ReasonZeroTimeout        = "unresolved_zero_timeout"
)

// Receipt holds the native receipt axes of one acknowledged record. Wall and
// Mono are the record's own receipt; they are never re-read from a clock.
type Receipt struct {
	ClockEpoch string
	Wall       time.Time
	Mono       time.Duration
	Lifecycle  uint64
	Instance   uint64
	Generation uint64
	RequestID  uint64
}

// After reports whether r is strictly later than other. Receipts of a newer
// lifecycle or generation are later; inside one, the monotonic receipt orders
// them when both use one clock epoch and the wall receipt otherwise.
func (r Receipt) After(other Receipt) bool {
	if r.Lifecycle != other.Lifecycle {
		return r.Lifecycle > other.Lifecycle
	}
	if r.Instance != other.Instance {
		return r.Instance > other.Instance
	}
	if r.Generation != other.Generation {
		return r.Generation > other.Generation
	}
	if r.ClockEpoch == other.ClockEpoch && r.ClockEpoch != "" {
		return r.Mono > other.Mono
	}
	return r.Wall.After(other.Wall)
}

func (r Receipt) obsolete(newest Receipt) bool {
	if r.Lifecycle != newest.Lifecycle {
		return r.Lifecycle < newest.Lifecycle
	}
	if r.Instance != newest.Instance {
		return r.Instance < newest.Instance
	}
	return r.Generation < newest.Generation
}

// At is the evaluation instant of a view.
type At struct {
	ClockEpoch string
	Wall       time.Time
	Mono       time.Duration
}

func (at At) elapsedSince(anchor Receipt) time.Duration {
	if at.ClockEpoch == anchor.ClockEpoch && at.ClockEpoch != "" {
		return at.Mono - anchor.Mono
	}
	return at.Wall.Sub(anchor.Wall)
}

// ProvisionalObservation is one acknowledged t28 with the receipt that anchors
// its timeout. A polled or prerequisite t28 anchors on its own read receipt; a
// mutation's t28 anchors on the correlated set acknowledgement.
type ProvisionalObservation struct {
	Readback tesla.ProvisionalCurrentLimitReadback
	Receipt  Receipt
	Anchor   Receipt
}

// PersistentObservation is one acknowledged t8 returned current.
type PersistentObservation struct {
	MaxOutputCurrentAmps uint32
	Receipt              Receipt
}

// Fact is one published value of the view.
type Fact struct {
	Element string
	Role    Role
	Amps    uint32
	Time    time.Time
	Symbol  string
	Receipt Receipt
	// Reason is set on a fact that carries no value: the element is withheld
	// or unresolved and the reason is what the publication states.
	Reason string
}

// Disposition names why an element has no value in the view.
type Disposition struct {
	Element string
	Reason  string
	Role    Role
	Receipt Receipt
}

// View is the publishable state at one instant; an element missing from Facts
// is unknown, with its Disposition where one applies.
type View struct {
	Facts        []Fact
	Dispositions []Disposition
}

// Fact returns the published fact of an element.
func (v View) Fact(element string) (Fact, bool) {
	for _, f := range v.Facts {
		if f.Element == element {
			return f, true
		}
	}
	return Fact{}, false
}

// Reason returns the disposition reason of an element, empty when none.
func (v View) Reason(element string) string {
	for _, d := range v.Dispositions {
		if d.Element == element {
			return d.Reason
		}
	}
	return ""
}

type amps struct {
	value   uint32
	receipt Receipt
}

type allocation struct {
	value    uint32
	known    bool
	receipt  Receipt
	inhibit  bool
	observed bool
}

type expiry struct {
	anchor  Receipt
	timeout uint32
	set     bool
}

type obsKey struct {
	persistent bool
	receipt    [4]uint64
}

// Feed turns acknowledged readback records into the elements of the table in
// the package documentation. It is safe for concurrent use.
type Feed struct {
	mu                 sync.Mutex
	seen               map[obsKey]struct{}
	newest             Receipt
	haveNew            bool
	persistent         *amps
	readbackConfigured *amps
	alloc              allocation
	exp                expiry
	zeroTO             bool
	zeroTORec          Receipt
	supply             bool
	supplyRec          Receipt
	published          bool
	withdrawn          bool
	lastProv           *ProvisionalObservation
}

// NewFeed returns an empty feed.
func NewFeed() *Feed { return &Feed{seen: map[obsKey]struct{}{}} }

// accept reports whether an observation with this receipt is taken: a replay
// of a receipt already taken, or one of an older lifecycle or generation than
// the newest taken, has no effect.
func (f *Feed) accept(persistent bool, r Receipt) bool {
	key := obsKey{persistent, [4]uint64{r.Lifecycle, r.Instance, r.Generation, r.RequestID}}
	if _, dup := f.seen[key]; dup {
		return false
	}
	if f.haveNew && r.obsolete(f.newest) {
		return false
	}
	f.seen[key] = struct{}{}
	f.newest, f.haveNew = r, true
	return true
}

// Persistent feeds a t8 returned current. A duplicate or an obsolete
// generation changes nothing and reports false.
func (f *Feed) Persistent(o PersistentObservation) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.accept(true, o.Receipt) {
		return false
	}
	f.persistent = &amps{o.MaxOutputCurrentAmps, o.Receipt}
	if f.published {
		f.withdrawn = true
	}
	return true
}

// Provisional feeds one t28. Every scalar is fed from its own presence: an
// absent scalar leaves its element as it was and nothing is made up from it.
func (f *Feed) Provisional(o ProvisionalObservation) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.accept(false, o.Receipt) {
		return false
	}
	rb := o.Readback
	f.withdrawn = false
	retained := o
	f.lastProv = &retained
	if rb.HasConfiguredCurrent {
		f.readbackConfigured = &amps{rb.ConfiguredCurrentAmps, o.Receipt}
	}
	if rb.HasInhibit {
		f.supply, f.supplyRec = rb.InhibitCharging, o.Anchor
		switch {
		case rb.InhibitCharging:
			f.alloc = allocation{value: 0, known: true, receipt: o.Anchor, inhibit: true, observed: true}
		case rb.HasLimitCurrent:
			f.alloc = allocation{value: rb.LimitCurrentMaxAmps, known: rb.LimitCurrentMaxAmps != 0, receipt: o.Anchor, observed: true}
		}
	}
	if rb.HasLimitTimeout {
		f.zeroTO, f.zeroTORec = rb.LimitTimeoutSeconds == 0, o.Receipt
		if f.zeroTO {
			f.exp = expiry{}
		} else {
			f.exp = expiry{anchor: o.Anchor, timeout: rb.LimitTimeoutSeconds, set: true}
		}
	}
	return true
}

// LastProvisional returns the t28 observation last taken, with its receipts
// exactly as they arrived; a persistent-only publication does not change it.
func (f *Feed) LastProvisional() (ProvisionalObservation, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastProv == nil {
		return ProvisionalObservation{}, false
	}
	return *f.lastProv, true
}

// Published marks that a view was published, so a later persistent-only
// publication withdraws the provisional allocation.
func (f *Feed) Published() {
	f.mu.Lock()
	f.published = true
	f.mu.Unlock()
}

// View evaluates the state at one instant.
func (f *Feed) View(at At) View {
	f.mu.Lock()
	defer f.mu.Unlock()
	var v View
	cfg, zero := f.configured()
	switch {
	case zero:
		v.Dispositions = append(v.Dispositions, Disposition{ElementConfiguredCurrent, ReasonZeroCurrent, RoleEVSE, f.readbackConfigured.receipt})
	case cfg != nil:
		v.Facts = append(v.Facts, Fact{Element: ElementConfiguredCurrent, Role: RoleEVSE, Amps: cfg.value, Receipt: cfg.receipt})
	}
	if f.withdrawn {
		receipt := f.alloc.receipt
		if !f.alloc.observed && f.persistent != nil {
			receipt = f.persistent.receipt
		}
		v.Dispositions = append(v.Dispositions, Disposition{ElementAllocatedCurrent, ReasonProvisionalMissing, RoleConnector, receipt})
	} else if f.alloc.observed {
		switch {
		case !f.alloc.known:
			v.Dispositions = append(v.Dispositions, Disposition{ElementAllocatedCurrent, ReasonZeroCurrent, RoleConnector, f.alloc.receipt})
		case f.exp.set && at.elapsedSince(f.exp.anchor) >= time.Duration(f.exp.timeout)*time.Second:
			v.Dispositions = append(v.Dispositions, Disposition{ElementAllocatedCurrent, ReasonProvisionalExpired, RoleConnector, f.alloc.receipt})
		default:
			v.Facts = append(v.Facts, Fact{Element: ElementAllocatedCurrent, Role: RoleConnector, Amps: f.alloc.value, Receipt: f.alloc.receipt})
		}
	}
	if f.exp.set {
		v.Facts = append(v.Facts, Fact{Element: ElementAllocatedExpiry, Role: RoleConnector,
			Time: f.exp.anchor.Wall.Add(time.Duration(f.exp.timeout) * time.Second).UTC(), Receipt: f.exp.anchor})
	} else if f.zeroTO {
		v.Dispositions = append(v.Dispositions, Disposition{ElementAllocatedExpiry, ReasonZeroTimeout, RoleConnector, f.zeroTORec})
	}
	if f.supply {
		v.Facts = append(v.Facts, Fact{Element: ElementSupply, Role: RoleEVSE, Symbol: SymbolSupplyDisabled, Receipt: f.supplyRec})
	}
	return v
}

// configured picks the later receipt between t8 and a t28 that carries the
// configured current; an explicit zero is unresolved, never a number.
func (f *Feed) configured() (candidate *amps, zero bool) {
	switch {
	case f.persistent == nil && f.readbackConfigured == nil:
		return nil, false
	case f.readbackConfigured == nil:
		return f.persistent, false
	case f.persistent == nil || f.readbackConfigured.receipt.After(f.persistent.receipt):
		return f.readbackConfigured, f.readbackConfigured.value == 0
	}
	return f.persistent, false
}
