package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TEDAPIOperationCommonSystemInfoV1 identifies the explicitly qualified
// FC100 Common system-information operation in this compatibility contract.
const TEDAPIOperationCommonSystemInfoV1 = "tesla.hsc.fc100.common_system_info.v1"

var teslaFC100CommonSystemInfoRequestPDU = []byte{0x04, 0x22, 0x02, 0x12, 0x00}

// FC100CommonSystemInfoReplayKind distinguishes an echoed FC100
// intermediate from the bounded opaque terminal body.
type FC100CommonSystemInfoReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the opaque body.
const (
	FC100CommonSystemInfoIntermediate FC100CommonSystemInfoReplayKind = "intermediate"
	FC100CommonSystemInfoTerminal     FC100CommonSystemInfoReplayKind = "terminal"
)

// FC100CommonSystemInfoReplay is a redacted replay result. It never
// contains terminal-body bytes or field-level system-information semantics.
type FC100CommonSystemInfoReplay struct {
	Kind           FC100CommonSystemInfoReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100CommonSystemInfoReplay accepts only the exact FC100 echo or
// one bounded Common family-4/tag-3 terminal. Terminal content remains opaque.
func DecodeTeslaFC100CommonSystemInfoReplay(payload []byte) (FC100CommonSystemInfoReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return FC100CommonSystemInfoReplay{}, err
	}
	message := response.Payload()
	requestMessage := teslaFC100CommonSystemInfoRequestPDU[1:]
	if bytes.Equal(message, requestMessage) {
		return FC100CommonSystemInfoReplay{Kind: FC100CommonSystemInfoIntermediate}, nil
	}
	commonMessages, err := decodeExactLengthDelimitedField(message, 4)
	if err != nil {
		return FC100CommonSystemInfoReplay{}, fmt.Errorf("tesla Common system-information envelope: %w", err)
	}
	systemInfoResponse, err := decodeExactLengthDelimitedField(commonMessages, 3)
	if err != nil {
		return FC100CommonSystemInfoReplay{}, fmt.Errorf("tesla Common system-information message: %w", err)
	}
	digest := sha256.Sum256(systemInfoResponse)
	return FC100CommonSystemInfoReplay{
		Kind:           FC100CommonSystemInfoTerminal,
		SnapshotLength: len(systemInfoResponse),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}, nil
}
