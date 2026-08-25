package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const maxTeslaFC100TEDAPIWireEntries = 64

// TeslaFC100WireEntry is one redacted numeric protobuf wire key. It carries no
// encoded value, field name, or operation meaning.
type TeslaFC100WireEntry struct {
	fieldNumber uint64
	wireType    uint8
}

// TeslaFC100TEDAPI is a bounded, redacted FC100 TEDAPI replay projection. It
// identifies ordered protobuf wire-key shapes and never assigns field names,
// operation meaning, admission, or send authority.
type TeslaFC100TEDAPI struct {
	envelopeLength int
	messageLength  int
	firstField     uint64
	firstWireType  uint8
	wireEntries    []TeslaFC100WireEntry
	payloadDigest  string
}

// DecodeTeslaFC100TEDAPI validates the exact FC100 length envelope and parses
// a bounded complete protobuf wire summary from its opaque nested message.
func DecodeTeslaFC100TEDAPI(payload []byte) (TeslaFC100TEDAPI, error) {
	envelope, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100TEDAPI{}, err
	}
	message := envelope.Payload()
	if len(message) == 0 {
		return TeslaFC100TEDAPI{}, fmt.Errorf("tesla FC100 TEDAPI message is empty")
	}
	entries, err := decodeTeslaFC100TEDAPIWireEntries(message)
	if err != nil {
		return TeslaFC100TEDAPI{}, err
	}
	digest := sha256.Sum256(message)
	return TeslaFC100TEDAPI{
		envelopeLength: len(payload),
		messageLength:  len(message),
		firstField:     entries[0].fieldNumber,
		firstWireType:  entries[0].wireType,
		wireEntries:    append([]TeslaFC100WireEntry(nil), entries...),
		payloadDigest:  hex.EncodeToString(digest[:]),
	}, nil
}

func decodeTeslaFC100TEDAPIWireEntries(message []byte) ([]TeslaFC100WireEntry, error) {
	entries := make([]TeslaFC100WireEntry, 0, maxTeslaFC100TEDAPIWireEntries)
	groups := make([]uint64, 0, maxTeslaFC100TEDAPIWireEntries)
	for offset := 0; offset < len(message); {
		key, consumed, err := decodeTeslaTEDAPIVarint(message[offset:])
		if err != nil {
			return nil, err
		}
		offset += consumed
		field := key >> 3
		wireType := uint8(key & 0x07)
		if field == 0 || field > (1<<29)-1 || wireType > 5 {
			return nil, fmt.Errorf("tesla FC100 TEDAPI key is invalid")
		}
		if len(entries) == maxTeslaFC100TEDAPIWireEntries {
			return nil, fmt.Errorf("tesla FC100 TEDAPI wire entry count exceeds bound")
		}
		entries = append(entries, TeslaFC100WireEntry{fieldNumber: field, wireType: wireType})
		switch wireType {
		case 0:
			_, valueLength, err := decodeTeslaTEDAPIVarint(message[offset:])
			if err != nil {
				return nil, err
			}
			offset += valueLength
		case 1:
			if len(message)-offset < 8 {
				return nil, fmt.Errorf("tesla FC100 TEDAPI fixed64 value is truncated")
			}
			offset += 8
		case 2:
			valueLength, lengthLength, err := decodeTeslaTEDAPIVarint(message[offset:])
			if err != nil {
				return nil, err
			}
			offset += lengthLength
			if valueLength > uint64(len(message)-offset) {
				return nil, fmt.Errorf("tesla FC100 TEDAPI length-delimited value is truncated")
			}
			offset += int(valueLength)
		case 3:
			groups = append(groups, field)
		case 4:
			if len(groups) == 0 || groups[len(groups)-1] != field {
				return nil, fmt.Errorf("tesla FC100 TEDAPI group boundary is invalid")
			}
			groups = groups[:len(groups)-1]
		case 5:
			if len(message)-offset < 4 {
				return nil, fmt.Errorf("tesla FC100 TEDAPI fixed32 value is truncated")
			}
			offset += 4
		}
	}
	if len(groups) != 0 {
		return nil, fmt.Errorf("tesla FC100 TEDAPI group boundary is truncated")
	}
	return entries, nil
}

func decodeTeslaTEDAPIVarint(message []byte) (uint64, int, error) {
	var value uint64
	for index, byteValue := range message {
		if index == 10 || (index == 9 && byteValue > 1) {
			return 0, 0, fmt.Errorf("tesla FC100 TEDAPI varint overflows")
		}
		value |= uint64(byteValue&0x7f) << (7 * index)
		if byteValue&0x80 == 0 {
			return value, index + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("tesla FC100 TEDAPI varint is truncated")
}

// EnvelopeLength returns the bounded FC100 PDU length including its prefix.
func (decoded TeslaFC100TEDAPI) EnvelopeLength() int { return decoded.envelopeLength }

// MessageLength returns the bounded nested-message length.
func (decoded TeslaFC100TEDAPI) MessageLength() int { return decoded.messageLength }

// FirstFieldNumber returns only the numeric first protobuf field identifier.
func (decoded TeslaFC100TEDAPI) FirstFieldNumber() uint64 { return decoded.firstField }

// FirstWireType returns the numeric wire type of the first key.
func (decoded TeslaFC100TEDAPI) FirstWireType() uint8 { return decoded.firstWireType }

// WireEntries returns independent ordered redacted wire-key metadata.
func (decoded TeslaFC100TEDAPI) WireEntries() []TeslaFC100WireEntry {
	return append([]TeslaFC100WireEntry(nil), decoded.wireEntries...)
}

// WireEntryCount returns the bounded number of retained wire entries.
func (decoded TeslaFC100TEDAPI) WireEntryCount() int { return len(decoded.wireEntries) }

// FieldNumber returns the numeric protobuf field identifier.
func (entry TeslaFC100WireEntry) FieldNumber() uint64 { return entry.fieldNumber }

// WireType returns the numeric protobuf wire type.
func (entry TeslaFC100WireEntry) WireType() uint8 { return entry.wireType }

// PayloadDigest returns deterministic redacted replay provenance.
func (decoded TeslaFC100TEDAPI) PayloadDigest() string { return decoded.payloadDigest }

// Payload is always nil: raw nested bytes remain outside this projection.
func (TeslaFC100TEDAPI) Payload() []byte { return nil }
