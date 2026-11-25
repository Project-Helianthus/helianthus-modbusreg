package tesla

import (
	"bytes"
	"context"
	"testing"
	"time"

	modbus "github.com/Project-Helianthus/helianthus-modbus"
)

type teslaSerialVector struct{ request, reply []byte }

// The pinned transport cannot inject a byte stream into its production opener.
// This adapter writes complete Modbus ADUs to a memory line and uses the
// transport's RTU codecs on both sides of the scripted exchange.
type teslaProvisionalSerial struct {
	t       *testing.T
	config  modbus.RTUProductionConfig
	line    bytes.Buffer
	writes  [][]byte
	vectors []teslaSerialVector
	// An uncorrelated reply is allowed only in a rejection vector.
	allowClassifierError bool
}

func (line *teslaProvisionalSerial) Close() error { return nil }
func (line *teslaProvisionalSerial) Recover(context.Context) error {
	line.t.Fatal("unexpected serial recovery")
	return nil
}

func (line *teslaProvisionalSerial) frame(unit byte, payload []byte) []byte {
	line.t.Helper()
	req, err := modbus.NewPrivateFunctionRequest(100, payload)
	if err != nil {
		line.t.Fatal(err)
	}
	frame, err := modbus.EncodeRTUPrivateFunctionADU(unit, req)
	if err != nil {
		line.t.Fatal(err)
	}
	return frame
}

func (line *teslaProvisionalSerial) ExchangePrivateFunction(_ context.Context, unit byte, request modbus.PrivateFunctionRequest, _ modbus.PrivateFunctionResponsePolicy, classifier modbus.RTUPrivateFunctionResponseClassifier) ([]modbus.RTUPrivateFunctionResponseADU, modbus.RTUPrivateFunctionEvidence, error) {
	line.t.Helper()
	payload := request.Payload()
	if !line.config.PrivateFunction.Admission.AdmitRTUPrivateFunction(modbus.RTUPrivateFunctionAdmissionRequest{
		Endpoint: line.config.Endpoint, Generation: 1, UnitID: unit, Function: request.FunctionCode(), Payload: payload,
	}) {
		line.t.Fatal("scripted request not admitted")
	}
	index := len(line.writes)
	frame, err := modbus.EncodeRTUPrivateFunctionADU(unit, request)
	if err != nil {
		line.t.Fatal(err)
	}
	line.line.Write(frame)
	written := bytes.Clone(line.line.Next(len(frame)))
	line.writes = append(line.writes, written)
	if index >= len(line.vectors) {
		line.t.Fatalf("unexpected serial write %x", written)
	}
	vector := line.vectors[index]
	if want := line.frame(0x10, vector.request); !bytes.Equal(written, want) {
		line.t.Fatalf("write %d = %x, want %x", index, written, want)
	}
	line.line.Write(line.frame(unit, vector.reply))
	raw := bytes.Clone(line.line.Next(line.line.Len()))
	response, err := modbus.DecodeRTUPrivateFunctionResponseADU(unit, request, raw)
	if err != nil {
		line.t.Fatal(err)
	}
	disposition, err := classifier.ClassifyRTUPrivateFunctionResponse(response)
	evidence := modbus.RTUPrivateFunctionEvidence{Generation: 1, RequestID: uint64(index + 1), RequestADU: written, ResponseADUs: [][]byte{raw}, Current: true, IntegrityValid: true, TerminalOutcome: "success"}
	if (err != nil && !line.allowClassifierError) || (err == nil && disposition != modbus.RTUPrivateFunctionTerminal) {
		line.t.Errorf("serial reply %x: not terminal (%v, %v)", raw, disposition, err)
	}
	return []modbus.RTUPrivateFunctionResponseADU{response}, evidence, err
}

func teslaProvisionalSerialAcquisition(t *testing.T, version string, poll bool, vectors []teslaSerialVector, customize ...func(*Gen3ReadAcquisitionConfig)) (*Gen3ReadAcquisition, *teslaProvisionalSerial) {
	t.Helper()
	config := teslaGen3ReadTestConfig(t, nil)
	gen := config.Gen3.config
	gen.Version = version
	gen.ProvisionalSetQualified = version == "24.44.3"
	config.Gen3 = mustTeslaGen3Profile(t, gen)
	if gen.ProvisionalSetQualified {
		hsc := teslaGen3ReadProfileConfig()
		hsc.WCProvisionalReadbackOperationVersion = HSCFC100OperationCompatibilityV1
		hsc.WCProvisionalReadbackQualified = true
		config.Profile = mustTeslaHSCProfile(t, hsc)
	}
	config.Windows[FC100OperationWCGetProvisional] = Gen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
	if poll {
		config.Operations = append(config.Operations, FC100OperationWCGetProvisional)
	}
	for _, configure := range customize {
		configure(&config)
	}
	line := &teslaProvisionalSerial{t: t, vectors: vectors}
	config.Open = func(production modbus.RTUProductionConfig) (Gen3ReadEndpoint, error) {
		line.config = production
		return line, nil
	}
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
	return acquisition, line
}

