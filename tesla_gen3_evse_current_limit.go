package modbusreg

import (
	"fmt"
	"math"
)

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
	request, requestErr := teslaGen3ExactFC100RequestBody(TeslaFC100OperationWCConfigureSettings, s.RequestPayload)
	requestAmps, requestDecodeErr := decodeTeslaGen3PersistentCurrentLimitBody(request)
	var terminalAmps uint32
	var terminalDecodeErr error
	if len(r) == 1 {
		terminalAmps, terminalDecodeErr = decodeTeslaGen3PersistentCurrentLimitBody(r[0].Body)
	}
	if s.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 ||
		requestErr != nil || requestDecodeErr != nil || terminalDecodeErr != nil ||
		e != nil || len(r) != 1 || r[0].Kind != TeslaFC100OperationTerminal ||
		s.MaxOutputCurrentAmps != requestAmps || requestAmps != terminalAmps {
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
	configuredCurrentAmps                    uint32
	setRequestPayload                        []byte
	ackPayload                               []byte
	readbackRequestPayload                   []byte
	readbackTerminalPayload                  []byte
}

func NewTeslaGen3ProvisionalCurrentLimit(s TeslaGen3ProvisionalCurrentLimitSpec) (TeslaGen3ProvisionalCurrentLimit, error) {
	a, e := DecodeTeslaFC100OperationSequence(s.OperationVersion, TeslaFC100OperationWCSetProvisional, s.SetRequestPayload, [][]byte{s.AckPayload})
	b, f := DecodeTeslaFC100OperationSequence(s.OperationVersion, TeslaFC100OperationWCGetProvisional, s.ReadbackRequestPayload, [][]byte{s.ReadbackTerminalPayload})
	setRequest, setRequestErr := teslaGen3ExactFC100RequestBody(TeslaFC100OperationWCSetProvisional, s.SetRequestPayload)
	setValues, setDecodeErr := decodeTeslaGen3ProvisionalCurrentLimitBody(setRequest, false)
	readbackRequest, readbackRequestErr := teslaGen3ExactFC100RequestBody(TeslaFC100OperationWCGetProvisional, s.ReadbackRequestPayload)
	var readbackValues teslaGen3ProvisionalCurrentLimitWire
	var readbackDecodeErr error
	if len(b) == 1 {
		readbackValues, readbackDecodeErr = decodeTeslaGen3ProvisionalCurrentLimitBody(b[0].Body, true)
	}
	if s.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 ||
		setRequestErr != nil || setDecodeErr != nil || readbackRequestErr != nil || len(readbackRequest) != 0 ||
		readbackDecodeErr != nil ||
		e != nil || f != nil || len(a) != 1 || len(b) != 1 ||
		a[0].Kind != TeslaFC100OperationTerminal || b[0].Kind != TeslaFC100OperationTerminal || len(a[0].Body) != 0 ||
		s.LimitCurrentMaxAmps != setValues.amps || s.LimitTimeoutSeconds != setValues.timeout || s.InhibitCharging != setValues.inhibit ||
		setValues.amps != readbackValues.amps || setValues.timeout != readbackValues.timeout || setValues.inhibit != readbackValues.inhibit {
		return TeslaGen3ProvisionalCurrentLimit{}, fmt.Errorf("Gen3 provisional current limit context is invalid")
	}
	return TeslaGen3ProvisionalCurrentLimit{
		operationVersion:        s.OperationVersion,
		limitCurrentMaxAmps:     s.LimitCurrentMaxAmps,
		limitTimeoutSeconds:     s.LimitTimeoutSeconds,
		inhibitCharging:         s.InhibitCharging,
		configuredCurrentAmps:   readbackValues.configured,
		setRequestPayload:       append([]byte(nil), s.SetRequestPayload...),
		ackPayload:              append([]byte(nil), s.AckPayload...),
		readbackRequestPayload:  append([]byte(nil), s.ReadbackRequestPayload...),
		readbackTerminalPayload: append([]byte(nil), s.ReadbackTerminalPayload...),
	}, nil
}

func teslaGen3ExactFC100RequestBody(operation TeslaFC100Operation, payload []byte) ([]byte, error) {
	spec, ok := teslaFC100OperationSpecs[operation]
	if !ok {
		return nil, fmt.Errorf("Gen3 FC100 operation is unsupported")
	}
	envelope, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload)
	if err != nil {
		return nil, err
	}
	family, err := decodeExactLengthDelimitedField(envelope.Payload(), spec.family)
	if err != nil {
		return nil, err
	}
	return decodeExactLengthDelimitedField(family, spec.requestTag)
}

func (v TeslaGen3ProvisionalCurrentLimit) OperationVersion() string    { return v.operationVersion }
func (v TeslaGen3ProvisionalCurrentLimit) LimitCurrentMaxAmps() uint32 { return v.limitCurrentMaxAmps }
func (v TeslaGen3ProvisionalCurrentLimit) LimitTimeoutSeconds() uint32 { return v.limitTimeoutSeconds }
func (v TeslaGen3ProvisionalCurrentLimit) InhibitCharging() bool       { return v.inhibitCharging }
func (v TeslaGen3ProvisionalCurrentLimit) ConfiguredCurrentAmps() uint32 {
	return v.configuredCurrentAmps
}
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

type teslaGen3CurrentLimitWireField struct {
	varint uint64
	bytes  []byte
}

