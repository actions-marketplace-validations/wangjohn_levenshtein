package policy

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/cfg"
)

// durationUses follows the values reaching each read, so a definite overwrite
// clears a measurement while a conditional overwrite or loop may retain it.
// Each closure has its own graph. Captured variables keep the conservative
// function-wide classification because their value depends on when it runs.
func (c wallClock) durationUses(body *ast.BlockStmt, skip func(ast.Node) bool) map[*ast.Ident]bool {
	uses := map[*ast.Ident]bool{}
	bodies := []*ast.BlockStmt{body}
	ast.Inspect(body, func(node ast.Node) bool {
		if skip(node) {
			return false
		}
		if lit, ok := node.(*ast.FuncLit); ok {
			bodies = append(bodies, lit.Body)
		}
		return true
	})
	for _, body := range bodies {
		c.flowDurations(body, uses)
	}
	return uses
}

func (c wallClock) flowDurations(body *ast.BlockStmt, uses map[*ast.Ident]bool) {
	graph := cfg.New(body, func(*ast.CallExpr) bool { return true })
	shared := c.closureWrites(body)
	incoming := make([]map[types.Object]bool, len(graph.Blocks))
	incoming[0] = maps.Clone(c.measured)
	pending := []*cfg.Block{graph.Blocks[0]}
	for len(pending) > 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		state := wallClock{
			pass:     c.pass,
			readings: c.readings,
			measured: maps.Clone(incoming[block.Index]),
		}
		if block.Kind == cfg.KindRangeBody {
			rangeStmt := block.Stmt.(*ast.RangeStmt)
			delete(state.measured, localVar(c.pass, rangeStmt.Key))
			delete(state.measured, localVar(c.pass, rangeStmt.Value))
		}
		maps.Copy(state.measured, shared)
		for _, node := range block.Nodes {
			ast.Inspect(node, func(node ast.Node) bool {
				if _, ok := node.(*ast.FuncLit); ok {
					return false
				}
				if ident, ok := node.(*ast.Ident); ok && state.measured[c.pass.TypesInfo.Uses[ident]] {
					uses[ident] = true
				}
				return true
			})
			state.storeDuration(node)
			maps.Copy(state.measured, shared)
		}
		for _, next := range block.Succs {
			changed := incoming[next.Index] == nil
			if changed {
				incoming[next.Index] = map[types.Object]bool{}
			}
			for obj, measured := range state.measured {
				if measured && !incoming[next.Index][obj] {
					incoming[next.Index][obj] = true
					changed = true
				}
			}
			if changed {
				pending = append(pending, next)
			}
		}
	}
}

func (c wallClock) storeDuration(node ast.Node) {
	values := map[types.Object]bool{}
	record := func(lhs ast.Expr, rhs []ast.Expr, index int, count int, compound bool) {
		obj := localVar(c.pass, lhs)
		if obj == nil {
			return
		}
		measured := compound && c.measured[obj]
		if len(rhs) == count {
			measured = measured || c.duration(rhs[index])
		}
		values[obj] = measured
	}
	switch n := node.(type) {
	case *ast.AssignStmt:
		for i, lhs := range n.Lhs {
			record(lhs, n.Rhs, i, len(n.Lhs), n.Tok != token.ASSIGN && n.Tok != token.DEFINE)
		}
	case *ast.ValueSpec:
		for i, name := range n.Names {
			record(name, n.Values, i, len(n.Names), false)
		}
	}
	// Evaluate every RHS before updating the state, including swaps.
	for obj, measured := range values {
		if measured {
			c.measured[obj] = true
		} else {
			delete(c.measured, obj)
		}
	}
}

// closureWrites retains function-wide information for variables a callback
// may change. Its execution order is not represented by the enclosing graph.
func (c wallClock) closureWrites(body *ast.BlockStmt) map[types.Object]bool {
	shared := map[types.Object]bool{}
	ast.PreorderStack(body, nil, func(node ast.Node, stack []ast.Node) bool {
		var writes []ast.Expr
		switch n := node.(type) {
		case *ast.AssignStmt:
			writes = n.Lhs
		case *ast.RangeStmt:
			writes = []ast.Expr{n.Key, n.Value}
		}
		for _, lhs := range writes {
			obj := localVar(c.pass, lhs)
			if !c.measured[obj] {
				continue
			}
			for _, ancestor := range stack {
				if lit, ok := ancestor.(*ast.FuncLit); ok && !c.pass.TypesInfo.Scopes[lit.Type].Contains(obj.Pos()) {
					shared[obj] = true
				}
			}
		}
		return true
	})
	return shared
}
