package modbusreg

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// HuaweiSmartLoggerReadOnlyProviderID identifies the read-only provider of the
// observed SmartLogger path.
const HuaweiSmartLoggerReadOnlyProviderID = "huawei.smartlogger.observed-readonly-provider@1.0.0"

const (
	huaweiSmartLoggerCounterOffset = 65521
	huaweiSUN2000ClassLabel        = "SUN2000"
)

// HuaweiSmartLoggerReadOnlyProvider reads the observed SmartLogger path over a
// supplied session: the counter guard and the extended inventory at unit 0
// under the observed card, the routed model read at each listed and allowed
// SUN2000 unit, and the M3 series monitoring groups at each child whose
// capability selection admits the profile. The PLC, meter and every other unit
// without a class contract are never read. Do not copy it.
type HuaweiSmartLoggerReadOnlyProvider struct {
	core *huaweiProviderCore
}

// NewHuaweiSmartLoggerReadOnlyProvider constructs the provider behind the
// enablement gate of the recorded SmartLogger disposition.
func NewHuaweiSmartLoggerReadOnlyProvider(config HuaweiProviderConfig) (*HuaweiSmartLoggerReadOnlyProvider, error) {
	disposition, _ := HuaweiRecordedProviderDisposition(HuaweiProviderPathSmartLogger)
	return newHuaweiSmartLoggerReadOnlyProvider(config, disposition)
}

func newHuaweiSmartLoggerReadOnlyProvider(config HuaweiProviderConfig, disposition HuaweiProviderDisposition) (*HuaweiSmartLoggerReadOnlyProvider, error) {
	profile, _ := huaweiProfileByID(HuaweiSUN2000M3MonitoringProfileID)
	loss := []HuaweiProviderLoss{
		{Kind: HuaweiProviderLossPlannedGroup, HasUnit: true, UnitID: 0, Offset: 40521, Quantity: 6, Detail: "logger plant power group: planned until an approved observation records its answer"},
		{Kind: HuaweiProviderLossPlannedGroup, HasUnit: true, UnitID: 0, Offset: 40560, Quantity: 4, Detail: "logger plant energy group: planned until an approved observation records its answer"},
	}
	loss = append(loss, huaweiChildProfileLoss(profile)...)
	core, err := newHuaweiProviderCore(HuaweiProviderPathSmartLogger, HuaweiSmartLoggerReadOnlyProviderID, config, disposition, runHuaweiSmartLoggerCycle, loss)
	if err != nil {
		return nil, err
	}
	return &HuaweiSmartLoggerReadOnlyProvider{core: core}, nil
}

// Start binds the first connection generation.
func (p *HuaweiSmartLoggerReadOnlyProvider) Start(session HuaweiProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.start(session)
}

// Stop closes the bound connection and fences a running refresh. Retained
// readings stay as history; Start binds a new connection generation.
func (p *HuaweiSmartLoggerReadOnlyProvider) Stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.stop()
}

// Reconnect closes the bound connection first and binds the new one as a new
// connection generation. The gateway and every child are identified again on
// it before any child reading is taken.
func (p *HuaweiSmartLoggerReadOnlyProvider) Reconnect(session HuaweiProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.reconnect(session)
}

// Close is terminal: it closes the bound connection and fences late results.
func (p *HuaweiSmartLoggerReadOnlyProvider) Close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.close()
}

// Refresh runs one cycle. It refuses a concurrent refresh, a refresh before
// NextRefreshAt and a refresh without a bound session, and sends nothing then.
func (p *HuaweiSmartLoggerReadOnlyProvider) Refresh(ctx context.Context) (HuaweiProviderCycle, error) {
	if p == nil {
		return HuaweiProviderCycle{}, ErrHuaweiProviderConfig
	}
	return p.core.refresh(ctx)
}

// LastComplete returns a detached copy of the last complete cycle.
func (p *HuaweiSmartLoggerReadOnlyProvider) LastComplete() (HuaweiProviderCycle, bool) {
	if p == nil {
		return HuaweiProviderCycle{}, false
	}
	return p.core.lastComplete()
}

// NextRefreshAt is the earliest start of the next cycle.
func (p *HuaweiSmartLoggerReadOnlyProvider) NextRefreshAt() time.Time { return p.core.nextRefreshAt() }

