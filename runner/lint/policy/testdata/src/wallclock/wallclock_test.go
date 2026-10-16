package wallclock

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpperBound(t *testing.T) {
	start := time.Now()
	Work()
	elapsed := time.Since(start)
	if elapsed > 3600*time.Millisecond { // want "test asserts on wall-clock elapsed time"
		t.Fatalf("took %v", elapsed)
	}
}

func TestLowerAndUpperBound(t *testing.T) {
	start := time.Now()
	Work()
	if elapsed := time.Since(start); elapsed < 750*time.Millisecond || elapsed > time.Second { // want "test asserts on wall-clock elapsed time"
		t.Errorf("took %v", elapsed)
	}
}

func TestInlineSince(t *testing.T) {
	start := time.Now()
	Work()
	if time.Since(start) > time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
}

func TestBoundAmongOtherConditions(t *testing.T) {
	start := time.Now()
	err := context.Canceled
	if err == nil || time.Since(start) > 10*time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatalf("err = %v", err)
	}
}

func TestNowSub(t *testing.T) {
	start := time.Now()
	Work()
	end := time.Now().UTC()
	if end.Sub(start) >= time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
	if time.Now().Sub(start) >= 2*time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
}

func TestDerivedValues(t *testing.T) {
	start := time.Now()
	Work()
	ms := time.Since(start).Milliseconds()
	if ms > 100 { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
	seconds := float64(time.Since(start)) / float64(time.Second)
	if seconds > 0.5 { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
	var slowest time.Duration
	slowest = max(slowest, time.Since(start))
	if slowest > time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
}

func TestFailsInElse(t *testing.T) {
	start := time.Now()
	Work()
	if time.Since(start) < time.Second { // want "test asserts on wall-clock elapsed time"
		t.Log("fast enough")
	} else {
		t.Fatal("too slow")
	}
}

func TestTableDriven(t *testing.T) {
	for _, budget := range []time.Duration{time.Second, 2 * time.Second} {
		start := time.Now()
		Work()
		if time.Since(start) > budget { // want "test asserts on wall-clock elapsed time"
			t.Errorf("over %v", budget)
		}
	}
}

func TestInSubtest(t *testing.T) {
	t.Run("work", func(t *testing.T) {
		start := time.Now()
		Work()
		if time.Since(start) > time.Second { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
	})
}

func TestAssertionLibrary(t *testing.T) {
	start := time.Now()
	Work()
	elapsed := time.Since(start)
	assert.Less(t, elapsed, time.Second)                // want "test asserts on wall-clock elapsed time"
	assert.GreaterOrEqual(t, elapsed, time.Millisecond) // want "test asserts on wall-clock elapsed time"
	assert.Lessf(t, elapsed, time.Second, "took %v", 1) // want "test asserts on wall-clock elapsed time"
	assert.InDelta(t, 1.0, elapsed.Seconds(), 0.1)      // want "test asserts on wall-clock elapsed time"
	assert.True(t, elapsed < time.Second)               // want "test asserts on wall-clock elapsed time"
	assert.New(t).Less(elapsed, time.Second)            // want "test asserts on wall-clock elapsed time"
	checkWithin(t, time.Since(start) <= 2*time.Second)  // want "test asserts on wall-clock elapsed time"
	assert.NotZero(t, elapsed)
}

func checkWithin(t *testing.T, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("out of range")
	}
}

// A helper that measures and asserts is reported where it asserts.
func requireFast(t *testing.T, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if time.Since(start) > time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
}

func TestLoggingOnly(t *testing.T) {
	start := time.Now()
	Work()
	elapsed := time.Since(start)
	t.Logf("took %v", elapsed)
	if elapsed > time.Second {
		t.Logf("slow run: %v", elapsed)
	}
	t.Logf("fast: %v", elapsed < time.Second)
}

func TestSkipsWhenSlow(t *testing.T) {
	start := time.Now()
	Work()
	if time.Since(start) > time.Second {
		t.Skip("machine too slow for the rest")
	}
}

func TestSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		time.Sleep(time.Second)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("took %v", elapsed)
		}
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

func TestSynctestNamedBody(t *testing.T) {
	synctest.Test(t, bubbleBody)
}

func bubbleBody(t *testing.T) {
	start := time.Now()
	time.Sleep(time.Second)
	if time.Since(start) != time.Second {
		t.Fatal("clock moved unexpectedly")
	}
	if time.Since(start) > time.Second {
		t.Fatal("too slow")
	}
}

func checkInjectedClock(t *testing.T, clock Clock) {
	start := clock.Now()
	Work()
	if clock.Since(start) > time.Second {
		t.Fatal("too slow")
	}
	end := clock.Now()
	if end.Sub(start) > time.Second {
		t.Fatal("too slow")
	}
}

// The recommended pattern: the stub reads the deadline it was given and the
// clock around the call, and the test compares those, not how long it took.
func TestDeadlineSeenByStub(t *testing.T) {
	const timeout = time.Second
	var deadline time.Time
	before := time.Now()
	stub := func(ctx context.Context) {
		deadline, _ = ctx.Deadline()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stub(ctx)
	after := time.Now()
	if deadline.Sub(before) < timeout {
		t.Errorf("deadline %v after the call, want at least %v", deadline.Sub(before), timeout)
	}
	if deadline.Sub(after) > timeout {
		t.Errorf("deadline too late")
	}
}

// start is not a reading of the real clock once it is also set from a fake one.
func checkReadingMixedWithFake(t *testing.T, clock Clock) {
	start := time.Now()
	start = clock.Now()
	end := time.Now()
	if end.Sub(start) > time.Second {
		t.Fatal("too slow")
	}
}

// A bound inside a for loop is how long a test waits for something before
// giving up, not a claim about how long the code takes.
func TestPollingTimeout(t *testing.T) {
	start := time.Now()
	for !done() {
		if time.Since(start) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
		time.Sleep(time.Millisecond)
	}
	for time.Since(start) < time.Second {
		Work()
	}
}

func done() bool {
	return true
}

func BenchmarkWork(b *testing.B) {
	start := time.Now()
	Work()
	if time.Since(start) > time.Second {
		b.Fatal("too slow")
	}
}

func TestBenchmarkLiteral(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) {
		start := time.Now()
		Work()
		if time.Since(start) > time.Second {
			b.Fatal("too slow")
		}
	})
	if result.N == 0 {
		t.Fatal("did not run")
	}
}

func TestEqualityIsNotABound(t *testing.T) {
	start := time.Now()
	if time.Since(start) == 0 {
		t.Fatal("clock did not move")
	}
}

func TestUntilDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	if time.Until(deadline) < 0 {
		t.Fatal("deadline already passed")
	}
}
