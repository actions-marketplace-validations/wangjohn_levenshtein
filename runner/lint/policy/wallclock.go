package policy

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// WallClock reports tests that assert on how long something took by the real
// clock, which passes on an idle machine and fails on a loaded one.
var WallClock = &analysis.Analyzer{
	Name: "LV1007",
	Doc:  "do not assert on wall-clock time measured in a test",
	Run:  runWallClock,
}

const wallClockMessage = "test asserts on wall-clock elapsed time, which flakes on a loaded machine; " +
	"run the code under testing/synctest, inject a clock, or compare the deadline a stub observes against clock readings taken inside the stub"

// orderingAssertions are assertion functions, such as testify's, that bound
// one value by another. A measured duration passed to one is an assertion on
// elapsed time. Each also matches with an f suffix, such as Lessf.
var orderingAssertions = map[string]bool{
	"Less":           true,
	"LessOrEqual":    true,
	"Greater":        true,
	"GreaterOrEqual": true,
	"InDelta":        true,
	"InEpsilon":      true,
}

// readingMethods return the time.Time they are called on, or a copy of it,
// so their result is still a reading of the clock.
var readingMethods = map[string]bool{
	"UTC":      true,
	"Local":    true,
	"Round":    true,
	"Truncate": true,
}

func runWallClock(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if Generated(pass.Fset, file) || !strings.HasSuffix(SourceName(pass.Fset, file), "_test.go") {
			continue
		}

		bubbles := synctestBodies(pass, file)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || benchmark(pass, fn.Type) || bubbles.funcs[pass.TypesInfo.Defs[fn.Name]] {
				continue
			}
			checkWallClock(pass, fn.Body, bubbles)
		}
	}
	return nil, nil
}

// bubbles are the callbacks a file passes to synctest.Test or synctest.Run.
// Inside one the clock is fake and advances only when every goroutine in the
// bubble is blocked, so a measured duration is exact.
type bubbles struct {
	literals map[*ast.FuncLit]bool
	funcs    map[types.Object]bool
}

func synctestBodies(pass *analysis.Pass, file *ast.File) bubbles {
	found := bubbles{
		literals: map[*ast.FuncLit]bool{},
		funcs:    map[types.Object]bool{},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !packageFunc(pass, call, "testing/synctest", "Test", "Run") {
			return true
		}
		for _, arg := range call.Args {
			switch a := ast.Unparen(arg).(type) {
			case *ast.FuncLit:
				found.literals[a] = true
			case *ast.Ident:
				if fn, ok := pass.TypesInfo.Uses[a].(*types.Func); ok {
					found.funcs[fn] = true
				}
			}
		}
		return true
	})
	return found
}

// checkWallClock reports the assertions in body that bound a duration
// measured with the real clock. Nested function literals are included, except
// synctest bubbles and benchmark bodies.
func checkWallClock(pass *analysis.Pass, body *ast.BlockStmt, bubbles bubbles) {
	skip := func(node ast.Node) bool {
		lit, ok := node.(*ast.FuncLit)
		return ok && (bubbles.literals[lit] || benchmark(pass, lit.Type))
	}
	clock := measureClock(pass, body, skip)

	ast.PreorderStack(body, nil, func(node ast.Node, stack []ast.Node) bool {
		if skip(node) {
			return false
		}
		switch n := node.(type) {
		case *ast.IfStmt:
			if polling(stack) || !(canFail(pass, n.Body) || failingElse(pass, n.Else)) {
				return true
			}
			if comparison := clock.boundIn(n.Cond); comparison != nil {
				pass.Reportf(comparison.Pos(), "%s", wallClockMessage)
			}
		case *ast.CallExpr:
			if clock.assertedBy(pass, n) {
				pass.Reportf(n.Pos(), "%s", wallClockMessage)
			}
		}
		return true
	})
}

func failingElse(pass *analysis.Pass, stmt ast.Stmt) bool {
	block, ok := stmt.(*ast.BlockStmt)
	return ok && canFail(pass, block)
}

