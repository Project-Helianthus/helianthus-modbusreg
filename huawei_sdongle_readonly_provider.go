package modbusreg

import (
	"context"
	"time"
)

// HuaweiSDongleReadOnlyProviderID identifies the read-only provider of the
// observed S-Dongle path.
const HuaweiSDongleReadOnlyProviderID = "huawei.sdongle.observed-readonly-provider@1.0.0"

const (
	huaweiSDongleSearchOffset   = 37410
	huaweiSDongleSearchQuantity = 3
)

// HuaweiSDongleReadOnlyProvider reads the observed S-Dongle path over a
// supplied session. Every cycle starts with the search-sequence read at the
// gateway unit, which is also the first request of each connection
// generation. On a new generation, after a guard change and after a failed
// guard the observed card's whole tuple is read again on that generation,
// including the refusal of basic identification at the gateway unit, before
// the M1 series groups are read at the one child unit. Only the gateway unit
// and the observed child unit are addressed; no meter is read. Do not copy it.
type HuaweiSDongleReadOnlyProvider struct {
	core *huaweiProviderCore
}

// NewHuaweiSDongleReadOnlyProvider constructs the provider behind the
// enablement gate of the recorded S-Dongle disposition. The allowlist must
// name exactly the observed child unit.
func NewHuaweiSDongleReadOnlyProvider(config HuaweiProviderConfig) (*HuaweiSDongleReadOnlyProvider, error) {
	disposition, _ := HuaweiRecordedProviderDisposition(HuaweiProviderPathSDongle)
	return newHuaweiSDongleReadOnlyProvider(config, disposition)
}

func newHuaweiSDongleReadOnlyProvider(config HuaweiProviderConfig, disposition HuaweiProviderDisposition) (*HuaweiSDongleReadOnlyProvider, error) {
	card := NewHuaweiSDongleObservedQualificationCard()
	if len(config.AllowedUnits) != 1 || config.AllowedUnits[0] != card.ChildUnit() {
		if !EvaluateHuaweiProviderEnablement(disposition.clone()).Admitted {
			return nil, ErrHuaweiProviderNotAdmitted
		}
		return nil, ErrHuaweiProviderConfig
	}
	profile, _ := huaweiProfileByID(HuaweiSUN2000M1MonitoringProfileID)
	gateway := card.UnitID()
	loss := []HuaweiProviderLoss{
		{Kind: HuaweiProviderLossPlannedGroup, HasUnit: true, UnitID: gateway, Offset: 37498, Quantity: 8, Detail: "dongle plant power group: planned until an approved observation records its answer"},
		{Kind: HuaweiProviderLossPlannedGroup, HasUnit: true, UnitID: gateway, Offset: 37516, Quantity: 2, Detail: "dongle plant AC power group: planned until an approved observation records its answer"},
		{Kind: HuaweiProviderLossMeter, HasUnit: true, UnitID: card.ChildUnit(), Offset: 37100, Quantity: 39, Detail: "inverter-exposed meter group: planned until a meter class contract exists"},
	}
	loss = append(loss, huaweiChildProfileLoss(profile)...)
	core, err := newHuaweiProviderCore(HuaweiProviderPathSDongle, HuaweiSDongleReadOnlyProviderID, config, disposition, runHuaweiSDongleCycle, loss)
	if err != nil {
		return nil, err
	}
	return &HuaweiSDongleReadOnlyProvider{core: core}, nil
}

// Start binds the first connection generation.
func (p *HuaweiSDongleReadOnlyProvider) Start(session HuaweiProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.start(session)
}

// Stop closes the bound connection and fences a running refresh.
func (p *HuaweiSDongleReadOnlyProvider) Stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.stop()
}

// Reconnect closes the bound connection first and binds the new one as a new
// connection generation, whose first request may take one extra attempt.
func (p *HuaweiSDongleReadOnlyProvider) Reconnect(session HuaweiProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.reconnect(session)
}

// Close is terminal: it closes the bound connection and fences late results.
func (p *HuaweiSDongleReadOnlyProvider) Close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.close()
}

