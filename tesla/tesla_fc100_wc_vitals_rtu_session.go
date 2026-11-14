package tesla

import (
	"context"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// FC100WCVitalsRTUSession is the already-configured generic RTU session
// boundary used by the WC vitals adapter. Its Exchange result has already
// passed generic framing, CRC, unit, function, and exception correlation.
type FC100WCVitalsRTUSession interface {
	Exchange(context.Context, byte, modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error)
}

// FC100WCVitalsRTUSessionAdapter connects one generic RTU session to the
// explicit qualified WC vitals executor. Its public execution result never
// exposes request or response bytes.
type FC100WCVitalsRTUSessionAdapter struct {
	executor *FC100WCVitalsExecutor
}

// NewFC100WCVitalsRTUSessionAdapter validates the selected profile and
// binds it to an injected RTU session without opening or configuring a port.
func NewFC100WCVitalsRTUSessionAdapter(
	profile HSCProfile,
	session FC100WCVitalsRTUSession,
) (*FC100WCVitalsRTUSessionAdapter, error) {
	if session == nil {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	executor, err := NewFC100WCVitalsExecutor(profile, teslaFC100WCVitalsSessionExchanger{session: session})
	if err != nil {
		return nil, err
	}
	return &FC100WCVitalsRTUSessionAdapter{executor: executor}, nil
}

// Execute invokes the fixed qualified request through one generic correlated
// RTU exchange and returns only existing bounded redacted replays.
func (adapter *FC100WCVitalsRTUSessionAdapter) Execute(ctx context.Context) ([]FC100WCVitalsReplay, error) {
	if adapter == nil || adapter.executor == nil {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	return adapter.executor.Execute(ctx)
}

type teslaFC100WCVitalsSessionExchanger struct {
	session FC100WCVitalsRTUSession
}

func (exchanger teslaFC100WCVitalsSessionExchanger) Exchange(
	ctx context.Context,
	unitID byte,
	request modbus.PrivateFunctionRequest,
	policy modbus.PrivateFunctionResponsePolicy,
) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	return exchanger.session.Exchange(ctx, unitID, request, policy)
}
