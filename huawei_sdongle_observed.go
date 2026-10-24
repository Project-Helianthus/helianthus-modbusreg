package modbusreg

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"strings"
)

const (
	// HuaweiSDongleObservedQualificationCardID identifies the card for the one
	// S-Dongle path that a read-only observation established: a gateway unit
	// that refuses basic device identification and answers its status reads,
	// fronting one inverter child. It is a separate card from the retained
	// evidence-blocked S-Dongle card, which keeps its identity and behavior.
	HuaweiSDongleObservedQualificationCardID = "huawei.qualification.sdongle.observed-readonly@1.0.0"

	// HuaweiQualificationGatewayBehaviourMismatch is the reason for gateway-unit
	// reads that fall outside the observed behavior.
	HuaweiQualificationGatewayBehaviourMismatch HuaweiQualificationReason = "GATEWAY_BEHAVIOUR_MISMATCH"

	huaweiSDongleObservedGatewayUnit  = 100
	huaweiSDongleObservedChildUnit    = 1
	huaweiSDongleObservedBasicRefusal = 1
	huaweiSDongleObservedChildModel   = "SUN2000-8KTL-M1"
	huaweiSDongleObservedChildVendor  = "Huawei"
	huaweiSDongleObservedChildFamily  = "SUN2000"

	huaweiSDongleObservedSearchType   = 4
	huaweiSDongleObservedSearchState  = 0
	huaweiSDongleObservedModelExtent  = 30
	huaweiSDongleObservedCountObject  = huaweiSmartLoggerInventoryStartObject
	huaweiSDongleObservedEntryObject  = huaweiSmartLoggerInventoryStartObject + 1
	huaweiSDongleObservedConformity   = 0x03
	huaweiSDongleObservedReadCode     = 3
	huaweiSDongleObservedMaxObjects   = 2
	huaweiSDongleObservedMaxObjectSum = huaweiClassicMaxPDUBytes - huaweiMEIResponseHeaderBytes
)

var errHuaweiSDongleObservedInventoryInvalid = errors.New("invalid observed S-Dongle child inventory")

// huaweiSDongleObservedProtocolWords are the gateway protocol words as read.
// Their encoding is not established, so they are compared as words and never
// decoded.
var huaweiSDongleObservedProtocolWords = []uint16{50, 0}

// huaweiSDongleObservedEntryKeys is the key set of the one observed child
// entry. Order is not validated.
var huaweiSDongleObservedEntryKeys = []string{"1", "2", "3", "4", "5", "6"}

// HuaweiSDongleObservedBasicReply is a decoded basic identification reply.
type HuaweiSDongleObservedBasicReply struct {
	Objects []HuaweiGatewayInventoryObject
}

// HuaweiSDongleObservedBasicOutcome is the outcome of one basic identification
// read: either an exception code or a decoded reply. A zero value means that the
// read was absent, which is never the observed behavior.
type HuaweiSDongleObservedBasicOutcome struct {
	ExceptionCode uint8
	Reply         *HuaweiSDongleObservedBasicReply
}

// HuaweiSDongleObservedGatewayReads are the already-read gateway-unit replies.
// UnitID is the unit every one of them was addressed to. Each word slice is the
// register words of one answered read; an exception or a timeout is an empty
// slice.
type HuaweiSDongleObservedGatewayReads struct {
	UnitID       uint8
	Basic        HuaweiSDongleObservedBasicOutcome
	SearchStatus []uint16
	Protocol     []uint16
	SearchBefore []uint16
	SearchAfter  []uint16
	Capacity     []uint16
}

// HuaweiSDongleObservedChildInventoryPage is one already-decoded extended
// identification response together with the wire facts the dialect tolerates.
type HuaweiSDongleObservedChildInventoryPage struct {
	ReadDeviceIDCode uint8
	Conformity       uint8
	// ResponseBytes is the length of the response PDU. It must equal the length
	// implied by the objects.
	ResponseBytes int
	// DeclaredObjects is the declared-object field, retained and never compared.
	DeclaredObjects int
	More            bool
	// NextObjectID is retained and counted when it is set without a continuation.
	NextObjectID uint8
	Objects      []HuaweiGatewayInventoryObject
}

