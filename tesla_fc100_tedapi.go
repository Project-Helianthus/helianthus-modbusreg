package modbusreg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TeslaFC100TEDAPI is a bounded, redacted FC100 TEDAPI replay projection. It
// identifies only the first protobuf key shape and never assigns field names,
// operation meaning, admission, or send authority.
type TeslaFC100TEDAPI struct {
	messageLength int
	firstField    uint64
	firstWireType uint8
	payloadDigest string
}

// DecodeTeslaFC100TEDAPI validates the exact FC100 length envelope and parses
// one bounded protobuf key varint from its opaque nested message.
func DecodeTeslaFC100TEDAPI(payload []byte) (TeslaFC100TEDAPI, error) {
	envelope, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100TEDAPI{}, err
	}
	message := envelope.Payload()
	if len(message) == 0 {
		return TeslaFC100TEDAPI{}, fmt.Errorf("tesla FC100 TEDAPI message is empty")
	}
	key, consumed, err := decodeTeslaTEDAPIKey(message)
	if err != nil {
		return TeslaFC100TEDAPI{}, err
	}
	field := key >> 3
	wireType := uint8(key & 0x07)
	if consumed == 0 || field == 0 || field > (1<<29)-1 || wireType > 5 {
		return TeslaFC100TEDAPI{}, fmt.Errorf("tesla FC100 TEDAPI key is invalid")
	}
	digest := sha256.Sum256(message)
	return TeslaFC100TEDAPI{
		messageLength: len(message),
		firstField:    field,
		firstWireType: wireType,
		payloadDigest: hex.EncodeToString(digest[:]),
	}, nil
}

func decodeTeslaTEDAPIKey(message []byte) (uint64, int, error) {
	var value uint64
	for index, byteValue := range message {
		if index == 10 || (index == 9 && byteValue > 1) {
			return 0, 0, fmt.Errorf("tesla FC100 TEDAPI key overflows")
		}
		value |= uint64(byteValue&0x7f) << (7 * index)
		if byteValue&0x80 == 0 {
			return value, index + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("tesla FC100 TEDAPI key is truncated")
}

// MessageLength returns the bounded nested-message length.
func (decoded TeslaFC100TEDAPI) MessageLength() int { return decoded.messageLength }

// FirstFieldNumber returns only the numeric first protobuf field identifier.
func (decoded TeslaFC100TEDAPI) FirstFieldNumber() uint64 { return decoded.firstField }

// FirstWireType returns the numeric wire type of the first key.
func (decoded TeslaFC100TEDAPI) FirstWireType() uint8 { return decoded.firstWireType }

// PayloadDigest returns deterministic redacted replay provenance.
func (decoded TeslaFC100TEDAPI) PayloadDigest() string { return decoded.payloadDigest }

// Payload is always nil: raw nested bytes remain outside this projection.
func (TeslaFC100TEDAPI) Payload() []byte { return nil }
