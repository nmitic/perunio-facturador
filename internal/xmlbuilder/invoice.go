package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
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
	// Anticipo document references (UBL puts cac:AdditionalDocumentReference
	// before cac:Signature). Empty except on facturas de regularización.
	AdditionalDocumentReferences []additionalDocumentReference `xml:"cac:AdditionalDocumentReference,omitempty"`
	Signature                    cacSignature
	SupplierParty                accountingSupplierParty
	CustomerParty                accountingCustomerParty
	// Detracción cuenta BN (cac:PaymentMeans) — UBL places it before
	// cac:PaymentTerms. nil when the document is not subject to detracción.
	PaymentMeans *paymentMeans
	PaymentTerms []paymentTerms
	// Anticipos applied (UBL puts cac:PrepaidPayment after cac:PaymentTerms and
	// before cac:AllowanceCharge). One entry per anticipo, row-paired with
	// AdditionalDocumentReferences.
	PrepaidPayments []prepaidPayment
	// Document-level allowances (UBL puts cac:AllowanceCharge before
	// cac:TaxTotal): the descuento global (Cat.53 02) if any, then one Cat.53 04
	// entry per anticipo applied.
	AllowanceCharges   []lineAllowanceCharge `xml:"cac:AllowanceCharge,omitempty"`
	TaxTotal           taxTotal
	LegalMonetaryTotal legalMonetaryTotal `xml:"cac:LegalMonetaryTotal"`
	InvoiceLines       []invoiceLine
}

