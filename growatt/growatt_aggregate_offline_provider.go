package growatt

import (
	"context"
	"errors"
	"fmt"

	modbusreg "github.com/Project-Helianthus/helianthus-modbusreg"
)

// AggregateOfflineCycle separates local attempt correlation from native
// identity. Deadline uses the same monotonic logical ticks as the caller clock.
type AggregateOfflineCycle struct {
	Context         AggregateContext
	Cycle, Deadline uint64
}

// AggregateOfflineReader returns supplied offline records once per cycle.
// It must honor cancellation and the caller's work bound, and must not mutate
// returned slices until Cycle returns. This callback grants no transport access.
type AggregateOfflineReader func(context.Context, AggregateOfflineCycle) (BMSReadOnlyInput, error)

// AggregateOfflineProvider composes one retention owner and one reader.
// It has no timer, retry, diagnostic bank, registration or acquisition authority.
type AggregateOfflineProvider struct {
	retention           *AggregateRetention
	reader              AggregateOfflineReader
	maxSlices, maxWords uint64
}

// NewAggregateOfflineProvider requires explicit positive cycle and input
// bounds. maxWords bounds the total returned words, not each individual slice.
// Bounds constrain accepted work; a synchronous reader must itself cooperate.
func NewAggregateOfflineProvider(maxCycleTicks, maxSlices, maxWords uint64, reader AggregateOfflineReader) (*AggregateOfflineProvider, error) {
	if reader == nil || maxSlices == 0 || maxWords == 0 {
		return nil, fmt.Errorf("offline reader and positive input bounds are required")
	}
	r, err := NewAggregateRetention(maxCycleTicks)
	if err != nil {
		return nil, err
	}
	return &AggregateOfflineProvider{retention: r, reader: reader, maxSlices: maxSlices, maxWords: maxWords}, nil
}

// Cycle begins one attempt, calls the reader once and closes through Complete
// or Fail. clock is a bounded, concurrency-safe logical clock supplied by the
// caller. Superseded, expired and cancelled completions cannot renew success.
// Concurrent cycles may supersede each other; reader concurrency is caller-owned.
func (p *AggregateOfflineProvider) Cycle(ctx context.Context, cycle AggregateOfflineCycle, clock func() uint64) error {
	if p == nil || p.retention == nil || p.reader == nil || p.maxSlices == 0 || p.maxWords == 0 || ctx == nil || clock == nil {
		return fmt.Errorf("invalid offline provider or cycle callbacks")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.retention.Begin(cycle.Context, cycle.Cycle, clock(), cycle.Deadline); err != nil {
		return err
	}
	if c := cycle.Context; !c.IdentityKnown || c.UnitID == 0 || c.UnitID > 247 || c.Function != modbusreg.FunctionReadHoldingRegisters || c.Revision != growattBMSExactRevision() {
		return errors.Join(fmt.Errorf("offline cycle lacks supported native context"), p.retention.Fail(cycle.Context, cycle.Cycle, clock()))
	}
	// Begin may race with cancellation; do not start reader work after observing it.
	if err := ctx.Err(); err != nil {
		return errors.Join(err, p.retention.Fail(cycle.Context, cycle.Cycle, clock()))
	}
	input, readErr := p.reader(ctx, cycle)
	now := clock()
	if err := ctx.Err(); err != nil {
		return errors.Join(err, p.retention.Fail(cycle.Context, cycle.Cycle, now))
	}
	if readErr != nil {
		return errors.Join(readErr, p.retention.Fail(cycle.Context, cycle.Cycle, now))
	}
	if uint64(len(input.Slices)) > p.maxSlices {
		return errors.Join(fmt.Errorf("offline slice bound exceeded"), p.retention.Fail(cycle.Context, cycle.Cycle, now))
	}
	remaining := p.maxWords
	for _, s := range input.Slices {
		if uint64(len(s.Words)) > remaining {
			return errors.Join(fmt.Errorf("offline word bound exceeded"), p.retention.Fail(cycle.Context, cycle.Cycle, now))
		}
		remaining -= uint64(len(s.Words))
	}
	return p.retention.Complete(cycle.Context, cycle.Cycle, now, input)
}

// Advance closes expired pending work without invoking the reader.
func (p *AggregateOfflineProvider) Advance(now uint64) error {
	if p == nil || p.retention == nil {
		return fmt.Errorf("invalid offline provider")
	}
	return p.retention.Advance(now)
}

// Snapshot returns the retention owner's detached envelope without advancing time.
func (p *AggregateOfflineProvider) Snapshot() AggregateSnapshot {
	if p == nil || p.retention == nil {
		return AggregateSnapshot{State: "absent"}
	}
	return p.retention.Snapshot()
}
