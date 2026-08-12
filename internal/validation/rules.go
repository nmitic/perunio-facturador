package validation

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

var (
	rucRegex       = regexp.MustCompile(`^\d{11}$`)
	dniRegex       = regexp.MustCompile(`^\d{8}$`)
	facturaIDRegex = regexp.MustCompile(`^[F][A-Z0-9]{3}-\d{1,8}$`)
	boletaIDRegex  = regexp.MustCompile(`^[B][A-Z0-9]{3}-\d{1,8}$`)
	isoDateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// cat07RateFraction converts the cbc:Percent a Cat.07 código declares ("18.00")
// into the fraction the tolerance checks multiply by (0.18). Zero for a código
// that declares no rate.
func cat07RateFraction(code string) float64 {
	pct, err := strconv.ParseFloat(sunat.Cat07Percent(code), 64)
	if err != nil {
		return 0
	}
	return pct / 100
}

func validateHeader(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// DocType must be valid
	switch req.DocType {
	case model.DocTypeFactura, model.DocTypeBoleta, model.DocTypeNotaCredito, model.DocTypeNotaDebito:
		// ok
	default:
		errs = append(errs, model.ValidationError{Code: 1003, Message: fmt.Sprintf("invalid document type: %s", req.DocType), Field: "docType"})
	}

	// Document ID format
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	switch req.DocType {
	case model.DocTypeFactura:
		if !facturaIDRegex.MatchString(docID) {
			errs = append(errs, model.ValidationError{Code: 1001, Message: "factura ID must match F[A-Z0-9]{3}-NNNNNNNN", Field: "series"})
		}
	case model.DocTypeBoleta:
		if !boletaIDRegex.MatchString(docID) {
			errs = append(errs, model.ValidationError{Code: 1001, Message: "boleta ID must match B[A-Z0-9]{3}-NNNNNNNN", Field: "series"})
		}
	}

	// Correlative must be > 0
	if req.Correlative <= 0 {
		errs = append(errs, model.ValidationError{Code: 1036, Message: "correlative must be > 0", Field: "correlative"})
	}

	// IssueDate: cannot be more than 2 days in the future
	if req.IssueDate != "" {
		issueDate, err := time.Parse("2006-01-02", req.IssueDate)
		if err != nil {
			errs = append(errs, model.ValidationError{Code: 2329, Message: "invalid issue date format", Field: "issueDate"})
		} else {
			maxDate := time.Now().AddDate(0, 0, 2)
			if issueDate.After(maxDate) {
				errs = append(errs, model.ValidationError{Code: 2329, Message: "issue date cannot be more than 2 days in the future", Field: "issueDate"})
			}
		}
	} else {
		errs = append(errs, model.ValidationError{Code: 2329, Message: "issue date is required", Field: "issueDate"})
	}

	// CurrencyCode required
	if req.CurrencyCode == "" {
		errs = append(errs, model.ValidationError{Code: 2071, Message: "currency code is required", Field: "currencyCode"})
	}

	return errs
}

func validateSupplier(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	if !rucRegex.MatchString(req.SupplierRUC) {
		errs = append(errs, model.ValidationError{Code: 1007, Message: "supplier RUC must be 11 digits", Field: "supplierRuc"})
	}

	if req.SupplierName == "" {
		errs = append(errs, model.ValidationError{Code: 1037, Message: "supplier name is required", Field: "supplierName"})
	} else if len(req.SupplierName) > 1500 {
		errs = append(errs, model.ValidationError{Code: 1037, Message: "supplier name exceeds 1500 characters", Field: "supplierName"})
	}

	return errs
}

// hasIdentifiedCustomer reports whether the receptor is a real, named party
// rather than the consumidor final placeholder a boleta falls back to
// ("0"/"0"/"CLIENTES VARIOS" — see db.document_mutations).
func hasIdentifiedCustomer(req model.IssueRequest) bool {
	return req.CustomerDocType != "" &&
		req.CustomerDocType != model.IdentityDocTribNoRUC &&
		req.CustomerDocNumber != "" &&
		req.CustomerDocNumber != model.ConsumidorFinalDocNumber
}

func validateCustomer(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// Factura: customer must have RUC (type "6") in most cases
	if req.DocType == model.DocTypeFactura && req.CustomerDocType != "6" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "factura customer must have document type 6 (RUC)", Field: "customerDocType"})
	}

	// RUC validation
	if req.CustomerDocType == "6" && !rucRegex.MatchString(req.CustomerDocNumber) {
		errs = append(errs, model.ValidationError{Code: 2017, Message: "customer RUC must be 11 digits", Field: "customerDocNumber"})
	}

	// DNI validation
	if req.CustomerDocType == "1" && !dniRegex.MatchString(req.CustomerDocNumber) {
		errs = append(errs, model.ValidationError{Code: 2801, Message: "customer DNI must be 8 digits", Field: "customerDocNumber"})
	}

	// Boleta > S/700: a real identity is required. Consumidor final
	// (IdentityDocTribNoRUC) is only valid for boletas <= S/700, so reject it
	// here too — otherwise the consumidor-final default could carry a
	// high-value boleta through.
	if req.DocType == model.DocTypeBoleta {
		total, err := strconv.ParseFloat(req.TotalAmount, 64)
		if err == nil && total > 700.00 {
			if !hasIdentifiedCustomer(req) {
				errs = append(errs, model.ValidationError{Code: 2800, Message: "boleta > S/700 requires customer identity document", Field: "customerDocType"})
			}
		}
	}

	// A boleta sujeta a detracción or carrying anticipos needs a named
	// adquirente regardless of amount. Not a SUNAT fault — the XML puts the
	// *emisor's* RUC in cac:AdditionalDocumentReference/cac:IssuerParty, so
	// SUNAT never sees the gap. It is ours to enforce because both features are
	// meaningless against "CLIENTES VARIOS": the constancia de depósito SPOT is
	// filed under the adquirente, and an anticipo can only be reconciled against
	// the final comprobante if both name the same buyer. The S/700 rule above
	// does not cover it — Cat.54 código 027 has a S/400 umbral and 039 has none
	// at all, so a SPOT boleta can sit well below S/700.
	//
	// A DNI is enough; do not require a RUC (type "6"). Natural persons are the
	// normal receptor of a boleta.
	if req.DocType == model.DocTypeBoleta && !hasIdentifiedCustomer(req) {
		switch {
		case req.Detraccion != nil:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "una boleta sujeta a detracción requiere identificar al adquirente (DNI o RUC), no consumidor final", Field: "customerDocType"})
		case len(req.Anticipos) > 0:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "una boleta que aplica anticipos requiere identificar al adquirente (DNI o RUC), no consumidor final", Field: "customerDocType"})
		case req.OperationType == model.OpAnticipos:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "una boleta de anticipo requiere identificar al adquirente (DNI o RUC), no consumidor final", Field: "customerDocType"})
		}
	}

	if req.CustomerName == "" {
		errs = append(errs, model.ValidationError{Code: 2017, Message: "customer name is required", Field: "customerName"})
	}

	return errs
}

