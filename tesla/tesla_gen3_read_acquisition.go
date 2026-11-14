package tesla

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// This file implements the driver-owned read-path acquisition lifecycle for
// the Tesla Wall Connector Gen3 HSC profile. It follows the public contracts
// in helianthus-docs-modbus: protocols/tesla/gen3-hsc-rtu.md (serial format,
// activation, single-flight qualification), protocols/tesla/tedapi.md (the
// retained FC100 observation pairs and the request state machine),
// protocols/tesla/gen3-current-limit-operation-v1.md (outcome classes, read
// retry allowlist, recovery requires a successor generation with a valid
// activation state), and protocols/modbus/private-function-codes.md (the
// production endpoint owns framing, correlation, fencing, and recovery).
//
// Only the five retained observation operations are admitted. Current-limit
// mutation and readback pairs are not acquisition operations here. The legacy
// (pre-Gen3) Wall Connector RS-485 profile is a separate, non-Modbus flow and
// is never reached from this lifecycle. A record is native evidence only: it
// is not a support, hardware, or live-qualification claim.

const (
	// Gen3ReadSourceRTUPrivateFunction identifies records acquired through
	// the helianthus-modbus RTU production private-function exchange.
	Gen3ReadSourceRTUPrivateFunction = "rtu_production_private_function"

	teslaGen3HSCBaud                 = 115200
	maxTeslaGen3ReadAttempts         = 3
	maxTeslaGen3ReadResponses        = 8
	maxTeslaGen3ReadResponseTimeout  = 30 * time.Second
	maxTeslaGen3ReadDeadline         = 5 * time.Minute
	maxTeslaGen3ReadActivationWindow = 60 * time.Second
	maxTeslaGen3ReadRetention        = 24 * time.Hour
)

// teslaGen3HSCActivationSequence is the documented device-originated
// activation sequence: "TESLA" followed by NUL, then "PASS" followed by NUL.
var teslaGen3HSCActivationSequence = []byte("TESLA\x00PASS\x00")

var (
	// ErrTeslaGen3ReadAcquisitionBusy reports that another lifecycle call
	// already owns the single in-flight slot. The caller is not queued.
	ErrTeslaGen3ReadAcquisitionBusy = errors.New("tesla Gen3 read acquisition is busy")
	// ErrTeslaGen3ReadAcquisitionUnavailable reports that the lifecycle has no
	// activated, unfenced endpoint. Restart is required.
	ErrTeslaGen3ReadAcquisitionUnavailable = errors.New("tesla Gen3 read acquisition is unavailable")
	// ErrTeslaGen3ReadAcquisitionClosed reports a closed lifecycle.
	ErrTeslaGen3ReadAcquisitionClosed = errors.New("tesla Gen3 read acquisition is closed")
	// ErrTeslaGen3HSCActivationFailed reports a failed, timed-out, reset, or
	// unexpected activation observation; the HSC exchange state stays disabled.
	ErrTeslaGen3HSCActivationFailed = errors.New("tesla Gen3 HSC activation failed")
)

// Gen3ReadEvidenceState keeps acknowledgement, rejection, timeout, and
// indeterminate outcomes distinct. Only Gen3ReadEvidenceAck carries a
// terminal value.
type Gen3ReadEvidenceState string

const (
	// Gen3ReadEvidenceAck is a correlated terminal in the active
	// generation that the selected operation decoder accepted.
	Gen3ReadEvidenceAck Gen3ReadEvidenceState = "ack"
	// Gen3ReadEvidenceRejected is a correlated Modbus exception or a
	// TEDAPI application-error terminal. It carries no value.
	Gen3ReadEvidenceRejected Gen3ReadEvidenceState = "rejected"
	// Gen3ReadEvidenceTimeout is an exchange or acquisition deadline
	// expiry after possible transmission. It never claims a value.
	Gen3ReadEvidenceTimeout Gen3ReadEvidenceState = "timeout"
	// Gen3ReadEvidenceIndeterminate is any other failure after possible
	// transmission, or a correlated response the operation decoder rejected.
	Gen3ReadEvidenceIndeterminate Gen3ReadEvidenceState = "indeterminate"
	// Gen3ReadEvidenceNoSend is a proven pre-send rejection.
	Gen3ReadEvidenceNoSend Gen3ReadEvidenceState = "no_send"
)

// Gen3ReadLifecycleState is the externally visible lifecycle state.
type Gen3ReadLifecycleState string

const (
	Gen3ReadLifecycleStopped     Gen3ReadLifecycleState = "stopped"
	Gen3ReadLifecycleActivating  Gen3ReadLifecycleState = "activating"
	Gen3ReadLifecycleReady       Gen3ReadLifecycleState = "ready"
	Gen3ReadLifecycleRecovering  Gen3ReadLifecycleState = "recovering"
	Gen3ReadLifecycleUnavailable Gen3ReadLifecycleState = "unavailable"
	Gen3ReadLifecycleClosed      Gen3ReadLifecycleState = "closed"
)

// Gen3HSCActivationRequest identifies the endpoint and lifecycle epoch
// whose activation is being observed.
type Gen3HSCActivationRequest struct {
	Endpoint string
	Epoch    uint64
}

