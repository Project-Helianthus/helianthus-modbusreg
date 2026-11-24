package huawei

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// The provider core and the cycle runner of the Huawei read-only production
// providers: the session binding, the refresh and every paced wire exchange
// of one cycle.

// huaweiLinkState is the pacing state of the bound connection generation.
type huaweiLinkState struct {
	lastEnd time.Time
	sent    bool
}

type huaweiCycleRunner func(*huaweiCycleRun)

type huaweiProviderCore struct {
	path       ProviderPath
	providerID string
	source     string
	allowed    []uint8
	pacing     ProviderPacing
	clock      func() time.Time
	wait       ProviderWait
	enablement ProviderEnablement
	run        huaweiCycleRunner
	staticLoss []ProviderLoss

	mu         sync.Mutex
	session    ProviderSession
	generation uint64
	started    bool
	closed     bool
	busy       bool
	closing    bool
	link       huaweiLinkState
	cycle      uint64
	nextAt     time.Time
	state      *huaweiProviderState
	last       *ProviderCycle
}

func newHuaweiProviderCore(path ProviderPath, providerID string, config ProviderConfig, disposition ProviderDisposition, run huaweiCycleRunner, loss []ProviderLoss) (*huaweiProviderCore, error) {
	enablement := EvaluateHuaweiProviderEnablement(disposition.clone())
	if !enablement.Admitted || disposition.Path != path {
		return nil, ErrHuaweiProviderNotAdmitted
	}
	pacing, ok := ProviderPacingFor(path)
	if !ok || config.Wait == nil || !validHuaweiProviderSource(config.Source) {
		return nil, ErrHuaweiProviderConfig
	}
	allowed := append([]uint8(nil), config.AllowedUnits...)
	sort.Slice(allowed, func(i, j int) bool { return allowed[i] < allowed[j] })
	for index, unit := range allowed {
		if unit == 0 || unit > huaweiSUN2000MaxUnit || (index > 0 && allowed[index-1] == unit) {
			return nil, ErrHuaweiProviderConfig
		}
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &huaweiProviderCore{
		path: path, providerID: providerID, source: config.Source, allowed: allowed, pacing: pacing, clock: clock,
		wait: config.Wait, enablement: enablement, run: run, staticLoss: append([]ProviderLoss(nil), loss...),
		state: &huaweiProviderState{children: map[uint8]*huaweiChildState{}, gateway: ProviderGatewayIdentity{State: ProviderGatewayNotIdentified}},
	}, nil
}

func (p *huaweiProviderCore) start(session ProviderSession) error {
	if p == nil || session == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		return ErrHuaweiProviderClosing
	}
	if p.started {
		return ErrHuaweiProviderStarted
	}
	p.bindLocked(session)
	return nil
}

// bindLocked starts a new connection generation. The gateway identity must be
// read again on it before any child reading is taken.
func (p *huaweiProviderCore) bindLocked(session ProviderSession) {
	p.session, p.started = session, true
	p.generation++
	p.link = huaweiLinkState{}
	p.state.identified = false
	p.state.reidentify = false
}

func (p *huaweiProviderCore) stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		p.mu.Unlock()
		return ErrHuaweiProviderClosing
	}
	if !p.started {
		p.mu.Unlock()
		return ErrHuaweiProviderNotStarted
	}
	session := p.unbindLocked()
	p.closing = true
	p.mu.Unlock()
	err := closeHuaweiProviderSession(session)
	p.mu.Lock()
	p.closing = false
	p.mu.Unlock()
	return err
}

// unbindLocked detaches the current connection and fences any running cycle.
// The caller closes the returned session after releasing the lock, so a Close
// that calls back into the provider cannot deadlock.
func (p *huaweiProviderCore) unbindLocked() ProviderSession {
	session := p.session
	p.session, p.started = nil, false
	p.generation++
	p.state.identified = false
	return session
}

func closeHuaweiProviderSession(session ProviderSession) error {
	if session == nil {
		return nil
	}
	return session.Close()
}

// reconnect closes the old session before the new one is bound: while the old
// Close runs the provider is stopped, so no cycle can read through the new
// session until the old connection is released. The new session is bound
// whether or not the old Close failed; that error is returned.
func (p *huaweiProviderCore) reconnect(session ProviderSession) error {
	if p == nil || session == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrHuaweiProviderClosed
	}
	if p.closing {
		p.mu.Unlock()
		return ErrHuaweiProviderClosing
	}
	var old ProviderSession
	if p.started {
		old = p.unbindLocked()
	}
	p.closing = true
	p.mu.Unlock()
	closeErr := closeHuaweiProviderSession(old)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closing = false
	if p.closed {
		return ErrHuaweiProviderClosed
	}
	if p.started {
		return ErrHuaweiProviderStarted
	}
	p.bindLocked(session)
	return closeErr
}

