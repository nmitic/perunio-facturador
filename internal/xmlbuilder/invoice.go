package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/perunio/perunio-facturador/internal/model"
)

// isZeroAmount returns true for any decimal string that represents zero
// (e.g. "0", "0.0", "0.00", "-0.00") or is empty.
func isZeroAmount(s string) bool {
	return parseDecimal(s) == 0
}

// parseDecimal returns the float value of a decimal string ("0" on parse error).
func parseDecimal(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("xmlbuilder: parseDecimal failed for %q, defaulting to 0: %v", s, err)
		return 0
	}
	return v
}

// formatDecimal renders a float as a fixed-2 decimal string.
func formatDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// isGratuitoCode reports whether a Cat.07 affectation code marks the line as
// transferencia/retiro a título gratuito (codes 11-16, 21, 31-37).
func isGratuitoCode(code string) bool {
	switch code {
	case "11", "12", "13", "14", "15", "16",
		"21",
		"31", "32", "33", "34", "35", "36", "37":
		return true
	}
	return false
}

// isGravadoGratuitoCode reports whether a Cat.07 code is gravado-gratuito
// (would have been subject to IGV had it been onerosa). SUNAT requires these
// to declare a non-zero IGV at 18% (rule 3111).
func isGravadoGratuitoCode(code string) bool {
	switch code {
	case "11", "12", "13", "14", "15", "16":
		return true
	}
	return false
}

// invoice is the UBL 2.1 Invoice XML root element.
type invoice struct {
	XMLName                  xml.Name `xml:"Invoice"`
	XMLNS                    string   `xml:"xmlns,attr"`
	XMLNSCAC                 string   `xml:"xmlns:cac,attr"`
	XMLNSCBC                 string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT                 string   `xml:"xmlns:ext,attr"`
	XMLNSSAC                 string   `xml:"xmlns:sac,attr"`
	XMLNSDS                  string   `xml:"xmlns:ds,attr"`
	UBLExtensions            ublExtensions
	UBLVersionID             string `xml:"cbc:UBLVersionID"`
	CustomizationID          string `xml:"cbc:CustomizationID"`
	ProfileID                profileIDElement
	ID                       string `xml:"cbc:ID"`
	IssueDate                string `xml:"cbc:IssueDate"`
	IssueTime                string `xml:"cbc:IssueTime,omitempty"`
	InvoiceTypeCode          invoiceTypeCode
	Notes                    []noteElement
	DocumentCurrencyCode     documentCurrencyCode
	Signature                cacSignature
	SupplierParty            accountingSupplierParty
	CustomerParty            accountingCustomerParty
	PaymentTerms             []paymentTerms
	TaxTotal                 taxTotal
	LegalMonetaryTotal       legalMonetaryTotal
	InvoiceLines             []invoiceLine
}

