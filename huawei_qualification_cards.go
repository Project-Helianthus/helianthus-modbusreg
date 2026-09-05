package modbusreg

import (
	"encoding/json"
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
func (HuaweiSmartLoggerQualificationCard) FirmwareTuples() []string {
	return []string{"V300R024C10SPC191", "V300R024C10SPC210"}
}
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
func (card HuaweiSmartLoggerQualificationCard) Evaluate(input HuaweiSmartLoggerQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	inventory, err := ParseHuaweiSmartLoggerOfflineInventory(input.Inventory)
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
			decoded, err := parseHuaweiSmartLoggerInventoryChild(object)
			if err != nil {
				rejected.reason = HuaweiQualificationInventoryInvalid
				return rejected
			}
			self = &decoded
		}
	}
	if self == nil || self.Attribute("model") != card.CandidateClass() || !containsHuaweiQualificationString(card.FirmwareTuples(), self.Attribute("software_version")) {
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
func (HuaweiEMMAQualificationCard) ID() string              { return HuaweiEMMAQualificationCardID }
func (HuaweiEMMAQualificationCard) CandidateClass() string  { return HuaweiEMMACanonicalClass }
func (HuaweiEMMAQualificationCard) UnitID() uint8           { return 0 }
func (HuaweiEMMAQualificationCard) ModelVariants() []string { return []string{"EMMA-A01", "EMMA-A02"} }
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
		return rejected
	}
	return HuaweiQualificationResult{
		candidateClass:  card.CandidateClass(),
		selectedClass:   card.CandidateClass(),
		modelVariant:    identity.ModelVariant(),
		readiness:       HuaweiQualificationEvidenceBlocked,
		reason:          HuaweiQualificationIdentityMatchedCapabilitiesMissing,
		identityMatched: true,
		missingEvidence: card.MissingCapabilityEvidence(),
	}
}

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

func containsHuaweiQualificationString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type HuaweiQualificationPackage struct{}

func NewHuaweiQualificationPackage() HuaweiQualificationPackage { return HuaweiQualificationPackage{} }
func (HuaweiQualificationPackage) ExpectedResult() HuaweiQualificationExpectedResult {
	return HuaweiQualificationExpectedResult{}
}

type HuaweiQualificationExpectedResult struct{}

func (HuaweiQualificationExpectedResult) MarshalJSON() ([]byte, error) {
	smartLogger := NewHuaweiSmartLoggerQualificationCard()
	emma := NewHuaweiEMMAQualificationCard()
	sdongle := NewHuaweiSDongleQualificationCard()
	classes := []map[string]any{
		{
			"class": smartLogger.CandidateClass(), "card_id": smartLogger.ID(), "status": HuaweiQualificationTestReady,
			"missing_evidence":    []string{"connected_smartlogger_identity_inventory_capture"},
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
