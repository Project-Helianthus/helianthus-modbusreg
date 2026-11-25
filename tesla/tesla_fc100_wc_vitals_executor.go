package tesla

import (
	"context"
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// FC100WCVitalsExecutor invokes the single qualified WC vitals operation
// through an already-correlated RTU exchange boundary. It owns neither a
// serial endpoint nor a general operation admission path.
type FC100WCVitalsExecutor struct {
	profile   HSCProfile
	exchanger modbusreg.QualifiedFunctionExchanger
}

// NewFC100WCVitalsExecutor validates the exact version-qualified WC
// vitals operation without invoking an exchange. An unqualified profile is
// rejected before the injected exchange boundary can be called.
func NewFC100WCVitalsExecutor(
	profile HSCProfile,
	exchanger modbusreg.QualifiedFunctionExchanger,
) (*FC100WCVitalsExecutor, error) {
	if exchanger == nil || !profile.OperationAdmissionFor(TEDAPIOperationWCVitalsV1).OutboundAllowed {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	return &FC100WCVitalsExecutor{profile: profile, exchanger: exchanger}, nil
}

// Execute invokes the qualified exchange without a firmware identity and
// returns echo and terminal diagnostics without typed values or raw bodies.
func (executor *FC100WCVitalsExecutor) Execute(ctx context.Context) ([]FC100WCVitalsReplay, error) {
	return executor.execute(ctx, DecodeTeslaFC100WCVitalsReplay)
}

// ExecuteForFirmware invokes the same qualified exchange and retains the native
// terminal. The supplied firmware selects decoding only; it grants no admission.
func (executor *FC100WCVitalsExecutor) ExecuteForFirmware(ctx context.Context, firmware string) ([]FC100WCVitalsReplay, error) {
	return executor.execute(ctx, func(payload []byte) (FC100WCVitalsReplay, error) {
		return DecodeWCVitalsReplayForFirmware(payload, firmware)
	})
}

func (executor *FC100WCVitalsExecutor) execute(ctx context.Context, decode func([]byte) (FC100WCVitalsReplay, error)) ([]FC100WCVitalsReplay, error) {
	if executor == nil || ctx == nil || executor.exchanger == nil {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	request, policy, err := executor.profile.EncodeQualifiedFunction(TEDAPIOperationWCVitalsV1)
	if err != nil {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	responses, err := executor.exchanger.Exchange(ctx, executor.profile.Node(), request, policy)
	if err != nil {
		return nil, err
	}
	if len(responses) == 0 || len(responses) > 8 {
		return nil, fmt.Errorf("tesla WC vitals response count is invalid")
	}
	replays := make([]FC100WCVitalsReplay, 0, len(responses))
	for _, response := range responses {
		replay, err := decode(response.Payload())
		if err != nil {
			return nil, err
		}
		replays = append(replays, replay)
	}
	return replays, nil
}