func teslaSerialMessage(family, tag uint64, body ...byte) []byte {
	inner := appendTeslaFC100Varint(nil, tag<<3|2)
	inner = append(append(inner, byte(len(body))), body...)
	return append([]byte{byte(len(inner) + 2), byte(family<<3 | 2), byte(len(inner))}, inner...)
}

func TestTeslaProvisionalCurrentLimitSerialPartialPoll(t *testing.T) {
	for _, version := range []string{"24.44.3", "26.34.0", "26.42.0"} {
		t.Run(version, func(t *testing.T) {
			var vectors []teslaSerialVector
			for _, pair := range [][2]uint64{{4, 2}, {6, 1}, {6, 3}, {6, 9}, {6, 11}} {
				vectors = append(vectors, teslaSerialVector{teslaSerialMessage(pair[0], pair[1]), teslaSerialMessage(pair[0], pair[1]+1)})
			}
			reply := teslaSerialMessage(6, 28, 10, 5, 8, 16, 16, 0x83, 7, 16, 32)
			vectors = append(vectors, teslaSerialVector{teslaSerialMessage(6, 27), reply})
			acq, line := teslaProvisionalSerialAcquisition(t, version, true, vectors)
			records, err := acq.PollOnce(context.Background())
			if err != nil || len(records) != len(vectors) || len(line.writes) != len(vectors) {
				t.Fatalf("poll: %d records, %d writes, %v", len(records), len(line.writes), err)
			}
			retained, _ := acq.Retained(FC100OperationWCGetProvisional)
			got, ok := retained.LastGood.ProvisionalReadback()
			if !retained.Current || !ok || got != (ProvisionalCurrentLimitReadback{16, 899, false, 32, true, true, false, true}) || !bytes.Equal(retained.LastGood.terminalPayload(), reply) {
				t.Fatalf("partial retained readback = %+v, %+v", got, retained)
			}
		})
	}
}

func TestTeslaProvisionalCurrentLimitSerialDocumentedSet(t *testing.T) {
	for _, tc := range []struct {
		name, version                   string
		pause                           bool
		boundReply, setReply, readReply []byte
		comparison                      CurrentLimitComparison
		status                          uint64
	}{
		{"countdown and identical reply", "26.34.0", true, nil, nil, teslaSerialMessage(6, 28, 10, 7, 8, 6, 16, 0x83, 7, 24, 1, 16, 32), CurrentLimitMatch, 0},
		{"countdown cannot exceed requested lifetime", "26.34.0", true, nil, nil, teslaSerialMessage(6, 28, 10, 7, 8, 6, 16, 0x85, 7, 24, 1, 16, 32), CurrentLimitMismatch, 0},
		{"canonical reply and partial readback", "26.42.0", false, nil, teslaSerialMessage(6, 25, 10, 5, 8, 16, 16, 0x84, 7), teslaSerialMessage(6, 28, 10, 5, 8, 16, 16, 0x84, 7, 16, 32), CurrentLimitUnknown, 0},
		{"application refusal", "26.34.0", false, nil, teslaSerialMessage(4, 1, 10, 2, 8, 3), nil, CurrentLimitUnknown, 3},
		{"queried bound below request", "26.34.0", false, teslaSerialMessage(6, 6, 10, 2, 8, 12), nil, nil, CurrentLimitUnknown, 0},
		{"missing installation bound", "26.42.0", false, teslaSerialMessage(6, 6), nil, nil, CurrentLimitUnknown, 0},
		{"unnamed patch", "26.42.1", false, nil, nil, nil, CurrentLimitUnknown, 0},
		{"unnamed minor", "26.43.0", false, nil, nil, nil, CurrentLimitUnknown, 0},
		{"unnamed major", "27.42.0", false, nil, nil, nil, CurrentLimitUnknown, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := ProvisionalCurrentLimitRequest{16, 900, false}
			set := teslaSerialMessage(6, 25, 10, 7, 8, 16, 16, 0x84, 7, 24, 0)
			if tc.pause {
				input.LimitCurrentMaxAmps = 0
				set = teslaSerialMessage(6, 25, 10, 7, 8, 6, 16, 0x84, 7, 24, 1)
			}
			var vectors []teslaSerialVector
			if tc.version == "26.34.0" || tc.version == "26.42.0" {
				bound := tc.boundReply
				if bound == nil {
					bound = teslaSerialMessage(6, 6, 10, 2, 8, 20)
				}
				vectors = append(vectors, teslaSerialVector{teslaSerialMessage(6, 5), bound})
			}
			if tc.readReply != nil || tc.status != 0 {
				reply := tc.setReply
				if reply == nil {
					reply = set
				}
				vectors = append(vectors, teslaSerialVector{set, reply})
			}
			if tc.readReply != nil {
				vectors = append(vectors, teslaSerialVector{teslaSerialMessage(6, 27), tc.readReply})
			}
			acq, line := teslaProvisionalSerialAcquisition(t, tc.version, false, vectors)
			got, err := acq.SetProvisionalCurrentLimit(context.Background(), input)
			if len(line.writes) != len(vectors) || got.Comparison != tc.comparison || got.HasValidated {
				t.Fatalf("%d writes, want %d; result=%+v, err=%v", len(line.writes), len(vectors), got, err)
			}
			switch {
			case tc.readReply != nil:
				retained, _ := acq.Retained(FC100OperationWCGetProvisional)
				if err != nil || !got.Acknowledged || got.EffectIndeterminate != (tc.comparison == CurrentLimitUnknown) || got.Set.TerminalTag != 25 || !retained.Current || retained.LastGood.RequestID != got.Readback.RequestID || !bytes.Equal(got.Readback.terminalPayload(), tc.readReply) {
					t.Fatalf("set/readback=%+v, %v", got, err)
				}
			case tc.status != 0:
				if err != nil || got.NoSend || got.Acknowledged || got.Outcome != Gen3ReadEvidenceRejected || !got.Set.HasApplicationStatus || got.Set.ApplicationStatus != tc.status {
					t.Fatalf("refusal=%+v, %v", got, err)
				}
			default:
				if err == nil || !got.NoSend || got.Outcome != Gen3ReadEvidenceRejected {
					t.Fatalf("not rejected before mutation: %+v, %v", got, err)
				}
			}
		})
	}
}

