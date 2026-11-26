package reader

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
)

// Errors of the Gen3 reader.
var (
	ErrChargerNotStarted = errors.New("charger reader is not started")
	ErrChargerFenced     = errors.New("charger reader is stopped or fenced")
)

type publishedFact struct {
	fact Fact
	rev  uint64
}

// ChargerReader is the running instance of one configured Gen3 charger. It owns
// one native read lifecycle, takes the polled t28 from it and the persistent
// t8, the prerequisite t28 and the mutation t28 from the callers that hold the
// native results, and publishes every accepted observation through the session.
type ChargerReader struct {
	cfg    ChargerConfig
	pub    ChargerPublication
	acq    *tesla.Gen3ReadAcquisition
	feed   *Feed
	vitals *VitalsFeed
	now    func() time.Time
	plan   GraphPlan
	graph  graphRecords

	fenced atomic.Bool

	mu        sync.Mutex
	session   driver.Session
	started   bool
	seq       uint64
	semantic  uint64
	published map[semreg.CandidateID]publishedFact
	revisions map[semreg.CandidateID]uint64
	linked    bool
	pubErr    error
	cancel    context.CancelFunc
	done      chan struct{}
}

var _ driver.Instance = (*ChargerReader)(nil)

// NewChargerReader admits a configured block. An empty block starts nothing and
// returns no reader; a block missing a value reports it by name.
func NewChargerReader(cfg ChargerConfig, pub ChargerPublication, deps ChargerDeps) (*ChargerReader, error) {
	if cfg.IsEmpty() {
		return nil, nil
	}
	if err := cfg.Admit(); err != nil {
		return nil, err
	}
	for field, bad := range map[string]bool{
		"publication.asset": pub.Asset.Validate() != nil, "publication.source": pub.Source.Validate() != nil,
		"publication.epoch": pub.Epoch.Validate() != nil, "publication.generation": pub.Generation == 0,
		"publication.clock_epoch": pub.ClockEpoch.Validate() != nil, "publication.evse": pub.EVSE == "",
		"publication.connector": pub.Connector == "", "publication.policy": pub.Policy.PolicyID.Validate() != nil,
	} {
		if bad {
			return nil, &ConfigError{field, "missing or invalid"}
		}
	}
	var plan GraphPlan
	var graph graphRecords
	if pub.Graph.IsZero() {
		pub.Graph = GraphConfig{}
	} else {
		if err := pub.Graph.Validate(); err != nil {
			return nil, &ConfigError{"publication.graph", err.Error()}
		}
		if err := pub.Graph.contains(pub.EVSE, pub.Connector); err != nil {
			return nil, &ConfigError{"publication.graph", err.Error()}
		}
		plan = pub.Graph.Plan()
		graph = pub.graphRecords(plan)
		for _, resource := range graph.resources {
			if err := resource.Validate(); err != nil {
				return nil, &ConfigError{"publication.graph", err.Error()}
			}
		}
		for _, relation := range graph.relations {
			if err := relation.Validate(); err != nil {
				return nil, &ConfigError{"publication.graph", err.Error()}
			}
		}
	}
	acqConfig, err := cfg.acquisitionConfig(deps)
	if err != nil {
		return nil, err
	}
	acq, err := tesla.NewGen3ReadAcquisition(acqConfig)
	if err != nil {
		return nil, &ConfigError{"lifecycle", err.Error()}
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &ChargerReader{cfg: cfg, pub: pub, acq: acq, feed: NewFeed(), vitals: NewVitalsFeed(), now: now, plan: plan, graph: graph,
		published: map[semreg.CandidateID]publishedFact{}, revisions: map[semreg.CandidateID]uint64{}}, nil
}

// Acquisition is the running native lifecycle: the one endpoint, reservation
// and generation that the reads and the native setter share.
func (r *ChargerReader) Acquisition() *tesla.Gen3ReadAcquisition { return r.acq }

// Graph is the configured graph the reader publishes with its first batch:
// resources and relations in the order the configuration gives, connectors by
// the numeric value of their slot. It is empty when no graph was configured.
func (r *ChargerReader) Graph() GraphPlan { return r.plan }

// UnknownRelations lists, with the reason, the relations of the graph that the
// device does not support. The kernel records an unknown relation as the
// absence of both an instance and an absence statement, so none is staged for
// them; the reason is stated here.
func (r *ChargerReader) UnknownRelations() []UnknownRelation {
	return append([]UnknownRelation(nil), r.plan.Unknown...)
}

// Feed is the readback state the reader publishes from.
func (r *ChargerReader) Feed() *Feed { return r.feed }

// Setter reports whether the bound setter can run and, when it cannot, why.
func (r *ChargerReader) Setter() SetterAvailability { return r.cfg.Setter() }

// CurrentLimitWritable is true only while the qualified t25 operation is.
func (r *ChargerReader) CurrentLimitWritable() bool { return r.cfg.CurrentLimitWritable() }

// View evaluates the readback state now.
func (r *ChargerReader) View() View { return r.view(At{Wall: r.now()}) }

// Start activates the lifecycle once and runs its polls on the instance
// lifetime; it returns when the lifecycle is ready or has failed.
func (r *ChargerReader) Start(ctx context.Context, session driver.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.fenced.Load() {
		return errors.New("charger reader already started or stopped")
	}
	if session == nil {
		return errors.New("charger reader requires a session")
	}
	if err := r.acq.Start(ctx); err != nil {
		_ = r.acq.Close()
		r.fenced.Store(true)
		return fmt.Errorf("charger lifecycle: %w", redactSerial(err))
	}
	run, cancel := context.WithCancel(context.Background())
	r.session, r.started, r.cancel, r.done = session, true, cancel, make(chan struct{})
	go r.poll(run, r.done)
	return nil
}

func redactSerial(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("serial %s: %w", pathErr.Op, pathErr.Err)
	}
	return err
}

