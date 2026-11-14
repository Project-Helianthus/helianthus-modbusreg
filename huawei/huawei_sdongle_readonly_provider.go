package huawei

import (
	"context"
	"time"
)

// SDongleReadOnlyProviderID identifies the read-only provider of the
// observed S-Dongle path.
const SDongleReadOnlyProviderID = "huawei.sdongle.observed-readonly-provider@1.0.0"

const (
	huaweiSDongleSearchOffset   = 37410
	huaweiSDongleSearchQuantity = 3
)

// SDongleReadOnlyProvider reads the observed S-Dongle path over a
// supplied session. Every cycle starts with the search-sequence read at the
// gateway unit, which is also the first request of each connection
// generation. On a new generation, after a guard change and after a failed
// guard the observed card's whole tuple is read again on that generation,
// including the refusal of basic identification at the gateway unit, and the
// child's admission reads the model identifier, the software text and the
// firmware text at the child unit, before the M1 series groups are read there
// once its catalog decision selects the M1 series row. Only the gateway unit
// and the observed child unit are addressed; no meter is read. Do not copy it.
type SDongleReadOnlyProvider struct {
	core *huaweiProviderCore
}

// NewSDongleReadOnlyProvider constructs the provider behind the
// enablement gate of the recorded S-Dongle disposition. The allowlist must
// name exactly the observed child unit.
func NewSDongleReadOnlyProvider(config ProviderConfig) (*SDongleReadOnlyProvider, error) {
	disposition, _ := RecordedProviderDisposition(ProviderPathSDongle)
	return newHuaweiSDongleReadOnlyProvider(config, disposition)
}

func newHuaweiSDongleReadOnlyProvider(config ProviderConfig, disposition ProviderDisposition) (*SDongleReadOnlyProvider, error) {
	card := NewSDongleObservedQualificationCard()
	if len(config.AllowedUnits) != 1 || config.AllowedUnits[0] != card.ChildUnit() {
		if !EvaluateHuaweiProviderEnablement(disposition.clone()).Admitted {
			return nil, ErrHuaweiProviderNotAdmitted
		}
		return nil, ErrHuaweiProviderConfig
	}
	profile, _ := huaweiProfileByID(SUN2000M1MonitoringProfileID)
	gateway := card.UnitID()
	loss := []ProviderLoss{
		{Kind: ProviderLossPlannedGroup, HasUnit: true, UnitID: gateway, Offset: 37498, Quantity: 8, Detail: "dongle plant power group: planned until an approved observation records its answer"},
		{Kind: ProviderLossPlannedGroup, HasUnit: true, UnitID: gateway, Offset: 37516, Quantity: 2, Detail: "dongle plant AC power group: planned until an approved observation records its answer"},
		{Kind: ProviderLossMeter, HasUnit: true, UnitID: card.ChildUnit(), Offset: 37100, Quantity: 39, Detail: "inverter-exposed meter group: planned until a meter class contract exists"},
	}
	loss = append(loss, huaweiChildProfileLoss(profile)...)
	core, err := newHuaweiProviderCore(ProviderPathSDongle, SDongleReadOnlyProviderID, config, disposition, runHuaweiSDongleCycle, loss)
	if err != nil {
		return nil, err
	}
	return &SDongleReadOnlyProvider{core: core}, nil
}

// Start binds the first connection generation.
func (p *SDongleReadOnlyProvider) Start(session ProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.start(session)
}

// Stop closes the bound connection and fences a running refresh.
func (p *SDongleReadOnlyProvider) Stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.stop()
}

// Reconnect closes the bound connection first and binds the new one as a new
// connection generation, whose first request may take one extra attempt.
func (p *SDongleReadOnlyProvider) Reconnect(session ProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.reconnect(session)
}

// Close is terminal: it closes the bound connection and fences late results.
func (p *SDongleReadOnlyProvider) Close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.close()
}

// Refresh runs one cycle under the same refusals as the SmartLogger provider.
func (p *SDongleReadOnlyProvider) Refresh(ctx context.Context) (ProviderCycle, error) {
	if p == nil {
		return ProviderCycle{}, ErrHuaweiProviderConfig
	}
	return p.core.refresh(ctx)
}

// LastComplete returns a detached copy of the last complete cycle.
func (p *SDongleReadOnlyProvider) LastComplete() (ProviderCycle, bool) {
	if p == nil {
		return ProviderCycle{}, false
	}
	return p.core.lastComplete()
}

