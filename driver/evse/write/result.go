package write

import (
	"time"

	"github.com/Project-Helianthus/helianthus-modbusreg/tesla"
	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// Level is the route outcome of one command.
type Level string

// The levels of a command. A refusal before the send is failed_no_contact with
// its reason; a mismatch between the returned t28 and the request stays a
// conflict or an acknowledgement and is never readback_confirmed.
const (
	LevelFailedNoContact   Level = "failed_no_contact"
	LevelProtocolAck       Level = "protocol_ack"
	LevelReadbackConfirmed Level = "readback_confirmed"
	LevelConflict          Level = "conflict"
	LevelNoEffect          Level = "no_effect"
	LevelUnknown           Level = "unknown"
)

// Result is the evidence of one command. The native request that was sent and
// the t28 that was returned are kept apart; neither stands for the other.
type Result struct {
	Level   Level
	Reason  string
	Release bool

	// Sent is the t25 request as encoded; HasSent is false when no t25 was sent.
	Sent    tesla.ProvisionalCurrentLimitRequest
	HasSent bool
	// Returned is the t28 read back after the t26; HasReturned is false when
	// none was.
	Returned    tesla.ProvisionalCurrentLimitReadback
	HasReturned bool
	// Matches reports that the returned current, timeout and inhibit equal the
	// request as the native comparison states.
	Matches bool
	// Published is the allocated current the returned t28 gives; it is absent
	// for a returned zero current with inhibit false.
	Published    uint32
	HasPublished bool

	Native tesla.ProvisionalCurrentLimitResult
	// Prerequisite is the t27 → t28 read this command made to learn the
	// configured current; HasPrerequisite is false when the latest t28 of the
	// generation was used.
	Prerequisite    tesla.Gen3ReadRecord
	HasPrerequisite bool
	// Configured is the configured current the checks used.
	Configured    uint32
	HasConfigured bool

	Started    time.Time
	Dispatched time.Time
	AckAt      time.Time

	// Snapshot and Candidate are the exact later snapshot and the allocated
	// current candidate of this readback; absent for a release.
	Snapshot  *semreg.Snapshot
	Candidate *semreg.FactCandidate

	PublicationErr error
}

// LimitRequestSent reports that a t25 request was encoded and possibly transmitted. A
// native exchange that failed in its prerequisite t27 read encoded no t25.
func (r Result) LimitRequestSent() bool { return r.HasSent && r.Native.PossiblyTransmitted }

func refused(reason string) Result { return Result{Level: LevelFailedNoContact, Reason: reason} }

func publishedOf(rb tesla.ProvisionalCurrentLimitReadback) (uint32, bool) {
	switch {
	case !rb.HasInhibit:
		return 0, false
	case rb.InhibitCharging:
		return 0, true
	case rb.HasLimitCurrent && rb.LimitCurrentMaxAmps != 0:
		return rb.LimitCurrentMaxAmps, true
	}
	return 0, false
}