func (p *huaweiProviderCore) close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	var old ProviderSession
	if p.started {
		old = p.unbindLocked()
	} else {
		p.generation++
	}
	p.mu.Unlock()
	return closeHuaweiProviderSession(old)
}

func (p *huaweiProviderCore) lastComplete() (ProviderCycle, bool) {
	if p == nil {
		return ProviderCycle{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil {
		return ProviderCycle{}, false
	}
	return p.last.clone(), true
}

func (p *huaweiProviderCore) nextRefreshAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nextAt
}

func (p *huaweiProviderCore) fenced(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed || p.generation != generation
}

// refresh runs one cycle. A concurrent refresh is refused without a request.
// No lock is held while the session is called. Stop, reconnect and close fence
// the running cycle: it sends nothing further and commits nothing.
func (p *huaweiProviderCore) refresh(ctx context.Context) (ProviderCycle, error) {
	if p == nil || ctx == nil {
		return ProviderCycle{}, ErrHuaweiProviderConfig
	}
	p.mu.Lock()
	switch {
	case p.closed:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderClosed
	case p.busy:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderBusy
	case !p.started:
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderNotStarted
	}
	now := p.clock()
	if !p.nextAt.IsZero() && now.Before(p.nextAt) {
		p.mu.Unlock()
		return ProviderCycle{}, ErrHuaweiProviderEarly
	}
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return ProviderCycle{}, err
	}
	p.busy = true
	p.cycle++
	run := &huaweiCycleRun{
		core: p, ctx: ctx, session: p.session, generation: p.generation, cycle: p.cycle, startedAt: now,
		link: p.link, state: p.state.clone(),
		out: ProviderCycle{ProviderID: p.providerID, Path: p.path, Source: p.source, Cycle: p.cycle, Generation: p.generation, StartedAt: now},
	}
	p.mu.Unlock()

	p.run(run)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.busy = false
	if p.closed {
		return ProviderCycle{}, ErrHuaweiProviderClosed
	}
	if p.generation != run.generation {
		// A fenced cycle commits no reading, but a group refused with
		// exception 3 stays refused for the next generation.
		p.carryRefusals(run.state)
		return ProviderCycle{}, ErrHuaweiProviderFenced
	}
	p.link = run.link
	end := p.clock()
	interval := p.pacing.Cycle
	if elapsed := end.Sub(now); elapsed*4 > interval*3 {
		interval = (elapsed*4 + 2) / 3
	}
	interval += time.Duration(run.waitCycles) * p.pacing.Cycle
	p.nextAt = now.Add(interval)
	if run.err != nil {
		// A cancelled cycle or a pacing violation commits no reading, but a
		// group refused with exception 3 stays refused.
		p.carryRefusals(run.state)
		return ProviderCycle{}, run.err
	}
	run.out.CompletedAt = end
	run.out.Loss = append(append([]ProviderLoss(nil), p.staticLoss...), run.out.Loss...)
	p.state = run.state
	result := run.out.clone()
	if result.Outcome == ProviderCycleComplete {
		kept := result.clone()
		p.last = &kept
	}
	return result, nil
}

// carryRefusals keeps the exception 3 refusals of an uncommitted cycle.
func (p *huaweiProviderCore) carryRefusals(from *huaweiProviderState) {
	for index, group := range from.plant {
		p.state.plant[index].refused = p.state.plant[index].refused || group.refused
	}
	for index, group := range from.emmaSite {
		p.state.emmaSite[index].refused = p.state.emmaSite[index].refused || group.refused
	}
	p.carryMeterRefusals(from)
	for unit, child := range from.children {
		for key := range child.refused {
			if kept, ok := p.state.children[unit]; ok {
				kept.refused[key] = true
				continue
			}
			if p.state.carriedRefusals == nil {
				p.state.carriedRefusals = map[uint8]map[string]bool{}
			}
			if p.state.carriedRefusals[unit] == nil {
				p.state.carriedRefusals[unit] = map[string]bool{}
			}
			p.state.carriedRefusals[unit][key] = true
		}
	}
}

// ---- one cycle ---------------------------------------------------------------

type huaweiReadStatus uint8

const (
	huaweiReadAnswered huaweiReadStatus = iota + 1
	huaweiReadException
	huaweiReadFailed
	huaweiReadStopped
)

