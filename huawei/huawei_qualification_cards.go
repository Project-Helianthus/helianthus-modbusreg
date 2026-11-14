package huawei

import (
	"encoding/json"
	"slices"
	"sort"
)

// Qualification card identities and the SmartLogger class name. The card
// contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/qualification-readiness-v1.md.
const (
	// SmartLoggerCanonicalClass is the class name the SmartLogger cards
	// select and the exact self-entry model the documentary card admits.
	SmartLoggerCanonicalClass = "SmartLogger"

	// SmartLoggerQualificationCardID identifies the documentary
	// SmartLogger card; the version suffix changes with its admission rules.
	SmartLoggerQualificationCardID = "huawei.qualification.smartlogger.readonly@1.0.0"
	// EMMAQualificationCardID identifies the documentary EMMA card.
	EMMAQualificationCardID = "huawei.qualification.emma.readonly@1.0.0"
	// SDongleQualificationCardID identifies the retained S-Dongle
	// persistent non-response record, which is evidence-blocked by design.
	SDongleQualificationCardID = "huawei.qualification.sdongle.evidence-blocked@1.0.0"

	huaweiQualificationExpectedResultSchema = "helianthus-huawei-qualification-readiness/v1"
)

// QualificationReadiness is the provider-local status of a qualification
// result. No value grants hardware readiness, admission or support.
type QualificationReadiness string

// Readiness values. Their wire strings are part of the expected-result JSON.
const (
	// QualificationInsufficientEvidence is the fail-closed status of any
	// input the card does not admit; it is also the zero-value status.
	QualificationInsufficientEvidence QualificationReadiness = "INSUFFICIENT_EVIDENCE"
	// QualificationTestReady means identity and inventory matched and
	// the bounded qualification procedure can run; it is not a hardware or
	// live qualification.
	QualificationTestReady QualificationReadiness = "QUALIFICATION_TEST_READY"
	// QualificationEvidenceBlocked means the card stops on named missing
	// evidence, either after an identity match or by retained record.
	QualificationEvidenceBlocked QualificationReadiness = "QUALIFICATION_EVIDENCE_BLOCKED"
	// QualificationAmbiguous is the resolved status when results of more
	// than one class matched; no class is chosen.
	QualificationAmbiguous QualificationReadiness = "AMBIGUOUS"
)

// QualificationReason is the closed reason that accompanies a readiness
// status.
type QualificationReason string

// Qualification reasons. Their wire strings are stable result evidence.
const (
	// QualificationIdentityInvalid covers a wrong unit, a wrong or
	// partial identity, refused software and the zero-value result.
	QualificationIdentityInvalid QualificationReason = "IDENTITY_INVALID"
	// QualificationInventoryInvalid means the inventory failed parsing,
	// stability or a limit; no partial child list is retained.
	QualificationInventoryInvalid QualificationReason = "INVENTORY_INVALID"
	// QualificationMatchedIdentityInventory accompanies a test-ready
	// result whose identity and complete inventory matched.
	QualificationMatchedIdentityInventory QualificationReason = "IDENTITY_INVENTORY_MATCHED"
	// QualificationIdentityMatchedCapabilitiesMissing accompanies an
	// identity match that still waits for capability fixtures.
	QualificationIdentityMatchedCapabilitiesMissing QualificationReason = "IDENTITY_MATCHED_CAPABILITIES_MISSING"
	// QualificationPersistentNonResponse is the reason of the retained
	// S-Dongle record of a live run stopped on persistent non-response.
	QualificationPersistentNonResponse QualificationReason = "LIVE_STOPPED_PERSISTENT_NON_RESPONSE"
	// QualificationMultipleClasses accompanies an ambiguous resolution.
	QualificationMultipleClasses QualificationReason = "MULTIPLE_CLASSES"
)