// HuaweiSDongleObservedChildReads are the already-read replies of the child
// unit. ModelBlock is the raw extent of the FC03 offset 30000 read.
type HuaweiSDongleObservedChildReads struct {
	UnitID     uint8
	ModelBlock []byte
	Basic      *HuaweiSDongleObservedBasicReply
	Inventory  HuaweiSDongleObservedChildInventoryPage
}

// HuaweiSDongleObservedQualificationInput is the evidence that the card
// evaluates. It carries answered replies only; the timing of an earlier
// unanswered attempt is not an input.
type HuaweiSDongleObservedQualificationInput struct {
	Gateway HuaweiSDongleObservedGatewayReads
	Child   HuaweiSDongleObservedChildReads
}

// HuaweiSDongleObservedQualificationCard is an inert, offline card for the
// observed S-Dongle single-inverter tuple:
//
//   - gateway unit 100 answers basic device identification with exception 1,
//     answers the search status read with 0, the protocol read with the words 50
//     and 0, the search sequence read with type 4, state 0 and an equal sequence
//     before and after the child reads, and a capacity that covers one child;
//   - child unit 1 answers the model read with the text SUN2000-8KTL-M1, a NUL,
//     opaque printable residue and NUL padding; answers basic identification with
//     vendor Huawei and product SUN2000; and answers the extended inventory with
//     one page, a count of 1 and one entry whose model equals the model read and
//     whose unit attribute equals the child unit.
//
// Every other tuple is insufficient evidence. The card sends nothing and grants
// no authority.
type HuaweiSDongleObservedQualificationCard struct{}

func NewHuaweiSDongleObservedQualificationCard() HuaweiSDongleObservedQualificationCard {
	return HuaweiSDongleObservedQualificationCard{}
}

func (HuaweiSDongleObservedQualificationCard) ID() string {
	return HuaweiSDongleObservedQualificationCardID
}
func (HuaweiSDongleObservedQualificationCard) CandidateClass() string {
	return HuaweiSDongleCanonicalClass
}
func (HuaweiSDongleObservedQualificationCard) UnitID() uint8 { return huaweiSDongleObservedGatewayUnit }
func (HuaweiSDongleObservedQualificationCard) ChildUnit() uint8 {
	return huaweiSDongleObservedChildUnit
}
func (HuaweiSDongleObservedQualificationCard) ChildModel() string {
	return huaweiSDongleObservedChildModel
}
func (HuaweiSDongleObservedQualificationCard) ChildVendor() string {
	return huaweiSDongleObservedChildVendor
}
func (HuaweiSDongleObservedQualificationCard) ChildFamily() string {
	return huaweiSDongleObservedChildFamily
}
func (HuaweiSDongleObservedQualificationCard) GatewayBasicException() uint8 {
	return huaweiSDongleObservedBasicRefusal
}
func (HuaweiSDongleObservedQualificationCard) GatewayProtocolWords() []uint16 {
	return slices.Clone(huaweiSDongleObservedProtocolWords)
}

// MissingEvidence lists what remains open for this tuple: the gateway's own
// product string, which it refuses to give; the encoding of the protocol words;
// and the direction of the EMMA overlap that no observation covers.
func (HuaweiSDongleObservedQualificationCard) MissingEvidence() []string {
	return []string{
		"sanitized_basic_mei_product_model_fixture",
		"exact_protocol_version_encoding_fixture",
		"pairwise_negative_overlap_with_emma_fixture",
	}
}

// Steps is the replay procedure. Every step is an FC03 or an FC2B / MEI 0x0e
// read. Extended inventory is read at the child unit only, never at the gateway
// unit.
func (HuaweiSDongleObservedQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_GATEWAY_EXPECT_EXCEPTION", unitID: huaweiSDongleObservedGatewayUnit, objectID: 0x00},
		{operation: "FC03_GATEWAY_SEARCH_STATUS", unitID: huaweiSDongleObservedGatewayUnit, offset: 37411, quantity: 1},
		{operation: "FC03_GATEWAY_PROTOCOL", unitID: huaweiSDongleObservedGatewayUnit, offset: 30068, quantity: 2},
		{operation: "FC03_GATEWAY_SEARCH_SEQUENCE_BEFORE", unitID: huaweiSDongleObservedGatewayUnit, offset: 37410, quantity: 3},
		{operation: "FC03_GATEWAY_CAPACITY", unitID: huaweiSDongleObservedGatewayUnit, offset: 37429, quantity: 1},
		{operation: "FC03_CHILD_MODEL", unitID: huaweiSDongleObservedChildUnit, offset: 30000, quantity: 15},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_CHILD", unitID: huaweiSDongleObservedChildUnit, objectID: 0x00},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY_CHILD", unitID: huaweiSDongleObservedChildUnit, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_GATEWAY_SEARCH_SEQUENCE_AFTER", unitID: huaweiSDongleObservedGatewayUnit, offset: 37410, quantity: 3},
	}
}

