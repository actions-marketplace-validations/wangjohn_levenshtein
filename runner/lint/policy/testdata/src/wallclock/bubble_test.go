package wallclock

import (
	"testing"
	"testing/synctest"
)

// bubbleInOtherFile, in clock_test.go, runs here as a bubble.
func TestSynctestBodyInOtherFile(t *testing.T) {
	synctest.Test(t, bubbleInOtherFile)
}