// Refresh runs one cycle under the same refusals as the SmartLogger provider.
func (p *HuaweiSDongleReadOnlyProvider) Refresh(ctx context.Context) (HuaweiProviderCycle, error) {
	if p == nil {
		return HuaweiProviderCycle{}, ErrHuaweiProviderConfig
	}
	return p.core.refresh(ctx)
}

// LastComplete returns a detached copy of the last complete cycle.
func (p *HuaweiSDongleReadOnlyProvider) LastComplete() (HuaweiProviderCycle, bool) {
	if p == nil {
		return HuaweiProviderCycle{}, false
	}
	return p.core.lastComplete()
}

// NextRefreshAt is the earliest start of the next cycle.
func (p *HuaweiSDongleReadOnlyProvider) NextRefreshAt() time.Time { return p.core.nextRefreshAt() }

// Pacing returns the pacing envelope of the S-Dongle path.
func (p *HuaweiSDongleReadOnlyProvider) Pacing() HuaweiProviderPacing { return p.core.pacing.clone() }

// Enablement returns the gate decision the provider was constructed under.
func (p *HuaweiSDongleReadOnlyProvider) Enablement() HuaweiProviderEnablement {
	enablement := p.core.enablement
	enablement.Detail = append([]string(nil), enablement.Detail...)
	enablement.Profiles = append([]string(nil), enablement.Profiles...)
	return enablement
}

func runHuaweiSDongleCycle(r *huaweiCycleRun) {
	defer func() { r.out.Gateway = r.state.gateway }()
	s := r.state
	card := NewHuaweiSDongleObservedQualificationCard()
	guard, result := r.readHolding(card.UnitID(), huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity)
	switch result.status {
	case huaweiReadStopped:
		return
	case huaweiReadAnswered:
	default:
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleIncomplete, HuaweiProviderReasonGuardFailed
		if s.identified {
			s.gateway.State = HuaweiProviderGatewayUnconfirmed
		} else {
			s.gateway.State = HuaweiProviderGatewayInsufficient
			r.out.Outcome = HuaweiProviderCycleInsufficient
		}
		s.reidentify = true
		if result.status == huaweiReadFailed {
			s.endpointFailures++
			r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
		}
		r.staleChildren(HuaweiProviderReasonListingUnconfirmed, false)
		return
	}
	if s.identified && !s.reidentify && s.identityGeneration == r.generation {
		if len(s.guard) == huaweiSDongleSearchQuantity && guard[1] == huaweiSDongleObservedSearchState && guard[2] == s.guard[2] {
			s.endpointFailures = 0
			s.gateway.State = HuaweiProviderGatewayIdentified
			r.out.Reason = HuaweiProviderReasonGuardHeld
			r.readSDongleChild()
			if !r.stopped() {
				r.out.Outcome = HuaweiProviderCycleComplete
			}
			return
		}
		s.reidentify = true
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleIncomplete, HuaweiProviderReasonGuardChanged
		r.staleChildren(HuaweiProviderReasonListingUnconfirmed, false)
		return
	}
	r.identifySDongle(guard)
}

