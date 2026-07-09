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

// formatUnitPrice renders a valor unitario for cac:Price/cbc:PriceAmount, which
// SUNAT allows at up to 10 decimals (n(12,10)). Trailing zeros are trimmed but
// at least 2 decimals are kept. The extra precision matters when a net unit
// price is derived by dividing a line total by a fractional/large quantity, so
// that Qty × Price reconciles to cbc:LineExtensionAmount within tolerance.
func formatUnitPrice(v float64) string {
	s := strconv.FormatFloat(v, 'f', 10, 64)
	if strings.ContainsRune(s, '.') {
		s = strings.TrimRight(s, "0")
		if dot := strings.IndexByte(s, '.'); len(s)-dot-1 < 2 {
			s += strings.Repeat("0", 2-(len(s)-dot-1))
		}
	}
	return s
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
	XMLName              xml.Name `xml:"Invoice"`
	XMLNS                string   `xml:"xmlns,attr"`
	XMLNSCAC             string   `xml:"xmlns:cac,attr"`
	XMLNSCBC             string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT             string   `xml:"xmlns:ext,attr"`
	XMLNSSAC             string   `xml:"xmlns:sac,attr"`
	XMLNSDS              string   `xml:"xmlns:ds,attr"`
	UBLExtensions        ublExtensions
	UBLVersionID         string `xml:"cbc:UBLVersionID"`
	CustomizationID      string `xml:"cbc:CustomizationID"`
	ProfileID            profileIDElement
	ID                   string `xml:"cbc:ID"`
	IssueDate            string `xml:"cbc:IssueDate"`
	IssueTime            string `xml:"cbc:IssueTime,omitempty"`
	InvoiceTypeCode      invoiceTypeCode
	Notes                []noteElement
	DocumentCurrencyCode documentCurrencyCode
	Signature            cacSignature
	SupplierParty        accountingSupplierParty
	CustomerParty        accountingCustomerParty
	// Detracción cuenta BN (cac:PaymentMeans) — UBL places it before
	// cac:PaymentTerms. nil when the document is not subject to detracción.
	PaymentMeans *paymentMeans
	PaymentTerms []paymentTerms
	// Document-level descuento global (UBL puts cac:AllowanceCharge before
	// cac:TaxTotal). nil when there is no global discount.
	AllowanceCharge    *lineAllowanceCharge `xml:"cac:AllowanceCharge,omitempty"`
	TaxTotal           taxTotal
	LegalMonetaryTotal legalMonetaryTotal `xml:"cac:LegalMonetaryTotal"`
	InvoiceLines       []invoiceLine
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

	// SUNAT leyenda 2006 — operación sujeta a detracción.
	inv.Notes = appendDetraccionLegend(inv.Notes, req)

	// Detracción cuenta BN (cac:PaymentMeans) — emitted before PaymentTerms.
	inv.PaymentMeans = buildDetraccionPaymentMeans(req)

	// Forma de pago (SUNAT err 3244): Contado, or Credito + one entry per cuota.
	inv.PaymentTerms = buildPaymentTerms(req)

	// Detracción código/porcentaje/monto — extra cac:PaymentTerms entry.
	if pt := buildDetraccionPaymentTerms(req); pt != nil {
		inv.PaymentTerms = append(inv.PaymentTerms, *pt)
	}

	// Descuento global (Cat.53 code 02) — document-level cac:AllowanceCharge.
	inv.AllowanceCharge = buildGlobalDiscount(req)

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
// from req.Items. Gravado-gratuito lines (codes 11-16) contribute their
// referential base + would-be IGV to the gratuita (9996) bucket so the document
// can declare a matching cac:TaxSubtotal — SUNAT reconciles the line gratuita
// base against this document subtotal (faults 3272/3306).
type docTaxBuckets struct {
	regularBase float64
	regularIGV  float64
	ivapBase    float64
	ivapIGV     float64
	exoBase     float64
	inafBase    float64
	expBase     float64
	gratBase    float64
	gratIGV     float64
}

func sumLineBuckets(items []model.LineItem) docTaxBuckets {
	var b docTaxBuckets
	for _, li := range items {
		taxCode := model.TaxCodeForAffectation(li.TaxExemptionReasonCode)
		if isGratuitoCode(li.TaxExemptionReasonCode) {
			// Every gratuito line contributes its referential base (+ would-be
			// IGV) to the 9996 bucket. IGV is 18% for gravado-gratuito (11-16)
			// and 0 for exonerado/inafecto gratuito (21, 31-37).
			b.gratBase += gratuitoReferentialBase(li)
			b.gratIGV += parseDecimal(li.IGVAmount)
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

// igvRate is the IGV rate (18%) applied to the gravado base imponible.
const igvRate = 0.18

// globalDiscountReasonCode is Cat.53 "02" — descuento global que afecta la base
// imponible del IGV/IVAP, the only global-discount type this service emits.
const globalDiscountReasonCode = "02"

// globalDiscountValue returns the parsed descuento global amount, or 0 when none.
func globalDiscountValue(req model.IssueRequest) float64 {
	return parseDecimal(req.GlobalDiscount)
}

// buildGlobalDiscount returns the document-level cac:AllowanceCharge for a
// descuento global (Cat.53 code 02), or nil when there is none. Per the SUNAT
// factura guide (punto 21/35) it carries ChargeIndicator=false, the gravado base
// as cbc:BaseAmount and the discount as cbc:Amount; SUNAT then reconciles the
// gravado cac:TaxSubtotal/cbc:TaxableAmount as BaseAmount − Amount (see
// buildDocumentTaxTotal) and the totals as LineExtensionAmount −
// AllowanceTotalAmount + TaxAmount.
func buildGlobalDiscount(req model.IssueRequest) *lineAllowanceCharge {
	amt := globalDiscountValue(req)
	if amt <= 0 {
		return nil
	}
	base := sumLineBuckets(req.Items).regularBase
	var factor float64
	if base != 0 {
		factor = amt / base
	}
	return &lineAllowanceCharge{
		ChargeIndicator:         false,
		AllowanceChargeReason:   globalDiscountReasonCode,
		MultiplierFactorNumeric: strconv.FormatFloat(factor, 'f', 5, 64),
		Amount:                  newCurrencyAmount(formatDecimal(amt), req.CurrencyCode),
		BaseAmount:              newCurrencyAmount(formatDecimal(base), req.CurrencyCode),
	}
}

func buildDocumentTaxTotal(req model.IssueRequest) taxTotal {
	cur := req.CurrencyCode
	buckets := sumLineBuckets(req.Items)

	// Descuento global (Cat.53 code 02) reduces the gravado base imponible: the
	// gravado cac:TaxSubtotal must declare BaseAmount − discount and the IGV is
	// recomputed at 18% on the reduced base, or SUNAT rejects the IGV total.
	if gd := globalDiscountValue(req); gd > 0 {
		buckets.regularBase -= gd
		if buckets.regularBase < 0 {
			buckets.regularBase = 0
		}
		buckets.regularIGV = parseDecimal(formatDecimal(buckets.regularBase * igvRate))
	}

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

	// Gratuita (9996) subtotal — gravado-gratuito lines. The document cbc:TaxAmount
	// is NOT increased (the customer pays no tax on gratuitas, so it stays the sum
	// of onerosa IGV/IVAP), but SUNAT requires the referential base + would-be IGV
	// to be consigned here under the GRA scheme so it reconciles with each line's
	// gratuita TaxSubtotal. Matches greenter's Factura-Gratuita output.
	if buckets.gratBase > 0 || buckets.gratIGV > 0 {
		// The rate tag is always emitted (fault 2992). 18% when there is would-be
		// IGV (gravado-gratuito, 11-16); 0.00 for exonerado/inafecto gratuito
		// (21, 31-37), which carry no IGV — matching their zero TaxAmount so
		// SUNAT's base×rate check still holds.
		gratPercent := "0.00"
		if buckets.gratIGV > 0 {
			gratPercent = "18.00"
		}
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.gratBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.gratIGV), cur),
			TaxCategory:   newTaxCategory(model.TaxGratuita.TaxCategoryID, gratPercent, model.TaxGratuita),
		})
	}

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
	// The descuento global we support is Cat.53 code "02" — afecta la base
	// imponible del IGV. SUNAT realises such a discount by REDUCING the valor de
	// venta (base imponible), NOT by cbc:AllowanceTotalAmount. So:
	//
	//   - cbc:LineExtensionAmount is reported NET of the global discount
	//     (req.Subtotal is the GROSS sum of line LineExtensionAmounts; subtract the
	//     discount). The lines themselves keep their gross value; only this total
	//     reflects the reduced valor de venta. Matches greenter's valorVenta.
	//   - cbc:AllowanceTotalAmount is OMITTED. It carries only the sum of discounts
	//     that do NOT affect the base (Cat.53 01/03), which we don't emit. Putting
	//     an afecta-base (02) discount here makes SUNAT's "sum of non-afecta global
	//     discounts" (0) differ from the declared total → fault 3300.
	//
	// The descuento itself stays documented as the //Invoice/cac:AllowanceCharge
	// (code 02) block and as the reduced gravado cac:TaxSubtotal/TaxableAmount.
	// Per-line descuentos likewise stay inside each line's LineExtensionAmount and
	// never surface as a document AllowanceTotalAmount.
	lineExtension := req.Subtotal
	if gd := globalDiscountValue(req); gd > 0 {
		lineExtension = formatDecimal(parseDecimal(req.Subtotal) - gd)
	}
	return legalMonetaryTotal{
		LineExtensionAmount: newCurrencyAmount(lineExtension, cur),
		TaxInclusiveAmount:  newCurrencyAmount(req.TaxInclusiveAmount, cur),
		PayableAmount:       newCurrencyAmount(req.TotalAmount, cur),
	}
}

