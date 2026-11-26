package write

import "time"

// Clock is the clock of the writer: the time of its evidence and the wait of the
// renewal.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