func decodeTeslaGen3CurrentLimitFields(body []byte, known map[uint64]uint8) (map[uint64]teslaGen3CurrentLimitWireField, error) {
	fields := make(map[uint64]teslaGen3CurrentLimitWireField, len(known))
	for offset := 0; offset < len(body); {
		key, keyWidth, err := decodeTeslaFC100Varint(body[offset:])
		if err != nil || key>>3 == 0 || key&7 > 5 {
			return nil, fmt.Errorf("Gen3 current-limit protobuf field is invalid")
		}
		number, wireType := key>>3, uint8(key&7)
		expectedWireType, isKnown := known[number]
		if !isKnown {
			_, consumed, err := decodeTeslaFC100RequestField(body[offset:])
			if err != nil {
				return nil, fmt.Errorf("Gen3 current-limit unknown protobuf field is malformed")
			}
			offset += consumed
			continue
		}
		if wireType != expectedWireType {
			return nil, fmt.Errorf("Gen3 current-limit protobuf field has wrong wire type")
		}
		if _, exists := fields[number]; exists {
			return nil, fmt.Errorf("Gen3 current-limit protobuf field is duplicated")
		}
		offset += keyWidth
		field := teslaGen3CurrentLimitWireField{}
		switch wireType {
		case 0:
			value, width, err := decodeTeslaFC100Varint(body[offset:])
			if err != nil {
				return nil, fmt.Errorf("Gen3 current-limit protobuf value is malformed")
			}
			field.varint = value
			offset += width
		case 2:
			length, width, err := decodeTeslaFC100Varint(body[offset:])
			if err != nil || length > uint64(len(body)-offset-width) {
				return nil, fmt.Errorf("Gen3 current-limit protobuf body is malformed")
			}
			offset += width
			field.bytes = append([]byte(nil), body[offset:offset+int(length)]...)
			offset += int(length)
		default:
			return nil, fmt.Errorf("Gen3 current-limit protobuf wire type is unsupported")
		}
		fields[number] = field
	}
	return fields, nil
}

func requiredTeslaGen3CurrentLimitField(fields map[uint64]teslaGen3CurrentLimitWireField, number uint64) (teslaGen3CurrentLimitWireField, error) {
	field, ok := fields[number]
	if !ok {
		return teslaGen3CurrentLimitWireField{}, fmt.Errorf("Gen3 current-limit protobuf field is absent")
	}
	return field, nil
}

func decodeTeslaGen3PersistentCurrentLimitBody(body []byte) (uint32, error) {
	outer, err := decodeTeslaGen3CurrentLimitFields(body, map[uint64]uint8{1: 2})
	if err != nil {
		return 0, err
	}
	settings, err := requiredTeslaGen3CurrentLimitField(outer, 1)
	if err != nil {
		return 0, err
	}
	inner, err := decodeTeslaGen3CurrentLimitFields(settings.bytes, map[uint64]uint8{1: 0})
	if err != nil {
		return 0, err
	}
	amps, err := requiredTeslaGen3CurrentLimitField(inner, 1)
	if err != nil || amps.varint > math.MaxInt32 {
		return 0, fmt.Errorf("Gen3 persistent current limit is invalid")
	}
	return uint32(amps.varint), nil
}

type teslaGen3ProvisionalCurrentLimitWire struct {
	amps, timeout, configured uint32
	inhibit                   bool
}

func decodeTeslaGen3ProvisionalCurrentLimitBody(body []byte, requireConfigured bool) (teslaGen3ProvisionalCurrentLimitWire, error) {
	known := map[uint64]uint8{1: 2}
	if requireConfigured {
		known[2] = 0
	}
	outer, err := decodeTeslaGen3CurrentLimitFields(body, known)
	if err != nil {
		return teslaGen3ProvisionalCurrentLimitWire{}, err
	}
	provisional, err := requiredTeslaGen3CurrentLimitField(outer, 1)
	if err != nil {
		return teslaGen3ProvisionalCurrentLimitWire{}, err
	}
	inner, err := decodeTeslaGen3CurrentLimitFields(provisional.bytes, map[uint64]uint8{1: 0, 2: 0, 3: 0})
	if err != nil {
		return teslaGen3ProvisionalCurrentLimitWire{}, err
	}
	amps, ampsErr := requiredTeslaGen3CurrentLimitField(inner, 1)
	timeout, timeoutErr := requiredTeslaGen3CurrentLimitField(inner, 2)
	inhibit, inhibitErr := requiredTeslaGen3CurrentLimitField(inner, 3)
	if ampsErr != nil || timeoutErr != nil || inhibitErr != nil || amps.varint > math.MaxUint32 || timeout.varint > math.MaxUint32 || inhibit.varint > 1 {
		return teslaGen3ProvisionalCurrentLimitWire{}, fmt.Errorf("Gen3 provisional current limit is invalid")
	}
	result := teslaGen3ProvisionalCurrentLimitWire{amps: uint32(amps.varint), timeout: uint32(timeout.varint), inhibit: inhibit.varint == 1}
	if requireConfigured {
		configured, err := requiredTeslaGen3CurrentLimitField(outer, 2)
		if err != nil || configured.varint > math.MaxUint32 {
			return teslaGen3ProvisionalCurrentLimitWire{}, fmt.Errorf("Gen3 configured current limit is invalid")
		}
		result.configured = uint32(configured.varint)
	}
	return result, nil
}
