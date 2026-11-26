package write

import (
	"math/big"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// The reasons a command is answered before anything is sent.
const (
	ReasonStopped        = "the writer is not running"
	ReasonGuard          = "the guard differs from the route of the provider"
	ReasonNotWritable    = "the t25 setter is not qualified for this build"
	ReasonNotReady       = "the endpoint lifecycle is not ready"
	ReasonCancelled      = "the context ended before the send"
	ReasonUnit           = "the value is not in amperes"
	ReasonNegative       = "the value is negative"
	ReasonNotWhole       = "the value is not a whole number of amperes"
	ReasonOutOfRange     = "the value does not fit the t25 current field"
	ReasonBelowFloor     = "the value is below the 6 A the control pilot signals"
	ReasonAboveConfig    = "the value is above the configured current of the latest t28"
	ReasonNoConfigured   = "the latest t28 gives no usable configured current"
	ReasonPrerequisite   = "the prerequisite t27 and t28 read did not give a t28"
	ReasonArguments      = "the request does not carry exactly one amperes value or the release"
	ReasonReleaseTrue    = "the release argument is true; only false is a release"
	ReasonForeignRoute   = "the request names a route or connector with no registered owner"
	ReasonSetRefused     = "the native setter refused the request before sending"
	ReasonNoAcknowledge  = "the t25 request was possibly transmitted and no t26 acknowledged it"
	ReasonNoReadback     = "the t26 acknowledged and no t28 was read back"
	ReasonRejected       = "the station rejected the t25 request"
	ReasonRenewalStopped = "the limit is not established to be in force"
)

// wholeAmps reads an exact whole number of amperes. A value with a fraction,
// whatever the exponent, is not truncated or rounded.
func wholeAmps(q semreg.Quantity) (uint32, string) {
	if q.Unit != UnitAmpere {
		return 0, ReasonUnit
	}
	n, ok := new(big.Int).SetString(q.Number.Coefficient, 10)
	if !ok {
		return 0, ReasonNotWhole
	}
	if n.Sign() < 0 {
		return 0, ReasonNegative
	}
	exp := int64(q.Number.Exponent10)
	switch {
	case n.Sign() == 0:
		return 0, ""
	case exp > 10:
		return 0, ReasonOutOfRange
	case exp >= 0:
		n.Mul(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil))
	case -exp > int64(len(n.String())):
		return 0, ReasonNotWhole
	default:
		quo, rem := new(big.Int).QuoRem(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(-exp), nil), new(big.Int))
		if rem.Sign() != 0 {
			return 0, ReasonNotWhole
		}
		n = quo
	}
	if !n.IsUint64() || n.Uint64() > 1<<32-1 {
		return 0, ReasonOutOfRange
	}
	return uint32(n.Uint64()), ""
}
