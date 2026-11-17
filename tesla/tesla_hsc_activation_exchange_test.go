package tesla

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// Synthetic activation reports. Tokens come from the package accessors.
// No serial port is opened.

func hscActivationEvent(kind HSCActivationEventKind, payload []byte) HSCActivationEvent {
	event := HSCActivationEvent{
		Kind:           kind,
		Bytes:          append([]byte(nil), payload...),
		LastByteOffset: time.Millisecond,
	}
	if kind == HSCActivationEventEcho {
		event.Turnaround = time.Millisecond
		event.WriteDuration = time.Millisecond
	}
	return event
}

func hscActivationSuccess() HSCActivationReport {
	greeting := HSCActivationGreeting()
	return HSCActivationReport{
		ClockEpoch: "synthetic",
		Events: []HSCActivationEvent{
			hscActivationEvent(HSCActivationEventGreeting, greeting),
			hscActivationEvent(HSCActivationEventEcho, greeting),
			hscActivationEvent(HSCActivationEventConfirmation, HSCActivationConfirmation()),
		},
	}
}

func hscActivationRefusalsThenSuccess(refusals int, echoOnRefusal bool) HSCActivationReport {
	greeting := HSCActivationGreeting()
	refusal := HSCActivationRefusal()
	events := make([]HSCActivationEvent, 0, refusals*3+3)
	for range refusals {
		events = append(events, hscActivationEvent(HSCActivationEventGreeting, greeting))
		if echoOnRefusal {
			events = append(events, hscActivationEvent(HSCActivationEventEcho, greeting))
		}
		events = append(events, hscActivationEvent(HSCActivationEventRefusal, refusal))
	}
	events = append(events,
		hscActivationEvent(HSCActivationEventGreeting, greeting),
		hscActivationEvent(HSCActivationEventEcho, greeting),
		hscActivationEvent(HSCActivationEventConfirmation, HSCActivationConfirmation()),
	)
	return HSCActivationReport{ClockEpoch: "synthetic", Events: events}
}

func hscPrepare(t *testing.T, probe bool, steps []teslaGen3WitnessStep, seed *teslaGen3FakeStep) *teslaGen3ReadHarness {
	t.Helper()
	harness := &teslaGen3ReadHarness{clock: &teslaGen3FakeClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}}
	config := teslaGen3ReadTestConfig(t, harness)
	config.ReadinessProbe = probe
	config.Operations = []FC100Operation{FC100OperationWCGetVitals}
	config.Windows = map[FC100Operation]Gen3ReadWindow{
		FC100OperationWCGetVitals: {Freshness: time.Minute, Retention: time.Hour},
	}
	if steps != nil {
		harness.witness.steps = steps
	}
	open := config.Open
	config.Open = func(production modbus.RTUProductionConfig) (Gen3ReadEndpoint, error) {
		endpoint, err := open(production)
		if err != nil || seed == nil {
			return endpoint, err
		}
		if fake, ok := endpoint.(*teslaGen3FakeEndpoint); ok {
			fake.push(*seed)
		}
		return endpoint, nil
	}
	acquisition, err := NewGen3ReadAcquisition(config)
	if err != nil {
		t.Fatal(err)
	}
	harness.acquisition = acquisition
	harness.profile = config.Profile
	t.Cleanup(func() { _ = acquisition.Close() })
	return harness
}

func hscVitalsTerminal(t *testing.T) teslaGen3FakeStep {
	t.Helper()
	return teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01})}
}

