package huawei

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// SmartLoggerReadOnlyProviderID identifies the read-only provider of the
// observed SmartLogger path.
const SmartLoggerReadOnlyProviderID = "huawei.smartlogger.observed-readonly-provider@1.0.0"

const (
	huaweiSmartLoggerCounterOffset = 65521
	huaweiSUN2000ClassLabel        = "SUN2000"
)

// SmartLoggerReadOnlyProvider reads the observed SmartLogger path over a
// supplied session: the counter guard and the extended inventory at unit 0
// under the observed card, the admission of each listed and allowed SUN2000
// unit (the routed model read, the model identifier, the software text and the
// firmware text, read on each identification event only), and the M3 series
// monitoring groups at each child whose catalog decision selects the M3
// series row. The PLC, meter and every other unit without a class contract are
// never read. Do not copy it.
type SmartLoggerReadOnlyProvider struct {
	core *huaweiProviderCore
}

// NewSmartLoggerReadOnlyProvider constructs the provider behind the
// enablement gate of the recorded SmartLogger disposition.
func NewSmartLoggerReadOnlyProvider(config ProviderConfig) (*SmartLoggerReadOnlyProvider, error) {
	disposition, _ := RecordedProviderDisposition(ProviderPathSmartLogger)
	return newHuaweiSmartLoggerReadOnlyProvider(config, disposition)
}

func newHuaweiSmartLoggerReadOnlyProvider(config ProviderConfig, disposition ProviderDisposition) (*SmartLoggerReadOnlyProvider, error) {
	profile, _ := huaweiProfileByID(SUN2000M3MonitoringProfileID)
	loss := []ProviderLoss{
		{Kind: ProviderLossPlannedGroup, HasUnit: true, UnitID: 0, Offset: 40521, Quantity: 6, Detail: "logger plant power group: planned until an approved observation records its answer"},
		{Kind: ProviderLossPlannedGroup, HasUnit: true, UnitID: 0, Offset: 40560, Quantity: 4, Detail: "logger plant energy group: planned until an approved observation records its answer"},
	}
	loss = append(loss, huaweiChildProfileLoss(profile)...)
	core, err := newHuaweiProviderCore(ProviderPathSmartLogger, SmartLoggerReadOnlyProviderID, config, disposition, runHuaweiSmartLoggerCycle, loss)
	if err != nil {
		return nil, err
	}
	return &SmartLoggerReadOnlyProvider{core: core}, nil
}

// Start binds the first connection generation.
func (p *SmartLoggerReadOnlyProvider) Start(session ProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.start(session)
}

// Stop closes the bound connection and fences a running refresh. Retained
// readings stay as history; Start binds a new connection generation.
func (p *SmartLoggerReadOnlyProvider) Stop() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.stop()
}

// Reconnect closes the bound connection first and binds the new one as a new
// connection generation. The gateway and every child are identified again on
// it before any child reading is taken.
func (p *SmartLoggerReadOnlyProvider) Reconnect(session ProviderSession) error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.reconnect(session)
}

// Close is terminal: it closes the bound connection and fences late results.
func (p *SmartLoggerReadOnlyProvider) Close() error {
	if p == nil {
		return ErrHuaweiProviderConfig
	}
	return p.core.close()
}

// Refresh runs one cycle. It refuses a concurrent refresh, a refresh before
// NextRefreshAt and a refresh without a bound session, and sends nothing then.
func (p *SmartLoggerReadOnlyProvider) Refresh(ctx context.Context) (ProviderCycle, error) {
	if p == nil {
		return ProviderCycle{}, ErrHuaweiProviderConfig
	}
	return p.core.refresh(ctx)
}

// LastComplete returns a detached copy of the last complete cycle.
func (p *SmartLoggerReadOnlyProvider) LastComplete() (ProviderCycle, bool) {
	if p == nil {
		return ProviderCycle{}, false
	}
	return p.core.lastComplete()
}

// NextRefreshAt is the earliest start of the next cycle.
func (p *SmartLoggerReadOnlyProvider) NextRefreshAt() time.Time { return p.core.nextRefreshAt() }

// Pacing returns the pacing envelope of the SmartLogger path.
func (p *SmartLoggerReadOnlyProvider) Pacing() ProviderPacing {
	return p.core.pacing.clone()
}

