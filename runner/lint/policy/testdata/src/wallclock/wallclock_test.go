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

// Only the compared values count; a duration or comparison in the message
// arguments is not asserted on.
func TestMessageArguments(t *testing.T) {
	start := time.Now()
	results := []int{1}
	assert.Less(t, len(results), 10, "after %v", time.Since(start))
	assert.Equal(t, 1, len(results), "fast=%v", time.Since(start) < time.Second)
	assert.New(t).Lessf(len(results), 10, "after %v", time.Since(start))
	assert.New(t).Lessf(time.Since(start), time.Second, "too slow") // want "test asserts on wall-clock elapsed time"
}

// A loop that restarts the clock each pass measures each pass, so its bound
// is an assertion, not a polling timeout.
func TestRepeatedMeasurement(t *testing.T) {
	for i := 0; i < 3; i++ {
		start := time.Now()
		Work()
		if time.Since(start) > 10*time.Millisecond { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
	}
	var start time.Time
	for attempt := 0; attempt < 3; attempt++ {
		start = time.Now()
		Work()
		elapsed := time.Since(start)
		assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
	}
}

// The innermost loop decides: a table loop inside a retry loop is still a
// table loop.
func TestTableInsideCounterLoop(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		for _, budget := range []time.Duration{time.Second} {
			start := time.Now()
			Work()
			if time.Since(start) > budget { // want "test asserts on wall-clock elapsed time"
				t.Errorf("over %v", budget)
			}
		}
	}
}

// A polling timeout written as an assertion call is exempt like the if form.
func TestPollingWithAssertion(t *testing.T) {
	start := time.Now()
	for !done() {
		assert.Less(t, time.Since(start), 5*time.Second)
		elapsed := time.Since(start)
		assert.True(t, elapsed < 5*time.Second)
		time.Sleep(time.Millisecond)
	}
}

func TestSynctestHelper(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		requireFastInBubble(t)
	})
}

// requireFastInBubble is only reached from a bubble, so its clock is fake.
func requireFastInBubble(t *testing.T) {
	t.Helper()
	start := time.Now()
	time.Sleep(time.Second)
	elapsedInBubble(t, start)
}

// elapsedInBubble is bubble code two calls deep.
func elapsedInBubble(t *testing.T, start time.Time) {
	t.Helper()
	if time.Since(start) > time.Second {
		t.Fatal("too slow")
	}
}

func TestSynctestHeldLiteral(t *testing.T) {
	body := func(t *testing.T) {
		start := time.Now()
		time.Sleep(time.Second)
		if time.Since(start) > time.Second {
			t.Fatal("too slow")
		}
	}
	synctest.Test(t, body)
}

type suite struct {
	budget time.Duration
}

func (s suite) run(t *testing.T) {
	start := time.Now()
	time.Sleep(s.budget)
	if time.Since(start) > s.budget {
		t.Fatal("too slow")
	}
}

func TestSynctestMethodValue(t *testing.T) {
	s := suite{budget: time.Second}
	synctest.Test(t, s.run)
}

// A variable that is also set from a call with several results, a range
// variable, or a store inside a bubble may hold a fake reading, so it is not a
// reading of the real clock.
func checkReadingFromTuple(t *testing.T, now func() (time.Time, error)) {
	start := time.Now()
	start, _ = now()
	if time.Now().Sub(start) > time.Second {
		t.Fatal("too slow")
	}
}

func checkReadingFromRange(t *testing.T, readings []time.Time) {
	start := time.Now()
	for _, start = range readings {
	}
	if time.Now().Sub(start) > time.Second {
		t.Fatal("too slow")
	}
}

func TestReadingSetInBubble(t *testing.T) {
	start := time.Now()
	synctest.Test(t, func(t *testing.T) {
		start = time.Now()
	})
	if time.Now().Sub(start) > time.Second {
		t.Fatal("too slow")
	}
}

func TestSynctestDeclaredLiteral(t *testing.T) {
	var body = func(t *testing.T) {
		start := time.Now()
		time.Sleep(time.Second)
		if time.Since(start) > time.Second {
			t.Fatal("too slow")
		}
	}
	synctest.Test(t, body)
}