// Gen3HSCActivationWitness returns the bytes the device originated for the
// documented activation sequence on the owned endpoint. The lifecycle compares
// them byte-for-byte; any error, timeout, or other byte sequence leaves the HSC
// exchange state disabled. The witness never admits an exchange by itself.
type Gen3HSCActivationWitness interface {
	ObserveTeslaGen3HSCActivation(context.Context, Gen3HSCActivationRequest) ([]byte, error)
}

// Gen3ReadEndpoint is the subset of the helianthus-modbus RTU production
// endpoint the lifecycle owns. *modbus.RTUProductionEndpoint satisfies it.
type Gen3ReadEndpoint interface {
	ExchangePrivateFunction(context.Context, byte, modbus.PrivateFunctionRequest, modbus.PrivateFunctionResponsePolicy, modbus.RTUPrivateFunctionResponseClassifier) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error)
	Recover(context.Context) error
	Close() error
}

// Gen3ReadEndpointOpener opens one production endpoint from the
// configuration the lifecycle constructs.
type Gen3ReadEndpointOpener func(modbus.RTUProductionConfig) (Gen3ReadEndpoint, error)

// OpenTeslaGen3ReadProductionEndpoint is the default opener: the
// helianthus-modbus production endpoint owns the serial stream.
func OpenTeslaGen3ReadProductionEndpoint(config modbus.RTUProductionConfig) (Gen3ReadEndpoint, error) {
	endpoint, err := modbus.OpenRTUProductionEndpoint(config)
	if err != nil {
		return nil, err
	}
	return endpoint, nil
}

// Gen3ReadWindow bounds how long an acknowledged record is fresh and
// how long it is retained as last-known-good evidence.
type Gen3ReadWindow struct {
	Freshness time.Duration
	Retention time.Duration
}

// Gen3ReadAcquisitionConfig is an explicit local configuration. No field
// is discovered or inferred.
type Gen3ReadAcquisitionConfig struct {
	// Endpoint is the public endpoint label carried in evidence.
	Endpoint string
	// LocalPath is passed only to the endpoint opener; it never appears in a
	// record.
	LocalPath       string
	Timing          modbus.RTUTiming
	ResponseTimeout time.Duration
	Profile         HSCProfile
	Gen3            Gen3HSCProfile
	// Operations is the ordered polling set, a subset of the five retained
	// observation operations.
	Operations []FC100Operation
	Windows    map[FC100Operation]Gen3ReadWindow
	// MaxAttempts bounds exchanges per acquisition, 1 through 3.
	MaxAttempts int
	// AcquisitionDeadline bounds one acquisition including retries and
	// recovery.
	AcquisitionDeadline time.Duration
	ActivationTimeout   time.Duration
	PollInterval        time.Duration
	Activation          Gen3HSCActivationWitness
	Open                Gen3ReadEndpointOpener
	Now                 func() time.Time
}

// Gen3ReadRecord is one acquisition outcome with native identity,
// profile/version, source, timestamps, and evidence state. Byte accessors
// return independent copies.
type Gen3ReadRecord struct {
	Endpoint               string
	Node                   byte
	Operation              FC100Operation
	Family                 uint64
	RequestTag             uint64
	TerminalTag            uint64
	Profile                string
	ProfileVersion         string
	OperationVersion       string
	Gen3Version            string
	Gen3VersionDisposition Gen3HSCVersionDisposition
	Source                 string
	LifecycleEpoch         uint64
	Generation             uint64
	RequestID              uint64
	Attempt                int
	MaxAttempts            int
	ObservedAt             time.Time
	ReceiptWall            time.Time
	SentAt                 time.Duration
	ReceivedAt             time.Duration
	ClockEpoch             string
	State                  Gen3ReadEvidenceState
	TransportOutcome       string
	IntegrityValid         bool
	HasExceptionStatus     bool
	ExceptionStatus        byte
	HasApplicationStatus   bool
	ApplicationStatus      uint64
	Cause                  string
	requestADU             []byte
	responseADUs           [][]byte
	results                []FC100OperationResult
}

// RequestADU returns the exact request frame, when one was constructed.
func (record Gen3ReadRecord) RequestADU() []byte { return cloneTeslaGen3Bytes(record.requestADU) }

// ResponseADUs returns every received frame in arrival order.
func (record Gen3ReadRecord) ResponseADUs() [][]byte {
	if record.responseADUs == nil {
		return nil
	}
	frames := make([][]byte, len(record.responseADUs))
	for index, frame := range record.responseADUs {
		frames[index] = cloneTeslaGen3Bytes(frame)
	}
	return frames
}

// Results returns the decoded FC100 sequence. It is nil unless a correlated
// sequence reached the operation decoder and was accepted.
func (record Gen3ReadRecord) Results() []FC100OperationResult {
	if record.results == nil {
		return nil
	}
	results := make([]FC100OperationResult, len(record.results))
	for index, result := range record.results {
		results[index] = result
		results[index].Body = cloneTeslaGen3Bytes(result.Body)
	}
	return results
}