func buildInvoiceLine(li model.LineItem, cur string) (invoiceLine, error) {
	// SUNAT rule 3271: gratuito lines emit cac:Price/PriceAmount = 0 (the
	// customer is charged nothing). FreeOfChargeIndicator is intentionally NOT
	// emitted (greenter doesn't, and it's not what SUNAT keys on).
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
		LineExtensionAmount: newCurrencyAmount(lineExtensionAmountFor(li), cur),
		PricingReference:    pr,
		AllowanceCharge:     buildLineDiscount(li, cur),
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(unitPrice, cur)},
	}, nil
}

// buildLineDiscount returns the line-level descuento por ítem (Cat.53 "00")
// when the line carries a non-zero discount, or nil otherwise. Without it
// SUNAT recomputes cbc:LineExtensionAmount as cac:Price/PriceAmount × Quantity
// and rejects the line with fault 3271 because the gross price no longer
// matches the net line total. Gratuito lines have a zero price and never carry
// a discount, so they are skipped.
func buildLineDiscount(li model.LineItem, cur string) *lineAllowanceCharge {
	if isZeroAmount(li.DiscountAmount) || isGratuitoCode(li.TaxExemptionReasonCode) {
		return nil
	}
	discount := parseDecimal(li.DiscountAmount)
	// BaseAmount is the gross line value (valor unitario × cantidad), i.e. the
	// net line total plus the discount being applied.
	base := parseDecimal(li.LineTotal) + discount
	var factor float64
	if base != 0 {
		factor = discount / base
	}
	return &lineAllowanceCharge{
		ChargeIndicator:         false,
		AllowanceChargeReason:   "00",
		MultiplierFactorNumeric: strconv.FormatFloat(factor, 'f', 5, 64),
		Amount:                  newCurrencyAmount(formatDecimal(discount), cur),
		BaseAmount:              newCurrencyAmount(formatDecimal(base), cur),
	}
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
	// every line, including gratuito (fault 2028 fires otherwise).
	if _, ok := validPriceTypeCodes[li.PriceTypeCode]; !ok {
		return nil, fmt.Errorf("xmlbuilder: line %d has invalid Cat.16 PriceTypeCode %q", li.LineNumber, li.PriceTypeCode)
	}

	// The referential PriceAmount carries a unit price whose meaning depends on
	// the Cat.16 code. For onerosa (01) it is the IGV-INCLUSIVE precio de venta,
	// which is exactly unitPriceWithTax. For gratuito (02) SUNAT instead reads it
	// as the IGV-EXCLUSIVE valor referencial — the per-unit base imponible — and
	// cross-checks it against cac:TaxSubtotal/cbc:TaxableAmount (fault 3272).
	// Reusing the IGV-inclusive price for gravado-gratuito (codes 11-16, IGV > 0)
	// makes the two diverge, so derive the IGV-exclusive per-unit base instead.
	refPrice := li.UnitPriceWithTax
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		if qty := parseDecimal(li.Quantity); qty != 0 {
			refPrice = formatDecimal(gratuitoReferentialBase(li) / qty)
		}
	}

	return &pricingReference{
		AlternativeConditionPrice: []alternativeConditionPrice{{
			PriceAmount: newCurrencyAmount(refPrice, cur),
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

// lineExtensionAmountFor returns the cbc:LineExtensionAmount for a line.
//
// SUNAT fault 3272 ("la base imponible a nivel de línea difiere…") fires when a
// line's value differs from its declared base imponible. For every gratuito
// line (Cat.07 11-16, 21, 31-37) the base imponible is the IGV-exclusive
// referential — NOT the zero amount the customer pays — so LineExtensionAmount
// must equal that referential and match cac:TaxSubtotal/cbc:TaxableAmount
// (greenter's Factura-Gratuita emits both as the same value). Onerosa lines use
// the ordinary valor de venta (li.LineTotal), which already matches their base.
func lineExtensionAmountFor(li model.LineItem) string {
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		return formatDecimal(gratuitoReferentialBase(li))
	}
	return li.LineTotal
}

// noteLineUnitPrice returns cac:Price/cbc:PriceAmount for a CreditNote/DebitNote
// line. Unlike an InvoiceLine, the SUNAT NC/ND line model has NO line-level
// cac:AllowanceCharge: per the official guide (punto 29) SUNAT computes the
// valor de venta del ítem as CreditedQuantity × cac:Price, with the descuento
// already deducted. So the descuento must be baked into the valor unitario
// here — the net per-unit price (post-discount LineExtensionAmount ÷ Quantity) —
// not declared as a separate AllowanceCharge (which SUNAT ignores → fault 3271).
// Gratuito lines charge nothing, so the price is 0.
func noteLineUnitPrice(li model.LineItem) string {
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		return "0.00"
	}
	qty := parseDecimal(li.Quantity)
	if qty == 0 {
		return li.UnitPrice
	}
	return formatUnitPrice(parseDecimal(lineExtensionAmountFor(li)) / qty)
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

	// 🔴 GRATUITO — every gratuito line (Cat.07 11-16, 21, 31-37) declares its
	// IGV-exclusive referential base under the GRA scheme (9996). SUNAT fault
	// 3224 requires that any line with a referential price > 0 be tributo 9996,
	// so the base must be the referential, NOT 0 — otherwise the operation looks
	// onerosa. TaxAmount is the declared (would-be) IGV: 18% for gravado-gratuito
	// (11-16, rules 3103 + 3111), 0 for exonerado/inafecto gratuito (21, 31-37).
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		taxableAmount = formatDecimal(gratuitoReferentialBase(li))
		taxAmount = li.IGVAmount
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
		// SUNAT requires the rate tag on every line tributo (fault 2992).
		// Gravado-gratuito (11-16) declares IGV at 18%; exonerado/inafecto
		// gratuito (21, 31-37) are not IGV-subject, so the rate is 0.00,
		// consistent with their zero TaxAmount.
		if isGravadoGratuitoCode(exemptionReasonCode) {
			tc.Percent = "18.00"
		} else {
			tc.Percent = "0.00"
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
