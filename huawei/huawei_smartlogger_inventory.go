package huawei

import (
	"errors"
	"strings"
)

const (
	huaweiSmartLoggerInventoryStartObject = 0x87
	huaweiSmartLoggerInventoryMaxPages    = 248
	huaweiSmartLoggerInventoryMaxObjects  = 248
	huaweiSmartLoggerInventoryMaxBytes    = 65536
	huaweiSmartLoggerInventoryMaxChildren = 247
)

var errHuaweiSmartLoggerInventoryInvalid = errors.New("invalid SmartLogger offline inventory")

// SmartLoggerOfflineInventoryInput is a detached, already-read extended
// MEI inventory. It deliberately contains no endpoint, unit, or transport
// state and cannot enable a profile or operation.
type SmartLoggerOfflineInventoryInput struct {
	ChangeCounterBefore uint16
	ChangeCounterAfter  uint16
	Pages               []SmartLoggerInventoryPage
}

// SmartLoggerInventoryPage is one already-decoded Read Device ID page.
type SmartLoggerInventoryPage struct {
	More         bool
	NextObjectID uint8
	Objects      []SmartLoggerInventoryObject
}

// GatewayInventoryPage is the shared offline extended-MEI page shape for
// Huawei gateway families. The SmartLogger name remains as a compatibility
// alias for the first caller.
type GatewayInventoryPage = SmartLoggerInventoryPage

// SmartLoggerInventoryObject retains a single opaque inventory object
// until its bounded attribute form has been validated.
type SmartLoggerInventoryObject struct {
	ObjectID uint8
	Value    []byte
}

// GatewayInventoryObject is one shared offline extended-MEI object.
type GatewayInventoryObject = SmartLoggerInventoryObject

// SmartLoggerInventory is a validated, default-denied child snapshot.
type SmartLoggerInventory struct {
	declaredChildren int
	children         []SmartLoggerInventoryChild
}

// GatewayInventory is a validated shared offline inventory view.
type GatewayInventory = SmartLoggerInventory

// DeclaredChildren returns the child count that the count object declared. A
// parsed inventory always has it equal to ChildCount.
func (i SmartLoggerInventory) DeclaredChildren() int { return i.declaredChildren }

// ChildCount returns the number of child entries; the gateway's own entry is
// not counted.
func (i SmartLoggerInventory) ChildCount() int { return len(i.children) }

// DefaultDenied is always true: a listed child is never admitted by listing.
func (i SmartLoggerInventory) DefaultDenied() bool { return true }

// Children returns a fresh slice of the child entries in object order, without
// the gateway's own entry.
func (i SmartLoggerInventory) Children() []SmartLoggerInventoryChild {
	return append([]SmartLoggerInventoryChild(nil), i.children...)
}

// SmartLoggerInventoryChild contains only non-sensitive child fields.
// Attribute values are accessed by their canonical names; the sensitive input
// attribute is validated but never retained.
type SmartLoggerInventoryChild struct {
	objectID        uint8
	model           string
	softwareVersion string
	protocolVersion string
	address         string
	featureVersion  string
	productType     string
	// opaque holds the retained opaque attributes in entry order, in the
	// key=value;key=value form of the entry, so the child stays comparable.
	opaque string
}

// GatewayOpaqueAttribute is a device-list attribute that a reader
// retains as native evidence and never decodes or compares: a key that no
// canonical name covers, such as the parent and registration keys or a key
// outside the observed set.
type GatewayOpaqueAttribute struct {
	Key   string
	Value string
}

// GatewayInventoryChild is a non-sensitive shared child view.
type GatewayInventoryChild = SmartLoggerInventoryChild

// ObjectID returns the extended identification object that carried the entry.
func (c SmartLoggerInventoryChild) ObjectID() uint8 { return c.objectID }

// Address returns the raw text of attribute key 5, the unit address the entry
// lists; it is not parsed into a unit identifier.
func (c SmartLoggerInventoryChild) Address() string { return c.address }

