package reader

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// ChargerBaud is the serial rate of the Gen3 charger; its format is 8N1 and none
// of baud, parity or stop bits is configurable.
const ChargerBaud = 115200

// The attempt bound of one read acquisition.
const (
	minAttempts = 1
	maxAttempts = 3
)

// ChargerConfig is one configured charger. Every duration and count the native
// lifecycle requires from its caller is supplied here and none has a default.
// The serial path is configuration: it reaches only the opener.
type ChargerConfig struct {
	Endpoint                     string
	SerialPath                   string
	Node                         byte
	Version                      string
	ResponseTimeout              time.Duration
	MaxResponseDelay             time.Duration
	MaxQuiescence                time.Duration
	PollInterval                 time.Duration
	AcquisitionDeadline          time.Duration
	ActivationTimeout            time.Duration
	MaxAttempts                  int
	PersistentIdentityCapacity   int
	Operations                   []tesla.FC100Operation
	Windows                      map[tesla.FC100Operation]tesla.Gen3ReadWindow
	ProvisionalReadbackQualified bool
	ProvisionalSetQualified      bool
	PersistentSettingsQualified  bool
}

// ConfigError names the field that stops a block from opening.
type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("charger configuration: %s: %s", e.Field, e.Reason)
}

// IsEmpty reports a block with nothing configured; it starts nothing.
func (c ChargerConfig) IsEmpty() bool { return isZeroBlock(c) }

func isZeroBlock(c ChargerConfig) bool {
	d := c
	if len(d.Operations) == 0 {
		d.Operations = nil
	}
	if len(d.Windows) == 0 {
		d.Windows = nil
	}
	return reflect.DeepEqual(d, ChargerConfig{})
}

var gen3Observations = map[tesla.FC100Operation]bool{
	tesla.FC100OperationCommonSystemInfo:      true,
	tesla.FC100OperationWCGetVitals:           true,
	tesla.FC100OperationWCGetLifetimeStats:    true,
	tesla.FC100OperationWCGetSystemInfo:       true,
	tesla.FC100OperationWCGetLoadSharingState: true,
	tesla.FC100OperationWCGetProvisional:      true,
}

func windowField(op tesla.FC100Operation) string { return "windows[" + string(op) + "]" }

func windowReason(w tesla.Gen3ReadWindow) string {
	switch {
	case w == (tesla.Gen3ReadWindow{}):
		return "missing"
	case w.Freshness <= 0:
		return "freshness must be positive"
	case w.Retention < w.Freshness:
		return "retention must be at least the freshness"
	}
	return ""
}

// Admit validates the block for the five ordinary reads and reports the first
// missing or invalid value by name. It opens nothing.
func (c ChargerConfig) Admit() error {
	need := func(field string, bad bool, reason string) error {
		if bad {
			return &ConfigError{field, reason}
		}
		return nil
	}
	checks := []error{
		need("serial_path", c.SerialPath == "", "missing"),
		need("endpoint", !modbusreg.ValidIdentity(c.Endpoint), "missing or invalid"),
		need("node", c.Node == 0 || c.Node > 247, "must be 1 through 247"),
		need("version", c.Version == "", "missing"),
		need("response_timeout", c.ResponseTimeout <= 0, "missing"),
		need("max_response_delay", c.MaxResponseDelay <= 0, "missing"),
		need("max_quiescence", c.MaxQuiescence <= 0, "missing"),
		need("poll_interval", c.PollInterval <= 0, "missing"),
		need("acquisition_deadline", c.AcquisitionDeadline <= 0, "missing"),
		need("activation_timeout", c.ActivationTimeout <= 0, "missing"),
		need("max_attempts", c.MaxAttempts < minAttempts || c.MaxAttempts > maxAttempts, "must be 1 through 3"),
		need("persistent_identity_capacity", c.PersistentIdentityCapacity < 0, "must not be negative"),
		need("operations", len(c.Operations) == 0, "missing"),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	seen := map[tesla.FC100Operation]bool{}
	for _, op := range c.Operations {
		if !gen3Observations[op] || seen[op] {
			return &ConfigError{"operations", "contains an operation that is not an admitted observation or repeats one: " + string(op)}
		}
		seen[op] = true
		if reason := windowReason(c.Windows[op]); reason != "" {
			return &ConfigError{windowField(op), reason}
		}
	}
	return nil
}

// SetterAvailability says whether the bound connector setter can run, and when
// it cannot, which value is the reason.
type SetterAvailability struct {
	Available bool
	Field     string
	Reason    string
}

// firmware reads the build label the way the native read lifecycle does: three
// dot-separated numbers, or three underscore-separated numbers after "wc3_".
func firmware(label string) ([3]uint64, bool) {
	var version [3]uint64
	label = strings.TrimSpace(label)
	sep := "."
	if strings.HasPrefix(label, "wc3_") {
		label, sep = strings.TrimPrefix(label, "wc3_"), "_"
	}
	parts := strings.Split(label, sep)
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, false
		}
		version[i] = v
	}
	return version, true
}