// identifySDongle reads the rest of the observed card's tuple on this
// connection generation. The guard read of the cycle is the search sequence
// before the child reads; the same read after them closes the bracket.
func (r *huaweiCycleRun) identifySDongle(before []uint16) {
	s := r.state
	card := NewHuaweiSDongleObservedQualificationCard()
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

	input := HuaweiSDongleObservedQualificationInput{Gateway: HuaweiSDongleObservedGatewayReads{UnitID: gateway, SearchBefore: before}}
	basic, result := r.readIdentification(gateway, HuaweiIdentificationRequest{ReadDeviceIDCode: 1})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	switch result.status {
	case huaweiReadException:
		input.Gateway.Basic.ExceptionCode = result.exceptionCode
	case huaweiReadAnswered:
		input.Gateway.Basic.Reply = &HuaweiSDongleObservedBasicReply{Objects: basic.Objects}
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
	model, halted := words(childUnit, huaweiSUN2000ModelOffset, huaweiSUN2000ModelQuantity)
	if halted {
		r.sdongleIdentityFailed(failed)
		return
	}
	input.Child = HuaweiSDongleObservedChildReads{UnitID: childUnit, ModelBlock: huaweiWordsToBytes(model)}
	childBasic, result := r.readIdentification(childUnit, HuaweiIdentificationRequest{ReadDeviceIDCode: 1})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	if result.status == huaweiReadAnswered {
		input.Child.Basic = &HuaweiSDongleObservedBasicReply{Objects: childBasic.Objects}
	}
	inventory, result := r.readIdentification(childUnit, HuaweiIdentificationRequest{ReadDeviceIDCode: 3, ObjectID: huaweiSmartLoggerInventoryStartObject})
	if stop(result) {
		r.sdongleIdentityFailed(failed)
		return
	}
	if result.status == huaweiReadAnswered {
		input.Child.Inventory = HuaweiSDongleObservedChildInventoryPage(inventory)
	}
	if input.Gateway.SearchAfter, halted = words(gateway, huaweiSDongleSearchOffset, huaweiSDongleSearchQuantity); halted {
		r.sdongleIdentityFailed(failed)
		return
	}

	evaluation := card.Evaluate(input)
	if !evaluation.IdentityMatched() || !evaluation.QualificationTestReady() {
		// A gateway mismatch withholds the instance.
		s.identified, s.reidentify = false, true
		s.gateway = HuaweiProviderGatewayIdentity{CardID: card.ID(), Class: card.CandidateClass(), UnitID: gateway,
			State: HuaweiProviderGatewayInsufficient, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd}
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleInsufficient, HuaweiProviderReasonGatewayMismatch
		r.staleChildren(HuaweiProviderReasonGatewayMismatch, true)
		return
	}
	s.endpointFailures = 0
	s.identified, s.identityGeneration, s.reidentify = true, r.generation, false
	s.guard = append([]uint16(nil), input.Gateway.SearchAfter...)
	s.sdongle = &input
	s.gateway = HuaweiProviderGatewayIdentity{
		CardID: card.ID(), Class: card.CandidateClass(), UnitID: gateway, ChildModel: card.ChildModel(),
		State: HuaweiProviderGatewayIdentified, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd,
	}
	child := s.child(childUnit)
	child.listedClass = card.ChildFamily()
	child.definitive, child.skipUntil = "", 0
	selection := SelectHuaweiSUN2000MonitoringProfile(HuaweiSUN2000CapabilityInput{SDongle: s.sdongle, UnitID: childUnit, Document: huaweiSUN2000MCEdition})
	r.out.Reason = HuaweiProviderReasonIdentified
	if !r.bindChildSelection(child, selection) {
		r.out.Outcome = HuaweiProviderCycleComplete
		return
	}
	state, reason, current := r.monitorChild(child)
	if r.stopped() || state == "" {
		return
	}
	r.out.Children = append(r.out.Children, r.childObservation(child, state, reason, current))
	r.out.Outcome = HuaweiProviderCycleComplete
}

func (r *huaweiCycleRun) sdongleIdentityFailed(failed bool) {
	if r.stopped() {
		return
	}
	s := r.state
	s.identified, s.reidentify = false, true
	s.gateway.State = HuaweiProviderGatewayInsufficient
	r.out.Outcome, r.out.Reason = HuaweiProviderCycleInsufficient, HuaweiProviderReasonGatewayReadFail
	if failed {
		s.endpointFailures++
		r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
	}
	r.staleChildren(HuaweiProviderReasonGatewayReadFail, false)
}

// readSDongleChild reads the identified child's groups after a held guard.
func (r *huaweiCycleRun) readSDongleChild() {
	card := NewHuaweiSDongleObservedQualificationCard()
	child := r.state.child(card.ChildUnit())
	switch {
	case child.identity == nil || child.identifiedGeneration != r.generation:
		r.out.Children = append(r.out.Children, r.childObservation(child, HuaweiProviderEvidenceWithheld, huaweiFirstReason(child.definitive, HuaweiProviderReasonIdentityInsuff), nil))
		return
	case child.skipUntil > r.cycle:
		r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), HuaweiProviderReasonChildBackoff, nil))
		return
	}
	state, reason, current := r.monitorChild(child)
	if r.stopped() || state == "" {
		return
	}
	r.out.Children = append(r.out.Children, r.childObservation(child, state, reason, current))
}

func huaweiFirstReason(values ...HuaweiProviderReason) HuaweiProviderReason {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
