package modbusreg

import (
	"encoding/json"
	"slices"
	"sort"
)

const (
	HuaweiSmartLoggerCanonicalClass = "SmartLogger"

	HuaweiSmartLoggerQualificationCardID = "huawei.qualification.smartlogger.readonly@1.0.0"
	HuaweiEMMAQualificationCardID        = "huawei.qualification.emma.readonly@1.0.0"
	HuaweiSDongleQualificationCardID     = "huawei.qualification.sdongle.evidence-blocked@1.0.0"

	huaweiQualificationExpectedResultSchema = "helianthus-huawei-qualification-readiness/v1"
)

type HuaweiQualificationReadiness string

const (
	HuaweiQualificationInsufficientEvidence HuaweiQualificationReadiness = "INSUFFICIENT_EVIDENCE"
	HuaweiQualificationTestReady            HuaweiQualificationReadiness = "QUALIFICATION_TEST_READY"
	HuaweiQualificationEvidenceBlocked      HuaweiQualificationReadiness = "QUALIFICATION_EVIDENCE_BLOCKED"
	HuaweiQualificationAmbiguous            HuaweiQualificationReadiness = "AMBIGUOUS"
)

type HuaweiQualificationReason string

const (
	HuaweiQualificationIdentityInvalid                    HuaweiQualificationReason = "IDENTITY_INVALID"
	HuaweiQualificationInventoryInvalid                   HuaweiQualificationReason = "INVENTORY_INVALID"
	HuaweiQualificationMatchedIdentityInventory           HuaweiQualificationReason = "IDENTITY_INVENTORY_MATCHED"
	HuaweiQualificationIdentityMatchedCapabilitiesMissing HuaweiQualificationReason = "IDENTITY_MATCHED_CAPABILITIES_MISSING"
	HuaweiQualificationPersistentNonResponse              HuaweiQualificationReason = "LIVE_STOPPED_PERSISTENT_NON_RESPONSE"
	HuaweiQualificationMultipleClasses                    HuaweiQualificationReason = "MULTIPLE_CLASSES"
)