func (r *ChargerReader) poll(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		records, err := r.acq.PollOnce(ctx)
		for _, record := range records {
			if err := r.IngestPolled(ctx, record); err != nil {
				r.notePublication(err)
			}
		}
		switch {
		case errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionClosed):
			return
		case errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionUnavailable):
			_ = r.acq.Restart(ctx)
		}
		r.notePublication(r.refresh(ctx))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// notePublication keeps the latest publication failure of the poll loop, and a
// refresh that publishes cleanly clears it. A stop or a fence is not a failure.
func (r *ChargerReader) notePublication(err error) {
	if errors.Is(err, ErrChargerFenced) || errors.Is(err, context.Canceled) {
		return
	}
	r.mu.Lock()
	r.pubErr = err
	r.mu.Unlock()
}

// PublicationError is the latest failure to begin, stage or commit a
// publication from the running polls, nil when the last one went through. It
// is distinct from the native outcome of any read.
func (r *ChargerReader) PublicationError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pubErr
}

// FenceWithdrawal stops every later publication without blocking.
func (r *ChargerReader) FenceWithdrawal() {
	r.fenced.Store(true)
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Stop ends the polls and closes the lifecycle's endpoint.
func (r *ChargerReader) Stop(ctx context.Context) driver.CloseReport {
	r.FenceWithdrawal()
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return driver.CloseReport{Err: ctx.Err()}
		}
	}
	if err := r.acq.Close(); err != nil {
		return driver.CloseReport{Err: redactSerial(err)}
	}
	return driver.CloseReport{Confirmed: true}
}

func receiptOf(record tesla.Gen3ReadRecord) Receipt {
	return Receipt{ClockEpoch: record.ClockEpoch, Wall: record.ReceiptWall, Mono: record.ReceivedAt,
		Lifecycle: record.LifecycleEpoch, Instance: record.EndpointInstance, Generation: record.Generation, RequestID: record.RequestID}
}

func (r *ChargerReader) correlated(record tesla.Gen3ReadRecord, operation tesla.FC100Operation) bool {
	return record.State == tesla.Gen3ReadEvidenceAck && record.Operation == operation &&
		record.Endpoint == r.cfg.Endpoint && record.Node == r.cfg.Node && record.RequestID != 0
}

func (r *ChargerReader) provisional(record tesla.Gen3ReadRecord, anchor *tesla.Gen3ReadRecord) bool {
	if !r.correlated(record, tesla.FC100OperationWCGetProvisional) {
		return false
	}
	readback, ok := record.ProvisionalReadback()
	if !ok {
		return false
	}
	observation := ProvisionalObservation{Readback: readback, Receipt: receiptOf(record), Anchor: receiptOf(record)}
	if anchor != nil {
		observation.Anchor = receiptOf(*anchor)
	}
	return r.feed.Provisional(observation)
}

