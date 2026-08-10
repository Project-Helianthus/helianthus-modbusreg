package modbusreg_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	reg "github.com/Project-Helianthus/helianthus-modbusreg"
)

func TestConcurrentDetectionsAreDeterministicAndIndependent(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.concurrent",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, err := reg.NewCatalog(profile)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, false))
	reader := detectionReader(t)
	const workers = 32
	start := make(chan struct{})
	results := make(chan []byte, workers)
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wait.Done()
			<-start
			decision, detectErr := detector.Detect(
				context.Background(),
				reader,
				reg.DetectionOptions{},
			)
			if detectErr != nil {
				errorsFound <- detectErr
				return
			}
			encoded, marshalErr := reg.MarshalDetectionDecision(decision)
			if marshalErr != nil {
				errorsFound <- marshalErr
				return
			}
			results <- encoded
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent detection: %v", err)
	}
	var canonical []byte
	count := 0
	for encoded := range results {
		count++
		if canonical == nil {
			canonical = encoded
			continue
		}
		if !reflect.DeepEqual(encoded, canonical) {
			t.Fatal("concurrent detection produced non-deterministic evidence")
		}
	}
	if count != workers {
		t.Fatalf("decisions=%d want %d", count, workers)
	}
	if reads := reader.declarationOrder(); len(reads) != workers*3 {
		t.Fatalf("reads=%d want %d", len(reads), workers*3)
	}
}

type cancellingProbeReader struct {
	mu    sync.Mutex
	reads int
}

func (reader *cancellingProbeReader) ReadProbe(
	ctx context.Context,
	_ reg.ProbeReadRequest,
) (reg.ProbeReadResult, error) {
	reader.mu.Lock()
	reader.reads++
	reader.mu.Unlock()
	<-ctx.Done()
	return reg.ProbeReadResult{}, ctx.Err()
}

func (reader *cancellingProbeReader) readCount() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.reads
}

func TestDetectionCancellationStopsTheBoundedPlan(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.cancelled-detection",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, _ := reg.NewCatalog(profile)
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, false))
	reader := &cancellingProbeReader{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := detector.Detect(ctx, reader, reg.DetectionOptions{})
	if !errors.Is(err, context.Canceled) ||
		decision.Outcome() != reg.DetectionNoMatch ||
		decision.Reason() != reg.DetectionReasonContextCancelled ||
		decision.SelectedProfileID() != "" {
		t.Fatalf("cancelled detection=(%+v,%v)", decision, err)
	}
	if reads := reader.readCount(); reads > 1 {
		t.Fatalf("cancelled detection continued for %d reads", reads)
	}
}

type orderedBlockingReader struct {
	mu      sync.Mutex
	results map[string]reg.ProbeReadResult
	entered chan string
	release chan struct{}
	reads   []string
}

func (reader *orderedBlockingReader) ReadProbe(
	ctx context.Context,
	request reg.ProbeReadRequest,
) (reg.ProbeReadResult, error) {
	reader.mu.Lock()
	reader.reads = append(reader.reads, request.DeclarationID())
	result := reader.results[request.DeclarationID()]
	reader.mu.Unlock()
	select {
	case reader.entered <- request.DeclarationID():
	case <-ctx.Done():
		return reg.ProbeReadResult{}, ctx.Err()
	}
	select {
	case <-reader.release:
		return result, nil
	case <-ctx.Done():
		return reg.ProbeReadResult{}, ctx.Err()
	}
}

func TestOneDetectionNeverExecutesProbeDeclarationsConcurrently(t *testing.T) {
	profile := detectionProfile(
		t,
		"example.standard.ordered-probes",
		"1.0.0",
		reg.MaturityQualified,
		reg.ProfileActive,
		true,
	)
	catalog, _ := reg.NewCatalog(profile)
	detector := newDetector(t, catalog, detectionCandidate(t, profile, 10, true, false))
	base := detectionReader(t)
	reader := &orderedBlockingReader{
		results: base.results,
		entered: make(chan string),
		release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		decision, err := detector.Detect(context.Background(), reader, reg.DetectionOptions{})
		if err == nil && decision.Outcome() != reg.DetectionMatched {
			err = errors.New("detection did not match")
		}
		done <- err
	}()
	for _, want := range []string{
		"manufacturer-identity",
		"model-identity",
		"firmware-identity",
	} {
		if got := <-reader.entered; got != want {
			t.Fatalf("probe order=%q want %q", got, want)
		}
		reader.release <- struct{}{}
	}
	if err := <-done; err != nil {
		t.Fatalf("Detect: %v", err)
	}
}