func validateAmounts(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// Verify required totals are present
	for _, field := range []struct {
		name, value string
	}{
		{"subtotal", req.Subtotal},
		{"totalIgv", req.TotalIGV},
		{"totalAmount", req.TotalAmount},
		{"taxInclusiveAmount", req.TaxInclusiveAmount},
	} {
		if field.value == "" {
			errs = append(errs, model.ValidationError{Code: 3021, Message: fmt.Sprintf("%s is required", field.name), Field: field.name})
		}
	}

	return errs
}

func validateLines(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	if len(req.Items) == 0 {
		errs = append(errs, model.ValidationError{Code: 2023, Message: "at least one line item is required", Field: "items"})
		return errs
	}

	lineIDs := make(map[int]bool)
	for _, li := range req.Items {
		// Unique line number
		if lineIDs[li.LineNumber] {
			errs = append(errs, model.ValidationError{Code: 2752, Message: fmt.Sprintf("duplicate line number: %d", li.LineNumber), Field: fmt.Sprintf("items[%d].lineNumber", li.LineNumber)})
		}
		lineIDs[li.LineNumber] = true

		// Line number > 0
		if li.LineNumber <= 0 {
			errs = append(errs, model.ValidationError{Code: 2023, Message: "line number must be > 0", Field: "items.lineNumber"})
		}

		// Quantity > 0
		qty, err := strconv.ParseFloat(li.Quantity, 64)
		if err != nil || qty <= 0 {
			errs = append(errs, model.ValidationError{Code: 2024, Message: fmt.Sprintf("line %d: quantity must be > 0", li.LineNumber), Field: fmt.Sprintf("items[%d].quantity", li.LineNumber)})
		}

		// Description required
		if strings.TrimSpace(li.Description) == "" {
			errs = append(errs, model.ValidationError{Code: 2025, Message: fmt.Sprintf("line %d: description is required", li.LineNumber), Field: fmt.Sprintf("items[%d].description", li.LineNumber)})
		}

		// Tax exemption reason code required, and must be one SUNAT defines.
		// Membership was never checked here: an código outside catálogo 07 used to
		// reach SUNAT and come back as a fault.
		if li.TaxExemptionReasonCode == "" {
			errs = append(errs, model.ValidationError{Code: 3105, Message: fmt.Sprintf("line %d: tax exemption reason code is required", li.LineNumber), Field: fmt.Sprintf("items[%d].taxExemptionReasonCode", li.LineNumber)})
		} else if !sunat.Cat07Valid(li.TaxExemptionReasonCode) {
			errs = append(errs, model.ValidationError{Code: 3105, Message: fmt.Sprintf("line %d: %q is not a catálogo 07 code", li.LineNumber, li.TaxExemptionReasonCode), Field: fmt.Sprintf("items[%d].taxExemptionReasonCode", li.LineNumber)})
		}

		// Tolerance check (±1 cent) on the tax an onerosa gravado line declares.
		// Which tributo applies and at what rate both come from the código, so the
		// checked rate and the emitted cbc:Percent cannot disagree.
		switch sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode) {
		case sunat.Cat05IGV:
			// IGV base = valor_venta + ISC when ISC applies (SUNAT spec).
			errs = append(errs, validateIGVTolerance(li, cat07RateFraction(li.TaxExemptionReasonCode), true)...)
		case sunat.Cat05IVAP:
			// IVAP base = valor_venta only — does not stack on ISC.
			errs = append(errs, validateIGVTolerance(li, cat07RateFraction(li.TaxExemptionReasonCode), false)...)
		}

		// Gratuito-specific rules: priceTypeCode must be '02' (precio
		// referencial), LineTotal must be 0 (the value goes into
		// PricingReference, not LineExtensionAmount).
		if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
			if li.PriceTypeCode != "02" {
				errs = append(errs, model.ValidationError{
					Code:    2010,
					Message: fmt.Sprintf("line %d: priceTypeCode debe ser '02' para operaciones gratuitas (recibido: %q)", li.LineNumber, li.PriceTypeCode),
					Field:   fmt.Sprintf("items[%d].priceTypeCode", li.LineNumber),
				})
			}
			if !isZeroDecimal(li.LineTotal) {
				errs = append(errs, model.ValidationError{
					Code:    2011,
					Message: fmt.Sprintf("line %d: lineTotal debe ser '0.00' para operaciones gratuitas (recibido: %q)", li.LineNumber, li.LineTotal),
					Field:   fmt.Sprintf("items[%d].lineTotal", li.LineNumber),
				})
			}
			// Referential price must be > 0 for any gratuita: it's the basis SUNAT
			// uses for both the informativo IGV and the catalog 16 type-02 price.
			if isZeroDecimal(li.UnitPriceWithTax) {
				errs = append(errs, model.ValidationError{
					Code:    3105,
					Message: fmt.Sprintf("line %d: unitPriceWithTax (precio referencial) debe ser > 0 para operaciones gratuitas", li.LineNumber),
					Field:   fmt.Sprintf("items[%d].unitPriceWithTax", li.LineNumber),
				})
			}
			// Gravado-gratuito (11-16) is still gravado IGV: SUNAT requires a
			// non-zero IGV tribute on the line (rule 3111; surfaces as 3105
			// when the tribute is effectively absent).
			if sunat.Cat07GravadoGratuito(li.TaxExemptionReasonCode) && isZeroDecimal(li.IGVAmount) {
				errs = append(errs, model.ValidationError{
					Code:    3111,
					Message: fmt.Sprintf("line %d: igvAmount debe ser > 0 para gravado-gratuito (cat.07 codes 11-16)", li.LineNumber),
					Field:   fmt.Sprintf("items[%d].igvAmount", li.LineNumber),
				})
			}
		} else if li.PriceTypeCode != "" && li.PriceTypeCode != "01" {
			errs = append(errs, model.ValidationError{
				Code:    2010,
				Message: fmt.Sprintf("line %d: priceTypeCode debe ser '01' para operaciones onerosas (recibido: %q)", li.LineNumber, li.PriceTypeCode),
				Field:   fmt.Sprintf("items[%d].priceTypeCode", li.LineNumber),
			})
		}
	}

	return errs
}

