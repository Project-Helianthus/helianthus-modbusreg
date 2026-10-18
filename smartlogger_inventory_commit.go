package modbusreg

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrSmartloggerInventoryBusy    = errors.New("SmartLogger inventory attempt busy")
	ErrSmartloggerInventoryClosed  = errors.New("SmartLogger inventory owner closed")
	ErrSmartloggerInventoryContext = errors.New("invalid SmartLogger inventory context or deadline")
)

// SmartloggerInventoryIdentity is caller-supplied correlation, not identity proof
// or profile admission. RoutingContext must be an opaque local identifier.
type SmartloggerInventoryIdentity struct {
	RoutingContext string
	UnitID         uint8
	ParentProfile  string
	SourceRevision string
	SelfModel      string
	Firmware       string
}

type SmartloggerInventoryContext struct {
	OwnerToken           string
	ConnectionGeneration uint64
	Identity             SmartloggerInventoryIdentity
}

type SmartloggerInventoryPlan struct {
	Context      SmartloggerInventoryContext
	AttemptToken uint64
	Deadline     time.Time
}

type SmartloggerInventoryOutcome string

const (
	SmartloggerInventoryComplete  SmartloggerInventoryOutcome = "complete"
	SmartloggerInventoryFailed    SmartloggerInventoryOutcome = "failed"
	SmartloggerInventoryCancelled SmartloggerInventoryOutcome = "cancelled"
	SmartloggerInventoryClosed    SmartloggerInventoryOutcome = "closed"
	SmartloggerInventoryRetired   SmartloggerInventoryOutcome = "retired"
)

// SmartloggerInventoryResult is returned by value; terminal outcomes cannot be
// rewritten by later callbacks. It carries no admission or qualification grant.
type SmartloggerInventoryResult struct {
	Plan        SmartloggerInventoryPlan
	Outcome     SmartloggerInventoryOutcome
	CompletedAt time.Time
}

func (SmartloggerInventoryResult) DefaultDenied() bool { return true }

// SmartloggerInventorySnapshot preserves native history. Current only identifies
// matching local context; it does not establish live identity or admission.
type SmartloggerInventorySnapshot struct {
	Plan        SmartloggerInventoryPlan
	ObservedAt  time.Time
	CompletedAt time.Time
	Inventory   HuaweiSmartLoggerInventory
	Native      HuaweiSmartLoggerOfflineInventoryInput
	Current     bool
}

// SmartloggerInventoryCommitOwner owns offline publication only. Do not copy it.
// Its zero value refuses attempts. No transport or scheduler is invoked.
type SmartloggerInventoryCommitOwner struct {
	mu      sync.Mutex
	context SmartloggerInventoryContext
	now     func() time.Time
	closed  bool
	next    uint64
	active  *SmartloggerInventoryAttempt
	last    *SmartloggerInventorySnapshot
}

// SmartloggerInventoryAttempt is an opaque handle to one frozen plan. Do not
// copy it. The owner serializes all its terminal transitions.
type SmartloggerInventoryAttempt struct {
	owner   *SmartloggerInventoryCommitOwner
	plan    SmartloggerInventoryPlan
	started time.Time
	result  SmartloggerInventoryResult
}

// NewSmartloggerInventoryCommitOwner creates explicitly invoked offline
// mechanics. The clock must be monotonic, nonblocking, and safe for this owner;
// nil uses time.Now. This constructor cannot admit a production profile.
func NewSmartloggerInventoryCommitOwner(context SmartloggerInventoryContext, clock func() time.Time) (*SmartloggerInventoryCommitOwner, error) {
	if !validSmartloggerInventoryContext(context) {
		return nil, ErrSmartloggerInventoryContext
	}
	if clock == nil {
		clock = time.Now
	}
	return &SmartloggerInventoryCommitOwner{context: context, now: clock}, nil
}

func validSmartloggerInventoryContext(c SmartloggerInventoryContext) bool {
	i := c.Identity
	return c.OwnerToken != "" && c.ConnectionGeneration != 0 && i.RoutingContext != "" && i.UnitID == 0 &&
		i.ParentProfile != "" && i.SourceRevision != "" && i.SelfModel == "SmartLogger" &&
		(i.Firmware == "V300R024C10SPC191" || i.Firmware == "V300R024C10SPC210")
}

func (o *SmartloggerInventoryCommitOwner) Begin(deadline time.Time) (*SmartloggerInventoryAttempt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, ErrSmartloggerInventoryClosed
	}
	if o.active != nil {
		return nil, ErrSmartloggerInventoryBusy
	}
	if o.now == nil || !validSmartloggerInventoryContext(o.context) || o.next == ^uint64(0) {
		return nil, ErrSmartloggerInventoryContext
	}
	now := o.now()
	if !deadline.After(now) || deadline.After(now.Add(15*time.Second)) {
		return nil, ErrSmartloggerInventoryContext
	}
	o.next++
	a := &SmartloggerInventoryAttempt{owner: o, started: now,
		plan: SmartloggerInventoryPlan{Context: o.context, AttemptToken: o.next, Deadline: deadline}}
	o.active = a
	return a, nil
}

