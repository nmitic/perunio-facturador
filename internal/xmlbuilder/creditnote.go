package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"strconv"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// creditNote is the UBL 2.1 CreditNote XML root element.
type creditNote struct {
	XMLName              xml.Name `xml:"CreditNote"`
	XMLNS                string   `xml:"xmlns,attr"`
	XMLNSCAC             string   `xml:"xmlns:cac,attr"`
	XMLNSCBC             string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT             string   `xml:"xmlns:ext,attr"`
	XMLNSDS              string   `xml:"xmlns:ds,attr"`
	UBLExtensions        ublExtensions
	UBLVersionID         string `xml:"cbc:UBLVersionID"`
	CustomizationID      string `xml:"cbc:CustomizationID"`
	ProfileID            profileIDElement
	ID                   string `xml:"cbc:ID"`
	IssueDate            string `xml:"cbc:IssueDate"`
	IssueTime            string `xml:"cbc:IssueTime,omitempty"`
	Notes                []noteElement
	DocumentCurrencyCode documentCurrencyCode
	DiscrepancyResponse  discrepancyResponse
	BillingReference     billingReference
	Signature            cacSignature
	SupplierParty        accountingSupplierParty
	CustomerParty        accountingCustomerParty
	// Detracción (cac:PaymentMeans + cac:PaymentTerms) — present only when the
	// nota references a factura sujeta a detracción. nil/empty otherwise.
	PaymentMeans       *paymentMeans
	PaymentTerms       []paymentTerms
	TaxTotal           taxTotal
	LegalMonetaryTotal legalMonetaryTotal `xml:"cac:LegalMonetaryTotal"`
	CreditNoteLines    []creditNoteLine
}

// buildCreditNoteXML creates UBL 2.1 CreditNote XML bytes.
func buildCreditNoteXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	refID := fmt.Sprintf("%s-%08d", req.ReferenceDocSeries, req.ReferenceDocCorrelative)

	root := ublRootFor(sunat.UblDocumentCreditNote)

	cn := creditNote{
		XMLNS:    root.NS,
		XMLNSCAC: nsCAC,
		XMLNSCBC: nsCBC,
		XMLNSEXT: nsEXT,
		XMLNSDS:  nsDS,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    root.UBLVersionID,
		CustomizationID: root.CustomizationID,
		ProfileID:       newProfileID(req.OperationType, req.Detraccion),
		ID:              docID,
		IssueDate:       req.IssueDate,
		IssueTime:       req.IssueTime,

		DocumentCurrencyCode: newDocumentCurrencyCode(req.CurrencyCode),

		DiscrepancyResponse: discrepancyResponse{
			ReferenceID:  refID,
			ResponseCode: req.ReasonCode,
			Description:  req.ReasonDescription,
		},
		BillingReference: billingReference{
			InvoiceDocumentReference: invoiceDocumentReference{
				ID:               refID,
				DocumentTypeCode: req.ReferenceDocType,
			},
		},

		Signature:     newCACSignature(req.SupplierRUC, req.SupplierName),
		SupplierParty: newSupplierParty(req.SupplierRUC, req.SupplierName, req.SupplierAddress, req.EstablishmentCode),
		CustomerParty: newCustomerParty(req.CustomerDocType, req.CustomerDocNumber, req.CustomerName, req.CustomerAddress),
	}

	for _, n := range req.Notes {
		cn.Notes = append(cn.Notes, noteElement{
			Value:            n.Text,
			LanguageLocaleID: n.Code,
		})
	}

	// Detracción (SPOT) — leyenda 2006 + cuenta BN + código/porcentaje/monto,
	// mirrored from the referenced factura.
	cn.Notes = appendDetraccionLegend(cn.Notes, req)
	cn.PaymentMeans = buildDetraccionPaymentMeans(req)
	if pt := buildDetraccionPaymentTerms(req); pt != nil {
		cn.PaymentTerms = append(cn.PaymentTerms, *pt)
	}

	// Descuento global (Cat.53 code 02). Unlike an Invoice, SUNAT does NOT honour
	// a document-level cac:AllowanceCharge on a CreditNote: it reconciles the sum
	// of the line valores de venta against the document gravado total (fault 3277
	// otherwise). So the discount is baked into the gravado line net prices and
	// the document totals are recomputed from those reduced lines — no doc-level
	// AllowanceCharge is emitted.
	docReq := req
	if gd := globalDiscountValue(req); gd > 0 {
		docReq.Items = distributeGlobalDiscountToNoteLines(req.Items, gd)
		docReq.GlobalDiscount = "0"
		docReq.Subtotal = formatDecimal(parseDecimal(req.Subtotal) - gd)
	}

	cn.TaxTotal = buildDocumentTaxTotal(docReq)
	cn.LegalMonetaryTotal = buildLegalMonetaryTotal(docReq)

	for _, li := range docReq.Items {
		line, err := buildCreditNoteLine(li, docReq.CurrencyCode)
		if err != nil {
			return nil, err
		}
		cn.CreditNoteLines = append(cn.CreditNoteLines, line)
	}

	return marshalISO8859(&cn)
}

