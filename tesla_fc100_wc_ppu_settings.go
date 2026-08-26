package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

var teslaFC100WCPPUSettingsRequestPDU = []byte{0x05, 0x32, 0x03, 0xba, 0x01, 0x00}

type TeslaFC100WCPPUSettingsReplayKind string

const (
	TeslaFC100WCPPUSettingsIntermediate TeslaFC100WCPPUSettingsReplayKind = "intermediate"
	TeslaFC100WCPPUSettingsTerminal     TeslaFC100WCPPUSettingsReplayKind = "terminal"
)

type TeslaFC100WCPPUSettingsReplay struct {
	Kind           TeslaFC100WCPPUSettingsReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// TeslaFC100WCPPUSettingsReplayDecoder decodes injected offline replay only.
// It owns no request construction, exchange, serial endpoint, or admission.
type TeslaFC100WCPPUSettingsReplayDecoder struct{}

func NewTeslaFC100WCPPUSettingsReplayDecoder(profile TeslaHSCProfile) (*TeslaFC100WCPPUSettingsReplayDecoder, error) {
	if profile.Disposition() != TeslaHSCQualifiedReadOnly ||
		profile.config.WCPPUSettingsReplayVersion != TeslaHSCWCPPUSettingsReplayCompatibilityV1 {
		return nil, ErrQualifiedFunctionNoSend
	}
	return &TeslaFC100WCPPUSettingsReplayDecoder{}, nil
}

func (TeslaFC100WCPPUSettingsReplayDecoder) Decode(payloads [][]byte) ([]TeslaFC100WCPPUSettingsReplay, error) {
	return DecodeTeslaFC100WCPPUSettingsReplaySequence(payloads)
}

func DecodeTeslaFC100WCPPUSettingsReplay(payload []byte) (TeslaFC100WCPPUSettingsReplay, error) {
	response, err := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if err != nil {
		return TeslaFC100WCPPUSettingsReplay{}, err
	}
	message := response.Payload()
	if bytes.Equal(message, teslaFC100WCPPUSettingsRequestPDU[1:]) {
		return TeslaFC100WCPPUSettingsReplay{Kind: TeslaFC100WCPPUSettingsIntermediate}, nil
	}
	wc, err := decodeExactLengthDelimitedField(message, 6)
	if err != nil {
		return TeslaFC100WCPPUSettingsReplay{}, fmt.Errorf("tesla WC PPU settings envelope: %w", err)
	}
	body, err := decodeExactLengthDelimitedField(wc, 24)
	if err != nil {
		return TeslaFC100WCPPUSettingsReplay{}, fmt.Errorf("tesla WC PPU settings message: %w", err)
	}
	digest := sha256.Sum256(body)
	return TeslaFC100WCPPUSettingsReplay{Kind: TeslaFC100WCPPUSettingsTerminal, SnapshotLength: len(body), SnapshotDigest: hex.EncodeToString(digest[:])}, nil
}

func DecodeTeslaFC100WCPPUSettingsReplaySequence(payloads [][]byte) ([]TeslaFC100WCPPUSettingsReplay, error) {
	if len(payloads) == 0 || len(payloads) > 2 {
		return nil, fmt.Errorf("tesla WC PPU settings response count is invalid")
	}
	results := make([]TeslaFC100WCPPUSettingsReplay, 0, len(payloads))
	seenIntermediate, seenTerminal := false, false
	for _, payload := range payloads {
		replay, err := DecodeTeslaFC100WCPPUSettingsReplay(payload)
		if err != nil {
			return nil, err
		}
		if seenTerminal || replay.Kind == TeslaFC100WCPPUSettingsIntermediate && seenIntermediate {
			return nil, fmt.Errorf("tesla WC PPU settings response sequence is invalid")
		}
		if replay.Kind == TeslaFC100WCPPUSettingsIntermediate {
			seenIntermediate = true
		} else {
			seenTerminal = true
		}
		results = append(results, replay)
	}
	if !seenTerminal {
		return nil, fmt.Errorf("tesla WC PPU settings response has no terminal")
	}
	return results, nil
}