// buildInvoiceXML creates UBL 2.1 Invoice XML bytes from an issue request.
func buildInvoiceXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)

	inv := invoice{
		XMLNS:    NSInvoice,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSEXT: NSEXT,
		XMLNSSAC: NSSAC,
		XMLNSDS:  NSDS,

		UBLExtensions: ublExtensions{
			Extension: buildInvoiceExtensions(req),
		},

		UBLVersionID:    UBLVersion21,
		CustomizationID: CustomizationID20,
		ProfileID:       newProfileID(req.OperationType),
		ID:              docID,
		IssueDate:       req.IssueDate,
		IssueTime:       req.IssueTime,
		InvoiceTypeCode: newInvoiceTypeCode(req.DocType, req.OperationType),

		DocumentCurrencyCode: newDocumentCurrencyCode(req.CurrencyCode),
		Signature:            newCACSignature(req.SupplierRUC, req.SupplierName),
		SupplierParty:        newSupplierParty(req.SupplierRUC, req.SupplierName, req.SupplierAddress, req.EstablishmentCode),
		CustomerParty:        newCustomerParty(req.CustomerDocType, req.CustomerDocNumber, req.CustomerName, req.CustomerAddress),
	}

	// Notes
	for _, n := range req.Notes {
		inv.Notes = append(inv.Notes, noteElement{
			Value:            n.Text,
			LanguageLocaleID: n.Code,
		})
	}

	// SUNAT leyenda 1002 — auto-injected when any line is gratuito and the
	// caller didn't already supply it. Required by Anexo 8 for documents that
	// include transferencia/retiro a título gratuito lines.
	if hasGratuitoLine(req.Items) && !hasNoteWithCode(inv.Notes, model.LegendTransfGratuita) {
		inv.Notes = append(inv.Notes, noteElement{
			Value:            "TRANSFERENCIA GRATUITA O A TITULO GRATUITO",
			LanguageLocaleID: model.LegendTransfGratuita,
		})
	}

	// Forma de pago (SUNAT err 3244): Contado, or Credito + one entry per cuota.
	inv.PaymentTerms = buildPaymentTerms(req)

	// Tax totals
	inv.TaxTotal = buildDocumentTaxTotal(req)

	// Monetary totals
	inv.LegalMonetaryTotal = buildLegalMonetaryTotal(req)

	// Lines
	for _, li := range req.Items {
		line, err := buildInvoiceLine(li, req.CurrencyCode)
		if err != nil {
			return nil, err
		}
		inv.InvoiceLines = append(inv.InvoiceLines, line)
	}

	return marshalISO8859(&inv)
}

// buildPaymentTerms emits one cac:PaymentTerms entry for Contado, or
// 1 + N entries for Credito (the leading "Credito" entry plus one per cuota).
// Cuota PaymentMeansIDs are zero-padded 3-digit per SUNAT spec ("Cuota001"...).
func buildPaymentTerms(req model.IssueRequest) []paymentTerms {
	if !strings.EqualFold(strings.TrimSpace(req.FormaPago), "credito") {
		return []paymentTerms{{ID: "FormaPago", PaymentMeansID: "Contado"}}
	}
	out := make([]paymentTerms, 0, len(req.Cuotas)+1)
	// SUNAT err 3251: leading Credito entry must carry the net pending amount.
	netPending := newCurrencyAmount(req.TotalAmount, req.CurrencyCode)
	out = append(out, paymentTerms{ID: "FormaPago", PaymentMeansID: "Credito", Amount: &netPending})
	for _, c := range req.Cuotas {
		amt := newCurrencyAmount(c.Monto, req.CurrencyCode)
		out = append(out, paymentTerms{
			ID:             "FormaPago",
			PaymentMeansID: fmt.Sprintf("Cuota%03d", c.Numero),
			Amount:         &amt,
			PaymentDueDate: c.FechaVencimiento,
		})
	}
	return out
}

// docTaxBuckets is the per-Cat.05 breakdown of a document's lines, computed
// from req.Items. Gratuito lines are excluded — SUNAT only allows gratuita
// taxes at line level (cac:InvoiceLine/cac:TaxTotal), not at document level.
type docTaxBuckets struct {
	regularBase float64
	regularIGV  float64
	ivapBase    float64
	ivapIGV     float64
	exoBase     float64
	inafBase    float64
	expBase     float64
}

func sumLineBuckets(items []model.LineItem) docTaxBuckets {
	var b docTaxBuckets
	for _, li := range items {
		taxCode := model.TaxCodeForAffectation(li.TaxExemptionReasonCode)
		if isGratuitoCode(li.TaxExemptionReasonCode) {
			continue
		}
		base := parseDecimal(li.LineTotal)
		isc := parseDecimal(li.ISCAmount)
		igv := parseDecimal(li.IGVAmount)
		switch taxCode {
		case "1000":
			// IGV base = valor_venta + ISC when ISC applies (SUNAT spec).
			b.regularBase += base + isc
			b.regularIGV += igv
		case "1016":
			b.ivapBase += base
			b.ivapIGV += igv
		case "9997":
			b.exoBase += base
		case "9998":
			b.inafBase += base
		case "9995":
			b.expBase += base
		default:
			b.regularBase += base + isc
			b.regularIGV += igv
		}
	}
	return b
}