type huaweiReadResult struct {
	status        huaweiReadStatus
	exceptionCode uint8
	// decodedException reports that the reply was decoded as a definitive
	// exception before a stop or cancellation ended the read.
	decodedException bool
	// last is the outcome of the last attempt of a failed read.
	last ProviderExchangeOutcome
	// overDeadline reports that the read gave up because the budget set on the
	// cycle run would have been exceeded.
	overDeadline bool
}

type huaweiCycleRun struct {
	core       *huaweiProviderCore
	ctx        context.Context
	session    ProviderSession
	generation uint64
	cycle      uint64
	startedAt  time.Time
	link       huaweiLinkState
	state      *huaweiProviderState
	out        ProviderCycle
	// waitCycles is the number of further cycles to skip before the next one.
	waitCycles int
	// budgetEnd, when set, bounds every wait and attempt of the reads made
	// while it is set; no request starts or runs past it.
	budgetEnd time.Time
	err       error
}

func (r *huaweiCycleRun) stopped() bool { return r.err != nil }

// exchange performs one read under the pacing contract: the gap or backoff is
// checked against the clock before every attempt, each attempt has the
// contract deadline, a failed attempt is repeated at most MaxAttempts-1 times
// and an exception other than 5, 6 or 11 is a definitive reply.
func (r *huaweiCycleRun) exchange(record ProviderExchange, send func(context.Context) (ProviderExchangeOutcome, uint8)) huaweiReadResult {
	if r.stopped() {
		return huaweiReadResult{status: huaweiReadStopped}
	}
	pacing := r.core.pacing
	r.out.PlannedReads++
	first := !r.link.sent
	tolerated := false
	failed := 0
	for {
		if r.core.fenced(r.generation) {
			r.err = ErrHuaweiProviderFenced
			return huaweiReadResult{status: huaweiReadStopped}
		}
		spacing := pacing.Gap
		if failed > 0 && failed-1 < len(pacing.Backoff) && pacing.Backoff[failed-1] > spacing {
			spacing = pacing.Backoff[failed-1]
		}
		if r.link.sent {
			required := r.link.lastEnd.Add(spacing)
			if !r.budgetEnd.IsZero() && !required.Before(r.budgetEnd) {
				return huaweiReadResult{status: huaweiReadFailed, last: ProviderExchangeTimeout, overDeadline: true}
			}
			if now := r.core.clock(); now.Before(required) {
				if err := r.core.wait(r.ctx, required.Sub(now)); err != nil {
					r.err = err
					return huaweiReadResult{status: huaweiReadStopped}
				}
				if r.core.clock().Before(required) {
					r.err = ErrHuaweiProviderPacing
					return huaweiReadResult{status: huaweiReadStopped}
				}
			}
		}
		if err := r.ctx.Err(); err != nil {
			r.err = err
			return huaweiReadResult{status: huaweiReadStopped}
		}
		if r.core.fenced(r.generation) {
			r.err = ErrHuaweiProviderFenced
			return huaweiReadResult{status: huaweiReadStopped}
		}
		attemptDeadline := pacing.AttemptDeadline
		if !r.budgetEnd.IsZero() {
			remaining := r.budgetEnd.Sub(r.core.clock())
			if remaining <= 0 {
				return huaweiReadResult{status: huaweiReadFailed, last: ProviderExchangeTimeout, overDeadline: true}
			}
			if remaining < attemptDeadline {
				attemptDeadline = remaining
			}
		}
		attemptCtx, cancel := context.WithTimeout(r.ctx, attemptDeadline)
		sent := r.core.clock()
		outcome, code := send(attemptCtx)
		end := r.core.clock()
		// A reply that arrives after the attempt deadline is not current: the
		// attempt context is read before it is cancelled.
		if (outcome == ProviderExchangeAnswered || outcome == ProviderExchangeException) && (errors.Is(attemptCtx.Err(), context.DeadlineExceeded) || end.Sub(sent) > attemptDeadline) {
			outcome, code = ProviderExchangeTimeout, 0
		}
		cancel()
		r.link.sent, r.link.lastEnd = true, end
		r.out.WireAttempts++
		attempt := record
		attempt.Attempt, attempt.Outcome, attempt.ExceptionCode, attempt.SentAt = failed+1, outcome, code, sent
		switch {
		case outcome == ProviderExchangeTimeout:
			attempt.Elapsed = ProviderAtDeadline
		case end.Sub(sent) <= pacing.PromptBound:
			attempt.Elapsed = ProviderWithinPromptBound
		default:
			attempt.Elapsed = ProviderWithinDeadline
		}
		if first && !tolerated && pacing.FirstRequestExtraAttempt && outcome == ProviderExchangeTimeout && failed == 0 {
			tolerated = true
			attempt.Elapsed = ProviderFirstRequestTolerated
			r.out.Exchanges = append(r.out.Exchanges, attempt)
			if err := r.ctx.Err(); err != nil {
				r.err = err
				return huaweiReadResult{status: huaweiReadStopped}
			}
			continue
		}
		r.out.Exchanges = append(r.out.Exchanges, attempt)
		if err := r.ctx.Err(); err != nil {
			r.err = err
			// The reply was decoded before the cancellation: a definitive
			// exception is handed to the caller so it is recorded.
			stopped := huaweiReadResult{status: huaweiReadStopped}
			if outcome == ProviderExchangeException && code != 5 && code != 6 && code != 11 {
				stopped.exceptionCode, stopped.decodedException = code, true
			}
			return stopped
		}
		switch outcome {
		case ProviderExchangeAnswered:
			return huaweiReadResult{status: huaweiReadAnswered}
		case ProviderExchangeException:
			if code != 5 && code != 6 && code != 11 {
				return huaweiReadResult{status: huaweiReadException, exceptionCode: code}
			}
		}
		failed++
		if failed >= pacing.MaxAttempts {
			return huaweiReadResult{status: huaweiReadFailed, last: outcome}
		}
	}
}

