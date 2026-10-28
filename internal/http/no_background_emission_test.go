package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// Guards the rule that a comprobante is never emitted without a user action.
//
// This service used to run a ticker that emitted scheduled comprobantes to SUNAT
// unattended. Scheduled periods are now only materialized and announced (by
// perunio-backend); the user emits them through POST /documents. The ways that
// rule could quietly come back are a goroutine started next to the server, or a
// non-request caller of the emission core — so those are what this checks, from
// source, since neither shows up in any request-level test.

// emissionEntryPoints are the functions that create or issue a comprobante.
var emissionEntryPoints = map[string]bool{
	"createDocumentDraft": true,
	"runIssuePipeline":    true,
}

func TestEmissionOnlyFromRequests(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nonTestFiles, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	calls := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.GoStmt:
						t.Errorf("%s: goroutine in %s — this package must not run background work",
							fset.Position(node.Pos()), fn.Name.Name)
					case *ast.CallExpr:
						name := calledMethod(node)
						if !emissionEntryPoints[name] {
							return true
						}
						calls++
						if !takesRequest(fn) {
							t.Errorf("%s: %s calls %s but does not handle an *http.Request — emission must start from a user's request",
								fset.Position(node.Pos()), fn.Name.Name, name)
						}
					}
					return true
				})
			}
		}
	}

	// A rename would make the loop above vacuous; fail instead.
	if calls == 0 {
		t.Fatal("found no calls to the emission entry points; update emissionEntryPoints")
	}
}

// The only goroutine main may start is the HTTP server itself.
func TestMainStartsOnlyTheHTTPServer(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../../cmd/app", nonTestFiles, 0)
	if err != nil {
		t.Fatalf("parse cmd/app: %v", err)
	}

	goroutines := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				stmt, ok := n.(*ast.GoStmt)
				if !ok {
					return true
				}
				goroutines++
				servesHTTP := false
				ast.Inspect(stmt, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok && calledMethod(call) == "ListenAndServe" {
						servesHTTP = true
					}
					return true
				})
				if !servesHTTP {
					t.Errorf("%s: goroutine that does not run the HTTP server", fset.Position(stmt.Pos()))
				}
				return true
			})
		}
	}
	if goroutines == 0 {
		t.Fatal("found no goroutine in cmd/app; the HTTP server check is vacuous")
	}
}

func nonTestFiles(fi fs.FileInfo) bool {
	return !strings.HasSuffix(fi.Name(), "_test.go")
}

// calledMethod returns the name of the function or method a call invokes.
func calledMethod(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name
	case *ast.Ident:
		return fun.Name
	}
	return ""
}

// takesRequest reports whether fn has an *http.Request parameter.
func takesRequest(fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "http" && sel.Sel.Name == "Request" {
			return true
		}
	}
	return false
}