// Pacing returns the pacing envelope of the SmartLogger path.
func (p *HuaweiSmartLoggerReadOnlyProvider) Pacing() HuaweiProviderPacing {
	return p.core.pacing.clone()
}

// Enablement returns the gate decision the provider was constructed under.
func (p *HuaweiSmartLoggerReadOnlyProvider) Enablement() HuaweiProviderEnablement {
	enablement := p.core.enablement
	enablement.Detail = append([]string(nil), enablement.Detail...)
	enablement.Profiles = append([]string(nil), enablement.Profiles...)
	return enablement
}

func runHuaweiSmartLoggerCycle(r *huaweiCycleRun) {
	defer func() { r.out.Gateway = r.state.gateway }()
	s := r.state
	if s.identified && !s.reidentify && s.identityGeneration == r.generation {
		words, result := r.readHolding(0, huaweiSmartLoggerCounterOffset, 1)
		switch result.status {
		case huaweiReadStopped:
			return
		case huaweiReadAnswered:
			if len(s.guard) == 1 && words[0] == s.guard[0] {
				s.endpointFailures = 0
				s.gateway.State = HuaweiProviderGatewayIdentified
				r.out.Reason = HuaweiProviderReasonGuardHeld
				r.readSmartLoggerChildren()
				if !r.stopped() {
					r.out.Outcome = HuaweiProviderCycleComplete
				}
				return
			}
			s.reidentify = true
			r.out.Outcome, r.out.Reason = HuaweiProviderCycleIncomplete, HuaweiProviderReasonGuardChanged
		default:
			s.gateway.State = HuaweiProviderGatewayUnconfirmed
			r.out.Outcome, r.out.Reason = HuaweiProviderCycleIncomplete, HuaweiProviderReasonGuardFailed
			if result.status == huaweiReadFailed {
				s.endpointFailures++
				r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
			}
		}
		r.staleChildren(HuaweiProviderReasonListingUnconfirmed, false)
		return
	}
	r.identifySmartLogger()
}

