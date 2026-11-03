package db

import (
	"strings"
	"testing"

	"maragu.dev/is"
)

func TestSplitDocID(t *testing.T) {
	tests := []struct {
		name        string
		docID       string
		series      string
		correlative int
		ok          bool
	}{
		{name: "should split a padded SERIE-CORRELATIVO", docID: "F001-00000042", series: "F001", correlative: 42, ok: true},
		{name: "should read an unpadded correlative as the same anticipo", docID: "F001-42", series: "F001", correlative: 42, ok: true},
		{name: "should upper-case the serie", docID: "b001-7", series: "B001", correlative: 7, ok: true},
		{name: "should trim surrounding spaces", docID: " F001-42 ", series: "F001", correlative: 42, ok: true},
		{name: "should refuse a docId without a dash", docID: "F00142"},
		{name: "should refuse an empty serie", docID: "-42"},
		{name: "should refuse a non-numeric correlative", docID: "F001-ABC"},
		{name: "should refuse a zero correlative", docID: "F001-0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			series, correlative, ok := splitDocID(test.docID)
			is.Equal(t, test.ok, ok)
			is.Equal(t, test.series, series)
			is.Equal(t, test.correlative, correlative)
		})
	}
}

// The report and the issue pipeline must agree on what "aplicado" means, so
// both build their SQL from these fragments. Pin the parts that carry the
// meaning: a nota de crédito annulling the regularización releases its
// anticipos, and a typed-in docId matches on serie + correlative.
func TestAnticipoSQLFragments(t *testing.T) {
	t.Run("should release anticipos of a regularización annulled by nota de crédito 01 or 02", func(t *testing.T) {
		sql := deductsAnticipos("r")
		is.True(t, strings.Contains(sql, "NOT EXISTS"))
		is.True(t, strings.Contains(sql, "nc.doc_type = '07'"))
		is.True(t, strings.Contains(sql, "IN ('01', '02')"))
		is.True(t, strings.Contains(sql, "nc.reference_doc_correlative = r.correlative"))
	})

	t.Run("should match a typed docId on serie and unpadded correlative", func(t *testing.T) {
		sql := anticipoRefersTo("a", "d")
		is.True(t, strings.Contains(sql, "a->>'sourceDocumentId' = d.id::text"))
		is.True(t, strings.Contains(sql, "ltrim(split_part(a->>'docId', '-', 2), '0') = d.correlative::text"))
	})
}
