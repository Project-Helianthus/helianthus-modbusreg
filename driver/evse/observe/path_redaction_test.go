package observe

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"time"
)

const redactionTestPath = "/synthetic/serial-port-7"

type pathErrorSource struct{ err error }

func (source pathErrorSource) ReceiveByte(context.Context) (byte, error) { return 0, source.err }
func (pathErrorSource) Close() error                                     { return nil }

func TestTeslaObserveOrdinaryReadErrorCauseCarriesNoPath(t *testing.T) {
	for name, cause := range map[string]error{
		"direct":  &fs.PathError{Op: "read", Path: redactionTestPath, Err: syscall.EIO},
		"wrapped": errors.Join(errors.New("wrapper"), &fs.PathError{Op: "read", Path: redactionTestPath, Err: syscall.EIO}),
	} {
		t.Run(name, func(t *testing.T) {
			endpoint, err := NewEndpoint("synthetic-e", 1, pathErrorSource{err: cause}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := endpoint.Observe(context.Background(), Request{ExpectedGeneration: 1, Deadline: time.Now().Add(time.Second),
				ClockEpoch: "c", RequestedBytes: 1, SegmentCap: 1, SegmentGap: time.Millisecond})
			if err != nil || observation.Termination != TerminationReadError {
				t.Fatalf("err=%v termination=%v", err, observation.Termination)
			}
			if strings.Contains(observation.Cause, redactionTestPath) || !strings.Contains(observation.Cause, "serial read") {
				t.Fatalf("cause %q", observation.Cause)
			}
		})
	}
}

func TestTeslaObserveOrdinaryOpenErrorCauseCarriesNoPath(t *testing.T) {
	harness := &runHarness{clock: &syntheticClock{base: time.Unix(0, 0)}}
	config := newRunConfig(harness, CaseInitial, 1)
	config.Open = func() (ByteSource, error) {
		return nil, &fs.PathError{Op: "open", Path: redactionTestPath, Err: syscall.ENOENT}
	}
	cause := runAttempt(context.Background(), config, config.Operator, 1).OpenError
	if strings.Contains(cause, redactionTestPath) || !strings.Contains(cause, "serial open") {
		t.Fatalf("open error %q", cause)
	}
}

func TestTeslaObserveBoundedCauseRedactsBeforeTruncating(t *testing.T) {
	long := &fs.PathError{Op: "read", Path: redactionTestPath + strings.Repeat("p", 4*maxCauseBytes), Err: syscall.EIO}
	got := boundedCause(long)
	if strings.Contains(got, "synthetic") || len(got) > maxCauseBytes {
		t.Fatalf("cause %q", got)
	}
}
