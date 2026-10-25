package modbusreg

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

const (
	// HuaweiSmartLoggerObservedQualificationCardID identifies the card for the
	// one SmartLogger-family tuple that a read-only observation established.
	// It is a separate card from the documentary SmartLogger card: the model
	// string, the firmware string and the inventory dialect all differ, and no
	// exact tuple of one card is an alias of a tuple of the other.
	HuaweiSmartLoggerObservedQualificationCardID = "huawei.qualification.smartlogger.observed-readonly@1.0.0"

	huaweiSmartLoggerObservedVendor   = "HUAWEI"
	huaweiSmartLoggerObservedModel    = "Smart Logger"
	huaweiSmartLoggerObservedFirmware = "V300R023C10SPC620"
	huaweiSmartLoggerObservedSelfUnit = "0"

	// The observation fixes only the lower bound of an accepted response: a
	// PDU of 263 bytes holding one 254-byte entry. The finite upper cap is this
	// registry's choice. 1024 bytes holds three maximal entries and stays far
	// below the aggregate ceiling of 65536 bytes.
	huaweiSmartLoggerObservedMinResponseBytes = 263
	huaweiSmartLoggerObservedMaxResponseBytes = 1024
	huaweiSmartLoggerObservedMaxEntryBytes    = 254
	huaweiMEIResponseHeaderBytes              = 7
	huaweiClassicMaxPDUBytes                  = 253
	huaweiSmartLoggerObservedMaxEntries       = huaweiSmartLoggerInventoryMaxObjects
	huaweiSmartLoggerObservedMaxUnit          = 247
)

// HuaweiQualificationBasicIdentityConflict is the reason for a decoded basic
// identification reply that disagrees with the extended self entry.
const HuaweiQualificationBasicIdentityConflict HuaweiQualificationReason = "BASIC_IDENTITY_CONFLICT"

var errHuaweiSmartLoggerObservedInventoryInvalid = errors.New("invalid observed SmartLogger inventory")

// HuaweiInventoryPageAnomalies counts the response shapes that the observed
// dialect tolerates and a strict reader would reject.
type HuaweiInventoryPageAnomalies struct {
	// OversizeResponses counts responses above the classic 253-byte PDU limit.
	OversizeResponses int
	// DeclaredObjectFieldDiffers counts responses whose declared-object field
	// is not the number of objects present in that response.
	DeclaredObjectFieldDiffers int
	// NextObjectWithoutContinuation counts responses that name a next object
	// identifier although no continuation follows.
	NextObjectWithoutContinuation int
}

// HuaweiSmartLoggerObservedInventoryPage is one already-decoded extended
// identification response together with the wire facts the dialect tolerates.
type HuaweiSmartLoggerObservedInventoryPage struct {
	// ResponseBytes is the length of the response PDU. It must equal the length
	// implied by the objects.
	ResponseBytes int
	// DeclaredObjects is the declared-object field, retained and never compared
	// with the objects present.
	DeclaredObjects int
	More            bool
	NextObjectID    uint8
	Objects         []HuaweiSmartLoggerInventoryObject
}

// HuaweiSmartLoggerObservedInventoryInput is a detached, already-read extended
// inventory. It carries no endpoint, session or transport state.
type HuaweiSmartLoggerObservedInventoryInput struct {
	ChangeCounterBefore uint16
	ChangeCounterAfter  uint16
	Pages               []HuaweiSmartLoggerObservedInventoryPage
}

// HuaweiSmartLoggerObservedBasicReply is a decoded basic identification reply.
// A nil reply means the read was absent or could not be decoded, which neither
// negates nor supplies identity.
type HuaweiSmartLoggerObservedBasicReply struct {
	Objects []HuaweiSmartLoggerInventoryObject
}