func classifyHuaweiProviderError(attemptCtx context.Context, err error) (ProviderExchangeOutcome, uint8) {
	var protocol *modbus.ProtocolError
	if errors.As(err, &protocol) {
		if protocol.Kind == modbus.ErrorExceptionResponse && protocol.ExceptionCode != 0 {
			return ProviderExchangeException, protocol.ExceptionCode
		}
		return ProviderExchangeMalformed, 0
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return ProviderExchangeTimeout, 0
	}
	return ProviderExchangeTransport, 0
}

// readHolding reads one FC03 extent.
func (r *huaweiCycleRun) readHolding(unit uint8, offset, quantity uint16) ([]uint16, huaweiReadResult) {
	request, err := modbus.NewReadRegistersRequest(modbus.FunctionReadHoldingRegisters, offset, quantity)
	if err != nil {
		r.err = ErrHuaweiProviderConfig
		return nil, huaweiReadResult{status: huaweiReadStopped}
	}
	want := modbus.ReadProvenance{Function: modbus.FunctionReadHoldingRegisters, Table: modbus.HoldingRegisters, Offset: offset, Quantity: quantity}
	var words []uint16
	result := r.exchange(ProviderExchange{Function: uint8(modbus.FunctionReadHoldingRegisters), UnitID: unit, Offset: offset, Quantity: quantity},
		func(attemptCtx context.Context) (ProviderExchangeOutcome, uint8) {
			response, err := r.session.ReadHolding(attemptCtx, unit, request)
			if err != nil {
				return classifyHuaweiProviderError(attemptCtx, err)
			}
			if response.Provenance != want || len(response.Words) != int(quantity) {
				return ProviderExchangeMalformed, 0
			}
			words = append([]uint16(nil), response.Words...)
			return ProviderExchangeAnswered, 0
		})
	if result.status != huaweiReadAnswered {
		return nil, result
	}
	return words, result
}

// readPlantGroups runs only after identification or a held guard. A group that
// does not answer cannot replace the kept words of that group or block its sibling.
func (r *huaweiCycleRun) readPlantGroups(plan huaweiPlantPlan) {
	for index, extent := range plan.groups {
		kept := &r.state.plant[index]
		outcome, code := r.readGatewayGroup(plan.unit, extent.offset, extent.quantity, kept)
		if r.stopped() {
			return
		}
		r.recordPlantGroup(kept.observation(plan.unit, extent, outcome, code))
	}
}

func (r *huaweiCycleRun) readGatewayGroup(unit uint8, offset, quantity uint16, kept *huaweiPlantState) (SUN2000GroupOutcome, uint8) {
	if kept.refused {
		return SUN2000GroupNotRead, 0
	}
	words, result := r.readHolding(unit, offset, quantity)
	kept.refused = result.exceptionCode == huaweiSUN2000RefusalCode
	switch result.status {
	case huaweiReadAnswered:
		kept.words = words
		kept.meta = huaweiGroupMeta{cycle: r.cycle, readAt: r.link.lastEnd, generation: r.generation}
		return SUN2000GroupAnswered, 0
	case huaweiReadException:
		return SUN2000GroupException, result.exceptionCode
	case huaweiReadFailed:
		return SUN2000GroupFailed, 0
	default:
		return SUN2000GroupNotRead, 0
	}
}

