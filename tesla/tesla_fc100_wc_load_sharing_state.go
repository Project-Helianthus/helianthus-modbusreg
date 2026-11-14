package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// TEDAPIOperationWCLoadSharingStateV1 identifies the explicitly
// qualified FC100 WC load-sharing-state operation.
const TEDAPIOperationWCLoadSharingStateV1 = "tesla.hsc.fc100.wc_load_sharing_state.v1"

var teslaFC100WCLoadSharingStateRequestPDU = []byte{0x04, 0x32, 0x02, 0x5a, 0x00}

// FC100WCLoadSharingStateReplayKind distinguishes an immediate echo from
// the opaque, terminal load-sharing-state body.
type FC100WCLoadSharingStateReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the opaque body.
const (
	FC100WCLoadSharingStateIntermediate FC100WCLoadSharingStateReplayKind = "intermediate"
	FC100WCLoadSharingStateTerminal     FC100WCLoadSharingStateReplayKind = "terminal"
)

// FC100WCLoadSharingStateReplay retains only redacted terminal metadata.
// It intentionally has no typed inner fields or raw body bytes.
type FC100WCLoadSharingStateReplay struct {
	Kind           FC100WCLoadSharingStateReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100WCLoadSharingStateReplay accepts one bounded FC100 echo or
// one terminal F6/tag-12 opaque body.
func DecodeTeslaFC100WCLoadSharingStateReplay(payload []byte) (FC100WCLoadSharingStateReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return FC100WCLoadSharingStateReplay{}, err
	}
	message := response.Payload()
	if bytes.Equal(message, teslaFC100WCLoadSharingStateRequestPDU[1:]) {
		return FC100WCLoadSharingStateReplay{Kind: FC100WCLoadSharingStateIntermediate}, nil
	}
	wc, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return FC100WCLoadSharingStateReplay{}, fmt.Errorf("tesla WC load-sharing-state envelope: %w", err)
	}
	body, err := decodeExactLengthDelimitedField(wc, 12)
	if err != nil {
		return FC100WCLoadSharingStateReplay{}, fmt.Errorf("tesla WC load-sharing-state message: %w", err)
	}
	digest := sha256.Sum256(body)
	return FC100WCLoadSharingStateReplay{
		Kind:           FC100WCLoadSharingStateTerminal,
		SnapshotLength: len(body),
		SnapshotDigest: hex.EncodeToString(digest[:]),
	}, nil
}

// DecodeTeslaFC100WCLoadSharingStateReplaySequence permits an optional
// immediate echo followed by exactly one terminal body.
func DecodeTeslaFC100WCLoadSharingStateReplaySequence(payloads [][]byte) ([]FC100WCLoadSharingStateReplay, error) {
	if len(payloads) == 0 || len(payloads) > 2 {
		return nil, fmt.Errorf("tesla WC load-sharing-state response count is invalid")
	}
	results := make([]FC100WCLoadSharingStateReplay, 0, len(payloads))
	seenIntermediate, seenTerminal := false, false
	for _, payload := range payloads {
		replay, err := DecodeTeslaFC100WCLoadSharingStateReplay(payload)
		if err != nil {
			return nil, err
		}
		if seenTerminal || replay.Kind == FC100WCLoadSharingStateIntermediate && seenIntermediate {
			return nil, fmt.Errorf("tesla WC load-sharing-state response sequence is invalid")
		}
		if replay.Kind == FC100WCLoadSharingStateIntermediate {
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

func decodeTeslaFC100WCLoadSharingStateSequence(payloads [][]byte) ([]modbusreg.QualifiedFunctionResult, error) {
	replays, err := DecodeTeslaFC100WCLoadSharingStateReplaySequence(payloads)
	if err != nil {
		return nil, err
	}
	results := make([]modbusreg.QualifiedFunctionResult, len(replays))
	for index, replay := range replays {
		results[index] = modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{
			Kind:          string(replay.Kind),
			PayloadLength: replay.SnapshotLength,
			PayloadDigest: replay.SnapshotDigest,
		}}
	}
	return results, nil
}
