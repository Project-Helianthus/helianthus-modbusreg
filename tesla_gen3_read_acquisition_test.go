package modbusreg

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

// The vectors below are self-contained synthetic FC100 frames built from the
// retained family/tag catalog; they carry no capture, serial, or device data.

var teslaGen3ReadTestOperations = []TeslaFC100Operation{
	TeslaFC100OperationCommonSystemInfo,
	TeslaFC100OperationWCGetVitals,
	TeslaFC100OperationWCGetLifetimeStats,
	TeslaFC100OperationWCGetSystemInfo,
	TeslaFC100OperationWCGetLoadSharingState,
}

func TestTeslaGen3ReadAcquisitionAcquiresEachObservationOperation(t *testing.T) {
	for _, operation := range teslaGen3ReadTestOperations {
		t.Run(string(operation), func(t *testing.T) {
			harness := newTeslaGen3ReadHarness(t, nil)
			endpoint := harness.endpoint()
			body := []byte{0x08, 0x01, 0x7a, 0x02, 0xaa, 0xbb}
			endpoint.push(teslaGen3FakeStep{echo: true, terminal: teslaGen3ReadTerminal(t, operation, body)})

			record, err := harness.acquisition.Acquire(context.Background(), operation)
			if err != nil {
				t.Fatal(err)
			}
			spec := teslaFC100OperationSpecs[operation]
			if record.State != TeslaGen3ReadEvidenceAck || record.Operation != operation ||
				record.Family != spec.family || record.RequestTag != spec.requestTag || record.TerminalTag != spec.responseTag {
				t.Fatalf("record identity = %#v", record)
			}
			if record.Endpoint != "evse-a" || record.Node != 0x10 || record.Profile != TeslaHSCProfileName ||
				record.ProfileVersion != TeslaHSCCompatibilityV1 ||
				record.OperationVersion != TeslaHSCFC100OperationCompatibilityV1 ||
				record.Gen3Version != "wc3_24_44_3" || record.Gen3VersionDisposition != TeslaGen3HSCVersionKnownObservation {
				t.Fatalf("record profile = %#v", record)
			}
			if record.Source != TeslaGen3ReadSourceRTUPrivateFunction || record.LifecycleEpoch != 1 ||
				record.Generation != 1 || record.RequestID != 1 || record.Attempt != 1 || record.MaxAttempts != 2 ||
				record.TransportOutcome != "success" || !record.ObservedAt.Equal(harness.clock.now()) {
				t.Fatalf("record source = %#v", record)
			}
			if !bytes.Equal(record.TerminalBody(), body) {
				t.Fatalf("terminal body = %x, want %x", record.TerminalBody(), body)
			}
			results := record.Results()
			if len(results) != 2 || results[0].Kind != TeslaFC100OperationIntermediate || results[1].Kind != TeslaFC100OperationTerminal {
				t.Fatalf("results = %#v", results)
			}
			qualified, _ := teslaGen3ReadQualifiedOperation(operation)
			want, _, err := harness.profile.EncodeQualifiedFunction(qualified)
			if err != nil {
				t.Fatal(err)
			}
			if len(endpoint.requests) != 1 || !bytes.Equal(endpoint.requests[0].Payload(), want.Payload()) ||
				endpoint.requests[0].FunctionCode() != teslaHSCFunction100 || endpoint.units[0] != 0x10 {
				t.Fatalf("requests = %#v", endpoint.requests)
			}
			if len(record.RequestADU()) == 0 || len(record.ResponseADUs()) != 2 {
				t.Fatalf("evidence frames = %x / %x", record.RequestADU(), record.ResponseADUs())
			}
			mutated := record.TerminalBody()
			mutated[0] ^= 0xff
			if bytes.Equal(record.TerminalBody(), mutated) {
				t.Fatal("terminal body is not defensively copied")
			}
		})
	}
}

func TestTeslaGen3ReadAcquisitionOwnsProductionEndpointConfiguration(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	config := harness.endpoint().config
	if config.Endpoint != "evse-a" || !config.Enabled || config.Serial.Path != "/dev/null-evse" ||
		config.Serial.Baud != 115200 || config.Serial.DataBits != 8 || config.Serial.Parity != modbus.RTUParityNone ||
		config.Serial.StopBits != 1 || config.ResponseTimeout != time.Second {
		t.Fatalf("production config = %#v", config)
	}
	if config.Admission == nil || config.PrivateFunction.Admission == nil || config.PrivateFunction.MaxResponses != 8 {
		t.Fatalf("production admission = %#v", config.PrivateFunction)
	}
	if config.Admission.AdmitRTURead(modbus.RTUReadAdmissionRequest{Endpoint: "evse-a", Generation: 1, UnitID: 0x10, Function: 3, Quantity: 1}) {
		t.Fatal("FC03/FC04 read admitted by the Tesla read-path lifecycle")
	}
	if harness.opens != 1 || harness.witness.calls() != 1 || harness.acquisition.State() != TeslaGen3ReadLifecycleReady {
		t.Fatalf("opens/activations/state = %d/%d/%s", harness.opens, harness.witness.calls(), harness.acquisition.State())
	}
}

func TestTeslaGen3ReadAcquisitionAdmissionBindsPlannedCandidate(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	admission := harness.endpoint().config.PrivateFunction.Admission
	request, _, err := harness.profile.EncodeQualifiedFunction(TeslaTEDAPIOperationWCVitalsV1)
	if err != nil {
		t.Fatal(err)
	}
	candidate := modbus.RTUPrivateFunctionAdmissionRequest{Endpoint: "evse-a", Generation: 1, UnitID: 0x10, Function: teslaHSCFunction100, Payload: request.Payload()}
	if admission.AdmitRTUPrivateFunction(candidate) {
		t.Fatal("candidate admitted without a planned lifecycle exchange")
	}
	endpoint := harness.endpoint()
	endpoint.onAdmission = func(planned modbus.RTUPrivateFunctionAdmissionRequest) {
		for name, mutate := range map[string]func(*modbus.RTUPrivateFunctionAdmissionRequest){
			"endpoint": func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Endpoint = "evse-b" },
			"unit":     func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.UnitID = 0x11 },
			"function": func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Function = teslaHSCFunction101 },
			"payload":  func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Payload = []byte{0x04, 0x32, 0x02, 0x3a, 0x00} },
			"stale":    func(c *modbus.RTUPrivateFunctionAdmissionRequest) { c.Generation = 0 },
		} {
			changed := planned
			changed.Payload = append([]byte(nil), planned.Payload...)
			mutate(&changed)
			if admission.AdmitRTUPrivateFunction(changed) {
				t.Errorf("mismatched %s candidate admitted", name)
			}
		}
	}
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})})
	if record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals); err != nil || record.State != TeslaGen3ReadEvidenceAck {
		t.Fatalf("planned candidate = %#v, %v", record, err)
	}
}

