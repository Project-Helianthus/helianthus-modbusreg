package write

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/driver/evse/reader"
	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

// Config is what the host supplies for the writer of one configured charger.
type Config struct {
	// Reader is the configured charger reader the writer wraps: its native
	// lifecycle, its feed and its publication.
	Reader *reader.ChargerReader
	// Publication is the publication the reader was built with.
	Publication reader.ChargerPublication
	// ClockEpoch identifies the monotonic axis of the writer's own evidence. It
	// is not the epoch of the endpoint's receipts and not the reader's.
	ClockEpoch semreg.ClockEpochID
	// Clock is the clock of the writer; nil is the system clock.
	Clock Clock
}

// Writer is the executor of the allocated current operation of one charger.
type Writer struct {
	cfg    Config
	acq    *tesla.Gen3ReadAcquisition
	clock  Clock
	origin time.Time
	fenced atomic.Bool
	wg     sync.WaitGroup
	runCtx context.Context
	// runCancel ends the renewals; it is safe to call from any goroutine.
	runCancel context.CancelFunc

	// mu is the lifecycle lock: one exchange runs at a time, and the guard is
	// checked under it immediately before each first send.
	mu         sync.Mutex
	session    driver.Session
	running    bool
	unregister func()
	attempts   uint64
	renewal    *renewal
	// id is the ownership of the session; before Start it is the one the reader
	// was configured with.
	id identity
}

var _ driver.Instance = (*Writer)(nil)

// NewWriter admits the configuration of a writer.
func NewWriter(cfg Config) (*Writer, error) {
	switch {
	case cfg.Reader == nil:
		return nil, errors.New("writer requires a configured charger reader")
	case cfg.Publication.Asset.Validate() != nil, cfg.Publication.Source.Validate() != nil, cfg.Publication.Epoch.Validate() != nil,
		cfg.Publication.Generation == 0, cfg.Publication.Connector == "":
		return nil, errors.New("writer requires the publication of its reader")
	case cfg.ClockEpoch.Validate() != nil:
		return nil, errors.New("writer requires a clock epoch")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	p := cfg.Publication
	id := identity{asset: p.Asset, source: p.Source, epoch: p.Epoch, generation: semreg.Uint64(strconv.FormatUint(p.Generation, 10))}
	return &Writer{cfg: cfg, acq: cfg.Reader.Acquisition(), clock: clock, origin: clock.Now(), runCtx: runCtx, runCancel: runCancel, id: id}, nil
}

func (w *Writer) capability() reader.Capability { return reader.SetAllocated }

// generation is the generation the reader was configured with; the session
// stamps its own on the route.
func (w *Writer) generation() semreg.Uint64 {
	return semreg.Uint64(strconv.FormatUint(w.cfg.Publication.Generation, 10))
}

// guardMatches compares the guard with the route of the provider as the session
// names it: the session's own ownership and the identifiers it scoped. The
// caller holds the lifecycle lock.
func (w *Writer) guardMatches(g operation.GuardClaims) bool {
	p, id := w.cfg.Publication, w.id
	return g.AssetID == id.asset && scopedAs(p.BindingIdentifier(), g.BindingID) && g.SourceID == id.source && g.SourceEpochID == id.epoch &&
		g.DriverGeneration == id.generation && scopedAs(p.ServiceInstanceIdentifier(w.capability()), g.ServiceInstance) &&
		scopedAs(p.CapabilityInstanceIdentifier(w.capability()), g.CapabilityInstance)
}

// admit answers, before any send, whether the writer may send for the guard. The
// caller holds the lifecycle lock.
func (w *Writer) admit(ctx context.Context, guard operation.GuardClaims) string {
	switch {
	case !w.running || w.fenced.Load():
		return ReasonStopped
	case !w.guardMatches(guard):
		return ReasonGuard
	case !w.cfg.Reader.CurrentLimitWritable():
		return ReasonNotWritable
	case w.acq.State() != tesla.Gen3ReadLifecycleReady:
		return ReasonNotReady
	case ctx.Err() != nil:
		return ReasonCancelled
	}
	return ""
}

// ApplyAllocatedCurrent performs one native provisional set for an amperes
// value: 0 A pauses, a whole number from 6 A up to the configured current of
// the latest t28 sets a limit. Any other value, a guard of another epoch or
// generation and a lifecycle that is not ready are answered before anything is
// sent. A value equal to the configured current is an ordinary set; release is
// never selected from a number.
func (w *Writer) ApplyAllocatedCurrent(ctx context.Context, guard operation.GuardClaims, amps semreg.Quantity) Result {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx == nil {
		return refused(ReasonCancelled)
	}
	if reason := w.admit(ctx, guard); reason != "" {
		return refused(reason)
	}
	n, reason := wholeAmps(amps)
	switch {
	case reason != "":
		return refused(reason)
	case n > 0 && n < FloorAmps:
		return refused(ReasonBelowFloor)
	case n == 0:
		pause := tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 0, LimitTimeoutSeconds: LimitTimeoutSeconds, InhibitCharging: true}
		return w.held(guard, pause, w.send(ctx, guard, pause, 0, Result{}, true))
	}
	configured, pre := w.configuredCurrent(ctx, guard)
	if pre.Reason != "" {
		return pre
	}
	if n > configured {
		pre.Level, pre.Reason = LevelFailedNoContact, ReasonAboveConfig
		return pre
	}
	request := tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: n, LimitTimeoutSeconds: LimitTimeoutSeconds}
	return w.held(guard, request, w.send(ctx, guard, request, n, pre, true))
}