// NextRefreshAt is the earliest start of the next cycle.
func (p *SDongleReadOnlyProvider) NextRefreshAt() time.Time { return p.core.nextRefreshAt() }

// Pacing returns the pacing envelope of the S-Dongle path.
func (p *SDongleReadOnlyProvider) Pacing() ProviderPacing { return p.core.pacing.clone() }

// Enablement returns the gate decision the provider was constructed under.
func (p *SDongleReadOnlyProvider) Enablement() ProviderEnablement {
	enablement := p.core.enablement
	enablement.Detail = append([]string(nil), enablement.Detail...)
	enablement.Profiles = append([]string(nil), enablement.Profiles...)
	return enablement
}

func runHuaweiSDongleCycle(r *huaweiCycleRun) {
	defer func() { r.out.Gateway = r.state.gateway }()
	s := r.state
	card := NewSDongleObservedQualificationCard()
	guard, result := r.readHolding(card.UnitID(), huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity)
	switch result.status {
	case huaweiReadStopped:
		return
	case huaweiReadAnswered:
	default:
		r.out.Outcome, r.out.Reason = ProviderCycleIncomplete, ProviderReasonGuardFailed
		if s.identified {
			s.gateway.State = ProviderGatewayUnconfirmed
		} else {
			s.gateway.State = ProviderGatewayInsufficient
			r.out.Outcome = ProviderCycleInsufficient
		}
		s.reidentify = true
		if result.status == huaweiReadFailed {
			s.endpointFailures++
			r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
		}
		r.staleChildren(ProviderReasonListingUnconfirmed, false)
		return
	}
	if s.identified && !s.reidentify && s.identityGeneration == r.generation {
		if len(s.guard) == huaweiSDongleSearchQuantity && guard[1] == huaweiSDongleObservedSearchState && guard[2] == s.guard[2] {
			s.endpointFailures = 0
			s.gateway.State = ProviderGatewayIdentified
			r.out.Reason = ProviderReasonGuardHeld
			r.readSDongleChild()
			if !r.stopped() {
				r.out.Outcome = ProviderCycleComplete
			}
			return
		}
		s.reidentify = true
		r.out.Outcome, r.out.Reason = ProviderCycleIncomplete, ProviderReasonGuardChanged
		r.staleChildren(ProviderReasonListingUnconfirmed, false)
		return
	}
	r.identifySDongle(guard)
}