// HuaweiSmartLoggerObservedQualificationInput is the evidence that the card
// evaluates. UnitID is the unit the counter and inventory reads were addressed to.
type HuaweiSmartLoggerObservedQualificationInput struct {
	UnitID    uint8
	Inventory HuaweiSmartLoggerObservedInventoryInput
	Basic     *HuaweiSmartLoggerObservedBasicReply
}

// HuaweiSmartLoggerObservedQualificationCard is an inert, offline card for the
// observed SmartLogger tuple: gateway unit 0, self-entry model "Smart Logger",
// firmware "V300R023C10SPC620" and self-entry unit "0". Every other tuple is
// insufficient evidence. The card sends nothing and grants no authority.
type HuaweiSmartLoggerObservedQualificationCard struct{}

func NewHuaweiSmartLoggerObservedQualificationCard() HuaweiSmartLoggerObservedQualificationCard {
	return HuaweiSmartLoggerObservedQualificationCard{}
}

func (HuaweiSmartLoggerObservedQualificationCard) ID() string {
	return HuaweiSmartLoggerObservedQualificationCardID
}
func (HuaweiSmartLoggerObservedQualificationCard) CandidateClass() string {
	return HuaweiSmartLoggerCanonicalClass
}
func (HuaweiSmartLoggerObservedQualificationCard) UnitID() uint8 { return 0 }
func (HuaweiSmartLoggerObservedQualificationCard) Vendor() string {
	return huaweiSmartLoggerObservedVendor
}
func (HuaweiSmartLoggerObservedQualificationCard) Model() string {
	return huaweiSmartLoggerObservedModel
}
func (HuaweiSmartLoggerObservedQualificationCard) Firmware() string {
	return huaweiSmartLoggerObservedFirmware
}
func (HuaweiSmartLoggerObservedQualificationCard) SelfEntryUnit() string {
	return huaweiSmartLoggerObservedSelfUnit
}

// MissingEvidence lists what remains open for this tuple: the direction of the
// SmartLogger and EMMA overlap that no observation covers.
func (HuaweiSmartLoggerObservedQualificationCard) MissingEvidence() []string {
	return []string{"pairwise_negative_overlap_with_emma_fixture"}
}

func (HuaweiSmartLoggerObservedQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_COUNTER_BEFORE", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY", unitID: 0, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_COUNTER_AFTER", unitID: 0, offset: 65521, quantity: 1},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_OPTIONAL", unitID: 0, objectID: 0x00},
	}
}

func (HuaweiSmartLoggerObservedQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiSmartLoggerInventoryMaxPages,
		MaxObjects:           huaweiSmartLoggerInventoryMaxObjects,
		MaxBytes:             huaweiSmartLoggerInventoryMaxBytes,
		MaxChildren:          huaweiSmartLoggerInventoryMaxChildren,
		MaxResponseBytes:     huaweiSmartLoggerObservedMaxResponseBytes,
	}
}

func (card HuaweiSmartLoggerObservedQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate returns a test-ready result only for the exact observed tuple. A
// wrong unit, an invalid or unstable inventory, a self entry outside the tuple
// and a disagreeing basic reply each produce an empty insufficient result with
// no retained child list.
func (card HuaweiSmartLoggerObservedQualificationCard) Evaluate(input HuaweiSmartLoggerObservedQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	if input.UnitID != card.UnitID() {
		return rejected
	}
	parsed, err := parseHuaweiSmartLoggerObservedInventory(input.UnitID, input.Inventory)
	if err != nil {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	if parsed.self.Attribute("model") != card.Model() || parsed.self.Attribute("software_version") != card.Firmware() ||
		parsed.self.Address() != card.SelfEntryUnit() {
		return rejected
	}
	corroborated, opaque := false, 0
	if input.Basic != nil {
		var agrees bool
		opaque, agrees = card.corroborate(*input.Basic)
		if !agrees {
			rejected.reason = HuaweiQualificationBasicIdentityConflict
			return rejected
		}
		corroborated = true
	}
	retained := parsed.inventory
	return HuaweiQualificationResult{
		candidateClass:     card.CandidateClass(),
		selectedClass:      card.CandidateClass(),
		modelVariant:       card.Model(),
		readiness:          HuaweiQualificationTestReady,
		reason:             HuaweiQualificationMatchedIdentityInventory,
		identityMatched:    true,
		inventory:          &retained,
		missingEvidence:    card.MissingEvidence(),
		pageAnomalies:      parsed.anomalies,
		basicCorroborated:  corroborated,
		basicOpaqueObjects: opaque,
	}
}

// corroborate compares the three basic-category objects with the exact tuple
// and counts the objects beyond that category, which are retained as opaque
// evidence and never interpreted.
func (card HuaweiSmartLoggerObservedQualificationCard) corroborate(reply HuaweiSmartLoggerObservedBasicReply) (int, bool) {
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
	if identity[0x00] != card.Vendor() || identity[0x01] != card.Model() || identity[0x02] != card.Firmware() {
		return 0, false
	}
	return opaque, true
}

type huaweiObservedInventory struct {
	inventory HuaweiSmartLoggerInventory
	self      HuaweiSmartLoggerInventoryChild
	anomalies HuaweiInventoryPageAnomalies
}

// parseHuaweiSmartLoggerObservedInventory requires an unchanged change counter
// and then validates the pages in the observed dialect.
func parseHuaweiSmartLoggerObservedInventory(gatewayUnit uint8, input HuaweiSmartLoggerObservedInventoryInput) (huaweiObservedInventory, error) {
	if input.ChangeCounterBefore != input.ChangeCounterAfter {
		return huaweiObservedInventory{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	return parseHuaweiObservedInventoryPages(gatewayUnit, input.Pages)
}

// parseHuaweiObservedInventoryPages validates the observed dialect. It
// differs from the documentary parser in the points the observation fixed: the
// count object counts the self entry, a response may exceed the classic frame
// limit up to a finite cap, the declared-object field is not compared, keys 9
// and 10 are known opaque keys and key order is free. The ceilings on pages,
// objects, children and aggregate bytes are unchanged. Any failure returns an
// empty result: there is no partial child list. A gateway without a change
// counter calls it directly.
func parseHuaweiObservedInventoryPages(gatewayUnit uint8, pages []HuaweiSmartLoggerObservedInventoryPage) (huaweiObservedInventory, error) {
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
		self        HuaweiSmartLoggerInventoryChild
		children    []HuaweiSmartLoggerInventoryChild
		anomalies   HuaweiInventoryPageAnomalies
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
		inventory: HuaweiSmartLoggerInventory{declaredChildren: declared - 1, children: append([]HuaweiSmartLoggerInventoryChild(nil), children...)},
		self:      self,
		anomalies: anomalies,
	}, nil
}

// parseHuaweiObservedEntry decodes one entry object: printable ASCII
// key=value pairs separated by semicolons, followed by terminal NUL or space
// padding. Keys 4, 9 and 10 are validated and never retained.
func parseHuaweiObservedEntry(object HuaweiSmartLoggerInventoryObject) (HuaweiSmartLoggerInventoryChild, error) {
	if len(object.Value) == 0 || len(object.Value) > huaweiSmartLoggerObservedMaxEntryBytes {
		return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	text := bytes.TrimRight(object.Value, "\x00 ")
	if len(text) == 0 || !isHuaweiSmartLoggerInventoryASCII(text) {
		return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
	}
	child := HuaweiSmartLoggerInventoryChild{objectID: object.ObjectID}
	seen := make(map[string]struct{})
	for _, attribute := range strings.Split(string(text), ";") {
		key, value, found := strings.Cut(attribute, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") {
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
		}
		if _, repeated := seen[key]; repeated {
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
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
		case "4", "9", "10":
		default:
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
		}
	}
	if child.model == "" || !isHuaweiObservedUnit(child.address) {
		return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerObservedInventoryInvalid
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