func buildDocumentTaxTotal(req model.IssueRequest) taxTotal {
	cur := req.CurrencyCode
	buckets := sumLineBuckets(req.Items)

	// Document-level cbc:TaxAmount: SUNAT expects the sum of taxes declared
	// across all subtotals (IGV + IVAP). Gratuita IGV is informativo and lives
	// inside its own subtotal; including it in the top-level TaxAmount also
	// matches the value emitted for documents without gratuito lines.
	totalTax := buckets.regularIGV + buckets.ivapIGV
	tt := taxTotal{
		TaxAmount: newCurrencyAmount(formatDecimal(totalTax), cur),
	}

	// IGV (regular gravado, 18%) subtotal
	if buckets.regularBase > 0 || buckets.regularIGV > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.regularBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.regularIGV), cur),
			TaxCategory:   newTaxCategory("S", "18.00", model.TaxIGV),
		})
	}

	// IVAP (4%) subtotal
	if buckets.ivapBase > 0 || buckets.ivapIGV > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.ivapBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.ivapIGV), cur),
			TaxCategory:   newTaxCategory("S", "4.00", model.TaxIVAP),
		})
	}

	// Gratuita: emitted only at line level (cac:InvoiceLine/cac:TaxTotal).
	// SUNAT rejects a document-level Gratuita subtotal.

	// ISC subtotal (document-level total comes from req.TotalISC).
	if !isZeroAmount(req.TotalISC) {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount(req.TotalISC, cur),
			TaxCategory:   newTaxCategory("S", "", model.TaxISC),
		})
	}

	// Other taxes subtotal (if present)
	if !isZeroAmount(req.TotalOtherTaxes) {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount(req.TotalOtherTaxes, cur),
			TaxCategory:   newTaxCategory("S", "", model.TaxOtros),
		})
	}

	// Fallback when no breakdown applies (e.g. exonerado-only document): emit
	// a single zero IGV subtotal so SUNAT doesn't reject for missing TaxTotal.
	if len(tt.TaxSubtotal) == 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount("0.00", cur),
			TaxCategory:   newTaxCategory("S", "18.00", model.TaxIGV),
		})
	}

	return tt
}

func hasGratuitoLine(items []model.LineItem) bool {
	for _, li := range items {
		if isGratuitoCode(li.TaxExemptionReasonCode) {
			return true
		}
	}
	return false
}

func hasNoteWithCode(notes []noteElement, code string) bool {
	for _, n := range notes {
		if n.LanguageLocaleID == code {
			return true
		}
	}
	return false
}

func buildLegalMonetaryTotal(req model.IssueRequest) legalMonetaryTotal {
	cur := req.CurrencyCode
	lmt := legalMonetaryTotal{
		LineExtensionAmount: newCurrencyAmount(req.Subtotal, cur),
		TaxInclusiveAmount:  newCurrencyAmount(req.TaxInclusiveAmount, cur),
		PayableAmount:       newCurrencyAmount(req.TotalAmount, cur),
	}

	if !isZeroAmount(req.TotalDiscount) {
		amt := newCurrencyAmount(req.TotalDiscount, cur)
		lmt.AllowanceTotalAmount = &amt
	}

	return lmt
}

func buildInvoiceLine(li model.LineItem, cur string) (invoiceLine, error) {
	// For gratuito lines: cbc:LineExtensionAmount and cac:Price/PriceAmount
	// are both 0 (rules 2640, 3271). The referential base (100) and IGV (18)
	// only live in cac:TaxSubtotal and in cac:PricingReference. SUNAT's 3272
	// has a gratuito-specific path keyed on TaxScheme/ID=9996, so the
	// LineExtensionAmount/TaxableAmount divergence is allowed; FreeOfChargeIndicator
	// is intentionally NOT emitted (greenter doesn't, and it's not what SUNAT
	// keys on for the carve-out).
	// SUNAT rule 3271: gratuito lines must emit cac:Price/PriceAmount = 0.
	unitPrice := li.UnitPrice
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		unitPrice = "0.00"
	}

	pr, err := buildPricingReference(li, cur)
	if err != nil {
		return invoiceLine{}, err
	}

	return invoiceLine{
		ID:                  strconv.Itoa(li.LineNumber),
		InvoicedQuantity:    quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(li.LineTotal, cur),
		PricingReference:    pr,
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(unitPrice, cur)},
	}, nil
}

