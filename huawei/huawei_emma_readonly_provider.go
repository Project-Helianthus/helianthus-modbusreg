package huawei

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"time"
)

// EMMAReadOnlyProviderID identifies the provider of the EMMA site groups.
const EMMAReadOnlyProviderID = "huawei.emma.readonly-provider@1.0.0"

// EMMAReadOnlyProvider reads the site's unit 0 groups and routed inverter
// groups. The caller supplies the session. The detached identity-only seam
// does not send or authorize any of these reads. Do not copy the provider.
type EMMAReadOnlyProvider struct {
	core *huaweiProviderCore
}

// NewEMMAReadOnlyProvider constructs the provider for the declared EMMA path.
func NewEMMAReadOnlyProvider(config ProviderConfig) (*EMMAReadOnlyProvider, error) {
	disposition, _ := RecordedProviderDisposition(ProviderPathEMMA)
	core, err := newHuaweiProviderCore(ProviderPathEMMA, EMMAReadOnlyProviderID, config, disposition, runHuaweiEMMACycle, nil)
	if err != nil {
		return nil, err
	}
	return &EMMAReadOnlyProvider{core: core}, nil
}

// Start binds the first connection generation.
func (p *EMMAReadOnlyProvider) Start(session ProviderSession) error { return p.core.start(session) }

// Stop closes the connection and fences a running refresh.
func (p *EMMAReadOnlyProvider) Stop() error { return p.core.stop() }

// Reconnect replaces the connection. Gateway and routed identities must be
// read again before monitoring on the new connection.
func (p *EMMAReadOnlyProvider) Reconnect(session ProviderSession) error {
	return p.core.reconnect(session)
}

// Close terminates the provider and fences late results.
func (p *EMMAReadOnlyProvider) Close() error { return p.core.close() }

// Refresh runs one paced read-only cycle.
func (p *EMMAReadOnlyProvider) Refresh(ctx context.Context) (ProviderCycle, error) {
	return p.core.refresh(ctx)
}

// NextRefreshAt is the earliest start of the next cycle.
func (p *EMMAReadOnlyProvider) NextRefreshAt() time.Time { return p.core.nextRefreshAt() }

// LastComplete returns a detached copy of the last complete cycle.
func (p *EMMAReadOnlyProvider) LastComplete() (ProviderCycle, bool) { return p.core.lastComplete() }

func runHuaweiEMMACycle(r *huaweiCycleRun) {
	s := r.state
	defer func() { r.out.Gateway = s.gateway }()
	if (!s.identified && !r.identifyEMMA()) || (s.listing == nil && !r.acquireEMMAListing()) {
		if !r.stopped() {
			r.readEMMASite(false)
			r.staleChildren(r.out.Reason, true)
		}
		return
	}
	r.out.Reason = ProviderReasonIdentified
	r.readEMMASite(true)
	if !r.stopped() {
		r.readEMMAChildren()
	}
	if !r.stopped() {
		r.out.Outcome = ProviderCycleComplete
	}
}

func (r *huaweiCycleRun) readEMMASite(identified bool) {
	for i, extent := range huaweiEMMASite {
		kept := &r.state.emmaSite[i]
		outcome, code := SUN2000GroupNotRead, uint8(0)
		if identified {
			outcome, code = r.readGatewayGroup(0, extent.offset, extent.quantity, kept)
		}
		if r.stopped() {
			return
		}
		group := extent.observation(*kept, outcome, code)
		r.out.EMMASite = append(r.out.EMMASite, group)
		r.recordGatewayGroupLoss(group.UnitID, group.ProviderGroupEvidence, "site group did not answer")
	}
	r.readMeterGroups(0, huaweiEMMAMeter[:], r.state.emmaMeter[:], MeterPolarityUnknown, identified)
}

