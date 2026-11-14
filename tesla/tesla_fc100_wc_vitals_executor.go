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

// Execute constructs the fixed qualified FC100 request, invokes one injected
// already-correlated RTU exchange, and returns only bounded redacted replays.
// It does not open a serial endpoint, expose response bytes, or select any
// fallback operation.
func (executor *FC100WCVitalsExecutor) Execute(ctx context.Context) ([]FC100WCVitalsReplay, error) {
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
		replay, err := DecodeTeslaFC100WCVitalsReplay(response.Payload())
		if err != nil {
			return nil, err
		}
		replays = append(replays, replay)
	}
	return replays, nil
}
