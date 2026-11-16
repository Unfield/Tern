package native

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// prune drops the statements the compiler writes after one that already
// leaves the block: the return every function ends with after a return of
// its own, a fallthrough after a break. Go accepts them, but go vet reports
// each as unreachable code, which buries an app's own findings.
func prune(src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var gone []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BlockStmt:
			n.List, gone = pruneList(n.List, gone)
		case *ast.CaseClause:
			n.Body, gone = pruneList(n.Body, gone)
		case *ast.CommClause:
			n.Body, gone = pruneList(n.Body, gone)
		}
		return true
	})
	if len(gone) == 0 {
		return src, nil
	}
	// Comments inside what was dropped would land somewhere else.
	kept := f.Comments[:0]
	for _, cg := range f.Comments {
		in := false
		for _, g := range gone {
			// A comment at the end of the line goes with the statement.
			if cg.Pos() >= g.Pos() && (cg.End() <= g.End() || fset.Position(cg.Pos()).Line == fset.Position(g.End()).Line) {
				in = true
				break
			}
		}
		if !in {
			kept = append(kept, cg)
		}
	}
	f.Comments = kept
	var b bytes.Buffer
	if err := format.Node(&b, fset, f); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// pruneList cuts list after its first statement that doesn't complete
// normally, up to the next label, which a goto may still reach.
func pruneList(list []ast.Stmt, gone []ast.Node) ([]ast.Stmt, []ast.Node) {
	for i, st := range list {
		end := len(list)
		for j := i + 1; j < len(list); j++ {
			if _, ok := list[j].(*ast.LabeledStmt); ok {
				end = j
				break
			}
		}
		if end == i+1 {
			continue
		}
		switch {
		case terminates(st):
		case leaves(st):
			// break and continue leave the block but aren't terminating
			// statements: keep a final return a function may need.
			if _, ok := list[end-1].(*ast.ReturnStmt); ok {
				continue
			}
		default:
			continue
		}
		for _, d := range list[i+1 : end] {
			gone = append(gone, d)
		}
		rest, more := pruneList(list[end:], gone)
		return append(list[:i+1:i+1], rest...), more
	}
	return list, gone
}

// leaves is terminates, with break and continue counted too: control
// doesn't reach the next statement either way.
func leaves(st ast.Stmt) bool {
	switch s := st.(type) {
	case *ast.BranchStmt:
		return true
	case *ast.BlockStmt:
		return len(s.List) > 0 && leaves(s.List[len(s.List)-1])
	case *ast.IfStmt:
		return s.Else != nil && leaves(s.Body) && leaves(s.Else)
	}
	return terminates(st)
}

// terminates is the Go spec's terminating statement, for the forms the
// compiler writes; loops, switches and selects answer false, which only
// keeps code.
func terminates(st ast.Stmt) bool {
	switch s := st.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return s.Tok == token.GOTO || s.Tok == token.FALLTHROUGH
	case *ast.ExprStmt:
		c, ok := s.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := c.Fun.(*ast.Ident)
		return ok && id.Name == "panic" && id.Obj == nil
	case *ast.BlockStmt:
		return len(s.List) > 0 && terminates(s.List[len(s.List)-1])
	case *ast.IfStmt:
		return s.Else != nil && terminates(s.Body) && terminates(s.Else)
	case *ast.LabeledStmt:
		return terminates(s.Stmt)
	}
	return false
}
