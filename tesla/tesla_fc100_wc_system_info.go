package tesla

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// TEDAPIOperationWCSystemInfoV1 identifies the explicitly qualified FC100 WC
// system-information operation; see
// https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/protocols/tesla/tedapi.md.
const TEDAPIOperationWCSystemInfoV1 = "tesla.hsc.fc100.wc_system_info.v1"

var teslaFC100WCSystemInfoRequestPDU = []byte{4, 0x32, 2, 0x4a, 0}

// FC100WCSystemInfoReplayKind distinguishes an echoed FC100 intermediate
// from the opaque terminal system-information body.
type FC100WCSystemInfoReplayKind string

// Replay kinds: Intermediate is the exact echo of the request message and carries
// no snapshot; Terminal carries the length and SHA-256 hex digest of the opaque body.
const (
	FC100WCSystemInfoIntermediate FC100WCSystemInfoReplayKind = "intermediate"
	FC100WCSystemInfoTerminal     FC100WCSystemInfoReplayKind = "terminal"
)

// FC100WCSystemInfoReplay retains only redacted terminal metadata: it has no
// typed fields and never contains body bytes.
type FC100WCSystemInfoReplay struct {
	Kind           FC100WCSystemInfoReplayKind
	SnapshotLength int
	SnapshotDigest string
}

// DecodeTeslaFC100WCSystemInfoReplay accepts one FC100 response that is either
// the exact request echo or a terminal family-6/tag-10 body; anything else is an
// error.
func DecodeTeslaFC100WCSystemInfoReplay(payload []byte) (FC100WCSystemInfoReplay, error) {
	r, e := DecodeTeslaHSCResponse(teslaHSCFunction100, payload)
	if e != nil {
		return FC100WCSystemInfoReplay{}, e
	}
	m := r.Payload()
	if bytes.Equal(m, teslaFC100WCSystemInfoRequestPDU[1:]) {
		return FC100WCSystemInfoReplay{Kind: FC100WCSystemInfoIntermediate}, nil
	}
	w, e := decodeExactLengthDelimitedField(m, 6)
	if e != nil {
		return FC100WCSystemInfoReplay{}, e
	}
	b, e := decodeExactLengthDelimitedField(w, 10)
	if e != nil {
		return FC100WCSystemInfoReplay{}, e
	}
	d := sha256.Sum256(b)
	return FC100WCSystemInfoReplay{Kind: FC100WCSystemInfoTerminal, SnapshotLength: len(b), SnapshotDigest: hex.EncodeToString(d[:])}, nil
}

// DecodeTeslaFC100WCSystemInfoReplaySequence accepts one or two responses: at
// most one echo, followed by exactly one terminal. Any other order or count is
// rejected.
func DecodeTeslaFC100WCSystemInfoReplaySequence(p [][]byte) ([]FC100WCSystemInfoReplay, error) {
	if len(p) == 0 || len(p) > 2 {
		return nil, fmt.Errorf("tesla WC system-information response count is invalid")
	}
	out := make([]FC100WCSystemInfoReplay, 0, len(p))
	echo, term := false, false
	for _, v := range p {
		x, e := DecodeTeslaFC100WCSystemInfoReplay(v)
		if e != nil {
			return nil, e
		}
		if term || (x.Kind == FC100WCSystemInfoIntermediate && echo) {
			return nil, fmt.Errorf("tesla WC system-information response sequence is invalid")
		}
		if x.Kind == FC100WCSystemInfoIntermediate {
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
func decodeTeslaFC100WCSystemInfoSequence(p [][]byte) ([]modbusreg.QualifiedFunctionResult, error) {
	r, e := DecodeTeslaFC100WCSystemInfoReplaySequence(p)
	if e != nil {
		return nil, e
	}
	out := make([]modbusreg.QualifiedFunctionResult, len(r))
	for i, x := range r {
		out[i] = modbusreg.QualifiedFunctionResult{Replay: &modbusreg.QualifiedFunctionReplay{Kind: string(x.Kind), PayloadLength: x.SnapshotLength, PayloadDigest: x.SnapshotDigest}}
	}
	return out, nil
}