func TestTeslaGen3ReadAcquisitionRejectsMutationAndUnqualifiedConfiguration(t *testing.T) {
	for _, operation := range []TeslaFC100Operation{
		TeslaFC100OperationWCConfigureSettings,
		TeslaFC100OperationWCSetProvisional,
		TeslaFC100OperationWCGetProvisional,
		"wc.get_config",
	} {
		config := teslaGen3ReadTestConfig(t, nil)
		config.Operations = append(config.Operations, operation)
		config.Windows[operation] = TeslaGen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
		if _, err := NewTeslaGen3ReadAcquisition(config); !errors.Is(err, ErrQualifiedFunctionNoSend) {
			t.Fatalf("%s accepted: %v", operation, err)
		}
	}
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) {
		config.Operations = []TeslaFC100Operation{TeslaFC100OperationWCGetVitals}
	})
	for _, operation := range []TeslaFC100Operation{TeslaFC100OperationWCSetProvisional, TeslaFC100OperationWCGetLifetimeStats} {
		if _, err := harness.acquisition.Acquire(context.Background(), operation); !errors.Is(err, ErrQualifiedFunctionNoSend) {
			t.Fatalf("unconfigured %s acquire error = %v", operation, err)
		}
	}
	if harness.endpoint().exchanges() != 0 {
		t.Fatalf("rejected operations exchanged %d times", harness.endpoint().exchanges())
	}

	cases := map[string]func(*TeslaGen3ReadAcquisitionConfig){
		"unqualified profile": func(c *TeslaGen3ReadAcquisitionConfig) {
			c.Profile = mustTeslaHSCProfile(t, TeslaHSCProfileConfig{Enabled: true, Node: 0x10, CompatibilityVersion: TeslaHSCCompatibilityV1})
		},
		"missing operation version": func(c *TeslaGen3ReadAcquisitionConfig) {
			cfg := teslaGen3ReadProfileConfig()
			cfg.WCLifetimeOperationVersion = ""
			c.Profile = mustTeslaHSCProfile(t, cfg)
		},
		"gen3 version unknown": func(c *TeslaGen3ReadAcquisitionConfig) {
			c.Gen3 = mustTeslaGen3Profile(t, TeslaGen3HSCProfileConfig{Enabled: true, ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true})
		},
		"gen3 not activation capable": func(c *TeslaGen3ReadAcquisitionConfig) {
			c.Gen3 = mustTeslaGen3Profile(t, TeslaGen3HSCProfileConfig{Enabled: true, Version: "wc3_24_44_3", PrivateFunctionCapable: true, OperationCapable: true})
		},
		"no activation witness": func(c *TeslaGen3ReadAcquisitionConfig) { c.Activation = nil },
		"zero attempts":         func(c *TeslaGen3ReadAcquisitionConfig) { c.MaxAttempts = 0 },
		"four attempts":         func(c *TeslaGen3ReadAcquisitionConfig) { c.MaxAttempts = 4 },
		"empty endpoint":        func(c *TeslaGen3ReadAcquisitionConfig) { c.Endpoint = "" },
		"path as label":         func(c *TeslaGen3ReadAcquisitionConfig) { c.Endpoint = "/dev/ttyUSB0" },
		"empty local path":      func(c *TeslaGen3ReadAcquisitionConfig) { c.LocalPath = "" },
		"unbounded timeout":     func(c *TeslaGen3ReadAcquisitionConfig) { c.ResponseTimeout = 0 },
		"unbounded deadline":    func(c *TeslaGen3ReadAcquisitionConfig) { c.AcquisitionDeadline = 0 },
		"unbounded activation":  func(c *TeslaGen3ReadAcquisitionConfig) { c.ActivationTimeout = 0 },
		"unbounded poll":        func(c *TeslaGen3ReadAcquisitionConfig) { c.PollInterval = 0 },
		"no operations":         func(c *TeslaGen3ReadAcquisitionConfig) { c.Operations = nil },
		"duplicate operation":   func(c *TeslaGen3ReadAcquisitionConfig) { c.Operations = append(c.Operations, c.Operations[0]) },
		"missing window":        func(c *TeslaGen3ReadAcquisitionConfig) { delete(c.Windows, TeslaFC100OperationWCGetVitals) },
		"retention below fresh": func(c *TeslaGen3ReadAcquisitionConfig) {
			c.Windows[TeslaFC100OperationWCGetVitals] = TeslaGen3ReadWindow{Freshness: time.Minute, Retention: time.Second}
		},
		"zero freshness": func(c *TeslaGen3ReadAcquisitionConfig) {
			c.Windows[TeslaFC100OperationWCGetVitals] = TeslaGen3ReadWindow{Retention: time.Second}
		},
		"deadline below timeout": func(c *TeslaGen3ReadAcquisitionConfig) { c.AcquisitionDeadline = c.ResponseTimeout / 2 },
	}
	for name, mutate := range cases {
		config := teslaGen3ReadTestConfig(t, nil)
		opened := false
		config.Open = func(modbus.RTUProductionConfig) (TeslaGen3ReadEndpoint, error) {
			opened = true
			return nil, errors.New("must not open")
		}
		mutate(&config)
		if _, err := NewTeslaGen3ReadAcquisition(config); !errors.Is(err, ErrQualifiedFunctionNoSend) {
			t.Errorf("%s accepted: %v", name, err)
		}
		if opened {
			t.Errorf("%s opened an endpoint", name)
		}
	}
}

