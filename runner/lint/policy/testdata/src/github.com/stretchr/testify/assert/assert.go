// Package assert stands in for testify's assert package, with the signatures
// LV1007's fixtures call.
package assert

type TestingT interface {
	Errorf(format string, args ...any)
}

type Assertions struct {
	t TestingT
}

func New(t TestingT) *Assertions {
	return &Assertions{t: t}
}

func Less(t TestingT, e1, e2 any, msgAndArgs ...any) bool {
	return true
}

func GreaterOrEqual(t TestingT, e1, e2 any, msgAndArgs ...any) bool {
	return true
}

func Lessf(t TestingT, e1, e2 any, msg string, args ...any) bool {
	return true
}

func InDelta(t TestingT, expected, actual any, delta float64, msgAndArgs ...any) bool {
	return true
}

func True(t TestingT, value bool, msgAndArgs ...any) bool {
	return true
}

func NotZero(t TestingT, value any, msgAndArgs ...any) bool {
	return true
}

func (a *Assertions) Less(e1, e2 any, msgAndArgs ...any) bool {
	return true
}

func Equal(t TestingT, expected, actual any, msgAndArgs ...any) bool {
	return true
}

func (a *Assertions) Lessf(e1, e2 any, msg string, args ...any) bool {
	return true
}

func Greater(t TestingT, e1, e2 any, msgAndArgs ...any) bool {
	return true
}

// CollectT is the TestingT an EventuallyWithT callback receives.
type CollectT struct{}

func (c *CollectT) Errorf(format string, args ...any) {}