func TestTeslaHSCActivationExchangeOpensEpoch(t *testing.T) {
	greeting := HSCActivationGreeting()
	greeting[0] ^= 0xff
	if bytes.Equal(greeting, HSCActivationGreeting()) || !bytes.Equal(HSCActivationGreeting(), []byte("TESLA\x00")) {
		t.Fatal("greeting accessor is not an independent copy of the token")
	}
	pass := HSCActivationConfirmation()
	pass[0] ^= 0xff
	if bytes.Equal(pass, HSCActivationConfirmation()) || !bytes.Equal(HSCActivationConfirmation(), []byte("PASS\x00")) {
		t.Fatal("confirmation accessor is not an independent copy of the token")
	}
	fail := HSCActivationRefusal()
	fail[0] ^= 0xff
	if bytes.Equal(fail, HSCActivationRefusal()) || !bytes.Equal(HSCActivationRefusal(), []byte("FAIL\x00")) {
		t.Fatal("refusal accessor is not an independent copy of the token")
	}

	report := hscActivationSuccess()
	report.Events[0].Turnaround = 5 * time.Second
	report.Events[0].WriteDuration = 5 * time.Second
	harness := hscPrepare(t, false, []teslaGen3WitnessStep{{report: report}}, nil)
	if harness.acquisition.ActivationOpening() != nil {
		t.Fatal("opening is present before activation")
	}
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if harness.acquisition.State() != Gen3ReadLifecycleReady || harness.witness.calls() != 1 {
		t.Fatalf("state/calls = %s/%d", harness.acquisition.State(), harness.witness.calls())
	}
	request := harness.witness.lastRequest
	endpoint := harness.endpoint()
	if request.Endpoint != "evse-a" || request.Epoch != 1 || request.Owned != endpoint {
		t.Fatalf("driver request = %+v", request)
	}
	opening := harness.acquisition.ActivationOpening()
	if opening == nil || opening.Epoch != 1 || opening.Basis != HSCActivationBasisDriverExchange || opening.Report.ClockEpoch != "synthetic" {
		t.Fatalf("opening = %+v", opening)
	}
	if len(opening.Report.Events) != 3 || opening.Report.Events[0].Turnaround != 0 || opening.Report.Events[0].WriteDuration != 0 ||
		opening.Report.Events[1].Turnaround != time.Millisecond || opening.Report.Events[1].WriteDuration != time.Millisecond {
		t.Fatalf("retained events = %+v", opening.Report.Events)
	}
	opening.Report.Events[1].Bytes[0] ^= 0xff
	again := harness.acquisition.ActivationOpening()
	if again == nil || bytes.Equal(again.Report.Events[1].Bytes, opening.Report.Events[1].Bytes) {
		t.Fatal("opening bytes are aliased to the caller")
	}
	endpoint.push(hscVitalsTerminal(t))
	record, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || record.State != Gen3ReadEvidenceAck || record.LifecycleEpoch != 1 || record.EndpointInstance == 0 || record.Generation != 1 {
		t.Fatalf("record = %+v, %v", record, err)
	}
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || !retained.Current || retained.LastGood.LifecycleEpoch != 1 || retained.LastGood.EndpointInstance != record.EndpointInstance || retained.LastGood.Generation != 1 {
		t.Fatalf("retained = %+v", retained)
	}
}

func TestTeslaHSCActivationRecoveryKeepsEpoch(t *testing.T) {
	harness := hscPrepare(t, false, nil, nil)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	opening := harness.acquisition.ActivationOpening()
	if opening == nil || opening.Epoch == 0 {
		t.Fatal("activation did not open an epoch")
	}
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{outcome: "write_fault"})
	endpoint.push(hscVitalsTerminal(t))
	record, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || record.State != Gen3ReadEvidenceAck || record.Attempt != 2 || record.LifecycleEpoch != opening.Epoch || record.Generation != 2 {
		t.Fatalf("recovered record = %+v, %v", record, err)
	}
	if harness.witness.calls() != 1 || endpoint.recovers != 1 || harness.acquisition.State() != Gen3ReadLifecycleReady {
		t.Fatalf("calls/recovers/state = %d/%d/%s", harness.witness.calls(), endpoint.recovers, harness.acquisition.State())
	}
	kept := harness.acquisition.ActivationOpening()
	if kept == nil || kept.Epoch != opening.Epoch || kept.Basis != opening.Basis {
		t.Fatalf("opening after recovery = %+v", kept)
	}
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || !retained.Current || retained.LastGood.Generation != 2 || retained.LastGood.LifecycleEpoch != opening.Epoch {
		t.Fatalf("retained after recovery = %+v", retained)
	}

	failed := hscPrepare(t, false, nil, nil)
	if err := failed.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	failedEpoch := failed.acquisition.ActivationOpening().Epoch
	failedEndpoint := failed.endpoint()
	failedEndpoint.recoverErr = errors.New("not quiet")
	failedEndpoint.push(teslaGen3FakeStep{outcome: "timeout"})
	record, err = failed.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || record.State != Gen3ReadEvidenceTimeout || failed.acquisition.State() != Gen3ReadLifecycleUnavailable ||
		failed.witness.calls() != 1 || failed.acquisition.ActivationOpening().Epoch != failedEpoch {
		t.Fatalf("failed recovery = %+v, %v, %s, calls %d", record, err, failed.acquisition.State(), failed.witness.calls())
	}
}

