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

	// Cat.07 replaced model.TaxCodeForAffectation, which mapped código to tributo
	// with range comparisons ("code >= 11 && code <= 16"). This pins the whole
	// mapping to what that function returned, so the move is provably a rename.
	t.Run("should map every Cat.07 código to the tributo TaxCodeForAffectation returned", func(t *testing.T) {
		for code, want := range map[string]string{
			"10": "1000",
			"11": "9996", "12": "9996", "13": "9996", "14": "9996", "15": "9996", "16": "9996",
			"17": "1016",
			"20": "9997",
			"21": "9996",
			"30": "9998",
			"31": "9996", "32": "9996", "33": "9996", "34": "9996", "35": "9996", "36": "9996", "37": "9996",
			"40": "9995",
		} {
			is.Equal(t, want, sunat.Cat07TaxSchemeCode(code))
			is.True(t, sunat.Cat05Valid(sunat.Cat07TaxSchemeCode(code)))
		}
		// The old function returned "" for anything else; so does the lookup.
		is.Equal(t, "", sunat.Cat07TaxSchemeCode("99"))
	})

	t.Run("should treat exactly the gratuito códigos as gratuito", func(t *testing.T) {
		// Includes 37, which both frontend lists omitted while Go and the backend
		// already accepted it. Every gratuito código declares tributo 9996.
		gratuito := []string{"11", "12", "13", "14", "15", "16", "21", "31", "32", "33", "34", "35", "36", "37"}
		for _, code := range gratuito {
			is.True(t, sunat.Cat07Gratuito(code))
			is.Equal(t, sunat.Cat05Gratuita, sunat.Cat07TaxSchemeCode(code))
		}
		for _, code := range []string{"10", "17", "20", "30", "40"} {
			is.True(t, !sunat.Cat07Gratuito(code))
		}

		// Gravado-gratuito is the subset that declares IGV it would have carried.
		for _, code := range []string{"11", "12", "13", "14", "15", "16"} {
			is.True(t, sunat.Cat07GravadoGratuito(code))
		}
		for _, code := range []string{"21", "31", "37", "10", "20"} {
			is.True(t, !sunat.Cat07GravadoGratuito(code))
		}
	})

	t.Run("should declare the cbc:Percent each código carries, and none where the tag is omitted", func(t *testing.T) {
		is.Equal(t, "18.00", sunat.Cat07Percent("10"))
		is.Equal(t, "4.00", sunat.Cat07Percent("17"))
		// Gravado-gratuito declares the 18% it would have carried (fault 2992);
		// exonerado/inafecto gratuito declare 0.00, matching their zero TaxAmount.
		is.Equal(t, "18.00", sunat.Cat07Percent("15"))
		is.Equal(t, "0.00", sunat.Cat07Percent("21"))
		is.Equal(t, "0.00", sunat.Cat07Percent("37"))
		// Empty means cbc:Percent is omitted entirely — the element is omitempty.
		is.Equal(t, "", sunat.Cat07Percent("20"))
		is.Equal(t, "", sunat.Cat07Percent("30"))
		is.Equal(t, "", sunat.Cat07Percent("40"))
	})

	// Cat.06 replaced a ten-constant block in catalog.go. The códigos it named
	// keep their values; the three SUNAT defines that it never named are new.
	t.Run("should keep the Cat.06 códigos model already named", func(t *testing.T) {
		is.Equal(t, "0", sunat.Cat06DocTribNoDomSinRUC)
		is.Equal(t, "1", sunat.Cat06DNI)
		is.Equal(t, "4", sunat.Cat06CarnetExtranjeria)
		is.Equal(t, "6", sunat.Cat06RUC)
		is.Equal(t, "7", sunat.Cat06Pasaporte)
		is.Equal(t, "A", sunat.Cat06CedulaDiplomatica)
		is.Equal(t, "B", sunat.Cat06DocPaisResidencia)
		is.Equal(t, "C", sunat.Cat06TIN)
		is.Equal(t, "D", sunat.Cat06IN)
	})

	t.Run("should carry the three códigos model never named, and rename E", func(t *testing.T) {
		// catalog.go stopped at E and called it NITE. SUNAT's catálogo sheet gives
		// E as the Tarjeta Andina de Migración and continues F, G, H.
		is.Equal(t, "E", sunat.Cat06TAM)
		is.Equal(t, "F", sunat.Cat06PTP)
		is.Equal(t, "G", sunat.Cat06Salvoconducto)
		is.Equal(t, "H", sunat.Cat06CPP)
		is.Equal(t, 13, len(sunat.Cat06Codes))
	})

	t.Run("should declare a número format only for RUC and DNI", func(t *testing.T) {
		// These two strings are the validation rule itself, shared with the
		// backend's cliente form. An empty pattern means SUNAT documents no
		// length rule, not that anything goes unvalidated.
		is.Equal(t, `^\d{11}$`, sunat.Cat06NumberPattern(sunat.Cat06RUC))
		is.Equal(t, `^\d{8}$`, sunat.Cat06NumberPattern(sunat.Cat06DNI))
		for _, code := range []string{"0", "4", "7", "A", "B", "C", "D", "E", "F", "G", "H"} {
			is.Equal(t, "", sunat.Cat06NumberPattern(code))
		}
	})

	t.Run("should treat only the sin-documento código as unidentifying", func(t *testing.T) {
		// hasIdentifiedCustomer turns on this: consumidor final is código 0.
		is.True(t, !sunat.Cat06Identifica(sunat.Cat06DocTribNoDomSinRUC))
		for _, code := range sunat.Cat06Codes {
			if code != sunat.Cat06DocTribNoDomSinRUC {
				is.True(t, sunat.Cat06Identifica(code))
			}
		}
	})

	t.Run("should carry every unidad de medida, and only KGM for a guía weight", func(t *testing.T) {
		// This service had no Cat.03 list at all and accepted any string as a
		// unitCode, so a typo reached SUNAT. The frontend had 41 but its line
		// editor offered the first 15.
		is.Equal(t, 41, len(sunat.Cat03Codes))
		is.True(t, sunat.Cat03Valid("NIU"))
		is.True(t, !sunat.Cat03Valid("XXX"))
		is.Equal(t, 1, len(sunat.Cat03GreWeightCodes))
		is.Equal(t, "KGM", sunat.Cat03GreWeightCodes[0])
	})

	t.Run("should offer only the three monedas the issuance UI supports", func(t *testing.T) {
		is.Equal(t, 3, len(sunat.Cat02SelectCodes))
		is.True(t, sunat.Cat02Valid("PEN"))
		is.True(t, !sunat.Cat02Valid("XYZ"))
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