func TestTeslaGen3ReadAcquisitionRequiresDocumentedActivation(t *testing.T) {
	for name, step := range map[string]teslaGen3WitnessStep{
		"unexpected bytes": {observed: []byte("TESLA\x00FAIL\x00")},
		"partial":          {observed: []byte("TESLA\x00")},
		"timeout":          {err: context.DeadlineExceeded},
		"reset":            {err: errors.New("device reset")},
	} {
		t.Run(name, func(t *testing.T) {
			config := teslaGen3ReadTestConfig(t, nil)
			witness := &teslaGen3FakeWitness{steps: []teslaGen3WitnessStep{step}}
			config.Activation = witness
			var endpoints []*teslaGen3FakeEndpoint
			config.Open = func(production modbus.RTUProductionConfig) (TeslaGen3ReadEndpoint, error) {
				endpoint := &teslaGen3FakeEndpoint{config: production, generation: 1}
				endpoints = append(endpoints, endpoint)
				return endpoint, nil
			}
			acquisition, err := NewTeslaGen3ReadAcquisition(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := acquisition.Start(context.Background()); !errors.Is(err, ErrTeslaGen3HSCActivationFailed) {
				t.Fatalf("start error = %v", err)
			}
			if acquisition.State() != TeslaGen3ReadLifecycleUnavailable || len(endpoints) != 1 || !endpoints[0].isClosed() {
				t.Fatalf("state/endpoints = %s/%d", acquisition.State(), len(endpoints))
			}
			if _, err := acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals); !errors.Is(err, ErrTeslaGen3ReadAcquisitionUnavailable) {
				t.Fatalf("acquire after failed activation = %v", err)
			}
			if endpoints[0].exchanges() != 0 {
				t.Fatal("exchange sent before activation completed")
			}
		})
	}
}

func TestTeslaGen3ReadAcquisitionSingleFlightFailsFast(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	endpoint := harness.endpoint()
	release := make(chan struct{})
	started := make(chan struct{})
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01}), started: started, release: release})

	done := make(chan error, 1)
	go func() {
		_, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
		done <- err
	}()
	<-started
	if _, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetLifetimeStats); !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
		t.Fatalf("concurrent acquire error = %v", err)
	}
	if _, err := harness.acquisition.PollOnce(context.Background()); !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
		t.Fatalf("concurrent poll error = %v", err)
	}
	if err := harness.acquisition.Restart(context.Background()); !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
		t.Fatalf("concurrent restart error = %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if endpoint.exchanges() != 1 || endpoint.maxInflightSeen() != 1 {
		t.Fatalf("exchanges/max in flight = %d/%d", endpoint.exchanges(), endpoint.maxInflightSeen())
	}
}

func TestTeslaGen3ReadAcquisitionTimeoutLeavesValueIndeterminate(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) { config.MaxAttempts = 1 })
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})})
	good, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || good.State != TeslaGen3ReadEvidenceAck {
		t.Fatalf("baseline = %#v, %v", good, err)
	}
	harness.clock.advance(100 * time.Millisecond)
	endpoint.push(teslaGen3FakeStep{echo: true, outcome: "timeout"})
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != TeslaGen3ReadEvidenceTimeout || record.TerminalBody() != nil || record.Results() != nil ||
		record.TransportOutcome != "transport_fault" || record.Generation != 1 || len(record.ResponseADUs()) != 1 {
		t.Fatalf("timeout record = %#v", record)
	}
	if endpoint.recovers != 1 || harness.witness.calls() != 2 || harness.acquisition.State() != TeslaGen3ReadLifecycleReady {
		t.Fatalf("recovers/activations/state = %d/%d/%s", endpoint.recovers, harness.witness.calls(), harness.acquisition.State())
	}
	retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || !retained.HasLastGood || retained.LastGood.RequestID != good.RequestID ||
		retained.LastAttempt.State != TeslaGen3ReadEvidenceTimeout || retained.Current {
		t.Fatalf("retained after timeout = %#v", retained)
	}

	for name, outcome := range map[string]string{"write fault": "write_fault", "cancellation": "cancel", "response bound": "response_bound", "classifier": "classifier_fault"} {
		endpoint.push(teslaGen3FakeStep{outcome: outcome})
		record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
		if err != nil || record.State != TeslaGen3ReadEvidenceIndeterminate || record.TerminalBody() != nil {
			t.Fatalf("%s record = %#v, %v", name, record, err)
		}
	}
}

func TestTeslaGen3ReadAcquisitionRetriesAreBounded(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) { config.MaxAttempts = 3 })
	endpoint := harness.endpoint()
	for range 4 {
		endpoint.push(teslaGen3FakeStep{outcome: "timeout"})
	}
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceTimeout || record.Attempt != 3 || record.MaxAttempts != 3 {
		t.Fatalf("exhausted record = %#v, %v", record, err)
	}
	if endpoint.exchanges() != 3 || endpoint.recovers != 3 || harness.witness.calls() != 4 || record.LifecycleEpoch != 3 {
		t.Fatalf("exchanges/recovers/activations/epoch = %d/%d/%d/%d", endpoint.exchanges(), endpoint.recovers, harness.witness.calls(), record.LifecycleEpoch)
	}

	retry := newTeslaGen3ReadHarness(t, nil)
	retryEndpoint := retry.endpoint()
	retryEndpoint.push(teslaGen3FakeStep{outcome: "timeout"})
	retryEndpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x02})})
	record, err = retry.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceAck || record.Attempt != 2 || record.Generation != 2 || record.LifecycleEpoch != 2 {
		t.Fatalf("retried record = %#v, %v", record, err)
	}

	rejected := newTeslaGen3ReadHarness(t, nil)
	rejectedEndpoint := rejected.endpoint()
	rejectedEndpoint.push(teslaGen3FakeStep{outcome: "exception", status: 0x01})
	record, err = rejected.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceRejected || !record.HasExceptionStatus || record.ExceptionStatus != 0x01 ||
		record.TerminalBody() != nil || rejectedEndpoint.exchanges() != 1 || rejectedEndpoint.recovers != 0 {
		t.Fatalf("exception record = %#v, %v", record, err)
	}
	rejectedEndpoint.push(teslaGen3FakeStep{echo: true, terminal: []byte{0x06, 0x22, 0x04, 0x0a, 0x02, 0x08, 0x05}})
	record, err = rejected.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceRejected || !record.HasApplicationStatus || record.ApplicationStatus != 5 ||
		record.TerminalBody() != nil || rejectedEndpoint.exchanges() != 2 {
		t.Fatalf("application error record = %#v, %v", record, err)
	}
	rejectedEndpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetLifetimeStats, []byte{0x08, 0x01})})
	rejectedEndpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})})
	record, err = rejected.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceAck || record.Attempt != 2 || record.Generation != 2 ||
		record.LifecycleEpoch != 2 || rejectedEndpoint.recovers != 1 {
		t.Fatalf("wrong-tag terminal retry = %#v, %v (recovers %d)", record, err, rejectedEndpoint.recovers)
	}
	if retained, _ := rejected.acquisition.Retained(TeslaFC100OperationWCGetVitals); retained.LastAttempt.RequestID != record.RequestID {
		t.Fatalf("retained attempt = %#v", retained.LastAttempt)
	}

	denied := newTeslaGen3ReadHarness(t, nil)
	denied.endpoint().push(teslaGen3FakeStep{outcome: "deny"})
	record, err = denied.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceNoSend || record.RequestID != 0 || denied.endpoint().exchanges() != 1 || denied.endpoint().recovers != 0 {
		t.Fatalf("pre-send rejection = %#v, %v", record, err)
	}
}

