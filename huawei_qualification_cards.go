package modbusreg

import (
	"encoding/json"
	"slices"
	"sort"
)

// Qualification card identities and the SmartLogger class name. The card
// contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md.
const (
	// HuaweiSmartLoggerCanonicalClass is the class name the SmartLogger cards
	// select and the exact self-entry model the documentary card admits.
	HuaweiSmartLoggerCanonicalClass = "SmartLogger"

	// HuaweiSmartLoggerQualificationCardID identifies the documentary
	// SmartLogger card; the version suffix changes with its admission rules.
	HuaweiSmartLoggerQualificationCardID = "huawei.qualification.smartlogger.readonly@1.0.0"
	// HuaweiEMMAQualificationCardID identifies the documentary EMMA card.
	HuaweiEMMAQualificationCardID = "huawei.qualification.emma.readonly@1.0.0"
	// HuaweiSDongleQualificationCardID identifies the retained S-Dongle
	// persistent non-response record, which is evidence-blocked by design.
	HuaweiSDongleQualificationCardID = "huawei.qualification.sdongle.evidence-blocked@1.0.0"

	huaweiQualificationExpectedResultSchema = "helianthus-huawei-qualification-readiness/v1"
)

// HuaweiQualificationReadiness is the provider-local status of a qualification
// result. No value grants hardware readiness, admission or support.
type HuaweiQualificationReadiness string

// Readiness values. Their wire strings are part of the expected-result JSON.
const (
	// HuaweiQualificationInsufficientEvidence is the fail-closed status of any
	// input the card does not admit; it is also the zero-value status.
	HuaweiQualificationInsufficientEvidence HuaweiQualificationReadiness = "INSUFFICIENT_EVIDENCE"
	// HuaweiQualificationTestReady means identity and inventory matched and
	// the bounded qualification procedure can run; it is not a hardware or
	// live qualification.
	HuaweiQualificationTestReady HuaweiQualificationReadiness = "QUALIFICATION_TEST_READY"
	// HuaweiQualificationEvidenceBlocked means the card stops on named missing
	// evidence, either after an identity match or by retained record.
	HuaweiQualificationEvidenceBlocked HuaweiQualificationReadiness = "QUALIFICATION_EVIDENCE_BLOCKED"
	// HuaweiQualificationAmbiguous is the resolved status when results of more
	// than one class matched; no class is chosen.
	HuaweiQualificationAmbiguous HuaweiQualificationReadiness = "AMBIGUOUS"
)

// HuaweiQualificationReason is the closed reason that accompanies a readiness
// status.
type HuaweiQualificationReason string

// Qualification reasons. Their wire strings are stable result evidence.
const (
	// HuaweiQualificationIdentityInvalid covers a wrong unit, a wrong or
	// partial identity, refused software and the zero-value result.
	HuaweiQualificationIdentityInvalid HuaweiQualificationReason = "IDENTITY_INVALID"
	// HuaweiQualificationInventoryInvalid means the inventory failed parsing,
	// stability or a limit; no partial child list is retained.
	HuaweiQualificationInventoryInvalid HuaweiQualificationReason = "INVENTORY_INVALID"
	// HuaweiQualificationMatchedIdentityInventory accompanies a test-ready
	// result whose identity and complete inventory matched.
	HuaweiQualificationMatchedIdentityInventory HuaweiQualificationReason = "IDENTITY_INVENTORY_MATCHED"
	// HuaweiQualificationIdentityMatchedCapabilitiesMissing accompanies an
	// identity match that still waits for capability fixtures.
	HuaweiQualificationIdentityMatchedCapabilitiesMissing HuaweiQualificationReason = "IDENTITY_MATCHED_CAPABILITIES_MISSING"
	// HuaweiQualificationPersistentNonResponse is the reason of the retained
	// S-Dongle record of a live run stopped on persistent non-response.
	HuaweiQualificationPersistentNonResponse HuaweiQualificationReason = "LIVE_STOPPED_PERSISTENT_NON_RESPONSE"
	// HuaweiQualificationMultipleClasses accompanies an ambiguous resolution.
	HuaweiQualificationMultipleClasses HuaweiQualificationReason = "MULTIPLE_CLASSES"
)

