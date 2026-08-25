package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TeslaTEDAPIOperationWCVitalsV1 identifies the explicitly qualified FC100
// WC vitals operation in this compatibility contract.
const TeslaTEDAPIOperationWCVitalsV1 = "tesla.hsc.fc100.wc_vitals.v1"

var teslaFC100WCVitalsRequestPDU = []byte{0x04, 0x32, 0x02, 0x0a, 0x00}

// TeslaFC100WCVitalsReplayKind distinguishes the echoed FC100 intermediate
// from the qualified terminal snapshot without exposing field values.
type TeslaFC100WCVitalsReplayKind string

const (
	TeslaFC100WCVitalsIntermediate TeslaFC100WCVitalsReplayKind = "intermediate"
	TeslaFC100WCVitalsTerminal     TeslaFC100WCVitalsReplayKind = "terminal"
)

// TeslaFC100WCVitalsReplay is a bounded redacted result of the terminal-body
// decoder. Snapshot values and raw bytes are intentionally omitted.
type TeslaFC100WCVitalsReplay struct {
	Kind           TeslaFC100WCVitalsReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// OperationAdmissionFor applies the profile gate and the operation-specific
// exact version gate. Unknown operations remain no-send.
func (profile TeslaHSCProfile) OperationAdmissionFor(operation string) TeslaTEDAPIOperationAdmission {
	if profile.Disposition() != TeslaHSCQualifiedReadOnly {
		return TeslaTEDAPIOperationAdmission{State: TeslaTEDAPIAdmissionBlockedProfile}
	}
	switch operation {
	case TeslaTEDAPIOperationWCVitalsV1:
		if profile.config.WCVitalsOperationVersion == TeslaHSCWCVitalsCompatibilityV1 {
			return TeslaTEDAPIOperationAdmission{
				State:           TeslaTEDAPIAdmissionAllowedWCVitals,
				OutboundAllowed: true,
			}
		}
	case TeslaTEDAPIOperationCommonSystemInfoV1:
		if profile.config.SystemInfoOperationVersion == TeslaHSCSystemInfoCompatibilityV1 {
			return TeslaTEDAPIOperationAdmission{
				State:           TeslaTEDAPIAdmissionAllowedCommonSystemInfo,
				OutboundAllowed: true,
			}
		}
	}
	return TeslaTEDAPIOperationAdmission{State: TeslaTEDAPIAdmissionBlockedNoAdmissibleOperation}
}

// DecodeTeslaFC100WCVitalsReplay decodes the exact FC100 WC vitals operation
// shape. It accepts the byte-exact echo or one fully bounded terminal response.
func DecodeTeslaFC100WCVitalsReplay(payload []byte) (TeslaFC100WCVitalsReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100WCVitalsReplay{}, err
	}
	message := response.Payload()
	requestMessage := teslaFC100WCVitalsRequestPDU[1:]
	if bytes.Equal(message, requestMessage) {
		return TeslaFC100WCVitalsReplay{Kind: TeslaFC100WCVitalsIntermediate}, nil
	}
	wcMessages, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return TeslaFC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals envelope: %w", err)
	}
	vitalsResponse, err := decodeExactLengthDelimitedField(wcMessages, 2)
	if err != nil {
		return TeslaFC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals message: %w", err)
	}
	digest := sha256.Sum256(vitalsResponse)
	return TeslaFC100WCVitalsReplay{
		Kind:           TeslaFC100WCVitalsTerminal,
		SnapshotLength: len(vitalsResponse),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}, nil
}

func decodeExactLengthDelimitedField(message []byte, fieldNumber uint64) ([]byte, error) {
	key, width, err := decodeTeslaFC100Varint(message)
	if err != nil || key != fieldNumber<<3|2 {
		return nil, fmt.Errorf("expected one length-delimited field")
	}
	length, lengthWidth, err := decodeTeslaFC100Varint(message[width:])
	if err != nil || length > uint64(len(message)-width-lengthWidth) ||
		int(length) != len(message)-width-lengthWidth {
		return nil, fmt.Errorf("field length is invalid")
	}
	return append([]byte(nil), message[width+lengthWidth:]...), nil
}

func decodeTeslaFC100Varint(input []byte) (uint64, int, error) {
	var value uint64
	for index, octet := range input {
		if index == 10 || index == 9 && octet > 1 {
			return 0, 0, fmt.Errorf("varint overflows")
		}
		value |= uint64(octet&0x7f) << (7 * index)
		if octet&0x80 == 0 {
			return value, index + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("varint is truncated")
}