// finishPlantGroups renders unattempted groups when admission or the guard
// failed. An identity mismatch fences the old gateway's plant words.
func (r *huaweiCycleRun) finishPlantGroups(plan huaweiPlantPlan) {
	if r.stopped() || r.out.Plant != nil {
		return
	}
	if r.state.gatewayFenced {
		r.state.plant = [2]huaweiPlantState{}
	}
	for index, extent := range plan.groups {
		r.recordPlantGroup(r.state.plant[index].observation(plan.unit, extent, SUN2000GroupNotRead, 0))
	}
}

func (r *huaweiCycleRun) recordPlantGroup(group ProviderPlantGroup) {
	r.out.Plant = append(r.out.Plant, group)
	r.recordGatewayGroupLoss(group.UnitID, group.ProviderGroupEvidence, "plant group did not answer")
}

func (r *huaweiCycleRun) recordGatewayGroupLoss(unit uint8, group ProviderGroupEvidence, failure string) {
	loss := ProviderLoss{HasUnit: true, UnitID: unit, Offset: group.Offset, Quantity: group.Quantity}
	switch {
	case group.Refused:
		loss.Kind, loss.Detail = ProviderLossRefusedGroup, huaweiRuntimeRefusalDetail(huaweiSUN2000RefusalCode)
	case group.Outcome == SUN2000GroupException:
		loss.Kind, loss.Detail = ProviderLossRefusedGroup, huaweiRuntimeRefusalDetail(group.ExceptionCode)
	case group.Outcome == SUN2000GroupFailed:
		loss.Kind, loss.Detail = ProviderLossFailedGroup, failure
	default:
		return
	}
	r.out.Loss = append(r.out.Loss, loss)
}

// readIdentification reads one Read Device Identification page.
func (r *huaweiCycleRun) readIdentification(unit uint8, request IdentificationRequest) (IdentificationPage, huaweiReadResult) {
	var page IdentificationPage
	result := r.exchange(ProviderExchange{Function: uint8(modbus.FunctionEncapsulatedInterface), UnitID: unit, ReadDeviceIDCode: request.ReadDeviceIDCode, ObjectID: request.ObjectID},
		func(attemptCtx context.Context) (ProviderExchangeOutcome, uint8) {
			reply, err := r.session.ReadDeviceIdentification(attemptCtx, unit, request)
			if err != nil {
				return classifyHuaweiProviderError(attemptCtx, err)
			}
			if reply.ReadDeviceIDCode != request.ReadDeviceIDCode || len(reply.Objects) == 0 {
				return ProviderExchangeMalformed, 0
			}
			page = reply.clone()
			return ProviderExchangeAnswered, 0
		})
	if result.status != huaweiReadAnswered {
		return IdentificationPage{}, result
	}
	return page, result
}

func huaweiWordsToBytes(words []uint16) []byte {
	out := make([]byte, 0, 2*len(words))
	for _, word := range words {
		out = append(out, byte(word>>8), byte(word))
	}
	return out
}

// ---- child admission -----------------------------------------------------------

// childAdmissionRead handles one admission read of a child and reports
// whether it answered. A read that failed leaves the child unadmitted: a
// reading of an earlier identity stays as stale history, nothing is guessed,
// and the admission runs again on a later cycle after the child back-off. An
// exception withholds the child until the next identification event. A stop
// adds nothing; the caller checks it.
func (r *huaweiCycleRun) childAdmissionRead(child *huaweiChildState, result huaweiReadResult) bool {
	switch result.status {
	case huaweiReadAnswered:
		return true
	case huaweiReadFailed:
		child.failures++
		child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
		reason := ProviderReasonChildReadFailed
		if result.last == ProviderExchangeTimeout {
			reason = ProviderReasonChildTimeout
		}
		if child.emmaPlan != nil {
			r.monitorEMMAChild(child, reason)
			return false
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), reason, nil))
	case huaweiReadException:
		if r.core.path == ProviderPathEMMA {
			child.catalog = nil
			return r.withholdEMMAChild(child, ProviderReasonIdentityInsuff, true)
		}
		child.withhold(ProviderReasonIdentityInsuff, nil)
		r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, child.definitive, nil))
	}
	return false
}