// HuaweiQualificationInventoryLimits are the finite bounds a caller applies to
// the inventory read of a card. Exhausting any of them rejects the whole
// inventory.
type HuaweiQualificationInventoryLimits struct {
	// DeadlineMilliseconds bounds the whole inventory read, in milliseconds.
	DeadlineMilliseconds uint32
	// MaxPages bounds the number of inventory responses.
	MaxPages uint32
	// MaxObjects bounds the number of inventory objects across all pages.
	MaxObjects uint32
	// MaxBytes bounds the aggregate response bytes across all pages.
	MaxBytes uint32
	// MaxChildren bounds the number of child entries, the self entry excluded.
	MaxChildren uint32
	// MaxResponseBytes is the finite cap on one inventory response. Zero means
	// the card applies no cap of its own and relies on the classic frame limit.
	MaxResponseBytes uint32
}

// HuaweiQualificationStep describes one caller-controlled read-only evidence
// input. It is inert metadata and carries no transport or dispatch method.
type HuaweiQualificationStep struct {
	operation        string
	unitID           uint8
	offset, quantity uint16
	objectID         uint8
}

// Operation names the read, such as an FC03 register read or an FC2B MEI
// inventory read; it is a label, not a dispatch key.
func (step HuaweiQualificationStep) Operation() string { return step.operation }

// UnitID returns the Modbus unit the read is addressed to.
func (step HuaweiQualificationStep) UnitID() uint8 { return step.unitID }

// Offset returns the PDU register offset of a register read; zero for an
// inventory read.
func (step HuaweiQualificationStep) Offset() uint16 { return step.offset }

// Quantity returns the register count of a register read; zero for an
// inventory read.
func (step HuaweiQualificationStep) Quantity() uint16 { return step.quantity }

// ObjectID returns the start object of an inventory read; zero for a register
// read.
func (step HuaweiQualificationStep) ObjectID() uint8 { return step.objectID }

// HuaweiQualificationResult is the immutable outcome of one card evaluation or
// of ResolveHuaweiQualification. Its zero value reads as insufficient evidence
// with an invalid identity, and every authority accessor reports false.
// Slice accessors return copies.
type HuaweiQualificationResult struct {
	candidateClass   string
	selectedClass    string
	modelVariant     string
	readiness        HuaweiQualificationReadiness
	reason           HuaweiQualificationReason
	identityMatched  bool
	inventory        *HuaweiGatewayInventory
	missingEvidence  []string
	candidateClasses []string

	pageAnomalies      HuaweiInventoryPageAnomalies
	basicCorroborated  bool
	basicOpaqueObjects int
	modelResidueBytes  int

	// selection is the catalog decision over the gateway's own software
	// text, recorded whenever the card reached it.
	selection *HuaweiGatewaySelection
	// observedFloor is the card's own floor, the observed build, and
	// observedRelation how the software text relates to it.
	observedFloor    string
	observedRelation HuaweiGatewayFloorRelation
	// protocolWords and typeWord are gateway words the card records and never
	// compares beyond its floor form.
	protocolWords []uint16
	typeWord      *uint16
	// childOpaque holds the opaque attributes of the admitted child entry.
	childOpaque []HuaweiGatewayOpaqueAttribute
}

// CandidateClass returns the class of the card that produced the result; it is
// empty for a resolved ambiguous or empty result.
func (result HuaweiQualificationResult) CandidateClass() string { return result.candidateClass }

// SelectedClass returns the matched class, or empty when nothing matched.
func (result HuaweiQualificationResult) SelectedClass() string { return result.selectedClass }

// ModelVariant returns the matched model token when the card distinguishes
// variants; it is empty otherwise.
func (result HuaweiQualificationResult) ModelVariant() string { return result.modelVariant }

