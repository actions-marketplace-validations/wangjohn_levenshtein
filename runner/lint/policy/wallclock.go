package policy

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
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

// arithmetic operators carry a measured duration into their result, as in
// elapsed / 2.
var arithmetic = map[token.Token]bool{
	token.ADD: true,
	token.SUB: true,
	token.MUL: true,
	token.QUO: true,
	token.REM: true,
}

// ordered operators bound one value by another.
var ordered = map[token.Token]bool{
	token.LSS: true,
	token.GTR: true,
	token.LEQ: true,
	token.GEQ: true,
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
	var tests []*ast.File
	for _, file := range pass.Files {
		if !Generated(pass.Fset, file) && strings.HasSuffix(SourceName(pass.Fset, file), "_test.go") {
			tests = append(tests, file)
		}
	}

	bubbles := synctestBodies(pass, tests)
	for _, file := range tests {
		// A file that does not import time cannot read the clock, so its
		// functions need no walk.
		if !imports(file, "time") {
			continue
		}
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

func imports(file *ast.File, path string) bool {
	return slices.ContainsFunc(file.Imports, func(spec *ast.ImportSpec) bool {
		return spec.Path.Value == strconv.Quote(path)
	})
}

// bubbles is the code a package runs inside synctest.Test or synctest.Run.
// Inside a bubble the clock is fake and advances only when every goroutine in
// the bubble is blocked, so a measured duration is exact.
type bubbles struct {
	literals map[*ast.FuncLit]bool
	funcs    map[types.Object]bool
}

// synctestBodies finds the bubble code in a package's test files: each
// callback passed to synctest.Test or synctest.Run, whether a function
// literal, a variable holding one, a named function, or a method value, and
// every function of the package that bubble code calls directly, transitively.
// A function that runs in a bubble anywhere is treated as bubble code
// everywhere, which keeps a shared helper's fake-clock bounds quiet at the cost
// of missing its real-clock calls.
func synctestBodies(pass *analysis.Pass, files []*ast.File) bubbles {
	found := bubbles{
		literals: map[*ast.FuncLit]bool{},
		funcs:    map[types.Object]bool{},
	}
	// Most test packages have no bubble; only a file that imports
	// testing/synctest can start one.
	if !slices.ContainsFunc(files, func(file *ast.File) bool { return imports(file, "testing/synctest") }) {
		return found
	}

	decls := map[types.Object]*ast.FuncDecl{}
	held := map[types.Object][]*ast.FuncLit{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				decls[pass.TypesInfo.Defs[n.Name]] = n
			case *ast.AssignStmt:
				if len(n.Lhs) == len(n.Rhs) {
					for i, lhs := range n.Lhs {
						if lit, ok := ast.Unparen(n.Rhs[i]).(*ast.FuncLit); ok {
							if obj := localVar(pass, lhs); obj != nil {
								held[obj] = append(held[obj], lit)
							}
						}
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) == len(n.Values) {
					for i, name := range n.Names {
						if lit, ok := ast.Unparen(n.Values[i]).(*ast.FuncLit); ok {
							held[pass.TypesInfo.Defs[name]] = append(held[pass.TypesInfo.Defs[name]], lit)
						}
					}
				}
			}
			return true
		})
	}

	var pending []*ast.BlockStmt
	addFunc := func(obj types.Object) {
		if obj == nil || found.funcs[obj] {
			return
		}
		found.funcs[obj] = true
		if decl := decls[obj]; decl != nil && decl.Body != nil {
			pending = append(pending, decl.Body)
		}
	}
	addLiteral := func(lit *ast.FuncLit) {
		if !found.literals[lit] {
			found.literals[lit] = true
			pending = append(pending, lit.Body)
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !packageFunc(pass, call, "testing/synctest", "Test", "Run") {
				return true
			}
			for _, arg := range call.Args {
				switch a := ast.Unparen(arg).(type) {
				case *ast.FuncLit:
					addLiteral(a)
				case *ast.Ident:
					switch obj := pass.TypesInfo.Uses[a].(type) {
					case *types.Func:
						addFunc(obj)
					case *types.Var:
						for _, lit := range held[obj] {
							addLiteral(lit)
						}
					}
				case *ast.SelectorExpr:
					if fn, ok := pass.TypesInfo.Uses[a.Sel].(*types.Func); ok {
						addFunc(fn)
					}
				}
			}
			return true
		})
	}
	for len(pending) > 0 {
		body := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		ast.Inspect(body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if fn := typeutil.StaticCallee(pass.TypesInfo, call); fn != nil && fn.Pkg() == pass.Pkg {
				addFunc(fn.Origin())
			}
			return true
		})
	}
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
			comparison := clock.boundIn(n.Cond)
			if comparison == nil || !canFail(pass, n.Body) && !failingElse(pass, n.Else) {
				return true
			}
			if !clock.polling(comparison, stack) {
				pass.Reportf(comparison.Pos(), "%s", wallClockMessage)
			}
		case *ast.CallExpr:
			if bound := clock.assertedBy(n); bound != nil && !clock.polling(bound, stack) {
				pass.Reportf(n.Pos(), "%s", wallClockMessage)
			}
		}
		return true
	})
}

