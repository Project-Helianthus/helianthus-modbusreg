package huawei

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

const (
	// SmartLoggerObservedQualificationCardID identifies the card for the
	// SmartLogger-family entry that a read-only observation established. It is
	// a separate card from the documentary SmartLogger card: the model string,
	// the software floor and the inventory dialect all differ, and the model of
	// one card is not an alias of the model of the other.
	SmartLoggerObservedQualificationCardID = "huawei.qualification.smartlogger.observed-readonly@1.0.0"

	huaweiSmartLoggerObservedVendor   = "HUAWEI"
	huaweiSmartLoggerObservedModel    = "Smart Logger"
	huaweiSmartLoggerObservedFirmware = "V300R023C10SPC620"
	huaweiSmartLoggerObservedSelfUnit = "0"

	// The observation fixes only the lower bound of an accepted response: a
	// PDU of 263 bytes holding one 254-byte entry. The finite upper cap is this
	// registry's resource policy, shared by every reader of the observed
	// dialect. 1024 bytes holds three maximal entries and stays far below the
	// aggregate ceiling of 65536 bytes.
	huaweiSmartLoggerObservedMinResponseBytes = 263
	huaweiSmartLoggerObservedMaxResponseBytes = 1024
	huaweiSmartLoggerObservedMaxEntryBytes    = 254
	huaweiMEIResponseHeaderBytes              = 7
	huaweiClassicMaxPDUBytes                  = 253
	huaweiSmartLoggerObservedMaxEntries       = huaweiSmartLoggerInventoryMaxObjects
	huaweiSmartLoggerObservedMaxUnit          = 247
)

// QualificationBasicIdentityConflict is the reason for a decoded basic
// identification reply that disagrees with the extended self entry.
const QualificationBasicIdentityConflict QualificationReason = "BASIC_IDENTITY_CONFLICT"

var errHuaweiSmartLoggerObservedInventoryInvalid = errors.New("invalid observed SmartLogger inventory")

// InventoryPageAnomalies counts the response shapes that the observed
// dialect tolerates and a strict reader would reject.
type InventoryPageAnomalies struct {
	// OversizeResponses counts responses above the classic 253-byte PDU limit.
	OversizeResponses int
	// DeclaredObjectFieldDiffers counts responses whose declared-object field
	// is not the number of objects present in that response.
	DeclaredObjectFieldDiffers int
	// NextObjectWithoutContinuation counts responses that name a next object
	// identifier although no continuation follows.
	NextObjectWithoutContinuation int
}

// SmartLoggerObservedInventoryPage is one already-decoded extended
// identification response together with the wire facts the dialect tolerates.
type SmartLoggerObservedInventoryPage struct {
	// ResponseBytes is the length of the response PDU. It must equal the length
	// implied by the objects.
	ResponseBytes int
	// DeclaredObjects is the declared-object field, retained and never compared
	// with the objects present.
	DeclaredObjects int
	More            bool
	NextObjectID    uint8
	Objects         []SmartLoggerInventoryObject
}

// SmartLoggerObservedInventoryInput is a detached, already-read extended
// inventory. It carries no endpoint, session or transport state.
type SmartLoggerObservedInventoryInput struct {
	ChangeCounterBefore uint16
	ChangeCounterAfter  uint16
	Pages               []SmartLoggerObservedInventoryPage
}

// SmartLoggerObservedBasicReply is a decoded basic identification reply.
// A nil reply means the read was absent or could not be decoded, which neither
// negates nor supplies identity.
type SmartLoggerObservedBasicReply struct {
	Objects []SmartLoggerInventoryObject
}

// SmartLoggerObservedQualificationInput is the evidence that the card
// evaluates. UnitID is the unit the counter and inventory reads were addressed to.
type SmartLoggerObservedQualificationInput struct {
	UnitID    uint8
	Inventory SmartLoggerObservedInventoryInput
	Basic     *SmartLoggerObservedBasicReply
}

// SmartLoggerObservedQualificationCard is an inert, offline card for the
// observed SmartLogger entry: gateway unit 0, self-entry model "Smart Logger",
// self-entry unit "0", and a self-entry software text that the catalog
// decision admits and that is at or above the observed build
// "V300R023C10SPC620", the floor of the observed facts, within the V300 line
// or of a line that no floor documents. The observed build is graded Proven and
// every other admitted build Hypothesis. Every other entry is insufficient
// evidence. The card sends nothing and grants no authority.
type SmartLoggerObservedQualificationCard struct{}