func TestTeslaHSCActivationAcceptsReportShapes(t *testing.T) {
	cases := map[string]HSCActivationReport{
		"refusal then confirmation": hscActivationRefusalsThenSuccess(1, true),
		"repeated refusals":         hscActivationRefusalsThenSuccess(3, true),
		"refusal without echo":      hscActivationRefusalsThenSuccess(2, false),
		"at event ceiling":          hscActivationRefusalsThenSuccess((maxHSCActivationReportEvents-3)/2, false),
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			harness := hscPrepare(t, true, []teslaGen3WitnessStep{{report: report}}, &teslaGen3FakeStep{
				terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01}),
			})
			if err := harness.acquisition.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			opening := harness.acquisition.ActivationOpening()
			if harness.acquisition.State() != Gen3ReadLifecycleReady || opening == nil || opening.Epoch != 1 ||
				opening.Basis != HSCActivationBasisDriverExchange || harness.endpoint().exchanges() != 0 || harness.witness.calls() != 1 {
				t.Fatalf("state/opening/exchanges = %s/%+v/%d", harness.acquisition.State(), opening, harness.endpoint().exchanges())
			}
			if len(report.Events) > maxHSCActivationReportEvents || len(opening.Report.Events) != len(report.Events) {
				t.Fatalf("events = %d retained %d", len(report.Events), len(opening.Report.Events))
			}
		})
	}
}

func TestTeslaHSCActivationRejectsReportShape(t *testing.T) {
	greeting := HSCActivationGreeting()
	confirmation := HSCActivationConfirmation()
	refusal := HSCActivationRefusal()
	differing := append([]byte(nil), greeting...)
	differing[0] ^= 0xff
	unknown := []byte{'H', 'E', 'L', 'L', 'O', 0}
	cases := map[string]HSCActivationReport{
		"confirmation without echo": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventGreeting, greeting),
				hscActivationEvent(HSCActivationEventConfirmation, confirmation),
			},
		},
		"final refusal": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventGreeting, greeting),
				hscActivationEvent(HSCActivationEventEcho, greeting),
				hscActivationEvent(HSCActivationEventRefusal, refusal),
			},
		},
		"final greeting": {
			ClockEpoch: "synthetic",
			Events:     []HSCActivationEvent{hscActivationEvent(HSCActivationEventGreeting, greeting)},
		},
		"differing echo": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventGreeting, greeting),
				hscActivationEvent(HSCActivationEventEcho, differing),
				hscActivationEvent(HSCActivationEventConfirmation, confirmation),
			},
		},
		"unknown bytes": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventGreeting, unknown),
				hscActivationEvent(HSCActivationEventEcho, unknown),
				hscActivationEvent(HSCActivationEventConfirmation, confirmation),
			},
		},
		"wrong order": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventEcho, greeting),
				hscActivationEvent(HSCActivationEventGreeting, greeting),
				hscActivationEvent(HSCActivationEventConfirmation, confirmation),
			},
		},
		"event after confirmation": {
			ClockEpoch: "synthetic",
			Events: []HSCActivationEvent{
				hscActivationEvent(HSCActivationEventGreeting, greeting),
				hscActivationEvent(HSCActivationEventEcho, greeting),
				hscActivationEvent(HSCActivationEventConfirmation, confirmation),
				hscActivationEvent(HSCActivationEventRefusal, refusal),
			},
		},
		"too many events": hscActivationRefusalsThenSuccess((maxHSCActivationReportEvents-1)/2, false),
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "too many events" && len(report.Events) <= maxHSCActivationReportEvents {
				t.Fatalf("fixture has %d events, ceiling %d", len(report.Events), maxHSCActivationReportEvents)
			}
			harness := hscPrepare(t, true, []teslaGen3WitnessStep{{report: report}}, &teslaGen3FakeStep{
				terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01}),
			})
			err := harness.acquisition.Start(context.Background())
			if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.acquisition.State() != Gen3ReadLifecycleUnavailable ||
				harness.acquisition.ActivationOpening() != nil || harness.endpoint().exchanges() != 0 {
				t.Fatalf("start = %v state %s exchanges %d", err, harness.acquisition.State(), harness.endpoint().exchanges())
			}
		})
	}
}

