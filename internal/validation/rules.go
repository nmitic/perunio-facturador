package validation

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// cat06Patterns compiles, once, the número format each Cat.06 código declares.
// A código with no declared pattern is absent — SUNAT documents no length rule
// for it, so there is nothing to check.
var cat06Patterns = func() map[string]*regexp.Regexp {
	m := make(map[string]*regexp.Regexp)
	for _, code := range sunat.Cat06Codes {
		if p := sunat.Cat06NumberPattern(code); p != "" {
			m[code] = regexp.MustCompile(p)
		}
	}
	return m
}()

// cat06NumberMatches reports whether a documento de identidad número matches the
// format its código declares. True when the código declares no format.
func cat06NumberMatches(code, number string) bool {
	re, ok := cat06Patterns[code]
	return !ok || re.MatchString(number)
}

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
	case sunat.Cat01Factura, sunat.Cat01Boleta, sunat.Cat01NotaCredito, sunat.Cat01NotaDebito:
		// ok
	default:
		errs = append(errs, model.ValidationError{Code: 1003, Message: fmt.Sprintf("invalid document type: %s", req.DocType), Field: "docType"})
	}

	// Document ID format
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	switch req.DocType {
	case sunat.Cat01Factura:
		if !facturaIDRegex.MatchString(docID) {
			errs = append(errs, model.ValidationError{Code: 1001, Message: "factura ID must match F[A-Z0-9]{3}-NNNNNNNN", Field: "series"})
		}
	case sunat.Cat01Boleta:
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

	// CurrencyCode required, and must be one catálogo 02 defines. Membership was
	// never checked: any three letters reached SUNAT as the documentCurrencyCode.
	if req.CurrencyCode == "" {
		errs = append(errs, model.ValidationError{Code: 2071, Message: "currency code is required", Field: "currencyCode"})
	} else if !sunat.Cat02Valid(req.CurrencyCode) {
		errs = append(errs, model.ValidationError{Code: 2071, Message: fmt.Sprintf("%q is not a catálogo 02 currency", req.CurrencyCode), Field: "currencyCode"})
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
		req.CustomerDocType != sunat.Cat06DocTribNoDomSinRUC &&
		req.CustomerDocNumber != "" &&
		req.CustomerDocNumber != model.ConsumidorFinalDocNumber
}

func validateCustomer(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	// Document type must be one SUNAT defines. Never checked before: an invented
	// type reached SUNAT and came back as a fault.
	if req.CustomerDocType != "" && !sunat.Cat06Valid(req.CustomerDocType) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("%q is not a catálogo 06 document type", req.CustomerDocType), Field: "customerDocType"})
	}

	// Factura: customer must have RUC (type "6") in most cases
	if req.DocType == sunat.Cat01Factura && req.CustomerDocType != sunat.Cat06RUC {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "factura customer must have document type 6 (RUC)", Field: "customerDocType"})
	}

	// RUC and DNI número formats. The patterns come from the catálogo, so the
	// rule the facturador enforces and the rule the backend's cliente form
	// enforces are one string.
	if req.CustomerDocType == sunat.Cat06RUC && !cat06NumberMatches(sunat.Cat06RUC, req.CustomerDocNumber) {
		errs = append(errs, model.ValidationError{Code: 2017, Message: "customer RUC must be 11 digits", Field: "customerDocNumber"})
	}
	if req.CustomerDocType == sunat.Cat06DNI && !cat06NumberMatches(sunat.Cat06DNI, req.CustomerDocNumber) {
		errs = append(errs, model.ValidationError{Code: 2801, Message: "customer DNI must be 8 digits", Field: "customerDocNumber"})
	}

	// Boleta > S/700: a real identity is required. Consumidor final
	// (IdentityDocTribNoRUC) is only valid for boletas <= S/700, so reject it
	// here too — otherwise the consumidor-final default could carry a
	// high-value boleta through.
	if req.DocType == sunat.Cat01Boleta {
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
	// does not cover it — the venta de inmueble (040) applies at any amount and
	// carries no boleta exclusion, so a SPOT boleta can sit well below S/700.
	//
	// A DNI is enough; do not require a RUC (type "6"). Natural persons are the
	// normal receptor of a boleta.
	if req.DocType == sunat.Cat01Boleta && !hasIdentifiedCustomer(req) {
		switch {
		case req.Detraccion != nil:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "una boleta sujeta a detracción requiere identificar al adquirente (DNI o RUC), no consumidor final", Field: "customerDocType"})
		case len(req.Anticipos) > 0:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "una boleta que aplica anticipos requiere identificar al adquirente (DNI o RUC), no consumidor final", Field: "customerDocType"})
		case req.OperationType == sunat.Cat51Anticipos:
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

		// Unidad de medida must be a real UN/ECE Rec. 20 code. This service used to
		// accept any string and let SUNAT reject the typo.
		if li.UnitCode != "" && !sunat.Cat03Valid(li.UnitCode) {
			errs = append(errs, model.ValidationError{Code: 2026, Message: fmt.Sprintf("line %d: %q is not a catálogo 03 unidad de medida", li.LineNumber, li.UnitCode), Field: fmt.Sprintf("items[%d].unitCode", li.LineNumber)})
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
	if req.DocType != sunat.Cat01Factura && req.DocType != sunat.Cat01Boleta && req.DocType != sunat.Cat01NotaCredito {
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
// (resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx, sheet Boleta2_0) carries the
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
	case sunat.Cat01Factura, sunat.Cat01Boleta:
	default:
		return append(errs, model.ValidationError{Code: 2800, Message: "anticipos solo aplican a facturas y boletas", Field: "anticipos"})
	}
	if req.OperationType == sunat.Cat51Anticipos {
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
// on a nota de crédito/débito that references one. It requires operationType
// 1001-1004 — or 0104, the anticipo marker, which xmlbuilder maps onto 1001-1004
// on the wire — a resolved cuenta BN, and a monto within ±1 cent of
// porcentaje × the SPOT base, in soles.
//
// The código is checked against Cat.54 (validateDetraccionCatalogo): it must be
// emittable, carry its catálogo rate, belong to an anexo the comprobante can
// carry, and the operación must exceed its umbral. DETRACCIONES.md has every
// rule with its source.
//
// Boletas are in scope: SUNAT's current validation pack
// (resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx) marks catálogo 51 codes
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
	case sunat.Cat01Factura, sunat.Cat01Boleta:
	case sunat.Cat01NotaCredito, sunat.Cat01NotaDebito:
		switch req.ReferenceDocType {
		case sunat.Cat01Factura, sunat.Cat01Boleta:
		default:
			errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a notas que referencian una factura o boleta", Field: "detraccion"})
		}
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "detracción solo aplica a facturas y boletas", Field: "detraccion"})
	}

	switch req.OperationType {
	case sunat.Cat51DetraccionGeneral, sunat.Cat51DetraccionHidrobiologicos, sunat.Cat51DetraccionPasajeros,
		sunat.Cat51DetraccionTransporteCarga, sunat.Cat51Anticipos:
	default:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "operationType debe ser 1001/1002/1003/1004 (o 0104 para un comprobante de anticipo) en una operación sujeta a detracción", Field: "operationType"})
	}
	if strings.TrimSpace(d.Codigo) == "" {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "código de detracción requerido (Cat.54)", Field: "detraccion.codigo"})
	}

	// 028 → 1003 declares an operación this pipeline cannot build, so it is
	// refused here with something the user can act on instead of letting SUNAT
	// reject the emission with a cryptic fault: transporte público de pasajeros
	// is not a percentage of the importe but a fixed amount per vehicle, which
	// the monto arithmetic below (porcentaje × total) cannot express. The
	// código → operación mapping is nonetheless correct in
	// xmlbuilder.detraccionOperationType.
	//
	// 004 → 1002 and 027 → 1004 each carry a block of their own data, checked
	// field by field.
	switch d.Codigo {
	case sunat.Cat54RecursosHidrobiologicos:
		errs = append(errs, validateHidrobiologicos(req)...)
		errs = append(errs, validateDetraccionCatalogo(req)...)
	case sunat.Cat54TransporteDePasajeros:
		errs = append(errs, model.ValidationError{Code: 2800, Message: "la detracción de transporte de pasajeros (028) aún no está soportada: se liquida por monto fijo por vehículo, no como porcentaje del importe", Field: "detraccion.codigo"})
	case sunat.Cat54ServicioDeTransporteDeCarga:
		errs = append(errs, validateTransporteCarga(req)...)
		errs = append(errs, validateDetraccionCatalogo(req)...)
	case "":
	default:
		errs = append(errs, validateDetraccionCatalogo(req)...)
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

	// The monto is always in soles. A foreign-currency document is converted
	// with its tipo de cambio first; without one (already reported above) the
	// arithmetic cannot be reconciled, so it is skipped rather than guessed.
	rate, ok := penRate(req)
	if !ok {
		return errs
	}
	total, _ := strconv.ParseFloat(req.TotalAmount, 64)
	penTotal := total * rate
	base := spotBase(req, penTotal)
	pct, _ := strconv.ParseFloat(d.Porcentaje, 64)
	expected := base * pct / 100
	if diff := monto - expected; diff > 0.01 || diff < -0.01 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("monto de detracción %.2f no coincide con %s%% de S/ %.2f", monto, d.Porcentaje, base), Field: "detraccion.monto"})
	}

	// The deposit comes out of what this comprobante collects, so it can
	// never exceed it. Unreachable while the porcentaje check above holds
	// (no Cat.54 rate is above 100%), but it is the failure mode of getting
	// the base wrong — e.g. computing SPOT over the whole operación on a
	// factura de regularización whose payable is only the saldo.
	if monto > penTotal+0.01 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("monto de detracción %.2f excede el total a pagar del comprobante S/ %.2f", monto, penTotal), Field: "detraccion.monto"})
	}

	return errs
}