// OpaqueAttributes lists the retained opaque attributes in entry order.
func (c SmartLoggerInventoryChild) OpaqueAttributes() []GatewayOpaqueAttribute {
	if c.opaque == "" {
		return nil
	}
	var attributes []GatewayOpaqueAttribute
	for _, pair := range strings.Split(c.opaque, ";") {
		key, value, _ := strings.Cut(pair, "=")
		attributes = append(attributes, GatewayOpaqueAttribute{Key: key, Value: value})
	}
	return attributes
}

// OpaqueAttribute returns the retained value of one opaque key.
func (c SmartLoggerInventoryChild) OpaqueAttribute(key string) (string, bool) {
	for _, attribute := range c.OpaqueAttributes() {
		if attribute.Key == key {
			return attribute.Value, true
		}
	}
	return "", false
}

// retainOpaque appends one opaque attribute. The entry grammar already
// excludes the separators from keys and values.
func (c *SmartLoggerInventoryChild) retainOpaque(key, value string) {
	if c.opaque != "" {
		c.opaque += ";"
	}
	c.opaque += key + "=" + value
}

// Attribute returns a decoded attribute by canonical name: model (key 1),
// software_version (key 2), protocol_version (key 3), feature_version (key 6;
// empty where the reader keeps key 6 opaque) or product_type (key 8). An
// unknown name or an absent attribute returns the empty string; the unit
// address is read with Address.
func (c SmartLoggerInventoryChild) Attribute(name string) string {
	switch name {
	case "model":
		return c.model
	case "software_version":
		return c.softwareVersion
	case "protocol_version":
		return c.protocolVersion
	case "feature_version":
		return c.featureVersion
	case "product_type":
		return c.productType
	default:
		return ""
	}
}

// ParseHuaweiSmartLoggerOfflineInventory validates documented pagination and
// child encoding from supplied pages. Any malformed or incomplete input is
// rejected without returning a partial inventory.
func ParseHuaweiSmartLoggerOfflineInventory(input SmartLoggerOfflineInventoryInput) (SmartLoggerInventory, error) {
	return parseHuaweiExtendedMEIOfflineInventory(input, func(model string) bool {
		return model == "SmartLogger"
	}, parseHuaweiSmartLoggerInventoryChild)
}