// NewSmartLoggerObservedQualificationCard returns the stateless card.
// The observation contract is
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/huawei/smartlogger-observed-inventory-v1.md.
func NewSmartLoggerObservedQualificationCard() SmartLoggerObservedQualificationCard {
	return SmartLoggerObservedQualificationCard{}
}

// ID returns SmartLoggerObservedQualificationCardID.
func (SmartLoggerObservedQualificationCard) ID() string {
	return SmartLoggerObservedQualificationCardID
}

// CandidateClass returns SmartLoggerCanonicalClass.
func (SmartLoggerObservedQualificationCard) CandidateClass() string {
	return SmartLoggerCanonicalClass
}

// UnitID returns the gateway unit identifier the counter and inventory reads
// must carry.
func (SmartLoggerObservedQualificationCard) UnitID() uint8 { return 0 }

// Vendor returns the vendor object an optional basic identification reply must
// carry to corroborate the self entry.
func (SmartLoggerObservedQualificationCard) Vendor() string {
	return huaweiSmartLoggerObservedVendor
}

// Model returns the exact self-entry model of the observed dialect. It is not
// an alias of the documentary card's model.
func (SmartLoggerObservedQualificationCard) Model() string {
	return huaweiSmartLoggerObservedModel
}

// Firmware is the observed build, the floor of the observed facts.
func (SmartLoggerObservedQualificationCard) Firmware() string {
	return huaweiSmartLoggerObservedFirmware
}

// SelfEntryUnit returns the unit text the self entry's address attribute must
// carry.
func (SmartLoggerObservedQualificationCard) SelfEntryUnit() string {
	return huaweiSmartLoggerObservedSelfUnit
}

// MissingEvidence lists what remains open for this tuple: the direction of the
// SmartLogger and EMMA overlap that no observation covers.
func (SmartLoggerObservedQualificationCard) MissingEvidence() []string {
	return []string{"pairwise_negative_overlap_with_emma_fixture"}
}

// Steps is the replay procedure: the change counter before and after the
// extended inventory read, then the optional basic identification read, all at
// the gateway unit.
func (SmartLoggerObservedQualificationCard) Steps() []QualificationStep {
	return []QualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_OPTIONAL", unitID: 0, objectID: 0x00},
	}
}

// Limits are the resource policy of the gateway contract and the shared
// response cap of the observed dialect. The observed response length is a
// lower bound, never a cap.
func (SmartLoggerObservedQualificationCard) Limits() QualificationInventoryLimits {
	return QualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiSmartLoggerInventoryMaxPages,
		MaxObjects:           huaweiSmartLoggerInventoryMaxObjects,
		MaxBytes:             huaweiSmartLoggerInventoryMaxBytes,
		MaxChildren:          huaweiSmartLoggerInventoryMaxChildren,
		MaxResponseBytes:     huaweiSmartLoggerObservedMaxResponseBytes,
	}
}

// EmptyResult returns the base rejection: insufficient evidence with an
// identity-invalid reason, no selection and no child list. Evaluate starts
// from it, may replace the reason with a more specific one and may attach the
// catalog selection and floor relation of the self-entry software text.
func (card SmartLoggerObservedQualificationCard) EmptyResult() QualificationResult {
	return QualificationResult{candidateClass: card.CandidateClass(), readiness: QualificationInsufficientEvidence, reason: QualificationIdentityInvalid}
}

