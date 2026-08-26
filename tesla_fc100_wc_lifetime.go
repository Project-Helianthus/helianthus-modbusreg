package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const TeslaTEDAPIOperationWCLifetimeV1 = "tesla.hsc.fc100.wc_lifetime.v1"

var teslaFC100WCLifetimeRequestPDU = []byte{0x04, 0x32, 0x02, 0x1a, 0x00}

type TeslaFC100WCLifetimeReplayKind string

const (
	TeslaFC100WCLifetimeIntermediate TeslaFC100WCLifetimeReplayKind = "intermediate"
	TeslaFC100WCLifetimeTerminal     TeslaFC100WCLifetimeReplayKind = "terminal"
)

type TeslaFC100WCLifetimeReplay struct {
	Kind           TeslaFC100WCLifetimeReplayKind
	SnapshotLength int
	SnapshotDigest string
}

func DecodeTeslaFC100WCLifetimeReplay(payload []byte) (TeslaFC100WCLifetimeReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100WCLifetimeReplay{}, err
	}
	message := response.Payload()
	if bytes.Equal(message, teslaFC100WCLifetimeRequestPDU[1:]) {
		return TeslaFC100WCLifetimeReplay{Kind: TeslaFC100WCLifetimeIntermediate}, nil
	}
	wc, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return TeslaFC100WCLifetimeReplay{}, fmt.Errorf("tesla WC lifetime envelope: %w", err)
	}
	body, err := decodeExactLengthDelimitedField(wc, 4)
	if err != nil {
		return TeslaFC100WCLifetimeReplay{}, fmt.Errorf("tesla WC lifetime message: %w", err)
	}
	digest := sha256.Sum256(body)
	return TeslaFC100WCLifetimeReplay{Kind: TeslaFC100WCLifetimeTerminal, SnapshotLength: len(body), SnapshotDigest: hex.EncodeToString(digest[:])}, nil
}

func DecodeTeslaFC100WCLifetimeReplaySequence(payloads [][]byte) ([]TeslaFC100WCLifetimeReplay, error) {
	if len(payloads) == 0 || len(payloads) > 2 {
		return nil, fmt.Errorf("tesla WC lifetime response count is invalid")
	}
	results := make([]TeslaFC100WCLifetimeReplay, 0, len(payloads))
	seenIntermediate, seenTerminal := false, false
	for _, payload := range payloads {
		replay, err := DecodeTeslaFC100WCLifetimeReplay(payload)
		if err != nil {
			return nil, err
		}
		if seenTerminal || replay.Kind == TeslaFC100WCLifetimeIntermediate && seenIntermediate || replay.Kind == TeslaFC100WCLifetimeTerminal && seenTerminal {
			return nil, fmt.Errorf("tesla WC lifetime response sequence is invalid")
		}
		if replay.Kind == TeslaFC100WCLifetimeIntermediate {
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