// Limits narrows the S-Dongle child inventory ceilings to the one observed
// page: the classic frame limit applies and no child list is retained.
func (HuaweiSDongleObservedQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             1,
		MaxObjects:           huaweiSDongleObservedMaxObjects,
		MaxBytes:             huaweiSDongleObservedMaxObjectSum,
		MaxChildren:          0,
		MaxResponseBytes:     huaweiClassicMaxPDUBytes,
	}
}

func (card HuaweiSDongleObservedQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate returns a test-ready result only for the exact observed tuple. A
// gateway read outside the tuple, a child unit other than the observed one, a
// model block or basic reply outside the tuple and an extended inventory that
// is not the one observed entry each produce an empty insufficient result.
// Register readability, a serial or ESN, the child's firmware text and the
// protocol words of the child are never predicates.
func (card HuaweiSDongleObservedQualificationCard) Evaluate(input HuaweiSDongleObservedQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	if !card.gatewayMatches(input.Gateway) {
		rejected.reason = HuaweiQualificationGatewayBehaviourMismatch
		return rejected
	}
	child := input.Child
	if child.UnitID != card.ChildUnit() {
		return rejected
	}
	model, residue, ok := parseHuaweiSDongleObservedModelBlock(child.ModelBlock)
	if !ok || model != card.ChildModel() {
		return rejected
	}
	if child.Basic == nil {
		return rejected
	}
	opaque, ok := card.corroborateChildBasic(*child.Basic)
	if !ok {
		rejected.reason = HuaweiQualificationBasicIdentityConflict
		return rejected
	}
	entryModel, anomalies, err := parseHuaweiSDongleObservedChildInventory(child.UnitID, child.Inventory)
	if err != nil {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	if entryModel != model {
		return rejected
	}
	return HuaweiQualificationResult{
		candidateClass:     card.CandidateClass(),
		selectedClass:      card.CandidateClass(),
		readiness:          HuaweiQualificationTestReady,
		reason:             HuaweiQualificationMatchedIdentityInventory,
		identityMatched:    true,
		missingEvidence:    card.MissingEvidence(),
		pageAnomalies:      anomalies,
		basicCorroborated:  true,
		basicOpaqueObjects: opaque,
		modelResidueBytes:  residue,
	}
}

// gatewayMatches compares the gateway-unit reads with the observed behavior.
// The sequence value is a counter and is compared only with itself; the
// capacity must cover the one child.
func (card HuaweiSDongleObservedQualificationCard) gatewayMatches(gateway HuaweiSDongleObservedGatewayReads) bool {
	if gateway.UnitID != card.UnitID() || gateway.Basic.Reply != nil || gateway.Basic.ExceptionCode != card.GatewayBasicException() {
		return false
	}
	if !slices.Equal(gateway.SearchStatus, []uint16{huaweiSDongleObservedSearchState}) ||
		!slices.Equal(gateway.Protocol, huaweiSDongleObservedProtocolWords) {
		return false
	}
	if len(gateway.SearchBefore) != 3 || !slices.Equal(gateway.SearchBefore, gateway.SearchAfter) ||
		gateway.SearchBefore[0] != huaweiSDongleObservedSearchType || gateway.SearchBefore[1] != huaweiSDongleObservedSearchState {
		return false
	}
	return len(gateway.Capacity) == 1 && gateway.Capacity[0] >= 1
}

// corroborateChildBasic requires vendor and product to be exactly the observed
// strings and a version object to be present, and counts the objects beyond the
// basic category, which are retained as opaque evidence and never interpreted.
func (card HuaweiSDongleObservedQualificationCard) corroborateChildBasic(reply HuaweiSDongleObservedBasicReply) (int, bool) {
	seen := make(map[uint8]struct{}, len(reply.Objects))
	identity := make(map[uint8]string, 3)
	opaque := 0
	for _, object := range reply.Objects {
		if _, repeated := seen[object.ObjectID]; repeated {
			return 0, false
		}
		seen[object.ObjectID] = struct{}{}
		if object.ObjectID > 0x02 {
			opaque++
			continue
		}
		if !isHuaweiSmartLoggerInventoryASCII(bytes.TrimRight(object.Value, "\x00")) {
			return 0, false
		}
		identity[object.ObjectID] = trimHuaweiTerminalPadding(string(object.Value))
	}
	if identity[0x00] != card.ChildVendor() || identity[0x01] != card.ChildFamily() || identity[0x02] == "" {
		return 0, false
	}
	return opaque, true
}

// parseHuaweiSDongleObservedModelBlock decodes the raw extent of the child
// model read. The text ends at the first NUL; it must be followed by printable
// ASCII residue and NUL padding only. The residue is retained by length and is
// never compared. A block without a terminating NUL may be truncated and is
// not matched.
func parseHuaweiSDongleObservedModelBlock(block []byte) (string, int, bool) {
	if len(block) != huaweiSDongleObservedModelExtent {
		return "", 0, false
	}
	end := bytes.IndexByte(block, 0)
	if end <= 0 || !isHuaweiSmartLoggerInventoryASCII(block[:end]) {
		return "", 0, false
	}
	rest := block[end+1:]
	residue := 0
	for residue < len(rest) && rest[residue] != 0 {
		if rest[residue] < 0x20 || rest[residue] > 0x7e {
			return "", 0, false
		}
		residue++
	}
	for _, padding := range rest[residue:] {
		if padding != 0 {
			return "", 0, false
		}
	}
	return trimHuaweiTerminalPadding(string(block[:end])), residue, true
}

// parseHuaweiSDongleObservedChildInventory validates the one observed page: the
// count object 1, then one entry whose key set is exactly 1 through 6 and whose
// unit attribute is the unit that was read. The declared-object field and a
// next object identifier set without a continuation are retained as anomalies
// and never compared. The entry model is returned for the agreement check.
func parseHuaweiSDongleObservedChildInventory(unit uint8, page HuaweiSDongleObservedChildInventoryPage) (string, HuaweiInventoryPageAnomalies, error) {
	invalid := func() (string, HuaweiInventoryPageAnomalies, error) {
		return "", HuaweiInventoryPageAnomalies{}, errHuaweiSDongleObservedInventoryInvalid
	}
	if page.ReadDeviceIDCode != huaweiSDongleObservedReadCode || page.Conformity != huaweiSDongleObservedConformity || page.More ||
		page.DeclaredObjects < 0 || page.DeclaredObjects > 255 || len(page.Objects) != huaweiSDongleObservedMaxObjects {
		return invalid()
	}
	count, entry := page.Objects[0], page.Objects[1]
	if count.ObjectID != huaweiSDongleObservedCountObject || len(count.Value) != 1 || count.Value[0] != 1 ||
		entry.ObjectID != huaweiSDongleObservedEntryObject {
		return invalid()
	}
	responseBytes := huaweiMEIResponseHeaderBytes + 2 + len(count.Value) + 2 + len(entry.Value)
	if responseBytes != page.ResponseBytes || responseBytes > huaweiClassicMaxPDUBytes {
		return invalid()
	}
	child, err := parseHuaweiObservedEntry(entry)
	if err != nil || child.address != strconv.Itoa(int(unit)) || !huaweiSDongleObservedEntryKeySet(entry.Value) {
		return invalid()
	}
	var anomalies HuaweiInventoryPageAnomalies
	if page.DeclaredObjects != len(page.Objects) {
		anomalies.DeclaredObjectFieldDiffers++
	}
	if page.NextObjectID != 0 {
		anomalies.NextObjectWithoutContinuation++
	}
	return child.model, anomalies, nil
}

// huaweiSDongleObservedEntryKeySet reports whether the entry holds exactly the
// observed keys. The entry has already passed the observed-dialect decoder, so
// keys are unique and well formed.
func huaweiSDongleObservedEntryKeySet(value []byte) bool {
	text := string(bytes.TrimRight(value, "\x00 "))
	attributes := strings.Split(text, ";")
	if len(attributes) != len(huaweiSDongleObservedEntryKeys) {
		return false
	}
	for _, attribute := range attributes {
		key, _, _ := strings.Cut(attribute, "=")
		if !slices.Contains(huaweiSDongleObservedEntryKeys, key) {
			return false
		}
	}
	return true
}