func TestTeslaGen3ReadAcquisitionFreshnessAndRetentionWindows(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) {
		config.Operations = []TeslaFC100Operation{TeslaFC100OperationWCGetVitals, TeslaFC100OperationWCGetSystemInfo}
		config.Windows = map[TeslaFC100Operation]TeslaGen3ReadWindow{
			TeslaFC100OperationWCGetVitals:     {Freshness: 10 * time.Second, Retention: time.Minute},
			TeslaFC100OperationWCGetSystemInfo: {Freshness: time.Hour, Retention: 2 * time.Hour},
		}
	})
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})})
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetSystemInfo, []byte{0x08, 0x02})})
	records, err := harness.acquisition.PollOnce(context.Background())
	if err != nil || len(records) != 2 || records[0].Operation != TeslaFC100OperationWCGetVitals || records[1].Operation != TeslaFC100OperationWCGetSystemInfo {
		t.Fatalf("first poll = %#v, %v", records, err)
	}
	if retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals); !ok || !retained.Fresh || !retained.Current {
		t.Fatalf("fresh vitals = %#v", retained)
	}

	harness.clock.advance(11 * time.Second)
	retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || retained.Fresh || !retained.HasLastGood || !retained.Current {
		t.Fatalf("stale vitals = %#v", retained)
	}
	endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x03})})
	records, err = harness.acquisition.PollOnce(context.Background())
	if err != nil || len(records) != 1 || records[0].Operation != TeslaFC100OperationWCGetVitals || endpoint.exchanges() != 3 {
		t.Fatalf("second poll = %#v, %v (exchanges %d)", records, err, endpoint.exchanges())
	}

	harness.clock.advance(2 * time.Minute)
	if _, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals); ok {
		t.Fatal("vitals retained beyond retention window")
	}
	if retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetSystemInfo); !ok || !retained.Fresh {
		t.Fatalf("system info within its own window = %#v", retained)
	}
	if _, ok := harness.acquisition.Retained(TeslaFC100OperationWCSetProvisional); ok {
		t.Fatal("mutation operation has a retained record")
	}
}

func TestTeslaGen3ReadAcquisitionReconnectRequiresRecoveryAndActivation(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{outcome: "write_fault"})
	harness.witness.push(teslaGen3WitnessStep{observed: []byte("TESLA\x00")})
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceIndeterminate || record.Attempt != 1 {
		t.Fatalf("record = %#v, %v", record, err)
	}
	if harness.acquisition.State() != TeslaGen3ReadLifecycleUnavailable || endpoint.recovers != 1 || endpoint.exchanges() != 1 {
		t.Fatalf("state/recovers/exchanges = %s/%d/%d", harness.acquisition.State(), endpoint.recovers, endpoint.exchanges())
	}
	if _, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals); !errors.Is(err, ErrTeslaGen3ReadAcquisitionUnavailable) {
		t.Fatalf("acquire while unavailable = %v", err)
	}

	failedRecovery := newTeslaGen3ReadHarness(t, nil)
	failedEndpoint := failedRecovery.endpoint()
	failedEndpoint.recoverErr = errors.New("not quiet")
	failedEndpoint.push(teslaGen3FakeStep{outcome: "timeout"})
	record, err = failedRecovery.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceTimeout || failedRecovery.acquisition.State() != TeslaGen3ReadLifecycleUnavailable ||
		failedEndpoint.exchanges() != 1 || failedRecovery.witness.calls() != 1 {
		t.Fatalf("failed recovery = %#v, %v, %s", record, err, failedRecovery.acquisition.State())
	}
}

func TestTeslaGen3ReadAcquisitionRestartStartsSuccessorEndpoint(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	first := harness.endpoint()
	first.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})})
	good, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil {
		t.Fatal(err)
	}
	firstAdmission := first.config.PrivateFunction.Admission
	if err := harness.acquisition.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := harness.endpoint()
	if second == first || !first.isClosed() || harness.opens != 2 || harness.witness.calls() != 2 {
		t.Fatalf("restart endpoints/opens/activations = %v/%d/%d", first.isClosed(), harness.opens, harness.witness.calls())
	}
	retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || retained.LastGood.RequestID != good.RequestID || retained.Current || !retained.Fresh {
		t.Fatalf("retained after restart = %#v", retained)
	}
	second.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x02})})
	second.onAdmission = func(planned modbus.RTUPrivateFunctionAdmissionRequest) {
		if firstAdmission.AdmitRTUPrivateFunction(planned) {
			t.Error("retired endpoint admission accepted a successor candidate")
		}
	}
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil || record.State != TeslaGen3ReadEvidenceAck || record.LifecycleEpoch != 2 || record.Generation != 1 {
		t.Fatalf("successor record = %#v, %v", record, err)
	}
	if retained, _ := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals); !retained.Current || retained.LastGood.LifecycleEpoch != 2 {
		t.Fatalf("retained successor = %#v", retained)
	}

	if err := harness.acquisition.Close(); err != nil || !second.isClosed() || harness.acquisition.State() != TeslaGen3ReadLifecycleClosed {
		t.Fatalf("close = %v / %v / %s", err, second.isClosed(), harness.acquisition.State())
	}
	if _, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals); !errors.Is(err, ErrTeslaGen3ReadAcquisitionClosed) {
		t.Fatalf("acquire after close = %v", err)
	}
	if err := harness.acquisition.Restart(context.Background()); !errors.Is(err, ErrTeslaGen3ReadAcquisitionClosed) {
		t.Fatalf("restart after close = %v", err)
	}
	if retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals); !ok || retained.Current {
		t.Fatalf("retained after close = %#v", retained)
	}
}

