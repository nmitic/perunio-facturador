package validation

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/perunio/perunio-facturador/internal/model"
)

var (
	rucRegex       = regexp.MustCompile(`^\d{11}$`)
	dniRegex       = regexp.MustCompile(`^\d{8}$`)
	facturaIDRegex = regexp.MustCompile(`^[F][A-Z0-9]{3}-\d{1,8}$`)
	boletaIDRegex  = regexp.MustCompile(`^[B][A-Z0-9]{3}-\d{1,8}$`)
	isoDateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

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
			if req.CustomerDocType == "" || req.CustomerDocType == model.IdentityDocTribNoRUC || req.CustomerDocNumber == "" {
				errs = append(errs, model.ValidationError{Code: 2800, Message: "boleta > S/700 requires customer identity document", Field: "customerDocType"})
			}
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

		// Tax exemption reason code required
		if li.TaxExemptionReasonCode == "" {
			errs = append(errs, model.ValidationError{Code: 3105, Message: fmt.Sprintf("line %d: tax exemption reason code is required", li.LineNumber), Field: fmt.Sprintf("items[%d].taxExemptionReasonCode", li.LineNumber)})
		}

		// IGV tolerance check (±1 cent) for gravado onerosa lines.
		// IGV base = valor_venta + ISC when ISC applies (SUNAT spec).
		if li.TaxExemptionReasonCode == "10" {
			errs = append(errs, validateIGVTolerance(li, 0.18, true)...)
		}

		// IVAP tolerance check (±1 cent) at 4 % for arroz pilado lines.
		// IVAP base = valor_venta only — does not stack on ISC.
		if li.TaxExemptionReasonCode == "17" {
			errs = append(errs, validateIGVTolerance(li, 0.04, false)...)
		}

		// Gratuito-specific rules: priceTypeCode must be '02' (precio
		// referencial), LineTotal must be 0 (the value goes into
		// PricingReference, not LineExtensionAmount).
		if isGratuitoLineCode(li.TaxExemptionReasonCode) {
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
			if isGravadoGratuitoLineCode(li.TaxExemptionReasonCode) && isZeroDecimal(li.IGVAmount) {
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

// isGravadoGratuitoLineCode reports whether a Cat.07 affectation code is
// gravado-gratuito (11-16): IGV-subject but free of charge.
func isGravadoGratuitoLineCode(code string) bool {
	switch code {
	case "11", "12", "13", "14", "15", "16":
		return true
	}
	return false
}

// isGratuitoLineCode reports whether a Cat.07 affectation code marks the line
// as a transferencia/retiro a título gratuito.
func isGratuitoLineCode(code string) bool {
	switch code {
	case "11", "12", "13", "14", "15", "16",
		"21",
		"31", "32", "33", "34", "35", "36", "37":
		return true
	}
	return false
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
		if isGratuitoLineCode(li.TaxExemptionReasonCode) {
			continue
		}
		switch model.TaxCodeForAffectation(li.TaxExemptionReasonCode) {
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

// validateAnticipos guards the anticipos a factura de regularización applies.
// Scope: facturas only, and gravado-only anticipos — each BaseAmount must be
// its TotalAmount net of 18% IGV. Their totals together may not exceed the
// total of the comprobante.
//
// Detracción is allowed alongside: the SPOT obligation arises per payment and
// is computed on the monto of the comprobante documenting it, so this factura's
// detracción sits on its PayableAmount (the saldo, net of the anticipos) while
// each anticipo already declared SPOT on what it collected. validateDetraccion
// enforces that base.
func validateAnticipos(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	if len(req.Anticipos) == 0 {
		return errs
	}
	if req.DocType != model.DocTypeFactura {
		return append(errs, model.ValidationError{Code: 2800, Message: "anticipos solo aplican a facturas", Field: "anticipos"})
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
// majority of documents. When present it must sit on a factura, or on a nota de
// crédito/débito that references a factura (SUNAT allows detracción only on
// operations that a factura documents — never on boletas). It requires
// operationType 1001/1002 — or 0104, the anticipo marker, which xmlbuilder maps
// onto 1001/1002 on the wire — a resolved cuenta BN, and, for PEN documents, a
// monto within ±1 cent of porcentaje × TotalAmount.
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

	// Document scope: factura, or nota referencing a factura.
	switch req.DocType {
	case model.DocTypeFactura:
	case model.DocTypeNotaCredito, model.DocTypeNotaDebito:
		if req.ReferenceDocType != model.DocTypeFactura {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a notas que referencian una factura", Field: "detraccion"})
		}
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a facturas (no boletas)", Field: "detraccion"})
	}

	switch req.OperationType {
	case model.OpDetraccion, model.OpDetraccionTransporte, model.OpAnticipos:
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "operationType debe ser 1001/1002 (o 0104 para una factura de anticipo) en una operación sujeta a detracción", Field: "operationType"})
	}
	if strings.TrimSpace(d.Codigo) == "" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "código de detracción requerido (Cat.54)", Field: "detraccion.codigo"})
	}
	if strings.TrimSpace(d.CuentaBN) == "" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "configure la cuenta de detracción de la empresa o especifíquela en el comprobante", Field: "detraccion.cuentaBN"})
	}

	monto, _ := strconv.ParseFloat(d.Monto, 64)
	if monto <= 0 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "monto de detracción debe ser mayor a 0", Field: "detraccion.monto"})
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
// add up (within ±1 cent) to the total payable amount.
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
	total, err := strconv.ParseFloat(req.TotalAmount, 64)
	if err == nil && math.Abs(sum-total) > 0.01 {
		errs = append(errs, model.ValidationError{
			Code:    3244,
			Message: fmt.Sprintf("la suma de cuotas (%.2f) no coincide con totalAmount (%.2f)", sum, total),
			Field:   "cuotas",
		})
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