// QualificationInventoryLimits are the finite bounds a caller applies to
// the inventory read of a card. Exhausting any of them rejects the whole
// inventory.
type QualificationInventoryLimits struct {
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

// QualificationStep describes one caller-controlled read-only evidence
// input. It is inert metadata and carries no transport or dispatch method.
type QualificationStep struct {
	operation        string
	unitID           uint8
	offset, quantity uint16
	objectID         uint8
}

// Operation names the read, such as an FC03 register read or an FC2B MEI
// inventory read; it is a label, not a dispatch key.
func (step QualificationStep) Operation() string { return step.operation }

// UnitID returns the Modbus unit the read is addressed to.
func (step QualificationStep) UnitID() uint8 { return step.unitID }

// Offset returns the PDU register offset of a register read; zero for an
// inventory read.
func (step QualificationStep) Offset() uint16 { return step.offset }

// Quantity returns the register count of a register read; zero for an
// inventory read.
func (step QualificationStep) Quantity() uint16 { return step.quantity }

// ObjectID returns the start object of an inventory read; zero for a register
// read.
func (step QualificationStep) ObjectID() uint8 { return step.objectID }

// QualificationResult is the immutable outcome of one card evaluation or
// of ResolveHuaweiQualification. Its zero value reads as insufficient evidence
// with an invalid identity, and every authority accessor reports false.
// Slice accessors return copies.
type QualificationResult struct {
	candidateClass   string
	selectedClass    string
	modelVariant     string
	readiness        QualificationReadiness
	reason           QualificationReason
	identityMatched  bool
	inventory        *GatewayInventory
	missingEvidence  []string
	candidateClasses []string

	pageAnomalies      InventoryPageAnomalies
	basicCorroborated  bool
	basicOpaqueObjects int
	modelResidueBytes  int

	// selection is the catalog decision over the gateway's own software
	// text, recorded whenever the card reached it.
	selection *GatewaySelection
	// observedFloor is the card's own floor, the observed build, and
	// observedRelation how the software text relates to it.
	observedFloor    string
	observedRelation GatewayFloorRelation
	// protocolWords and typeWord are gateway words the card records and never
	// compares beyond its floor form.
	protocolWords []uint16
	typeWord      *uint16
	// childOpaque holds the opaque attributes of the admitted child entry.
	childOpaque []GatewayOpaqueAttribute
}

// CandidateClass returns the class of the card that produced the result; it is
// empty for a resolved ambiguous or empty result.
func (result QualificationResult) CandidateClass() string { return result.candidateClass }

// SelectedClass returns the matched class, or empty when nothing matched.
func (result QualificationResult) SelectedClass() string { return result.selectedClass }

// ModelVariant returns the matched model token when the card distinguishes
// variants; it is empty otherwise.
func (result QualificationResult) ModelVariant() string { return result.modelVariant }

// Readiness returns the status, defaulting to insufficient evidence for the
// zero value.
func (result QualificationResult) Readiness() QualificationReadiness {
	if result.readiness == "" {
		return QualificationInsufficientEvidence
	}
	return result.readiness
}

// Reason returns the closed reason, defaulting to an invalid identity for the
// zero value.
func (result QualificationResult) Reason() QualificationReason {
	if result.reason == "" {
		return QualificationIdentityInvalid
	}
	return result.reason
}

// IdentityMatched reports whether the card matched the identity it checks.
func (result QualificationResult) IdentityMatched() bool { return result.identityMatched }

// PageAnomalies returns the tolerated response-shape counts of an observed
// dialect inventory; it is zero for cards that tolerate none.
func (result QualificationResult) PageAnomalies() InventoryPageAnomalies {
	return result.pageAnomalies
}

// BasicCorroborated reports whether a decoded basic identification reply
// agreed with the extended self entry. False also covers an absent reply.
func (result QualificationResult) BasicCorroborated() bool { return result.basicCorroborated }

// BasicOpaqueObjectCount returns how many extra basic identification objects
// were retained as opaque evidence.
func (result QualificationResult) BasicOpaqueObjectCount() int {
	return result.basicOpaqueObjects
}

// ChildOpaqueAttributes lists the opaque attributes of the admitted child
// entry in entry order; they are retained evidence only.
func (result QualificationResult) ChildOpaqueAttributes() []GatewayOpaqueAttribute {
	return slices.Clone(result.childOpaque)
}

// ModelResidueBytes is the length of the printable residue that followed the
// terminating NUL of a model block. The residue is retained evidence only.
func (result QualificationResult) ModelResidueBytes() int { return result.modelResidueBytes }

// Selection is the catalog decision over the gateway's own software text:
// the selected floor and its editions, the grade and the reason, or the
// candidates of an ambiguous decision, or the closed reason of a refusal. It is
// absent when the card stopped before the software text.
func (result QualificationResult) Selection() (GatewaySelection, bool) {
	if result.selection == nil {
		return GatewaySelection{}, false
	}
	return *result.selection, true
}

// SoftwareGrade is the grade of the software applicability: Proven for the
// observed build of an observed card, Hypothesis for an inherited build, and
// Unknown when no software text was compared.
func (result QualificationResult) SoftwareGrade() EvidenceGrade {
	if result.selection == nil || !result.IdentityMatched() {
		return GradeUnknown
	}
	return result.selection.Grade()
}

// ObservedFloor is the card's own floor, the build its observation fixed, and
// how the software text relates to it. It is empty for a card without one.
func (result QualificationResult) ObservedFloor() (string, GatewayFloorRelation) {
	return result.observedFloor, result.observedRelation
}

// GatewayProtocolWords are the protocol words that the card read at the
// gateway unit, recorded as read.
func (result QualificationResult) GatewayProtocolWords() []uint16 {
	return append([]uint16(nil), result.protocolWords...)
}

// GatewayTypeWord is the type word that the card recorded at the gateway unit.
// It is a configuration value and never a predicate.
func (result QualificationResult) GatewayTypeWord() (uint16, bool) {
	if result.typeWord == nil {
		return 0, false
	}
	return *result.typeWord, true
}

// QualificationTestReady reports whether Readiness is
// QualificationTestReady.
func (result QualificationResult) QualificationTestReady() bool {
	return result.Readiness() == QualificationTestReady
}

// HardwareTestReady is always false: no card makes a class hardware-test ready.
func (QualificationResult) HardwareTestReady() bool { return false }

// DefaultDenied is always true: a result never lifts the default denial.
func (QualificationResult) DefaultDenied() bool { return true }

// CatalogRegistered is always false: a result registers nothing.
func (QualificationResult) CatalogRegistered() bool { return false }

// AutomaticRuntimeAdmission is always false: a result admits no runtime
// provider.
func (QualificationResult) AutomaticRuntimeAdmission() bool { return false }

// LiveQualified is always false: cards evaluate offline evidence only.
func (QualificationResult) LiveQualified() bool { return false }

// SupportClaim is always false: identity is not telemetry support.
func (QualificationResult) SupportClaim() bool { return false }

// WriteAuthority is always false: cards are read-only.
func (QualificationResult) WriteAuthority() bool { return false }

// NativeFactCount is always zero: cards retain no native telemetry facts.
func (QualificationResult) NativeFactCount() int { return 0 }

// AutomaticRequestCount is always zero: cards issue no requests.
func (QualificationResult) AutomaticRequestCount() int { return 0 }

// FallbackAttempted is always false: no card falls back to another class.
func (QualificationResult) FallbackAttempted() bool { return false }

// MissingEvidence returns a copy of the named evidence that still blocks the
// class, in card order.
func (result QualificationResult) MissingEvidence() []string {
	return append([]string(nil), result.missingEvidence...)
}

// CandidateClasses returns a copy of the sorted, distinct matched classes that
// resolution saw; it is empty on a single card's result.
func (result QualificationResult) CandidateClasses() []string {
	return append([]string(nil), result.candidateClasses...)
}

// Inventory returns a copy of the retained inventory of a test-ready
// inventory card. The second result is false when no inventory was retained,
// which is always the case after a failure.
func (result QualificationResult) Inventory() (GatewayInventory, bool) {
	if result.inventory == nil {
		return GatewayInventory{}, false
	}
	inventory := *result.inventory
	inventory.children = append([]SmartLoggerInventoryChild(nil), result.inventory.children...)
	return inventory, true
}

// SmartLoggerQualificationCard is the stateless, offline documentary
// card for the SmartLogger family at gateway unit 0. It sends nothing and
// grants no authority; see Evaluate for its admission rule.
type SmartLoggerQualificationCard struct{}

// SmartLoggerQualificationInput is the already-read evidence the
// documentary SmartLogger card evaluates.
type SmartLoggerQualificationInput struct {
	// UnitID is the unit the counter and inventory reads were addressed to.
	UnitID uint8
	// Inventory is the detached extended inventory with its counter bracket.
	Inventory SmartLoggerOfflineInventoryInput
}

// NewSmartLoggerQualificationCard returns the stateless card.
func NewSmartLoggerQualificationCard() SmartLoggerQualificationCard {
	return SmartLoggerQualificationCard{}
}

// ID returns SmartLoggerQualificationCardID.
func (SmartLoggerQualificationCard) ID() string { return SmartLoggerQualificationCardID }

// CandidateClass returns SmartLoggerCanonicalClass.
func (SmartLoggerQualificationCard) CandidateClass() string {
	return SmartLoggerCanonicalClass
}

// UnitID returns gateway unit 0, the only unit the card applies to.
func (SmartLoggerQualificationCard) UnitID() uint8 { return 0 }

// Steps returns a fresh copy of the caller-run read sequence: the change
// counter, the extended inventory, and the counter again. The two counter
// values must be equal for the inventory to be admitted.
func (SmartLoggerQualificationCard) Steps() []QualificationStep {
	return []QualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
	}
}