// buildInvoiceXML creates UBL 2.1 Invoice XML bytes from an issue request.
func buildInvoiceXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)

	root := ublRootFor(sunat.UblDocumentInvoice)

	inv := invoice{
		XMLNS:    root.NS,
		XMLNSCAC: nsCAC,
		XMLNSCBC: nsCBC,
		XMLNSEXT: nsEXT,
		XMLNSSAC: nsSAC,
		XMLNSDS:  nsDS,

		UBLExtensions: ublExtensions{
			Extension: buildInvoiceExtensions(req),
		},

		UBLVersionID:    root.UBLVersionID,
		CustomizationID: root.CustomizationID,
		ProfileID:       newProfileID(req.OperationType, req.Detraccion),
		ID:              docID,
		IssueDate:       req.IssueDate,
		IssueTime:       req.IssueTime,
		InvoiceTypeCode: newInvoiceTypeCode(req.DocType, req.OperationType, req.Detraccion),

		DocumentCurrencyCode: newDocumentCurrencyCode(req.CurrencyCode),
		Signature:            newCACSignature(req.SupplierRUC, req.SupplierName),
		SupplierParty:        newSupplierParty(req.SupplierRUC, req.SupplierName, req.SupplierAddress, req.EstablishmentCode),
		CustomerParty:        newCustomerParty(req.CustomerDocType, req.CustomerDocNumber, req.CustomerName, req.CustomerAddress),
	}

	// Leyendas: 1000 (monto en letras) is generated, the rest come from req.
	inv.Notes = buildNotes(req)

	// SUNAT leyenda 1002 — auto-injected when any line is gratuito and the
	// caller didn't already supply it. Required by Anexo 8 for documents that
	// include transferencia/retiro a título gratuito lines.
	if hasGratuitoLine(req.Items) && !hasNoteWithCode(inv.Notes, sunat.Cat52TransfGratuita) {
		inv.Notes = append(inv.Notes, noteElement{
			Value:            sunat.Cat52Texto(sunat.Cat52TransfGratuita),
			LanguageLocaleID: sunat.Cat52TransfGratuita,
		})
	}

	// SUNAT leyenda 2007 — operación sujeta al IVAP. A factura or boleta with
	// IVAP lines that omits it is accepted with OBSERV 4264.
	if hasIVAPLine(req.Items) && !hasNoteWithCode(inv.Notes, sunat.Cat52IVAP) {
		inv.Notes = append(inv.Notes, noteElement{
			Value:            sunat.Cat52Texto(sunat.Cat52IVAP),
			LanguageLocaleID: sunat.Cat52IVAP,
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

	// Anticipos (factura de regularización): document references + prepaid
	// payments, row-paired via cbc:DocumentStatusCode = cbc:ID.
	inv.AdditionalDocumentReferences = buildAnticipoReferences(req)
	inv.PrepaidPayments = buildPrepaidPayments(req)

	// Document-level cac:AllowanceCharge entries: descuento global (Cat.53 02)
	// and/or the anticipo deductions (Cat.53 04).
	inv.AllowanceCharges = buildDocumentAllowances(req)

	// Tax totals
	inv.TaxTotal = buildDocumentTaxTotal(req)

	// Monetary totals
	inv.LegalMonetaryTotal = buildLegalMonetaryTotal(req)

	// Lines. The transporte de carga (1004) trip block describes the operation
	// the whole comprobante documents, and SUNAT's rules only require one
	// occurrence of each tag, so it rides on the first line alone. The recursos
	// hidrobiológicos (1002) concepts, in contrast, are checked on every line.
	hidrobiologicos := sunatOperationType(req.OperationType, req.Detraccion) == sunat.Cat51DetraccionHidrobiologicos
	for i, li := range req.Items {
		line, err := buildInvoiceLine(li, req.CurrencyCode)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			line.Delivery = buildLineDelivery(req)
		}
		if hidrobiologicos {
			props, err := buildHidrobiologicosProperties(req.Hidrobiologicos, li)
			if err != nil {
				return nil, err
			}
			line.Item.AdditionalItemProperty = props
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
	// SUNAT err 3251: leading Credito entry must carry the net pending amount —
	// the PAYABLE MINUS THE DETRACCIÓN, not the payable. The receptor deposits the
	// detracción into the emisor's restricted cuenta at the Banco de la Nación and
	// owes only the remainder on credit, so the cuotas below add up to this figure
	// and not to TotalAmount. See model.NetoPendientePago.
	netPending := newCurrencyAmount(formatDecimal(req.NetoPendientePago()), req.CurrencyCode)
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
	// gratGravBase is the part of gratBase from gravado gratuitos (11-16), the
	// base their would-be IGV (gratIGV) was computed on.
	gratGravBase float64
	// otrosBase / otrosAmount are the Σ of the otros tributos (9999) line
	// subtotals: the document 9999 subtotal must equal them (3297 / 3299).
	otrosBase   float64
	otrosAmount float64
	// iscBase / iscAmount are the Σ of the ISC line subtotals (onerosa lines
	// only): the document ISC subtotal must equal them (3296 / 3298).
	iscBase   float64
	iscAmount float64
}

func sumLineBuckets(items []model.LineItem) docTaxBuckets {
	var b docTaxBuckets
	for _, li := range items {
		taxCode := sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode)
		if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
			// Every gratuito line contributes its referential base (+ would-be
			// IGV) to the 9996 bucket. IGV is the issuer's rate (18 or 10.5) for
			// gravado-gratuito (11-16) and 0 for exonerado/inafecto gratuito
			// (21, 31-37).
			base := gratuitoReferentialBase(li)
			b.gratBase += base
			b.gratIGV += parseDecimal(li.IGVAmount)
			if sunat.Cat07GravadoGratuito(li.TaxExemptionReasonCode) {
				b.gratGravBase += base
			}
			continue
		}
		base := parseDecimal(li.LineTotal)
		isc := parseDecimal(li.ISCAmount)
		igv := parseDecimal(li.IGVAmount)
		if isc != 0 {
			b.iscBase += base
			b.iscAmount += isc
		}
		if hasOtrosTributos(li) {
			b.otrosBase += base
			b.otrosAmount += parseDecimal(li.OtherTaxesAmount)
		}
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

// The IGV rates SUNAT accepts on a gravado line ("por la tasa vigente del IGV
// (18.00 o 10.50)", rule 3279). 10.5 % is the MYPE restaurantes / hoteles /
// alojamientos regime (padrón 01, OBSERV 4439 otherwise). Cat.07 carries only
// 18.00, so the rate a line was taxed at is read from its own amounts.
const (
	igvRateGeneral = "18.00"
	igvRateReduced = "10.50"
)

// lineIGVRate is the IGV rate an onerosa gravado line was taxed at, snapped to
// the nearer of the two rates SUNAT accepts: IGV ÷ base (base includes ISC).
// A line with no base falls back to the general rate.
func lineIGVRate(li model.LineItem) string {
	return snapIGVRate(parseDecimal(li.IGVAmount), parseDecimal(li.LineTotal)+parseDecimal(li.ISCAmount))
}

// gratuitoIGVRate is the rate a gravado gratuito line (Cat.07 11-16) declares
// for its would-be IGV, read the same way from its referential base. ERROR 3462
// compares it with the onerosa lines', so a 10.5 % issuer's gratuitos are
// 10.5 % too, never the 18.00 Cat.07 lists.
func gratuitoIGVRate(li model.LineItem) string {
	return snapIGVRate(parseDecimal(li.IGVAmount), gratuitoReferentialBase(li))
}

// snapIGVRate snaps igv ÷ base to the nearer of the two rates SUNAT accepts.
// No base falls back to the general rate.
func snapIGVRate(igv, base float64) string {
	if base <= 0 {
		return igvRateGeneral
	}
	pct := igv / base * 100
	if math.Abs(pct-10.5) < math.Abs(pct-18) {
		return igvRateReduced
	}
	return igvRateGeneral
}

// documentIGVRate is the rate the document's gravado lines declare. Every IGV
// line of a document must declare the same one (ERROR 3462), so the first
// onerosa gravado line stands for all of them.
func documentIGVRate(items []model.LineItem) string {
	for _, li := range items {
		if isOnerosaIGVLine(li) {
			return lineIGVRate(li)
		}
	}
	return igvRateGeneral
}

func isOnerosaIGVLine(li model.LineItem) bool {
	return sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode) == sunat.Cat05IGV &&
		!sunat.Cat07Gratuito(li.TaxExemptionReasonCode)
}

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
	baseAmount := newCurrencyAmount(formatDecimal(base), req.CurrencyCode)
	return &lineAllowanceCharge{
		ChargeIndicator:         false,
		AllowanceChargeReason:   globalDiscountReasonCode,
		MultiplierFactorNumeric: strconv.FormatFloat(factor, 'f', 5, 64),
		Amount:                  newCurrencyAmount(formatDecimal(amt), req.CurrencyCode),
		BaseAmount:              &baseAmount,
	}
}

// anticipoReasonCode is Cat.53 "04" — descuento global por anticipos gravados
// que afecta la base imponible.
const anticipoReasonCode = "04"

// anticipoTotals sums the applied anticipos two ways: base is the IGV-exclusive
// total (what the Cat.53 "04" allowances deduct from the base imponible) and
// paid the IGV-inclusive one (cbc:PrepaidAmount). Both figures reach the XML,
// at different levels — see buildLegalMonetaryTotal.
func anticipoTotals(req model.IssueRequest) (base, paid float64) {
	for _, a := range req.Anticipos {
		base += parseDecimal(a.BaseAmount)
		paid += parseDecimal(a.TotalAmount)
	}
	return base, paid
}

// buildAnticipoAllowances emits one document-level cac:AllowanceCharge per
// applied anticipo, Cat.53 "04", carrying the anticipo's IGV-EXCLUSIVE base as
// cbc:Amount. They are what SUNAT keys on to accept the reduced gravado
// cac:TaxSubtotal (the IGV of the anticipo was already declared by the factura
// de anticipo), and they are mandatory whenever cbc:PrepaidAmount is present:
// omitting them is fault 3287 "Si se informa 'Total de anticipos' debe
// consignar los descuentos globales por anticipo con monto mayor a cero".
//
// Unlike the descuento global they carry no cbc:BaseAmount or
// cbc:MultiplierFactorNumeric — there is nothing to prorate, the anticipo is an
// absolute amount already invoiced.
func buildAnticipoAllowances(req model.IssueRequest) []lineAllowanceCharge {
	if len(req.Anticipos) == 0 {
		return nil
	}
	out := make([]lineAllowanceCharge, 0, len(req.Anticipos))
	for _, a := range req.Anticipos {
		base := parseDecimal(a.BaseAmount)
		if base <= 0 {
			continue
		}
		out = append(out, lineAllowanceCharge{
			ChargeIndicator:       false,
			AllowanceChargeReason: anticipoReasonCode,
			Amount:                newCurrencyAmount(formatDecimal(base), req.CurrencyCode),
		})
	}
	return out
}

// buildAnticipoReferences emits one cac:AdditionalDocumentReference per applied
// anticipo: the anticipo's SERIE-CORRELATIVO, its Cat.12 type (02/03), the
// 1-based row number (cbc:DocumentStatusCode) pairing it with the matching
// cac:PrepaidPayment, and the emitter's RUC. v1 only supports anticipos issued
// by the same supplier, so the RUC is always req.SupplierRUC.
func buildAnticipoReferences(req model.IssueRequest) []additionalDocumentReference {
	if len(req.Anticipos) == 0 {
		return nil
	}
	out := make([]additionalDocumentReference, 0, len(req.Anticipos))
	for i, a := range req.Anticipos {
		out = append(out, additionalDocumentReference{
			ID:                 a.DocID,
			DocumentTypeCode:   a.DocTypeCode,
			DocumentStatusCode: fmt.Sprint(i + 1),
			IssuerParty: &docRefIssuerParty{
				PartyIdentification: partyIdentification{
					ID: schemeID{Value: req.SupplierRUC, SchemeID: "6"},
				},
			},
		})
	}
	return out
}

// buildPrepaidPayments emits one cac:PrepaidPayment per applied anticipo with
// the IGV-inclusive monto anticipado, row-numbered to match the references.
func buildPrepaidPayments(req model.IssueRequest) []prepaidPayment {
	if len(req.Anticipos) == 0 {
		return nil
	}
	out := make([]prepaidPayment, 0, len(req.Anticipos))
	for i, a := range req.Anticipos {
		out = append(out, prepaidPayment{
			ID: schemeID{
				Value:            fmt.Sprint(i + 1),
				SchemeName:       "Anticipo",
				SchemeAgencyName: "PE:SUNAT",
			},
			PaidAmount: newCurrencyAmount(a.TotalAmount, req.CurrencyCode),
		})
	}
	return out
}

// buildDocumentAllowances assembles the document-level cac:AllowanceCharge
// entries: the descuento global (Cat.53 02) and one descuento por anticipo
// (Cat.53 04) per applied anticipo. Both are afecta-base — their amounts reduce
// the gravado base imponible in buildDocumentTaxTotal — and neither ever
// surfaces as cbc:AllowanceTotalAmount (fault 3300), which carries only
// discounts that do NOT affect the base.
//
// They differ in the totales: the descuento global also reduces
// cbc:LineExtensionAmount and cbc:TaxInclusiveAmount (the customer owes less),
// while an anticipo leaves the sale declared in full and is deducted once more,
// as cbc:PrepaidAmount. See buildLegalMonetaryTotal.
func buildDocumentAllowances(req model.IssueRequest) []lineAllowanceCharge {
	var out []lineAllowanceCharge
	if gd := buildGlobalDiscount(req); gd != nil {
		out = append(out, *gd)
	}
	return append(out, buildAnticipoAllowances(req)...)
}

func buildDocumentTaxTotal(req model.IssueRequest) taxTotal {
	cur := req.CurrencyCode
	buckets := sumLineBuckets(req.Items)

	// El descuento global (Cat.53 code 02) reduce la base imponible del IGV: el
	// cac:TaxSubtotal gravado declara BaseAmount − descuento y el IGV se
	// recalcula al 18% sobre la base reducida, o SUNAT rechaza el total de IGV.
	//
	// Los anticipos (Cat.53 code 04) reducen esta misma base, por su importe SIN
	// IGV: la factura de anticipo ya declaró el IGV de lo cobrado, así que la
	// factura de regularización solo tributa el saldo. Las líneas siguen
	// declarando la venta completa — solo este total del documento va neto.
	docRate := documentIGVRate(req.Items)
	anticipoBase, _ := anticipoTotals(req)
	if deduction := globalDiscountValue(req) + anticipoBase; deduction > 0 {
		buckets.regularBase -= deduction
		if buckets.regularBase < 0 {
			buckets.regularBase = 0
		}
		buckets.regularIGV = parseDecimal(formatDecimal(buckets.regularBase * parseDecimal(docRate) / 100))
	}

	// Document-level cbc:TaxAmount: SUNAT expects the sum of taxes declared
	// across all subtotals (IGV + IVAP). Gratuita IGV is informativo and lives
	// inside its own subtotal; including it in the top-level TaxAmount also
	// matches the value emitted for documents without gratuito lines.
	// ISC and otros tributos are tributos the customer pays, so they are part
	// of the total too (3294).
	totalTax := buckets.regularIGV + buckets.ivapIGV + buckets.iscAmount + buckets.otrosAmount
	tt := taxTotal{
		TaxAmount: newCurrencyAmount(formatDecimal(totalTax), cur),
	}

	// IGV (regular gravado) subtotal, at the rate the lines declare
	if buckets.regularBase > 0 || buckets.regularIGV > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.regularBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.regularIGV), cur),
			TaxCategory:   newTaxCategory("S", docRate, sunat.Cat05IGV),
		})
	}

	// IVAP (4%) subtotal
	if buckets.ivapBase > 0 || buckets.ivapIGV > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.ivapBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.ivapIGV), cur),
			TaxCategory:   newTaxCategory("S", sunat.Cat05Rate(sunat.Cat05IVAP), sunat.Cat05IVAP),
		})
	}

	// Exonerado (9997), inafecto (9998) and exportación (9995) subtotals. Each
	// tributo used by a line with a base > 0 needs its own document subtotal
	// (ERROR 2638), carrying the Σ of those line bases and no tax (3000).
	for _, z := range []struct {
		base    float64
		taxCode string
	}{
		{buckets.exoBase, sunat.Cat05Exonerado},
		{buckets.inafBase, sunat.Cat05Inafecto},
		{buckets.expBase, sunat.Cat05Exportacion},
	} {
		if z.base > 0 {
			tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
				TaxableAmount: newCurrencyAmount(formatDecimal(z.base), cur),
				TaxAmount:     newCurrencyAmount("0.00", cur),
				TaxCategory:   newTaxCategory(sunat.Cat05TaxCategoryId(z.taxCode), "", z.taxCode),
			})
		}
	}

	// Gratuita (9996) subtotal — gravado-gratuito lines. The document cbc:TaxAmount
	// is NOT increased (the customer pays no tax on gratuitas, so it stays the sum
	// of onerosa IGV/IVAP), but SUNAT requires the referential base + would-be IGV
	// to be consigned here under the GRA scheme so it reconciles with each line's
	// gratuita TaxSubtotal. Matches greenter's Factura-Gratuita output.
	if buckets.gratBase > 0 || buckets.gratIGV > 0 {
		// The rate tag is always emitted (fault 2992). The gravado gratuitos'
		// rate (18 or 10.5, 3462) when there is would-be IGV (11-16), read from
		// their own base; 0.00 for exonerado/inafecto gratuito
		// (21, 31-37), which carry no IGV — matching their zero TaxAmount so
		// SUNAT's base×rate check still holds.
		gratPercent := "0.00"
		if buckets.gratIGV > 0 {
			gratPercent = snapIGVRate(buckets.gratIGV, buckets.gratGravBase)
		}
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.gratBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.gratIGV), cur),
			TaxCategory:   newTaxCategory(sunat.Cat05TaxCategoryId(sunat.Cat05Gratuita), gratPercent, sunat.Cat05Gratuita),
		})
	}

	// ISC subtotal: the Σ of the ISC lines' bases and amounts, not the whole
	// valor de venta — a plain line next to an ISC line carries no ISC (3296).
	if buckets.iscBase > 0 || buckets.iscAmount > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.iscBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.iscAmount), cur),
			TaxCategory:   newTaxCategory("S", "", sunat.Cat05ISC),
		})
	}

	// Otros tributos subtotal: the Σ of the 9999 lines' bases and amounts, not
	// the whole valor de venta — a line without otros tributos carries none
	// (3297 / 3299). At most one per document.
	if buckets.otrosBase > 0 || buckets.otrosAmount > 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(formatDecimal(buckets.otrosBase), cur),
			TaxAmount:     newCurrencyAmount(formatDecimal(buckets.otrosAmount), cur),
			TaxCategory:   newTaxCategory(sunat.Cat05TaxCategoryId(sunat.Cat05Otros), "", sunat.Cat05Otros),
		})
	}

	// Fallback when no line contributed a base at all: emit a single zero IGV
	// subtotal so SUNAT doesn't reject for missing TaxTotal. Never reached by an
	// exonerado/inafecto/exportación document, which has its own subtotal above
	// (a 1000 subtotal there is ERROR 3277 / 3107).
	if len(tt.TaxSubtotal) == 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount("0.00", cur),
			TaxCategory:   newTaxCategory("S", sunat.Cat05Rate(sunat.Cat05IGV), sunat.Cat05IGV),
		})
	}

	return tt
}