// validateDetraccionCatalogo checks the detracción against Cat.54: the código
// must be emittable, the porcentaje must be its catálogo rate, a comprobante
// that grants no crédito fiscal (a boleta, or a nota that corrects one) cannot
// carry an Anexo 2 or 3 or transporte de carga código, neither can one for a
// no-domiciliado usuario (Anexo 3 and carga), and the operación must exceed
// the código's umbral. See DETRACCIONES.md › "Cuándo aplica" and "Cuándo NO
// aplica".
func validateDetraccionCatalogo(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	d := req.Detraccion
	codigo := strings.TrimSpace(d.Codigo)
	if !sunat.Cat54Emit(codigo) {
		return append(errs, model.ValidationError{Code: 3033, Message: fmt.Sprintf("el código de detracción %s no está en el catálogo 54 vigente", codigo), Field: "detraccion.codigo"})
	}

	// 004 alone has a conditional rate (4% only for proveedores on SUNAT's
	// listado); porcentajeAlternativo is the other one it may legally declare.
	pct, _ := strconv.ParseFloat(d.Porcentaje, 64)
	want := sunat.Cat54Porcentaje(codigo)
	alt := sunat.Cat54PorcentajeAlternativo(codigo)
	if math.Abs(pct-want) > 0.001 && (alt == 0 || math.Abs(pct-alt) > 0.001) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("el porcentaje de detracción del código %s es %s%%, no %s%%", codigo, strconv.FormatFloat(want, 'f', -1, 64), d.Porcentaje), Field: "detraccion.porcentaje"})
	}

	// R.S. 183-2004 arts. 8 and 13 and R.S. 073-2006 art. 3: Anexos 2 and 3
	// and transporte de carga exclude comprobantes that grant no crédito
	// fiscal. Anexo 1 (art. 3) and the inmueble regime carry no such exclusion,
	// and it "no opera" when the adquirente is a Sector Público Nacional entity
	// (art. 18 a) LIR), which the user declares at emission.
	//
	// The same Anexo 3 and carga exclude a no-domiciliado usuario (R.S. 183-2004
	// art. 13; R.S. 073-2006 art. 3). Operation type 0401 (venta a no
	// domiciliado) is already refused by validateDetraccion; here the receptor's
	// own document type says it. It is checked only off a boleta, whose
	// anonymous consumidor final also uses Cat.06 "0" and is excluded above.
	anexo := sunat.Cat54Anexo(codigo)
	noDomiciliado := req.CustomerDocType == sunat.Cat06DocTribNoDomSinRUC ||
		req.CustomerDocType == sunat.Cat06DocPaisResidencia
	switch {
	case isSinCreditoFiscal(req) && !req.AdquirenteSectorPublico && (anexo == "2" || anexo == "3" || anexo == "carga"):
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("el código %s no aplica en boletas: la detracción de los Anexos 2 y 3 y del transporte de carga excluye los comprobantes que no otorgan crédito fiscal", codigo), Field: "detraccion.codigo"})
	case !isSinCreditoFiscal(req) && noDomiciliado && (anexo == "3" || anexo == "carga"):
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("el código %s no aplica a un usuario no domiciliado: la detracción del Anexo 3 y del transporte de carga lo excluye", codigo), Field: "detraccion.codigo"})
	}

	// Umbral: strictly greater than, over the operación in soles. A nota mirrors
	// the operation it corrects and an anticipo (0104) is one payment of a larger
	// operación, so neither is measured on its own. An Anexo 1 bien is subject at
	// any importe when one transport unit carries more than ½ UIT of them (R.S.
	// 183-2004 art. 2.1), which the user declares at emission.
	if isNota(req.DocType) || req.OperationType == sunat.Cat51Anticipos {
		return errs
	}
	unidadTransporte := anexo == "1" && req.Anexo1UnidadTransporte
	rate, ok := penRate(req)
	if !ok {
		return errs
	}
	importe, _ := strconv.ParseFloat(req.TaxInclusiveAmount, 64)
	if acordado, err := strconv.ParseFloat(req.MontoOperacion, 64); err == nil && acordado > importe {
		importe = acordado
	}
	base := spotBase(req, importe*rate)
	umbral := sunat.Cat54Umbral(codigo)
	if unidadTransporte {
		// The answer is what makes the proveedor the depositor
		// (DetraccionDepositaEmisor); above the umbral the buyer deposits as
		// usual, so the answer does not belong there.
		if base > umbral {
			errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("la operación (S/ %.2f) ya supera el umbral de S/ %s del código %s: no corresponde declarar la unidad de transporte", base, strconv.FormatFloat(umbral, 'f', 2, 64), codigo), Field: "anexo1UnidadTransporte"})
		}
		return errs
	}
	if base <= umbral {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("la operación (S/ %.2f) no supera el umbral de S/ %s del código %s: no está sujeta a detracción", base, strconv.FormatFloat(umbral, 'f', 2, 64), codigo), Field: "detraccion"})
	}
	return errs
}

