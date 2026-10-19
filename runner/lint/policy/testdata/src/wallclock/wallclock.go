package wallclock

import "time"

// Clock is an injected clock, the alternative LV1007 suggests.
type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
}

func Work() {}

// Slow is outside a test file, so its timing is not an assertion.
func Slow() bool {
	start := time.Now()
	Work()
	return time.Since(start) > time.Second
}
