package modbusreg

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

// HuaweiSmartLoggerOfflineInventoryInput is a detached, already-read extended
// MEI inventory. It deliberately contains no endpoint, unit, or transport
// state and cannot enable a profile or operation.
type HuaweiSmartLoggerOfflineInventoryInput struct {
	ChangeCounterBefore uint16
	ChangeCounterAfter  uint16
	Pages               []HuaweiSmartLoggerInventoryPage
}

// HuaweiSmartLoggerInventoryPage is one already-decoded Read Device ID page.
type HuaweiSmartLoggerInventoryPage struct {
	More         bool
	NextObjectID uint8
	Objects      []HuaweiSmartLoggerInventoryObject
}

// HuaweiSmartLoggerInventoryObject retains a single opaque inventory object
// until its bounded attribute form has been validated.
type HuaweiSmartLoggerInventoryObject struct {
	ObjectID uint8
	Value    []byte
}

// HuaweiSmartLoggerInventory is a validated, default-denied child snapshot.
type HuaweiSmartLoggerInventory struct {
	declaredChildren int
	children         []HuaweiSmartLoggerInventoryChild
}

func (i HuaweiSmartLoggerInventory) DeclaredChildren() int { return i.declaredChildren }

func (i HuaweiSmartLoggerInventory) ChildCount() int { return len(i.children) }

func (i HuaweiSmartLoggerInventory) DefaultDenied() bool { return true }

func (i HuaweiSmartLoggerInventory) Children() []HuaweiSmartLoggerInventoryChild {
	return append([]HuaweiSmartLoggerInventoryChild(nil), i.children...)
}

// HuaweiSmartLoggerInventoryChild contains only non-sensitive child fields.
// Attribute values are accessed by their canonical names; the sensitive input
// attribute is validated but never retained.
type HuaweiSmartLoggerInventoryChild struct {
	objectID        uint8
	model           string
	softwareVersion string
	protocolVersion string
	address         string
	featureVersion  string
	productType     string
}

func (c HuaweiSmartLoggerInventoryChild) ObjectID() uint8 { return c.objectID }

func (c HuaweiSmartLoggerInventoryChild) Address() string { return c.address }

func (c HuaweiSmartLoggerInventoryChild) Attribute(name string) string {
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
func ParseHuaweiSmartLoggerOfflineInventory(input HuaweiSmartLoggerOfflineInventoryInput) (HuaweiSmartLoggerInventory, error) {
	if input.ChangeCounterBefore != input.ChangeCounterAfter || len(input.Pages) == 0 || len(input.Pages) > huaweiSmartLoggerInventoryMaxPages {
		return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
	}

	expectedObjectID := uint8(huaweiSmartLoggerInventoryStartObject)
	seenObjects := make(map[uint8]struct{})
	seenAddresses := make(map[string]struct{})
	var (
		declaredChildren = -1
		children         []HuaweiSmartLoggerInventoryChild
		selfSeen         bool
		wrapped          bool
		objectCount      int
		byteCount        int
	)

	for pageIndex, page := range input.Pages {
		if len(page.Objects) == 0 {
			return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
		}
		for _, object := range page.Objects {
			if objectCount == huaweiSmartLoggerInventoryMaxObjects || object.ObjectID != expectedObjectID {
				return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			if _, alreadySeen := seenObjects[object.ObjectID]; alreadySeen {
				return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			objectCount++
			seenObjects[object.ObjectID] = struct{}{}
			if len(object.Value) > huaweiSmartLoggerInventoryMaxBytes-byteCount-2 {
				return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			byteCount += len(object.Value) + 2

			if object.ObjectID == huaweiSmartLoggerInventoryStartObject {
				if pageIndex != 0 || len(seenObjects) != 1 || len(object.Value) != 1 || object.Value[0] > huaweiSmartLoggerInventoryMaxChildren {
					return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				declaredChildren = int(object.Value[0])
			} else {
				child, err := parseHuaweiSmartLoggerInventoryChild(object)
				if err != nil {
					return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				if child.model == "SmartLogger" {
					if selfSeen || len(children) != 0 {
						return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					selfSeen = true
				} else {
					if !selfSeen || child.address == "" {
						return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					if _, duplicateAddress := seenAddresses[child.address]; duplicateAddress {
						return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
					}
					seenAddresses[child.address] = struct{}{}
					children = append(children, child)
				}
			}

			nextObjectID := object.ObjectID + 1
			if nextObjectID == 0 && object.ObjectID == 0xff {
				if wrapped {
					return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
				}
				wrapped = true
			}
			expectedObjectID = nextObjectID
		}
		if page.More {
			if page.NextObjectID != expectedObjectID || pageIndex == len(input.Pages)-1 {
				return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
			}
			continue
		}
		if page.NextObjectID != 0 || pageIndex != len(input.Pages)-1 {
			return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
		}
	}

	if declaredChildren < 0 || !selfSeen || declaredChildren != len(children) {
		return HuaweiSmartLoggerInventory{}, errHuaweiSmartLoggerInventoryInvalid
	}
	return HuaweiSmartLoggerInventory{declaredChildren: declaredChildren, children: append([]HuaweiSmartLoggerInventoryChild(nil), children...)}, nil
}

func parseHuaweiSmartLoggerInventoryChild(object HuaweiSmartLoggerInventoryObject) (HuaweiSmartLoggerInventoryChild, error) {
	if len(object.Value) == 0 || !isHuaweiSmartLoggerInventoryASCII(object.Value) {
		return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
	}
	child := HuaweiSmartLoggerInventoryChild{objectID: object.ObjectID}
	seen := make(map[string]struct{})
	for _, attribute := range strings.Split(string(object.Value), ";") {
		key, value, found := strings.Cut(attribute, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") {
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
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
			return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
		}
	}
	if child.model == "" {
		return HuaweiSmartLoggerInventoryChild{}, errHuaweiSmartLoggerInventoryInvalid
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
