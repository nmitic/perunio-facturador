package xmlbuilder

import (
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// FillSummaryTotals sets an RC line's valor de venta per tipo de operación
// (Cat.11) from the comprobante's own lines. subtotal is the document's net
// total valor de venta (descuentos globales already applied): the gravada is
// what remains of it after the exonerada, inafecta and exportación lines, so a
// global discount, which only reduces the gravada base, needs no recomputation.
//
// Gratuito lines are not part of subtotal. Their referential base goes to
// TotalFree and, by type, to the TotalFree* fields; the gravadas' would-be IGV
// to TotalFreeIGV.
func FillSummaryTotals(item *model.SummaryItem, subtotal string, lines []model.LineItem) {
	var exo, inaf, exp, freeGrav, freeExo, freeInaf, freeIGV float64
	for _, li := range lines {
		code := li.TaxExemptionReasonCode
		if sunat.Cat07Gratuito(code) {
			base := gratuitoReferentialBase(li)
			switch {
			case sunat.Cat07GravadoGratuito(code):
				freeGrav += base
				freeIGV += parseDecimal(li.IGVAmount)
			// Cat.07 groups códigos by their first digit: 2x exonerado,
			// 3x inafecto. There is no gratuito exportación código.
			case strings.HasPrefix(code, "2"):
				freeExo += base
			default:
				freeInaf += base
			}
			continue
		}
		switch sunat.Cat07TaxSchemeCode(code) {
		case sunat.Cat05Exonerado:
			exo += parseDecimal(li.LineTotal)
		case sunat.Cat05Inafecto:
			inaf += parseDecimal(li.LineTotal)
		case sunat.Cat05Exportacion:
			exp += parseDecimal(li.LineTotal)
		}
	}

	gravada := parseDecimal(subtotal) - exo - inaf - exp
	if gravada < 0 {
		gravada = 0
	}
	item.TotalGravada = formatDecimal(gravada)
	item.TotalExonerated = formatDecimal(exo)
	item.TotalUnaffected = formatDecimal(inaf)
	item.TotalExport = formatDecimal(exp)
	item.TotalFree = formatDecimal(freeGrav + freeExo + freeInaf)
	item.TotalFreeGravada = formatDecimal(freeGrav)
	item.TotalFreeExonerated = formatDecimal(freeExo)
	item.TotalFreeUnaffected = formatDecimal(freeInaf)
	item.TotalFreeIGV = formatDecimal(freeIGV)
}