func (r *huaweiCycleRun) identifyEMMA() bool {
	s := r.state
	card := NewEMMAObservedQualificationCard()
	var fields [3]string
	for index, step := range card.Steps()[:3] {
		words, result := r.readHolding(card.UnitID(), step.Offset(), step.Quantity())
		if result.status != huaweiReadAnswered {
			if !r.stopped() {
				s.gateway.State = ProviderGatewayInsufficient
				r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayReadFail
				if result.status == huaweiReadFailed {
					s.endpointFailures++
					r.waitCycles = huaweiEndpointBackoffCycles(s.endpointFailures)
				}
			}
			return false
		}
		text, ok := parseHuaweiEMMAObservedGatewayBlock(huaweiWordsToBytes(words), 2*int(step.Quantity()))
		if !ok {
			return r.rejectEMMAIdentity()
		}
		fields[index] = text
	}
	decision := EvaluateHuaweiEMMAOfflineIdentity(fields[0], fields[1], fields[2])
	if fields[0] != card.FamilyLabel() || !decision.Matched() {
		return r.rejectEMMAIdentity()
	}
	model, build := decision.ModelVariant(), decision.Selection().Build()
	// Before the first committed identity, only refusals can have carried over.
	if s.gateway.Model != "" && (s.gateway.Model != model || s.gateway.Firmware != build) {
		r.clearEMMAReadings()
	}
	s.identified, s.identityGeneration, s.gatewayFenced, s.endpointFailures = true, r.generation, false, 0
	s.gateway = ProviderGatewayIdentity{CardID: card.ID(), Class: card.CandidateClass(), UnitID: card.UnitID(), Model: model,
		Firmware: build, State: ProviderGatewayIdentified, Generation: r.generation, IdentifiedAt: r.link.lastEnd}
	s.listing = nil
	for _, child := range s.children {
		child.definitive, child.identifiedGeneration, child.skipUntil = "", 0, 0
	}
	return true
}

func (r *huaweiCycleRun) acquireEMMAListing() bool {
	card := NewEMMAObservedQualificationCard()
	listing, ok := r.readEMMAListing(card)
	if r.stopped() {
		return false
	}
	if !ok {
		// Leave the listing unconfirmed for the next cycle; site identity still holds.
		return true
	}
	gateway := r.state.gateway
	if emmaSelfConflicts(listing.self, gateway.Model, gateway.Firmware, card.Product()) {
		return r.rejectEMMAIdentity()
	}
	// Unguarded listing routes only; the child must corroborate its own identity.
	r.state.listing = append([]GatewayInventoryChild{}, listing.inventory.Children()...)
	return true
}

func emmaSelfConflicts(self GatewayInventoryChild, model, build, product string) bool {
	listedBuild, listedProduct := self.Attribute("software_version"), self.Attribute("product_type")
	return self.Attribute("model") != model || (listedBuild != "" && listedBuild != build) || (listedProduct != "" && listedProduct != product)
}

func (r *huaweiCycleRun) rejectEMMAIdentity() bool {
	r.clearEMMAReadings()
	r.state.identified = false
	r.state.gateway = ProviderGatewayIdentity{State: ProviderGatewayInsufficient}
	r.state.gatewayFenced = true
	r.out.Outcome, r.out.Reason = ProviderCycleInsufficient, ProviderReasonGatewayMismatch
	return false
}

func (r *huaweiCycleRun) clearEMMAReadings() {
	s := r.state
	s.emmaSite = [len(huaweiEMMASite)]huaweiPlantState{}
	s.emmaMeter = huaweiMeterState{}
	s.children = map[uint8]*huaweiChildState{}
	s.listing, s.carriedRefusals = nil, nil
}

func (r *huaweiCycleRun) readEMMAListing(card EMMAObservedQualificationCard) (huaweiObservedInventory, bool) {
	limits := card.Limits()
	r.budgetEnd = r.core.clock().Add(time.Duration(limits.DeadlineMilliseconds) * time.Millisecond)
	defer func() { r.budgetEnd = time.Time{} }()
	request := IdentificationRequest{ReadDeviceIDCode: 3, ObjectID: huaweiSmartLoggerInventoryStartObject}
	var pages []EMMAObservedInventoryPage
	var objects, totalBytes int
	for len(pages) < int(limits.MaxPages) {
		page, result := r.readIdentification(card.UnitID(), request)
		if result.status != huaweiReadAnswered || page.Conformity != huaweiEMMAObservedConformity {
			break
		}
		objects += len(page.Objects)
		totalBytes += page.ResponseBytes
		next, valid := huaweiEMMAInventoryPageEnd(request.ObjectID, page)
		if !valid || page.ResponseBytes > int(limits.MaxResponseBytes) || objects > int(limits.MaxObjects) || totalBytes > int(limits.MaxBytes) {
			break
		}
		pages = append(pages, EMMAObservedInventoryPage(page))
		if !page.More {
			inventory, err := parseHuaweiEMMAObservedInventory(card.UnitID(), pages)
			return inventory, err == nil
		}
		request.ObjectID = next
	}
	return huaweiObservedInventory{}, false
}

func huaweiEMMAInventoryPageEnd(start uint8, page IdentificationPage) (uint8, bool) {
	next, size := start, huaweiMEIResponseHeaderBytes
	for _, object := range page.Objects {
		if object.ObjectID != next {
			return 0, false
		}
		next++
		size += 2 + len(object.Value)
	}
	if size != page.ResponseBytes || (page.More && (page.NextObjectID != next || next == start)) {
		return 0, false
	}
	return next, true
}

