package modbusreg

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"strings"
)

const (
	// HuaweiEMMAObservedQualificationCardID identifies the card for the one EMMA
	// path that a read-only observation established: a gateway unit that names
	// its family and model, answers basic and extended device identification,
	// and fronts two SUN2000-family inverter units. It is a separate card from
	// the documentary EMMA card, which keeps its identity and behavior.
	HuaweiEMMAObservedQualificationCardID = "huawei.qualification.emma.observed-readonly@1.0.0"

	huaweiEMMAObservedGatewayUnit = 0
	huaweiEMMAObservedLabel       = "SmartHEMS"
	huaweiEMMAObservedModel       = "EMMA-A02"
	huaweiEMMAObservedVendor      = "Huawei"
	huaweiEMMAObservedProduct     = "HEMS"
	huaweiEMMAObservedChildFamily = "SUN2000"

	huaweiEMMAObservedLabelExtent    = 30
	huaweiEMMAObservedModelExtent    = 40
	huaweiEMMAObservedSoftwareExtent = 30
	huaweiEMMAObservedChildExtent    = 30

	huaweiEMMAObservedReadCode   = 3
	huaweiEMMAObservedConformity = 0x03
	huaweiEMMAObservedMaxPages   = 5
	huaweiEMMAObservedMaxObject  = 5
	huaweiEMMAObservedMaxBytes   = huaweiEMMAObservedMaxPages * (huaweiClassicMaxPDUBytes - huaweiMEIResponseHeaderBytes)
)

var errHuaweiEMMAObservedInventoryInvalid = errors.New("invalid observed EMMA inventory")

// huaweiEMMAObservedMember is one child entry of the observed inventory: the
// unit it names, the product type (key 8) and the family token that its model
// string starts with. An empty family token means that the model is recorded
// and not compared.
type huaweiEMMAObservedMember struct {
	unit        string
	productType string
	modelFamily string
}

// huaweiEMMAObservedMembers are the child entries of the observed inventory in
// object order. Units 3 and 4 are the inverter units that the card reads. The
// entry of unit 2 is recorded by its product type; its model is withheld and
// the unit is never read.
var huaweiEMMAObservedMembers = []huaweiEMMAObservedMember{
	{unit: "2", productType: "BackupBox"},
	{unit: "3", productType: huaweiEMMAObservedChildFamily, modelFamily: huaweiEMMAObservedChildFamily},
	{unit: "4", productType: huaweiEMMAObservedChildFamily, modelFamily: huaweiEMMAObservedChildFamily},
}

var huaweiEMMAObservedChildUnits = []uint8{3, 4}

// HuaweiEMMAObservedBasicReply is a decoded basic identification reply.
type HuaweiEMMAObservedBasicReply struct {
	Objects []HuaweiGatewayInventoryObject
}

// HuaweiEMMAObservedInventoryPage is one already-decoded extended identification
// response together with the wire facts the dialect tolerates.
type HuaweiEMMAObservedInventoryPage struct {
	ReadDeviceIDCode uint8
	Conformity       uint8
	// ResponseBytes is the length of the response PDU. It must equal the length
	// implied by the objects.
	ResponseBytes int
	// DeclaredObjects is the declared-object field, retained and never compared.
	DeclaredObjects int
	More            bool
	NextObjectID    uint8
	Objects         []HuaweiGatewayInventoryObject
}

// HuaweiEMMAObservedGatewayReads are the already-read gateway-unit replies.
// UnitID is the unit every one of them was addressed to. Each block is the raw
// register extent of one FC03 read: Offering offset 30000 quantity 15, Model
// offset 30222 quantity 20 and Software offset 30035 quantity 15.
type HuaweiEMMAObservedGatewayReads struct {
	UnitID    uint8
	Offering  []byte
	Model     []byte
	Software  []byte
	Basic     *HuaweiEMMAObservedBasicReply
	Inventory []HuaweiEMMAObservedInventoryPage
}

