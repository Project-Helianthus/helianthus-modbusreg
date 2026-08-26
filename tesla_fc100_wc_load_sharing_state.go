package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TeslaTEDAPIOperationWCLoadSharingStateV1 identifies the explicitly
// qualified FC100 WC load-sharing-state operation.
const TeslaTEDAPIOperationWCLoadSharingStateV1 = "tesla.hsc.fc100.wc_load_sharing_state.v1"

var teslaFC100WCLoadSharingStateRequestPDU = []byte{0x04, 0x32, 0x02, 0x5a, 0x00}

// TeslaFC100WCLoadSharingStateReplayKind distinguishes an immediate echo from
// the opaque, terminal load-sharing-state body.
type TeslaFC100WCLoadSharingStateReplayKind string

const (
	TeslaFC100WCLoadSharingStateIntermediate TeslaFC100WCLoadSharingStateReplayKind = "intermediate"
	TeslaFC100WCLoadSharingStateTerminal     TeslaFC100WCLoadSharingStateReplayKind = "terminal"
)

// TeslaFC100WCLoadSharingStateReplay retains only redacted terminal metadata.
// It intentionally has no typed inner fields or raw body bytes.
type TeslaFC100WCLoadSharingStateReplay struct {
	Kind           TeslaFC100WCLoadSharingStateReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100WCLoadSharingStateReplay accepts one bounded FC100 echo or
// one terminal F6/tag-12 opaque body.
func DecodeTeslaFC100WCLoadSharingStateReplay(payload []byte) (TeslaFC100WCLoadSharingStateReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100WCLoadSharingStateReplay{}, err
	}
	message := response.Payload()
	if bytes.Equal(message, teslaFC100WCLoadSharingStateRequestPDU[1:]) {
		return TeslaFC100WCLoadSharingStateReplay{Kind: TeslaFC100WCLoadSharingStateIntermediate}, nil
	}
	wc, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return TeslaFC100WCLoadSharingStateReplay{}, fmt.Errorf("tesla WC load-sharing-state envelope: %w", err)
	}
	body, err := decodeExactLengthDelimitedField(wc, 12)
	if err != nil {
		return TeslaFC100WCLoadSharingStateReplay{}, fmt.Errorf("tesla WC load-sharing-state message: %w", err)
	}
	digest := sha256.Sum256(body)
	return TeslaFC100WCLoadSharingStateReplay{
		Kind:           TeslaFC100WCLoadSharingStateTerminal,
		SnapshotLength: len(body),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}, nil
}

// DecodeTeslaFC100WCLoadSharingStateReplaySequence permits an optional
// immediate echo followed by exactly one terminal body.
func DecodeTeslaFC100WCLoadSharingStateReplaySequence(payloads [][]byte) ([]TeslaFC100WCLoadSharingStateReplay, error) {
	if len(payloads) == 0 || len(payloads) > 2 {
		return nil, fmt.Errorf("tesla WC load-sharing-state response count is invalid")
	}
	results := make([]TeslaFC100WCLoadSharingStateReplay, 0, len(payloads))
	seenIntermediate, seenTerminal := false, false
	for _, payload := range payloads {
		replay, err := DecodeTeslaFC100WCLoadSharingStateReplay(payload)
		if err != nil {
			return nil, err
		}
		if seenTerminal || replay.Kind == TeslaFC100WCLoadSharingStateIntermediate && seenIntermediate {
			return nil, fmt.Errorf("tesla WC load-sharing-state response sequence is invalid")
		}
		if replay.Kind == TeslaFC100WCLoadSharingStateIntermediate {
			seenIntermediate = true
		} else {
			seenTerminal = true
		}
		results = append(results, replay)
	}
	if !seenTerminal {
		return nil, fmt.Errorf("tesla WC load-sharing-state response has no terminal")
	}
	return results, nil
}

func decodeTeslaFC100WCLoadSharingStateSequence(payloads [][]byte) ([]QualifiedFunctionResult, error) {
	replays, err := DecodeTeslaFC100WCLoadSharingStateReplaySequence(payloads)
	if err != nil {
		return nil, err
	}
	results := make([]QualifiedFunctionResult, len(replays))
	for index, replay := range replays {
		results[index] = QualifiedFunctionResult{Replay: &QualifiedFunctionReplay{
			Kind:          string(replay.Kind),
			PayloadLength: replay.SnapshotLength,
			PayloadDigest: replay.SnapshotDigest,
		}}
	}
	return results, nil
}