// documentedLayout reports the builds whose t25/t26 and t27/t28 layout the
// public pages document; the native operation qualifies them on their own.
// The tesla package exposes no predicate for it, so it is restated here.
func (c ChargerConfig) documentedLayout() bool {
	v, ok := firmware(c.Version)
	return ok && (v == [3]uint64{26, 34, 0} || v == [3]uint64{26, 42, 0})
}

// Setter derives the setter's availability from the same rule the native t25
// operation applies to the configured build: a documented layout qualifies on
// its own; any other build qualifies only at the 24.44.3 floor with the
// explicit t25/t26 declaration and the t27/t28 read declaration. The actual
// off-poll t28 window must also be valid. A missing or invalid window makes the
// setter unavailable only; the five reads are unaffected.
func (c ChargerConfig) Setter() SetterAvailability {
	if !c.documentedLayout() {
		if v, ok := firmware(c.Version); !ok || v != [3]uint64{24, 44, 3} {
			return SetterAvailability{Field: "version", Reason: "t25/t26 is not qualified for this build"}
		}
		switch {
		case !c.ProvisionalSetQualified:
			return SetterAvailability{Field: "provisional_set_qualified", Reason: "t25/t26 is not qualified for this build"}
		case !c.ProvisionalReadbackQualified:
			return SetterAvailability{Field: "provisional_readback_qualified", Reason: "t27/t28 is not qualified for this build"}
		}
	}
	if reason := windowReason(c.Windows[tesla.FC100OperationWCGetProvisional]); reason != "" {
		return SetterAvailability{Field: windowField(tesla.FC100OperationWCGetProvisional), Reason: reason}
	}
	return SetterAvailability{Available: true}
}

// ReasonSetterConstraintOpen is why the connector setter capability is not
// published even where the t25 operation is qualified: the EVSE pack requires
// exactly one evse.limit.allocated_current constraint on
// evse.capability.set_allocated_current, no source gives that constraint a
// value, and the pack rule is open. No value is invented for it.
const ReasonSetterConstraintOpen = "the EVSE pack requires the evse.limit.allocated_current constraint on evse.capability.set_allocated_current and no source states its value; the pack rule is open"

// UnpublishedCapability is a capability the reader does not publish, with the
// reason.
type UnpublishedCapability struct {
	Capability semreg.DefinitionID
	// Field names the configuration that withholds it; empty when the reason is
	// not a configuration value.
	Field  string
	Reason string
}

// UnpublishedCapabilities lists the capabilities that are not published. The
// connector setter capability is never published while the pack rule for its
// constraint is open; where the t25 operation is not qualified for the build
// its reason is the one of the setter, otherwise the open pack rule.
func (c ChargerConfig) UnpublishedCapabilities() []UnpublishedCapability {
	if s := c.Setter(); !s.Available {
		return []UnpublishedCapability{{Capability: SetAllocated.Capability, Field: s.Field, Reason: s.Reason}}
	}
	return []UnpublishedCapability{{Capability: SetAllocated.Capability, Reason: ReasonSetterConstraintOpen}}
}