type HuaweiQualificationInventoryLimits struct {
	DeadlineMilliseconds uint32
	MaxPages             uint32
	MaxObjects           uint32
	MaxBytes             uint32
	MaxChildren          uint32
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

func (step HuaweiQualificationStep) Operation() string { return step.operation }
func (step HuaweiQualificationStep) UnitID() uint8     { return step.unitID }
func (step HuaweiQualificationStep) Offset() uint16    { return step.offset }
func (step HuaweiQualificationStep) Quantity() uint16  { return step.quantity }
func (step HuaweiQualificationStep) ObjectID() uint8   { return step.objectID }

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

	// selection is the catalogue decision over the gateway's own software
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

func (result HuaweiQualificationResult) CandidateClass() string { return result.candidateClass }
func (result HuaweiQualificationResult) SelectedClass() string  { return result.selectedClass }
func (result HuaweiQualificationResult) ModelVariant() string   { return result.modelVariant }
func (result HuaweiQualificationResult) Readiness() HuaweiQualificationReadiness {
	if result.readiness == "" {
		return HuaweiQualificationInsufficientEvidence
	}
	return result.readiness
}
func (result HuaweiQualificationResult) Reason() HuaweiQualificationReason {
	if result.reason == "" {
		return HuaweiQualificationIdentityInvalid
	}
	return result.reason
}
func (result HuaweiQualificationResult) IdentityMatched() bool { return result.identityMatched }
func (result HuaweiQualificationResult) PageAnomalies() HuaweiInventoryPageAnomalies {
	return result.pageAnomalies
}
func (result HuaweiQualificationResult) BasicCorroborated() bool { return result.basicCorroborated }
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

// Selection is the catalogue decision over the gateway's own software text:
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
func (result HuaweiQualificationResult) QualificationTestReady() bool {
	return result.Readiness() == HuaweiQualificationTestReady
}
func (HuaweiQualificationResult) HardwareTestReady() bool         { return false }
func (HuaweiQualificationResult) DefaultDenied() bool             { return true }
func (HuaweiQualificationResult) CatalogRegistered() bool         { return false }
func (HuaweiQualificationResult) AutomaticRuntimeAdmission() bool { return false }
func (HuaweiQualificationResult) LiveQualified() bool             { return false }
func (HuaweiQualificationResult) SupportClaim() bool              { return false }
func (HuaweiQualificationResult) WriteAuthority() bool            { return false }
func (HuaweiQualificationResult) NativeFactCount() int            { return 0 }
func (HuaweiQualificationResult) AutomaticRequestCount() int      { return 0 }
func (HuaweiQualificationResult) FallbackAttempted() bool         { return false }
func (result HuaweiQualificationResult) MissingEvidence() []string {
	return append([]string(nil), result.missingEvidence...)
}
func (result HuaweiQualificationResult) CandidateClasses() []string {
	return append([]string(nil), result.candidateClasses...)
}
func (result HuaweiQualificationResult) Inventory() (HuaweiGatewayInventory, bool) {
	if result.inventory == nil {
		return HuaweiGatewayInventory{}, false
	}
	inventory := *result.inventory
	inventory.children = append([]HuaweiSmartLoggerInventoryChild(nil), result.inventory.children...)
	return inventory, true
}

type HuaweiSmartLoggerQualificationCard struct{}

type HuaweiSmartLoggerQualificationInput struct {
	UnitID    uint8
	Inventory HuaweiSmartLoggerOfflineInventoryInput
}

func NewHuaweiSmartLoggerQualificationCard() HuaweiSmartLoggerQualificationCard {
	return HuaweiSmartLoggerQualificationCard{}
}

func (HuaweiSmartLoggerQualificationCard) ID() string { return HuaweiSmartLoggerQualificationCardID }
func (HuaweiSmartLoggerQualificationCard) CandidateClass() string {
	return HuaweiSmartLoggerCanonicalClass
}
func (HuaweiSmartLoggerQualificationCard) UnitID() uint8 { return 0 }
func (HuaweiSmartLoggerQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
	}
}
func (HuaweiSmartLoggerQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiSmartLoggerInventoryMaxPages,
		MaxObjects:           huaweiSmartLoggerInventoryMaxObjects,
		MaxBytes:             huaweiSmartLoggerInventoryMaxBytes,
		MaxChildren:          huaweiSmartLoggerInventoryMaxChildren,
	}
}
func (card HuaweiSmartLoggerQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate admits a stable, complete inventory at unit 0 whose self entry is
// exactly the model SmartLogger and whose software text the catalogue
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

type HuaweiEMMAQualificationCard struct{}

type HuaweiEMMAQualificationInput struct {
	UnitID   uint8
	Offering string
	Model    string
	Firmware string
}

func NewHuaweiEMMAQualificationCard() HuaweiEMMAQualificationCard {
	return HuaweiEMMAQualificationCard{}
}
func (HuaweiEMMAQualificationCard) ID() string             { return HuaweiEMMAQualificationCardID }
func (HuaweiEMMAQualificationCard) CandidateClass() string { return HuaweiEMMACanonicalClass }
func (HuaweiEMMAQualificationCard) UnitID() uint8          { return 0 }

// ModelVariants are the observed models of the card. Admission follows the
// catalogue's model rows and holds every model token of the EMMA family.
func (HuaweiEMMAQualificationCard) ModelVariants() []string {
	return append([]string(nil), huaweiEMMAObservedModels...)
}
func (HuaweiEMMAQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_OFFERING", unitID: 0, offset: 30000, quantity: 15},
		{operation: "FC03_MODEL", unitID: 0, offset: 30222, quantity: 20},
		{operation: "FC03_FIRMWARE", unitID: 0, offset: 30035, quantity: 15},
	}
}
func (HuaweiEMMAQualificationCard) MissingCapabilityEvidence() []string {
	return []string{"sanitized_readonly_capability_fixture", "negative_overlap_with_smartlogger", "model_specific_capability_fixture"}
}
func (card HuaweiEMMAQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}
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

func NewHuaweiSDongleQualificationCard() HuaweiSDongleQualificationCard {
	return HuaweiSDongleQualificationCard{}
}
func (HuaweiSDongleQualificationCard) ID() string             { return HuaweiSDongleQualificationCardID }
func (HuaweiSDongleQualificationCard) CandidateClass() string { return HuaweiSDongleCanonicalClass }
func (HuaweiSDongleQualificationCard) UnitID() uint8          { return 100 }
func (HuaweiSDongleQualificationCard) HardStop() bool         { return true }
func (HuaweiSDongleQualificationCard) Steps() []HuaweiQualificationStep {
	return nil
}
func (HuaweiSDongleQualificationCard) Status() HuaweiQualificationReadiness {
	return HuaweiQualificationEvidenceBlocked
}
func (HuaweiSDongleQualificationCard) RequiredConnectionContext() []string {
	return []string{"endpoint", "port", "unit_id_100", "gateway_child_topology"}
}
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

type HuaweiQualificationPackage struct{}

func NewHuaweiQualificationPackage() HuaweiQualificationPackage { return HuaweiQualificationPackage{} }
func (HuaweiQualificationPackage) ExpectedResult() HuaweiQualificationExpectedResult {
	return HuaweiQualificationExpectedResult{}
}

type HuaweiQualificationExpectedResult struct{}

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