// Readiness returns the status, defaulting to insufficient evidence for the
// zero value.
func (result HuaweiQualificationResult) Readiness() HuaweiQualificationReadiness {
	if result.readiness == "" {
		return HuaweiQualificationInsufficientEvidence
	}
	return result.readiness
}

// Reason returns the closed reason, defaulting to an invalid identity for the
// zero value.
func (result HuaweiQualificationResult) Reason() HuaweiQualificationReason {
	if result.reason == "" {
		return HuaweiQualificationIdentityInvalid
	}
	return result.reason
}

// IdentityMatched reports whether the card matched the identity it checks.
func (result HuaweiQualificationResult) IdentityMatched() bool { return result.identityMatched }

// PageAnomalies returns the tolerated response-shape counts of an observed
// dialect inventory; it is zero for cards that tolerate none.
func (result HuaweiQualificationResult) PageAnomalies() HuaweiInventoryPageAnomalies {
	return result.pageAnomalies
}

// BasicCorroborated reports whether a decoded basic identification reply
// agreed with the extended self entry. False also covers an absent reply.
func (result HuaweiQualificationResult) BasicCorroborated() bool { return result.basicCorroborated }

// BasicOpaqueObjectCount returns how many extra basic identification objects
// were retained as opaque evidence.
func (result HuaweiQualificationResult) BasicOpaqueObjectCount() int {
	return result.basicOpaqueObjects
}

// ChildOpaqueAttributes lists the opaque attributes of the admitted child
// entry in entry order; they are retained evidence only.
func (result HuaweiQualificationResult) ChildOpaqueAttributes() []HuaweiGatewayOpaqueAttribute {
	return slices.Clone(result.childOpaque)
}

// ModelResidueBytes is the length of the printable residue that followed the
// terminating NUL of a model block. The residue is retained evidence only.
func (result HuaweiQualificationResult) ModelResidueBytes() int { return result.modelResidueBytes }

// Selection is the catalog decision over the gateway's own software text:
// the selected floor and its editions, the grade and the reason, or the
// candidates of an ambiguous decision, or the closed reason of a refusal. It is
// absent when the card stopped before the software text.
func (result HuaweiQualificationResult) Selection() (HuaweiGatewaySelection, bool) {
	if result.selection == nil {
		return HuaweiGatewaySelection{}, false
	}
	return *result.selection, true
}

// SoftwareGrade is the grade of the software applicability: Proven for the
// observed build of an observed card, Hypothesis for an inherited build, and
// Unknown when no software text was compared.
func (result HuaweiQualificationResult) SoftwareGrade() HuaweiEvidenceGrade {
	if result.selection == nil || !result.IdentityMatched() {
		return HuaweiGradeUnknown
	}
	return result.selection.Grade()
}

// ObservedFloor is the card's own floor, the build its observation fixed, and
// how the software text relates to it. It is empty for a card without one.
func (result HuaweiQualificationResult) ObservedFloor() (string, HuaweiGatewayFloorRelation) {
	return result.observedFloor, result.observedRelation
}

// GatewayProtocolWords are the protocol words that the card read at the
// gateway unit, recorded as read.
func (result HuaweiQualificationResult) GatewayProtocolWords() []uint16 {
	return append([]uint16(nil), result.protocolWords...)
}

// GatewayTypeWord is the type word that the card recorded at the gateway unit.
// It is a configuration value and never a predicate.
func (result HuaweiQualificationResult) GatewayTypeWord() (uint16, bool) {
	if result.typeWord == nil {
		return 0, false
	}
	return *result.typeWord, true
}

// QualificationTestReady reports whether Readiness is
// HuaweiQualificationTestReady.
func (result HuaweiQualificationResult) QualificationTestReady() bool {
	return result.Readiness() == HuaweiQualificationTestReady
}

// HardwareTestReady is always false: no card makes a class hardware-test ready.
func (HuaweiQualificationResult) HardwareTestReady() bool { return false }