// isZeroDecimal returns true for decimal strings representing zero.
func isZeroDecimal(s string) bool {
	if s == "" {
		return true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return false
	}
	return v == 0
}

func validateIGVTolerance(li model.LineItem, rate float64, includeISCInBase bool) []model.ValidationError {
	var errs []model.ValidationError

	lineTotal, err := strconv.ParseFloat(li.LineTotal, 64)
	if err != nil {
		return errs
	}
	igvAmount, err := strconv.ParseFloat(li.IGVAmount, 64)
	if err != nil {
		return errs
	}
	base := lineTotal
	if includeISCInBase {
		iscAmount, _ := strconv.ParseFloat(li.ISCAmount, 64)
		base += iscAmount
	}

	expectedIGV := base * rate
	if math.Abs(expectedIGV-igvAmount) > 0.01 { // ±0.01 sol tolerance per SUNAT manual §7
		errs = append(errs, model.ValidationError{
			Code:    3103,
			Message: fmt.Sprintf("line %d: IGV amount %.2f exceeds tolerance (expected ~%.2f at %.0f%%)", li.LineNumber, igvAmount, expectedIGV, rate*100),
			Field:   fmt.Sprintf("items[%d].igvAmount", li.LineNumber),
		})
	}

	return errs
}

// validateGlobalDiscount guards the descuento global (Cat.53 code 02). It
// applies to facturas/boletas and to notas de crédito (motivo 04, the discount
// is credited as a document-level cac:AllowanceCharge over the referenced
// lines), and may not exceed the gravado base imponible it reduces — a larger
// discount would drive the IGV base negative (SUNAT 3271/3206).
func validateGlobalDiscount(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	gd, _ := strconv.ParseFloat(req.GlobalDiscount, 64)
	if gd <= 0 {
		return errs
	}
	if req.DocType != model.DocTypeFactura && req.DocType != model.DocTypeBoleta && req.DocType != model.DocTypeNotaCredito {
		return append(errs, model.ValidationError{Code: 2800, Message: "descuento global solo aplica a factura/boleta/nota de crédito", Field: "globalDiscount"})
	}
	var gravado float64
	for _, li := range req.Items {
		if sunat.Cat07Gratuito(li.TaxExemptionReasonCode) {
			continue
		}
		switch sunat.Cat07TaxSchemeCode(li.TaxExemptionReasonCode) {
		case "1000", "1016":
			v, _ := strconv.ParseFloat(li.LineTotal, 64)
			gravado += v
		}
	}
	if gd > gravado+0.01 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "descuento global no puede exceder la base gravada", Field: "globalDiscount"})
	}
	return errs
}