// Evaluate returns a test-ready result for the observed entry. A wrong unit, an
// invalid or unstable inventory, a self entry with another model or unit, a
// software text that the catalog decision refuses or that is below the
// observed build, and a disagreeing basic reply each produce an empty
// insufficient result with no retained child list.
func (card SmartLoggerObservedQualificationCard) Evaluate(input SmartLoggerObservedQualificationInput) QualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	parsed, err := parseHuaweiSmartLoggerObservedInventory(input.UnitID, input.Inventory)
	if err != nil {
		rejected.reason = QualificationInventoryInvalid
		return rejected
	}
	if parsed.self.Attribute("model") != card.Model() || parsed.self.Address() != card.SelfEntryUnit() {
		return rejected
	}
	software := parsed.self.Attribute("software_version")
	selection := DecideHuaweiGatewaySelection(GatewaySelectionInput{
		Family:   card.CandidateClass(),
		Scope:    GatewayScope{Endpoint: GatewayEndpointSelf, Unit: input.UnitID},
		Software: software,
	})
	rejected.selection = &selection
	if !selection.Admitted() || selection.Build() == "" {
		return rejected
	}
	relation := huaweiGatewayRelationOfFloor(card.CandidateClass(), software, card.Firmware())
	rejected.observedFloor, rejected.observedRelation = card.Firmware(), relation
	if relation == GatewayRelationBelow || relation == GatewayRelationNotEvaluated {
		return rejected
	}
	selection = selection.withObservedBuild(card.Firmware())
	corroborated, opaque := false, 0
	if input.Basic != nil {
		var agrees bool
		opaque, agrees = card.corroborate(*input.Basic, software)
		if !agrees {
			rejected.reason = QualificationBasicIdentityConflict
			return rejected
		}
		corroborated = true
	}
	retained := parsed.inventory
	return QualificationResult{
		candidateClass:     card.CandidateClass(),
		selectedClass:      card.CandidateClass(),
		modelVariant:       card.Model(),
		readiness:          QualificationTestReady,
		reason:             QualificationMatchedIdentityInventory,
		identityMatched:    true,
		inventory:          &retained,
		missingEvidence:    card.MissingEvidence(),
		pageAnomalies:      parsed.anomalies,
		basicCorroborated:  corroborated,
		basicOpaqueObjects: opaque,
		selection:          &selection,
		observedFloor:      card.Firmware(),
		observedRelation:   relation,
	}
}

// corroborate compares the three basic-category objects with the vendor, the
// model and the self entry's software text, and counts the objects beyond that
// category, which are retained as opaque evidence and never interpreted.
func (card SmartLoggerObservedQualificationCard) corroborate(reply SmartLoggerObservedBasicReply, software string) (int, bool) {
	seen := make(map[uint8]struct{}, len(reply.Objects))
	identity := make(map[uint8]string, 3)
	opaque := 0
	for _, object := range reply.Objects {
		if _, repeated := seen[object.ObjectID]; repeated {
			return 0, false
		}
		seen[object.ObjectID] = struct{}{}
		if object.ObjectID <= 0x02 {
			identity[object.ObjectID] = trimHuaweiTerminalPadding(string(object.Value))
			continue
		}
		opaque++
	}
	if identity[0x00] != card.Vendor() || identity[0x01] != card.Model() || identity[0x02] != software {
		return 0, false
	}
	return opaque, true
}

type huaweiObservedInventory struct {
	inventory SmartLoggerInventory
	self      SmartLoggerInventoryChild
	anomalies InventoryPageAnomalies
}