// TerminalBody returns the complete bounded native terminal body for an ACK
// record and nil for every other state.
func (record Gen3ReadRecord) TerminalBody() []byte {
	if record.State != Gen3ReadEvidenceAck {
		return nil
	}
	for _, result := range record.results {
		if result.Kind == FC100OperationTerminal {
			return cloneTeslaGen3Bytes(result.Body)
		}
	}
	return nil
}

func (record Gen3ReadRecord) clone() Gen3ReadRecord {
	record.requestADU = record.RequestADU()
	record.responseADUs = record.ResponseADUs()
	record.results = record.Results()
	return record
}

// Gen3ReadRetained is the retained view of one operation: the last ACK
// (last-known-good) and the last attempt. Fresh is age-based; Current also
// requires the record's lifecycle epoch to be the active ready epoch.
type Gen3ReadRetained struct {
	LastGood       Gen3ReadRecord
	HasLastGood    bool
	LastAttempt    Gen3ReadRecord
	HasLastAttempt bool
	Fresh          bool
	Current        bool
}

type teslaGen3ReadRetention struct {
	lastGood       Gen3ReadRecord
	hasLastGood    bool
	lastAttempt    Gen3ReadRecord
	hasLastAttempt bool
}

type teslaGen3ReadPlan struct {
	instance uint64
	payload  []byte
}

// Gen3ReadAcquisition owns one production endpoint, its activation
// state, single-flight scheduling, bounded retries, and retained records.
type Gen3ReadAcquisition struct {
	config     Gen3ReadAcquisitionConfig
	operations []FC100Operation
	windows    map[FC100Operation]Gen3ReadWindow

	mu       sync.Mutex
	state    Gen3ReadLifecycleState
	busy     bool
	closed   bool
	endpoint Gen3ReadEndpoint
	// instance identifies the owned endpoint; admission callbacks bound to a
	// retired instance deny every candidate.
	instance uint64
	// epoch increments for every activated endpoint generation (start,
	// restart, recovery).
	epoch uint64
	// epochGeneration pins the transport generation admitted in this epoch;
	// zero until the first admission. lastGeneration is the previous epoch's
	// generation on the same endpoint instance.
	epochGeneration uint64
	lastGeneration  uint64
	plan            *teslaGen3ReadPlan
	retained        map[FC100Operation]*teslaGen3ReadRetention
}