// validateAnticipos guards the anticipos a comprobante de regularización
// applies. Scope: facturas and boletas, and gravado-only anticipos — each
// BaseAmount must be its TotalAmount net of 18% IGV. Their totals together may
// not exceed the total of the comprobante.
//
// Boletas are in scope because SUNAT's current validation pack
// (resources/AjustesValidacionesCPEv20260212.xlsx, sheet Boleta2_0) carries the
// full anticipo rule set — cac:PrepaidPayment, the cac:AdditionalDocumentReference
// pairing and cbc:PrepaidAmount, fault 3287 included — identical to Factura2_0.
// Catálogo 51 in that same workbook marks the detracción operation types
// "Factura, Boletas", and Cat.12 code 03 is literally "Boleta de Venta – emitida
// por anticipos". Notas remain out: they regularize nothing.
//
// Detracción is allowed alongside: the SPOT obligation arises per payment and
// is computed on the monto of the comprobante documenting it, so this
// comprobante's detracción sits on its PayableAmount (the saldo, net of the
// anticipos) while each anticipo already declared SPOT on what it collected.
// validateDetraccion enforces that base.
func validateAnticipos(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	if len(req.Anticipos) == 0 {
		return errs
	}
	switch req.DocType {
	case model.DocTypeFactura, model.DocTypeBoleta:
	default:
		return append(errs, model.ValidationError{Code: 2800, Message: "anticipos solo aplican a facturas y boletas", Field: "anticipos"})
	}
	if req.OperationType == model.OpAnticipos {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "una factura de anticipo (0104) no puede aplicar anticipos", Field: "operationType"})
	}

	var totalSum float64
	for i, a := range req.Anticipos {
		field := fmt.Sprintf("anticipos[%d]", i)
		switch a.DocTypeCode {
		case "02", "03":
		default:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "docTypeCode de anticipo debe ser 02 (factura) o 03 (boleta)", Field: field + ".docTypeCode"})
		}
		total, _ := strconv.ParseFloat(a.TotalAmount, 64)
		base, _ := strconv.ParseFloat(a.BaseAmount, 64)
		if total <= 0 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "monto de anticipo debe ser mayor a 0", Field: field + ".totalAmount"})
			continue
		}
		if base <= 0 || base >= total {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "base de anticipo debe ser mayor a 0 y menor al total con IGV", Field: field + ".baseAmount"})
			continue
		}
		// Gravado-only in v1: total must be base + 18% IGV (±2 cents rounding).
		if diff := base*1.18 - total; diff > 0.02 || diff < -0.02 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("anticipo %s: total %.2f no equivale a base %.2f más IGV 18%%", a.DocID, total, base), Field: field + ".baseAmount"})
		}
		totalSum += total
	}

	// Los anticipos se deducen del total de la venta (cbc:PrepaidAmount), no de
	// la base imponible, así que el límite es ese total: pasarse dejaría un
	// PayableAmount negativo. Ver buildLegalMonetaryTotal.
	taxInclusive, _ := strconv.ParseFloat(req.TaxInclusiveAmount, 64)
	if totalSum > taxInclusive+0.01 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "la suma de anticipos no puede exceder el total del comprobante", Field: "anticipos"})
	}
	return errs
}