func TestTeslaHSCActivationRejectsLateAndCancelledReport(t *testing.T) {
	t.Run("at deadline", func(t *testing.T) {
		var harness *teslaGen3ReadHarness
		harness = hscPrepare(t, true, []teslaGen3WitnessStep{{
			before: func() { harness.clock.advance(time.Second) },
			report: hscActivationSuccess(),
		}}, &teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01})})
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.acquisition.State() != Gen3ReadLifecycleUnavailable ||
			harness.endpoint().exchanges() != 0 || harness.acquisition.ActivationOpening() != nil {
			t.Fatalf("late start = %v state %s exchanges %d", err, harness.acquisition.State(), harness.endpoint().exchanges())
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		harness := hscPrepare(t, true, []teslaGen3WitnessStep{{report: hscActivationSuccess()}}, &teslaGen3FakeStep{
			terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01}),
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := harness.acquisition.Start(ctx)
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.acquisition.State() != Gen3ReadLifecycleUnavailable ||
			harness.endpoint().exchanges() != 0 {
			t.Fatalf("cancelled start = %v state %s exchanges %d", err, harness.acquisition.State(), harness.endpoint().exchanges())
		}
	})
}

func TestTeslaHSCActivationRestartAfterObservation(t *testing.T) {
	harness := hscPrepare(t, false, nil, nil)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := harness.endpoint()
	first.push(hscVitalsTerminal(t))
	good, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || good.State != Gen3ReadEvidenceAck {
		t.Fatal(err)
	}
	before := harness.acquisition.ActivationOpening()
	if err := harness.acquisition.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := harness.endpoint()
	after := harness.acquisition.ActivationOpening()
	if second == first || !first.isClosed() || harness.opens != 2 || harness.witness.calls() != 1 || second.exchanges() != 0 {
		t.Fatalf("opens/calls/exchanges = %d/%d/%d", harness.opens, harness.witness.calls(), second.exchanges())
	}
	if harness.acquisition.State() != Gen3ReadLifecycleReady || after == nil || after.Epoch != before.Epoch || after.Basis != before.Basis ||
		!bytes.Equal(after.Report.Events[1].Bytes, before.Report.Events[1].Bytes) {
		t.Fatalf("opening after restart = %+v state %s", after, harness.acquisition.State())
	}
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || retained.Current || retained.LastGood.EndpointInstance != good.EndpointInstance {
		t.Fatalf("historical record = %+v", retained)
	}
	second.push(hscVitalsTerminal(t))
	record, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || record.State != Gen3ReadEvidenceAck || record.LifecycleEpoch != good.LifecycleEpoch || record.Generation != 1 ||
		record.EndpointInstance == good.EndpointInstance {
		t.Fatalf("successor = %+v, %v", record, err)
	}
	retained, ok = harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || !retained.Current || retained.LastGood.EndpointInstance != record.EndpointInstance || retained.LastGood.LifecycleEpoch != record.LifecycleEpoch {
		t.Fatalf("successor retained = %+v", retained)
	}
}

func TestTeslaHSCActivationRestartBeforeObservation(t *testing.T) {
	harness := hscPrepare(t, false, []teslaGen3WitnessStep{{err: errors.New("reset")}}, nil)
	if err := harness.acquisition.Start(context.Background()); !errors.Is(err, ErrTeslaGen3HSCActivationFailed) {
		t.Fatalf("start = %v", err)
	}
	if harness.acquisition.State() != Gen3ReadLifecycleUnavailable || harness.acquisition.ActivationOpening() != nil || harness.witness.calls() != 1 {
		t.Fatalf("state/calls = %s/%d", harness.acquisition.State(), harness.witness.calls())
	}
	if err := harness.acquisition.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := harness.endpoint()
	opening := harness.acquisition.ActivationOpening()
	if harness.acquisition.State() != Gen3ReadLifecycleReady || harness.witness.calls() != 2 || harness.opens != 2 ||
		second.exchanges() != 0 || opening == nil || opening.Epoch != 1 || opening.Basis != HSCActivationBasisDriverExchange {
		t.Fatalf("restart state/calls/exchanges/opening = %s/%d/%d/%+v", harness.acquisition.State(), harness.witness.calls(), second.exchanges(), opening)
	}
}

