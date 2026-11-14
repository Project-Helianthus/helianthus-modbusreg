package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TEDAPIOperationWCVitalsV1 identifies the explicitly qualified FC100
// WC vitals operation in this compatibility contract.
const TEDAPIOperationWCVitalsV1 = "tesla.hsc.fc100.wc_vitals.v1"

var teslaFC100WCVitalsRequestPDU = []byte{0x04, 0x32, 0x02, 0x0a, 0x00}

// FC100WCVitalsReplayKind distinguishes the echoed FC100 intermediate
// from the qualified terminal snapshot without exposing field values.
type FC100WCVitalsReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the terminal body.
const (
	FC100WCVitalsIntermediate FC100WCVitalsReplayKind = "intermediate"
	FC100WCVitalsTerminal     FC100WCVitalsReplayKind = "terminal"
)

// FC100WCVitalsReplay is a bounded redacted result of the terminal-body
// decoder. Snapshot values and raw bytes are intentionally omitted.
type FC100WCVitalsReplay struct {
	Kind           FC100WCVitalsReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// OperationAdmissionFor applies the profile gate and the operation-specific
// exact version gate. Unknown operations remain no-send.
func (profile HSCProfile) OperationAdmissionFor(operation string) TEDAPIOperationAdmission {
	if profile.Disposition() != HSCQualifiedReadOnly {
		return TEDAPIOperationAdmission{State: TEDAPIAdmissionBlockedProfile}
	}
	switch operation {
	case TEDAPIOperationWCVitalsV1:
		if profile.config.WCVitalsOperationVersion == HSCWCVitalsCompatibilityV1 {
			return TEDAPIOperationAdmission{
				State:           TEDAPIAdmissionAllowedWCVitals,
				OutboundAllowed: true,
			}
		}
	case TEDAPIOperationCommonSystemInfoV1:
		if profile.config.SystemInfoOperationVersion == HSCSystemInfoCompatibilityV1 {
			return TEDAPIOperationAdmission{
				State:           TEDAPIAdmissionAllowedCommonSystemInfo,
				OutboundAllowed: true,
			}
		}
	case TEDAPIOperationWCLifetimeV1:
		if profile.config.WCLifetimeOperationVersion == HSCWCLifetimeCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCLifetime, OutboundAllowed: true}
		}
	case TEDAPIOperationWCSystemInfoV1:
		if profile.config.WCSystemInfoOperationVersion == HSCWCSystemInfoCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCSystemInfo, OutboundAllowed: true}
		}
	case TEDAPIOperationWCLoadSharingStateV1:
		if profile.config.WCLoadSharingStateOperationVersion == HSCWCLoadSharingStateCompatibilityV1 {
			return TEDAPIOperationAdmission{State: TEDAPIAdmissionAllowedWCLoadSharingState, OutboundAllowed: true}
		}
	}
	return TEDAPIOperationAdmission{State: TEDAPIAdmissionBlockedNoAdmissibleOperation}
}

// DecodeTeslaFC100WCVitalsReplay decodes the exact FC100 WC vitals operation
// shape. It accepts the byte-exact echo or one fully bounded terminal response.
func DecodeTeslaFC100WCVitalsReplay(payload []byte) (FC100WCVitalsReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return FC100WCVitalsReplay{}, err
	}
	message := response.Payload()
	requestMessage := teslaFC100WCVitalsRequestPDU[1:]
	if bytes.Equal(message, requestMessage) {
		return FC100WCVitalsReplay{Kind: FC100WCVitalsIntermediate}, nil
	}
	wcMessages, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return FC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals envelope: %w", err)
	}
	vitalsResponse, err := decodeExactLengthDelimitedField(wcMessages, 2)
	if err != nil {
		return FC100WCVitalsReplay{}, fmt.Errorf("tesla WC vitals message: %w", err)
	}
	digest := sha256.Sum256(vitalsResponse)
	return FC100WCVitalsReplay{
		Kind:           FC100WCVitalsTerminal,
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