// readChildIdentityWords reads the model identifier, the software text and the
// firmware text at the child's routed unit, one request at a time under the
// path's pacing. It reports false when a read did not answer; the child's
// observation is then already added, or the cycle was stopped.
func (r *huaweiCycleRun) readChildIdentityWords(child *huaweiChildState) (SUN2000RoutedIdentity, bool) {
	identifier, result := r.readHolding(child.unit, huaweiSUN2000IdentifierOffset, 1)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	software, result := r.readHolding(child.unit, huaweiSUN2000SoftwareOffset, huaweiSUN2000TextQuantity)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	firmware, result := r.readHolding(child.unit, huaweiSUN2000FirmwareOffset, huaweiSUN2000TextQuantity)
	if !r.childAdmissionRead(child, result) {
		return SUN2000RoutedIdentity{}, false
	}
	return SUN2000RoutedIdentity{
		IdentifierWords: identifier, SoftwareBlock: huaweiWordsToBytes(software), FirmwareBlock: huaweiWordsToBytes(firmware),
	}, true
}

// bindChildSelection admits the child on the profile of a selected catalog
// row, recording the decision. A changed identity fences the earlier reading.
// Anything else withholds the child until the next identification event: a
// decision that selected no row with its closed reason, an ambiguous decision
// with every candidate, a selected row whose family has no profile on this
// path, a model that disagrees with the listing, or insufficient evidence.
func (r *huaweiCycleRun) bindChildSelection(child *huaweiChildState, selection SUN2000CapabilitySelection) bool {
	var decision *SUN2000CatalogDecision
	if taken, ok := selection.Catalog(); ok {
		decision = &taken
	}
	profile, ok := selection.Profile()
	if selection.Outcome() == SUN2000ProfileSelected && ok && decision != nil && decision.Outcome() == SUN2000CatalogSelected {
		identity := &huaweiChildIdentity{profile: profile, model: profile.Model()}
		if child.identity != nil && child.identity.key() != identity.key() {
			child.fence()
		}
		child.identity, child.identifiedGeneration, child.definitive, child.catalog = identity, r.generation, "", decision
		return true
	}
	reason := ProviderReasonIdentityInsuff
	switch {
	case selection.Reason() == SUN2000ReasonCatalogAmbiguous:
		reason = ProviderReasonCatalogAmbiguous
	case selection.Reason() == SUN2000ReasonCatalogNotSelected:
		reason = ProviderReasonCatalogNotSelected
	case selection.Outcome() == SUN2000NoAdmissibleProfile:
		reason = ProviderReasonNoProfile
	case selection.Reason() == SUN2000ReasonListingModelConflict:
		reason = ProviderReasonIdentityMismatch
	}
	child.withhold(reason, decision)
	r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, reason, nil))
	return false
}

// huaweiChildBackoffCycles is the registry's back-off of one child after
// consecutive failures: none after the first, then doubling up to ten cycles.
// huaweiChildSkipUntil returns the first cycle at which a failed child is read
// again; the skip check is strict, so the backoff length is skipped in full.
func huaweiChildSkipUntil(cycle uint64, failures int) uint64 {
	wait := huaweiChildBackoffCycles(failures)
	if wait == 0 {
		return cycle
	}
	return cycle + wait + 1
}

func huaweiChildBackoffCycles(failures int) uint64 {
	if failures < 2 {
		return 0
	}
	wait := uint64(1)
	for step := 2; step < failures && wait < 10; step++ {
		wait *= 2
	}
	if wait > 10 {
		wait = 10
	}
	return wait
}

// huaweiEndpointBackoffCycles doubles from one cycle up to ten cycles.
func huaweiEndpointBackoffCycles(failures int) int {
	wait := 1
	for step := 1; step < failures && wait < 10; step++ {
		wait *= 2
	}
	if wait > 10 {
		wait = 10
	}
	return wait
}

