package modbusreg

import "fmt"

const TeslaGen3CurrentLimitOperationVersion24443 = "wc3_24_44_3"
const (
	teslaGen3MinCurrentAmps uint32 = 6
	teslaGen3MaxTimeoutS    uint32 = 86399
)

type TeslaGen3PersistentCurrentLimitSpec struct {
	OperationVersion                string
	MaxOutputCurrentAmps            uint32
	RequestPayload, TerminalPayload []byte
}
type TeslaGen3PersistentCurrentLimit struct {
	operationVersion                string
	maxOutputCurrentAmps            uint32
	requestPayload, terminalPayload []byte
}

func NewTeslaGen3PersistentCurrentLimit(s TeslaGen3PersistentCurrentLimitSpec) (TeslaGen3PersistentCurrentLimit, error) {
	r, e := DecodeTeslaFC100OperationSequence(s.OperationVersion, TeslaFC100OperationWCConfigureSettings, s.RequestPayload, [][]byte{s.TerminalPayload})
	if s.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 ||
		!teslaGen3ExactFC100Request(TeslaFC100OperationWCConfigureSettings, s.RequestPayload) ||
		e != nil || len(r) != 1 || r[0].Kind != TeslaFC100OperationTerminal {
		return TeslaGen3PersistentCurrentLimit{}, fmt.Errorf("Gen3 persistent current limit context is invalid")
	}
	return TeslaGen3PersistentCurrentLimit{s.OperationVersion, s.MaxOutputCurrentAmps, append([]byte(nil), s.RequestPayload...), append([]byte(nil), s.TerminalPayload...)}, nil
}
func (v TeslaGen3PersistentCurrentLimit) OperationVersion() string     { return v.operationVersion }
func (v TeslaGen3PersistentCurrentLimit) MaxOutputCurrentAmps() uint32 { return v.maxOutputCurrentAmps }
func (v TeslaGen3PersistentCurrentLimit) RequestPayload() []byte {
	return append([]byte(nil), v.requestPayload...)
}
func (v TeslaGen3PersistentCurrentLimit) TerminalPayload() []byte {
	return append([]byte(nil), v.terminalPayload...)
}

type TeslaGen3ProvisionalCurrentLimitSpec struct {
	OperationVersion                                                               string
	LimitCurrentMaxAmps, LimitTimeoutSeconds                                       uint32
	InhibitCharging                                                                bool
	SetRequestPayload, AckPayload, ReadbackRequestPayload, ReadbackTerminalPayload []byte
}
type TeslaGen3ProvisionalCurrentLimit struct {
	operationVersion                         string
	limitCurrentMaxAmps, limitTimeoutSeconds uint32
	inhibitCharging                          bool
	setRequestPayload                        []byte
	ackPayload                               []byte
	readbackRequestPayload                   []byte
	readbackTerminalPayload                  []byte
}

func NewTeslaGen3ProvisionalCurrentLimit(s TeslaGen3ProvisionalCurrentLimitSpec) (TeslaGen3ProvisionalCurrentLimit, error) {
	a, e := DecodeTeslaFC100OperationSequence(s.OperationVersion, TeslaFC100OperationWCSetProvisional, s.SetRequestPayload, [][]byte{s.AckPayload})
	b, f := DecodeTeslaFC100OperationSequence(s.OperationVersion, TeslaFC100OperationWCGetProvisional, s.ReadbackRequestPayload, [][]byte{s.ReadbackTerminalPayload})
	if s.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 ||
		!teslaGen3ExactFC100Request(TeslaFC100OperationWCSetProvisional, s.SetRequestPayload) ||
		!teslaGen3ExactFC100Request(TeslaFC100OperationWCGetProvisional, s.ReadbackRequestPayload) ||
		e != nil || f != nil || len(a) != 1 || len(b) != 1 ||
		a[0].Kind != TeslaFC100OperationTerminal || b[0].Kind != TeslaFC100OperationTerminal {
		return TeslaGen3ProvisionalCurrentLimit{}, fmt.Errorf("Gen3 provisional current limit context is invalid")
	}
	return TeslaGen3ProvisionalCurrentLimit{
		operationVersion:        s.OperationVersion,
		limitCurrentMaxAmps:     s.LimitCurrentMaxAmps,
		limitTimeoutSeconds:     s.LimitTimeoutSeconds,
		inhibitCharging:         s.InhibitCharging,
		setRequestPayload:       append([]byte(nil), s.SetRequestPayload...),
		ackPayload:              append([]byte(nil), s.AckPayload...),
		readbackRequestPayload:  append([]byte(nil), s.ReadbackRequestPayload...),
		readbackTerminalPayload: append([]byte(nil), s.ReadbackTerminalPayload...),
	}, nil
}

func teslaGen3ExactFC100Request(operation TeslaFC100Operation, payload []byte) bool {
	spec, ok := teslaFC100OperationSpecs[operation]
	if !ok {
		return false
	}
	envelope, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload)
	if err != nil {
		return false
	}
	family, err := decodeExactLengthDelimitedField(envelope.Payload(), spec.family)
	if err != nil {
		return false
	}
	_, err = decodeExactLengthDelimitedField(family, spec.requestTag)
	return err == nil
}

func (v TeslaGen3ProvisionalCurrentLimit) OperationVersion() string    { return v.operationVersion }
func (v TeslaGen3ProvisionalCurrentLimit) LimitCurrentMaxAmps() uint32 { return v.limitCurrentMaxAmps }
func (v TeslaGen3ProvisionalCurrentLimit) LimitTimeoutSeconds() uint32 { return v.limitTimeoutSeconds }
func (v TeslaGen3ProvisionalCurrentLimit) InhibitCharging() bool       { return v.inhibitCharging }
func (v TeslaGen3ProvisionalCurrentLimit) SetRequestPayload() []byte {
	return append([]byte(nil), v.setRequestPayload...)
}
func (v TeslaGen3ProvisionalCurrentLimit) AckPayload() []byte {
	return append([]byte(nil), v.ackPayload...)
}
func (v TeslaGen3ProvisionalCurrentLimit) ReadbackRequestPayload() []byte {
	return append([]byte(nil), v.readbackRequestPayload...)
}
func (v TeslaGen3ProvisionalCurrentLimit) ReadbackTerminalPayload() []byte {
	return append([]byte(nil), v.readbackTerminalPayload...)
}

type TeslaGen3InteroperableCurrentLimit struct{ maxAmps, timeoutSeconds uint32 }

func NewTeslaGen3InteroperableCurrentLimit(a, t uint32) (TeslaGen3InteroperableCurrentLimit, error) {
	if a < teslaGen3MinCurrentAmps || t == 0 || t > teslaGen3MaxTimeoutS {
		return TeslaGen3InteroperableCurrentLimit{}, fmt.Errorf("Gen3 interoperable current limit is out of bounds")
	}
	return TeslaGen3InteroperableCurrentLimit{a, t}, nil
}
func (v TeslaGen3InteroperableCurrentLimit) MaxAmps() uint32        { return v.maxAmps }
func (v TeslaGen3InteroperableCurrentLimit) TimeoutSeconds() uint32 { return v.timeoutSeconds }