// identifySDongle reads the rest of the observed card's tuple on this
// connection generation. The guard read of the cycle is the search sequence
// before the gateway software and child reads; the same read after them closes
// the bracket.
func (r *huaweiCycleRun) identifySDongle(before []uint16) {
	s := r.state
	card := NewSDongleObservedQualificationCard()
	gateway, childUnit := card.UnitID(), card.ChildUnit()
	failed := false
	stop := func(result huaweiReadResult) bool {
		switch result.status {
		case huaweiReadStopped:
			return true
		case huaweiReadFailed:
			failed = true
			return true
		}
		return false
	}
	words := func(unit uint8, offset, quantity uint16) ([]uint16, bool) {
		value, result := r.readHolding(unit, offset, quantity)
		return value, stop(result)
	}

	input := SDongleObservedQualificationInput{Gateway: SDongleObservedGatewayReads{UnitID: gateway, SearchBefore: before}}
	basic, result := r.readIdentification(gateway, IdentificationRequest{ReadDeviceIDCode: 1})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	switch result.status {
	case huaweiReadException:
		input.Gateway.Basic.ExceptionCode = result.exceptionCode
	case huaweiReadAnswered:
		input.Gateway.Basic.Reply = &SDongleObservedBasicReply{Objects: basic.Objects}
	}
	var halted bool
	if input.Gateway.SearchStatus, halted = words(gateway, 37411, 1); halted {
		r.sdongleIdentityFailed(failed)
		return
	}
	if input.Gateway.Protocol, halted = words(gateway, 30068, 2); halted {
		r.sdongleIdentityFailed(failed)
		return
	}
	if input.Gateway.Capacity, halted = words(gateway, 37429, 1); halted {
		r.sdongleIdentityFailed(failed)
		return
	}
	o24Software, o24Result := r.readHolding(gateway, 30050, 15)
	if stop(o24Result) || o24Result.status != huaweiReadAnswered {
		r.sdongleIdentityFailed(failed)
		return
	}
	input.Gateway.Software = huaweiWordsToBytes(o24Software)
	model, halted := words(childUnit, huaweiSUN2000ModelOffset, huaweiSUN2000ModelQuantity)
	if halted {
		r.sdongleIdentityFailed(failed)
		return
	}
	input.Child = SDongleObservedChildReads{UnitID: childUnit, ModelBlock: huaweiWordsToBytes(model)}
	childBasic, result := r.readIdentification(childUnit, IdentificationRequest{ReadDeviceIDCode: 1})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	if result.status == huaweiReadAnswered {
		input.Child.Basic = &SDongleObservedBasicReply{Objects: childBasic.Objects}
	}
	inventory, result := r.readIdentification(childUnit, IdentificationRequest{ReadDeviceIDCode: 3, ObjectID: huaweiSmartLoggerInventoryStartObject})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	if result.status == huaweiReadAnswered {
		input.Child.Inventory = SDongleObservedChildInventoryPage(inventory)
	}
	if input.Gateway.SearchAfter, halted = words(gateway, huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity); halted {
		r.sdongleIdentityFailed(failed)
		return
	}

	evaluation := card.Evaluate(input)
	if !evaluation.IdentityMatched() || !evaluation.QualificationTestReady() {
		// A gateway mismatch withholds the instance.
		s.identified, s.reidentify, s.gatewayFenced = false, true, true
		s.gateway = ProviderGatewayIdentity{CardID: card.ID(), Class: card.CandidateClass(), UnitID: gateway,
			State: ProviderGatewayInsufficient, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd}
		r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayMismatch
		r.staleChildren(ProviderReasonGatewayMismatch, true)
		return
	}
	s.endpointFailures = 0
	s.identified, s.identityGeneration, s.reidentify, s.gatewayFenced = true, r.generation, false, false
	s.guard = append([]uint16(nil), input.Gateway.SearchAfter...)
	s.sdongle = &input
	s.gateway = ProviderGatewayIdentity{
		CardID: card.ID(), Class: card.CandidateClass(), UnitID: gateway, ChildModel: card.ChildModel(),
		State: ProviderGatewayIdentified, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd,
	}
	child := s.child(childUnit)
	child.listedClass = card.ChildFamily()
	child.identificationEvent()
	r.out.Reason = ProviderReasonIdentified
	if !r.admitSDongleChild(child) {
		if !r.stopped() {
			r.out.Outcome = ProviderCycleComplete
		}
		return
	}
	state, reason, current := r.monitorChild(child)
	if r.stopped() || state == "" {
		return
	}
	r.out.Children = append(r.out.Children, r.childObservation(child, state, reason, current))
	r.out.Outcome = ProviderCycleComplete
}

// admitSDongleChild runs the child's admission after the observed card has
// matched on this connection generation: the model identifier, the software
// text and the firmware text at the child unit, and the SUN2000 capability
// selection over them with the child model that the card binds. It reports
// whether the child is identified; otherwise it has already added the child's
// observation, or the cycle was stopped.
func (r *huaweiCycleRun) admitSDongleChild(child *huaweiChildState) bool {
	identity, ok := r.readChildIdentityWords(child)
	if !ok {
		return false
	}
	selection := SelectHuaweiSUN2000MonitoringProfile(SUN2000CapabilityInput{SDongle: r.state.sdongle, UnitID: child.unit, Identity: identity})
	return r.bindChildSelection(child, selection)
}

func (r *huaweiCycleRun) sdongleIdentityFailed(failed bool) {
	if r.stopped() {
		return
	}
	s := r.state
	s.identified, s.reidentify = false, true
	s.gateway.State = ProviderGatewayInsufficient
	r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayReadFail
	if failed {
		s.endpointFailures++
		r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
	}
	r.staleChildren(ProviderReasonGatewayReadFail, false)
}

// readSDongleChild reads the identified child's groups after a held guard. A
// child withheld by its admission stays withheld until the next
// identification event; an admission that a failed read left incomplete runs
// again, after the child back-off.
func (r *huaweiCycleRun) readSDongleChild() {
	card := NewSDongleObservedQualificationCard()
	child := r.state.child(card.ChildUnit())
	switch {
	case child.definitive != "":
		r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, child.definitive, nil))
		return
	case child.skipUntil > r.cycle:
		r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), ProviderReasonChildBackoff, nil))
		return
	case child.identity == nil || child.identifiedGeneration != r.generation:
		if !r.admitSDongleChild(child) {
			return
		}
	}
	state, reason, current := r.monitorChild(child)
	if r.stopped() || state == "" {
		return
	}
	r.out.Children = append(r.out.Children, r.childObservation(child, state, reason, current))
}