// validateDetraccion guards the detracción (SPOT). It is nil for the vast
// majority of documents. When present it must sit on a factura or a boleta, or
// on a nota de crédito/débito that references a factura. It requires
// operationType 1001/1002 — or 0104, the anticipo marker, which xmlbuilder maps
// onto 1001/1002 on the wire — a resolved cuenta BN, and, for PEN documents, a
// monto within ±1 cent of porcentaje × TotalAmount.
//
// Boletas are in scope: SUNAT's current validation pack
// (resources/AjustesValidacionesCPEv20260212.xlsx) marks catálogo 51 codes
// 1001-1004 as "Factura, Boletas" in its Catálogos sheet, and the Boleta2_0
// sheet carries the same detracción rules as Factura2_0 — cac:PaymentTerms and
// cac:PaymentMeans both keyed "Detraccion", leyenda 2006, faults
// 3127/3128/3034/3035/3208. The 2018 boleta XML guide PDF omits those elements,
// but it is stale: it also omits forma de pago / cuotas, which boletas do carry.
//
// A boleta sujeta a detracción additionally needs an identified adquirente —
// see validateCustomer, which is where that is enforced.
//
// Caveat on notas: in that same validation pack, NotaDebito2_0 carries the
// detracción rule set but NotaCredito2_0 carries none at all. So a nota de
// débito sujeta a detracción is validated markup, while a nota de crédito's is
// simply unvalidated — SUNAT neither requires nor checks it there. We keep
// emitting it on both (a NC that corrects a SPOT operation should mirror it),
// but only the ND half has a rule behind it. Verify each against beta.
//
// TotalAmount is the document's cbc:PayableAmount, and that is deliberate: the
// SPOT base is the importe of the comprobante that documents the payment. On a
// factura de regularización the payable is already net of the anticipos, so the
// detracción covers only the saldo being collected — each anticipo declared its
// own share when it was issued, and the deposits sum to porcentaje × operación.
func validateDetraccion(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	d := req.Detraccion
	if d == nil {
		return errs
	}

	// Document scope: factura or boleta, or a nota referencing either. A nota
	// mirrors the detracción of the operation it corrects, so it follows
	// whatever that comprobante could carry.
	switch req.DocType {
	case model.DocTypeFactura, model.DocTypeBoleta:
	case model.DocTypeNotaCredito, model.DocTypeNotaDebito:
		switch req.ReferenceDocType {
		case model.DocTypeFactura, model.DocTypeBoleta:
		default:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a notas que referencian una factura o boleta", Field: "detraccion"})
		}
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a facturas y boletas", Field: "detraccion"})
	}

	switch req.OperationType {
	case model.OpDetraccion, model.OpDetraccionHidrobiologicos, model.OpDetraccionPasajeros,
		model.OpDetraccionTransporteCarga, model.OpAnticipos:
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "operationType debe ser 1001/1002/1003/1004 (o 0104 para un comprobante de anticipo) en una operación sujeta a detracción", Field: "operationType"})
	}
	if strings.TrimSpace(d.Codigo) == "" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "código de detracción requerido (Cat.54)", Field: "detraccion.codigo"})
	}

	// Two Cat.54 códigos declare an operación this pipeline cannot fully build,
	// so refuse them here with something the user can act on instead of letting
	// SUNAT reject the emission with a cryptic fault.
	//
	//   004 → 1002: SUNAT demands three per-line cac:AdditionalItemProperty
	//        entries (Cat.55 3001 matrícula de embarcación, 3006 cantidad de
	//        especies, 3005 fecha de descarga — faults 3063/3133/3134), and the
	//        builder has no AdditionalItemProperty support at all.
	//   028 → 1003: transporte público de pasajeros is not a percentage of the
	//        importe; it is a fixed amount per vehicle, which the monto
	//        arithmetic below (porcentaje × total) cannot express.
	//
	// The código → operación mapping for both is nonetheless correct in
	// xmlbuilder.detraccionOperationType, so lifting either block is only a
	// matter of building the missing markup.
	switch d.Codigo {
	case model.DetraccionHidrobiologicos:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "la detracción de recursos hidrobiológicos (004) aún no está soportada: requiere matrícula de embarcación, especies y fecha de descarga por ítem", Field: "detraccion.codigo"})
	case model.DetraccionTransportePasaj:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "la detracción de transporte de pasajeros (028) aún no está soportada: se liquida por monto fijo por vehículo, no como porcentaje del importe", Field: "detraccion.codigo"})
	case model.DetraccionTransporteCarga:
		errs = append(errs, validateTransporteCarga(req)...)
	}
	if strings.TrimSpace(d.CuentaBN) == "" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "configure la cuenta de detracción de la empresa o especifíquela en el comprobante", Field: "detraccion.cuentaBN"})
	}

	monto, _ := strconv.ParseFloat(d.Monto, 64)
	if monto <= 0 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "monto de detracción debe ser mayor a 0", Field: "detraccion.monto"})
	}

	// The monto is in soles while the document is not, so without a tipo de
	// cambio the neto pendiente de pago cannot be computed — and that figure is
	// what the cuotas must add up to and what the printed comprobante states as
	// "neto a pagar". Required rather than assumed.
	if req.CurrencyCode != "" && req.CurrencyCode != "PEN" {
		if rate, err := strconv.ParseFloat(req.ExchangeRate, 64); err != nil || rate <= 0 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "indique el tipo de cambio: la detracción se declara en soles y el comprobante está en " + req.CurrencyCode, Field: "exchangeRate"})
		}
	}

	// Arithmetic check only for PEN — detracción monto is always in soles, so a
	// non-PEN document declares a converted amount we can't reconcile here
	// (handled upstream). Guard against false negatives by skipping it.
	total, _ := strconv.ParseFloat(req.TotalAmount, 64)
	if req.CurrencyCode == "PEN" {
		pct, _ := strconv.ParseFloat(d.Porcentaje, 64)
		expected := total * pct / 100
		if diff := monto - expected; diff > 0.01 || diff < -0.01 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("monto de detracción %.2f no coincide con %s%% de %.2f", monto, d.Porcentaje, total), Field: "detraccion.monto"})
		}

		// The deposit comes out of what this comprobante collects, so it can
		// never exceed it. Unreachable while the porcentaje check above holds
		// (no Cat.54 rate is above 100%), but it is the failure mode of getting
		// the base wrong — e.g. computing SPOT over the whole operación on a
		// factura de regularización whose payable is only the saldo.
		if monto > total+0.01 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("monto de detracción %.2f excede el total a pagar del comprobante %.2f", monto, total), Field: "detraccion.monto"})
		}
	}

	return errs
}