// parseHuaweiSmartLoggerObservedInventory requires an unchanged change counter
// and then validates the pages in the observed dialect.
func parseHuaweiSmartLoggerObservedInventory(gatewayUnit uint8, input SmartLoggerObservedInventoryInput) (huaweiObservedInventory, error) {
	if input.ChangeCounterBefore != input.ChangeCounterAfter {
		return huaweiObservedInventory{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	return parseHuaweiObservedInventoryPages(gatewayUnit, input.Pages)
}

// parseHuaweiObservedInventoryPages validates the observed dialect. It
// differs from the documentary parser in the points the observation fixed: the
// count object counts the self entry, a response may exceed the classic frame
// limit up to the resource cap, the declared-object field is not compared, a
// key outside the named ones is retained as opaque native evidence and key
// order is free. Every observed page count, entry length, key set and response
// size is a lower bound; the ceilings on pages, objects, children, response
// and aggregate bytes are resource policy. Any failure returns an empty
// result: there is no partial child list. A gateway without a change counter
// calls it directly.
func parseHuaweiObservedInventoryPages(gatewayUnit uint8, pages []SmartLoggerObservedInventoryPage) (huaweiObservedInventory, error) {
	gatewayAddress := strconv.Itoa(int(gatewayUnit))
	invalid := func() (huaweiObservedInventory, error) {
		return huaweiObservedInventory{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	if len(pages) == 0 || len(pages) > huaweiSmartLoggerInventoryMaxPages {
		return invalid()
	}

	var (
		expected    = uint8(huaweiSmartLoggerInventoryStartObject)
		declared    = -1
		entries     int
		aggregate   int
		self        SmartLoggerInventoryChild
		children    []SmartLoggerInventoryChild
		anomalies   InventoryPageAnomalies
		seenAddress = make(map[string]struct{})
	)
	for pageIndex, page := range pages {
		if len(page.Objects) == 0 || page.DeclaredObjects < 0 || page.DeclaredObjects > 255 {
			return invalid()
		}
		responseBytes := huaweiMEIResponseHeaderBytes
		for objectIndex, object := range page.Objects {
			if object.ObjectID != expected {
				return invalid()
			}
			responseBytes += 2 + len(object.Value)
			aggregate += 2 + len(object.Value)
			if aggregate > huaweiSmartLoggerInventoryMaxBytes {
				return invalid()
			}
			if pageIndex == 0 && objectIndex == 0 {
				if object.ObjectID != huaweiSmartLoggerInventoryStartObject || len(object.Value) != 1 ||
					object.Value[0] == 0 || int(object.Value[0]) > huaweiSmartLoggerObservedMaxEntries {
					return invalid()
				}
				declared = int(object.Value[0])
			} else {
				entries++
				if entries > huaweiSmartLoggerObservedMaxEntries {
					return invalid()
				}
				child, err := parseHuaweiObservedEntry(object)
				if err != nil {
					return invalid()
				}
				if entries == 1 {
					if child.address != gatewayAddress {
						return invalid()
					}
					self = child
				} else {
					if child.address == "0" || child.address == gatewayAddress {
						return invalid()
					}
					if _, repeated := seenAddress[child.address]; repeated {
						return invalid()
					}
					seenAddress[child.address] = struct{}{}
					children = append(children, child)
				}
			}
			expected++
		}
		if responseBytes != page.ResponseBytes || responseBytes > huaweiSmartLoggerObservedMaxResponseBytes {
			return invalid()
		}
		if responseBytes > huaweiClassicMaxPDUBytes {
			anomalies.OversizeResponses++
		}
		if page.DeclaredObjects != len(page.Objects) {
			anomalies.DeclaredObjectFieldDiffers++
		}
		if page.More {
			if page.NextObjectID != expected || pageIndex == len(pages)-1 {
				return invalid()
			}
			continue
		}
		if page.NextObjectID != 0 || pageIndex != len(pages)-1 {
			return invalid()
		}
	}
	if declared < 0 || entries != declared {
		return invalid()
	}
	return huaweiObservedInventory{
		inventory: SmartLoggerInventory{declaredChildren: declared - 1, children: append([]SmartLoggerInventoryChild(nil), children...)},
		self:      self,
		anomalies: anomalies,
	}, nil
}

// parseHuaweiObservedEntry decodes one entry object: printable ASCII
// key=value pairs separated by semicolons, followed by terminal NUL or space
// padding. Key 4 is validated and never retained. A key outside the named ones,
// keys 9 and 10 among them, is retained as opaque native evidence when it is a
// canonical attribute number; any other key, a repeated key or a malformed
// pair rejects the entry.
func parseHuaweiObservedEntry(object SmartLoggerInventoryObject) (SmartLoggerInventoryChild, error) {
	if len(object.Value) == 0 || len(object.Value) > huaweiSmartLoggerObservedMaxEntryBytes {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	text := bytes.TrimRight(object.Value, "\x00 ")
	if len(text) == 0 || !isHuaweiSmartLoggerInventoryASCII(text) {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	child := SmartLoggerInventoryChild{objectID: object.ObjectID}
	seen := make(map[string]struct{})
	for _, attribute := range strings.Split(string(text), ";") {
		key, value, found := strings.Cut(attribute, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
		}
		if _, repeated := seen[key]; repeated {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
		}
		seen[key] = struct{}{}
		switch key {
		case "1":
			child.model = value
		case "2":
			child.softwareVersion = value
		case "3":
			child.protocolVersion = value
		case "5":
			child.address = value
		case "6":
			child.featureVersion = value
		case "8":
			child.productType = value
		case "4":
		default:
			if _, ok := huaweiDescriptorKeyNumber(key); !ok {
				return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
			}
			child.retainOpaque(key, value)
		}
	}
	if child.model == "" || !isHuaweiObservedUnit(child.address) {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	return child, nil
}

// isHuaweiObservedUnit accepts the canonical unsigned decimal form of a unit
// from 0 through 247: no sign, no space and no leading zero.
func isHuaweiObservedUnit(value string) bool {
	if len(value) == 0 || len(value) > 3 || (len(value) > 1 && value[0] == '0') {
		return false
	}
	unit := 0
	for _, digit := range []byte(value) {
		if digit < '0' || digit > '9' {
			return false
		}
		unit = unit*10 + int(digit-'0')
	}
	return unit <= huaweiSmartLoggerObservedMaxUnit
}
