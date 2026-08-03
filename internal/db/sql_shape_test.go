package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestInsertStatementsAreBalanced checks every `INSERT INTO t (...) VALUES (...)`
// in this package: the target column list and the expression list must have the
// same length, and the $N placeholders must run 1..N with no gaps.
//
// Worth a source-parsing test because nothing else catches it. The statements
// are untyped strings, so `go build` is silent; the tables here need RLS and a
// live pool, so no unit test executes them. A miscounted column list therefore
// ships clean and fails at runtime as `INSERT has more target columns than
// expressions (SQLSTATE 42601)` — a 500 on every document creation. That is
// exactly how adding `transporte_carga` to issued_documents broke: one column
// appended, its `$42` forgotten.
func TestInsertStatementsAreBalanced(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	insertStart := regexp.MustCompile(`INSERT INTO\s+(\w+)\s*\(`)
	placeholder := regexp.MustCompile(`\$(\d+)`)
	checked := 0

	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				sql := lit.Value
				for _, m := range insertStart.FindAllStringSubmatchIndex(sql, -1) {
					table := sql[m[2]:m[3]]
					cols, end := balancedParens(sql, m[1]-1)
					if end < 0 {
						continue
					}
					rest := strings.TrimLeft(sql[end+1:], " \t\r\n")
					if !strings.HasPrefix(rest, "VALUES") {
						continue // INSERT ... SELECT, not a VALUES list
					}
					open := strings.Index(rest, "(")
					if open < 0 {
						continue
					}
					vals, vend := balancedParens(rest, open)
					if vend < 0 {
						continue
					}

					pos := fset.Position(lit.Pos())
					nCols, nVals := topLevelItems(cols), topLevelItems(vals)
					if nCols != nVals {
						t.Errorf("%s:%d INSERT INTO %s: %d target columns but %d expressions",
							pos.Filename, pos.Line, table, nCols, nVals)
					}

					seen := map[int]bool{}
					highest := 0
					for _, p := range placeholder.FindAllStringSubmatch(vals, -1) {
						v, err := strconv.Atoi(p[1])
						if err != nil {
							continue
						}
						seen[v] = true
						if v > highest {
							highest = v
						}
					}
					for i := 1; i <= highest; i++ {
						if !seen[i] {
							t.Errorf("%s:%d INSERT INTO %s: placeholder $%d is missing (highest is $%d)",
								pos.Filename, pos.Line, table, i, highest)
						}
					}
					checked++
				}
				return true
			})
		}
	}

	if checked == 0 {
		t.Fatal("no INSERT ... VALUES statements found — the scanner stopped working")
	}
}

// balancedParens returns the contents of the parenthesised group starting at
// open (which must index a "("), plus the index of its closing ")".
func balancedParens(s string, open int) (string, int) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], i
			}
		}
	}
	return "", -1
}

// topLevelItems counts comma-separated items, ignoring commas nested inside
// parentheses — NULLIF(origin, 'manual') is one item, not two.
func topLevelItems(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	depth, n := 0, 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}