// distributeGlobalDiscountToNoteLines bakes a descuento global (Cat.53 02) into
// the gravado (IGV/IVAP) line valores de venta of a nota, proportionally to each
// line's base. SUNAT's CreditNote has no honoured document-level
// cac:AllowanceCharge — it reconciles Σ line valor de venta against the document
// gravado total (fault 3277) — so the discount must live in the lines.
//
// Each gravado line's LineTotal, IGVAmount and per-unit prices are scaled by
// factor = (base − share) / base. The shares are rounded to 2 decimals and the
// LAST gravado line absorbs the rounding remainder so Σ share == discount
// exactly. Non-gravado lines (exonerado/inafecto/gratuito/export) are untouched.
func distributeGlobalDiscountToNoteLines(items []model.LineItem, discount float64) []model.LineItem {
	if discount <= 0 {
		return items
	}

	var gravadoGross float64
	var gravadoIdx []int
	for i, li := range items {
		if isGratuitoCode(li.TaxExemptionReasonCode) {
			continue
		}
		switch model.TaxCodeForAffectation(li.TaxExemptionReasonCode) {
		case "1000", "1016":
			gravadoGross += parseDecimal(li.LineTotal)
			gravadoIdx = append(gravadoIdx, i)
		}
	}
	if gravadoGross <= 0 || len(gravadoIdx) == 0 {
		return items
	}

	out := make([]model.LineItem, len(items))
	copy(out, items)

	var allocated float64
	for n, idx := range gravadoIdx {
		li := out[idx]
		base := parseDecimal(li.LineTotal)
		var share float64
		if n == len(gravadoIdx)-1 {
			share = discount - allocated // last line absorbs the rounding remainder
		} else {
			share = parseDecimal(formatDecimal(discount * base / gravadoGross))
			allocated += share
		}
		newBase := base - share
		if newBase < 0 {
			newBase = 0
		}
		var factor float64
		if base != 0 {
			factor = newBase / base
		}
		// Scale the value, the (would-be) IGV and the per-unit prices by the same
		// factor so the line stays internally consistent regardless of rate/ISC.
		li.LineTotal = formatDecimal(newBase)
		li.IGVAmount = formatDecimal(parseDecimal(li.IGVAmount) * factor)
		li.ISCAmount = formatDecimal(parseDecimal(li.ISCAmount) * factor)
		li.UnitPrice = formatDecimal(parseDecimal(li.UnitPrice) * factor)
		li.UnitPriceWithTax = formatDecimal(parseDecimal(li.UnitPriceWithTax) * factor)
		out[idx] = li
	}
	return out
}

func buildCreditNoteLine(li model.LineItem, cur string) (creditNoteLine, error) {
	pr, err := buildPricingReference(li, cur)
	if err != nil {
		return creditNoteLine{}, err
	}
	// SUNAT's CreditNoteLine has no line-level cac:AllowanceCharge: the descuento
	// is baked into the net valor unitario (noteLineUnitPrice). See its doc comment.
	return creditNoteLine{
		ID:                  strconv.Itoa(li.LineNumber),
		CreditedQuantity:    quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(lineExtensionAmountFor(li), cur),
		PricingReference:    pr,
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(noteLineUnitPrice(li), cur)},
	}, nil
}
