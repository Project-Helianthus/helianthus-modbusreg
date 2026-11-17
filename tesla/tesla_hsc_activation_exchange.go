package tesla

import (
	"bytes"
	"context"
	"fmt"
	"time"
)

// maxHSCActivationReportEvents bounds how many events a retained activation
// report may carry. It is a resource ceiling, not a measured retry count.
const maxHSCActivationReportEvents = 32

// The responder greeting, the confirmation, and the refusal. Each token is
// followed by NUL. Callers receive copies.
var (
	hscActivationGreetingToken     = []byte("TESLA\x00")
	hscActivationConfirmationToken = []byte("PASS\x00")
	hscActivationRefusalToken      = []byte("FAIL\x00")
)

// HSCActivationGreeting returns a copy of the responder greeting token.
func HSCActivationGreeting() []byte { return cloneTeslaGen3Bytes(hscActivationGreetingToken) }

// HSCActivationConfirmation returns a copy of the confirmation token.
func HSCActivationConfirmation() []byte {
	return cloneTeslaGen3Bytes(hscActivationConfirmationToken)
}

// HSCActivationRefusal returns a copy of the refusal token.
func HSCActivationRefusal() []byte { return cloneTeslaGen3Bytes(hscActivationRefusalToken) }

// HSCActivationEventKind is the role of one activation event.
type HSCActivationEventKind string

const (
	// HSCActivationEventGreeting is the responder greeting.
	HSCActivationEventGreeting HSCActivationEventKind = "greeting"
	// HSCActivationEventEcho is the host echo of the greeting just received.
	HSCActivationEventEcho HSCActivationEventKind = "echo"
	// HSCActivationEventConfirmation is the responder confirmation.
	HSCActivationEventConfirmation HSCActivationEventKind = "confirmation"
	// HSCActivationEventRefusal is the responder refusal.
	HSCActivationEventRefusal HSCActivationEventKind = "refusal"
)

// HSCActivationEvent is one event in an activation report. LastByteOffset is
// the host-side monotonic offset of the event's last byte. Turnaround and
// WriteDuration are set only on an echo: Turnaround runs from the last
// greeting byte to the start of the write, and WriteDuration is the write.
type HSCActivationEvent struct {
	Kind           HSCActivationEventKind
	Bytes          []byte
	LastByteOffset time.Duration
	Turnaround     time.Duration
	WriteDuration  time.Duration
}

// HSCActivationReport is the ordered activation evidence a driver returns.
// ClockEpoch labels the host monotonic clock. Events are copied on the way
// in and on the way out.
type HSCActivationReport struct {
	Events     []HSCActivationEvent
	ClockEpoch string
}

func (report HSCActivationReport) clone() HSCActivationReport {
	if report.Events == nil {
		return report
	}
	events := make([]HSCActivationEvent, len(report.Events))
	for index, event := range report.Events {
		event.Bytes = cloneTeslaGen3Bytes(event.Bytes)
		if event.Kind != HSCActivationEventEcho {
			event.Turnaround = 0
			event.WriteDuration = 0
		}
		events[index] = event
	}
	report.Events = events
	return report
}

// HSCActivationRequest is the activation the lifecycle asks a driver to run.
// Owned is the endpoint the lifecycle just opened. The driver must not retain
// it after DriveTeslaHSCActivation returns.
type HSCActivationRequest struct {
	Endpoint string
	Epoch    uint64
	Owned    Gen3ReadEndpoint
}

// HSCActivationDriver drives the activation exchange on the owned endpoint.
// A nil driver is NO_SEND. The driver does not admit a read by itself.
type HSCActivationDriver interface {
	DriveTeslaHSCActivation(context.Context, HSCActivationRequest) (HSCActivationReport, error)
}

// HSCActivationBasis says whether the driver exchange or the readiness probe
// opened the epoch.
type HSCActivationBasis string

const (
	// HSCActivationBasisDriverExchange means the driver report opened the epoch.
	HSCActivationBasisDriverExchange HSCActivationBasis = "driver_exchange"
	// HSCActivationBasisReadinessProbe means the readiness probe opened the epoch.
	HSCActivationBasisReadinessProbe HSCActivationBasis = "probe"
)

// HSCActivationOpening is a copy of the activation that opened the current epoch.
type HSCActivationOpening struct {
	Report HSCActivationReport
	Epoch  uint64
	Basis  HSCActivationBasis
}

// ActivationOpening returns a copy of the activation that opened the current
// epoch. It is nil before an activation has been observed.
func (acquisition *Gen3ReadAcquisition) ActivationOpening() *HSCActivationOpening {
	if acquisition == nil {
		return nil
	}
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if !acquisition.activationObserved {
		return nil
	}
	return &HSCActivationOpening{
		Report: acquisition.activationReport.clone(),
		Epoch:  acquisition.epoch,
		Basis:  acquisition.activationBasis,
	}
}

// hscActivationReportAccepted reports whether events are one or more attempts
// of greeting, optional echo, and refusal, ending in greeting, echo, and
// confirmation. Each echo must equal the greeting immediately before it, and
// every token must match exactly.
func hscActivationReportAccepted(report HSCActivationReport) bool {
	events := report.Events
	if len(events) == 0 || len(events) > maxHSCActivationReportEvents {
		return false
	}
	index := 0
	confirmed := false
	for index < len(events) && !confirmed {
		if events[index].Kind != HSCActivationEventGreeting || !bytes.Equal(events[index].Bytes, hscActivationGreetingToken) {
			return false
		}
		greeting := events[index].Bytes
		index++
		echoed := false
		if index < len(events) && events[index].Kind == HSCActivationEventEcho {
			if !bytes.Equal(events[index].Bytes, greeting) {
				return false
			}
			echoed = true
			index++
		}
		if index >= len(events) {
			return false
		}
		switch events[index].Kind {
		case HSCActivationEventRefusal:
			if !bytes.Equal(events[index].Bytes, hscActivationRefusalToken) {
				return false
			}
			index++
		case HSCActivationEventConfirmation:
			if !echoed || !bytes.Equal(events[index].Bytes, hscActivationConfirmationToken) {
				return false
			}
			index++
			confirmed = true
		default:
			return false
		}
	}
	return confirmed && index == len(events)
}