// identifySmartLogger runs the observed card's procedure: the counter, the
// extended inventory from object 0x87, the counter again and the optional
// basic identification, all at unit 0 on this connection generation.
func (r *huaweiCycleRun) identifySmartLogger() {
	s := r.state
	card := NewHuaweiSmartLoggerObservedQualificationCard()
	gatewayFailed := func(result huaweiReadResult) {
		s.identified = false
		s.gateway.State, s.gateway.CardReason = HuaweiProviderGatewayInsufficient, ""
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleInsufficient, HuaweiProviderReasonGatewayReadFail
		if result.status == huaweiReadFailed {
			s.endpointFailures++
			r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
		}
		r.staleChildren(HuaweiProviderReasonGatewayReadFail, false)
	}
	listingUnstable := func() {
		s.unstable++
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleIncomplete, HuaweiProviderReasonListingUnstable
		if s.unstable >= 3 {
			s.unstable = 0
			r.out.Reason = HuaweiProviderReasonListingBackoff
			r.waitCycles = 10
		}
		r.staleChildren(HuaweiProviderReasonListingUnstable, false)
	}

	before, result := r.readHolding(0, huaweiSmartLoggerCounterOffset, 1)
	if result.status == huaweiReadStopped {
		return
	}
	if result.status != huaweiReadAnswered {
		gatewayFailed(result)
		return
	}
	bracketStart := r.link.lastEnd
	limits := card.Limits()
	request := HuaweiIdentificationRequest{ReadDeviceIDCode: 3, ObjectID: huaweiSmartLoggerInventoryStartObject}
	var pages []HuaweiSmartLoggerObservedInventoryPage
	for {
		page, result := r.readIdentification(0, request)
		if result.status == huaweiReadStopped {
			return
		}
		if result.status != huaweiReadAnswered {
			gatewayFailed(result)
			return
		}
		pages = append(pages, HuaweiSmartLoggerObservedInventoryPage{
			ResponseBytes: page.ResponseBytes, DeclaredObjects: page.DeclaredObjects, More: page.More, NextObjectID: page.NextObjectID, Objects: page.Objects,
		})
		if !page.More {
			break
		}
		if len(pages) >= int(limits.MaxPages) || page.NextObjectID <= request.ObjectID {
			listingUnstable()
			return
		}
		request.ObjectID = page.NextObjectID
	}
	after, result := r.readHolding(0, huaweiSmartLoggerCounterOffset, 1)
	if result.status == huaweiReadStopped {
		return
	}
	if result.status != huaweiReadAnswered {
		gatewayFailed(result)
		return
	}
	if before[0] != after[0] || r.link.lastEnd.Sub(bracketStart) > time.Duration(limits.DeadlineMilliseconds)*time.Millisecond {
		listingUnstable()
		return
	}
	var basic *HuaweiSmartLoggerObservedBasicReply
	page, result := r.readIdentification(0, HuaweiIdentificationRequest{ReadDeviceIDCode: 1})
	if result.status == huaweiReadStopped {
		return
	}
	if result.status == huaweiReadAnswered {
		basic = &HuaweiSmartLoggerObservedBasicReply{Objects: page.Objects}
	}
	input := HuaweiSmartLoggerObservedQualificationInput{
		UnitID:    card.UnitID(),
		Inventory: HuaweiSmartLoggerObservedInventoryInput{ChangeCounterBefore: before[0], ChangeCounterAfter: after[0], Pages: pages},
		Basic:     basic,
	}
	evaluation := card.Evaluate(input)
	inventory, ok := evaluation.Inventory()
	if !evaluation.IdentityMatched() || !evaluation.QualificationTestReady() || !ok {
		if evaluation.Reason() == HuaweiQualificationInventoryInvalid {
			listingUnstable()
			return
		}
		// A gateway mismatch withholds the instance: no child is read and no
		// child emits a fact.
		s.identified = false
		s.gateway = HuaweiProviderGatewayIdentity{CardID: card.ID(), Class: card.CandidateClass(), UnitID: card.UnitID(),
			State: HuaweiProviderGatewayInsufficient, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd}
		r.out.Outcome, r.out.Reason = HuaweiProviderCycleInsufficient, HuaweiProviderReasonGatewayMismatch
		r.staleChildren(HuaweiProviderReasonGatewayMismatch, true)
		return
	}
	s.unstable, s.endpointFailures = 0, 0
	s.identified, s.identityGeneration, s.reidentify = true, r.generation, false
	s.guard = []uint16{after[0]}
	s.smartLogger = &input
	s.listing = inventory.Children()
	s.gateway = HuaweiProviderGatewayIdentity{
		CardID: card.ID(), Class: card.CandidateClass(), UnitID: card.UnitID(), Model: card.Model(), Firmware: card.Firmware(),
		State: HuaweiProviderGatewayIdentified, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd,
	}
	// An identification event: every child is identified again at its routed
	// unit before it counts as identified.
	for _, child := range s.children {
		child.definitive, child.identifiedGeneration, child.skipUntil = "", 0, 0
	}
	r.out.Reason = HuaweiProviderReasonIdentified
	r.readSmartLoggerChildren()
	if !r.stopped() {
		r.out.Outcome = HuaweiProviderCycleComplete
	}
}