// monitorChild reads the plan of an identified child's profile, skipping the
// groups refused at runtime, decodes it with the profile and retains the last
// complete reading of each group through RetainHuaweiSUN2000Monitoring. An
// exception reply is recorded and the next group is read; a failed read ends
// the child's cycle.
func (r *huaweiCycleRun) monitorChild(child *huaweiChildState) (ProviderEvidenceState, ProviderReason, map[uint16]SUN2000GroupReading) {
	profile := child.identity.profile
	current := map[uint16]SUN2000GroupReading{}
	var readings []SUN2000GroupReading
	state, reason := ProviderEvidenceCurrent, ProviderReasonRead
	readAt := map[uint16]time.Time{}
	for _, step := range profile.Plan(child.unit) {
		if child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] {
			continue
		}
		words, result := r.readHolding(child.unit, step.Offset, step.Quantity)
		reading := SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity}
		switch result.status {
		case huaweiReadAnswered:
			reading.Outcome, reading.Words, reading.Cycle = SUN2000GroupAnswered, words, r.cycle
			readAt[step.Offset] = r.link.lastEnd
		case huaweiReadException:
			reading.Outcome, reading.ExceptionCode = SUN2000GroupException, result.exceptionCode
			if result.exceptionCode == huaweiSUN2000RefusalCode {
				child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] = true
			}
			state, reason = ProviderEvidenceStale, ProviderReasonChildException
		case huaweiReadFailed:
			reading.Outcome = SUN2000GroupFailed
			state, reason = ProviderEvidenceStale, ProviderReasonChildReadFailed
			if result.last == ProviderExchangeTimeout {
				reason = ProviderReasonChildTimeout
			}
		default:
			// Stopped: a refusal decoded before the stop is still recorded.
			if result.decodedException && result.exceptionCode == huaweiSUN2000RefusalCode {
				child.refused[huaweiRefusalKey(profile.ID(), step.Offset, step.Quantity)] = true
			}
			return "", "", nil
		}
		current[step.Offset] = reading
		readings = append(readings, reading)
		if result.status == huaweiReadFailed {
			break
		}
	}
	if r.core.path == ProviderPathSDongle {
		meterReason := r.readSDongleMeter(child, state == ProviderEvidenceCurrent || reason == ProviderReasonChildException)
		if state == ProviderEvidenceCurrent && meterReason != ProviderReasonRead {
			state, reason = ProviderEvidenceStale, meterReason
		}
		if r.stopped() {
			return "", "", nil
		}
	}
	decoded, err := profile.Decode(child.unit, readings)
	if err != nil {
		return ProviderEvidenceStale, ProviderReasonChildReadFailed, current
	}
	retained := RetainHuaweiSUN2000Monitoring(child.reading, decoded)
	for offset, at := range readAt {
		child.groups[offset] = huaweiGroupMeta{cycle: r.cycle, readAt: at, generation: r.generation}
	}
	child.reading = retained
	if state == ProviderEvidenceCurrent {
		child.failures, child.skipUntil = 0, 0
	} else if reason != ProviderReasonChildException {
		child.failures++
		child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
	}
	return state, reason, current
}

// childObservation renders one child for the cycle output.
func (r *huaweiCycleRun) childObservation(child *huaweiChildState, state ProviderEvidenceState, reason ProviderReason, current map[uint16]SUN2000GroupReading) ProviderChildObservation {
	out := ProviderChildObservation{
		UnitID: child.unit, ListedClass: child.listedClass, Evidence: state, Reason: reason, Fenced: child.fenced.clone(),
		Catalog: huaweiCloneCatalogDecision(child.catalog),
	}
	if child.identity != nil {
		out.Family, out.ProfileID, out.Model = child.identity.profile.Family(), child.identity.profile.ID(), child.identity.model
		out.IdentifiedGeneration = child.identifiedGeneration
	}
	if state != ProviderEvidenceWithheld && child.reading.ProfileID() != "" && child.identity != nil {
		if current == nil && r.core.path == ProviderPathSDongle {
			r.readSDongleMeter(child, false)
		}
		if current == nil {
			// Not read this cycle: every kept group is retained with its
			// original cycle, through the same retention as a failed read.
			current = map[uint16]SUN2000GroupReading{}
			if empty, err := child.identity.profile.Decode(child.unit, nil); err == nil {
				view := *child
				view.reading = RetainHuaweiSUN2000Monitoring(child.reading, empty)
				out.Reading, out.Groups = view.reading, view.groupEvidence(current)
				return r.recordChildLoss(out, state, reason)
			}
		}
		out.Reading = child.reading
		out.Groups = child.groupEvidence(current)
	}
	return r.recordChildLoss(out, state, reason)
}

func (r *huaweiCycleRun) recordChildLoss(out ProviderChildObservation, state ProviderEvidenceState, reason ProviderReason) ProviderChildObservation {
	if state == ProviderEvidenceWithheld {
		r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossWithheldChild, HasUnit: true, UnitID: out.UnitID, Detail: huaweiWithheldDetail(reason, out.Catalog)})
	}
	for _, group := range out.Groups {
		if group.Refused {
			r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossRefusedGroup, HasUnit: true, UnitID: out.UnitID,
				Offset: group.Offset, Quantity: group.Quantity, Detail: huaweiRuntimeRefusalDetail(huaweiSUN2000RefusalCode)})
		}
	}
	return out
}

