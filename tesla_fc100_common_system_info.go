package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TeslaTEDAPIOperationCommonSystemInfoV1 identifies the explicitly qualified
// FC100 Common system-information operation in this compatibility contract.
const TeslaTEDAPIOperationCommonSystemInfoV1 = "tesla.hsc.fc100.common_system_info.v1"

var teslaFC100CommonSystemInfoRequestPDU = []byte{0x04, 0x22, 0x02, 0x12, 0x00}

// TeslaFC100CommonSystemInfoReplayKind distinguishes an echoed FC100
// intermediate from the bounded opaque terminal body.
type TeslaFC100CommonSystemInfoReplayKind string

const (
	TeslaFC100CommonSystemInfoIntermediate TeslaFC100CommonSystemInfoReplayKind = "intermediate"
	TeslaFC100CommonSystemInfoTerminal     TeslaFC100CommonSystemInfoReplayKind = "terminal"
)

// TeslaFC100CommonSystemInfoReplay is a redacted replay result. It never
// contains terminal-body bytes or field-level system-information semantics.
type TeslaFC100CommonSystemInfoReplay struct {
	Kind           TeslaFC100CommonSystemInfoReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100CommonSystemInfoReplay accepts only the exact FC100 echo or
// one bounded Common family-4/tag-3 terminal. Terminal content remains opaque.
func DecodeTeslaFC100CommonSystemInfoReplay(payload []byte) (TeslaFC100CommonSystemInfoReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100CommonSystemInfoReplay{}, err
	}
	message := response.Payload()
	requestMessage := teslaFC100CommonSystemInfoRequestPDU[1:]
	if bytes.Equal(message, requestMessage) {
		return TeslaFC100CommonSystemInfoReplay{Kind: TeslaFC100CommonSystemInfoIntermediate}, nil
	}
	commonMessages, err := decodeExactLengthDelimitedField(message, 4)
	if err != nil {
		return TeslaFC100CommonSystemInfoReplay{}, fmt.Errorf("tesla Common system-information envelope: %w", err)
	}
	systemInfoResponse, err := decodeExactLengthDelimitedField(commonMessages, 3)
	if err != nil {
		return TeslaFC100CommonSystemInfoReplay{}, fmt.Errorf("tesla Common system-information message: %w", err)
	}
	digest := sha256.Sum256(systemInfoResponse)
	return TeslaFC100CommonSystemInfoReplay{
		Kind:           TeslaFC100CommonSystemInfoTerminal,
		SnapshotLength: len(systemInfoResponse),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}, nil
}