// Limits returns the inventory bounds of the card: a 15-second deadline and
// the shared page, object, byte and child caps. MaxResponseBytes stays zero.
func (SmartLoggerQualificationCard) Limits() QualificationInventoryLimits {
	return QualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiSmartLoggerInventoryMaxPages,
		MaxObjects:           huaweiSmartLoggerInventoryMaxObjects,
		MaxBytes:             huaweiSmartLoggerInventoryMaxBytes,
		MaxChildren:          huaweiSmartLoggerInventoryMaxChildren,
	}
}

// EmptyResult returns the card's fail-closed result: insufficient evidence
// with an invalid identity and nothing retained.
func (card SmartLoggerQualificationCard) EmptyResult() QualificationResult {
	return QualificationResult{candidateClass: card.CandidateClass(), readiness: QualificationInsufficientEvidence, reason: QualificationIdentityInvalid}
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
func (card SmartLoggerQualificationCard) Evaluate(input SmartLoggerQualificationInput) QualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	inventory, err := parseHuaweiExtendedMEIOfflineInventory(input.Inventory, func(model string) bool {
		return model == SmartLoggerCanonicalClass
	}, parseHuaweiDocumentaryInventoryChild)
	if err != nil {
		rejected.reason = QualificationInventoryInvalid
		return rejected
	}
	var self *SmartLoggerInventoryChild
	for _, page := range input.Inventory.Pages {
		for _, object := range page.Objects {
			if object.ObjectID != huaweiSmartLoggerInventoryStartObject+1 {
				continue
			}
			decoded, err := parseHuaweiDocumentaryInventoryChild(object)
			if err != nil {
				rejected.reason = QualificationInventoryInvalid
				return rejected
			}
			self = &decoded
		}
	}
	if self == nil || self.Attribute("model") != card.CandidateClass() {
		return rejected
	}
	selection := DecideHuaweiGatewaySelection(GatewaySelectionInput{
		Family:   card.CandidateClass(),
		Scope:    GatewayScope{Endpoint: GatewayEndpointSelf, Unit: input.UnitID},
		Software: self.Attribute("software_version"),
	})
	if !selection.Admitted() || selection.Build() == "" {
		rejected.selection = &selection
		return rejected
	}
	retained := inventory
	return QualificationResult{
		candidateClass:  card.CandidateClass(),
		selectedClass:   card.CandidateClass(),
		readiness:       QualificationTestReady,
		reason:          QualificationMatchedIdentityInventory,
		identityMatched: true,
		inventory:       &retained,
		missingEvidence: []string{"connected_smartlogger_identity_inventory_capture"},
		selection:       &selection,
	}
}