func TestTeslaGen3ReadAcquisitionRunPollsUntilCancelled(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) {
		config.Operations = []TeslaFC100Operation{TeslaFC100OperationWCGetVitals}
		config.Windows = map[TeslaFC100Operation]TeslaGen3ReadWindow{TeslaFC100OperationWCGetVitals: {Freshness: time.Nanosecond, Retention: time.Millisecond}}
		config.PollInterval = time.Millisecond
		config.Now = time.Now
	})
	endpoint := harness.endpoint()
	endpoint.fallback = func() teslaGen3FakeStep {
		time.Sleep(time.Microsecond)
		return teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01})}
	}
	ctx, cancel := context.WithCancel(context.Background())
	endpoint.afterExchange = func(count int) {
		if count == 3 {
			cancel()
		}
	}
	if err := harness.acquisition.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v", err)
	}
	if endpoint.exchanges() < 3 || endpoint.maxInflightSeen() != 1 {
		t.Fatalf("run exchanges/max in flight = %d/%d", endpoint.exchanges(), endpoint.maxInflightSeen())
	}
}

func TestTeslaGen3ReadAcquisitionConcurrentCallersUnderRace(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	endpoint := harness.endpoint()
	endpoint.fallback = func() teslaGen3FakeStep {
		return teslaGen3FakeStep{outcome: "exception", status: 0x04}
	}
	var wg sync.WaitGroup
	for index := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			operation := teslaGen3ReadTestOperations[index%len(teslaGen3ReadTestOperations)]
			for range 20 {
				switch index % 4 {
				case 0:
					_, _ = harness.acquisition.PollOnce(context.Background())
				case 1:
					_, _ = harness.acquisition.Retained(operation)
					_ = harness.acquisition.State()
				default:
					_, err := harness.acquisition.Acquire(context.Background(), operation)
					if err != nil && !errors.Is(err, ErrTeslaGen3ReadAcquisitionBusy) {
						t.Errorf("acquire error = %v", err)
					}
				}
			}
		}()
	}
	wg.Wait()
	if endpoint.maxInflightSeen() != 1 {
		t.Fatalf("max in flight = %d", endpoint.maxInflightSeen())
	}
}

// --- test harness -------------------------------------------------------

type teslaGen3ReadHarness struct {
	acquisition *TeslaGen3ReadAcquisition
	profile     TeslaHSCProfile
	witness     *teslaGen3FakeWitness
	clock       *teslaGen3FakeClock
	mu          sync.Mutex
	endpoints   []*teslaGen3FakeEndpoint
	opens       int
}

func newTeslaGen3ReadHarness(t *testing.T, mutate func(*TeslaGen3ReadAcquisitionConfig)) *teslaGen3ReadHarness {
	t.Helper()
	harness := &teslaGen3ReadHarness{clock: &teslaGen3FakeClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}}
	config := teslaGen3ReadTestConfig(t, harness)
	if mutate != nil {
		mutate(&config)
	}
	harness.profile = config.Profile
	acquisition, err := NewTeslaGen3ReadAcquisition(config)
	if err != nil {
		t.Fatal(err)
	}
	harness.acquisition = acquisition
	if err := acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = acquisition.Close() })
	return harness
}

func (harness *teslaGen3ReadHarness) endpoint() *teslaGen3FakeEndpoint {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.endpoints[len(harness.endpoints)-1]
}

func teslaGen3ReadProfileConfig() TeslaHSCProfileConfig {
	return TeslaHSCProfileConfig{
		Enabled: true, Node: 0x10, PassiveCompatible: true, CompatibilityVersion: TeslaHSCCompatibilityV1,
		WCVitalsOperationVersion:           TeslaHSCWCVitalsCompatibilityV1,
		SystemInfoOperationVersion:         TeslaHSCSystemInfoCompatibilityV1,
		WCLifetimeOperationVersion:         TeslaHSCWCLifetimeCompatibilityV1,
		WCSystemInfoOperationVersion:       TeslaHSCWCSystemInfoCompatibilityV1,
		WCLoadSharingStateOperationVersion: TeslaHSCWCLoadSharingStateCompatibilityV1,
	}
}