func TestTeslaHSCActivationProbeAcknowledgedOnSilence(t *testing.T) {
	var harness *teslaGen3ReadHarness
	seed := hscVitalsTerminal(t)
	harness = hscPrepare(t, true, []teslaGen3WitnessStep{{
		before: func() { harness.clock.advance(time.Second) },
		report: HSCActivationReport{ClockEpoch: "synthetic-probe"},
	}}, &seed)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	endpoint := harness.endpoint()
	opening := harness.acquisition.ActivationOpening()
	if harness.acquisition.State() != Gen3ReadLifecycleReady || harness.witness.calls() != 1 || endpoint.exchanges() != 1 ||
		opening == nil || opening.Basis != HSCActivationBasisReadinessProbe || opening.Epoch != 1 || opening.Report.ClockEpoch != "synthetic-probe" ||
		len(opening.Report.Events) != 0 {
		t.Fatalf("probe opening = %+v state %s exchanges %d", opening, harness.acquisition.State(), endpoint.exchanges())
	}
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || !retained.Current || retained.LastGood.State != Gen3ReadEvidenceAck || retained.LastGood.LifecycleEpoch != 1 ||
		retained.LastGood.Generation != 1 || retained.LastGood.EndpointInstance == 0 {
		t.Fatalf("probe record = %+v", retained)
	}
	if err := harness.acquisition.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	kept := harness.acquisition.ActivationOpening()
	if harness.witness.calls() != 1 || harness.acquisition.State() != Gen3ReadLifecycleReady || kept == nil ||
		kept.Basis != HSCActivationBasisReadinessProbe || kept.Epoch != opening.Epoch || harness.endpoint().exchanges() != 0 {
		t.Fatalf("restart after probe calls/opening = %d/%+v", harness.witness.calls(), kept)
	}
}

func TestTeslaHSCActivationProbeOtherOutcomes(t *testing.T) {
	silence := func(harness **teslaGen3ReadHarness) teslaGen3WitnessStep {
		return teslaGen3WitnessStep{
			before: func() { (*harness).clock.advance(time.Second) },
			report: HSCActivationReport{ClockEpoch: "synthetic"},
		}
	}
	t.Run("rejected", func(t *testing.T) {
		var harness *teslaGen3ReadHarness
		seed := teslaGen3FakeStep{outcome: "exception", status: 0x01}
		harness = hscPrepare(t, true, []teslaGen3WitnessStep{silence(&harness)}, &seed)
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.acquisition.State() != Gen3ReadLifecycleUnavailable ||
			harness.endpoint().exchanges() != 1 || harness.endpoint().recovers != 0 || !harness.endpoint().isClosed() ||
			harness.acquisition.ActivationOpening() != nil {
			t.Fatalf("rejected probe = %v state %s exchanges %d recovers %d", err, harness.acquisition.State(), harness.endpoint().exchanges(), harness.endpoint().recovers)
		}
	})
	t.Run("timed out", func(t *testing.T) {
		var harness *teslaGen3ReadHarness
		seed := teslaGen3FakeStep{outcome: "timeout"}
		harness = hscPrepare(t, true, []teslaGen3WitnessStep{silence(&harness)}, &seed)
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.acquisition.State() != Gen3ReadLifecycleUnavailable ||
			harness.endpoint().exchanges() != 1 || harness.endpoint().recovers != 0 || !harness.endpoint().isClosed() ||
			harness.acquisition.ActivationOpening() != nil {
			t.Fatalf("timed out probe = %v state %s exchanges %d recovers %d", err, harness.acquisition.State(), harness.endpoint().exchanges(), harness.endpoint().recovers)
		}
	})
	t.Run("stays off", func(t *testing.T) {
		var harness *teslaGen3ReadHarness
		harness = hscPrepare(t, false, []teslaGen3WitnessStep{silence(&harness)}, nil)
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.endpoint().exchanges() != 0 || harness.acquisition.ActivationOpening() != nil {
			t.Fatalf("probe off = %v exchanges %d", err, harness.endpoint().exchanges())
		}
	})
	t.Run("skipped after an event", func(t *testing.T) {
		greeting := HSCActivationGreeting()
		report := HSCActivationReport{ClockEpoch: "synthetic", Events: []HSCActivationEvent{
			hscActivationEvent(HSCActivationEventGreeting, greeting),
			hscActivationEvent(HSCActivationEventEcho, greeting),
			hscActivationEvent(HSCActivationEventRefusal, HSCActivationRefusal()),
		}}
		harness := hscPrepare(t, true, []teslaGen3WitnessStep{{report: report}}, &teslaGen3FakeStep{
			terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01}),
		})
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.endpoint().exchanges() != 0 {
			t.Fatalf("probe after event = %v exchanges %d", err, harness.endpoint().exchanges())
		}
	})
	t.Run("early empty report", func(t *testing.T) {
		harness := hscPrepare(t, true, []teslaGen3WitnessStep{{
			report: HSCActivationReport{ClockEpoch: "synthetic"},
		}}, &teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, FC100OperationWCGetVitals, []byte{0x08, 0x01})})
		err := harness.acquisition.Start(context.Background())
		if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || harness.endpoint().exchanges() != 0 || harness.acquisition.ActivationOpening() != nil {
			t.Fatalf("early silence = %v exchanges %d", err, harness.endpoint().exchanges())
		}
	})
}