// validateTransporteCarga guards the extra block a detracción de transporte de
// carga (Cat.54 027 → tipo de operación 1004) must carry. SUNAT treats every
// one of these as an ERROR, not an observation, so a missing field is a
// rejected emission: ubigeo + dirección of both ends of the trip (3116-3119),
// the detalle del viaje (3120), and the three valores referenciales
// (3122/3124/3125/3126). Rejecting here turns those into something the user can
// fix in the form.
func validateTransporteCarga(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	t := req.TransporteCarga
	if t == nil {
		return append(errs, model.ValidationError{Code: 2800, Message: "el transporte de carga (027) requiere los datos del viaje: origen, destino, detalle y valores referenciales", Field: "transporteCarga"})
	}

	required := []struct {
		value, field, label string
	}{
		{t.OrigenUbigeo, "transporteCarga.origenUbigeo", "el ubigeo del punto de origen"},
		{t.OrigenDireccion, "transporteCarga.origenDireccion", "la dirección del punto de origen"},
		{t.DestinoUbigeo, "transporteCarga.destinoUbigeo", "el ubigeo del punto de destino"},
		{t.DestinoDireccion, "transporteCarga.destinoDireccion", "la dirección del punto de destino"},
		{t.DetalleViaje, "transporteCarga.detalleViaje", "el detalle del viaje"},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "indique " + r.label, Field: r.field})
		}
	}
	for _, u := range []struct{ value, field string }{
		{t.OrigenUbigeo, "transporteCarga.origenUbigeo"},
		{t.DestinoUbigeo, "transporteCarga.destinoUbigeo"},
	} {
		if v := strings.TrimSpace(u.value); v != "" && !ubigeoRegex.MatchString(v) {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "el ubigeo debe tener 6 dígitos (catálogo 13)", Field: u.field})
		}
	}
	// SUNAT wants all three declared and greater than zero: the SPOT base is the
	// greater of the importe and the valor referencial.
	for _, v := range []struct{ value, field, label string }{
		{t.ValorReferencialServicio, "transporteCarga.valorReferencialServicio", "del servicio de transporte"},
		{t.ValorReferencialCargaEfectiva, "transporteCarga.valorReferencialCargaEfectiva", "sobre la carga efectiva"},
		{t.ValorReferencialCargaUtil, "transporteCarga.valorReferencialCargaUtil", "sobre la carga útil nominal"},
	} {
		amount, err := strconv.ParseFloat(strings.TrimSpace(v.value), 64)
		if err != nil || amount <= 0 {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "indique el valor referencial " + v.label + " (mayor a 0)", Field: v.field})
		}
	}
	return errs
}