// DefaultDenied is always true: a result never lifts the default denial.
func (HuaweiQualificationResult) DefaultDenied() bool { return true }

// CatalogRegistered is always false: a result registers nothing.
func (HuaweiQualificationResult) CatalogRegistered() bool { return false }

// AutomaticRuntimeAdmission is always false: a result admits no runtime
// provider.
func (HuaweiQualificationResult) AutomaticRuntimeAdmission() bool { return false }

// LiveQualified is always false: cards evaluate offline evidence only.
func (HuaweiQualificationResult) LiveQualified() bool { return false }

// SupportClaim is always false: identity is not telemetry support.
func (HuaweiQualificationResult) SupportClaim() bool { return false }

// WriteAuthority is always false: cards are read-only.
func (HuaweiQualificationResult) WriteAuthority() bool { return false }

// NativeFactCount is always zero: cards retain no native telemetry facts.
func (HuaweiQualificationResult) NativeFactCount() int { return 0 }

// AutomaticRequestCount is always zero: cards issue no requests.
func (HuaweiQualificationResult) AutomaticRequestCount() int { return 0 }

// FallbackAttempted is always false: no card falls back to another class.
func (HuaweiQualificationResult) FallbackAttempted() bool { return false }

// MissingEvidence returns a copy of the named evidence that still blocks the
// class, in card order.
func (result HuaweiQualificationResult) MissingEvidence() []string {
	return append([]string(nil), result.missingEvidence...)
}

// CandidateClasses returns a copy of the sorted, distinct matched classes that
// resolution saw; it is empty on a single card's result.
func (result HuaweiQualificationResult) CandidateClasses() []string {
	return append([]string(nil), result.candidateClasses...)
}

// Inventory returns a copy of the retained inventory of a test-ready
// inventory card. The second result is false when no inventory was retained,
// which is always the case after a failure.
func (result HuaweiQualificationResult) Inventory() (HuaweiGatewayInventory, bool) {
	if result.inventory == nil {
		return HuaweiGatewayInventory{}, false
	}
	inventory := *result.inventory
	inventory.children = append([]HuaweiSmartLoggerInventoryChild(nil), result.inventory.children...)
	return inventory, true
}

// HuaweiSmartLoggerQualificationCard is the stateless, offline documentary
// card for the SmartLogger family at gateway unit 0. It sends nothing and
// grants no authority; see Evaluate for its admission rule.
type HuaweiSmartLoggerQualificationCard struct{}

// HuaweiSmartLoggerQualificationInput is the already-read evidence the
// documentary SmartLogger card evaluates.
type HuaweiSmartLoggerQualificationInput struct {
	// UnitID is the unit the counter and inventory reads were addressed to.
	UnitID uint8
	// Inventory is the detached extended inventory with its counter bracket.
	Inventory HuaweiSmartLoggerOfflineInventoryInput
}

// NewHuaweiSmartLoggerQualificationCard returns the stateless card.
func NewHuaweiSmartLoggerQualificationCard() HuaweiSmartLoggerQualificationCard {
	return HuaweiSmartLoggerQualificationCard{}
}

// ID returns HuaweiSmartLoggerQualificationCardID.
func (HuaweiSmartLoggerQualificationCard) ID() string { return HuaweiSmartLoggerQualificationCardID }

// CandidateClass returns HuaweiSmartLoggerCanonicalClass.
func (HuaweiSmartLoggerQualificationCard) CandidateClass() string {
	return HuaweiSmartLoggerCanonicalClass
}

// UnitID returns gateway unit 0, the only unit the card applies to.
func (HuaweiSmartLoggerQualificationCard) UnitID() uint8 { return 0 }

// Steps returns a fresh copy of the caller-run read sequence: the change
// counter, the extended inventory, and the counter again. The two counter
// values must be equal for the inventory to be admitted.
func (HuaweiSmartLoggerQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
	}
}