// polling reports whether the innermost loop around a node, within its
// function, is a three-clause or condition-only for loop. A bound checked
// there is usually how long a test waits for a condition before giving up,
// not a claim about how long the code under test takes. A range loop, as in a
// table-driven test, does not count.
func polling(stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i].(type) {
		case *ast.FuncLit:
			return false
		case *ast.ForStmt:
			return true
		}
	}
	return false
}

// wallClock tracks, within one function, the variables that hold a reading of
// the real clock and the variables that hold a duration measured from one.
type wallClock struct {
	pass     *analysis.Pass
	readings map[types.Object]bool
	measured map[types.Object]bool
}

// assignment is one value stored in a variable: op is token.DEFINE or
// token.ASSIGN for a plain store and the arithmetic operator for one such as
// +=.
type assignment struct {
	op    token.Token
	value ast.Expr
}

func measureClock(pass *analysis.Pass, body *ast.BlockStmt, skip func(ast.Node) bool) wallClock {
	assigned := map[types.Object][]assignment{}
	ast.Inspect(body, func(node ast.Node) bool {
		if skip(node) {
			return false
		}
		switch n := node.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, lhs := range n.Lhs {
				if obj := localVar(pass, lhs); obj != nil {
					assigned[obj] = append(assigned[obj], assignment{op: n.Tok, value: n.Rhs[i]})
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) != len(n.Values) {
				return true
			}
			for i, name := range n.Names {
				if obj := pass.TypesInfo.Defs[name]; obj != nil {
					assigned[obj] = append(assigned[obj], assignment{op: token.DEFINE, value: n.Values[i]})
				}
			}
		}
		return true
	})

	clock := wallClock{
		pass:     pass,
		readings: map[types.Object]bool{},
		measured: map[types.Object]bool{},
	}
	for changed := true; changed; {
		changed = false
		for obj, values := range assigned {
			if !clock.readings[obj] && clock.allReadings(values) {
				clock.readings[obj] = true
				changed = true
			}
			if !clock.measured[obj] && clock.anyMeasured(values) {
				clock.measured[obj] = true
				changed = true
			}
		}
	}
	return clock
}

func localVar(pass *analysis.Pass, expr ast.Expr) types.Object {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	if obj, ok := pass.TypesInfo.Defs[ident].(*types.Var); ok {
		return obj
	}
	if obj, ok := pass.TypesInfo.Uses[ident].(*types.Var); ok {
		return obj
	}
	return nil
}

// allReadings reports whether every value a variable is given is a reading of
// the real clock, so a variable that is also set from an injected clock is
// not one.
func (c wallClock) allReadings(values []assignment) bool {
	for _, value := range values {
		if (value.op != token.DEFINE && value.op != token.ASSIGN) || !c.reading(value.value) {
			return false
		}
	}
	return true
}

func (c wallClock) anyMeasured(values []assignment) bool {
	for _, value := range values {
		if c.duration(value.value) {
			return true
		}
	}
	return false
}

// reading reports whether expr is time.Now(), a copy of it such as
// time.Now().UTC(), or a variable that only ever holds one.
func (c wallClock) reading(expr ast.Expr) bool {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return c.readings[c.pass.TypesInfo.Uses[e]]
	case *ast.CallExpr:
		if packageFunc(c.pass, e, "time", "Now") {
			return true
		}
		method, ok := e.Fun.(*ast.SelectorExpr)
		return ok && readingMethods[method.Sel.Name] && timeMethod(c.pass, e) && c.reading(method.X)
	}
	return false
}

// duration reports whether expr is a duration measured with the real clock:
// time.Since, the difference of two readings, or a value computed from one,
// such as elapsed.Milliseconds() or elapsed / 2.
func (c wallClock) duration(expr ast.Expr) bool {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return c.measured[c.pass.TypesInfo.Uses[e]]
	case *ast.UnaryExpr:
		return c.duration(e.X)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
			return c.duration(e.X) || c.duration(e.Y)
		}
	case *ast.CallExpr:
		return c.measuredCall(e)
	}
	return false
}

