package tesla

import (
	"context"
	"testing"
	"time"
)

func TestTeslaProvisionalCurrentLimitUnqualifiedPollIsScoped(t *testing.T) {
	config := teslaGen3ReadTestConfig(t, nil)
	gen3 := config.Gen3.config
	gen3.Version = "wc3_24_44_4"
	config.Gen3 = mustTeslaGen3Profile(t, gen3)
	config.Operations = append(config.Operations, FC100OperationWCGetProvisional)
	config.Windows[FC100OperationWCGetProvisional] = Gen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
	acquisition, err := NewGen3ReadAcquisition(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := acquisition.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := acquisition.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	endpoint := acquisition.endpoint.(*teslaGen3FakeEndpoint)
	for _, op := range config.Operations[:5] {
		endpoint.push(teslaGen3FakeStep{terminal: teslaGen3ReadTerminal(t, op, nil)})
	}
	records, err := acquisition.PollOnce(context.Background())
	if err != nil || len(records) != 5 || endpoint.exchanges() != 5 {
		t.Fatalf("poll = %d records, %d exchanges, %v", len(records), endpoint.exchanges(), err)
	}
	for _, record := range records {
		if record.State != Gen3ReadEvidenceAck || record.Operation == FC100OperationWCGetProvisional {
			t.Fatalf("unexpected poll record %+v", record)
		}
	}
}

func TestTeslaProvisionalCurrentLimitPollTypedReadback(t *testing.T) {
	for _, malformed := range [][]byte{{10, 2, 10, 0}, {10, 4, 8, 16, 8, 16}, {10, 4, 8, 16}} {
		h := newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
			gen3 := config.Gen3.config
			gen3.Version = "26.34.0"
			config.Gen3 = mustTeslaGen3Profile(t, gen3)
			config.Operations = append(config.Operations, FC100OperationWCGetProvisional)
			config.Windows[FC100OperationWCGetProvisional] = Gen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
		})
		ctx := context.Background()
		for _, op := range teslaGen3ReadTestOperations {
			teslaPushTerminal(t, h, op, nil)
		}
		bound := uint32(32)
		teslaPushTerminal(t, h, FC100OperationWCGetProvisional, teslaReadbackBody(ProvisionalCurrentLimitRequest{16, 899, true}, &bound, false))
		records, err := h.acquisition.PollOnce(ctx)
		if err != nil || len(records) != 6 || h.endpoint().exchanges() != 6 {
			t.Fatalf("poll=%d, %v", len(records), err)
		}
		request := h.endpoint().requests[5]
		body, err := teslaGen3ExactFC100RequestBody(FC100OperationWCGetProvisional, request.Payload())
		if err != nil || len(body) != 0 {
			t.Fatalf("t27=%x, %v", request.Payload(), err)
		}
		good, ok := records[5].ProvisionalReadback()
		if !ok || good != (ProvisionalCurrentLimitReadback{16, 899, true, 32, true, true, true, true}) {
			t.Fatalf("got %+v", good)
		}
		good.ConfiguredCurrentAmps = 7
		retained, _ := h.acquisition.Retained(FC100OperationWCGetProvisional)
		check, _ := retained.LastGood.ProvisionalReadback()
		if check.ConfiguredCurrentAmps != 32 {
			t.Fatal("typed payload mutated")
		}
		denied, err := h.acquisition.SetProvisionalCurrentLimit(ctx, ProvisionalCurrentLimitRequest{16, 900, false})
		if err == nil || !denied.NoSend || h.endpoint().exchanges() != 6 {
			t.Fatalf("t27 admitted t25: %+v, %v", denied, err)
		}
		teslaPushTerminal(t, h, FC100OperationWCGetProvisional, malformed)
		attempt, err := h.acquisition.Acquire(ctx, FC100OperationWCGetProvisional)
		retained, _ = h.acquisition.Retained(FC100OperationWCGetProvisional)
		if err != nil || attempt.State != Gen3ReadEvidenceIndeterminate || !attempt.IntegrityValid || !retained.HasLastGood || retained.LastGood.RequestID != records[5].RequestID || retained.LastAttempt.RequestID != attempt.RequestID || len(attempt.ResponseADUs()) != 1 {
			t.Fatalf("attempt=%+v, err=%v", attempt, err)
		}
	}
}

func TestTeslaProvisionalCurrentLimitReadSchemaAndCapabilityAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		schema    string
		qualified bool
	}{
		{HSCFC100OperationCompatibilityV1, false}, {"", true}, {"other_schema", true},
	} {
		h := newTeslaGen3ReadHarness(t, func(config *Gen3ReadAcquisitionConfig) {
			gen3 := config.Gen3.config
			gen3.ProvisionalSetQualified = true
			config.Gen3 = mustTeslaGen3Profile(t, gen3)
			hsc := teslaGen3ReadProfileConfig()
			hsc.WCProvisionalReadbackOperationVersion = tc.schema
			hsc.WCProvisionalReadbackQualified = tc.qualified
			config.Profile = mustTeslaHSCProfile(t, hsc)
			config.Operations = append(config.Operations, FC100OperationWCGetProvisional)
			config.Windows[FC100OperationWCGetProvisional] = Gen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
		})
		for _, operation := range teslaGen3ReadTestOperations {
			teslaPushTerminal(t, h, operation, nil)
		}
		records, err := h.acquisition.PollOnce(context.Background())
		if err != nil || len(records) != 5 || h.endpoint().exchanges() != 5 {
			t.Fatalf("schema %q capability %t: %d, %v", tc.schema, tc.qualified, len(records), err)
		}
		result, err := h.acquisition.SetProvisionalCurrentLimit(context.Background(), ProvisionalCurrentLimitRequest{16, 900, false})
		if err == nil || !result.NoSend || h.endpoint().exchanges() != 5 {
			t.Fatalf("schema %q capability %t: %+v, %v", tc.schema, tc.qualified, result, err)
		}
	}
}