// Readings held by declared variables count like those from :=, and a
// declaration inside a loop restarts the clock each pass.
func TestDeclaredReadings(t *testing.T) {
	var start = time.Now()
	Work()
	var end time.Time
	end = time.Now()
	if end.Sub(start) > time.Second { // want "test asserts on wall-clock elapsed time"
		t.Fatal("too slow")
	}
	for i := 0; i < 3; i++ {
		var begin = time.Now()
		Work()
		if time.Now().Sub(begin) > time.Second { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
	}
}

// A clock restarted in the post statement is restarted each pass.
func TestRestartedInPost(t *testing.T) {
	start := time.Now()
	for i := 0; i < 3; i, start = i+1, time.Now() {
		Work()
		if time.Since(start) > time.Second { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
	}
	for begin := time.Now(); !done(); {
		if time.Since(begin) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
	}
}

// A pass may read the clock for the end of a measurement; only restarting
// the start makes the bound a measurement of the pass.
func TestPollingReadsNow(t *testing.T) {
	start := time.Now()
	for !done() {
		now := time.Now()
		if now.Sub(start) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
	}
}

// A range over a channel, such as a ticker's, polls like a condition loop.
func TestPollingTicker(t *testing.T) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	start := time.Now()
	for range ticker.C {
		if done() {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
	}
	for range ticker.C {
		begin := time.Now()
		Work()
		if time.Since(begin) > time.Second { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
		break
	}
}

// A goroutine or callback in the loop runs when called, not as part of a pass,
// so a reading assigned there does not restart the loop's clock.
func TestPollingWithCallback(t *testing.T) {
	start := time.Now()
	reset := func() {}
	for !done() {
		reset = func() { start = time.Now() }
		if time.Since(start) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
	}
	reset()
}

type timedSuite struct {
	t *testing.T
}

func (s timedSuite) T() *testing.T {
	return s.t
}

// A testify function's TestingT need not be a test variable.
func (s timedSuite) checkFast() {
	start := time.Now()
	Work()
	elapsed := time.Since(start)
	assert.True(s.T(), elapsed < time.Second)   // want "test asserts on wall-clock elapsed time"
	assert.Greater(s.T(), time.Second, elapsed) // want "test asserts on wall-clock elapsed time"
	assert.Less(s.T(), 1, 2, "after %v", elapsed)
}

func checkEventually(c *assert.CollectT, start time.Time) {
	assert.True(c, time.Since(start) < time.Second) // want "test asserts on wall-clock elapsed time"
}

// An assertion call in an if condition is reported once, as the call.
func TestAssertionInCondition(t *testing.T) {
	start := time.Now()
	Work()
	if !assert.True(t, time.Since(start) < time.Second) { // want "test asserts on wall-clock elapsed time"
		t.FailNow()
	}
}

// A failure in a later arm of an else-if chain depends on the bound too.
func TestFailsInElseIf(t *testing.T) {
	start := time.Now()
	Work()
	elapsed := time.Since(start)
	ok := elapsed != 0
	if elapsed < time.Second { // want "test asserts on wall-clock elapsed time"
		t.Log("fast")
	} else if ok {
		t.Log("slow")
	} else {
		t.Fatal("clock did not move")
	}
}

func TestSynctestLocalHelper(t *testing.T) {
	check := func(t *testing.T) {
		start := time.Now()
		time.Sleep(time.Second)
		if time.Since(start) > time.Second {
			t.Fatal("too slow")
		}
	}
	var nested = func(t *testing.T) { check(t) }

	synctest.Test(t, func(t *testing.T) { nested(t) })
}

type genericSuite[T any] struct{}

func (genericSuite[T]) check(t *testing.T) {
	start := time.Now()
	time.Sleep(time.Second)
	if time.Since(start) > time.Second {
		t.Fatal("too slow")
	}
}

func TestSynctestGenericMethod(t *testing.T) {
	s := genericSuite[int]{}
	synctest.Test(t, s.check)
}

func TestPollingCopiedStart(t *testing.T) {
	start := time.Now()
	for !done() {
		begin := start.UTC()
		var copied = begin
		if time.Since(copied) > 5*time.Second {
			t.Fatal("timed out waiting")
		}
	}
}

func TestCopiedRestartedStart(t *testing.T) {
	for !done() {
		start := time.Now()
		begin := start
		if time.Since(begin) > time.Second { // want "test asserts on wall-clock elapsed time"
			t.Fatal("too slow")
		}
	}
}

func TestOverwrittenDuration(t *testing.T) {
	start := time.Now()
	elapsed := time.Since(start)
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"

	elapsed = 42 * time.Millisecond
	if elapsed > time.Second {
		t.Fatal("bad fixed duration")
	}
	assert.Less(t, elapsed, time.Second)
	copied := elapsed
	if copied > time.Second {
		t.Fatal("bad copied duration")
	}

	elapsed = time.Since(start)
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
}

func checkConditionalOverwrite(t *testing.T, replace bool) {
	elapsed := time.Since(time.Now())
	if replace {
		elapsed = time.Millisecond
	}
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"

	if replace {
		elapsed = time.Millisecond
	} else {
		elapsed = 2 * time.Millisecond
	}
	assert.Less(t, elapsed, time.Second)
}

func TestDurationAcrossLoop(t *testing.T) {
	elapsed := time.Millisecond
	for range []int{1, 2} {
		assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
		elapsed = time.Since(time.Now())
	}

	elapsed = time.Millisecond
	elapsed += time.Since(time.Now())
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
	elapsed += time.Millisecond
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
}

func TestDurationWrittenByCallback(t *testing.T) {
	elapsed := time.Millisecond
	measure := func() { elapsed = time.Since(time.Now()) }
	measure()
	assert.Less(t, elapsed, time.Second) // want "test asserts on wall-clock elapsed time"
}

func TestDurationSwap(t *testing.T) {
	elapsed := time.Since(time.Now())
	fixed := time.Millisecond
	elapsed, fixed = fixed, elapsed
	assert.Less(t, elapsed, time.Second)
	assert.Less(t, fixed, time.Second) // want "test asserts on wall-clock elapsed time"
}

func TestOverwrittenDurationInSubtest(t *testing.T) {
	t.Run("fixed", func(t *testing.T) {
		elapsed := time.Since(time.Now())
		t.Log(elapsed)
		elapsed = time.Millisecond
		assert.Less(t, elapsed, time.Second)
	})
}
