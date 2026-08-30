package modbusreg

import "fmt"

// TeslaGen3CurrentLimitOperationVersion24443 identifies the only admitted
// Gen3 current-limit operation version.
const TeslaGen3CurrentLimitOperationVersion24443 = "wc3_24_44_3"

const (
	teslaGen3MinCurrentAmps uint32 = 6
	teslaGen3MaxTimeoutS    uint32 = 86399
)

type TeslaGen3PersistentCurrentLimitSpec struct {
	OperationVersion     string
	MaxOutputCurrentAmps uint32
	Raw                  []byte
}

type TeslaGen3PersistentCurrentLimit struct {
	operationVersion     string
	maxOutputCurrentAmps uint32
	raw                  []byte
}

func NewTeslaGen3PersistentCurrentLimit(spec TeslaGen3PersistentCurrentLimitSpec) (TeslaGen3PersistentCurrentLimit, error) {
	if spec.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 || len(spec.Raw) == 0 || len(spec.Raw) > 252 {
		return TeslaGen3PersistentCurrentLimit{}, fmt.Errorf("Gen3 persistent current limit is not qualified")
	}
	return TeslaGen3PersistentCurrentLimit{operationVersion: spec.OperationVersion, maxOutputCurrentAmps: spec.MaxOutputCurrentAmps, raw: append([]byte(nil), spec.Raw...)}, nil
}
func (v TeslaGen3PersistentCurrentLimit) OperationVersion() string     { return v.operationVersion }
func (v TeslaGen3PersistentCurrentLimit) MaxOutputCurrentAmps() uint32 { return v.maxOutputCurrentAmps }
func (v TeslaGen3PersistentCurrentLimit) Raw() []byte                  { return append([]byte(nil), v.raw...) }

type TeslaGen3ProvisionalCurrentLimitSpec struct {
	OperationVersion                         string
	LimitCurrentMaxAmps, LimitTimeoutSeconds uint32
	InhibitCharging                          bool
	ReadbackRaw                              []byte
}
type TeslaGen3ProvisionalCurrentLimit struct {
	operationVersion                         string
	limitCurrentMaxAmps, limitTimeoutSeconds uint32
	inhibitCharging                          bool
	readbackRaw                              []byte
}

func NewTeslaGen3ProvisionalCurrentLimit(spec TeslaGen3ProvisionalCurrentLimitSpec) (TeslaGen3ProvisionalCurrentLimit, error) {
	if spec.OperationVersion != TeslaGen3CurrentLimitOperationVersion24443 || len(spec.ReadbackRaw) == 0 || len(spec.ReadbackRaw) > 252 {
		return TeslaGen3ProvisionalCurrentLimit{}, fmt.Errorf("Gen3 provisional current limit is not qualified")
	}
	return TeslaGen3ProvisionalCurrentLimit{spec.OperationVersion, spec.LimitCurrentMaxAmps, spec.LimitTimeoutSeconds, spec.InhibitCharging, append([]byte(nil), spec.ReadbackRaw...)}, nil
}
func (v TeslaGen3ProvisionalCurrentLimit) LimitCurrentMaxAmps() uint32 { return v.limitCurrentMaxAmps }
func (v TeslaGen3ProvisionalCurrentLimit) LimitTimeoutSeconds() uint32 { return v.limitTimeoutSeconds }
func (v TeslaGen3ProvisionalCurrentLimit) InhibitCharging() bool       { return v.inhibitCharging }

type TeslaGen3InteroperableCurrentLimit struct{ MaxAmps, TimeoutSeconds uint32 }

func NewTeslaGen3InteroperableCurrentLimit(amps, timeout uint32) (TeslaGen3InteroperableCurrentLimit, error) {
	if amps < teslaGen3MinCurrentAmps || timeout == 0 || timeout > teslaGen3MaxTimeoutS {
		return TeslaGen3InteroperableCurrentLimit{}, fmt.Errorf("Gen3 interoperable current limit is out of bounds")
	}
	return TeslaGen3InteroperableCurrentLimit{amps, timeout}, nil
}