// failingElse reports whether an else branch can fail the test, following an
// else-if chain through each arm.
func failingElse(pass *analysis.Pass, stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return canFail(pass, s)
	case *ast.IfStmt:
		return canFail(pass, s.Body) || failingElse(pass, s.Else)
	}
	return false
}

// polling reports whether bound is how long a test waits for a condition
// before giving up rather than a claim about how long the code under test
// takes: the innermost loop around it, within its function, is a three-clause
// or condition-only for loop, or a range over a channel such as a ticker's,
// and no pass of it restarts the clock, so the start of every measurement the
// bound depends on was read before the loop. A pass may still read the clock
// for the end of a measurement, as in now.Sub(start). Any other range loop,
// as in a table-driven test, is not polling.
func (c wallClock) polling(bound ast.Expr, stack []ast.Node) bool {
	for _, node := range slices.Backward(stack) {
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.RangeStmt:
			if _, ok := c.pass.TypesInfo.TypeOf(n.X).Underlying().(*types.Chan); !ok {
				return false
			}
			return !c.restarts(n.Body, nil, c.startsUnder(bound))
		case *ast.ForStmt:
			return !c.restarts(n.Body, n.Post, c.startsUnder(bound))
		}
	}
	return false
}

// restarts reports whether a pass of a loop, its body or its post statement,
// assigns one of starts. Function literals in the loop run when something
// calls them, not as part of the pass, and are not searched.
func (c wallClock) restarts(body *ast.BlockStmt, post ast.Stmt, starts map[types.Object]bool) bool {
	assigns := func(lhs ast.Expr) bool {
		return starts[localVar(c.pass, lhs)]
	}
	restarted := false
	visit := func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			restarted = restarted || slices.ContainsFunc(n.Lhs, assigns)
		case *ast.ValueSpec:
			restarted = restarted || slices.ContainsFunc(n.Names, func(name *ast.Ident) bool { return assigns(name) })
		}
		return !restarted
	}
	ast.Inspect(body, visit)
	if post != nil {
		ast.Inspect(post, visit)
	}
	return restarted
}

// startsUnder returns the clock-reading variables that start a measurement
// expr depends on: the argument of time.Since or of a Sub, directly or through
// the measured variables expr uses. The end reading of a Sub, such as now in
// now.Sub(start), is not a start.
func (c wallClock) startsUnder(expr ast.Expr) map[types.Object]bool {
	found := map[types.Object]bool{}
	seen := map[types.Object]bool{}
	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		ast.Inspect(expr, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				if start := c.start(n); start != nil {
					ast.Inspect(start, func(node ast.Node) bool {
						if ident, ok := node.(*ast.Ident); ok && c.readings[c.pass.TypesInfo.Uses[ident]] {
							found[c.pass.TypesInfo.Uses[ident]] = true
						}
						return true
					})
					return false
				}
			case *ast.Ident:
				obj := c.pass.TypesInfo.Uses[n]
				if c.measured[obj] && !seen[obj] {
					seen[obj] = true
					for _, value := range c.assigned[obj] {
						if value != nil {
							visit(value)
						}
					}
				}
			}
			return true
		})
	}
	visit(expr)
	return found
}