type huaweiEMMAChildPlan struct {
	model   string
	profile CatalogMonitoringProfile
	steps   []SUN2000ReadStep
}

func (r *huaweiCycleRun) readEMMAChildren() {
	listed := make(map[uint8]GatewayInventoryChild, len(r.state.listing))
	for _, entry := range r.state.listing {
		unit, _ := strconv.ParseUint(entry.Address(), 10, 8) // the inventory parser checked it
		listed[uint8(unit)] = entry
	}
	units := append([]uint8(nil), r.core.allowed...)
	for unit := range listed {
		if !slices.Contains(r.core.allowed, unit) {
			units = append(units, unit)
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i] < units[j] })
	for _, unit := range units {
		entry := listed[unit]
		child := r.state.child(unit)
		child.listedClass = entry.Attribute("product_type")
		switch {
		case !slices.Contains(r.core.allowed, unit):
			r.withholdEMMAChild(child, ProviderReasonNotAllowlisted, false)
		case r.state.listing == nil:
			r.withholdEMMAChild(child, ProviderReasonListingUnconfirmed, false)
		case entry.Address() == "":
			r.withholdEMMAChild(child, ProviderReasonNotListed, true)
		case child.listedClass != huaweiSUN2000ClassLabel:
			r.withholdEMMAChild(child, ProviderReasonNoClassContract, true)
		case child.definitive != "":
			r.withholdEMMAChild(child, child.definitive, false)
		case child.skipUntil > r.cycle:
			if child.emmaPlan != nil {
				r.monitorEMMAChild(child, ProviderReasonChildBackoff)
			} else {
				r.withholdEMMAChild(child, ProviderReasonChildBackoff, false)
			}
		default:
			if child.identifiedGeneration == r.generation || r.identifyEMMAChild(child, entry.Attribute("model")) {
				r.monitorEMMAChild(child, "")
			}
		}
		if r.stopped() {
			return
		}
	}
}

func (r *huaweiCycleRun) withholdEMMAChild(child *huaweiChildState, reason ProviderReason, fence bool) bool {
	if fence {
		fenceEMMAChild(child)
		child.definitive = reason
	}
	r.out.Children = append(r.out.Children, r.childObservation(child, ProviderEvidenceWithheld, reason, nil))
	return false
}

func (r *huaweiCycleRun) identifyEMMAChild(child *huaweiChildState, listedModel string) bool {
	words, result := r.readHolding(child.unit, huaweiSUN2000ModelOffset, huaweiSUN2000ModelQuantity)
	if !r.childAdmissionRead(child, result) {
		return false
	}
	model, _, valid := parseHuaweiEMMAObservedBlock(huaweiWordsToBytes(words), huaweiSUN2000ModelBlockBytes)
	if !valid {
		return r.withholdEMMAChild(child, ProviderReasonIdentityInsuff, true)
	}
	if model != listedModel {
		return r.withholdEMMAChild(child, ProviderReasonIdentityMismatch, true)
	}
	identity, ok := r.readChildIdentityWords(child)
	if !ok {
		return false
	}
	input, valid := huaweiSUN2000CatalogInputOf(model, identity)
	if !valid {
		return r.withholdEMMAChild(child, ProviderReasonIdentityInsuff, true)
	}
	decision := DecideHuaweiSUN2000CatalogSelection(input)
	plan := huaweiEMMAChildPlan{model: model}
	previous := child.catalog
	child.catalog = &decision
	if row, selected := decision.Row(); selected {
		profiles, err := CatalogMonitoringProfiles()
		if err != nil {
			return r.withholdEMMAChild(child, ProviderReasonNoProfile, true)
		}
		for _, profile := range profiles {
			if profile.Card() == row.Card {
				plan.profile, plan.steps = profile, profile.Plan(child.unit)
				break
			}
		}
	} else if decision.Reason() == SUN2000CatalogNoCatalogRow {
		plan.steps = []SUN2000ReadStep{
			{Function: 3, UnitID: child.unit, Offset: 32000, Quantity: 16},
			{Function: 3, UnitID: child.unit, Offset: 32016, Quantity: 24},
			{Function: 3, UnitID: child.unit, Offset: 32106, Quantity: 10},
		}
	} else if decision.Outcome() == SUN2000CatalogAmbiguous {
		return r.withholdEMMAChild(child, ProviderReasonCatalogAmbiguous, true)
	}
	if len(plan.steps) == 0 {
		return r.withholdEMMAChild(child, ProviderReasonCatalogNotSelected, true)
	}
	if child.emmaPlan != nil && !sameEMMAChildIdentity(*child.emmaPlan, plan, previous, decision) {
		fenceEMMAChild(child)
	}
	child.emmaPlan, child.identifiedGeneration = &plan, r.generation
	return true
}