func hasGratuitoLine(items []model.LineItem) bool {
	for _, li := range items {
		if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
			return true
		}
	}
	return false
}

func hasIVAPLine(items []model.LineItem) bool {
	for _, li := range items {
		if sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode) == sunat.Cat05IVAP {
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
	//
	// Anticipos only half work like the descuento global. The Cat.53 "04"
	// allowances DO reduce the base imponible (buildDocumentTaxTotal), but here
	// the sale stays declared IN FULL — LineExtensionAmount and
	// TaxInclusiveAmount are gross — and what was already collected is deducted
	// once, IGV included, as cbc:PrepaidAmount:
	//
	//	PayableAmount = TaxInclusiveAmount − PrepaidAmount
	//
	// So TaxInclusiveAmount is NOT LineExtensionAmount + the document TaxAmount
	// here: it is the full sale (líneas + IGV de las líneas), while the document
	// TaxAmount only covers the untaxed remainder.
	//
	// Both halves are load-bearing, each proven by a SUNAT beta rejection on
	// 2026-07-31: deducting the anticipo from the base AND from the payable
	// total (PayableAmount = reduced TaxInclusiveAmount) is fault 3280 "El
	// importe total del comprobante no coincide con el valor calculado";
	// deducting it only from the payable total, with no Cat.53 "04" allowance,
	// is fault 3287. The shape below is the one SUNAT accepts — a full sale of
	// 1000 + 180 IGV with two anticipos of 118 emits LineExtension 1000,
	// TaxSubtotal 800/144, TaxInclusive 1180, Prepaid 236, Payable 944.
	//
	// req.TaxInclusiveAmount and req.TotalAmount already arrive in that shape
	// from the frontend's sumTotals.
	_, anticipoPaid := anticipoTotals(req)
	lineExtension := req.Subtotal
	if deduction := globalDiscountValue(req); deduction > 0 {
		lineExtension = formatDecimal(parseDecimal(req.Subtotal) - deduction)
	}
	lmt := legalMonetaryTotal{
		LineExtensionAmount: newCurrencyAmount(lineExtension, cur),
		TaxInclusiveAmount:  newCurrencyAmount(req.TaxInclusiveAmount, cur),
		PayableAmount:       newCurrencyAmount(req.TotalAmount, cur),
	}
	if anticipoPaid > 0 {
		prepaid := newCurrencyAmount(formatDecimal(anticipoPaid), cur)
		lmt.PrepaidAmount = &prepaid
	}
	return lmt
}

func buildInvoiceLine(li model.LineItem, cur string) (invoiceLine, error) {
	// SUNAT rule 3271: gratuito lines emit cac:Price/PriceAmount = 0 (the
	// customer is charged nothing). FreeOfChargeIndicator is intentionally NOT
	// emitted (greenter doesn't, and it's not what SUNAT keys on).
	unitPrice := li.UnitPrice
	if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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

// buildLineDelivery emits the cac:Delivery block a detracción de transporte de
// carga (Cat.54 027 → tipo de operación 1004) must carry, or nil for every
// other comprobante.
//
// SUNAT requires, all as ERRORs: the ubigeo + detailed address of both ends of
// the trip (faults 3116-3119), a free-text detalle del viaje (3120), and
// exactly one of each of the three valores referenciales (3122/3124/3125/3126)
// — it computes the SPOT base as the greater of the importe and the valor
// referencial, which is why all three are declared. Amounts are always PEN,
// like the detracción monto itself.
//
// The optional cac:Shipment/cac:Consignment extras (tramo id, per-tramo
// preliminary value, carga útil in TNE) are deliberately not emitted: they are
// OBSERV-only, never ERROR.
func buildLineDelivery(req model.IssueRequest) *lineDelivery {
	t := req.TransporteCarga
	if t == nil {
		return nil
	}
	return &lineDelivery{
		DeliveryLocation: &deliveryLocation{
			Address: newUbigeoAddress(t.DestinoUbigeo, t.DestinoDireccion),
		},
		Despatch: &despatch{
			Instructions:    t.DetalleViaje,
			DespatchAddress: newUbigeoAddress(t.OrigenUbigeo, t.OrigenDireccion),
		},
		DeliveryTerms: []deliveryTerms{
			newValorReferencial("01", t.ValorReferencialServicio),
			newValorReferencial("02", t.ValorReferencialCargaEfectiva),
			newValorReferencial("03", t.ValorReferencialCargaUtil),
		},
	}
}

// buildHidrobiologicosProperties returns the six Cat.55 concepts a line of a
// detracción de recursos hidrobiológicos (Cat.54 004 → tipo de operación 1002)
// must declare, each an ERROR when missing: 3001 matrícula (3063), 3002 nombre
// de la embarcación (3130), 3003 especie (3131), 3004 lugar de descarga (3132),
// 3005 fecha de descarga (3134, in cac:UsabilityPeriod: 3065) and 3006 cantidad
// (3133, as cbc:ValueQuantity in TNE: 3135/3115). SUNAT runs them in the line
// template, so every line carries all six. Missing data is refused here, since
// SUNAT would reject the comprobante anyway; validation reports it field by
// field first.
func buildHidrobiologicosProperties(h *model.Hidrobiologicos, li model.LineItem) ([]itemProperty, error) {
	if h == nil {
		return nil, fmt.Errorf("detracción 004 (1002): faltan los datos de la embarcación y la descarga (hidrobiologicos)")
	}
	if strings.TrimSpace(li.EspecieVendida) == "" || strings.TrimSpace(li.CantidadEspecie) == "" {
		return nil, fmt.Errorf("detracción 004 (1002): la línea %d no declara la especie vendida y su cantidad (hidrobiologicos)", li.LineNumber)
	}
	prop := func(code string) itemProperty {
		e, _ := sunat.Cat55(code)
		return itemProperty{
			Name: e.Label,
			NameCode: itemPropertyCode{
				Value:          code,
				ListName:       "Propiedad del item",
				ListAgencyName: "PE:SUNAT",
				ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo55",
			},
		}
	}
	withValue := func(code, value string) itemProperty {
		p := prop(code)
		p.Value = value
		return p
	}
	fecha := prop(sunat.Cat55FechaDescarga)
	fecha.UsabilityPeriod = &usabilityPeriod{StartDate: h.FechaDescarga}
	cantidad := prop(sunat.Cat55CantidadEspecie)
	cantidad.ValueQuantity = &quantity{Value: formatDecimal(parseDecimal(li.CantidadEspecie)), UnitCode: sunat.Cat03TNE}
	return []itemProperty{
		withValue(sunat.Cat55MatriculaEmbarcacion, h.MatriculaEmbarcacion),
		withValue(sunat.Cat55NombreEmbarcacion, h.NombreEmbarcacion),
		withValue(sunat.Cat55TipoEspecie, li.EspecieVendida),
		withValue(sunat.Cat55LugarDescarga, h.LugarDescarga),
		fecha,
		cantidad,
	}, nil
}

func newUbigeoAddress(ubigeo, direccion string) ubigeoAddress {
	return ubigeoAddress{
		ID: ubigeoID{
			Value:            ubigeo,
			SchemeName:       "Ubigeos",
			SchemeAgencyName: "PE:INEI",
		},
		AddressLine: addressLine{Line: direccion},
	}
}

// newValorReferencial builds one cac:DeliveryTerms entry. The monto is always
// in soles — the valor referencial is a SPOT figure, like the detracción.
func newValorReferencial(tipo, monto string) deliveryTerms {
	return deliveryTerms{
		ID:     tipo,
		Amount: newCurrencyAmount(monto, "PEN"),
	}
}

// buildLineDiscount returns the line-level descuento por ítem (Cat.53 "00")
// when the line carries a non-zero discount, or nil otherwise. Without it
// SUNAT recomputes cbc:LineExtensionAmount as cac:Price/PriceAmount × Quantity
// and rejects the line with fault 3271 because the gross price no longer
// matches the net line total. Gratuito lines have a zero price and never carry
// a discount, so they are skipped.
func buildLineDiscount(li model.LineItem, cur string) *lineAllowanceCharge {
	if isZeroAmount(li.DiscountAmount) || sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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
	baseAmount := newCurrencyAmount(formatDecimal(base), cur)
	return &lineAllowanceCharge{
		ChargeIndicator:         false,
		AllowanceChargeReason:   "00",
		MultiplierFactorNumeric: strconv.FormatFloat(factor, 'f', 5, 64),
		Amount:                  newCurrencyAmount(formatDecimal(discount), cur),
		BaseAmount:              &baseAmount,
	}
}

func buildPricingReference(li model.LineItem, cur string) (*pricingReference, error) {
	// SUNAT requires cac:PricingReference / cac:AlternativeConditionPrice on
	// every line, including gratuito (fault 2028 fires otherwise).
	if !sunat.Cat16Emit(li.PriceTypeCode) {
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
	if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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
	if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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
	if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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
		if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
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
	taxCode := sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode)
	if !sunat.Cat05Valid(taxCode) {
		taxCode = sunat.Cat05IGV
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
	if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
		taxableAmount = formatDecimal(gratuitoReferentialBase(li))
		taxAmount = li.IGVAmount
	} else {
		// ✅ ISC rule: IGV base includes ISC
		if !isZeroAmount(li.ISCAmount) && taxCode == "1000" {
			base := parseDecimal(li.LineTotal) + parseDecimal(li.ISCAmount)
			taxableAmount = formatDecimal(base)
		}
	}

	// The line cbc:TaxAmount is the sum of its tributos: IGV plus ISC (3292).
	lineTax := taxAmount
	hasISC := !isZeroAmount(li.ISCAmount) && !sunat.Cat07Gratuito(li.TaxExemptionReasonCode)
	if hasISC {
		lineTax = formatDecimal(parseDecimal(lineTax) + parseDecimal(li.ISCAmount))
	}
	hasOtros := hasOtrosTributos(li)
	if hasOtros {
		lineTax = formatDecimal(parseDecimal(lineTax) + parseDecimal(li.OtherTaxesAmount))
	}

	lineCat := newLineTaxCategory(li.TaxExemptionReasonCode, taxCode)
	switch {
	case isOnerosaIGVLine(li):
		// The rate the line was actually taxed at, 18 or 10.5 (3103).
		lineCat.Percent = lineIGVRate(li)
	case sunat.Cat07GravadoGratuito(li.TaxExemptionReasonCode):
		// Same for a gravado gratuito's would-be IGV (3103, 3462).
		lineCat.Percent = gratuitoIGVRate(li)
	}

	tt := taxTotal{
		TaxAmount: newCurrencyAmount(lineTax, cur),
		TaxSubtotal: []taxSubtotal{
			{
				TaxableAmount: newCurrencyAmount(taxableAmount, cur),
				TaxAmount:     newCurrencyAmount(taxAmount, cur),
				TaxCategory:   lineCat,
			},
		},
	}

	// ✅ ISC subtotal (only for onerosa)
	if hasISC {
		// The ISC rate is the line's own (ISC ÷ valor de venta), declared like
		// every other line tributo (2992 / 3104).
		iscCat := newTaxCategory("S", iscPercent(li), sunat.Cat05ISC)
		iscCat.TierRange = li.ISCTierRange

		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(li.LineTotal, cur),
			TaxAmount:     newCurrencyAmount(li.ISCAmount, cur),
			TaxCategory:   iscCat,
		})
	}

	// Otros tributos (9999): its own subtotal on the line's valor de venta, at
	// the line's rate (2992, 3109), with no afectación code (3050).
	if hasOtros {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(li.LineTotal, cur),
			TaxAmount:     newCurrencyAmount(li.OtherTaxesAmount, cur),
			TaxCategory: newTaxCategory(sunat.Cat05TaxCategoryId(sunat.Cat05Otros),
				formatDecimal(parseDecimal(li.OtherTaxesRate)), sunat.Cat05Otros),
		})
	}

	return tt
}

