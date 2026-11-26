package reader

import (
	"bytes"
	"context"
	"errors"
	modbus "github.com/Project-Helianthus/helianthus-modbus"
	"strings"
	"testing"
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
)

func expectedRequest(t *testing.T, op tesla.FC100Operation) []byte {
	t.Helper()
	request, err := tesla.BuildTeslaFC100OperationRequest(tesla.HSCFC100OperationCompatibilityV1, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request.Payload()
}

// run constructs, scripts and starts a reader. The script is queued while the
// lifecycle opens its endpoint.
func run(t *testing.T, cfg ChargerConfig, script func(*scriptedEndpoint)) (*ChargerReader, *fakeSession, *charger) {
	t.Helper()
	c := newCharger(t)
	c.deps.Now = func() time.Time { return time.Unix(61, 0) }
	open := c.deps.Open
	c.deps.Open = func(config modbus.RTUProductionConfig) (tesla.Gen3ReadEndpoint, error) {
		endpoint, err := open(config)
		if err == nil && script != nil {
			script(c.endpoint)
		}
		return endpoint, err
	}
	reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
	if err != nil || reader == nil {
		t.Fatalf("reader %v err %v", reader, err)
	}
	session := &fakeSession{}
	if err := reader.Start(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Stop(context.Background()) })
	return reader, session, c
}

func TestEmptyBlockOpensNothing(t *testing.T) {
	c := newCharger(t)
	for name, cfg := range map[string]ChargerConfig{"zero": {}, "empty collections": {Operations: []tesla.FC100Operation{}, Windows: map[tesla.FC100Operation]tesla.Gen3ReadWindow{}}} {
		reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
		if reader != nil || err != nil || c.opens != 0 {
			t.Fatalf("%s: reader=%v err=%v opens=%d", name, reader, err, c.opens)
		}
	}
}

func TestMissingValueIsReportedByNameAndOpensNothing(t *testing.T) {
	for field, mutate := range map[string]func(*ChargerConfig){
		"serial_path":          func(c *ChargerConfig) { c.SerialPath = "" },
		"endpoint":             func(c *ChargerConfig) { c.Endpoint = "" },
		"node":                 func(c *ChargerConfig) { c.Node = 0 },
		"version":              func(c *ChargerConfig) { c.Version = "" },
		"response_timeout":     func(c *ChargerConfig) { c.ResponseTimeout = 0 },
		"max_response_delay":   func(c *ChargerConfig) { c.MaxResponseDelay = 0 },
		"max_quiescence":       func(c *ChargerConfig) { c.MaxQuiescence = 0 },
		"poll_interval":        func(c *ChargerConfig) { c.PollInterval = 0 },
		"acquisition_deadline": func(c *ChargerConfig) { c.AcquisitionDeadline = 0 },
		"activation_timeout":   func(c *ChargerConfig) { c.ActivationTimeout = 0 },
		"max_attempts":         func(c *ChargerConfig) { c.MaxAttempts = 0 },
		"operations":           func(c *ChargerConfig) { c.Operations = nil },
		"windows[wc.get_vitals]": func(c *ChargerConfig) {
			delete(c.Windows, tesla.FC100OperationWCGetVitals)
		},
	} {
		t.Run(field, func(t *testing.T) {
			c := newCharger(t)
			cfg := syntheticConfig(fiveOperations...)
			mutate(&cfg)
			reader, err := NewChargerReader(cfg, syntheticPublication(), c.deps)
			var named *ConfigError
			if reader != nil || !errors.As(err, &named) || named.Field != field || c.opens != 0 {
				t.Fatalf("reader=%v err=%v opens=%d", reader, err, c.opens)
			}
			if strings.Contains(err.Error(), syntheticSerialPath) {
				t.Fatalf("the serial path is in %q", err)
			}
		})
	}
	for _, attempts := range []int{4, -1} {
		cfg := syntheticConfig(fiveOperations...)
		cfg.MaxAttempts = attempts
		var named *ConfigError
		if err := cfg.Admit(); !errors.As(err, &named) || named.Field != "max_attempts" {
			t.Fatalf("attempts %d: %v", attempts, err)
		}
	}
}

func TestSixPollBlockSendsOnlyTheActivationAndSixReads(t *testing.T) {
	cfg := syntheticConfig(append(append([]tesla.FC100Operation(nil), fiveOperations...), tesla.FC100OperationWCGetProvisional)...)
	cfg.ProvisionalReadbackQualified = true
	_, session, c := run(t, cfg, func(e *scriptedEndpoint) {
		pushPolls(e, cfg.Operations, scalars{limit: u(16), timeout: u(0), inhibit: b(false), configured: u(32)})
	})
	waitFor(t, "six reads", func() bool { return len(c.endpoint.sent()) == 6 && len(session.committed()) == 1 })
	sent := c.endpoint.sent()
	for i, op := range cfg.Operations {
		if !bytes.Equal(sent[i].Payload(), expectedRequest(t, op)) {
			t.Fatalf("request %d is not %s", i, op)
		}
	}
	if c.act.calls != 1 || c.opens != 1 {
		t.Fatalf("activation %d opens %d", c.act.calls, c.opens)
	}
	if got := c.endpoint.config.Endpoint; strings.Contains(got, "/") {
		t.Fatalf("endpoint label %q looks like a path", got)
	}
}

