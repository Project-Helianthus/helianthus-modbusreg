package modbusreg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const TeslaTEDAPIOperationWCSystemInfoV1 = "tesla.hsc.fc100.wc_system_info.v1"

var teslaFC100WCSystemInfoRequestPDU = []byte{4, 0x32, 2, 0x4a, 0}

type TeslaFC100WCSystemInfoReplayKind string

const (
	TeslaFC100WCSystemInfoIntermediate TeslaFC100WCSystemInfoReplayKind = "intermediate"
	TeslaFC100WCSystemInfoTerminal     TeslaFC100WCSystemInfoReplayKind = "terminal"
)

type TeslaFC100WCSystemInfoReplay struct {
	Kind           TeslaFC100WCSystemInfoReplayKind
	SnapshotLength int
	SnapshotDigest string
}

func DecodeTeslaFC100WCSystemInfoReplay(payload []byte) (TeslaFC100WCSystemInfoReplay, error) {
	r, e := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if e != nil {
		return TeslaFC100WCSystemInfoReplay{}, e
	}
	m := r.Payload()
	if bytes.Equal(m, teslaFC100WCSystemInfoRequestPDU[1:]) {
		return TeslaFC100WCSystemInfoReplay{Kind: TeslaFC100WCSystemInfoIntermediate}, nil
	}
	w, e := decodeExactLengthDelimitedField(m, 6)
	if e != nil {
		return TeslaFC100WCSystemInfoReplay{}, e
	}
	b, e := decodeExactLengthDelimitedField(w, 10)
	if e != nil {
		return TeslaFC100WCSystemInfoReplay{}, e
	}
	d := sha256.Sum256(b)
	return TeslaFC100WCSystemInfoReplay{Kind: TeslaFC100WCSystemInfoTerminal, SnapshotLength: len(b), SnapshotDigest: hex.EncodeToString(d[:])}, nil
}
func DecodeTeslaFC100WCSystemInfoReplaySequence(p [][]byte) ([]TeslaFC100WCSystemInfoReplay, error) {
	if len(p) == 0 || len(p) > 2 {
		return nil, fmt.Errorf("tesla WC system-information response count is invalid")
	}
	out := make([]TeslaFC100WCSystemInfoReplay, 0, len(p))
	echo, term := false, false
	for _, v := range p {
		x, e := DecodeTeslaFC100WCSystemInfoReplay(v)
		if e != nil {
			return nil, e
		}
		if term || (x.Kind == TeslaFC100WCSystemInfoIntermediate && echo) {
			return nil, fmt.Errorf("tesla WC system-information response sequence is invalid")
		}
		if x.Kind == TeslaFC100WCSystemInfoIntermediate {
			echo = true
		} else {
			term = true
		}
		out = append(out, x)
	}
	if !term {
		return nil, fmt.Errorf("tesla WC system-information response has no terminal")
	}
	return out, nil
}
func decodeTeslaFC100WCSystemInfoSequence(p [][]byte) ([]QualifiedFunctionResult, error) {
	r, e := DecodeTeslaFC100WCSystemInfoReplaySequence(p)
	if e != nil {
		return nil, e
	}
	out := make([]QualifiedFunctionResult, len(r))
	for i, x := range r {
		out[i] = QualifiedFunctionResult{Replay: &QualifiedFunctionReplay{Kind: string(x.Kind), PayloadLength: x.SnapshotLength, PayloadDigest: x.SnapshotDigest}}
	}
	return out, nil
}