// HuaweiEMMAObservedChildReads are the already-read replies of one inverter
// unit. ModelBlock is the raw extent of the FC03 offset 30000 read.
type HuaweiEMMAObservedChildReads struct {
	UnitID     uint8
	ModelBlock []byte
}

// HuaweiEMMAObservedQualificationInput is the evidence that the card evaluates.
// It carries answered replies only; the serial-bearing blocks, the firmware
// blocks, the protocol words and every timing are not inputs.
type HuaweiEMMAObservedQualificationInput struct {
	Gateway  HuaweiEMMAObservedGatewayReads
	Children []HuaweiEMMAObservedChildReads
}

// HuaweiEMMAObservedQualificationCard is an inert, offline card for the observed
// EMMA tuple:
//
//   - gateway unit 0 answers the family label SmartHEMS, the model EMMA-A02 and a
//     software label that is that family label, a space and a version-style
//     token, each as text followed by NUL padding only, and the three satisfy the
//     documentary EMMA identity gate, whose branch rule and floors apply to the
//     token;
//   - it answers basic identification with vendor Huawei, product HEMS and a
//     version object that equals the token of the software label;
//   - it answers extended identification with a count object of 4 and four
//     entries: its own entry at unit 0 (the model, the product type HEMS and the
//     version token), an entry with product type BackupBox at unit 2 whose
//     model is recorded and never compared, and two entries of the SUN2000
//     family at units 3 and 4;
//   - units 3 and 4 answer the model read with the model string of their own
//     entry, a SUN2000-family string.
//
// Every other tuple is insufficient evidence. The card sends nothing and grants
// no authority.
type HuaweiEMMAObservedQualificationCard struct{}

func NewHuaweiEMMAObservedQualificationCard() HuaweiEMMAObservedQualificationCard {
	return HuaweiEMMAObservedQualificationCard{}
}

func (HuaweiEMMAObservedQualificationCard) ID() string {
	return HuaweiEMMAObservedQualificationCardID
}
func (HuaweiEMMAObservedQualificationCard) CandidateClass() string { return HuaweiEMMACanonicalClass }
func (HuaweiEMMAObservedQualificationCard) UnitID() uint8          { return huaweiEMMAObservedGatewayUnit }
func (HuaweiEMMAObservedQualificationCard) FamilyLabel() string    { return huaweiEMMAObservedLabel }
func (HuaweiEMMAObservedQualificationCard) Model() string          { return huaweiEMMAObservedModel }
func (HuaweiEMMAObservedQualificationCard) Vendor() string         { return huaweiEMMAObservedVendor }
func (HuaweiEMMAObservedQualificationCard) Product() string        { return huaweiEMMAObservedProduct }
func (HuaweiEMMAObservedQualificationCard) ChildFamily() string {
	return huaweiEMMAObservedChildFamily
}
func (HuaweiEMMAObservedQualificationCard) ChildUnits() []uint8 {
	return slices.Clone(huaweiEMMAObservedChildUnits)
}

// MissingEvidence lists what remains open for this tuple: the capability
// fixtures of the monitoring profile, which this identity slice does not
// provide, and the addressing of the battery group, which the gateway fronts but
// does not list or answer as a unit.
func (HuaweiEMMAObservedQualificationCard) MissingEvidence() []string {
	return []string{
		"sanitized_readonly_capability_fixture",
		"model_specific_capability_fixture",
		"battery_addressing_behind_gateway_fixture",
	}
}