// hasOtrosTributos reports whether an onerosa line pays otros tributos (9999).
// Gratuito lines never do: validation refuses it.
func hasOtrosTributos(li model.LineItem) bool {
	return !isZeroAmount(li.OtherTaxesAmount) && !sunat.Cat07Gratuito(li.TaxExemptionReasonCode)
}

// iscPercent is the ISC rate the line was taxed at, ISC ÷ valor de venta, as a
// percentage with two decimals.
func iscPercent(li model.LineItem) string {
	base := parseDecimal(li.LineTotal)
	if base == 0 {
		return "0.00"
	}
	return formatDecimal(parseDecimal(li.ISCAmount) / base * 100)
}

// newTaxCategory builds a cac:TaxCategory for a Cat.05 tributo. The two UN/ECE
// scheme identifiers are different code lists on different elements and are both
// correct as written: 5305 ("duty or tax or fee category code") qualifies the
// category S/E/O/G, 5153 ("duty or tax or fee type name code") qualifies the
// tributo itself.
func newTaxCategory(categoryID, percent, taxCode string) taxCategory {
	tc := taxCategory{
		ID: taxCategoryID{
			Value:          categoryID,
			SchemeID:       "UN/ECE 5305",
			SchemeAgencyID: "6",
		},
		TaxScheme: taxSchemeXML{
			ID: taxSchemeID{
				Value:          taxCode,
				SchemeID:       "UN/ECE 5153",
				SchemeAgencyID: "6",
			},
			Name:        sunat.Cat05Name(taxCode),
			TaxTypeCode: sunat.Cat05TaxTypeCode(taxCode),
		},
	}
	if percent != "" {
		tc.Percent = percent
	}
	return tc
}

func newLineTaxCategory(exemptionReasonCode, taxCode string) taxCategory {
	tc := newTaxCategory(sunat.Cat05TaxCategoryId(taxCode), "", taxCode)

	// cbc:Percent is the rate the line declares, which Cat.07 carries per código:
	// 18.00 for gravado, 4.00 for IVAP, 18.00 for gravado-gratuito (the IGV it
	// would have carried), 0.00 for exonerado/inafecto gratuito. Cat.07 has no
	// rate for exonerado, inafecto and exportación, but SUNAT wants the tag on
	// every line tributo except ICBPER (ERROR 2992), so they declare 0.00 — a
	// value SUNAT's 3102 regex accepts.
	tc.Percent = sunat.Cat07Percent(exemptionReasonCode)
	if tc.Percent == "" && taxCode != sunat.Cat05ICBPER {
		tc.Percent = "0.00"
	}
	if !sunat.Cat07Valid(exemptionReasonCode) {
		// Unknown código: buildLineTaxTotal has already fallen back to IGV, so
		// declare IGV's rate rather than emitting a tributo with no rate.
		tc.Percent = sunat.Cat05Rate(sunat.Cat05IGV)
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