func fenceEMMAChild(child *huaweiChildState) {
	if child.emmaPlan != nil && len(child.reading.groups) != 0 {
		child.fenced = &ProviderFencedReading{ProfileID: child.emmaPlan.profile.ID(), Model: child.emmaPlan.model,
			Reading: child.reading, Groups: child.groupEvidence(nil)}
	}
	child.fence()
	child.emmaPlan, child.identifiedGeneration = nil, 0
	child.refused = map[string]bool{}
}

func sameEMMAChildIdentity(old, next huaweiEMMAChildPlan, previous *SUN2000CatalogDecision, current SUN2000CatalogDecision) bool {
	if previous == nil || old.model != next.model || old.profile.ID() != next.profile.ID() {
		return false
	}
	oldID, oldRead := previous.Identifier()
	id, read := current.Identifier()
	oldBuild, oldSource := previous.Build()
	build, source := current.Build()
	return oldID == id && oldRead == read && oldBuild == build && oldSource == source && previous.FirmwareText() == current.FirmwareText()
}

// A stale reason renders the retained reading without admitting the child or
// changing its history. An empty reason runs the admitted monitoring plan.
func (r *huaweiCycleRun) monitorEMMAChild(child *huaweiChildState, staleReason ProviderReason) {
	read := staleReason == ""
	plan := child.emmaPlan
	out := ProviderChildObservation{UnitID: child.unit, Model: plan.model, ProfileID: plan.profile.ID(), Family: plan.profile.Family(),
		ListedClass: child.listedClass, IdentifiedGeneration: child.identifiedGeneration, Catalog: child.catalog, Fenced: child.fenced,
		Evidence: ProviderEvidenceCurrent, Reason: ProviderReasonRead}
	if !read {
		out.Evidence, out.Reason = ProviderEvidenceStale, staleReason
	}
	if plan.profile.ID() == "" {
		if read {
			out.Reason = ProviderReasonNoProfile
		}
		r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossUnconsumedWords, HasUnit: true, UnitID: child.unit,
			Detail: "no catalogue card; groups remain native"})
	}
	if len(plan.profile.battery.fields) == 0 {
		r.out.Loss = append(r.out.Loss, ProviderLoss{Kind: ProviderLossBattery, HasUnit: true, UnitID: child.unit,
			Detail: "site level only: no documented inverter-side battery decoder for the child"})
	}
	groups := make([]SUN2000GroupReading, 0, len(plan.steps))
	for index, step := range plan.steps {
		key := huaweiRefusalKey(out.ProfileID, step.Offset, step.Quantity)
		kept := huaweiPlantState{refused: child.refused[key]}
		if index < len(child.reading.groups) {
			kept.words, kept.meta = child.reading.groups[index].Words, child.groups[step.Offset]
		}
		outcome, code := SUN2000GroupNotRead, uint8(0)
		if read {
			outcome, code = r.readGatewayGroup(child.unit, step.Offset, step.Quantity, &kept)
		}
		if read && kept.refused {
			child.refused[key] = true
		}
		if r.stopped() {
			return
		}
		evidence := kept.evidence(step.Offset, step.Quantity, outcome, code)
		group := SUN2000GroupReading{Offset: step.Offset, Quantity: step.Quantity, Outcome: outcome, ExceptionCode: code}
		if len(kept.words) != 0 {
			group.Outcome, group.ExceptionCode = SUN2000GroupAnswered, 0
			group.Words, group.Cycle, group.Retained = kept.words, evidence.Cycle, evidence.Retained
			if staleReason == "" {
				child.groups[step.Offset] = kept.meta
			}
		}
		groups, out.Groups = append(groups, group), append(out.Groups, evidence)
		r.recordGatewayGroupLoss(child.unit, evidence, "inverter group did not answer")
		switch {
		case outcome == SUN2000GroupFailed:
			out.Evidence, out.Reason = ProviderEvidenceStale, ProviderReasonChildReadFailed
			child.failures++
			child.skipUntil = huaweiChildSkipUntil(r.cycle, child.failures)
			read = false
		case evidence.Refused || outcome == SUN2000GroupException:
			if out.Evidence == ProviderEvidenceCurrent {
				out.Evidence, out.Reason = ProviderEvidenceStale, ProviderReasonChildException
			}
		}
	}
	if read {
		child.failures, child.skipUntil = 0, 0
	}
	out.Reading = SUN2000MonitoringReading{unitID: child.unit, profileID: out.ProfileID, groups: groups}
	if plan.profile.ID() != "" {
		out.Reading.facts = plan.profile.catalogFacts(groups)
	}
	if staleReason == "" {
		child.reading = out.Reading
	}
	r.out.Children = append(r.out.Children, out)
}