// EMMAQualificationCard is the stateless, offline documentary card for
// the EMMA family at gateway unit 0. A matched identity stays evidence-blocked
// on its named capability fixtures; the card sends nothing and grants no
// authority.
type EMMAQualificationCard struct{}

// EMMAQualificationInput is the already-read FC03 identity tuple the
// EMMA card evaluates. The texts are the decoded register strings; terminal
// padding is removed by the card.
type EMMAQualificationInput struct {
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

// NewEMMAQualificationCard returns the stateless card.
func NewEMMAQualificationCard() EMMAQualificationCard {
	return EMMAQualificationCard{}
}

// ID returns EMMAQualificationCardID.
func (EMMAQualificationCard) ID() string { return EMMAQualificationCardID }

// CandidateClass returns EMMACanonicalClass.
func (EMMAQualificationCard) CandidateClass() string { return EMMACanonicalClass }

// UnitID returns gateway unit 0, the only unit the card applies to.
func (EMMAQualificationCard) UnitID() uint8 { return 0 }

// ModelVariants are the observed models of the card. Admission follows the
// catalog's model rows and holds every model token of the EMMA family.
func (EMMAQualificationCard) ModelVariants() []string {
	return append([]string(nil), huaweiEMMAObservedModels...)
}

// Steps returns a fresh copy of the caller-run FC03 identity reads at unit 0:
// offering, model and software version.
func (EMMAQualificationCard) Steps() []QualificationStep {
	return []QualificationStep{
		{operation: "FC03_OFFERING", unitID: 0, offset: 30000, quantity: 15},
		{operation: "FC03_MODEL", unitID: 0, offset: 30222, quantity: 20},
		{operation: "FC03_FIRMWARE", unitID: 0, offset: 30035, quantity: 15},
	}
}

// MissingCapabilityEvidence returns a fresh copy of the named fixtures that
// keep a matched EMMA identity evidence-blocked.
func (EMMAQualificationCard) MissingCapabilityEvidence() []string {
	return []string{"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture"}
}

// EmptyResult returns the card's fail-closed result: insufficient evidence
// with an invalid identity and nothing retained.
func (card EMMAQualificationCard) EmptyResult() QualificationResult {
	return QualificationResult{candidateClass: card.CandidateClass(), readiness: QualificationInsufficientEvidence, reason: QualificationIdentityInvalid}
}

// Evaluate matches the identity tuple at unit 0 through
// EvaluateHuaweiEMMAOfflineIdentity. A match returns evidence-blocked with the
// capability fixtures as missing evidence and zero native facts; it never
// creates a capability. A mismatch returns EmptyResult, which keeps the
// catalog selection only when the software text was refused.
func (card EMMAQualificationCard) Evaluate(input EMMAQualificationInput) QualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	identity := EvaluateHuaweiEMMAOfflineIdentity(input.Offering, input.Model, input.Firmware)
	if !identity.Matched() {
		if identity.Reason() == EMMAOfflineFirmwareMismatch {
			selection := identity.Selection()
			rejected.selection = &selection
		}
		return rejected
	}
	selection := identity.Selection()
	return QualificationResult{
		candidateClass:  card.CandidateClass(),
		selectedClass:   card.CandidateClass(),
		modelVariant:    identity.ModelVariant(),
		readiness:       QualificationEvidenceBlocked,
		reason:          QualificationIdentityMatchedCapabilitiesMissing,
		identityMatched: true,
		missingEvidence: card.MissingCapabilityEvidence(),
		selection:       &selection,
	}
}