// start returns the reading a measurement starts from: the argument of
// time.Since(start) or of end.Sub(start). It returns nil for any other call.
func (c wallClock) start(call *ast.CallExpr) ast.Expr {
	if packageFunc(c.pass, call, "time", "Since") && len(call.Args) == 1 {
		return call.Args[0]
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if ok && method.Sel.Name == "Sub" && len(call.Args) == 1 && timeMethod(c.pass, call) {
		return call.Args[0]
	}
	return nil
}

// wallClock tracks, within one function, the variables that hold a reading of
// the real clock and the variables that hold a duration measured from one.
// assigned holds every value stored in each variable, with nil for one that
// cannot be seen: one result of a multi-value call, a range variable, or a
// store inside a synctest bubble, whose clock is fake.
type wallClock struct {
	pass     *analysis.Pass
	assigned map[types.Object][]ast.Expr
	readings map[types.Object]bool
	measured map[types.Object]bool
}

func measureClock(pass *analysis.Pass, body *ast.BlockStmt, skip func(ast.Node) bool) wallClock {
	assigned := map[types.Object][]ast.Expr{}
	record := func(lhs ast.Expr, value ast.Expr) {
		if obj := localVar(pass, lhs); obj != nil {
			assigned[obj] = append(assigned[obj], value)
		}
	}
	var collect func(node ast.Node, hidden bool)
	collect = func(root ast.Node, hidden bool) {
		ast.Inspect(root, func(node ast.Node) bool {
			if node != root && !hidden && skip(node) {
				collect(node, true)
				return false
			}
			switch n := node.(type) {
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					var value ast.Expr
					if !hidden && len(n.Lhs) == len(n.Rhs) {
						value = n.Rhs[i]
					}
					record(lhs, value)
				}
			case *ast.RangeStmt:
				record(n.Key, nil)
				record(n.Value, nil)
			case *ast.ValueSpec:
				// A declaration without values starts at the zero value,
				// which reads no clock.
				if n.Values == nil {
					return true
				}
				for i, name := range n.Names {
					var value ast.Expr
					if !hidden && len(n.Names) == len(n.Values) {
						value = n.Values[i]
					}
					record(name, value)
				}
			}
			return true
		})
	}
	collect(body, false)

	clock := wallClock{
		pass:     pass,
		assigned: assigned,
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
			if !clock.measured[obj] && slices.ContainsFunc(values, clock.duration) {
				clock.measured[obj] = true
				changed = true
			}
		}
	}
	return clock
}

func localVar(pass *analysis.Pass, expr ast.Expr) types.Object {
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
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
func (c wallClock) allReadings(values []ast.Expr) bool {
	return !slices.ContainsFunc(values, func(value ast.Expr) bool { return !c.reading(value) })
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
		return arithmetic[e.Op] && (c.duration(e.X) || c.duration(e.Y))
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
		return slices.ContainsFunc(call.Args, c.duration)
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
// duration on either side, outside any assertion call in cond.
func (c wallClock) boundIn(cond ast.Expr) *ast.BinaryExpr {
	var found *ast.BinaryExpr
	ast.Inspect(cond, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			// An assertion call in a condition is reported as a call.
			return c.assertedBy(n) == nil
		case *ast.BinaryExpr:
			if ordered[n.Op] && (c.duration(n.X) || c.duration(n.Y)) {
				found = n
			}
		}
		return true
	})
	return found
}

// assertedBy returns the measured duration or comparison call asserts on, or
// nil. It looks only at the values an assertion compares, which follow the
// test value or a testify function's TestingT, or open the arguments of a
// testify Assertions method: the first
// two of an ordering assertion such as assert.Less, and the first of any other
// call, such as require.True(t, elapsed < time.Second). Trailing message
// arguments, such as the "took %v", elapsed of assert.Less(t, n, 10, "took
// %v", elapsed), are not asserted on.
func (c wallClock) assertedBy(call *ast.CallExpr) ast.Expr {
	operands := call.Args
	if i := slices.IndexFunc(call.Args, func(arg ast.Expr) bool { return testValue(c.pass, arg) }); i >= 0 {
		operands = call.Args[i+1:]
	} else if fn := testifyFunc(c.pass, call); fn == nil {
		return nil
	} else if fn.Signature().Recv() == nil {
		// A package function such as assert.True takes its TestingT first,
		// here one that is not a plain test variable, such as s.T() or an
		// EventuallyWithT callback's *assert.CollectT.
		operands = call.Args[min(1, len(call.Args)):]
	}

	if orderingAssertions[strings.TrimSuffix(calleeName(call), "f")] {
		for _, operand := range operands[:min(2, len(operands))] {
			if c.duration(operand) {
				return operand
			}
		}
		return nil
	}
	if len(operands) == 0 {
		return nil
	}
	if comparison, ok := ast.Unparen(operands[0]).(*ast.BinaryExpr); ok {
		if bound := c.boundIn(comparison); bound != nil {
			return bound
		}
	}
	return nil
}

// testifyFunc returns the testify assert or require function or method call
// calls, or nil.
func testifyFunc(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || fn.Pkg() == nil {
		return nil
	}
	path := fn.Pkg().Path()
	if strings.HasSuffix(path, "/testify/assert") || strings.HasSuffix(path, "/testify/require") {
		return fn
	}
	return nil
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