// huaweiRuntimeRefusalDetail is the loss text for a group the unit refused.
func huaweiRuntimeRefusalDetail(code uint8) string {
	if code == huaweiSUN2000RefusalCode {
		return "refused with exception 3 at runtime; not sent again"
	}
	return "refused with exception " + strconv.Itoa(int(code)) + " at runtime"
}

// CatalogProfileLosses is what a catalogue profile still does not publish.
// The hand profiles keep their own holes; a catalogue profile consumes the
// status, string, power and energy words its card documents, and it does not
// list those words as unconsumed. Battery and meter facts stay unadmitted.
func CatalogProfileLosses(profile CatalogMonitoringProfile) []ProviderLoss {
	if profile.ID() == "" {
		return nil
	}
	return []ProviderLoss{
		{Kind: ProviderLossBattery, Detail: "battery offsets are never read; a zero word is never a battery value"},
		{Kind: ProviderLossMeter, Detail: "meter values are not part of the inverter catalogue profile"},
	}
}

// huaweiWithheldDetail names why a child is withheld. A catalog withhold
// adds the closed reason, and an ambiguous one names every candidate row.
func huaweiWithheldDetail(reason ProviderReason, decision *SUN2000CatalogDecision) string {
	detail := string(reason)
	if decision == nil {
		return detail
	}
	switch reason {
	case ProviderReasonCatalogNotSelected:
		detail += ": " + string(decision.Reason())
	case ProviderReasonCatalogAmbiguous:
		detail += ":"
		for index, row := range decision.Candidates() {
			if index > 0 {
				detail += ","
			}
			detail += " " + row.Card + " " + strconv.Itoa(row.Identifier)
		}
	}
	return detail
}

// staleChildren renders every identified child as stale for a cycle whose
// children were not read, keeping their retained readings with original age.
func (r *huaweiCycleRun) staleChildren(reason ProviderReason, withheld bool) {
	units := make([]int, 0, len(r.state.children))
	for unit := range r.state.children {
		units = append(units, int(unit))
	}
	sort.Ints(units)
	for _, unit := range units {
		child := r.state.children[uint8(unit)]
		state, why := ProviderEvidenceStale, reason
		if r.state.gatewayFenced {
			// A gateway mismatch holds until the identity is read again and
			// matches; a later read failure never releases the readings.
			withheld, why = true, ProviderReasonGatewayMismatch
		}
		if r.state.gatewayFenced {
			child.meter = huaweiMeterState{}
		}
		if r.core.path == ProviderPathSmartLogger && !withheld && child.listedClass == huaweiMeterClassLabel && child.ineligible == "" {
			r.readLoggerMeter(child, false)
			r.observeLoggerMeter(child, ProviderEvidenceStale, why)
			continue
		}
		if withheld || child.identity == nil || child.reading.ProfileID() == "" || child.ineligible != "" {
			state = ProviderEvidenceWithheld
		}
		if child.ineligible != "" && !withheld && child.reading.ProfileID() != "" {
			why = child.ineligible
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, state, why, nil))
	}
}

// huaweiChildProfileLoss names what a child profile never publishes.
func huaweiChildProfileLoss(profile SUN2000MonitoringProfile) []ProviderLoss {
	var loss []ProviderLoss
	for _, refused := range profile.RefusedGroups() {
		loss = append(loss, ProviderLoss{Kind: ProviderLossRefusedGroup, Offset: refused.Offset, Quantity: refused.Quantity,
			Detail: profile.Family() + ": refused with exception " + strconv.Itoa(int(refused.ExceptionCode)) + "; never read"})
	}
	loss = append(loss,
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: huaweiSUN2000StatusGroup.offset, Quantity: huaweiSUN2000StatusGroup.quantity,
			Detail: "status group retained as native evidence; no state is published from it"},
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: 32032, Quantity: 8, Detail: "string words past string 8"},
		ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: 32108, Quantity: 4, Detail: "no numeric source field detected in interval"},
		ProviderLoss{Kind: ProviderLossBattery, Detail: "battery offsets are never read; a zero word is never a battery value"},
	)
	if profile.ID() == SUN2000M1MonitoringProfileID {
		loss = append(loss, ProviderLoss{Kind: ProviderLossUnconsumedWords, Offset: huaweiSUN2000PowerGroup.offset, Quantity: huaweiSUN2000PowerGroup.quantity,
			Detail: "power group words other than input power, active power and the observed status code"})
	}
	return loss
}

func huaweiProfileByID(id string) (SUN2000MonitoringProfile, bool) {
	for _, profile := range SUN2000MonitoringProfiles() {
		if profile.ID() == id {
			return profile, true
		}
	}
	return SUN2000MonitoringProfile{}, false
}