// SDongleQualificationCard is the retained record of the persistent
// non-response stop. Its result never changes. The shared resolution lets a
// matched result of the observed S-Dongle card supersede it, and nothing else.
type SDongleQualificationCard struct{}

// NewSDongleQualificationCard returns the stateless record.
func NewSDongleQualificationCard() SDongleQualificationCard {
	return SDongleQualificationCard{}
}

// ID returns SDongleQualificationCardID.
func (SDongleQualificationCard) ID() string { return SDongleQualificationCardID }

// CandidateClass returns SDongleCanonicalClass.
func (SDongleQualificationCard) CandidateClass() string { return SDongleCanonicalClass }

// UnitID returns unit 100, the unit of the recorded stopped run.
func (SDongleQualificationCard) UnitID() uint8 { return 100 }

// HardStop is always true: further I/O stays stopped until the missing
// evidence and action-time operator approval exist.
func (SDongleQualificationCard) HardStop() bool { return true }

// Steps returns nil: the record defines no read sequence.
func (SDongleQualificationCard) Steps() []QualificationStep {
	return nil
}

// Status is always QualificationEvidenceBlocked.
func (SDongleQualificationCard) Status() QualificationReadiness {
	return QualificationEvidenceBlocked
}

// RequiredConnectionContext returns a fresh copy of the connection facts a
// future attempt must confirm before any read.
func (SDongleQualificationCard) RequiredConnectionContext() []string {
	return []string{"endpoint", "port", "unit_id_100", "gateway_child_topology"}
}

