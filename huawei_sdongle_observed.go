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

	huaweiSDongleObservedSearchState    = 0
	huaweiSDongleObservedModelExtent    = 30
	huaweiSDongleObservedSoftwareExtent = 30
	huaweiSDongleObservedCountObject    = huaweiSmartLoggerInventoryStartObject
	huaweiSDongleObservedConformity     = 0x03
	huaweiSDongleObservedReadCode       = 3

	// The resource policy of a separately qualified S-Dongle child unit: 121
	// objects (the count and up to 120 entries) and 32768 bytes. The observed
	// two objects, one entry and 87-byte response are lower bounds, never caps;
	// one response is bounded by the shared cap of the observed dialect.
	huaweiSDongleChildMaxObjects = 121
	huaweiSDongleChildMaxBytes   = 32768
)

var errHuaweiSDongleObservedInventoryInvalid = errors.New("invalid observed S-Dongle child inventory")

// huaweiSDongleObservedProtocolWords are the gateway protocol words as read.
// Their encoding is not established, so they are never decoded. They are
// compared in the floor form of the editions' compatibility note, word by
// word as read: the leading word, which stands where the main revision
// stands, is the same, and the second word, which stands where the release
// revision stands, is at or above the observed one.
var huaweiSDongleObservedProtocolWords = []uint16{50, 0}

// huaweiSDongleObservedEntryKeys are the keys of the observed child entry. They
// are a lower bound: each must be present once, and a further key is retained
// as opaque native evidence. Order is not validated.
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
	// Software is the raw extent of an FC03 offset 30050 quantity 15 read of
	// the gateway's own software text, or empty when it was not read. The
	// observation did not read it; a caller that has it supplies it, and the
	// catalogue decision then compares it with the S-Dongle floors.
	Software []byte
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
//     answers the search status read with 0, the protocol read with the leading
//     word 50 and a second word of 0 or more, the search sequence read with state
//     0 and an equal sequence before and after the child reads, and a capacity
//     that covers one child; the type word of the search sequence is recorded
//     and never compared, because the editions define it as the network
//     connection type, a configuration value;
//   - a software text at gateway unit 100, when one was read, is admitted by the
//     catalogue decision over the S-Dongle floors;
//   - child unit 1 answers the model read with the text SUN2000-8KTL-M1, a NUL,
//     opaque printable residue and NUL padding; answers basic identification with
//     vendor Huawei and product SUN2000; and answers the extended inventory with
//     one page whose count object equals its entries, each carrying keys 1
//     through 6 at least, and exactly one of which has the child unit as its
//     unit attribute and the model read as its model.
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

// Limits are the resource policy of a separately qualified S-Dongle child
// unit within the one page the card reads; no child list is retained. The
// observed entry count, key set and response length are lower bounds.
func (HuaweiSDongleObservedQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             1,
		MaxObjects:           huaweiSDongleChildMaxObjects,
		MaxBytes:             huaweiSDongleChildMaxBytes,
		MaxChildren:          0,
		MaxResponseBytes:     huaweiSmartLoggerObservedMaxResponseBytes,
	}
}

func (card HuaweiSDongleObservedQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// Evaluate returns a test-ready result for the observed tuple. A gateway read
// outside the tuple, a gateway software text that the catalogue decision
// refuses, a child unit other than the observed one, a model block or basic
// reply outside the tuple and an extended inventory without the child's entry
// each produce an empty insufficient result. Register readability, a serial or
// ESN, the type word, the child's firmware text and the protocol words of the
// child are never predicates.
func (card HuaweiSDongleObservedQualificationCard) Evaluate(input HuaweiSDongleObservedQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	typeWord, ok := card.gatewayMatches(input.Gateway)
	if !ok {
		rejected.reason = HuaweiQualificationGatewayBehaviourMismatch
		return rejected
	}
	software := ""
	if len(input.Gateway.Software) > 0 {
		text, ok := parseHuaweiEMMAObservedGatewayBlock(input.Gateway.Software, huaweiSDongleObservedSoftwareExtent)
		if !ok {
			rejected.reason = HuaweiQualificationGatewayBehaviourMismatch
			return rejected
		}
		software = text
	}
	selection := DecideHuaweiGatewaySelection(HuaweiGatewaySelectionInput{
		Family:   card.CandidateClass(),
		Scope:    HuaweiGatewayScope{Endpoint: HuaweiGatewayEndpointSelf, Unit: input.Gateway.UnitID},
		Software: software,
	})
	rejected.selection = &selection
	if !selection.Admitted() {
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
	entry, anomalies, err := parseHuaweiSDongleObservedChildInventory(child.UnitID, child.Inventory)
	if err != nil {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	if entry.model != model {
		return rejected
	}
	childOpaque := entry.OpaqueAttributes()
	if productType := entry.Attribute("product_type"); productType != "" {
		// Key 8 is held as the product type by the shared entry decoder; the
		// S-Dongle card does not compare it, so it is retained as native evidence.
		childOpaque = append(childOpaque, HuaweiGatewayOpaqueAttribute{Key: "8", Value: productType})
		orderHuaweiSDongleOpaqueByEntry(childOpaque, input.Child.Inventory.Objects, entry.ObjectID())
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
		selection:          &selection,
		protocolWords:      slices.Clone(input.Gateway.Protocol),
		typeWord:           &typeWord,
		childOpaque:        childOpaque,
	}
}

// gatewayMatches compares the gateway-unit reads with the observed behavior and
// returns the type word, which it records and never compares. The protocol
// words take the floor form; the state word must be the completed state before
// and after; the sequence value is a counter and is compared only with itself;
// the capacity must cover the one child.
func (card HuaweiSDongleObservedQualificationCard) gatewayMatches(gateway HuaweiSDongleObservedGatewayReads) (uint16, bool) {
	if gateway.UnitID != card.UnitID() || gateway.Basic.Reply != nil || gateway.Basic.ExceptionCode != card.GatewayBasicException() {
		return 0, false
	}
	if !slices.Equal(gateway.SearchStatus, []uint16{huaweiSDongleObservedSearchState}) || !huaweiSDongleProtocolWordsAtOrAbove(gateway.Protocol) {
		return 0, false
	}
	if len(gateway.SearchBefore) != 3 || len(gateway.SearchAfter) != 3 ||
		gateway.SearchBefore[1] != huaweiSDongleObservedSearchState || gateway.SearchAfter[1] != huaweiSDongleObservedSearchState ||
		gateway.SearchBefore[2] != gateway.SearchAfter[2] {
		return 0, false
	}
	if len(gateway.Capacity) != 1 || gateway.Capacity[0] < 1 {
		return 0, false
	}
	return gateway.SearchBefore[0], true
}

// huaweiSDongleProtocolWordsAtOrAbove applies the floor form to the protocol
// words as read: two words, the leading one equal to the observed leading word
// and the second at or above the observed second word.
func huaweiSDongleProtocolWordsAtOrAbove(words []uint16) bool {
	observed := huaweiSDongleObservedProtocolWords
	return len(words) == len(observed) && words[0] == observed[0] && words[1] >= observed[1]
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

// parseHuaweiSDongleObservedChildInventory validates the one page: the count
// object, then as many entries as it counts in object order, each carrying at
// least keys 1 through 6 and a distinct unit attribute, and exactly one of them
// naming the unit that was read. The observed count of 1 is a lower bound, as
// are the observed key set and response length; the policy bounds the objects,
// one entry, the response and the aggregate bytes. The declared-object field
// and a next object identifier set without a continuation are retained as
// anomalies and never compared. The entry of the unit read is returned for the
// agreement check and for its retained opaque attributes.
func parseHuaweiSDongleObservedChildInventory(unit uint8, page HuaweiSDongleObservedChildInventoryPage) (HuaweiSmartLoggerInventoryChild, HuaweiInventoryPageAnomalies, error) {
	invalid := func() (HuaweiSmartLoggerInventoryChild, HuaweiInventoryPageAnomalies, error) {
		return HuaweiSmartLoggerInventoryChild{}, HuaweiInventoryPageAnomalies{}, errHuaweiSDongleObservedInventoryInvalid
	}
	if page.ReadDeviceIDCode != huaweiSDongleObservedReadCode || page.Conformity != huaweiSDongleObservedConformity || page.More ||
		page.DeclaredObjects < 0 || page.DeclaredObjects > 255 || len(page.Objects) < 2 || len(page.Objects) > huaweiSDongleChildMaxObjects {
		return invalid()
	}
	count := page.Objects[0]
	if count.ObjectID != huaweiSDongleObservedCountObject || len(count.Value) != 1 || int(count.Value[0]) != len(page.Objects)-1 {
		return invalid()
	}
	responseBytes := huaweiMEIResponseHeaderBytes + 2 + len(count.Value)
	var read HuaweiSmartLoggerInventoryChild
	matched := 0
	seen := make(map[string]struct{}, len(page.Objects)-1)
	for index, entry := range page.Objects[1:] {
		if entry.ObjectID != uint8(huaweiSDongleObservedCountObject+1+index) {
			return invalid()
		}
		responseBytes += 2 + len(entry.Value)
		child, err := parseHuaweiObservedEntry(entry)
		if err != nil || !huaweiSDongleObservedEntryKeySet(entry.Value) {
			return invalid()
		}
		if _, repeated := seen[child.address]; repeated {
			return invalid()
		}
		seen[child.address] = struct{}{}
		if child.address == strconv.Itoa(int(unit)) {
			read, matched = child, matched+1
		}
	}
	if responseBytes != page.ResponseBytes || responseBytes > huaweiSmartLoggerObservedMaxResponseBytes ||
		responseBytes-huaweiMEIResponseHeaderBytes > huaweiSDongleChildMaxBytes || matched != 1 {
		return invalid()
	}
	var anomalies HuaweiInventoryPageAnomalies
	if responseBytes > huaweiClassicMaxPDUBytes {
		anomalies.OversizeResponses++
	}
	if page.DeclaredObjects != len(page.Objects) {
		anomalies.DeclaredObjectFieldDiffers++
	}
	if page.NextObjectID != 0 {
		anomalies.NextObjectWithoutContinuation++
	}
	return read, anomalies, nil
}

// huaweiSDongleObservedEntryKeySet reports whether the entry holds at least the
// observed keys. The entry has already passed the observed-dialect decoder, so
// keys are unique and well formed, and a further key is retained as opaque.
func huaweiSDongleObservedEntryKeySet(value []byte) bool {
	text := string(bytes.TrimRight(value, "\x00 "))
	present := map[string]struct{}{}
	for _, attribute := range strings.Split(text, ";") {
		key, _, _ := strings.Cut(attribute, "=")
		present[key] = struct{}{}
	}
	for _, key := range huaweiSDongleObservedEntryKeys {
		if _, ok := present[key]; !ok {
			return false
		}
	}
	return true
}

// orderHuaweiSDongleOpaqueByEntry restores entry order: the attributes are
// sorted by the position of their key in the text of the entry object.
func orderHuaweiSDongleOpaqueByEntry(attributes []HuaweiGatewayOpaqueAttribute, objects []HuaweiGatewayInventoryObject, objectID uint8) {
	var text string
	for _, object := range objects {
		if object.ObjectID == objectID {
			text = string(object.Value)
		}
	}
	position := make(map[string]int)
	for index, pair := range strings.Split(text, ";") {
		key, _, _ := strings.Cut(pair, "=")
		if _, seen := position[key]; !seen {
			position[key] = index
		}
	}
	slices.SortStableFunc(attributes, func(a, b HuaweiGatewayOpaqueAttribute) int {
		return position[a.Key] - position[b.Key]
	})
}