// validPriceTypeCodes is the set of accepted Cat.16 PriceTypeCode values.
// 01 = onerosa, 02 = referencial gratuita, 03 = referencial exportación.
var validPriceTypeCodes = map[string]struct{}{
	"01": {},
	"02": {},
	"03": {},
}

func buildPricingReference(li model.LineItem, cur string) (*pricingReference, error) {
	// SUNAT requires cac:PricingReference / cac:AlternativeConditionPrice on
	// every line, including gratuito (fault 2028 fires otherwise). For
	// gratuito lines the PriceTypeCode is "02" (referencial) and the
	// PriceAmount is the IGV-inclusive referential value.
	if _, ok := validPriceTypeCodes[li.PriceTypeCode]; !ok {
		return nil, fmt.Errorf("xmlbuilder: line %d has invalid Cat.16 PriceTypeCode %q", li.LineNumber, li.PriceTypeCode)
	}
	return &pricingReference{
		AlternativeConditionPrice: []alternativeConditionPrice{{
			PriceAmount: newCurrencyAmount(li.UnitPriceWithTax, cur),
			PriceTypeCode: priceTypeCode{
				Value:          li.PriceTypeCode,
				ListName:       "Tipo de Precio",
				ListAgencyName: "PE:SUNAT",
				ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo16",
			},
		}},
	}, nil
}

// gratuitoReferentialBase returns the IGV-exclusive referential base for a
// gratuito line: (UnitPriceWithTax × Quantity) − IGVAmount. For exonerado
// (21) and inafecto (31-37) gratuito codes IGVAmount is 0, so the formula
// degenerates to UnitPriceWithTax × Quantity. This is the value SUNAT expects
// in cac:TaxSubtotal/cbc:TaxableAmount and aggregated into
// sac:AdditionalMonetaryTotal ID=1004.
func gratuitoReferentialBase(li model.LineItem) float64 {
	gross := parseDecimal(li.UnitPriceWithTax) * parseDecimal(li.Quantity)
	return gross - parseDecimal(li.IGVAmount)
}

// buildAdditionalMonetaryTotals emits sac:AdditionalMonetaryTotal entries.
// Currently only ID=1004 (total valor referencial of operaciones gratuitas)
// is produced, when the document contains at least one gratuito line. SUNAT
// requires this block to interpret invoices whose LegalMonetaryTotal is zero
// but whose lines carry referential value (transferencia gratuita).
func buildAdditionalMonetaryTotals(req model.IssueRequest) []additionalMonetaryTotal {
	var totalFreeBase float64
	for _, li := range req.Items {
		if isGratuitoCode(li.TaxExemptionReasonCode) {
			totalFreeBase += gratuitoReferentialBase(li)
		}
	}
	if totalFreeBase == 0 {
		return nil
	}
	return []additionalMonetaryTotal{{
		ID:            "1004",
		PayableAmount: newCurrencyAmount(formatDecimal(totalFreeBase), req.CurrencyCode),
	}}
}

// buildInvoiceExtensions returns the ext:UBLExtension list for an Invoice.
// SUNAT places sac:AdditionalInformation (with AdditionalMonetaryTotals such
// as ID=1004 for gratuito) inside its own ext:UBLExtension, separate from the
// XMLDSig signature extension. The signature extension is always last.
func buildInvoiceExtensions(req model.IssueRequest) []ublExtension {
	var exts []ublExtension
	if amts := buildAdditionalMonetaryTotals(req); len(amts) > 0 {
		exts = append(exts, ublExtension{
			ExtensionContent: extensionContent{
				AdditionalInformation: &sacAdditionalInformation{
					AdditionalMonetaryTotals: amts,
				},
			},
		})
	}
	exts = append(exts, ublExtension{ExtensionContent: newExtensionContent()})
	return exts
}