func (c wallClock) measuredCall(call *ast.CallExpr) bool {
	if packageFunc(c.pass, call, "time", "Since") {
		return true
	}
	if tv, ok := c.pass.TypesInfo.Types[call.Fun]; ok && tv.IsType() && len(call.Args) == 1 {
		return c.duration(call.Args[0])
	}
	if builtinCall(c.pass, call, "min") || builtinCall(c.pass, call, "max") {
		for _, arg := range call.Args {
			if c.duration(arg) {
				return true
			}
		}
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !timeMethod(c.pass, call) {
		return false
	}
	if method.Sel.Name == "Sub" && len(call.Args) == 1 {
		return c.reading(method.X) && c.reading(call.Args[0])
	}
	return durationValue(c.pass, method.X) && c.duration(method.X)
}

// boundIn returns the first ordered comparison in cond that has a measured
// duration on either side.
func (c wallClock) boundIn(cond ast.Expr) *ast.BinaryExpr {
	var found *ast.BinaryExpr
	ast.Inspect(cond, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BinaryExpr:
			switch n.Op {
			case token.LSS, token.GTR, token.LEQ, token.GEQ:
				if c.duration(n.X) || c.duration(n.Y) {
					found = n
				}
			}
		}
		return true
	})
	return found
}

// assertedBy reports whether call is an assertion on a measured duration: an
// ordering assertion such as assert.Less given one, or any call that receives
// the test value and an ordered comparison on one, such as
// require.True(t, elapsed < time.Second).
func (c wallClock) assertedBy(pass *analysis.Pass, call *ast.CallExpr) bool {
	receivesTest := false
	for _, arg := range call.Args {
		if testValue(pass, arg) {
			receivesTest = true
		}
	}
	if !receivesTest && !testifyCall(pass, call) {
		return false
	}

	ordering := orderingAssertions[strings.TrimSuffix(calleeName(call), "f")]
	for _, arg := range call.Args {
		if ordering && c.duration(arg) {
			return true
		}
		if comparison, ok := ast.Unparen(arg).(*ast.BinaryExpr); ok && c.boundIn(comparison) != nil {
			return true
		}
	}
	return false
}

// testifyCall reports whether call is a method of testify's assert.Assertions
// or require.Assertions, which hold the test value themselves.
func testifyCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn := typeutil.Callee(pass.TypesInfo, call)
	if fn == nil || fn.Pkg() == nil {
		return false
	}
	path := fn.Pkg().Path()
	return strings.HasSuffix(path, "/testify/assert") || strings.HasSuffix(path, "/testify/require")
}

func calleeName(call *ast.CallExpr) string {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// packageFunc reports whether call calls one of the named functions of the
// package at path.
func packageFunc(pass *analysis.Pass, call *ast.CallExpr, path string, names ...string) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != path {
		return false
	}
	if fn.Signature().Recv() != nil {
		return false
	}
	for _, name := range names {
		if fn.Name() == name {
			return true
		}
	}
	return false
}

// timeMethod reports whether call calls a method declared in package time,
// such as time.Time.Sub or time.Duration.Seconds.
func timeMethod(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "time" && fn.Signature().Recv() != nil
}

func durationValue(pass *analysis.Pass, expr ast.Expr) bool {
	named, ok := types.Unalias(pass.TypesInfo.TypeOf(expr)).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && named.Obj().Name() == "Duration"
}

// benchmark reports whether a function takes a *testing.B, whose timings are
// reported rather than asserted.
func benchmark(pass *analysis.Pass, fn *ast.FuncType) bool {
	for _, param := range fn.Params.List {
		pointer, ok := types.Unalias(pass.TypesInfo.TypeOf(param.Type)).(*types.Pointer)
		if ok && testingNamed(pointer.Elem(), "B") {
			return true
		}
	}
	return false
}
