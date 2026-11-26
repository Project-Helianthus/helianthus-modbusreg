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
	ErrGen3NotStarted = errors.New("charger reader is not started")
	ErrGen3Fenced     = errors.New("charger reader is stopped or fenced")
)

type publishedFact struct {
	fact Fact
	rev  uint64
}

// Gen3Reader is the running instance of one configured Gen3 charger. It owns
// one native read lifecycle, takes the polled t28 from it and the persistent
// t8, the prerequisite t28 and the mutation t28 from the callers that hold the
// native results, and publishes every accepted observation through the session.
type Gen3Reader struct {
	cfg  Gen3Config
	pub  Gen3Publication
	acq  *tesla.Gen3ReadAcquisition
	feed *Feed
	now  func() time.Time

	fenced atomic.Bool

	mu        sync.Mutex
	session   driver.Session
	started   bool
	seq       uint64
	semantic  uint64
	published map[semreg.CandidateID]publishedFact
	revisions map[semreg.CandidateID]uint64
	linked    bool
	cancel    context.CancelFunc
	done      chan struct{}
}

var _ driver.Instance = (*Gen3Reader)(nil)

// NewGen3Reader admits a configured block. An empty block starts nothing and
// returns no reader; a block missing a value reports it by name.
func NewGen3Reader(cfg Gen3Config, pub Gen3Publication, deps Gen3Deps) (*Gen3Reader, error) {
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
	return &Gen3Reader{cfg: cfg, pub: pub, acq: acq, feed: NewFeed(), now: now,
		published: map[semreg.CandidateID]publishedFact{}, revisions: map[semreg.CandidateID]uint64{}}, nil
}

// Acquisition is the running native lifecycle: the one endpoint, reservation
// and generation that the reads and the native setter share.
func (r *Gen3Reader) Acquisition() *tesla.Gen3ReadAcquisition { return r.acq }

// Feed is the readback state the reader publishes from.
func (r *Gen3Reader) Feed() *Feed { return r.feed }

// Setter reports whether the bound setter can run and, when it cannot, why.
func (r *Gen3Reader) Setter() SetterAvailability { return r.cfg.Setter() }

// CurrentLimitWritable is true only while the qualified t25 operation is.
func (r *Gen3Reader) CurrentLimitWritable() bool { return r.cfg.CurrentLimitWritable() }

// View evaluates the readback state now.
func (r *Gen3Reader) View() View { return r.feed.View(At{Wall: r.now()}) }

// Start activates the lifecycle once and runs its polls on the instance
// lifetime; it returns when the lifecycle is ready or has failed.
func (r *Gen3Reader) Start(ctx context.Context, session driver.Session) error {
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

func (r *Gen3Reader) poll(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		records, err := r.acq.PollOnce(ctx)
		for _, record := range records {
			_ = r.IngestPolled(ctx, record)
		}
		switch {
		case errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionClosed):
			return
		case errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionUnavailable):
			_ = r.acq.Restart(ctx)
		}
		_ = r.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// FenceWithdrawal stops every later publication without blocking.
func (r *Gen3Reader) FenceWithdrawal() {
	r.fenced.Store(true)
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Stop ends the polls and closes the lifecycle's endpoint.
func (r *Gen3Reader) Stop(ctx context.Context) driver.CloseReport {
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

func (r *Gen3Reader) correlated(record tesla.Gen3ReadRecord, operation tesla.FC100Operation) bool {
	return record.State == tesla.Gen3ReadEvidenceAck && record.Operation == operation &&
		record.Endpoint == r.cfg.Endpoint && record.Node == r.cfg.Node && record.RequestID != 0
}

func (r *Gen3Reader) provisional(record tesla.Gen3ReadRecord, anchor *tesla.Gen3ReadRecord) bool {
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
func (r *Gen3Reader) IngestPolled(ctx context.Context, record tesla.Gen3ReadRecord) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	if !r.provisional(record, nil) {
		return nil
	}
	return r.publish(ctx)
}

// IngestPersistent feeds the t8 of the native persistent-settings result. A
// duplicate, an obsolete generation, an uncorrelated or an unqualified result
// has no effect; the error is only a publication error.
func (r *Gen3Reader) IngestPersistent(ctx context.Context, result tesla.PersistentSettingsResult) error {
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
func (r *Gen3Reader) IngestSetResult(ctx context.Context, result tesla.ProvisionalCurrentLimitResult) error {
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

func (r *Gen3Reader) ready(ctx context.Context) error {
	if ctx == nil {
		return errors.New("charger reader requires a context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.fenced.Load() {
		return ErrGen3Fenced
	}
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if !started {
		return ErrGen3NotStarted
	}
	return nil
}

func (r *Gen3Reader) refresh(ctx context.Context) error {
	if err := r.ready(ctx); err != nil {
		return err
	}
	return r.publish(ctx)
}

// publish stages the difference between the view now and what was published,
// applying the session's operator links on every transaction, and commits it.
func (r *Gen3Reader) publish(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fenced.Load() {
		return ErrGen3Fenced
	}
	at := At{Wall: r.now()}
	view := r.feed.View(at)
	tx, err := r.session.Publisher().Begin(ctx)
	if err != nil {
		return err
	}
	batch, revisions := r.batch(view, at)
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
	for _, f := range view.Facts {
		id := r.pub.candidateID(f.Element, f.Role)
		next[id] = publishedFact{fact: f, rev: revisions[id]}
	}
	r.published = next
	r.feed.Published()
	return nil
}

func (r *Gen3Reader) ownLinks(pending []semreg.IdentityLink) []semreg.IdentityLink {
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