// parseHuaweiExtendedMEIOfflineInventory validates the documented pagination
// and decodes each child object with parseChild.
func parseHuaweiExtendedMEIOfflineInventory(input SmartLoggerOfflineInventoryInput, isSelfEntry func(string) bool,
	parseChild func(SmartLoggerInventoryObject) (SmartLoggerInventoryChild, error)) (SmartLoggerInventory, error) {
	if input.ChangeCounterBefore != input.ChangeCounterAfter || len(input.Pages) == 0 || len(input.Pages) > huaweiSmartLoggerInventoryMaxPages {
		return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
	}

	expectedObjectID := uint8(huaweiSmartLoggerInventoryStartObject)
	seenObjects := make(map[uint8]struct{})
	seenAddresses := make(map[string]struct{})
	var (
		declaredChildren = -1
		children         []SmartLoggerInventoryChild
		selfSeen         bool
		wrapped          bool
		recordCount      int
		byteCount        int
	)

	for pageIndex, page := range input.Pages {
		if len(page.Objects) == 0 {
			return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
		}
		for _, object := range page.Objects {
			if object.ObjectID != expectedObjectID {
				return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			if _, alreadySeen := seenObjects[object.ObjectID]; alreadySeen {
				return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			seenObjects[object.ObjectID] = struct{}{}
			if len(object.Value) > huaweiSmartLoggerInventoryMaxBytes-byteCount-2 {
				return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			byteCount += len(object.Value) + 2

			if object.ObjectID == huaweiSmartLoggerInventoryStartObject {
				if pageIndex != 0 || len(seenObjects) != 1 || len(object.Value) != 1 || object.Value[0] > huaweiSmartLoggerInventoryMaxChildren {
					return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				declaredChildren = int(object.Value[0])
			} else {
				if recordCount == huaweiSmartLoggerInventoryMaxObjects {
					return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				recordCount++
				child, err := parseChild(object)
				if err != nil {
					return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				if isSelfEntry(child.model) {
					if selfSeen || len(children) != 0 {
						return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					selfSeen = true
				} else {
					if !selfSeen || child.address == "" {
						return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					if _, duplicateAddress := seenAddresses[child.address]; duplicateAddress {
						return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					seenAddresses[child.address] = struct{}{}
					children = append(children, child)
				}
			}

			nextObjectID := object.ObjectID + 1
			if nextObjectID == 0 && object.ObjectID == 0xff {
				if wrapped {
					return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				wrapped = true
			}
			expectedObjectID = nextObjectID
		}
		if page.More {
			if page.NextObjectID != expectedObjectID || pageIndex == len(input.Pages)-1 {
				return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			continue
		}
		if page.NextObjectID != 0 || pageIndex != len(input.Pages)-1 {
			return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
		}
	}

	if declaredChildren < 0 || !selfSeen || declaredChildren != len(children) {
		return SmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
	}
	return SmartLoggerInventory{declaredChildren: declaredChildren, children: append([]SmartLoggerInventoryChild(nil), children...)}, nil
}

func parseHuaweiSmartLoggerInventoryChild(object SmartLoggerInventoryObject) (SmartLoggerInventoryChild, error) {
	if len(object.Value) == 0 || !isHuaweiSmartLoggerInventoryASCII(object.Value) {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
	}
	child := SmartLoggerInventoryChild{objectID: object.ObjectID}
	seen := make(map[string]struct{})
	for _, attribute := range strings.Split(string(object.Value), ";") {
		key, value, found := strings.Cut(attribute, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
		seen[key] = struct{}{}
		switch key {
		case "1":
			child.model = value
		case "2":
			child.softwareVersion = value
		case "3":
			child.protocolVersion = value
		case "4":
			// Sensitive input is intentionally validated but never retained.
		case "5":
			child.address = value
		case "6":
			child.featureVersion = value
		case "8":
			child.productType = value
		default:
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
	}
	if child.model == "" {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
	}
	return child, nil
}

// parseHuaweiDocumentaryInventoryChild decodes one child object under the
// device-list dictionary of the consulted editions: keys 1, 2, 3 and 5 by their
// canonical names, key 8 as the product type, key 4 validated and never
// retained, and every other key, key 6 among them, retained as opaque native
// evidence. Key 6 has a different grammar in different editions and is never
// decoded. A key that is not a canonical attribute number, a repeated key or a
// malformed pair rejects the object.
func parseHuaweiDocumentaryInventoryChild(object SmartLoggerInventoryObject) (SmartLoggerInventoryChild, error) {
	if len(object.Value) == 0 || !isHuaweiSmartLoggerInventoryASCII(object.Value) {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
	}
	child := SmartLoggerInventoryChild{objectID: object.ObjectID}
	seen := make(map[string]struct{})
	for _, attribute := range strings.Split(string(object.Value), ";") {
		key, value, found := strings.Cut(attribute, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
		seen[key] = struct{}{}
		switch key {
		case "1":
			child.model = value
		case "2":
			child.softwareVersion = value
		case "3":
			child.protocolVersion = value
		case "4":
			// Sensitive input is intentionally validated but never retained.
		case "5":
			child.address = value
		case "8":
			child.productType = value
		default:
			if _, ok := huaweiDescriptorKeyNumber(key); !ok {
				return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
			}
			child.retainOpaque(key, value)
		}
	}
	if child.model == "" {
		return SmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
	}
	return child, nil
}

func isHuaweiSmartLoggerInventoryASCII(value []byte) bool {
	for _, character := range value {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}