func (a *SmartloggerInventoryAttempt) Plan() SmartloggerInventoryPlan { return a.plan }

// Commit consumes already-collected pages. The caller must not mutate input
// concurrently with this call. Publication occurs only after complete native
// validation and a final deadline check while retirement is excluded.
func (a *SmartloggerInventoryAttempt) Commit(plan SmartloggerInventoryPlan, input HuaweiSmartLoggerOfflineInventoryInput) SmartloggerInventoryResult {
	o := a.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	if a.result.Outcome != "" {
		return a.result
	}
	if o.closed || o.active != a || plan != a.plan || o.context != a.plan.Context || !o.now().Before(a.plan.Deadline) {
		return a.finishLocked(SmartloggerInventoryFailed)
	}
	inventory, err := ParseHuaweiSmartLoggerOfflineInventory(input)
	if err != nil {
		return a.finishLocked(SmartloggerInventoryFailed)
	}
	// The parser deliberately omits the self record from its child view. Check
	// its exact firmware independently rather than inferring it from children.
	selfValid := false
	for _, page := range input.Pages {
		for _, object := range page.Objects {
			if object.ObjectID == 0x88 {
				self, err := parseHuaweiSmartLoggerInventoryChild(object)
				selfValid = err == nil && self.model == a.plan.Context.Identity.SelfModel && self.softwareVersion == a.plan.Context.Identity.Firmware
			}
		}
	}
	if !selfValid {
		return a.finishLocked(SmartloggerInventoryFailed)
	}
	native := cloneSmartloggerInventoryNative(input)
	now := o.now()
	if !now.Before(a.plan.Deadline) || now.Before(a.started) {
		return a.finishLocked(SmartloggerInventoryFailed)
	}
	o.last = &SmartloggerInventorySnapshot{Plan: a.plan, ObservedAt: a.started, CompletedAt: now, Inventory: inventory, Native: native}
	return a.finishAtLocked(SmartloggerInventoryComplete, now)
}

func (a *SmartloggerInventoryAttempt) finishAtLocked(outcome SmartloggerInventoryOutcome, now time.Time) SmartloggerInventoryResult {
	if a.result.Outcome == "" {
		a.result = SmartloggerInventoryResult{Plan: a.plan, Outcome: outcome, CompletedAt: now}
		if a.owner.active == a {
			a.owner.active = nil
		}
	}
	return a.result
}

func (a *SmartloggerInventoryAttempt) finishLocked(outcome SmartloggerInventoryOutcome) SmartloggerInventoryResult {
	return a.finishAtLocked(outcome, a.owner.now())
}

func (a *SmartloggerInventoryAttempt) Cancel() SmartloggerInventoryResult {
	a.owner.mu.Lock()
	defer a.owner.mu.Unlock()
	if a.result.Outcome != "" {
		return a.result
	}
	return a.finishLocked(SmartloggerInventoryCancelled)
}

func (o *SmartloggerInventoryCommitOwner) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = true
	if o.active != nil {
		o.active.finishLocked(SmartloggerInventoryClosed)
	}
}

// Retire requires a strictly newer generation, preventing old context reuse.
// Retained history is never relabeled as evidence for the new connection.
func (o *SmartloggerInventoryCommitOwner) Retire(context SmartloggerInventoryContext) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrSmartloggerInventoryClosed
	}
	if o.now == nil || !validSmartloggerInventoryContext(context) || context.ConnectionGeneration <= o.context.ConnectionGeneration {
		return ErrSmartloggerInventoryContext
	}
	if o.active != nil {
		o.active.finishLocked(SmartloggerInventoryRetired)
	}
	o.context = context
	return nil
}

func (o *SmartloggerInventoryCommitOwner) LastComplete() (SmartloggerInventorySnapshot, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.last == nil {
		return SmartloggerInventorySnapshot{}, false
	}
	s := *o.last
	s.Inventory.children = append([]HuaweiSmartLoggerInventoryChild(nil), s.Inventory.children...)
	s.Native = cloneSmartloggerInventoryNative(s.Native)
	s.Current = !o.closed && s.Plan.Context == o.context
	return s, true
}

func cloneSmartloggerInventoryNative(input HuaweiSmartLoggerOfflineInventoryInput) HuaweiSmartLoggerOfflineInventoryInput {
	copy := input
	copy.Pages = make([]HuaweiSmartLoggerInventoryPage, len(input.Pages))
	for p, page := range input.Pages {
		copy.Pages[p] = page
		copy.Pages[p].Objects = make([]HuaweiSmartLoggerInventoryObject, len(page.Objects))
		for i, object := range page.Objects {
			copy.Pages[p].Objects[i] = object
			copy.Pages[p].Objects[i].Value = append([]byte(nil), object.Value...)
		}
	}
	return copy
}
