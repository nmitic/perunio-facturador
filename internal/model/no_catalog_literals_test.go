package model_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"maragu.dev/is"
)

// Stops catálogo data from growing back in Go.
//
// Every SUNAT código here comes from sunat-catalogs. What this guards against is
// not a wrong value but a *second* definition: internal/model/catalog.go grew to
// 246 lines of códigos that the shared package also held, and two byte-identical
// switch pairs computed the same Cat.07 -> Cat.05 mapping in different files.
//
// The signal is a switch or map whose cases are código-shaped string literals.
// Comparing one código against one constant is fine and common; three or more in
// a row is a list.
func TestNoCatalogCodeListsOutsideSunatCatalogs(t *testing.T) {
	// Each entry needs a reason. Growing this list without one is the regression.
	allow := []string{
		// SQL literals: interpolating a constant into a query is uglier than the
		// literal, and a Go test pins the value (see TestAnticipoCodeMatches).
		"internal/db/reports.go",
		"internal/db/ventas_anticipo.go",
		// Test fixtures legitimately spell out códigos.
		"_test.go",
	}

	// Three or more quoted 2- or 4-digit codes in one case/composite literal.
	codeList := regexp.MustCompile(`"(?:\d{2}|\d{4})"\s*(?:,|:)\s*(?:[^,\n]*,\s*)?"(?:\d{2}|\d{4})"\s*(?:,|:)[^,\n]*,?\s*"(?:\d{2}|\d{4})"`)

	lineComment := regexp.MustCompile(`(?m)//.*$`)

	var offenders []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		for _, a := range allow {
			if strings.Contains(filepath.ToSlash(path), a) {
				return nil
			}
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Comments legitimately list códigos to document a field; only code counts.
		if codeList.Match(lineComment.ReplaceAll(src, nil)) {
			offenders = append(offenders, filepath.ToSlash(path))
		}
		return nil
	})
	is.NotError(t, err)
	is.Equal(t, 0, len(offenders))
	if len(offenders) > 0 {
		t.Logf("código lists found outside sunat-catalogs: %v", offenders)
	}
}
