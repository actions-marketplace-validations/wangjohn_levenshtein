package wallclock

import (
	"testing"
	"time"
)

// bubbleInOtherFile is passed to synctest.Test from bubble_test.go.
func bubbleInOtherFile(t *testing.T) {
	start := time.Now()
	time.Sleep(time.Second)
	if time.Since(start) > time.Second {
		t.Fatal("too slow")
	}
}