// CurrentLimitWritable is true only from the qualified t25 operation, never
// from configuration alone. It states the operation, not a published
// capability: the setter capability is not published (see
// UnpublishedCapabilities).
func (c ChargerConfig) CurrentLimitWritable() bool { return c.Setter().Available }

// PeriodicProvisionalRead reports whether t27 is polled: it is selected in
// Operations and its read is qualified for the build.
func (c ChargerConfig) PeriodicProvisionalRead() bool {
	for _, op := range c.Operations {
		if op == tesla.FC100OperationWCGetProvisional {
			return c.ProvisionalReadbackQualified || c.documentedLayout()
		}
	}
	return false
}

func (c ChargerConfig) acquisitionConfig(deps ChargerDeps) (tesla.Gen3ReadAcquisitionConfig, error) {
	timing, err := modbus.NewRTUTiming(modbus.RTUTimingConfig{
		Baud: ChargerBaud, DataBits: 8, Parity: modbus.RTUParityNone, StopBits: 1,
		MaxResponseLatency: c.MaxResponseDelay, MaxQuiescence: c.MaxQuiescence,
	})
	if err != nil {
		return tesla.Gen3ReadAcquisitionConfig{}, &ConfigError{"timing", err.Error()}
	}
	profile := tesla.HSCProfileConfig{
		Enabled: true, Node: c.Node, PassiveCompatible: true, CompatibilityVersion: tesla.HSCCompatibilityV1,
		WCVitalsOperationVersion:           tesla.HSCWCVitalsCompatibilityV1,
		SystemInfoOperationVersion:         tesla.HSCSystemInfoCompatibilityV1,
		WCLifetimeOperationVersion:         tesla.HSCWCLifetimeCompatibilityV1,
		WCSystemInfoOperationVersion:       tesla.HSCWCSystemInfoCompatibilityV1,
		WCLoadSharingStateOperationVersion: tesla.HSCWCLoadSharingStateCompatibilityV1,
	}
	if c.ProvisionalReadbackQualified {
		profile.WCProvisionalReadbackOperationVersion = tesla.HSCFC100OperationCompatibilityV1
		profile.WCProvisionalReadbackQualified = true
	}
	hsc, err := tesla.NewHSCProfile(profile)
	if err != nil {
		return tesla.Gen3ReadAcquisitionConfig{}, &ConfigError{"node", err.Error()}
	}
	gen3Config := tesla.Gen3HSCProfileConfig{
		Enabled: true, Version: c.Version, ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true,
		ProvisionalSetQualified: c.ProvisionalSetQualified,
	}
	if c.PersistentSettingsQualified {
		gen3Config.PersistentSettingsOperationVersion = tesla.HSCFC100OperationCompatibilityV1
		gen3Config.PersistentCurrentOnlyQualified = true
	}
	gen3, err := tesla.NewGen3HSCProfile(gen3Config)
	if err != nil {
		return tesla.Gen3ReadAcquisitionConfig{}, &ConfigError{"version", err.Error()}
	}
	windows := make(map[tesla.FC100Operation]tesla.Gen3ReadWindow, len(c.Windows))
	for op, w := range c.Windows {
		windows[op] = w
	}
	return tesla.Gen3ReadAcquisitionConfig{
		PersistentIdentityCapacity: c.PersistentIdentityCapacity,
		Endpoint:                   c.Endpoint, LocalPath: c.SerialPath, Timing: timing, ResponseTimeout: c.ResponseTimeout,
		Profile: hsc, Gen3: gen3, Operations: append([]tesla.FC100Operation(nil), c.Operations...), Windows: windows,
		MaxAttempts: c.MaxAttempts, AcquisitionDeadline: c.AcquisitionDeadline, ActivationTimeout: c.ActivationTimeout,
		PollInterval: c.PollInterval, Activation: deps.Activation, Open: deps.Open, Now: deps.Now,
	}, nil
}

// ChargerDeps are the collaborators the host supplies: the activation driver, the
// endpoint opener (nil opens the production endpoint) and the clock.
type ChargerDeps struct {
	Activation tesla.HSCActivationDriver
	Open       tesla.Gen3ReadEndpointOpener
	Now        func() time.Time
}
