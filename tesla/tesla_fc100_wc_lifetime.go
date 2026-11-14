package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// TEDAPIOperationWCLifetimeV1 identifies the explicitly qualified FC100 WC
// lifetime-statistics operation; see
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/tesla/tedapi.md.
const TEDAPIOperationWCLifetimeV1 = "tesla.hsc.fc100.wc_lifetime.v1"

var teslaFC100WCLifetimeRequestPDU = []byte{0x04, 0x32, 0x02, 0x1a, 0x00}

// FC100WCLifetimeReplayKind distinguishes an echoed FC100 intermediate from
// the opaque terminal lifetime body.
type FC100WCLifetimeReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the opaque body.
const (
	FC100WCLifetimeIntermediate FC100WCLifetimeReplayKind = "intermediate"
	FC100WCLifetimeTerminal     FC100WCLifetimeReplayKind = "terminal"
)

// FC100WCLifetimeReplay retains only redacted terminal metadata: it has no
// typed fields and never contains body bytes.
type FC100WCLifetimeReplay struct {
	Kind           FC100WCLifetimeReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100WCLifetimeReplay accepts one FC100 response that is either the
// exact request echo or a terminal family-6/tag-4 body; anything else is an error.
func DecodeTeslaFC100WCLifetimeReplay(payload []byte) (FC100WCLifetimeReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return FC100WCLifetimeReplay{}, err
	}
	message := response.Payload()
	if bytes.Equal(message, teslaFC100WCLifetimeRequestPDU[1:]) {
		return FC100WCLifetimeReplay{Kind: FC100WCLifetimeIntermediate}, nil
	}
	wc, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return FC100WCLifetimeReplay{}, fmt.Errorf("tesla WC lifetime envelope: %w", err)
	}
	body, err := decodeExactLengthDelimitedField(wc, 4)
	if err != nil {
		return FC100WCLifetimeReplay{}, fmt.Errorf("tesla WC lifetime message: %w", err)
	}
	digest := sha256.Sum256(body)
	return FC100WCLifetimeReplay{Kind: FC100WCLifetimeTerminal, SnapshotLength: len(body), SnapshotDigest: hex.EncodeToString(digest[:])}, nil
}

// DecodeTeslaFC100WCLifetimeReplaySequence accepts one or two responses: at most
// one echo, followed by exactly one terminal. Any other order or count is rejected.
func DecodeTeslaFC100WCLifetimeReplaySequence(payloads [][]byte) ([]FC100WCLifetimeReplay, error) {
	if len(payloads) == 0 || len(payloads) > 2 {
		return nil, fmt.Errorf("tesla WC lifetime response count is invalid")
	}
	results := make([]FC100WCLifetimeReplay, 0, len(payloads))
	seenIntermediate, seenTerminal := false, false
	for _, payload := range payloads {
		replay, err := DecodeTeslaFC100WCLifetimeReplay(payload)
		if err != nil {
			return nil, err
		}
		if seenTerminal || replay.Kind == FC100WCLifetimeIntermediate && seenIntermediate || replay.Kind == FC100WCLifetimeTerminal && seenTerminal {
			return nil, fmt.Errorf("tesla WC lifetime response sequence is invalid")
		}
		if replay.Kind == FC100WCLifetimeIntermediate {
			seenIntermediate = true
		} else {
			seenTerminal = true
		}
		results = append(results, replay)
	}
	if !seenTerminal {
		return nil, fmt.Errorf("tesla WC lifetime response has no terminal")
	}
	return results, nil
}