// IngestPolled feeds one polled t28; its timeout runs from its own receipt.
func (r *ChargerReader) IngestPolled(ctx context.Context, record tesla.Gen3ReadRecord) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	if !r.provisional(record, nil) && !r.ingestVitals(record) {
		return nil
	}
	return r.publish(ctx)
}

// IngestPersistent feeds the t8 of the native persistent-settings result. A
// duplicate, an obsolete generation, an uncorrelated or an unqualified result
// has no effect; the error is only a publication error.
func (r *ChargerReader) IngestPersistent(ctx context.Context, result tesla.PersistentSettingsResult) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	receipt := result.Receipt
	if result.Duplicate || result.NoSend || !result.Current || !result.HasReturnedCurrent || result.ReturnedMaxOutputCurrentAmps < 0 ||
		result.Outcome != tesla.Gen3ReadEvidenceAck || !r.correlated(receipt, tesla.FC100OperationWCConfigureSettings) {
		return nil
	}
	if !r.feed.Persistent(PersistentObservation{MaxOutputCurrentAmps: uint32(result.ReturnedMaxOutputCurrentAmps), Receipt: receiptOf(receipt)}) {
		return nil
	}
	return r.publish(ctx)
}

// IngestSetResult feeds the t28 records of the native setter's exchange: the
// prerequisite read, which anchors on its own receipt, and the mutation
// readback, which anchors on the correlated set acknowledgement. The returned
// values are fed even when they differ from the request.
func (r *ChargerReader) IngestSetResult(ctx context.Context, result tesla.ProvisionalCurrentLimitResult) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	changed := r.provisional(result.Prerequisite, nil)
	var anchor *tesla.Gen3ReadRecord
	if result.Acknowledged && r.correlated(result.Set, tesla.FC100OperationWCSetProvisional) &&
		result.Set.Generation != 0 && result.Set.Generation == result.Readback.Generation &&
		result.Set.EndpointInstance == result.Readback.EndpointInstance && result.Set.LifecycleEpoch == result.Readback.LifecycleEpoch {
		anchor = &result.Set
	}
	if r.provisional(result.Readback, anchor) {
		changed = true
	}
	if !changed {
		return nil
	}
	return r.publish(ctx)
}

func (r *ChargerReader) ready(ctx context.Context) error {
	if ctx == nil {
		return errors.New("charger reader requires a context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.fenced.Load() {
		return ErrChargerFenced
	}
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if !started {
		return ErrChargerNotStarted
	}
	return nil
}

func (r *ChargerReader) refresh(ctx context.Context) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	return r.publish(ctx)
}

// publish stages the difference between the view now and what was published,
// applying the session's operator links on every transaction, and commits it.
func (r *ChargerReader) publish(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fenced.Load() {
		return ErrChargerFenced
	}
	at := At{Wall: r.now()}
	view := r.view(at)
	tx, err := r.session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	batch, revisions, items := r.batch(view, at)
	pending := tx.OperatorLinks(batch.BindingUpserts)
	batch.IdentityLinkUpserts = carryLinks(batch.IdentityLinkUpserts, r.ownLinks(pending))
	if batchEmpty(batch) {
		return tx.Abort(ctx)
	}
	digest, err := batch.ComputedDigest()
	if err == nil {
		batch.BatchDigest = digest
		err = tx.Stage(batch)
	}
	if err != nil {
		return errors.Join(err, tx.Abort(ctx))
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.seq++
	r.semantic++
	r.linked = true
	r.revisions = revisions
	next := map[semreg.CandidateID]publishedFact{}
	for _, f := range items {
		id := r.pub.candidateID(f.Element, f.Role)
		next[id] = publishedFact{fact: f, rev: revisions[id]}
	}
	r.published = next
	r.feed.Published()
	return nil
}

func (r *ChargerReader) ownLinks(pending []semreg.IdentityLink) []semreg.IdentityLink {
	var own []semreg.IdentityLink
	for _, link := range pending {
		if link.BindingID == r.pub.bindingID() {
			own = append(own, link)
		}
	}
	return own
}

func carryLinks(built, pending []semreg.IdentityLink) []semreg.IdentityLink {
	if len(pending) == 0 {
		return built
	}
	replaced := map[semreg.NativeBindingID]bool{}
	for _, link := range pending {
		replaced[link.BindingID] = true
	}
	var out []semreg.IdentityLink
	for _, link := range built {
		if !replaced[link.BindingID] {
			out = append(out, link)
		}
	}
	return append(out, pending...)
}