// spotBase is the amount the SPOT applies to, in soles: the importe, or for
// transporte de carga (027) the greater of the importe and the valor
// referencial del servicio (R.S. 073-2006 art. 4).
func spotBase(req model.IssueRequest, penImporte float64) float64 {
	if req.Detraccion == nil || req.Detraccion.Codigo != sunat.Cat54ServicioDeTransporteDeCarga || req.TransporteCarga == nil {
		return penImporte
	}
	if vr, err := strconv.ParseFloat(req.TransporteCarga.ValorReferencialServicio, 64); err == nil && vr > penImporte {
		return vr
	}
	return penImporte
}

// penRate is the factor that converts the document's amounts into soles: 1 for
// PEN, the tipo de cambio otherwise. ok is false when a foreign-currency
// document carries no usable rate.
func penRate(req model.IssueRequest) (float64, bool) {
	if req.CurrencyCode == "" || req.CurrencyCode == "PEN" {
		return 1, true
	}
	rate, err := strconv.ParseFloat(req.ExchangeRate, 64)
	if err != nil || rate <= 0 {
		return 0, false
	}
	return rate, true
}

// validateHidrobiologicos guards the six Cat.55 concepts a detracción de
// recursos hidrobiológicos (Cat.54 004 → 1002) declares on every line. Each is
// an ERROR when missing (3063, 3130-3135); the lengths are SUNAT's formats
// (OBSERV 4280) and the cantidad is decimal 12,2 greater than zero (4281).
func validateHidrobiologicos(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError
	h := req.Hidrobiologicos
	if h == nil {
		return append(errs, model.ValidationError{Code: 3063, Message: "la detracción de recursos hidrobiológicos (004) requiere la matrícula y el nombre de la embarcación, y el lugar y la fecha de descarga", Field: "hidrobiologicos"})
	}
	for _, f := range []struct {
		code   int
		value  string
		max    int
		field  string
		nombre string
	}{
		{3063, h.MatriculaEmbarcacion, 15, "hidrobiologicos.matriculaEmbarcacion", "la matrícula de la embarcación"},
		{3130, h.NombreEmbarcacion, 100, "hidrobiologicos.nombreEmbarcacion", "el nombre de la embarcación"},
		{3132, h.LugarDescarga, 100, "hidrobiologicos.lugarDescarga", "el lugar de descarga"},
	} {
		v := strings.TrimSpace(f.value)
		switch {
		case v == "":
			errs = append(errs, model.ValidationError{Code: f.code, Message: "indique " + f.nombre, Field: f.field})
		case utf8.RuneCountInString(v) > f.max || strings.ContainsAny(v, "\n\r\t"):
			errs = append(errs, model.ValidationError{Code: 4280, Message: fmt.Sprintf("%s admite hasta %d caracteres en una sola línea", f.nombre, f.max), Field: f.field})
		}
	}
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(h.FechaDescarga)); err != nil {
		errs = append(errs, model.ValidationError{Code: 3134, Message: "indique la fecha de descarga (AAAA-MM-DD)", Field: "hidrobiologicos.fechaDescarga"})
	}
	for i, li := range req.Items {
		especie := strings.TrimSpace(li.EspecieVendida)
		switch {
		case especie == "":
			errs = append(errs, model.ValidationError{Code: 3131, Message: fmt.Sprintf("línea %d: indique la especie vendida", li.LineNumber), Field: fmt.Sprintf("items[%d].especieVendida", i)})
		case utf8.RuneCountInString(especie) > 150 || strings.ContainsAny(especie, "\n\r\t"):
			errs = append(errs, model.ValidationError{Code: 4280, Message: fmt.Sprintf("línea %d: la especie vendida admite hasta 150 caracteres en una sola línea", li.LineNumber), Field: fmt.Sprintf("items[%d].especieVendida", i)})
		}
		cantidad, err := strconv.ParseFloat(strings.TrimSpace(li.CantidadEspecie), 64)
		switch {
		case strings.TrimSpace(li.CantidadEspecie) == "":
			errs = append(errs, model.ValidationError{Code: 3133, Message: fmt.Sprintf("línea %d: indique la cantidad de especie vendida, en toneladas", li.LineNumber), Field: fmt.Sprintf("items[%d].cantidadEspecie", i)})
		case err != nil || cantidad <= 0 || cantidad >= 1e12:
			errs = append(errs, model.ValidationError{Code: 4281, Message: fmt.Sprintf("línea %d: la cantidad de especie vendida debe ser mayor a 0, con hasta 2 decimales", li.LineNumber), Field: fmt.Sprintf("items[%d].cantidadEspecie", i)})
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
		if v := strings.TrimSpace(u.value); v != "" && !sunat.UbigeoDistritoValid(v) {
			errs = append(errs, model.ValidationError{Code: 2800, Message: "el ubigeo no corresponde a un distrito INEI", Field: u.field})
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
	if !sunat.Cat09Emit(req.ReasonCode) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("invalid NC reason code: %s", req.ReasonCode), Field: "reasonCode"})
	}

	// NC on boletas: codes 04, 05, 08 not allowed. Guarded on Cat09Emit so an
	// código that already failed the check above reports one error, not two —
	// the old negative map could only ever match an emittable código.
	if req.ReferenceDocType == sunat.Cat01Boleta && sunat.Cat09Emit(req.ReasonCode) && !sunat.Cat09OnBoleta(req.ReasonCode) {
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
	if !sunat.Cat10Emit(req.ReasonCode) {
		errs = append(errs, model.ValidationError{Code: 2800, Message: fmt.Sprintf("invalid ND reason code: %s (only 01, 02 allowed)", req.ReasonCode), Field: "reasonCode"})
	}

	// Reference document required
	if req.ReferenceDocSeries == "" || req.ReferenceDocCorrelative <= 0 {
		errs = append(errs, model.ValidationError{Code: 2800, Message: "reference document is required for ND", Field: "referenceDocSeries"})
	}

	return errs
}

// isSinCreditoFiscal reports whether the comprobante grants no crédito fiscal:
// a boleta, or a nota that corrects one.
func isSinCreditoFiscal(req model.IssueRequest) bool {
	return req.DocType == sunat.Cat01Boleta ||
		(isNota(req.DocType) && req.ReferenceDocType == sunat.Cat01Boleta)
}

// isNota reports whether docType is a nota de crédito or débito.
func isNota(docType string) bool {
	return docType == sunat.Cat01NotaCredito || docType == sunat.Cat01NotaDebito
}
