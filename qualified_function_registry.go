package modbusreg

import (
	"context"
	"errors"
	"fmt"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// ErrQualifiedFunctionNoSend reports a rejected profile or operation before
// any exchange has been invoked.
var ErrQualifiedFunctionNoSend = errors.New("qualified function operation is not admitted")

// QualifiedFunctionSelector identifies one profile-scoped operation. It
// deliberately contains no caller-provided function-code or raw payload.
type QualifiedFunctionSelector struct {
	Endpoint      string
	UnitID        byte
	VendorProfile string
	Operation     string
}

// QualifiedFunctionReplay is a generic redacted result available when a
// selected codec intentionally withholds response bytes.
type QualifiedFunctionReplay struct {
	Kind          string
	PayloadLength int
	PayloadDigest string
}

// QualifiedFunctionResult retains only the representation selected by the
// codec. The registry does not infer fields, units, or operation semantics.
type QualifiedFunctionResult struct {
	Payload []byte
	Replay  *QualifiedFunctionReplay
}

// QualifiedFunctionCodec owns construction and normal-response decoding for
// one selected vendor profile. It is never selected from a function code.
type QualifiedFunctionCodec interface {
	EncodeQualifiedFunction(string) (modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, error)
	DecodeQualifiedFunction(string, modbus.PrivateFunctionCode, []byte) (QualifiedFunctionResult, error)
}

// QualifiedFunctionProfile binds one codec to one endpoint and unit identity.
type QualifiedFunctionProfile struct {
	Endpoint      string
	UnitID        byte
	VendorProfile string
	Codec         QualifiedFunctionCodec
}

// QualifiedFunctionExchanger is the generic raw RTU exchange boundary. It
// owns framing, correlation, deadlines, retry, and serialization; it owns no
// vendor codec or registry selection.
type QualifiedFunctionExchanger interface {
	Exchange(context.Context, byte, modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error)
}

// QualifiedFunctionRegistry is an immutable endpoint-scoped codec selector.
type QualifiedFunctionRegistry struct {
	profiles []QualifiedFunctionProfile
}

// NewQualifiedFunctionRegistry validates a finite set of endpoint-scoped
// profiles. More than one matching endpoint/unit is retained as an explicit
// ambiguity and fails closed at dispatch time.
func NewQualifiedFunctionRegistry(profiles []QualifiedFunctionProfile) (*QualifiedFunctionRegistry, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("qualified function registry has no profiles")
	}
	copyProfiles := make([]QualifiedFunctionProfile, len(profiles))
	for index, profile := range profiles {
		if !validIdentity(profile.Endpoint) || profile.UnitID == 0 || profile.UnitID > 247 ||
			!validIdentity(profile.VendorProfile) || profile.Codec == nil {
			return nil, fmt.Errorf("qualified function profile is invalid")
		}
		copyProfiles[index] = profile
	}
	return &QualifiedFunctionRegistry{profiles: copyProfiles}, nil
}

// Dispatch selects exactly one endpoint/unit profile, validates its explicit
// profile and operation identity, then exchanges only the codec-constructed
// request. Every selection or codec failure is no-send.
func (registry *QualifiedFunctionRegistry) Dispatch(
	ctx context.Context,
	exchanger QualifiedFunctionExchanger,
	selector QualifiedFunctionSelector,
) ([]QualifiedFunctionResult, error) {
	if registry == nil || ctx == nil || exchanger == nil ||
		!validIdentity(selector.Endpoint) || selector.UnitID == 0 || selector.UnitID > 247 ||
		!validIdentity(selector.VendorProfile) || !validIdentity(selector.Operation) {
		return nil, ErrQualifiedFunctionNoSend
	}
	var selected QualifiedFunctionProfile
	matches := 0
	for _, profile := range registry.profiles {
		if profile.Endpoint == selector.Endpoint && profile.UnitID == selector.UnitID {
			selected = profile
			matches++
		}
	}
	if matches != 1 || selected.VendorProfile != selector.VendorProfile {
		return nil, ErrQualifiedFunctionNoSend
	}
	request, policy, err := selected.Codec.EncodeQualifiedFunction(selector.Operation)
	if err != nil {
		return nil, ErrQualifiedFunctionNoSend
	}
	responses, err := exchanger.Exchange(ctx, selector.UnitID, request, policy)
	if err != nil {
		return nil, err
	}
	results := make([]QualifiedFunctionResult, 0, len(responses))
	for _, response := range responses {
		result, err := selected.Codec.DecodeQualifiedFunction(
			selector.Operation,
			request.FunctionCode(),
			response.Payload(),
		)
		if err != nil {
			return nil, err
		}
		result.Payload = append([]byte(nil), result.Payload...)
		if result.Replay != nil {
			replay := *result.Replay
			result.Replay = &replay
		}
		results = append(results, result)
	}
	return results, nil
}