// Limits returns the inventory bounds of the card: a 15-second deadline and
// the shared page, object, byte and child caps. MaxResponseBytes stays zero.
func (HuaweiSmartLoggerQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiSmartLoggerInventoryMaxPages,
		MaxObjects:           huaweiSmartLoggerInventoryMaxObjects,
		MaxBytes:             huaweiSmartLoggerInventoryMaxBytes,
		MaxChildren:          huaweiSmartLoggerInventoryMaxChildren,
	}
}

// EmptyResult returns the card's fail-closed result: insufficient evidence
// with an invalid identity and nothing retained.
func (card HuaweiSmartLoggerQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate admits a stable, complete inventory at unit 0 whose self entry is
// exactly the model SmartLogger and whose software text the catalog
// decision admits: a build of the V300 line at or above the documented
// minimum, or a build that no documented floor relates to (grade Hypothesis).
// Child attributes follow the device-list dictionary of the admitted editions,
// which the selection records; a key that no admitted edition defines is
// retained as opaque native evidence. The count object counts the children
// only; the observed dialect that counts the self entry stays with the
// observed card.
func (card HuaweiSmartLoggerQualificationCard) Evaluate(input HuaweiSmartLoggerQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	inventory, err := parseHuaweiExtendedMEIOfflineInventory(input.Inventory, func(model string) bool {
		return model == HuaweiSmartLoggerCanonicalClass
	}, parseHuaweiDocumentaryInventoryChild)
	if err != nil {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	var self *HuaweiSmartLoggerInventoryChild
	for _, page := range input.Inventory.Pages {
		for _, object := range page.Objects {
			if object.ObjectID != huaweiSmartLoggerInventoryStartObject+1 {
				continue
			}
			decoded, err := parseHuaweiDocumentaryInventoryChild(object)
			if err != nil {
				rejected.reason = HuaweiQualificationInventoryInvalid
				return rejected
			}
			self = &decoded
		}
	}
	if self == nil || self.Attribute("model") != card.CandidateClass() {
		return rejected
	}
	selection := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
		Family:   card.CandidateClass(),
		Scope:    HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: input.UnitID},
		Software: self.Attribute("software_version"),
	})
	if !selection.Admitted() || selection.Build() == "" {
		rejected.selection = &selection
		return rejected
	}
	retained := inventory
	return HuaweiQualificationResult{
		candidateClass:  card.CandidateClass(),
		selectedClass:   card.CandidateClass(),
		readiness:       HuaweiQualificationTestReady,
		reason:          HuaweiQualificationMatchedIdentityInventory,
		identityMatched: true,
		inventory:       &retained,
		missingEvidence: []string{"connected_smartlogger_identity_inventory_capture"},
		selection:       &selection,
	}
}

// HuaweiEMMAQualificationCard is the stateless, offline documentary card for
// the EMMA family at gateway unit 0. A matched identity stays evidence-blocked
// on its named capability fixtures; the card sends nothing and grants no
// authority.
type HuaweiEMMAQualificationCard struct{}

// HuaweiEMMAQualificationInput is the already-read FC03 identity tuple the
// EMMA card evaluates. The texts are the decoded register strings; terminal
// padding is removed by the card.
type HuaweiEMMAQualificationInput struct {
	// UnitID is the unit the three reads were addressed to.
	UnitID uint8
	// Offering is the offering text.
	Offering string
	// Model is the model text; admission follows the catalog's EMMA model rows.
	Model string
	// Firmware is the software version text, compared with the floor of its
	// own branch.
	Firmware string
}

// NewHuaweiEMMAQualificationCard returns the stateless card.
func NewHuaweiEMMAQualificationCard() HuaweiEMMAQualificationCard {
	return HuaweiEMMAQualificationCard{}
}

// ID returns HuaweiEMMAQualificationCardID.
func (HuaweiEMMAQualificationCard) ID() string { return HuaweiEMMAQualificationCardID }

// CandidateClass returns HuaweiEMMACanonicalClass.
func (HuaweiEMMAQualificationCard) CandidateClass() string { return HuaweiEMMACanonicalClass }