// Result returns the fixed evidence-blocked, persistent non-response result
// with its named missing evidence. It never matches an identity.
func (card SDongleQualificationCard) Result() QualificationResult {
	return QualificationResult{
		candidateClass: card.CandidateClass(),
		readiness:      QualificationEvidenceBlocked,
		reason:         QualificationPersistentNonResponse,
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
// QualificationAmbiguous; anything else is insufficient evidence.
func ResolveHuaweiQualification(results ...QualificationResult) QualificationResult {
	results = withoutHuaweiSiblingNonMatches(results)
	results = withoutHuaweiSupersededMatches(results)
	matches := make([]QualificationResult, 0, len(results))
	classes := make([]string, 0, len(results))
	var persistentBlock *QualificationResult
	for _, result := range results {
		if result.Readiness() == QualificationEvidenceBlocked && result.Reason() == QualificationPersistentNonResponse {
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
		return QualificationResult{
			readiness:        QualificationAmbiguous,
			reason:           QualificationMultipleClasses,
			candidateClasses: classes,
		}
	}
	if len(matches) == 1 {
		return QualificationResult{
			readiness:        QualificationInsufficientEvidence,
			reason:           QualificationIdentityInvalid,
			candidateClasses: classes,
		}
	}
	return QualificationResult{readiness: QualificationInsufficientEvidence, reason: QualificationIdentityInvalid}
}

// withoutHuaweiSiblingNonMatches drops a non-matching result when another card
// of the same class matched. Cards of one class are alternative entries, so the
// rejection of one says nothing against the entry that matched. A persistent
// non-response record is a non-matching result of its own class: an exact tuple
// of that class contradicts it and supersedes it, and nothing else does. A
// match never hides a result of a different class.
func withoutHuaweiSiblingNonMatches(results []QualificationResult) []QualificationResult {
	matched := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.IdentityMatched() && result.SelectedClass() != "" {
			matched[result.SelectedClass()] = struct{}{}
		}
	}
	if len(matched) == 0 {
		return results
	}
	kept := make([]QualificationResult, 0, len(results))
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
func withoutHuaweiSupersededMatches(results []QualificationResult) []QualificationResult {
	complete := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.IdentityMatched() && result.SelectedClass() != "" && result.Readiness() == QualificationTestReady {
			complete[result.SelectedClass()] = struct{}{}
		}
	}
	if len(complete) == 0 {
		return results
	}
	kept := make([]QualificationResult, 0, len(results))
	for _, result := range results {
		_, superseded := complete[result.SelectedClass()]
		if superseded && result.IdentityMatched() && result.Readiness() == QualificationEvidenceBlocked &&
			result.Reason() == QualificationIdentityMatchedCapabilitiesMissing {
			continue
		}
		kept = append(kept, result)
	}
	return kept
}

// QualificationPackage is the stateless entry point to the published
// expected outcome of every Huawei qualification card.
type QualificationPackage struct{}

// NewQualificationPackage returns the stateless package.
func NewQualificationPackage() QualificationPackage { return QualificationPackage{} }

// ExpectedResult returns the expected-result document of the package.
func (QualificationPackage) ExpectedResult() QualificationExpectedResult {
	return QualificationExpectedResult{}
}

// QualificationExpectedResult is the expected status, card identity and
// missing evidence of each class card, documentary and observed. The values
// are read from the cards where a card exposes them.
type QualificationExpectedResult struct{}

// MarshalJSON encodes the expected result under the
// helianthus-huawei-qualification-readiness/v1 schema, with every
// default-denied and authority field fixed at its denied value.
func (QualificationExpectedResult) MarshalJSON() ([]byte, error) {
	smartLogger := NewSmartLoggerQualificationCard()
	observed := NewSmartLoggerObservedQualificationCard()
	emma := NewEMMAQualificationCard()
	sdongle := NewSDongleQualificationCard()
	sdongleObserved := NewSDongleObservedQualificationCard()
	emmaObserved := NewEMMAObservedQualificationCard()
	classes := []map[string]any{
		{
			"class": smartLogger.CandidateClass(), "card_id": smartLogger.ID(), "status": QualificationTestReady,
			"missing_evidence":    []string{"connected_smartlogger_identity_inventory_capture"},
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": observed.CandidateClass(), "card_id": observed.ID(), "status": QualificationTestReady,
			"missing_evidence":    observed.MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": emma.CandidateClass(), "card_id": emma.ID(), "status": QualificationEvidenceBlocked,
			"missing_evidence":    emma.MissingCapabilityEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": sdongle.CandidateClass(), "card_id": sdongle.ID(), "status": QualificationEvidenceBlocked,
			"missing_evidence":    sdongle.Result().MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": sdongleObserved.CandidateClass(), "card_id": sdongleObserved.ID(), "status": QualificationTestReady,
			"missing_evidence":    sdongleObserved.MissingEvidence(),
			"hardware_test_ready": false, "live_qualified": false, "automatic_request_count": 0, "native_fact_count": 0,
		},
		{
			"class": emmaObserved.CandidateClass(), "card_id": emmaObserved.ID(), "status": QualificationTestReady,
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