// Steps is the replay procedure. Every step is an FC03 or an FC2B / MEI 0x0e
// read. The inverter units are the ones that the inventory entries name.
func (HuaweiEMMAObservedQualificationCard) Steps() []HuaweiQualificationStep {
	return []HuaweiQualificationStep{
		{operation: "FC03_GATEWAY_FAMILY_LABEL", unitID: huaweiEMMAObservedGatewayUnit, offset: 30000, quantity: 15},
		{operation: "FC03_GATEWAY_MODEL", unitID: huaweiEMMAObservedGatewayUnit, offset: 30222, quantity: 20},
		{operation: "FC03_GATEWAY_SOFTWARE_LABEL", unitID: huaweiEMMAObservedGatewayUnit, offset: 30035, quantity: 15},
		{operation: "FC2B_MEI_0E_READDEV01_BASIC_GATEWAY", unitID: huaweiEMMAObservedGatewayUnit, objectID: 0x00},
		{operation: "FC2B_MEI_0E_READDEV03_INVENTORY_GATEWAY", unitID: huaweiEMMAObservedGatewayUnit, objectID: huaweiSmartLoggerInventoryStartObject},
		{operation: "FC03_CHILD_MODEL", unitID: 3, offset: 30000, quantity: 15},
		{operation: "FC03_CHILD_MODEL", unitID: 4, offset: 30000, quantity: 15},
	}
}

// Limits narrows the shared inventory ceilings to the observed inventory: at
// most five responses of at most the classic frame size, five objects (the
// count and four entries) and three children.
func (HuaweiEMMAObservedQualificationCard) Limits() HuaweiQualificationInventoryLimits {
	return HuaweiQualificationInventoryLimits{
		DeadlineMilliseconds: 15000,
		MaxPages:             huaweiEMMAObservedMaxPages,
		MaxObjects:           huaweiEMMAObservedMaxObject,
		MaxBytes:             huaweiEMMAObservedMaxBytes,
		MaxChildren:          uint32(len(huaweiEMMAObservedMembers)),
		MaxResponseBytes:     huaweiClassicMaxPDUBytes,
	}
}

func (card HuaweiEMMAObservedQualificationCard) EmptyResult() HuaweiQualificationResult {
	return HuaweiQualificationResult{candidateClass: card.CandidateClass(), readiness: HuaweiQualificationInsufficientEvidence, reason: HuaweiQualificationIdentityInvalid}
}

// huaweiEMMAObservedBranchPrefix is the software branch of the observed tuple;
// the floor of that branch is applied by the documentary gate.
const huaweiEMMAObservedBranchPrefix = "V100R025C00SPC"

