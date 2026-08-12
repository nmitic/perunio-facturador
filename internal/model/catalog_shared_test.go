package model_test

import (
	"testing"

	"maragu.dev/is"

	"github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// The catálogos are moving out of internal/model and into the shared
// perunio-sunat-catalogs module, one catálogo at a time. Until a given catálogo
// has moved, this pins the two definitions together: the migration is then a
// rename, and any disagreement fails here rather than at SUNAT.
//
// Delete each subtest as its catálogo's constants leave model/catalog.go.
func TestSharedCatalogAgreesWithModel(t *testing.T) {
	t.Run("should give Cat.16 the same codes model already names", func(t *testing.T) {
		is.Equal(t, model.PriceTypeUnitWithIGV, sunat.Cat16PrecioUnitario)
		is.Equal(t, model.PriceTypeReferential, sunat.Cat16ValorReferencial)
	})

	t.Run("should know the Cat.16 code the XML builder accepts but model never named", func(t *testing.T) {
		// xmlbuilder.validPriceTypeCodes accepts 01, 02 and 03, while
		// model/catalog.go names only the first two. The shared catálogo carries
		// all three, so the constant exists before the builder needs it.
		is.True(t, sunat.Cat16Valid(sunat.Cat16ValorReferencialExportacion))
		is.Equal(t, "03", sunat.Cat16ValorReferencialExportacion)
	})

	// Cat.05 has already left catalog.go, so there is no local definition left to
	// pin against. What replaces it: the tributo metadata the XML builders read
	// straight out of the catálogo and put on the wire. An edit to cat05.json that
	// changed one of these would silently alter every emitted cac:TaxScheme, so it
	// fails here instead.
	t.Run("should describe every Cat.05 tributo the way SUNAT expects it on the wire", func(t *testing.T) {
		for _, tc := range []struct{ code, name, taxTypeCode, categoryID string }{
			{sunat.Cat05IGV, "IGV", "VAT", "S"},
			{sunat.Cat05IVAP, "IVAP", "VAT", "S"},
			{sunat.Cat05ISC, "ISC", "EXC", "S"},
			{sunat.Cat05ICBPER, "ICBPER", "OTH", "S"},
			{sunat.Cat05Exportacion, "EXP", "FRE", "G"},
			{sunat.Cat05Gratuita, "GRA", "FRE", "E"},
			{sunat.Cat05Exonerado, "EXO", "VAT", "E"},
			{sunat.Cat05Inafecto, "INA", "FRE", "O"},
			{sunat.Cat05Otros, "OTROS", "OTH", "S"},
		} {
			is.Equal(t, tc.name, sunat.Cat05Name(tc.code))
			is.Equal(t, tc.taxTypeCode, sunat.Cat05TaxTypeCode(tc.code))
			is.Equal(t, tc.categoryID, sunat.Cat05TaxCategoryId(tc.code))
		}
	})

	t.Run("should carry the statutory rate of the two rated tributos", func(t *testing.T) {
		// These are the strings that reach cbc:Percent. A rate written as "18" or
		// "0.18" instead of "18.00" is SUNAT fault 2992 territory.
		is.Equal(t, "18.00", sunat.Cat05Rate(sunat.Cat05IGV))
		is.Equal(t, "4.00", sunat.Cat05Rate(sunat.Cat05IVAP))
		// Only 9996 is the gratuito scheme; the flag drives the gratuito subtotal.
		is.True(t, sunat.Cat05Gratuito(sunat.Cat05Gratuita))
		is.True(t, !sunat.Cat05Gratuito(sunat.Cat05IGV))
	})

	// RC and RA are UBL 2.0 while every other document is 2.1 — the #1 cause of a
	// silently rejected document, now one row of ubl-document.json per builder.
	t.Run("should keep RC and RA on UBL 2.0 and everything else on 2.1", func(t *testing.T) {
		for _, code := range []string{
			sunat.UblDocumentInvoice, sunat.UblDocumentCreditNote,
			sunat.UblDocumentDebitNote, sunat.UblDocumentDespatchAdvice,
		} {
			is.Equal(t, "2.1", sunat.UblDocumentUblVersion(code))
			is.Equal(t, "2.0", sunat.UblDocumentCustomizationId(code))
		}

		is.Equal(t, "2.0", sunat.UblDocumentUblVersion(sunat.UblDocumentSummaryDocuments))
		is.Equal(t, "1.1", sunat.UblDocumentCustomizationId(sunat.UblDocumentSummaryDocuments))

		is.Equal(t, "2.0", sunat.UblDocumentUblVersion(sunat.UblDocumentVoidedDocuments))
		is.Equal(t, "1.0", sunat.UblDocumentCustomizationId(sunat.UblDocumentVoidedDocuments))
	})

	t.Run("should reject a code SUNAT does not define", func(t *testing.T) {
		is.True(t, !sunat.Cat16Valid("99"))

		_, ok := sunat.Cat16("99")
		is.True(t, !ok)
	})

	t.Run("should keep every selectable code inside the catálogo", func(t *testing.T) {
		// A picker must never offer a code the pipeline would reject. This is the
		// invariant the scattered per-repo copies used to violate.
		for _, code := range sunat.Cat16SelectCodes {
			is.True(t, sunat.Cat16Valid(code))
			is.True(t, sunat.Cat16Emit(code))
		}
	})
}