func TestFiveReadsWithoutSetterSendNoLimitReadAndConsumeNoReadbackWindow(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	reader, _, c := run(t, cfg, func(e *scriptedEndpoint) { pushPolls(e, cfg.Operations, scalars{}) })
	waitFor(t, "five reads", func() bool { return len(c.endpoint.sent()) == 5 })
	time.Sleep(50 * time.Millisecond)
	if n := len(c.endpoint.sent()); n != 5 {
		t.Fatalf("%d requests", n)
	}
	if reader.Setter().Available || reader.CurrentLimitWritable() {
		t.Fatalf("setter %+v", reader.Setter())
	}
	for _, op := range c.endpoint.sent() {
		if bytes.Equal(op.Payload(), expectedRequest(t, tesla.FC100OperationWCGetProvisional)) {
			t.Fatal("a t27 was sent")
		}
	}
}

func TestSetterWindowMissingOrInvalidOnlyDisablesTheSetter(t *testing.T) {
	for name, window := range map[string]struct {
		w      tesla.Gen3ReadWindow
		reason string
		drop   bool
	}{
		"missing":             {drop: true, reason: "missing"},
		"zero freshness":      {w: tesla.Gen3ReadWindow{Retention: time.Second}, reason: "freshness must be positive"},
		"retention too short": {w: tesla.Gen3ReadWindow{Freshness: 2 * time.Second, Retention: time.Second}, reason: "retention must be at least the freshness"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := syntheticConfig(fiveOperations...)
			cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = true, true
			if window.drop {
				delete(cfg.Windows, tesla.FC100OperationWCGetProvisional)
			} else {
				cfg.Windows[tesla.FC100OperationWCGetProvisional] = window.w
			}
			reader, _, c := run(t, cfg, func(e *scriptedEndpoint) { pushPolls(e, cfg.Operations, scalars{}) })
			waitFor(t, "five reads", func() bool { return len(c.endpoint.sent()) == 5 })
			setter := reader.Setter()
			if setter.Available || setter.Field != "windows[wc.get_provisional]" || setter.Reason != window.reason || reader.CurrentLimitWritable() {
				t.Fatalf("setter %+v", setter)
			}
			waitFor(t, "poll cycle to finish", func() bool {
				_, err := reader.Acquisition().SetProvisionalCurrentLimit(context.Background(), tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 6, LimitTimeoutSeconds: 900})
				return !errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionBusy)
			})
			if n := len(c.endpoint.sent()); n != 5 {
				t.Fatalf("the setter sent traffic: %d requests", n)
			}
		})
	}
}

func TestSetterUnqualifiedNamesTheQualification(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	if s := cfg.Setter(); s.Available || s.Field != "provisional_set_qualified" {
		t.Fatalf("%+v", s)
	}
	cfg.ProvisionalSetQualified = true
	if s := cfg.Setter(); s.Available || s.Field != "provisional_readback_qualified" {
		t.Fatalf("%+v", s)
	}
	cfg.ProvisionalReadbackQualified = true
	if s := cfg.Setter(); !s.Available || !cfg.CurrentLimitWritable() {
		t.Fatalf("%+v", s)
	}
}

func TestFiveReadsWithSetterRetainReadbackWindowAndFeedThePrerequisite(t *testing.T) {
	cfg := syntheticConfig(fiveOperations...)
	cfg.ProvisionalReadbackQualified, cfg.ProvisionalSetQualified = true, true
	reader, session, c := run(t, cfg, func(e *scriptedEndpoint) { pushPolls(e, cfg.Operations, scalars{}) })
	waitFor(t, "five reads", func() bool { return len(c.endpoint.sent()) == 5 })
	c.endpoint.push(step{echo: true, terminal: terminal(6, 28, readbackBody(scalars{limit: u(6), timeout: u(900), inhibit: b(false), configured: u(16)}))})
	var result tesla.ProvisionalCurrentLimitResult
	waitFor(t, "the setter to run", func() bool {
		var err error
		result, err = reader.Acquisition().SetProvisionalCurrentLimit(context.Background(), tesla.ProvisionalCurrentLimitRequest{LimitCurrentMaxAmps: 20, LimitTimeoutSeconds: 900})
		return !errors.Is(err, tesla.ErrTeslaGen3ReadAcquisitionBusy)
	})
	sent := c.endpoint.sent()
	if len(sent) != 6 || !bytes.Equal(sent[5].Payload(), expectedRequest(t, tesla.FC100OperationWCGetProvisional)) {
		t.Fatalf("the trace is not five reads and one on-demand t27: %d", len(sent))
	}
	if !result.NoSend {
		t.Fatalf("the 20 A request was not refused before t25: %+v", result)
	}
	if err := reader.IngestSetResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	f, ok := reader.View().Fact(ElementConfiguredCurrent)
	if !ok || f.Amps != 16 || f.Receipt.RequestID != result.Prerequisite.RequestID || f.Receipt.RequestID == 0 {
		t.Fatalf("configured %+v ok=%v prerequisite receipt %d", f, ok, result.Prerequisite.RequestID)
	}
	batches := session.committed()
	if len(batches) == 0 {
		t.Fatal("nothing published")
	}
}
