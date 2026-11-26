package observe

import (
	"strings"
	"time"
)

func priorAttemptMutations() []priorMutation {
	long := strings.Repeat("x", maxCauseBytes+1)
	return []priorMutation{
		// Termination and its cause.
		{"unknown termination", attemptCaptured, func(a *Attempt) { a.Observation.Termination = "bogus" }, "termination"},
		{"empty termination", attemptCaptured, func(a *Attempt) { a.Observation.Termination = "" }, "termination"},
		{"closed termination", attemptCaptured, func(a *Attempt) { a.Observation.Termination = TerminationClosed }, "termination"},
		{"generation_lost termination", attemptDeadline, func(a *Attempt) { a.Observation.Termination = TerminationGenerationLost }, "termination"},
		{"partial window called captured", attemptDeadline, func(a *Attempt) { a.Observation.Termination = TerminationCaptured }, "short of the requested"},
		{"segment_bound window called captured", attemptSegmentBound, func(a *Attempt) { a.Observation.Termination = TerminationCaptured }, "short of the requested"},
		{"captured with overflow", attemptCaptured, func(a *Attempt) {
			a.Observation.Coverage.Overflow, a.Observation.Coverage.OverflowOffsets = []byte{1}, []time.Duration{ms(9)}
			a.Observation.Coverage.CapturedBytes = a.Observation.Coverage.RequestedBytes
		}, "overflow byte without"},
		{"captured at its deadline", attemptCaptured, func(a *Attempt) { a.Observation.Return = a.Observation.Deadline }, "at or after its deadline"},
		{"full window called deadline", attemptCaptured, func(a *Attempt) { a.Observation.Termination = TerminationDeadline }, "no read that could have ended"},
		{"segment_bound called deadline", attemptSegmentBound, func(a *Attempt) { a.Observation.Termination = TerminationDeadline }, "no read that could have ended"},
		{"full window called segment_bound", attemptCaptured, func(a *Attempt) { a.Observation.Termination = TerminationSegmentBound }, "segment_bound without"},
		{"segment_bound at its deadline", attemptSegmentBound, func(a *Attempt) { a.Observation.Return = a.Observation.Deadline }, "at or after its deadline"},
		{"full window called read_error", attemptCaptured, func(a *Attempt) {
			a.Observation.Termination, a.Observation.Cause = TerminationReadError, "synthetic"
		}, "read could not have reached"},
		{"read_error without a cause", attemptReadError, func(a *Attempt) { a.Observation.Cause = "" }, "read_error without a cause"},
		{"read_error with a long cause", attemptReadError, func(a *Attempt) { a.Observation.Cause = long }, "cause is longer"},
		{"read_error at its deadline", attemptReadError, func(a *Attempt) { a.Observation.Return = a.Observation.Deadline }, "at or after its deadline"},
		{"cause on a captured window", attemptCaptured, func(a *Attempt) { a.Observation.Cause = "synthetic" }, "cause beside a termination"},
		{"cause on a deadline", attemptDeadline, func(a *Attempt) { a.Observation.Cause = "synthetic" }, "cause beside a termination"},
		{"read_error called captured", attemptReadError, func(a *Attempt) { a.Observation.Termination, a.Observation.Cause = TerminationCaptured, "" }, "short of the requested"},

		// Identity and enumerated fields.
		{"instance of another attempt", attemptCaptured, func(a *Attempt) { a.Observation.Instance = 2 }, "attempt number"},
		{"instance zero", attemptDeadline, func(a *Attempt) { a.Observation.Instance = 0 }, "attempt number"},
		{"generation", attemptCaptured, func(a *Attempt) { a.Observation.Generation = 2 }, "generation"},
		{"observation id", attemptCaptured, func(a *Attempt) { a.Observation.ID = 2 }, "observation id"},
		{"clock epoch of another attempt", attemptCaptured, func(a *Attempt) { a.Observation.ClockEpoch = clockEpochPrefix + "a02" }, "clock epoch"},
		{"clock epoch free text", attemptCaptured, func(a *Attempt) { a.Observation.ClockEpoch = "synthetic-c" }, "clock epoch"},
		{"deadline zero", attemptCaptured, func(a *Attempt) { a.Observation.Deadline = 0 }, "deadline offset"},
		{"deadline above bound", attemptCaptured, func(a *Attempt) { a.Observation.Deadline = MaxObservationWindow + 1 }, "deadline offset"},
		{"deadline negative", attemptCaptured, func(a *Attempt) { a.Observation.Deadline = -ms(1) }, "deadline offset"},
		{"return negative", attemptCaptured, func(a *Attempt) { a.Observation.Return = -1 }, "return offset"},
		{"timing resolution", attemptCaptured, func(a *Attempt) { a.Observation.Coverage.TimingResolution = "wire" }, "timing resolution"},
		{"framing result", attemptCaptured, func(a *Attempt) { a.Observation.Coverage.Framing = "rtu_crc_valid" }, "framing result"},
		{"limitations removed", attemptCaptured, func(a *Attempt) { a.Observation.Coverage.Limitations = nil }, "limitations"},
		{"limitation added", attemptCaptured, func(a *Attempt) {
			a.Observation.Coverage.Limitations = append(a.Observation.Coverage.Limitations, "sender attribution is Proven")
		}, "limitations"},
		{"limitation reworded", attemptCaptured, func(a *Attempt) { a.Observation.Coverage.Limitations[0] = "sender is the device" }, "limitations"},

		// Attempt shape and lifecycle replay.
		{"case", attemptCaptured, func(a *Attempt) { a.Case = "reset" }, "case"},
		{"case empty", attemptDeadline, func(a *Attempt) { a.Case = "" }, "case"},
		{"observation removed", attemptCaptured, func(a *Attempt) { a.Observation = nil }, "lifecycle replay without an observation"},
		{"observation and lifecycle removed", attemptCaptured, func(a *Attempt) { a.Observation, a.Lifecycle = nil, nil }, "neither an observation"},
		{"open error beside an observation", attemptCaptured, func(a *Attempt) { a.OpenError = "synthetic" }, "open error beside"},
		{"lifecycle removed", attemptCaptured, func(a *Attempt) { a.Lifecycle = nil }, "without a lifecycle replay"},
		{"open error removed", attemptOpenError, func(a *Attempt) { a.OpenError = "" }, "neither an observation"},
		{"open error with a replay", attemptOpenError, func(a *Attempt) { a.Lifecycle = &LifecycleReplay{Disposition: LifecycleReplayDisposition} }, "lifecycle replay without an observation"},
		{"open error long", attemptOpenError, func(a *Attempt) { a.OpenError = long }, "open error is longer"},
		{"replay disposition", attemptCaptured, func(a *Attempt) { a.Lifecycle.Disposition = "activation evidence" }, "disposition"},
		{"replay state unknown", attemptCaptured, func(a *Attempt) { a.Lifecycle.State = "bogus" }, "lifecycle state"},
		{"replay state empty", attemptCaptured, func(a *Attempt) { a.Lifecycle.State = "" }, "lifecycle state"},
		{"replay readiness flipped", attemptCaptured, func(a *Attempt) { a.Lifecycle.Ready = !a.Lifecycle.Ready }, "readiness disagrees"},
		{"replay ready with a refused acquisition", attemptDeadline, func(a *Attempt) {
			a.Lifecycle.Ready, a.Lifecycle.State = true, "ready"
		}, "ready yet records a refused acquisition"},
		{"replay ready for a partial window", attemptDeadline, func(a *Attempt) {
			a.Lifecycle.Ready, a.Lifecycle.State, a.Lifecycle.AcquireRefused = true, "ready", false
		}, "ready for an incomplete window"},
		{"replay ready for a captured passive window", attemptCaptured, func(a *Attempt) {
			a.Lifecycle.Ready, a.Lifecycle.State, a.Lifecycle.AcquireRefused, a.Lifecycle.Error = true, "ready", false, ""
		}, "ready for a passive observation"},
		{"replay not constructed without an error", attemptCaptured, func(a *Attempt) {
			a.Lifecycle.State, a.Lifecycle.Ready, a.Lifecycle.AcquireRefused, a.Lifecycle.Error = lifecycleNotConstructed, false, false, ""
		}, "never constructed"},
		{"replay frames negative", attemptCaptured, func(a *Attempt) { a.Lifecycle.FramesSent = -1 }, "frame count"},
		{"replay error long", attemptCaptured, func(a *Attempt) { a.Lifecycle.Error = long }, "replay error is longer"},
	}
}