// readSmartLoggerChildren handles the union of the validated listing and the
// operator allowlist in ascending unit order. Only listed and allowed units
// whose listing label names the SUN2000 class are read.
func (r *huaweiCycleRun) readSmartLoggerChildren() {
	s := r.state
	listed := map[uint8]HuaweiGatewayInventoryChild{}
	units := map[uint8]bool{}
	for _, entry := range s.listing {
		unit, err := strconv.Atoi(entry.Address())
		if err != nil || unit <= 0 || unit > huaweiSUN2000MaxUnit {
			continue
		}
		listed[uint8(unit)] = entry
		units[uint8(unit)] = true
	}
	allowed := map[uint8]bool{}
	for _, unit := range r.core.allowed {
		allowed[unit] = true
		units[unit] = true
	}
	ordered := make([]int, 0, len(units))
	for unit := range units {
		ordered = append(ordered, int(unit))
	}
	sort.Ints(ordered)
	for _, value := range ordered {
		unit := uint8(value)
		entry, isListed := listed[unit]
		child := s.child(unit)
		child.listedClass = ""
		if isListed {
			child.listedClass = entry.Attribute("product_type")
			if child.listedClass == "" {
				child.listedClass = entry.Attribute("model")
			}
		}
		withhold := func(reason HuaweiProviderReason) {
			r.out.Children = append(r.out.Children, r.childObservation(child, HuaweiProviderEvidenceWithheld, reason, nil))
		}
		switch {
		case !isListed:
			withhold(HuaweiProviderReasonNotListed)
			continue
		case !allowed[unit]:
			withhold(HuaweiProviderReasonNotAllowlisted)
			continue
		case entry.Attribute("product_type") != huaweiSUN2000ClassLabel:
			withhold(HuaweiProviderReasonNoClassContract)
			continue
		case child.definitive != "":
			withhold(child.definitive)
			continue
		case child.skipUntil > r.cycle:
			r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), HuaweiProviderReasonChildBackoff, nil))
			continue
		}
		if child.identifiedGeneration != r.generation || child.identity == nil {
			if !r.identifySmartLoggerChild(child) {
				if r.stopped() {
					return
				}
				continue
			}
		}
		state, reason, current := r.monitorChild(child)
		if r.stopped() || state == "" {
			return
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, state, reason, current))
	}
}

func huaweiStaleOrWithheld(child *huaweiChildState) HuaweiProviderEvidenceState {
	if child.identity != nil && child.reading.ProfileID() != "" {
		return HuaweiProviderEvidenceStale
	}
	return HuaweiProviderEvidenceWithheld
}

// identifySmartLoggerChild reads the routed model at the child's unit and
// runs the SUN2000 capability selection. It reports whether the child is
// identified on this connection generation; otherwise it has already added the
// child's observation.
func (r *huaweiCycleRun) identifySmartLoggerChild(child *huaweiChildState) bool {
	s := r.state
	words, result := r.readHolding(child.unit, huaweiSUN2000ModelOffset, huaweiSUN2000ModelQuantity)
	switch result.status {
	case huaweiReadStopped:
		return false
	case huaweiReadFailed:
		child.failures++
		child.skipUntil = r.cycle + huaweiChildBackoffCycles(child.failures)
		reason := HuaweiProviderReasonChildReadFailed
		if result.last == HuaweiProviderExchangeTimeout {
			reason = HuaweiProviderReasonChildTimeout
		}
		r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), reason, nil))
		return false
	case huaweiReadException:
		child.definitive = HuaweiProviderReasonIdentityInsuff
		child.fence()
		child.identity = nil
		r.out.Children = append(r.out.Children, r.childObservation(child, HuaweiProviderEvidenceWithheld, child.definitive, nil))
		return false
	}
	selection := SelectHuaweiSUN2000MonitoringProfile(HuaweiSUN2000CapabilityInput{
		SmartLogger: s.smartLogger, UnitID: child.unit, RoutedModelBlock: huaweiWordsToBytes(words), Document: huaweiSUN2000MCEdition,
	})
	return r.bindChildSelection(child, selection)
}

// bindChildSelection binds a selected profile to the child. A changed identity
// fences the earlier reading; anything other than one selected profile
// withholds the child until the next identification event.
func (r *huaweiCycleRun) bindChildSelection(child *huaweiChildState, selection HuaweiSUN2000CapabilitySelection) bool {
	profile, ok := selection.Profile()
	if selection.Outcome() == HuaweiSUN2000ProfileSelected && ok {
		identity := &huaweiChildIdentity{profile: profile, model: profile.Model()}
		if child.identity != nil && child.identity.key() != identity.key() {
			child.fence()
		}
		child.identity, child.identifiedGeneration, child.definitive = identity, r.generation, ""
		return true
	}
	switch {
	case selection.Outcome() == HuaweiSUN2000NoAdmissibleProfile:
		child.definitive = HuaweiProviderReasonNoProfile
	case selection.Reason() == HuaweiSUN2000ReasonListingModelConflict:
		child.definitive = HuaweiProviderReasonIdentityMismatch
	default:
		child.definitive = HuaweiProviderReasonIdentityInsuff
	}
	child.fence()
	child.identity = nil
	r.out.Children = append(r.out.Children, r.childObservation(child, HuaweiProviderEvidenceWithheld, child.definitive, nil))
	return false
}