func TestTeslaHSCActivationRunRestartsUnavailable(t *testing.T) {
	harness := hscPrepare(t, false, nil, nil)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	endpoint := harness.endpoint()
	endpoint.recoverErr = errors.New("not quiet")
	endpoint.push(teslaGen3FakeStep{outcome: "timeout"})
	if _, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals); err != nil {
		t.Fatal(err)
	}
	if harness.acquisition.State() != Gen3ReadLifecycleUnavailable {
		t.Fatalf("state = %s", harness.acquisition.State())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- harness.acquisition.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		harness.mu.Lock()
		opens := harness.opens
		harness.mu.Unlock()
		if opens >= 2 && harness.witness.calls() == 1 && harness.acquisition.State() == Gen3ReadLifecycleReady {
			cancel()
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return")
	}
	harness.mu.Lock()
	opens := harness.opens
	harness.mu.Unlock()
	if opens < 2 || harness.witness.calls() != 1 || harness.acquisition.State() != Gen3ReadLifecycleReady {
		t.Fatalf("opens/calls/state = %d/%d/%s", opens, harness.witness.calls(), harness.acquisition.State())
	}
}

func hscReadyRecord(t *testing.T) (*teslaGen3ReadHarness, Gen3ReadRecord) {
	t.Helper()
	harness := hscPrepare(t, false, nil, nil)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	harness.endpoint().push(hscVitalsTerminal(t))
	record, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
	if err != nil || record.State != Gen3ReadEvidenceAck {
		t.Fatalf("record = %+v, %v", record, err)
	}
	harness.acquisition.mu.Lock()
	epoch, instance, generation := harness.acquisition.epoch, harness.acquisition.instance, harness.acquisition.epochGeneration
	harness.acquisition.mu.Unlock()
	if record.LifecycleEpoch != epoch || record.EndpointInstance != instance || record.Generation != generation || generation == 0 {
		t.Fatalf("identity precondition epoch %d/%d instance %d/%d generation %d/%d", record.LifecycleEpoch, epoch, record.EndpointInstance, instance, record.Generation, generation)
	}
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || !retained.Current {
		t.Fatalf("precondition retained = %+v", retained)
	}
	return harness, record
}

func TestTeslaHSCActivationCurrencyEpoch(t *testing.T) {
	// Catches a production mutation that removes the LifecycleEpoch comparison from recordCurrentLocked.
	harness, good := hscReadyRecord(t)
	harness.acquisition.mu.Lock()
	harness.acquisition.epoch++
	harness.acquisition.mu.Unlock()
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || retained.Current {
		t.Fatal("recordCurrentLocked ignored LifecycleEpoch")
	}
	delayed := harness.acquisition.publish(good, harness.endpoint())
	if delayed.State != Gen3ReadEvidenceIndeterminate || delayed.LifecycleEpoch != good.LifecycleEpoch ||
		delayed.EndpointInstance != good.EndpointInstance || delayed.Generation != good.Generation {
		t.Fatalf("publish ignored LifecycleEpoch: %+v", delayed)
	}
	after, _ := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if after.Current || !after.HasLastGood || after.LastGood.RequestID != good.RequestID {
		t.Fatalf("epoch mismatch replaced last good: %+v", after)
	}
}

