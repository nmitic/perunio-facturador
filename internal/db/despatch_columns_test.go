package db

import (
	"strings"
	"testing"

	"github.com/perunio/perunio-facturador/internal/model"
)

type countingRow struct{ n int }

func (r *countingRow) Scan(dest ...any) error { r.n = len(dest); return nil }

// TestDespatchColumnsMatchScan: despatchColumns and scanDespatch are edited
// separately (pdf_branding_id was appended to both); a mismatch only fails at
// runtime, on every guía read.
func TestDespatchColumnsMatchScan(t *testing.T) {
	cols := 0
	for _, c := range strings.Split(despatchColumns, ",") {
		if strings.TrimSpace(c) != "" {
			cols++
		}
	}
	row := &countingRow{}
	var d model.Despatch
	if err := scanDespatch(row, &d); err != nil {
		t.Fatal(err)
	}
	if row.n != cols {
		t.Fatalf("despatchColumns has %d columns, scanDespatch scans %d", cols, row.n)
	}
	if !strings.Contains(despatchColumns, "pdf_branding_id") {
		t.Fatal("despatchColumns lost pdf_branding_id")
	}
}
