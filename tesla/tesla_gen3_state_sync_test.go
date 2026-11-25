package tesla

import (
	"bytes"
	"context"
	"testing"
)

func TestTeslaNativeCommandStateSync(t *testing.T) {
	for _, firmware := range []string{"MID 25.2.2", "26.18.0", "26.34.0", "24.28.3", "24.44.3", "26.42.0", "26.34.1"} {
		t.Run(firmware, func(t *testing.T) {
			if firmware == "24.28.3" {
				if _, err := EncodeTeslaStateSync(firmware, StateSyncRequest{}); err == nil {
					t.Fatal("undocumented layout was encoded")
				}
				return
			}
			h := newTeslaNativeCommandHarness(t, firmware)
			ep := h.endpoint()
			input := StateSyncRequest{Report: &SiteControllerReport{
				FieldOne: teslaNativeTestPointer(uint32(1)), FieldFour: teslaNativeTestPointer(uint32(4)),
				FieldFive: teslaNativeTestPointer(uint32(5)), FieldSix: teslaNativeTestPointer(uint32(6)), ScalarGroup: &SiteControllerScalarGroup{FieldOne: teslaNativeTestPointer(uint32(0x12345678))},
				Measurements: &SiteControllerMeasurements{
					FieldOne: teslaNativeTestPointer(uint32(17)), FieldTwo: teslaNativeTestPointer(uint32(18)),
					FieldThree: teslaNativeTestPointer(uint32(19)), FieldFour: teslaNativeTestPointer(uint32(20)),
					FieldFive: teslaNativeTestPointer(uint32(21)), FieldSix: teslaNativeTestPointer(uint32(22)),
					FieldSeven: teslaNativeTestPointer(true), FieldEight: teslaNativeTestPointer(false), FieldNine: teslaNativeTestPointer(true),
				},
			}}
			switch firmware {
			case "MID 25.2.2", "26.18.0", "26.34.0":
			default:
				receipt, reply, err := h.acquisition.ExchangeStateSync(context.Background(), input)
				if err == nil || receipt.State != Gen3ReadEvidenceNoSend || reply.Snapshot != nil || ep.exchanges() != 0 {
					t.Fatalf("unsupported firmware: %+v %+v %v", receipt, reply, err)
				}
				return
			}
			want := []byte{49, 10, 47, 8, 1, 18, 30, 8, 17, 16, 18, 29, 19, 0, 0, 0, 37, 20, 0, 0, 0, 45, 21, 0, 0, 0, 53, 22, 0, 0, 0, 56, 1, 64, 0, 72, 1, 26, 5, 13, 0x78, 0x56, 0x34, 0x12, 32, 4, 40, 5, 48, 6}
			terminal := []byte{19, 10, 15, 8, 11, 18, 9, 34, 7, 13, 0x78, 0x56, 0x34, 0x12, 16, 12, 40, 15, 16, 42}
			terminal = append(terminal, 120, 1)
			terminal[0] += 2
			ep.push(teslaGen3FakeStep{terminal: terminal})
			receipt, reply, err := h.acquisition.ExchangeStateSync(context.Background(), input)
			if err != nil || receipt.State != Gen3ReadEvidenceAck || reply.Snapshot == nil || reply.Snapshot.FieldOne == nil || *reply.Snapshot.FieldOne != 11 || reply.SnapshotAgeTicks == nil || *reply.SnapshotAgeTicks != 42 || !bytes.Equal(reply.Payload(), terminal) {
				t.Fatalf("reply: %+v %+v %v", receipt, reply, err)
			}
			group := reply.Snapshot.FourValues
			if group == nil || (*group)[3] == nil || (*group)[3].Value == nil || *(*group)[3].Value != 0x12345678 || (*group)[3].Code == nil || *(*group)[3].Code != 12 {
				t.Fatalf("four value group: %+v", group)
			}
			if field := reply.Snapshot.FieldFive; (field != nil) != (firmware == "26.34.0") || field != nil && *field != 15 {
				t.Fatalf("unexpected firmware field: %v", field)
			}
			if ep.exchanges() != 1 || ep.requests[0].FunctionCode().Byte() != 102 || !bytes.Equal(ep.requests[0].Payload(), want) {
				t.Fatalf("request: %x, want %x", ep.requests[0].Payload(), want)
			}
			ep.push(teslaGen3FakeStep{outcome: "exception", status: 4})
			receipt, reply, err = h.acquisition.ExchangeStateSync(context.Background(), input)
			if err != nil || receipt.State != Gen3ReadEvidenceRejected || !receipt.HasExceptionStatus || receipt.ExceptionStatus != 4 || reply.Snapshot != nil || ep.exchanges() != 2 {
				t.Fatalf("exception: %+v %+v %v", receipt, reply, err)
			}
		})
	}
}

func TestTeslaNativeCommandStateSyncReportPresence(t *testing.T) {
	for _, tc := range []struct {
		report *SiteControllerReport
		want   []byte
	}{
		{nil, []byte{0}}, {&SiteControllerReport{}, []byte{2, 10, 0}},
		{&SiteControllerReport{ScalarGroup: &SiteControllerScalarGroup{}}, []byte{4, 10, 2, 26, 0}},
	} {
		request, err := EncodeTeslaStateSync("26.34.0", StateSyncRequest{Report: tc.report})
		if err != nil || !bytes.Equal(request.Payload(), tc.want) {
			t.Fatalf("report presence: %x %v", request.Payload(), err)
		}
	}
}

func teslaNativeTestPointer[T any](value T) *T { return &value }