// Enablement returns the gate decision the provider was constructed under.
func (p *SmartLoggerReadOnlyProvider) Enablement() ProviderEnablement {
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
				s.gateway.State = ProviderGatewayIdentified
				r.out.Reason = ProviderReasonGuardHeld
				r.readSmartLoggerChildren()
				if !r.stopped() {
					r.out.Outcome = ProviderCycleComplete
				}
				return
			}
			s.reidentify = true
			r.out.Outcome, r.out.Reason = ProviderCycleIncomplete, ProviderReasonGuardChanged
		default:
			s.reidentify = true
			s.gateway.State = ProviderGatewayUnconfirmed
			r.out.Outcome, r.out.Reason = ProviderCycleIncomplete, ProviderReasonGuardFailed
			if result.status == huaweiReadFailed {
				s.endpointFailures++
				r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
			}
		}
		r.staleChildren(ProviderReasonListingUnconfirmed, false)
		return
	}
	r.identifySmartLogger()
}

// identifySmartLogger runs the observed card's procedure: the counter, the
// extended inventory from object 0x87, the counter again and the optional
// basic identification, all at unit 0 on this connection generation.
func (r *huaweiCycleRun) identifySmartLogger() {
	s := r.state
	card := NewSmartLoggerObservedQualificationCard()
	gatewayFailed := func(result huaweiReadResult) {
		s.identified = false
		s.gateway.State, s.gateway.CardReason = ProviderGatewayInsufficient, ""
		r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayReadFail
		if result.status == huaweiReadFailed {
			s.endpointFailures++
			r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
		}
		r.staleChildren(ProviderReasonGatewayReadFail, false)
	}
	listingUnstable := func() {
		s.unstable++
		r.out.Outcome, r.out.Reason = ProviderCycleIncomplete, ProviderReasonListingUnstable
		if s.unstable >= 3 {
			s.unstable = 0
			r.out.Reason = ProviderReasonListingBackoff
			r.waitCycles = 10
		}
		r.staleChildren(ProviderReasonListingUnstable, false)
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
	request := IdentificationRequest{ReadDeviceIDCode: 3, ObjectID: huaweiSmartLoggerInventoryStartObject}
	var pages []SmartLoggerObservedInventoryPage
	wrapped := false
	r.budgetEnd = bracketStart.Add(time.Duration(limits.DeadlineMilliseconds) * time.Millisecond)
	defer func() { r.budgetEnd = time.Time{} }()
	for {
		page, result := r.readIdentification(0, request)
		if result.status == huaweiReadStopped {
			return
		}
		if result.overDeadline {
			listingUnstable()
			return
		}
		if result.status != huaweiReadAnswered {
			gatewayFailed(result)
			return
		}
		pages = append(pages, SmartLoggerObservedInventoryPage{
			ResponseBytes: page.ResponseBytes, DeclaredObjects: page.DeclaredObjects, More: page.More, NextObjectID: page.NextObjectID, Objects: page.Objects,
		})
		if r.link.lastEnd.Sub(bracketStart) > time.Duration(limits.DeadlineMilliseconds)*time.Millisecond {
			listingUnstable()
			return
		}
		if !page.More {
			break
		}
		// The continuation rule of the observed inventory parser: ascending,
		// with one bounded wrap past the last object identifier.
		if page.NextObjectID <= request.ObjectID {
			if wrapped || page.NextObjectID == request.ObjectID {
				listingUnstable()
				return
			}
			wrapped = true
		}
		if len(pages) >= int(limits.MaxPages) {
			listingUnstable()
			return
		}
		request.ObjectID = page.NextObjectID
	}
	after, result := r.readHolding(0, huaweiSmartLoggerCounterOffset, 1)
	r.budgetEnd = time.Time{}
	if result.status == huaweiReadStopped {
		return
	}
	if result.overDeadline {
		listingUnstable()
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
	var basic *SmartLoggerObservedBasicReply
	page, result := r.readIdentification(0, IdentificationRequest{ReadDeviceIDCode: 1})
	if result.status == huaweiReadStopped {
		return
	}
	if result.status == huaweiReadAnswered {
		basic = &SmartLoggerObservedBasicReply{Objects: page.Objects}
	}
	input := SmartLoggerObservedQualificationInput{
		UnitID:    card.UnitID(),
		Inventory: SmartLoggerObservedInventoryInput{ChangeCounterBefore: before[0], ChangeCounterAfter: after[0], Pages: pages},
		Basic:     basic,
	}
	evaluation := card.Evaluate(input)
	inventory, ok := evaluation.Inventory()
	if !evaluation.IdentityMatched() || !evaluation.QualificationTestReady() || !ok {
		if evaluation.Reason() == QualificationInventoryInvalid {
			listingUnstable()
			return
		}
		// A gateway mismatch withholds the instance: no child is read and no
		// child emits a fact.
		s.identified, s.gatewayFenced = false, true
		s.gateway = ProviderGatewayIdentity{CardID: card.ID(), Class: card.CandidateClass(), UnitID: card.UnitID(),
			State: ProviderGatewayInsufficient, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd}
		r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayMismatch
		r.staleChildren(ProviderReasonGatewayMismatch, true)
		return
	}
	s.unstable, s.endpointFailures = 0, 0
	s.identified, s.identityGeneration, s.reidentify, s.gatewayFenced = true, r.generation, false, false
	s.guard = []uint16{after[0]}
	s.smartLogger = &input
	// The identity publishes the software text that was read, which may be a
	// build above the card's observed one.
	software := card.Firmware()
	if selection, selected := evaluation.Selection(); selected && selection.Build() != "" {
		software = selection.Build()
	}
	s.listing = inventory.Children()
	s.gateway = ProviderGatewayIdentity{
		CardID: card.ID(), Class: card.CandidateClass(), UnitID: card.UnitID(), Model: card.Model(), Firmware: software,
		State: ProviderGatewayIdentified, CardReason: evaluation.Reason(), Generation: r.generation, IdentifiedAt: r.link.lastEnd,
	}
	// An identification event: every child is identified again at its routed
	// unit before it counts as identified.
	for _, child := range s.children {
		child.identificationEvent()
	}
	r.out.Reason = ProviderReasonIdentified
	r.readSmartLoggerChildren()
	if !r.stopped() {
		r.out.Outcome = ProviderCycleComplete
	}
}

// readSmartLoggerChildren handles the union of the validated listing and the
// operator allowlist in ascending unit order. Only listed and allowed units
// whose listing label names the SUN2000 class are read.
func (r *huaweiCycleRun) readSmartLoggerChildren() {
	s := r.state
	listed := map[uint8]GatewayInventoryChild{}
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
	for unit, child := range s.children {
		if !units[unit] {
			child.ineligible = ProviderReasonNotListed
		}
	}
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
		child.ineligible = ""
		withhold := func(reason ProviderReason) {
			r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, reason, nil))
		}
		switch {
		case !isListed:
			child.ineligible = ProviderReasonNotListed
			withhold(ProviderReasonNotListed)
			continue
		case !allowed[unit]:
			withhold(ProviderReasonNotAllowlisted)
			continue
		case entry.Attribute("product_type") != huaweiSUN2000ClassLabel:
			child.ineligible = ProviderReasonNoClassContract
			withhold(ProviderReasonNoClassContract)
			continue
		case child.definitive != "":
			withhold(child.definitive)
			continue
		case child.skipUntil > r.cycle:
			r.out.Children = append(r.out.Children, r.childObservation(child, huaweiStaleOrWithheld(child), ProviderReasonChildBackoff, nil))
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

func huaweiStaleOrWithheld(child *huaweiChildState) ProviderEvidenceState {
	if child.identity != nil && child.reading.ProfileID() != "" {
		return ProviderEvidenceStale
	}
	return ProviderEvidenceWithheld
}

// identifySmartLoggerChild runs the child's admission at its routed unit: the
// model read, then the model identifier, the software text and the firmware
// text, and the SUN2000 capability selection over them, whose catalog
// decision admits or withholds the child. It reports whether the child is
// identified on this connection generation; otherwise it has already added the
// child's observation, or the cycle was stopped.
func (r *huaweiCycleRun) identifySmartLoggerChild(child *huaweiChildState) bool {
	model, result := r.readHolding(child.unit, huaweiSUN2000ModelOffset, huaweiSUN2000ModelQuantity)
	if !r.childAdmissionRead(child, result) {
		return false
	}
	input := SUN2000CapabilityInput{
		SmartLogger: r.state.smartLogger, UnitID: child.unit, RoutedModelBlock: huaweiWordsToBytes(model),
	}
	// A routed model that contradicts the listing or is not text withholds the
	// child before any further read, so a later failed read cannot keep the
	// old identity.
	if early := SelectHuaweiSUN2000MonitoringProfile(input); early.Reason() == SUN2000ReasonListingModelConflict ||
		early.Reason() == SUN2000ReasonModelBlockInvalid {
		return r.bindChildSelection(child, early)
	}
	identity, ok := r.readChildIdentityWords(child)
	if !ok {
		return false
	}
	input.Identity = identity
	selection := SelectHuaweiSUN2000MonitoringProfile(input)
	return r.bindChildSelection(child, selection)
}