func validateCreditNote(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// Reason code must be valid Cat.09
	if !model.ValidNCType(req.ReasonCode) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("invalid NC reason code: %s", req.ReasonCode), Field: "reasonCode"})
	}

	// NC on boletas: codes 04, 05, 08 not allowed
	if req.ReferenceDocType == model.DocTypeBoleta && model.NCTypesNotAllowedOnBoleta[req.ReasonCode] {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("NC reason code %s not allowed for boletas", req.ReasonCode), Field: "reasonCode"})
	}

	// Reference document required
	if req.ReferenceDocSeries == "" || req.ReferenceDocCorrelative <= 0 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "reference document is required for NC", Field: "referenceDocSeries"})
	}

	return errs
}

// validatePaymentTerms enforces SUNAT rules around forma de pago.
// SUNAT err 3244 fires when missing entirely; for credit sales the cuotas must
// add up (within ±1 cent) to the monto neto pendiente de pago — the payable, less
// the detracción when the operation is subject to SPOT.
func validatePaymentTerms(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	fp := strings.ToLower(strings.TrimSpace(req.FormaPago))
	if fp == "" {
		fp = "contado"
	}
	if fp != "contado" && fp != "credito" {
		errs = append(errs, model.ValidationError{Code: 3244, Message: fmt.Sprintf("formaPago inválido: %q (use 'contado' o 'credito')", req.FormaPago), Field: "formaPago"})
		return errs
	}
	if fp == "contado" {
		if len(req.Cuotas) > 0 {
			errs = append(errs, model.ValidationError{Code: 3244, Message: "no debe enviar cuotas cuando formaPago es 'contado'", Field: "cuotas"})
		}
		return errs
	}

	// Credito
	if len(req.Cuotas) == 0 {
		errs = append(errs, model.ValidationError{Code: 3244, Message: "debe consignar al menos una cuota cuando formaPago es 'credito'", Field: "cuotas"})
		return errs
	}
	issueDate, _ := time.Parse("2006-01-02", req.IssueDate)
	var sum float64
	for i, c := range req.Cuotas {
		if c.Numero <= 0 {
			errs = append(errs, model.ValidationError{Code: 3244, Message: fmt.Sprintf("cuota %d: numero inválido", i+1), Field: fmt.Sprintf("cuotas[%d].numero", i)})
		}
		if !isoDateRegex.MatchString(c.FechaVencimiento) {
			errs = append(errs, model.ValidationError{Code: 3244, Message: fmt.Sprintf("cuota %d: fechaVencimiento debe ser YYYY-MM-DD", i+1), Field: fmt.Sprintf("cuotas[%d].fechaVencimiento", i)})
		} else if due, err := time.Parse("2006-01-02", c.FechaVencimiento); err != nil {
			errs = append(errs, model.ValidationError{Code: 3244, Message: fmt.Sprintf("cuota %d: fechaVencimiento inválida", i+1), Field: fmt.Sprintf("cuotas[%d].fechaVencimiento", i)})
		} else if !issueDate.IsZero() && !due.After(issueDate) {
			errs = append(errs, model.ValidationError{Code: 3267, Message: fmt.Sprintf("cuota %d: fechaVencimiento (%s) debe ser posterior a la fecha de emisión (%s)", i+1, c.FechaVencimiento, req.IssueDate), Field: fmt.Sprintf("cuotas[%d].fechaVencimiento", i)})
		}
		amt, err := strconv.ParseFloat(c.Monto, 64)
		if err != nil || amt <= 0 {
			errs = append(errs, model.ValidationError{Code: 3244, Message: fmt.Sprintf("cuota %d: monto inválido (%q)", i+1, c.Monto), Field: fmt.Sprintf("cuotas[%d].monto", i)})
			continue
		}
		sum += amt
	}
	// The cuotas settle the monto neto pendiente de pago, which on a comprobante
	// sujeto a detracción is the payable MINUS the detracción: the receptor
	// deposits that part straight into the emisor's restricted BN cuenta, on its
	// own legal deadline, so it is never owed on credit and never falls due on a
	// cuota. Without detracción this is just totalAmount and the check is
	// unchanged. See model.NetoPendientePago.
	if _, err := strconv.ParseFloat(req.TotalAmount, 64); err == nil {
		neto := req.NetoPendientePago()
		if math.Abs(sum-neto) > 0.01 {
			label := "totalAmount"
			if req.Detraccion != nil {
				label = "el neto pendiente de pago (total menos detracción)"
			}
			errs = append(errs, model.ValidationError{
				Code:    3244,
				Message: fmt.Sprintf("la suma de cuotas (%.2f) no coincide con %s (%.2f)", sum, label, neto),
				Field:   "cuotas",
			})
		}
	}
	return errs
}

func validateDebitNote(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// Only codes 01 and 02 are valid for ND
	if !model.ValidNDType(req.ReasonCode) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("invalid ND reason code: %s (only 01, 02 allowed)", req.ReasonCode), Field: "reasonCode"})
	}

	// Reference document required
	if req.ReferenceDocSeries == "" || req.ReferenceDocCorrelative <= 0 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "reference document is required for ND", Field: "referenceDocSeries"})
	}

	return errs
}