// NewGen3ReadAcquisition validates the explicit configuration and every
// profile, version, and operation gate. It performs no I/O.
func NewGen3ReadAcquisition(config Gen3ReadAcquisitionConfig) (*Gen3ReadAcquisition, error) {
	if err := validateTeslaGen3ReadAcquisitionConfig(config); err != nil {
		return nil, err
	}
	if config.Open == nil {
		config.Open = OpenTeslaGen3ReadProductionEndpoint
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	windows := make(map[FC100Operation]Gen3ReadWindow, len(config.Operations))
	retained := make(map[FC100Operation]*teslaGen3ReadRetention, len(config.Operations))
	for _, operation := range config.Operations {
		windows[operation] = config.Windows[operation]
		retained[operation] = &teslaGen3ReadRetention{}
	}
	operations := append([]FC100Operation(nil), config.Operations...)
	config.Operations = nil
	config.Windows = nil
	return &Gen3ReadAcquisition{
		config: config, operations: operations, windows: windows,
		state: Gen3ReadLifecycleStopped, retained: retained,
	}, nil
}

func validateTeslaGen3ReadAcquisitionConfig(config Gen3ReadAcquisitionConfig) error {
	noSend := func(reason string) error {
		return fmt.Errorf("%w: tesla Gen3 read acquisition %s", modbusreg.ErrQualifiedFunctionNoSend, reason)
	}
	if !modbusreg.ValidIdentity(config.Endpoint) || config.LocalPath == "" {
		return noSend("endpoint is invalid")
	}
	if config.Profile.Disposition() != HSCQualifiedReadOnly || !config.Gen3.ExchangeEligible() {
		return noSend("profile is not qualified")
	}
	// The retained catalog and decoders are qualified only for the exact
	// wc3_24_44_3 operation version. Another Gen3 version label, even a known
	// observation or a compatible candidate, has no qualified mapping here.
	if config.Gen3.Version() != HSCFC100OperationCompatibilityV1 {
		return noSend("Gen3 version is not the qualified operation version")
	}
	if config.Activation == nil {
		return noSend("has no activation witness")
	}
	if config.ResponseTimeout <= 0 || config.ResponseTimeout > maxTeslaGen3ReadResponseTimeout ||
		config.AcquisitionDeadline < config.ResponseTimeout || config.AcquisitionDeadline > maxTeslaGen3ReadDeadline ||
		config.ActivationTimeout <= 0 || config.ActivationTimeout > maxTeslaGen3ReadActivationWindow ||
		config.PollInterval <= 0 || config.PollInterval > maxTeslaGen3ReadRetention {
		return noSend("time bound is invalid")
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > maxTeslaGen3ReadAttempts {
		return noSend("attempt bound is invalid")
	}
	if len(config.Operations) == 0 {
		return noSend("has no operations")
	}
	seen := make(map[FC100Operation]bool, len(config.Operations))
	for _, operation := range config.Operations {
		qualified, ok := teslaGen3ReadQualifiedOperation(operation)
		if !ok || seen[operation] || !config.Profile.OperationAdmissionFor(qualified).OutboundAllowed {
			return noSend("operation is not an admitted observation")
		}
		seen[operation] = true
		window, ok := config.Windows[operation]
		if !ok || window.Freshness <= 0 || window.Retention < window.Freshness || window.Retention > maxTeslaGen3ReadRetention {
			return noSend("freshness or retention window is invalid")
		}
	}
	return nil
}

// teslaGen3ReadQualifiedOperation maps a retained observation operation to
// its exact version-gated qualified request. Mutation and readback pairs have
// no mapping and are never acquired here.
func teslaGen3ReadQualifiedOperation(operation FC100Operation) (string, bool) {
	switch operation {
	case FC100OperationCommonSystemInfo:
		return TEDAPIOperationCommonSystemInfoV1, true
	case FC100OperationWCGetVitals:
		return TEDAPIOperationWCVitalsV1, true
	case FC100OperationWCGetLifetimeStats:
		return TEDAPIOperationWCLifetimeV1, true
	case FC100OperationWCGetSystemInfo:
		return TEDAPIOperationWCSystemInfoV1, true
	case FC100OperationWCGetLoadSharingState:
		return TEDAPIOperationWCLoadSharingStateV1, true
	default:
		return "", false
	}
}

// State returns the current lifecycle state.
func (acquisition *Gen3ReadAcquisition) State() Gen3ReadLifecycleState {
	if acquisition == nil {
		return Gen3ReadLifecycleClosed
	}
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	return acquisition.state
}

func (acquisition *Gen3ReadAcquisition) reserve() error {
	if acquisition == nil {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if acquisition.closed {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	if acquisition.busy {
		return ErrTeslaGen3ReadAcquisitionBusy
	}
	acquisition.busy = true
	return nil
}

func (acquisition *Gen3ReadAcquisition) release() {
	acquisition.mu.Lock()
	acquisition.busy = false
	acquisition.mu.Unlock()
}

// Start opens the production endpoint and completes activation. It is valid
// only from the stopped state; use Restart afterwards.
func (acquisition *Gen3ReadAcquisition) Start(ctx context.Context) error {
	if ctx == nil {
		return modbusreg.ErrQualifiedFunctionNoSend
	}
	if err := acquisition.reserve(); err != nil {
		return err
	}
	defer acquisition.release()
	acquisition.mu.Lock()
	state := acquisition.state
	acquisition.mu.Unlock()
	if state != Gen3ReadLifecycleStopped {
		return fmt.Errorf("tesla Gen3 read acquisition already started")
	}
	return acquisition.startLocked(ctx)
}

// Restart closes the owned endpoint, opens a successor, and re-activates it.
// Retained records stay as historical last-known-good evidence.
func (acquisition *Gen3ReadAcquisition) Restart(ctx context.Context) error {
	if ctx == nil {
		return modbusreg.ErrQualifiedFunctionNoSend
	}
	if err := acquisition.reserve(); err != nil {
		return err
	}
	defer acquisition.release()
	return acquisition.startLocked(ctx)
}

// startLocked runs with the single-flight slot held.
func (acquisition *Gen3ReadAcquisition) startLocked(ctx context.Context) error {
	acquisition.retireEndpoint()
	endpoint, err := acquisition.config.Open(acquisition.productionConfig())
	if err != nil || endpoint == nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		if err == nil {
			err = errors.New("tesla Gen3 read endpoint opener returned no endpoint")
		}
		return fmt.Errorf("%w: %w", ErrTeslaGen3ReadAcquisitionUnavailable, err)
	}
	acquisition.mu.Lock()
	if acquisition.closed {
		acquisition.mu.Unlock()
		_ = endpoint.Close()
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	acquisition.endpoint = endpoint
	acquisition.lastGeneration = 0
	acquisition.epochGeneration = 0
	acquisition.mu.Unlock()
	if err := acquisition.activate(ctx); err != nil {
		acquisition.retireEndpoint()
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return err
	}
	return nil
}

// productionConfig builds the production endpoint configuration with the
// documented Gen3 HSC serial format (115200 8N1). The FC03/FC04 read path is
// denied; the private-function path admits only the planned candidate.
func (acquisition *Gen3ReadAcquisition) productionConfig() modbus.RTUProductionConfig {
	acquisition.mu.Lock()
	acquisition.instance++
	admission := teslaGen3ReadAdmission{acquisition: acquisition, instance: acquisition.instance}
	acquisition.mu.Unlock()
	return modbus.RTUProductionConfig{
		Endpoint: acquisition.config.Endpoint,
		Serial: modbus.RTUSerialConfig{
			Path: acquisition.config.LocalPath, Baud: teslaGen3HSCBaud, DataBits: 8,
			Parity: modbus.RTUParityNone, StopBits: 1,
		},
		Timing:          acquisition.config.Timing,
		ResponseTimeout: acquisition.config.ResponseTimeout,
		Enabled:         true,
		Admission:       teslaGen3ReadDenyRegisterReads{},
		PrivateFunction: modbus.RTUPrivateFunctionConfig{Admission: admission, MaxResponses: maxTeslaGen3ReadResponses},
	}
}

// retireEndpoint closes and forgets the owned endpoint, retiring its
// admission instance. Records from it remain historical.
func (acquisition *Gen3ReadAcquisition) retireEndpoint() {
	acquisition.mu.Lock()
	endpoint := acquisition.endpoint
	acquisition.endpoint = nil
	acquisition.instance++
	acquisition.plan = nil
	acquisition.mu.Unlock()
	if endpoint != nil {
		_ = endpoint.Close()
	}
}

// retireOwned retires endpoint if the lifecycle still owns it and leaves the
// lifecycle unavailable until Restart.
func (acquisition *Gen3ReadAcquisition) retireOwned(endpoint Gen3ReadEndpoint) {
	acquisition.mu.Lock()
	owned := acquisition.endpoint == endpoint
	acquisition.mu.Unlock()
	if owned {
		acquisition.retireEndpoint()
	}
	acquisition.setState(Gen3ReadLifecycleUnavailable)
}

func (acquisition *Gen3ReadAcquisition) setState(state Gen3ReadLifecycleState) {
	acquisition.mu.Lock()
	if !acquisition.closed {
		acquisition.state = state
	}
	acquisition.mu.Unlock()
}

// activate observes the documented device-originated activation sequence
// within ActivationTimeout. Success opens a new lifecycle epoch.
func (acquisition *Gen3ReadAcquisition) activate(ctx context.Context) error {
	acquisition.mu.Lock()
	if acquisition.closed {
		acquisition.mu.Unlock()
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	acquisition.state = Gen3ReadLifecycleActivating
	epoch := acquisition.epoch + 1
	acquisition.mu.Unlock()
	deadline := acquisition.config.Now().Add(acquisition.config.ActivationTimeout)
	activationCtx, cancel := context.WithTimeout(ctx, acquisition.config.ActivationTimeout)
	defer cancel()
	observed, err := acquisition.config.Activation.ObserveTeslaGen3HSCActivation(
		activationCtx, Gen3HSCActivationRequest{Endpoint: acquisition.config.Endpoint, Epoch: epoch})
	returned := acquisition.config.Now()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, err)
	}
	// A report delivered at or after the activation deadline, or after
	// cancellation, is not a completed activation even when its bytes match.
	// The deadline is compared against the report-return time on the
	// lifecycle clock because the context timer may not have fired yet.
	if !returned.Before(deadline) {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, context.DeadlineExceeded)
	}
	if contextDeadline, ok := activationCtx.Deadline(); ok && !time.Now().Before(contextDeadline) {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, context.DeadlineExceeded)
	}
	if !bytes.Equal(observed, teslaGen3HSCActivationSequence) {
		return fmt.Errorf("%w: unexpected activation bytes", ErrTeslaGen3HSCActivationFailed)
	}
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if acquisition.closed {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	// Re-check cancellation inside the readiness commit.
	if ctxErr := activationCtx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, ctxErr)
	}
	acquisition.epoch = epoch
	acquisition.state = Gen3ReadLifecycleReady
	return nil
}