func TestTeslaProvisionalCurrentLimitSerialCompatibilitySet(t *testing.T) {
	read := teslaSerialMessage(6, 27)
	params := []byte{10, 7, 8, 16, 16, 0x84, 7, 24, 0}
	bound := teslaSerialMessage(6, 28, append(bytes.Clone(params), 16, 32)...)
	result := teslaSerialMessage(6, 28, 10, 5, 8, 16, 16, 0x84, 7, 16, 32)
	acq, line := teslaProvisionalSerialAcquisition(t, "24.44.3", false, []teslaSerialVector{
		{read, bound}, {teslaSerialMessage(6, 25, params...), teslaSerialMessage(6, 26)}, {read, result},
	})
	got, err := acq.SetProvisionalCurrentLimit(context.Background(), ProvisionalCurrentLimitRequest{16, 900, false})
	retained, _ := acq.Retained(FC100OperationWCGetProvisional)
	if err != nil || len(line.writes) != 3 || !got.Acknowledged || got.Comparison != CurrentLimitUnknown || got.HasValidated || !retained.Current || !bytes.Equal(retained.LastGood.terminalPayload(), result) {
		t.Fatalf("compatibility exchange = %+v, %v", got, err)
	}
}

func TestTeslaProvisionalCurrentLimitSerialRejectsCompatibilityReplyOnDocumentedBuild(t *testing.T) {
	reply := teslaSerialMessage(6, 26)
	acq, line := teslaProvisionalSerialAcquisition(t, "26.34.0", false, []teslaSerialVector{
		{teslaSerialMessage(6, 5), teslaSerialMessage(6, 6, 10, 2, 8, 20)},
		{teslaSerialMessage(6, 25, 10, 7, 8, 16, 16, 0x84, 7, 24, 0), reply},
	})
	line.allowClassifierError = true
	got, err := acq.SetProvisionalCurrentLimit(context.Background(), ProvisionalCurrentLimitRequest{16, 900, false})
	if err != nil || len(line.writes) != 2 || got.Acknowledged || got.Outcome != Gen3ReadEvidenceIndeterminate || !got.EffectIndeterminate || !bytes.Equal(got.Set.terminalPayload(), reply) {
		t.Fatalf("wrong reply = %+v, %v", got, err)
	}
}

func TestTeslaPersistentSettingsSerialReturnedState(t *testing.T) {
	returned := []byte{10, 4, 8, 12, 0x68, 1}
	reply := teslaSerialMessage(6, 8, returned...)
	acq, line := teslaProvisionalSerialAcquisition(t, "26.34.0", false, []teslaSerialVector{
		{teslaSerialMessage(6, 7, 10, 2, 8, 16), reply},
	}, func(config *Gen3ReadAcquisitionConfig) {
		gen := config.Gen3.config
		gen.PersistentSettingsOperationVersion = HSCFC100OperationCompatibilityV1
		gen.PersistentCurrentOnlyQualified = true
		config.Gen3 = mustTeslaGen3Profile(t, gen)
		config.PersistentIdentityCapacity = 1
		config.Windows[FC100OperationWCConfigureSettings] = Gen3ReadWindow{Freshness: time.Second, Retention: time.Minute}
	})
	input := PersistentSettingsRequest{16, "settings-test", time.Now().Add(time.Second)}
	got, err := acq.ConfigurePersistentCurrent(context.Background(), input)
	if err != nil || len(line.writes) != 1 || !got.Current || !got.HasReturnedCurrent || got.ReturnedMaxOutputCurrentAmps != 12 || got.Comparison != CurrentLimitMismatch || got.Requested != input || !bytes.Equal(got.Receipt.RequestADU(), line.writes[0]) || !bytes.Equal(got.Receipt.terminalPayload(), reply) {
		t.Fatalf("persistent exchange = %+v, %v", got, err)
	}
}
