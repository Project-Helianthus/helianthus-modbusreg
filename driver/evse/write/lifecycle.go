package write

import (
	"context"
	"errors"
	"fmt"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/driver"
	"github.com/Project-Helianthus/helianthus-semreg/semreg/v1/operation"
)

// PathKey is the durable key of the source this writer declares for the
// allocated current of its connector.
const PathKey = "tesla.wc3.provisional_current_limit"

func (w *Writer) element(node driver.NodeRef) driver.Element {
	text := w.cfg.Publication.Connector
	return driver.Element{Subject: node, Key: semreg.FactKey{
		PackID: evsePack.ID, PackVersion: evsePack.Version, FactID: ArgumentID,
		Dimensions: []semreg.Dimension{{ID: DimensionID, Value: semreg.Value{Kind: semreg.ValueText, Text: &text}}},
	}}
}

func (w *Writer) route(node driver.NodeRef) driver.SettingRoute {
	p := w.cfg.Publication
	element := w.element(node)
	return driver.SettingRoute{
		For: element, Operation: definition(OperationID),
		Owner: driver.SourceRef{For: element, Producer: node, PathKey: PathKey},
		Route: operation.Route{
			CapabilityInstance: p.CapabilityInstanceIdentifier(w.capability()), ServiceInstance: p.ServiceInstanceIdentifier(w.capability()),
			BindingID: p.BindingIdentifier(), SourceID: p.Source, SourceEpochID: p.Epoch, DriverGeneration: w.generation(),
		},
	}
}

// Start declares the source of the allocated current of the connector and the
// setting route of the operation in its own transaction on the session of the
// reader, then registers the executor. It starts after the reader. Where the
// bound setter is not qualified for the build, nothing is declared and no
// executor is registered; the reads of the reader are unchanged.
func (w *Writer) Start(ctx context.Context, session driver.Session) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case ctx == nil || session == nil:
		return errors.New("charger writer requires a context and a session")
	case w.running || w.fenced.Load():
		return errors.New("charger writer already started or stopped")
	}
	w.session, w.running = session, true
	if !w.cfg.Reader.CurrentLimitWritable() {
		return nil
	}
	route := w.route(session.Native().Root())
	tx, err := session.Publisher().Begin(ctx)
	if err != nil {
		return fmt.Errorf("charger writer route: %w", err)
	}
	for _, stage := range []error{tx.DeclareSource(driver.Source{For: route.For, Producer: route.Owner.Producer, PathKey: PathKey}), tx.DeclareSettingRoute(route)} {
		if stage != nil {
			return fmt.Errorf("charger writer route: %w", errors.Join(stage, tx.Abort(ctx)))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("charger writer route: %w", err)
	}
	unregister, err := session.RegisterExecutor(route, w)
	if err != nil {
		return fmt.Errorf("charger writer executor: %w", err)
	}
	w.unregister = unregister
	return nil
}

// FenceWithdrawal ends every later send and renewal without blocking.
func (w *Writer) FenceWithdrawal() {
	w.fenced.Store(true)
	w.runCancel()
}

// Stop removes the executor, ends the renewal and drains it.
func (w *Writer) Stop(ctx context.Context) driver.CloseReport {
	w.FenceWithdrawal()
	w.mu.Lock()
	w.running = false
	w.disarm()
	unregister := w.unregister
	w.unregister = nil
	w.mu.Unlock()
	if unregister != nil {
		unregister()
	}
	drained := make(chan struct{})
	go func() { w.wg.Wait(); close(drained) }()
	select {
	case <-drained:
		return driver.CloseReport{Confirmed: true}
	case <-ctx.Done():
		return driver.CloseReport{Err: ctx.Err()}
	}
}