func teslaGen3ReadTestConfig(t *testing.T, harness *teslaGen3ReadHarness) TeslaGen3ReadAcquisitionConfig {
	t.Helper()
	timing, err := modbus.NewRTUTiming(modbus.RTUTimingConfig{
		Baud: 115200, DataBits: 8, Parity: modbus.RTUParityNone, StopBits: 1,
		MaxResponseLatency: 500 * time.Millisecond, MaxQuiescence: 900 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	windows := make(map[TeslaFC100Operation]TeslaGen3ReadWindow)
	for _, operation := range teslaGen3ReadTestOperations {
		windows[operation] = TeslaGen3ReadWindow{Freshness: 30 * time.Second, Retention: 10 * time.Minute}
	}
	config := TeslaGen3ReadAcquisitionConfig{
		Endpoint: "evse-a", LocalPath: "/dev/null-evse", Timing: timing, ResponseTimeout: time.Second,
		Profile:             mustTeslaHSCProfile(t, teslaGen3ReadProfileConfig()),
		Gen3:                mustTeslaGen3Profile(t, TeslaGen3HSCProfileConfig{Enabled: true, Version: "wc3_24_44_3", ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true}),
		Operations:          append([]TeslaFC100Operation(nil), teslaGen3ReadTestOperations...),
		Windows:             windows,
		MaxAttempts:         2,
		AcquisitionDeadline: 10 * time.Second,
		ActivationTimeout:   time.Second,
		PollInterval:        time.Second,
		Activation:          &teslaGen3FakeWitness{},
	}
	if harness != nil {
		harness.witness = config.Activation.(*teslaGen3FakeWitness)
		config.Now = harness.clock.now
		config.Open = func(production modbus.RTUProductionConfig) (TeslaGen3ReadEndpoint, error) {
			harness.mu.Lock()
			defer harness.mu.Unlock()
			endpoint := &teslaGen3FakeEndpoint{config: production, generation: 1}
			harness.endpoints = append(harness.endpoints, endpoint)
			harness.opens++
			return endpoint, nil
		}
	} else {
		config.Open = func(production modbus.RTUProductionConfig) (TeslaGen3ReadEndpoint, error) {
			return &teslaGen3FakeEndpoint{config: production, generation: 1}, nil
		}
	}
	return config
}

func mustTeslaHSCProfile(t *testing.T, config TeslaHSCProfileConfig) TeslaHSCProfile {
	t.Helper()
	profile, err := NewTeslaHSCProfile(config)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func mustTeslaGen3Profile(t *testing.T, config TeslaGen3HSCProfileConfig) TeslaGen3HSCProfile {
	t.Helper()
	profile, err := NewTeslaGen3HSCProfile(config)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func teslaGen3ReadTerminal(t *testing.T, operation TeslaFC100Operation, body []byte) []byte {
	t.Helper()
	spec, ok := teslaFC100OperationSpecs[operation]
	if !ok {
		t.Fatalf("unknown operation %s", operation)
	}
	inner := appendTeslaFC100Varint(nil, spec.responseTag<<3|2)
	inner = appendTeslaFC100Varint(inner, uint64(len(body)))
	inner = append(inner, body...)
	message := appendTeslaFC100Varint(nil, spec.family<<3|2)
	message = appendTeslaFC100Varint(message, uint64(len(inner)))
	message = append(message, inner...)
	return append([]byte{byte(len(message))}, message...)
}

type teslaGen3FakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (clock *teslaGen3FakeClock) now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.at
}

func (clock *teslaGen3FakeClock) advance(delta time.Duration) {
	clock.mu.Lock()
	clock.at = clock.at.Add(delta)
	clock.mu.Unlock()
}

type teslaGen3WitnessStep struct {
	before   func()
	waitDone bool
	observed []byte
	err      error
}

type teslaGen3FakeWitness struct {
	mu       sync.Mutex
	steps    []teslaGen3WitnessStep
	observed int
}

func (witness *teslaGen3FakeWitness) push(step teslaGen3WitnessStep) {
	witness.mu.Lock()
	witness.steps = append(witness.steps, step)
	witness.mu.Unlock()
}

func (witness *teslaGen3FakeWitness) calls() int {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	return witness.observed
}

func (witness *teslaGen3FakeWitness) ObserveTeslaGen3HSCActivation(ctx context.Context, request TeslaGen3HSCActivationRequest) ([]byte, error) {
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.observed++
	if ctx == nil || request.Endpoint != "evse-a" || request.Epoch == 0 {
		return nil, errors.New("invalid activation request")
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("activation is unbounded")
	}
	if len(witness.steps) == 0 {
		return []byte("TESLA\x00PASS\x00"), nil
	}
	step := witness.steps[0]
	witness.steps = witness.steps[1:]
	if step.before != nil {
		step.before()
	}
	if step.waitDone {
		<-ctx.Done()
	}
	return append([]byte(nil), step.observed...), step.err
}

type teslaGen3FakeStep struct {
	beforeComplete func()
	echo           bool
	terminal       []byte
	outcome        string
	status         byte
	started        chan struct{}
	release        chan struct{}
}

type teslaGen3FakeEndpoint struct {
	mu            sync.Mutex
	config        modbus.RTUProductionConfig
	generation    uint64
	nextID        uint64
	fenced        bool
	closed        bool
	inflight      int
	maxInflight   int
	exchangeCount int
	recovers      int
	recoverErr    error
	steps         []teslaGen3FakeStep
	fallback      func() teslaGen3FakeStep
	onAdmission   func(modbus.RTUPrivateFunctionAdmissionRequest)
	afterExchange func(int)
	requests      []modbus.PrivateFunctionRequest
	units         []byte
}

func (endpoint *teslaGen3FakeEndpoint) push(step teslaGen3FakeStep) {
	endpoint.mu.Lock()
	endpoint.steps = append(endpoint.steps, step)
	endpoint.mu.Unlock()
}

func (endpoint *teslaGen3FakeEndpoint) exchanges() int {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.exchangeCount
}

func (endpoint *teslaGen3FakeEndpoint) maxInflightSeen() int {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.maxInflight
}

func (endpoint *teslaGen3FakeEndpoint) isClosed() bool {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.closed
}

var errTeslaGen3FakeState = errors.New("fake endpoint state")

func (endpoint *teslaGen3FakeEndpoint) ExchangePrivateFunction(
	ctx context.Context,
	unit byte,
	request modbus.PrivateFunctionRequest,
	policy modbus.PrivateFunctionResponsePolicy,
	classifier modbus.RTUPrivateFunctionResponseClassifier,
) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error) {
	endpoint.mu.Lock()
	if endpoint.closed || endpoint.fenced || endpoint.inflight != 0 || policy.MaxAttempts() != 1 {
		endpoint.mu.Unlock()
		return nil, modbus.RTUPrivateFunctionEvidence{}, errTeslaGen3FakeState
	}
	endpoint.inflight++
	if endpoint.inflight > endpoint.maxInflight {
		endpoint.maxInflight = endpoint.inflight
	}
	endpoint.exchangeCount++
	count := endpoint.exchangeCount
	endpoint.requests = append(endpoint.requests, request)
	endpoint.units = append(endpoint.units, unit)
	var step teslaGen3FakeStep
	switch {
	case len(endpoint.steps) != 0:
		step = endpoint.steps[0]
		endpoint.steps = endpoint.steps[1:]
	case endpoint.fallback != nil:
		step = endpoint.fallback()
	default:
		step = teslaGen3FakeStep{outcome: "timeout"}
	}
	candidate := modbus.RTUPrivateFunctionAdmissionRequest{Endpoint: endpoint.config.Endpoint, Generation: endpoint.generation, UnitID: unit, Function: request.FunctionCode(), Payload: request.Payload()}
	onAdmission := endpoint.onAdmission
	admission := endpoint.config.PrivateFunction.Admission
	endpoint.mu.Unlock()
	defer func() {
		endpoint.mu.Lock()
		endpoint.inflight--
		after := endpoint.afterExchange
		endpoint.mu.Unlock()
		if after != nil {
			after(count)
		}
	}()

	if onAdmission != nil {
		onAdmission(candidate)
	}
	if step.outcome == "deny" || !admission.AdmitRTUPrivateFunction(candidate) {
		return nil, modbus.RTUPrivateFunctionEvidence{}, errors.New("fake no-send")
	}
	if step.started != nil {
		close(step.started)
	}
	if step.release != nil {
		select {
		case <-step.release:
		case <-ctx.Done():
		}
	}
	requestADU, err := modbus.EncodeRTUPrivateFunctionADU(unit, request)
	if err != nil {
		return nil, modbus.RTUPrivateFunctionEvidence{}, err
	}
	endpoint.mu.Lock()
	endpoint.nextID++
	evidence := modbus.RTUPrivateFunctionEvidence{
		Endpoint: endpoint.config.Endpoint, Generation: endpoint.generation, RequestID: endpoint.nextID,
		UnitID: unit, Function: request.FunctionCode(), RequestADU: requestADU, SentAt: time.Millisecond,
		ReceivedAt: 2 * time.Millisecond, ReceiptWall: time.Unix(1, 0), ClockEpoch: "fake",
	}
	endpoint.mu.Unlock()

	var responses []modbus.RTUPrivateFunctionResponseADU
	appendFrame := func(payload []byte) error {
		responseRequest, err := modbus.NewPrivateFunctionRequest(request.FunctionCode(), payload)
		if err != nil {
			return err
		}
		frame, err := modbus.EncodeRTUPrivateFunctionADU(unit, responseRequest)
		if err != nil {
			return err
		}
		response, err := modbus.DecodeRTUPrivateFunctionResponseADU(unit, request, frame)
		if err != nil {
			return err
		}
		evidence.ResponseADUs = append(evidence.ResponseADUs, frame)
		responses = append(responses, response)
		return nil
	}
	if step.echo {
		if err := appendFrame(request.Payload()); err != nil {
			return nil, evidence, err
		}
		if classifier != nil {
			disposition, err := classifier.ClassifyRTUPrivateFunctionResponse(responses[0])
			if err != nil || disposition != modbus.RTUPrivateFunctionContinue {
				return nil, endpoint.fail(evidence, "classifier_fault"), errors.New("echo was not continued")
			}
		}
	}
	switch step.outcome {
	case "":
		if err := appendFrame(step.terminal); err != nil {
			return nil, evidence, err
		}
		if classifier != nil {
			disposition, err := classifier.ClassifyRTUPrivateFunctionResponse(responses[len(responses)-1])
			if err != nil {
				return nil, endpoint.fail(evidence, "classifier_fault"), err
			}
			if disposition != modbus.RTUPrivateFunctionTerminal {
				return nil, endpoint.fail(evidence, "response_bound"), errors.New("terminal was not classified terminal")
			}
		}
		if step.beforeComplete != nil {
			step.beforeComplete()
		}
		endpoint.mu.Lock()
		evidence.Current = !endpoint.closed && !endpoint.fenced
		endpoint.mu.Unlock()
		evidence.IntegrityValid = true
		evidence.TerminalOutcome = "success"
		return responses, evidence, nil
	case "exception":
		frame := []byte{unit, request.FunctionCode().Byte() | 0x80, step.status, 0, 0}
		evidence.ResponseADUs = append(evidence.ResponseADUs, frame)
		evidence.IntegrityValid = true
		evidence.TerminalOutcome = "exception"
		return nil, evidence, errors.New("correlated exception")
	case "timeout":
		return nil, endpoint.fail(evidence, "transport_fault"), context.DeadlineExceeded
	case "block":
		<-ctx.Done()
		return nil, endpoint.fail(evidence, "transport_fault"), ctx.Err()
	case "cancel":
		return nil, endpoint.fail(evidence, "transport_fault"), context.Canceled
	case "write_fault":
		return nil, endpoint.fail(evidence, "write_fault"), errors.New("short write")
	default:
		return nil, endpoint.fail(evidence, step.outcome), errors.New(step.outcome)
	}
}

func (endpoint *teslaGen3FakeEndpoint) fail(evidence modbus.RTUPrivateFunctionEvidence, outcome string) modbus.RTUPrivateFunctionEvidence {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	endpoint.fenced = true
	evidence.TerminalOutcome = outcome
	evidence.Current = false
	return evidence
}

func (endpoint *teslaGen3FakeEndpoint) Recover(ctx context.Context) error {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	if ctx == nil || endpoint.closed || !endpoint.fenced || endpoint.inflight != 0 {
		return errTeslaGen3FakeState
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("recovery is unbounded")
	}
	endpoint.recovers++
	if endpoint.recoverErr != nil {
		return endpoint.recoverErr
	}
	endpoint.generation++
	endpoint.fenced = false
	return nil
}

func (endpoint *teslaGen3FakeEndpoint) Close() error {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	endpoint.closed = true
	endpoint.fenced = true
	return nil
}

func TestTeslaGen3ReadAcquisitionBindsGen3VersionToOperationContract(t *testing.T) {
	for _, version := range []string{"wc3_24_28_3", "25.1.0", "24.44.3", " wc3_24_44_3"} {
		config := teslaGen3ReadTestConfig(t, nil)
		config.Gen3 = mustTeslaGen3Profile(t, TeslaGen3HSCProfileConfig{
			Enabled: true, Version: version, ActivationCapable: true, PrivateFunctionCapable: true, OperationCapable: true,
		})
		if !config.Gen3.ExchangeEligible() {
			t.Fatalf("%q precondition: Gen3 profile not eligible", version)
		}
		if _, err := NewTeslaGen3ReadAcquisition(config); !errors.Is(err, ErrQualifiedFunctionNoSend) {
			t.Fatalf("Gen3 version %q constructed against the wc3_24_44_3 catalog: %v", version, err)
		}
	}
}

func TestTeslaGen3ReadAcquisitionCloseDuringCompletionPublishesNoValue(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, nil)
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{
		echo:           true,
		terminal:       teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01}),
		beforeComplete: func() { _ = harness.acquisition.Close() },
	})
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != TeslaGen3ReadEvidenceIndeterminate || record.TransportOutcome != "success" ||
		record.TerminalBody() != nil || record.Results() != nil || len(record.ResponseADUs()) != 2 {
		t.Fatalf("retired completion record = %#v", record)
	}
	retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || retained.HasLastGood || retained.LastAttempt.State != TeslaGen3ReadEvidenceIndeterminate || retained.Current {
		t.Fatalf("retired completion retained = %#v", retained)
	}
	if endpoint.exchanges() != 1 || endpoint.recovers != 0 || harness.acquisition.State() != TeslaGen3ReadLifecycleClosed {
		t.Fatalf("exchanges/recovers/state = %d/%d/%s", endpoint.exchanges(), endpoint.recovers, harness.acquisition.State())
	}
}