// Evaluate returns a test-ready result only for the exact observed tuple. A
// gateway block outside the tuple, a basic reply that disagrees with the
// gateway, an inventory outside the observed shape and a child read that is not
// the SUN2000-family string of its own entry each produce an empty insufficient
// result. Register readability, a serial or ESN, the firmware blocks and the
// protocol words are never predicates.
func (card HuaweiEMMAObservedQualificationCard) Evaluate(input HuaweiEMMAObservedQualificationInput) HuaweiQualificationResult {
	rejected := card.EmptyResult()
	gateway := input.Gateway
	if gateway.UnitID != card.UnitID() {
		return rejected
	}
	label, ok := parseHuaweiEMMAObservedGatewayBlock(gateway.Offering, huaweiEMMAObservedLabelExtent)
	if !ok || label != card.FamilyLabel() {
		return rejected
	}
	model, ok := parseHuaweiEMMAObservedGatewayBlock(gateway.Model, huaweiEMMAObservedModelExtent)
	if !ok {
		return rejected
	}
	software, ok := parseHuaweiEMMAObservedGatewayBlock(gateway.Software, huaweiEMMAObservedSoftwareExtent)
	if !ok {
		return rejected
	}
	// The documentary gate reads the three blocks: it admits two model aliases
	// and the software label of two branches above their floors, and this card
	// pins the one alias that was observed. A software label that passes the gate
	// starts with the family label and a space, so the token is what follows.
	if decision := EvaluateHuaweiEMMAOfflineIdentity(label, model, software); !decision.Matched() || decision.ModelVariant() != card.Model() {
		return rejected
	}
	token := strings.TrimPrefix(software, card.FamilyLabel()+" ")
	// The gate admits two branches; only the observed one is in the tuple.
	if !strings.HasPrefix(token, huaweiEMMAObservedBranchPrefix) {
		return rejected
	}
	if gateway.Basic == nil {
		return rejected
	}
	opaque, version, ok := card.corroborateBasic(*gateway.Basic)
	if !ok || version != token {
		rejected.reason = HuaweiQualificationBasicIdentityConflict
		return rejected
	}
	parsed, err := parseHuaweiEMMAObservedInventory(gateway.UnitID, gateway.Inventory)
	if err != nil {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	if parsed.self.Attribute("model") != model || parsed.self.Attribute("product_type") != card.Product() ||
		parsed.self.Attribute("software_version") != token {
		return rejected
	}
	children := parsed.inventory.Children()
	if len(children) != len(huaweiEMMAObservedMembers) {
		rejected.reason = HuaweiQualificationInventoryInvalid
		return rejected
	}
	for index, member := range huaweiEMMAObservedMembers {
		family, _ := huaweiEMMAObservedModelFamily(children[index].Attribute("model"))
		if children[index].Address() != member.unit || children[index].Attribute("product_type") != member.productType ||
			(member.modelFamily != "" && family != member.modelFamily) {
			rejected.reason = HuaweiQualificationInventoryInvalid
			return rejected
		}
	}
	if !card.childReadsMatch(input.Children, children) {
		return rejected
	}
	retained := parsed.inventory
	return HuaweiQualificationResult{
		candidateClass:     card.CandidateClass(),
		selectedClass:      card.CandidateClass(),
		modelVariant:       model,
		readiness:          HuaweiQualificationTestReady,
		reason:             HuaweiQualificationMatchedIdentityInventory,
		identityMatched:    true,
		inventory:          &retained,
		missingEvidence:    card.MissingEvidence(),
		pageAnomalies:      parsed.anomalies,
		basicCorroborated:  true,
		basicOpaqueObjects: opaque,
	}
}

// corroborateBasic requires vendor and product to be exactly the observed
// strings and a version object to be present, returns that version for the
// agreement with the software label, and counts the objects beyond the basic
// category, which are retained as opaque evidence and never interpreted.
func (card HuaweiEMMAObservedQualificationCard) corroborateBasic(reply HuaweiEMMAObservedBasicReply) (int, string, bool) {
	seen := make(map[uint8]struct{}, len(reply.Objects))
	identity := make(map[uint8]string, 3)
	opaque := 0
	for _, object := range reply.Objects {
		if _, repeated := seen[object.ObjectID]; repeated {
			return 0, "", false
		}
		seen[object.ObjectID] = struct{}{}
		if object.ObjectID > 0x02 {
			opaque++
			continue
		}
		if !isHuaweiSmartLoggerInventoryASCII(bytes.TrimRight(object.Value, "\x00")) {
			return 0, "", false
		}
		identity[object.ObjectID] = trimHuaweiTerminalPadding(string(object.Value))
	}
	if identity[0x00] != card.Vendor() || identity[0x01] != card.Product() {
		return 0, "", false
	}
	return opaque, identity[0x02], true
}

// childReadsMatch requires exactly one read for each inverter unit, and for
// each the model string of the inventory entry of that unit, of the SUN2000
// family. The residue after the model text is retained by the block parser and
// never compared.
func (card HuaweiEMMAObservedQualificationCard) childReadsMatch(reads []HuaweiEMMAObservedChildReads, entries []HuaweiGatewayInventoryChild) bool {
	if len(reads) != len(huaweiEMMAObservedChildUnits) {
		return false
	}
	byUnit := make(map[uint8][]byte, len(reads))
	for _, read := range reads {
		byUnit[read.UnitID] = read.ModelBlock
	}
	for _, unit := range huaweiEMMAObservedChildUnits {
		block, answered := byUnit[unit]
		if !answered {
			return false
		}
		text, _, ok := parseHuaweiEMMAObservedBlock(block, huaweiEMMAObservedChildExtent)
		if !ok {
			return false
		}
		family, ok := huaweiEMMAObservedModelFamily(text)
		if !ok || family != card.ChildFamily() || !isHuaweiEMMAObservedToken(text) {
			return false
		}
		matched := false
		for _, entry := range entries {
			if entry.Address() == strconv.Itoa(int(unit)) && entry.Attribute("model") == text {
				matched = true
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// parseHuaweiEMMAObservedGatewayBlock decodes the raw extent of a gateway read:
// printable text that ends at the first NUL and is followed by NUL padding only.
func parseHuaweiEMMAObservedGatewayBlock(block []byte, extent int) (string, bool) {
	text, residue, ok := parseHuaweiEMMAObservedBlock(block, extent)
	if !ok || residue != 0 {
		return "", false
	}
	return text, true
}

// parseHuaweiEMMAObservedBlock decodes a raw register extent: printable text
// that ends at the first NUL, followed to the end of the extent by NUL bytes and
// printable ASCII only. The number of printable bytes after the first NUL is the
// residue and is returned; its content is never compared. A block without a
// terminating NUL may be truncated and is not matched.
func parseHuaweiEMMAObservedBlock(block []byte, extent int) (string, int, bool) {
	if len(block) != extent {
		return "", 0, false
	}
	end := bytes.IndexByte(block, 0)
	if end <= 0 || !isHuaweiSmartLoggerInventoryASCII(block[:end]) {
		return "", 0, false
	}
	residue := 0
	for _, character := range block[end+1:] {
		switch {
		case character == 0:
		case character >= 0x20 && character <= 0x7e:
			residue++
		default:
			return "", 0, false
		}
	}
	text := trimHuaweiTerminalPadding(string(block[:end]))
	if text == "" {
		return "", 0, false
	}
	return text, residue, true
}

// isHuaweiEMMAObservedToken accepts a non-empty token of letters, digits, dots,
// hyphens and underscores; the extent of the block bounds its length. It is used
// for the model string of an inverter, whose family token is compared and whose
// remainder must not be a wildcard or free text.
func isHuaweiEMMAObservedToken(token string) bool {
	if token == "" {
		return false
	}
	for _, character := range []byte(token) {
		switch {
		case character >= '0' && character <= '9', character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z',
			character == '.', character == '-', character == '_':
		default:
			return false
		}
	}
	return true
}

// huaweiEMMAObservedModelFamily returns the family token of a model string: the
// text before the first hyphen, which must be followed by a non-empty remainder.
func huaweiEMMAObservedModelFamily(model string) (string, bool) {
	family, rest, found := strings.Cut(model, "-")
	if !found || family == "" || rest == "" {
		return "", false
	}
	return family, true
}

// parseHuaweiEMMAObservedInventory validates the pages: read code 3, conformity
// 0x03, at most five responses of at most the classic frame size, and then the
// observed dialect without a change counter, because the gateway answers the
// counter register with an exception.
func parseHuaweiEMMAObservedInventory(gatewayUnit uint8, pages []HuaweiEMMAObservedInventoryPage) (huaweiObservedInventory, error) {
	if len(pages) == 0 || len(pages) > huaweiEMMAObservedMaxPages {
		return huaweiObservedInventory{}, errHuaweiEMMAObservedInventoryInvalid
	}
	shared := make([]HuaweiSmartLoggerObservedInventoryPage, 0, len(pages))
	for _, page := range pages {
		if page.ReadDeviceIDCode != huaweiEMMAObservedReadCode || page.Conformity != huaweiEMMAObservedConformity ||
			page.ResponseBytes > huaweiClassicMaxPDUBytes {
			return huaweiObservedInventory{}, errHuaweiEMMAObservedInventoryInvalid
		}
		shared = append(shared, HuaweiSmartLoggerObservedInventoryPage{
			ResponseBytes:   page.ResponseBytes,
			DeclaredObjects: page.DeclaredObjects,
			More:            page.More,
			NextObjectID:    page.NextObjectID,
			Objects:         page.Objects,
		})
	}
	return parseHuaweiObservedInventoryPages(gatewayUnit, shared)
}