// UnitID returns gateway unit 0, the only unit the card applies to.
func (HuaweiEMMAQualificationCard) UnitID() uint8 { return 0 }

// ModelVariants are the observed models of the card. Admission follows the
// catalog's model rows and holds every model token of the EMMA family.
func (HuaweiEMMAQualificationCard) ModelVariants() []string {
	return append([]string(nil), huaweiEMMAObservedModels...)
}

// Steps returns a fresh copy of the caller-run FC03 identity reads at unit 0:
// offering, model and software version.
func (HuaweiEMMAQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_OFFERING", unitID: 0, offset: 30000, quantity: 15},
		{operation: "FC03_MODEL", unitID: 0, offset: 30222, quantity: 20},
		{operation: "FC03_FIRMWARE", unitID: 0, offset: 30035, quantity: 15},
	}
}

// MissingCapabilityEvidence returns a fresh copy of the named fixtures that
// keep a matched EMMA identity evidence-blocked.
func (HuaweiEMMAQualificationCard) MissingCapabilityEvidence() []string {
	return []string{"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture"}
}

// EmptyResult returns the card's fail-closed result: insufficient evidence
// with an invalid identity and nothing retained.
func (card HuaweiEMMAQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate matches the identity tuple at unit 0 through
// EvaluateHuaweiEMMAOfflineIdentity. A match returns evidence-blocked with the
// capability fixtures as missing evidence and zero native facts; it never
// creates a capability. A mismatch returns EmptyResult, which keeps the
// catalog selection only when the software text was refused.
func (card HuaweiEMMAQualificationCard) Evaluate(input HuaweiEMMAQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	identity := EvaluateHuaweiEMMAOfflineIdentity(input.Offering, input.Model, input.Firmware)
	if !identity.Matched() {
		if identity.Reason() == HuaweiEMMAOfflineFirmwareMismatch {
			selection := identity.Selection()
			rejected.selection = &selection
		}
		return rejected
	}
	selection := identity.Selection()
	return HuaweiQualificationResult{
		candidateClass:  card.CandidateClass(),
		selectedClass:   card.CandidateClass(),
		modelVariant:    identity.ModelVariant(),
		readiness:       HuaweiQualificationEvidenceBlocked,
		reason:          HuaweiQualificationIdentityMatchedCapabilitiesMissing,
		identityMatched: true,
		missingEvidence: card.MissingCapabilityEvidence(),
		selection:       &selection,
	}
}

// HuaweiSDongleQualificationCard is the retained record of the persistent
// non-response stop. Its result never changes. The shared resolution lets a
// matched result of the observed S-Dongle card supersede it, and nothing else.
type HuaweiSDongleQualificationCard struct{}

// NewHuaweiSDongleQualificationCard returns the stateless record.
func NewHuaweiSDongleQualificationCard() HuaweiSDongleQualificationCard {
	return HuaweiSDongleQualificationCard{}
}

// ID returns HuaweiSDongleQualificationCardID.
func (HuaweiSDongleQualificationCard) ID() string { return HuaweiSDongleQualificationCardID }

// CandidateClass returns HuaweiSDongleCanonicalClass.
func (HuaweiSDongleQualificationCard) CandidateClass() string { return HuaweiSDongleCanonicalClass }

// UnitID returns unit 100, the unit of the recorded stopped run.
func (HuaweiSDongleQualificationCard) UnitID() uint8 { return 100 }

// HardStop is always true: further I/O stays stopped until the missing
// evidence and action-time operator approval exist.
func (HuaweiSDongleQualificationCard) HardStop() bool { return true }

// Steps returns nil: the record defines no read sequence.
func (HuaweiSDongleQualificationCard) Steps() []HuaweiQualificationStep {
	return nil
}

// Status is always HuaweiQualificationEvidenceBlocked.
func (HuaweiSDongleQualificationCard) Status() HuaweiQualificationReadiness {
	return HuaweiQualificationEvidenceBlocked
}

// RequiredConnectionContext returns a fresh copy of the connection facts a
// future attempt must confirm before any read.
func (HuaweiSDongleQualificationCard) RequiredConnectionContext() []string {
	return []string{"endpoint", "port", "unit_id_100", "gateway_child_topology"}
}

// Result returns the fixed evidence-blocked, persistent non-response result
// with its named missing evidence. It never matches an identity.
func (card HuaweiSDongleQualificationCard) Result() HuaweiQualificationResult {
	return HuaweiQualificationResult{
		candidateClass: card.CandidateClass(),
		readiness:      HuaweiQualificationEvidenceBlocked,
		reason:         HuaweiQualificationPersistentNonResponse,
		missingEvidence: []string{
			"confirmed_gateway_connection_context",
			"gateway_unit_100_topology",
			"sanitized_basic_mei_product_model_fixture",
			"exact_protocol_version_encoding_fixture",
			"completed_search_stable_sequence_capacity_fixture",
			"separately_qualified_child_unit_inventory_fixture",
		},
	}
}

// ResolveHuaweiQualification combines card results without choosing a winner.
// It first drops non-matches of a class that matched elsewhere and
// evidence-blocked matches superseded by a test-ready match of the same class.
// A remaining persistent non-response record wins; one match from a single
// remaining result is returned with its class list; several matches resolve to
// HuaweiQualificationAmbiguous; anything else is insufficient evidence.
func ResolveHuaweiQualification(results ...HuaweiQualificationResult) HuaweiQualificationResult {
	results = withoutHuaweiSiblingNonMatches(results)
	results = withoutHuaweiSupersededMatches(results)
	matches := make([]HuaweiQualificationResult, 0, len(results))
	classes := make([]string, 0, len(results))
	var persistentBlock *HuaweiQualificationResult
	for _, result := range results {
		if result.Readiness() == HuaweiQualificationEvidenceBlocked && result.Reason() == HuaweiQualificationPersistentNonResponse {
			copy := result
			persistentBlock = &copy
		}
		if !result.IdentityMatched() || result.SelectedClass() == "" {
			continue
		}
		matches = append(matches, result)
		classes = append(classes, result.SelectedClass())
	}
	if persistentBlock != nil {
		return *persistentBlock
	}
	sort.Strings(classes)
	classes = slices.Compact(classes)
	if len(matches) == 1 && len(results) == 1 {
		selected := matches[0]
		selected.candidateClasses = classes
		return selected
	}
	if len(matches) > 1 {
		return HuaweiQualificationResult{
			readiness:        HuaweiQualificationAmbiguous,
			reason:           HuaweiQualificationMultipleClasses,
			candidateClasses: classes,
		}
	}
	if len(matches) == 1 {
		return HuaweiQualificationResult{
			readiness:        HuaweiQualificationInsufficientEvidence,
			reason:           HuaweiQualificationIdentityInvalid,
			candidateClasses: classes,
		}
	}
	return HuaweiQualificationResult{readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// withoutHuaweiSiblingNonMatches drops a non-matching result when another card
// of the same class matched. Cards of one class are alternative entries, so the
// rejection of one says nothing against the entry that matched. A persistent
// non-response record is a non-matching result of its own class: an exact tuple
// of that class contradicts it and supersedes it, and nothing else does. A
// match never hides a result of a different class.
func withoutHuaweiSiblingNonMatches(results []HuaweiQualificationResult) []HuaweiQualificationResult {
	matched := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.IdentityMatched() && result.SelectedClass() != "" {
			matched[result.SelectedClass()] = struct{}{}
		}
	}
	if len(matched) == 0 {
		return results
	}
	kept := make([]HuaweiQualificationResult, 0, len(results))
	for _, result := range results {
		_, sibling := matched[result.CandidateClass()]
		if !result.IdentityMatched() && sibling {
			continue
		}
		kept = append(kept, result)
	}
	return kept
}

// withoutHuaweiSupersededMatches drops an identity match that still waits for
// its capability evidence when a result of the same class matched a complete
// observed tuple. Both results describe one device: the documentary card says
// that the identity is known and the capabilities are not, and the observed card
// adds the fixtures behind the identity and the inventory. Results of other
// classes, and two complete matches of one class, are left as they are.
func withoutHuaweiSupersededMatches(results []HuaweiQualificationResult) []HuaweiQualificationResult {
	complete := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.IdentityMatched() && result.SelectedClass() != "" && result.Readiness() == HuaweiQualificationTestReady {
			complete[result.SelectedClass()] = struct{}{}
		}
	}
	if len(complete) == 0 {
		return results
	}
	kept := make([]HuaweiQualificationResult, 0, len(results))
	for _, result := range results {
		_, superseded := complete[result.SelectedClass()]
		if superseded && result.IdentityMatched() && result.Readiness() == HuaweiQualificationEvidenceBlocked &&
			result.Reason() == HuaweiQualificationIdentityMatchedCapabilitiesMissing {
			continue
		}
		kept = append(kept, result)
	}
	return kept
}

// HuaweiQualificationPackage is the stateless entry point to the published
// expected outcome of every Huawei qualification card.
type HuaweiQualificationPackage struct{}

// NewHuaweiQualificationPackage returns the stateless package.
func NewHuaweiQualificationPackage() HuaweiQualificationPackage { return HuaweiQualificationPackage{} }

// ExpectedResult returns the expected-result document of the package.
func (HuaweiQualificationPackage) ExpectedResult() HuaweiQualificationExpectedResult {
	return HuaweiQualificationExpectedResult{}
}

// HuaweiQualificationExpectedResult is the expected status, card identity and
// missing evidence of each class card, documentary and observed. The values
// are read from the cards where a card exposes them.
type HuaweiQualificationExpectedResult struct{}

// MarshalJSON encodes the expected result under the
// helianthus-huawei-qualification-readiness/v1 schema, with every
// default-denied and authority field fixed at its denied value.
func (HuaweiQualificationExpectedResult) MarshalJSON() ([]byte, error) {
	smartLogger := NewHuaweiSmartLoggerQualificationCard()
	observed := NewHuaweiSmartLoggerObservedQualificationCard()
	emma := NewHuaweiEMMAQualificationCard()
	sdongle := NewHuaweiSDongleQualificationCard()
	sdongleObserved := NewHuaweiSDongleObservedQualificationCard()
	emmaObserved := NewHuaweiEMMAObservedQualificationCard()
	classes := []map[string]any{
		{
			"class": smartLogger.CandidateClass(), "card_id": smartLogger.ID(), "status": HuaweiQualificationTestReady,
			"missing_evidence":    []string{"connected_smartlogger_identity_inventory_capture"},
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": observed.CandidateClass(), "card_id": observed.ID(), "status": HuaweiQualificationTestReady,
			"missing_evidence":    observed.MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": emma.CandidateClass(), "card_id": emma.ID(), "status": HuaweiQualificationEvidenceBlocked,
			"missing_evidence":    emma.MissingCapabilityEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": sdongle.CandidateClass(), "card_id": sdongle.ID(), "status": HuaweiQualificationEvidenceBlocked,
			"missing_evidence":    sdongle.Result().MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": sdongleObserved.CandidateClass(), "card_id": sdongleObserved.ID(), "status": HuaweiQualificationTestReady,
			"missing_evidence":    sdongleObserved.MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": emmaObserved.CandidateClass(), "card_id": emmaObserved.ID(), "status": HuaweiQualificationTestReady,
			"missing_evidence":    emmaObserved.MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
	}
	return json.Marshal(map[string]any{
		"schema":                      huaweiQualificationExpectedResultSchema,
		"classes":                     classes,
		"default_denied":              true,
		"catalog_registered":          false,
		"automatic_runtime_admission": false,
		"support_claim":               false,
		"write_authority":             false,
	})
}