func TestTeslaGen3ReadAcquisitionRecoveryStaysWithinAcquisitionBudget(t *testing.T) {
	harness := newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) {
		config.ResponseTimeout = 40 * time.Millisecond
		config.AcquisitionDeadline = 80 * time.Millisecond
		config.MaxAttempts = 3
	})
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{outcome: "block"})
	began := time.Now()
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	elapsed := time.Since(began)
	if err != nil || record.State != TeslaGen3ReadEvidenceTimeout || record.TerminalBody() != nil || record.Attempt != 1 {
		t.Fatalf("deadline record = %#v, %v", record, err)
	}
	if elapsed > 500*time.Millisecond || endpoint.recovers != 0 || harness.witness.calls() != 1 ||
		endpoint.exchanges() != 1 || harness.acquisition.State() != TeslaGen3ReadLifecycleUnavailable {
		t.Fatalf("elapsed/recovers/activations/exchanges/state = %s/%d/%d/%d/%s",
			elapsed, endpoint.recovers, harness.witness.calls(), endpoint.exchanges(), harness.acquisition.State())
	}

	cancelled := newTeslaGen3ReadHarness(t, nil)
	cancelledEndpoint := cancelled.endpoint()
	started := make(chan struct{})
	cancelledEndpoint.push(teslaGen3FakeStep{outcome: "block", started: started})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	began = time.Now()
	record, err = cancelled.acquisition.Acquire(ctx, TeslaFC100OperationWCGetVitals)
	elapsed = time.Since(began)
	if err != nil || record.State != TeslaGen3ReadEvidenceIndeterminate || record.TerminalBody() != nil {
		t.Fatalf("cancelled record = %#v, %v", record, err)
	}
	if elapsed > 500*time.Millisecond || cancelledEndpoint.recovers != 0 || cancelled.witness.calls() != 1 ||
		cancelled.acquisition.State() != TeslaGen3ReadLifecycleUnavailable {
		t.Fatalf("cancelled elapsed/recovers/activations/state = %s/%d/%d/%s",
			elapsed, cancelledEndpoint.recovers, cancelled.witness.calls(), cancelled.acquisition.State())
	}
}