func TestTeslaHSCActivationCurrencyInstance(t *testing.T) {
	// Catches a production mutation that removes the EndpointInstance comparison from recordCurrentLocked.
	harness, good := hscReadyRecord(t)
	harness.acquisition.mu.Lock()
	harness.acquisition.instance++
	harness.acquisition.mu.Unlock()
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || retained.Current {
		t.Fatal("recordCurrentLocked ignored EndpointInstance")
	}
	delayed := harness.acquisition.publish(good, harness.endpoint())
	if delayed.State != Gen3ReadEvidenceIndeterminate || delayed.EndpointInstance != good.EndpointInstance ||
		delayed.LifecycleEpoch != good.LifecycleEpoch || delayed.Generation != good.Generation {
		t.Fatalf("publish ignored EndpointInstance: %+v", delayed)
	}
	after, _ := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if after.Current || after.LastGood.EndpointInstance != good.EndpointInstance {
		t.Fatalf("instance mismatch replaced last good: %+v", after)
	}
}

func TestTeslaHSCActivationCurrencyGeneration(t *testing.T) {
	// Catches a production mutation that removes the Generation comparison from recordCurrentLocked.
	harness, good := hscReadyRecord(t)
	harness.acquisition.mu.Lock()
	harness.acquisition.epochGeneration++
	harness.acquisition.mu.Unlock()
	retained, ok := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if !ok || retained.Current {
		t.Fatal("recordCurrentLocked ignored Generation")
	}
	delayed := harness.acquisition.publish(good, harness.endpoint())
	if delayed.State != Gen3ReadEvidenceIndeterminate || delayed.Generation != good.Generation ||
		delayed.LifecycleEpoch != good.LifecycleEpoch || delayed.EndpointInstance != good.EndpointInstance {
		t.Fatalf("publish ignored Generation: %+v", delayed)
	}
	after, _ := harness.acquisition.Retained(FC100OperationWCGetVitals)
	if after.Current || after.LastGood.Generation != good.Generation {
		t.Fatalf("generation mismatch replaced last good: %+v", after)
	}
}

func TestTeslaHSCActivationConcurrentCallers(t *testing.T) {
	harness := hscPrepare(t, false, nil, nil)
	if err := harness.acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	endpoint := harness.endpoint()
	release := make(chan struct{})
	started := make(chan struct{})
	step := hscVitalsTerminal(t)
	step.started = started
	step.release = release
	endpoint.push(step)
	done := make(chan error, 1)
	go func() {
		_, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
		done <- err
	}()
	<-started
	if _, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals); !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
		t.Fatalf("concurrent acquire = %v", err)
	}
	if err := harness.acquisition.Restart(context.Background()); !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
		t.Fatalf("concurrent restart = %v", err)
	}
	if opening := harness.acquisition.ActivationOpening(); opening == nil || opening.Epoch == 0 {
		t.Fatal("opening missing during an in-flight exchange")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if endpoint.maxInflightSeen() != 1 {
		t.Fatalf("max in flight = %d", endpoint.maxInflightSeen())
	}

	endpoint.fallback = func() teslaGen3FakeStep {
		return teslaGen3FakeStep{outcome: "exception", status: 0x04}
	}
	var wg sync.WaitGroup
	for index := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				switch index % 4 {
				case 0:
					_, _ = harness.acquisition.PollOnce(context.Background())
				case 1:
					_, _ = harness.acquisition.Retained(FC100OperationWCGetVitals)
					_ = harness.acquisition.State()
				case 2:
					_ = harness.acquisition.ActivationOpening()
				default:
					_, err := harness.acquisition.Acquire(context.Background(), FC100OperationWCGetVitals)
					if err != nil && !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
						t.Errorf("acquire = %v", err)
					}
				}
			}
		}()
	}
	wg.Wait()
	if endpoint.maxInflightSeen() != 1 || harness.witness.calls() != 1 {
		t.Fatalf("max in flight/calls = %d/%d", endpoint.maxInflightSeen(), harness.witness.calls())
	}
}
