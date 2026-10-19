package wallclock

import "time"

func requireFastWithoutTesting() {
	start := time.Now()
	Work()
	if time.Since(start) > time.Second { // want "test asserts on wall-clock elapsed time"
		panic("too slow")
	}
}