// configuredCurrent is the configured current of the latest t28 of the
// generation; with none, the t27 → t28 read that learns it. A refusal carries
// its reason in the returned result; the read, when made, is in it either way.
func (w *Writer) configuredCurrent(ctx context.Context, guard operation.GuardClaims) (uint32, Result) {
	var pre Result
	var record tesla.Gen3ReadRecord
	if retained, ok := w.acq.Retained(tesla.FC100OperationWCGetProvisional); ok && retained.HasLastGood && retained.Current {
		record = retained.LastGood
	} else {
		if reason := w.admit(ctx, guard); reason != "" {
			return 0, refused(reason)
		}
		read, err := w.acq.Acquire(ctx, tesla.FC100OperationWCGetProvisional)
		if err != nil {
			return 0, refused(ReasonPrerequisite + ": " + err.Error())
		}
		pre.Prerequisite, pre.HasPrerequisite = read, true
		pre.PublicationErr = w.cfg.Reader.IngestPolled(ctx, read)
		record = read
	}
	readback, ok := record.ProvisionalReadback()
	switch {
	case !ok:
		pre.Level, pre.Reason = LevelFailedNoContact, ReasonPrerequisite
	case !readback.HasConfiguredCurrent || readback.ConfiguredCurrentAmps == 0:
		pre.Level, pre.Reason = LevelFailedNoContact, ReasonNoConfigured
	default:
		pre.Configured, pre.HasConfigured = readback.ConfiguredCurrentAmps, true
		return readback.ConfiguredCurrentAmps, pre
	}
	return 0, pre
}

// send makes the one native exchange of a command and answers with its
// evidence. The caller holds the lifecycle lock and has admitted the guard.
func (w *Writer) send(ctx context.Context, guard operation.GuardClaims, request tesla.ProvisionalCurrentLimitRequest, want uint32, pre Result, snapshot bool) Result {
	if reason := w.admit(ctx, guard); reason != "" {
		pre.Level, pre.Reason = LevelFailedNoContact, reason
		return pre
	}
	started := w.clock.Now()
	native, err := w.acq.SetProvisionalCurrentLimit(ctx, request)
	r := pre
	r.Native, r.Started = native, started
	r.PublicationErr = errors.Join(r.PublicationErr, w.cfg.Reader.IngestSetResult(ctx, native))
	r.Sent, r.HasSent = native.Encoded, native.HasEncoded
	switch {
	case native.PossiblyTransmitted && !native.HasEncoded:
		r.Level, r.Reason = LevelFailedNoContact, ReasonPrerequisite
		if native.Reason != "" {
			r.Reason += ": " + native.Reason
		}
		return r
	case !native.PossiblyTransmitted:
		r.Level, r.Reason = LevelFailedNoContact, ReasonSetRefused
		if err != nil {
			r.Reason += ": " + err.Error()
		}
		return r
	case native.Set.State == tesla.Gen3ReadEvidenceRejected:
		r.Level, r.Reason = LevelNoEffect, ReasonRejected
		return r
	case !native.Acknowledged:
		r.Level, r.Reason = LevelUnknown, ReasonNoAcknowledge
		return r
	}
	r.AckAt = native.Set.ReceiptWall
	r.Dispatched = sentWall(native.Set)
	readback, ok := native.Readback.ProvisionalReadback()
	if !ok || native.Outcome != tesla.Gen3ReadEvidenceAck {
		r.Level, r.Reason = LevelProtocolAck, ReasonNoReadback
		return r
	}
	r.Returned, r.HasReturned = readback, true
	r.Matches = native.Comparison == tesla.CurrentLimitMatch
	r.Published, r.HasPublished = publishedOf(readback)
	r.Level = LevelProtocolAck
	if snapshot && (r.Matches || (r.HasPublished && r.Published != want)) {
		if snapshot, candidate, ok := w.later(ctx, native.Set); ok {
			r.Snapshot, r.Candidate = &snapshot, &candidate
			r.Level = LevelConflict
			if r.Matches {
				r.Level = LevelReadbackConfirmed
			}
		}
	}
	return r
}

// sentWall is the wall time at which the t25 request was written: its
// acknowledgement's wall receipt less the exchange's own duration.
func sentWall(set tesla.Gen3ReadRecord) time.Time {
	if set.ReceivedAt >= set.SentAt {
		return set.ReceiptWall.Add(-(set.ReceivedAt - set.SentAt))
	}
	return set.ReceiptWall
}

// later is the exact later snapshot that carries the allocated current
// candidate published from this exchange's acknowledgement.
func (w *Writer) later(ctx context.Context, set tesla.Gen3ReadRecord) (semreg.Snapshot, semreg.FactCandidate, bool) {
	input := w.session.SemanticInput()
	if input == nil {
		return semreg.Snapshot{}, semreg.FactCandidate{}, false
	}
	cut, err := input.Capture(ctx, driver.Filter{Assets: []semreg.AssetID{w.id.asset}})
	if err != nil || len(cut.Snapshots) == 0 {
		return semreg.Snapshot{}, semreg.FactCandidate{}, false
	}
	id := w.cfg.Publication.ConnectorCandidateIdentifier(reader.ElementAllocatedCurrent)
	want := semreg.Int64(strconv.FormatInt(set.ReceiptWall.UTC().UnixNano(), 10))
	for _, snapshot := range cut.Snapshots {
		for _, envelope := range snapshot.Facts {
			for _, candidate := range envelope.Candidates {
				if scopedAs(id, candidate.CandidateID) && candidate.Times.ReceivedAt.UnixNanoseconds == want {
					return snapshot, candidate, true
				}
			}
		}
	}
	return semreg.Snapshot{}, semreg.FactCandidate{}, false
}