func buildLineTaxTotal(li model.LineItem, cur string) taxTotal {
	taxCode := model.TaxCodeForAffectation(li.TaxExemptionReasonCode)

	ts, ok := model.TaxSchemeByCode(taxCode)
	if !ok {
		ts = model.TaxIGV
	}

	// Default (onerosa)
	taxableAmount := li.LineTotal
	taxAmount := li.IGVAmount

	// 🔴 GRATUITO:
	//   - gravado-gratuito (codes 11-16): TaxableAmount is the IGV-exclusive
	//     referential base ((UnitPriceWithTax × Qty) - IGVAmount), TaxAmount is
	//     the declared IGV (SUNAT rules 3103 + 3111).
	//   - exonerado-/inafecto-gratuito (21, 31-37): both 0.00.
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		if isGravadoGratuitoCode(li.TaxExemptionReasonCode) {
			taxableAmount = formatDecimal(gratuitoReferentialBase(li))
			taxAmount = li.IGVAmount
		} else {
			taxableAmount = "0.00"
			taxAmount = "0.00"
		}
	} else {
		// ✅ ISC rule: IGV base includes ISC
		if !isZeroAmount(li.ISCAmount) && taxCode == "1000" {
			base := parseDecimal(li.LineTotal) + parseDecimal(li.ISCAmount)
			taxableAmount = formatDecimal(base)
		}
	}

	tt := taxTotal{
		TaxAmount: newCurrencyAmount(taxAmount, cur),
		TaxSubtotal: []taxSubtotal{
			{
				TaxableAmount: newCurrencyAmount(taxableAmount, cur),
				TaxAmount:     newCurrencyAmount(taxAmount, cur),
				TaxCategory:   newLineTaxCategory(li.TaxExemptionReasonCode, ts),
			},
		},
	}

	// ✅ ISC subtotal (only for onerosa)
	if !isZeroAmount(li.ISCAmount) && !isGratuitoCode(li.TaxExemptionReasonCode) {
		iscCat := newTaxCategory("S", "", model.TaxISC)
		iscCat.TierRange = li.ISCTierRange

		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(li.LineTotal, cur),
			TaxAmount:     newCurrencyAmount(li.ISCAmount, cur),
			TaxCategory:   iscCat,
		})
	}

	return tt
}

func newTaxCategory(categoryID, percent string, ts model.TaxSchemeType) taxCategory {
	tc := taxCategory{
		ID: taxCategoryID{
			Value:          categoryID,
			SchemeID:       "UN/ECE 5305",
			SchemeAgencyID: "6",
		},
		TaxScheme: taxSchemeXML{
			ID: taxSchemeID{
				Value:          ts.Code,
				SchemeID:       "UN/ECE 5153",
				SchemeAgencyID: "6",
			},
			Name:        ts.Name,
			TaxTypeCode: ts.TaxTypeCode,
		},
	}
	if percent != "" {
		tc.Percent = percent
	}
	return tc
}

func newLineTaxCategory(exemptionReasonCode string, ts model.TaxSchemeType) taxCategory {
	tc := newTaxCategory(ts.TaxCategoryID, "", ts)

	// Set percent based on tax type
	switch ts.Code {
	case "1000": // IGV
		tc.Percent = "18.00"
	case "1016": // IVAP
		tc.Percent = "4.00"
	case "9996": // Gratuita — gravado-gratuito (11-16) declares IGV at 18%
		if isGravadoGratuitoCode(exemptionReasonCode) {
			tc.Percent = "18.00"
		}
	}

	if exemptionReasonCode != "" {
		tc.TaxExemptionReasonCode = &taxExemptionCode{
			Value:          exemptionReasonCode,
			ListAgencyName: "PE:SUNAT",
			ListName:       "Afectacion del IGV",
			ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo07",
		}
	}

	return tc
}