func (acquisition *Gen3ReadAcquisition) commitOpeningLocked(epoch uint64, report HSCActivationReport, basis HSCActivationBasis) {
	acquisition.epoch = epoch
	acquisition.state = Gen3ReadLifecycleReady
	acquisition.activationObserved = true
	acquisition.activationBasis = basis
	acquisition.activationReport = report.clone()
}

// activate drives activation on the owned endpoint. Success opens the next
// lifecycle epoch. Silence for the whole activation timeout can run the
// readiness probe instead of opening an epoch from the driver report.
func (acquisition *Gen3ReadAcquisition) activate(ctx context.Context) error {
	acquisition.mu.Lock()
	if acquisition.closed {
		acquisition.mu.Unlock()
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	endpoint := acquisition.endpoint
	acquisition.state = Gen3ReadLifecycleActivating
	epoch := acquisition.epoch + 1
	acquisition.mu.Unlock()

	deadline := acquisition.config.Now().Add(acquisition.config.ActivationTimeout)
	activationCtx, cancel := context.WithTimeout(ctx, acquisition.config.ActivationTimeout)
	defer cancel()
	report, err := acquisition.config.Activation.DriveTeslaHSCActivation(activationCtx, HSCActivationRequest{
		Endpoint: acquisition.config.Endpoint,
		Epoch:    epoch,
		Owned:    endpoint,
	})
	returned := acquisition.config.Now()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, err)
	}
	if len(report.Events) > maxHSCActivationReportEvents {
		return fmt.Errorf("%w: unexpected activation report", ErrTeslaGen3HSCActivationFailed)
	}
	report = report.clone()
	if len(report.Events) == 0 {
		return acquisition.finishSilentActivation(ctx, deadline, returned, epoch, endpoint, report)
	}
	if !returned.Before(deadline) {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, context.DeadlineExceeded)
	}
	if contextDeadline, ok := activationCtx.Deadline(); ok && !time.Now().Before(contextDeadline) {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, context.DeadlineExceeded)
	}
	if !hscActivationReportAccepted(report) {
		return fmt.Errorf("%w: unexpected activation report", ErrTeslaGen3HSCActivationFailed)
	}
	acquisition.mu.Lock()
	defer acquisition.mu.Unlock()
	if acquisition.closed {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	if ctxErr := activationCtx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, ctxErr)
	}
	acquisition.commitOpeningLocked(epoch, report, HSCActivationBasisDriverExchange)
	return nil
}

func (acquisition *Gen3ReadAcquisition) finishSilentActivation(ctx context.Context, deadline, returned time.Time, epoch uint64, endpoint Gen3ReadEndpoint, report HSCActivationReport) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, ctx.Err())
	}
	if returned.Before(deadline) || !acquisition.config.ReadinessProbe {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, context.DeadlineExceeded)
	}
	return acquisition.probeReadiness(ctx, epoch, endpoint, report)
}

// probeReadiness admits one planned read of the first configured operation
// while the lifecycle is still activating. Only an acknowledgement opens the epoch.
func (acquisition *Gen3ReadAcquisition) probeReadiness(ctx context.Context, epoch uint64, endpoint Gen3ReadEndpoint, report HSCActivationReport) error {
	if len(acquisition.operations) == 0 {
		return fmt.Errorf("%w: readiness probe has no operation", ErrTeslaGen3HSCActivationFailed)
	}
	operation := acquisition.operations[0]
	qualified, ok := teslaGen3ReadQualifiedOperation(operation)
	if !ok {
		return fmt.Errorf("%w: readiness probe has no operation", ErrTeslaGen3HSCActivationFailed)
	}
	request, policy, err := acquisition.config.Profile.EncodeQualifiedFunction(qualified)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTeslaGen3HSCActivationFailed, err)
	}
	acquisition.mu.Lock()
	if acquisition.closed || acquisition.endpoint != endpoint {
		acquisition.mu.Unlock()
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	acquisition.activatingProbe = true
	acquisition.mu.Unlock()
	defer func() {
		acquisition.mu.Lock()
		acquisition.activatingProbe = false
		acquisition.mu.Unlock()
	}()
	record, _ := acquisition.exchange(ctx, endpoint, operation, request, policy, 1)
	acquisition.mu.Lock()
	owned := !acquisition.closed && acquisition.endpoint == endpoint && ctx.Err() == nil
	acknowledged := owned && record.State == Gen3ReadEvidenceAck && record.Generation != 0 && record.Generation == acquisition.epochGeneration
	if acknowledged {
		acquisition.commitOpeningLocked(epoch, report, HSCActivationBasisReadinessProbe)
		record.LifecycleEpoch = epoch
		record.EndpointInstance = acquisition.instance
	}
	acquisition.mu.Unlock()
	published := acquisition.publish(record, endpoint)
	if !acknowledged {
		acquisition.retireOwned(endpoint)
		return fmt.Errorf("%w: readiness probe was not acknowledged", ErrTeslaGen3HSCActivationFailed)
	}
	if published.State != Gen3ReadEvidenceAck {
		return ErrTeslaGen3ReadAcquisitionClosed
	}
	return nil
}
