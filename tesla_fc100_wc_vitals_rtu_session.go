package modbusreg

import (
	"context"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// TeslaFC100WCVitalsRTUSession is the already-configured generic RTU session
// boundary used by the WC vitals adapter. Its Exchange result has already
// passed generic framing, CRC, unit, function, and exception correlation.
type TeslaFC100WCVitalsRTUSession interface {
	Exchange(context.Context, byte, modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy) ([]modbus.RTUPrivateFunctionResponseADU, error)
}

// TeslaFC100WCVitalsRTUSessionAdapter connects one generic RTU session to the
// explicit qualified WC vitals executor. Its public execution result never
// exposes request or response bytes.
type TeslaFC100WCVitalsRTUSessionAdapter struct {
	executor *TeslaFC100WCVitalsExecutor
}

// NewTeslaFC100WCVitalsRTUSessionAdapter validates the selected profile and
// binds it to an injected RTU session without opening or configuring a port.
func NewTeslaFC100WCVitalsRTUSessionAdapter(
	profile TeslaHSCProfile,
	session TeslaFC100WCVitalsRTUSession,
) (*TeslaFC100WCVitalsRTUSessionAdapter, error) {
	if session == nil {
		return nil, ErrQualifiedFunctionNoSend
	}
	executor, err := NewTeslaFC100WCVitalsExecutor(profile, teslaFC100WCVitalsSessionExchanger{session: session})
	if err != nil {
		return nil, err
	}
	return &TeslaFC100WCVitalsRTUSessionAdapter{executor: executor}, nil
}

// Execute invokes the fixed qualified request through one generic correlated
// RTU exchange and returns only existing bounded redacted replays.
func (adapter *TeslaFC100WCVitalsRTUSessionAdapter) Execute(ctx context.Context) ([]TeslaFC100WCVitalsReplay, error) {
	if adapter == nil || adapter.executor == nil {
		return nil, ErrQualifiedFunctionNoSend
	}
	return adapter.executor.Execute(ctx)
}

type teslaFC100WCVitalsSessionExchanger struct {
	session TeslaFC100WCVitalsRTUSession
}

func (exchanger teslaFC100WCVitalsSessionExchanger) Exchange(
	ctx context.Context,
	unitID byte,
	request modbus.PrivateFunctionRequest,
	policy modbus.PrivateFunctionResponsePolicy,
) ([]modbus.RTUPrivateFunctionResponseADU, error) {
	return exchanger.session.Exchange(ctx, unitID, request, policy)
}