func TestTeslaGen3ReadAcquisitionRejectsExpiredActivation(t *testing.T) {
	for name, tc := range map[string]struct {
		cancelled bool
		step      teslaGen3WitnessStep
	}{
		"cancelled start": {cancelled: true, step: teslaGen3WitnessStep{observed: []byte("TESLA\x00PASS\x00")}},
		"late success":    {step: teslaGen3WitnessStep{waitDone: true, observed: []byte("TESLA\x00PASS\x00")}},
	} {
		t.Run(name, func(t *testing.T) {
			config := teslaGen3ReadTestConfig(t, nil)
			config.ActivationTimeout = 20 * time.Millisecond
			config.Activation = &teslaGen3FakeWitness{steps: []teslaGen3WitnessStep{tc.step}}
			var endpoint *teslaGen3FakeEndpoint
			config.Open = func(production modbus.RTUProductionConfig) (TeslaGen3ReadEndpoint, error) {
				endpoint = &teslaGen3FakeEndpoint{config: production, generation: 1}
				return endpoint, nil
			}
			acquisition, err := NewTeslaGen3ReadAcquisition(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			if err := acquisition.Start(ctx); !errors.Is(err, ErrTeslaGen3HSCActivationFailed) {
				t.Fatalf("start error = %v", err)
			}
			if acquisition.State() != TeslaGen3ReadLifecycleUnavailable || endpoint == nil || !endpoint.isClosed() {
				t.Fatalf("state = %s", acquisition.State())
			}
			if _, err := acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals); !errors.Is(err, ErrTeslaGen3ReadAcquisitionUnavailable) {
				t.Fatalf("acquire after expired activation = %v", err)
			}
		})
	}
}

func TestTeslaGen3ReadAcquisitionCloseBeforePromotionPublishesNoValue(t *testing.T) {
	var harness *teslaGen3ReadHarness
	var armed atomic.Bool
	var closedInBarrier atomic.Bool
	harness = newTeslaGen3ReadHarness(t, func(config *TeslaGen3ReadAcquisitionConfig) {
		clock := config.Now
		config.Now = func() time.Time {
			// Barrier: the first clock read after transport success lets Close
			// complete before the record can be promoted.
			if armed.CompareAndSwap(true, false) {
				if err := harness.acquisition.Close(); err == nil {
					closedInBarrier.Store(true)
				}
			}
			return clock()
		}
	})
	endpoint := harness.endpoint()
	endpoint.push(teslaGen3FakeStep{
		echo:           true,
		terminal:       teslaGen3ReadTerminal(t, TeslaFC100OperationWCGetVitals, []byte{0x08, 0x01}),
		beforeComplete: func() { armed.Store(true) },
	})
	record, err := harness.acquisition.Acquire(context.Background(), TeslaFC100OperationWCGetVitals)
	if err != nil {
		t.Fatal(err)
	}
	if !closedInBarrier.Load() || !endpoint.isClosed() {
		t.Fatal("barrier did not close the lifecycle after transport success")
	}
	if record.State != TeslaGen3ReadEvidenceIndeterminate || record.TransportOutcome != "success" ||
		record.TerminalBody() != nil || record.Results() != nil || len(record.ResponseADUs()) != 2 {
		t.Fatalf("record after Close won = %#v", record)
	}
	retained, ok := harness.acquisition.Retained(TeslaFC100OperationWCGetVitals)
	if !ok || retained.HasLastGood || retained.LastAttempt.State != TeslaGen3ReadEvidenceIndeterminate || retained.Current {
		t.Fatalf("retained after Close won = %#v", retained)
	}
}

func TestTeslaGen3ReadAcquisitionRejectsActivationAtOrAfterDeadline(t *testing.T) {
	for name, tc := range map[string]struct {
		advance time.Duration
		ready   bool
	}{
		"before deadline": {advance: 999 * time.Millisecond, ready: true},
		"at deadline":     {advance: time.Second},
		"after deadline":  {advance: time.Second + time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			clock := &teslaGen3FakeClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
			config := teslaGen3ReadTestConfig(t, nil)
			config.Now = clock.now
			config.ActivationTimeout = time.Second
			// The fake clock passes the activation deadline while the real
			// context timer has not fired, so ctx.Err() is still nil.
			config.Activation = &teslaGen3FakeWitness{steps: []teslaGen3WitnessStep{{
				before:   func() { clock.advance(tc.advance) },
				observed: []byte("TESLA\x00PASS\x00"),
			}}}
			acquisition, err := NewTeslaGen3ReadAcquisition(config)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = acquisition.Close() }()
			err = acquisition.Start(context.Background())
			if tc.ready {
				if err != nil || acquisition.State() != TeslaGen3ReadLifecycleReady {
					t.Fatalf("start = %v, state %s", err, acquisition.State())
				}
				return
			}
			if !errors.Is(err, ErrTeslaGen3HSCActivationFailed) || acquisition.State() != TeslaGen3ReadLifecycleUnavailable {
				t.Fatalf("start = %v, state %s", err, acquisition.State())
			}
		})
	}
}
