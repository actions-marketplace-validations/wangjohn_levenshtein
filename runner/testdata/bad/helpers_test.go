package bad

import (
	"os"
	"testing"
	"time"
)

// thelper: a helper has to announce itself with t.Helper().
func expectZero(t *testing.T, value int) {
	if value != 0 {
		t.Fatalf("value = %d", value)
	}
}

// tparallel: the subtests run in parallel but the parent test does not.
func TestParallelSubtests(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		expectZero(t, 0)
	})
}

// LV1006: nothing in the test can fail, whatever the code under test returns.
func TestNoAssertion(t *testing.T) {
	t.Log(len("zero"))
}

// usetesting: the variable stays set for every test that runs after this one.
func TestSetenv(t *testing.T) {
	if err := os.Setenv("LEVENSHTEIN_FIXTURE", "1"); err != nil {
		t.Fatal(err)
	}
}

// LV1007: the bound holds on an idle machine and fails on a loaded one.
func TestFinishesQuickly(t *testing.T) {
	start := time.Now()
	expectZero(t, 0)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v", elapsed)
	}
}