// Close closes the owned endpoint. An in-flight exchange observes endpoint
// loss and completes as an indeterminate record; nothing is resent.
func (acquisition *Gen3ReadAcquisition) Close() error {
	if acquisition == nil {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	acquisition.mu.Lock()
	if acquisition.closed {
		acquisition.mu.Unlock()
		return nil
	}
	acquisition.closed = true
	acquisition.state = Gen3ReadLifecycleClosed
	endpoint := acquisition.endpoint
	acquisition.endpoint = nil
	acquisition.instance++
	acquisition.plan = nil
	acquisition.mu.Unlock()
	if endpoint != nil {
		return endpoint.Close()
	}
	return nil
}

// Acquire performs one bounded acquisition of a configured observation
// operation. Concurrent callers fail fast with ErrTeslaGen3ReadAcquisitionBusy.
// Exchange outcomes are returned as records; errors report lifecycle or
// admission refusals before any exchange.
func (acquisition *Gen3ReadAcquisition) Acquire(ctx context.Context, operation FC100Operation) (Gen3ReadRecord, error) {
	if ctx == nil {
		return Gen3ReadRecord{}, modbusreg.ErrQualifiedFunctionNoSend
	}
	if err := acquisition.reserve(); err != nil {
		return Gen3ReadRecord{}, err
	}
	defer acquisition.release()
	return acquisition.acquireLocked(ctx, operation)
}

// PollOnce performs one bounded polling cycle: each configured operation whose
// last-known-good record is not fresh is acquired once, in configured order.
func (acquisition *Gen3ReadAcquisition) PollOnce(ctx context.Context) ([]Gen3ReadRecord, error) {
	if ctx == nil {
		return nil, modbusreg.ErrQualifiedFunctionNoSend
	}
	if err := acquisition.reserve(); err != nil {
		return nil, err
	}
	defer acquisition.release()
	var records []Gen3ReadRecord
	for _, operation := range acquisition.operations {
		if retained, ok := acquisition.Retained(operation); ok && retained.Fresh && retained.Current {
			continue
		}
		record, err := acquisition.acquireLocked(ctx, operation)
		if err != nil {
			return records, err
		}
		records = append(records, record)
		if ctx.Err() != nil {
			return records, ctx.Err()
		}
	}
	return records, nil
}

// Run polls every PollInterval until ctx ends. An unavailable lifecycle is
// restarted at most once per interval. It returns ctx's error.
func (acquisition *Gen3ReadAcquisition) Run(ctx context.Context) error {
	if ctx == nil {
		return modbusreg.ErrQualifiedFunctionNoSend
	}
	ticker := time.NewTicker(acquisition.config.PollInterval)
	defer ticker.Stop()
	for {
		_, err := acquisition.PollOnce(ctx)
		switch {
		case errors.Is(err, ErrTeslaGen3ReadAcquisitionClosed):
			return err
		case errors.Is(err, ErrTeslaGen3ReadAcquisitionUnavailable):
			_ = acquisition.Restart(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (acquisition *Gen3ReadAcquisition) acquireLocked(parent context.Context, operation FC100Operation) (Gen3ReadRecord, error) {
	qualified, ok := teslaGen3ReadQualifiedOperation(operation)
	if _, configured := acquisition.windows[operation]; !ok || !configured {
		return Gen3ReadRecord{}, modbusreg.ErrQualifiedFunctionNoSend
	}
	request, policy, err := acquisition.config.Profile.EncodeQualifiedFunction(qualified)
	if err != nil {
		return Gen3ReadRecord{}, modbusreg.ErrQualifiedFunctionNoSend
	}
	ctx, cancel := context.WithTimeout(parent, acquisition.config.AcquisitionDeadline)
	defer cancel()
	var record Gen3ReadRecord
	for attempt := 1; attempt <= acquisition.config.MaxAttempts; attempt++ {
		acquisition.mu.Lock()
		closed, state, endpoint := acquisition.closed, acquisition.state, acquisition.endpoint
		acquisition.mu.Unlock()
		if closed {
			if attempt == 1 {
				return Gen3ReadRecord{}, ErrTeslaGen3ReadAcquisitionClosed
			}
			return record, nil
		}
		if state != Gen3ReadLifecycleReady || endpoint == nil {
			if attempt == 1 {
				return Gen3ReadRecord{}, ErrTeslaGen3ReadAcquisitionUnavailable
			}
			return record, nil
		}
		var followUp teslaGen3ReadFollowUp
		record, followUp = acquisition.exchange(ctx, endpoint, operation, request, policy, attempt)
		record = acquisition.publish(record, endpoint)
		switch followUp {
		case teslaGen3ReadFollowUpRecover:
			if !acquisition.recover(ctx, endpoint) {
				return record, nil
			}
		case teslaGen3ReadFollowUpRetire:
			acquisition.retireOwned(endpoint)
			return record, nil
		}
		switch record.State {
		case Gen3ReadEvidenceAck, Gen3ReadEvidenceRejected, Gen3ReadEvidenceNoSend:
			return record, nil
		}
		if ctx.Err() != nil {
			return record, nil
		}
	}
	return record, nil
}

// teslaGen3ReadFollowUp is the lifecycle action an exchange outcome requires.
type teslaGen3ReadFollowUp uint8

const (
	teslaGen3ReadFollowUpNone teslaGen3ReadFollowUp = iota
	// teslaGen3ReadFollowUpRecover: the transport fenced the generation.
	teslaGen3ReadFollowUpRecover
	// teslaGen3ReadFollowUpRetire: the transport reported success for a
	// sequence the operation decoder rejected; the endpoint is not fenced,
	// so it is retired instead of being reused in the same generation.
	teslaGen3ReadFollowUpRetire
)

// exchange performs one single-attempt production exchange and classifies it.
func (acquisition *Gen3ReadAcquisition) exchange(
	ctx context.Context,
	endpoint Gen3ReadEndpoint,
	operation FC100Operation,
	request modbus.PrivateFunctionRequest,
	policy modbus.PrivateFunctionResponsePolicy,
	attempt int,
) (Gen3ReadRecord, teslaGen3ReadFollowUp) {
	spec := teslaFC100OperationSpecs[operation]
	acquisition.mu.Lock()
	epoch := acquisition.epoch
	acquisition.plan = &teslaGen3ReadPlan{instance: acquisition.instance, payload: request.Payload()}
	acquisition.mu.Unlock()
	responses, evidence, err := endpoint.ExchangePrivateFunction(
		ctx, acquisition.config.Profile.Node(), request, policy, teslaGen3ReadClassifier{operation: operation, request: request.Payload()})
	acquisition.mu.Lock()
	acquisition.plan = nil
	acquisition.mu.Unlock()

	record := Gen3ReadRecord{
		Endpoint: acquisition.config.Endpoint, Node: acquisition.config.Profile.Node(), Operation: operation,
		Family: spec.family, RequestTag: spec.requestTag, TerminalTag: spec.responseTag,
		Profile: HSCProfileName, ProfileVersion: HSCCompatibilityV1,
		OperationVersion: HSCFC100OperationCompatibilityV1,
		Gen3Version:      acquisition.config.Gen3.Version(), Gen3VersionDisposition: acquisition.config.Gen3.VersionDisposition(),
		Source: Gen3ReadSourceRTUPrivateFunction, LifecycleEpoch: epoch,
		Generation: evidence.Generation, RequestID: evidence.RequestID,
		Attempt: attempt, MaxAttempts: acquisition.config.MaxAttempts,
		ObservedAt: acquisition.config.Now(), ReceiptWall: evidence.ReceiptWall,
		SentAt: evidence.SentAt, ReceivedAt: evidence.ReceivedAt, ClockEpoch: evidence.ClockEpoch,
		TransportOutcome: evidence.TerminalOutcome, IntegrityValid: evidence.IntegrityValid,
		requestADU: cloneTeslaGen3Bytes(evidence.RequestADU),
	}
	if evidence.ResponseADUs != nil {
		record.responseADUs = make([][]byte, len(evidence.ResponseADUs))
		for index, frame := range evidence.ResponseADUs {
			record.responseADUs[index] = cloneTeslaGen3Bytes(frame)
		}
	}
	if err != nil {
		record.Cause = err.Error()
	}
	switch {
	case err != nil && evidence.RequestID == 0:
		record.State = Gen3ReadEvidenceNoSend
		return record, teslaGen3ReadFollowUpNone
	case err != nil && evidence.TerminalOutcome == "exception":
		record.State = Gen3ReadEvidenceRejected
		if frames := evidence.ResponseADUs; len(frames) != 0 {
			if last := frames[len(frames)-1]; len(last) == 5 && last[1] == request.FunctionCode().Byte()|0x80 {
				record.HasExceptionStatus = true
				record.ExceptionStatus = last[2]
			}
		}
		return record, teslaGen3ReadFollowUpNone
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		record.State = Gen3ReadEvidenceTimeout
		return record, teslaGen3ReadFollowUpRecover
	case err != nil:
		record.State = Gen3ReadEvidenceIndeterminate
		return record, teslaGen3ReadFollowUpRecover
	}
	// A receipt that was not current at the transport never carries a value.
	// Ownership of the endpoint and epoch is checked again when the record
	// is published (see publish), under the lifecycle mutex.
	if !evidence.Current {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = "receipt is not current for the owned endpoint epoch"
		return record, teslaGen3ReadFollowUpNone
	}
	payloads := make([][]byte, len(responses))
	for index, response := range responses {
		payloads[index] = response.Payload()
	}
	results, decodeErr := DecodeTeslaFC100OperationSequence(HSCFC100OperationCompatibilityV1, operation, request.Payload(), payloads)
	if decodeErr != nil {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = decodeErr.Error()
		return record, teslaGen3ReadFollowUpRetire
	}
	record.results = results
	record.State = Gen3ReadEvidenceAck
	for _, result := range results {
		if result.Kind == FC100OperationApplicationError {
			record.State = Gen3ReadEvidenceRejected
			record.HasApplicationStatus = true
			record.ApplicationStatus = result.Status
		}
	}
	return record, teslaGen3ReadFollowUpNone
}

// recover runs the transport's bounded recovery and then re-activates, which
// opens a successor epoch. Failure leaves the lifecycle unavailable.
func (acquisition *Gen3ReadAcquisition) recover(ctx context.Context, endpoint Gen3ReadEndpoint) bool {
	acquisition.mu.Lock()
	if acquisition.closed || acquisition.endpoint != endpoint {
		acquisition.mu.Unlock()
		return false
	}
	acquisition.state = Gen3ReadLifecycleRecovering
	acquisition.mu.Unlock()
	// Recovery and re-activation stay inside the acquisition budget. An
	// expired or cancelled acquisition leaves the endpoint fenced and the
	// lifecycle unavailable; Restart is the bounded way back.
	if ctx.Err() != nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return false
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, acquisition.config.ResponseTimeout+acquisition.config.Timing.InterFrame())
	defer cancel()
	if err := endpoint.Recover(recoveryCtx); err != nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return false
	}
	acquisition.mu.Lock()
	acquisition.lastGeneration = acquisition.epochGeneration
	acquisition.epochGeneration = 0
	acquisition.mu.Unlock()
	if err := acquisition.activate(ctx); err != nil {
		acquisition.setState(Gen3ReadLifecycleUnavailable)
		return false
	}
	return true
}

// publish linearizes the final ownership check, ACK promotion, and retention
// with Close and endpoint retirement: all three run in one critical section
// of the lifecycle mutex, which Close also takes. When the lifecycle no longer
// owns the endpoint and epoch the record was acquired in, an ACK is downgraded
// to indeterminate with its frames preserved and decoded values cleared.
func (acquisition *Gen3ReadAcquisition) publish(record Gen3ReadRecord, endpoint Gen3ReadEndpoint) Gen3ReadRecord {
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if record.State == Gen3ReadEvidenceAck &&
		(acquisition.closed || acquisition.endpoint != endpoint || acquisition.epoch != record.LifecycleEpoch ||
			acquisition.state != Gen3ReadLifecycleReady) {
		record.State = Gen3ReadEvidenceIndeterminate
		record.Cause = "receipt is not current for the owned endpoint epoch"
		record.results = nil
	}
	acquisition.retainLocked(record)
	return record.clone()
}

func (acquisition *Gen3ReadAcquisition) retainLocked(record Gen3ReadRecord) {
	entry := acquisition.retained[record.Operation]
	if entry == nil {
		return
	}
	entry.lastAttempt = record.clone()
	entry.hasLastAttempt = true
	if record.State == Gen3ReadEvidenceAck {
		entry.lastGood = record.clone()
		entry.hasLastGood = true
	}
}

// Retained returns the retained view of one operation. Records older than the
// operation's retention window are discarded and not returned.
func (acquisition *Gen3ReadAcquisition) Retained(operation FC100Operation) (Gen3ReadRetained, bool) {
	if acquisition == nil {
		return Gen3ReadRetained{}, false
	}
	window, ok := acquisition.windows[operation]
	if !ok {
		return Gen3ReadRetained{}, false
	}
	now := acquisition.config.Now()
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	entry := acquisition.retained[operation]
	if entry.hasLastGood && now.Sub(entry.lastGood.ObservedAt) > window.Retention {
		entry.lastGood, entry.hasLastGood = Gen3ReadRecord{}, false
	}
	if entry.hasLastAttempt && now.Sub(entry.lastAttempt.ObservedAt) > window.Retention {
		entry.lastAttempt, entry.hasLastAttempt = Gen3ReadRecord{}, false
	}
	if !entry.hasLastGood && !entry.hasLastAttempt {
		return Gen3ReadRetained{}, false
	}
	view := Gen3ReadRetained{
		LastGood: entry.lastGood.clone(), HasLastGood: entry.hasLastGood,
		LastAttempt: entry.lastAttempt.clone(), HasLastAttempt: entry.hasLastAttempt,
	}
	if entry.hasLastGood {
		age := now.Sub(entry.lastGood.ObservedAt)
		view.Fresh = age >= 0 && age <= window.Freshness
		view.Current = !acquisition.closed && acquisition.state == Gen3ReadLifecycleReady &&
			entry.lastGood.LifecycleEpoch == acquisition.epoch
	}
	return view, true
}

// admit is the private-function admission decision for the owned endpoint.
func (acquisition *Gen3ReadAcquisition) admit(instance uint64, candidate modbus.RTUPrivateFunctionAdmissionRequest) bool {
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	plan := acquisition.plan
	if acquisition.closed || acquisition.state != Gen3ReadLifecycleReady || plan == nil ||
		instance != acquisition.instance || plan.instance != instance ||
		candidate.Endpoint != acquisition.config.Endpoint || candidate.UnitID != acquisition.config.Profile.Node() ||
		candidate.Function != teslaHSCFunction100 || !bytes.Equal(candidate.Payload, plan.payload) {
		return false
	}
	if acquisition.epochGeneration == 0 {
		if candidate.Generation <= acquisition.lastGeneration {
			return false
		}
		acquisition.epochGeneration = candidate.Generation
		return true
	}
	return candidate.Generation == acquisition.epochGeneration
}

type teslaGen3ReadAdmission struct {
	acquisition *Gen3ReadAcquisition
	instance    uint64
}

func (admission teslaGen3ReadAdmission) AdmitRTUPrivateFunction(candidate modbus.RTUPrivateFunctionAdmissionRequest) bool {
	return admission.acquisition != nil && admission.acquisition.admit(admission.instance, candidate)
}

// teslaGen3ReadDenyRegisterReads satisfies the production endpoint's required
// FC03/FC04 admission while admitting no register read.
type teslaGen3ReadDenyRegisterReads struct{}

func (teslaGen3ReadDenyRegisterReads) AdmitRTURead(modbus.RTUReadAdmissionRequest) bool { return false }

// teslaGen3ReadClassifier marks the exact FC100 request echo as an
// intermediate frame. Any other frame is terminal only when it is the
// selected operation's exact family/terminal-tag container or a TEDAPI
// application error; every other frame is a classifier fault, which fences
// the transport generation so no later request can be satisfied by it.
type teslaGen3ReadClassifier struct {
	operation FC100Operation
	request   []byte
}

func (classifier teslaGen3ReadClassifier) ClassifyRTUPrivateFunctionResponse(response modbus.RTUPrivateFunctionResponseADU) (modbus.RTUPrivateFunctionDisposition, error) {
	payload := response.Payload()
	if bytes.Equal(payload, classifier.request) {
		return modbus.RTUPrivateFunctionContinue, nil
	}
	envelope, err := DecodeTeslaHSCEnvelope(teslaHSCFunction100, payload)
	if err != nil {
		return 0, err
	}
	message := envelope.Payload()
	if _, ok := decodeTeslaFC100ApplicationError(message); ok {
		return modbus.RTUPrivateFunctionTerminal, nil
	}
	spec, ok := teslaFC100OperationSpecs[classifier.operation]
	if !ok {
		return 0, fmt.Errorf("tesla Gen3 read operation is not correlated")
	}
	family, err := decodeExactLengthDelimitedField(message, spec.family)
	if err != nil {
		return 0, fmt.Errorf("tesla Gen3 read terminal family is not correlated: %w", err)
	}
	if _, err := decodeExactLengthDelimitedField(family, spec.responseTag); err != nil {
		return 0, fmt.Errorf("tesla Gen3 read terminal tag is not correlated: %w", err)
	}
	return modbus.RTUPrivateFunctionTerminal, nil
}

func cloneTeslaGen3Bytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
