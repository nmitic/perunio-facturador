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

	// Boleta > S/700: customer identity required
	if req.DocType == model.DocTypeBoleta {
		total, err := strconv.ParseFloat(req.TotalAmount, 64)
		if err == nil && total > 700.00 {
			if req.CustomerDocType == "" || req.CustomerDocNumber == "" {
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

		// IGV tolerance check (±1 cent) for gravado onerosa lines
		if li.TaxExemptionReasonCode == "10" {
			errs = append(errs, validateIGVTolerance(li)...)
		}
	}

	return errs
}

func validateIGVTolerance(li model.LineItem) []model.ValidationError {
	var errs []model.ValidationError

	lineTotal, err := strconv.ParseFloat(li.LineTotal, 64)
	if err != nil {
		return errs
	}
	igvAmount, err := strconv.ParseFloat(li.IGVAmount, 64)
	if err != nil {
		return errs
	}

	expectedIGV := lineTotal * 0.18
	if math.Abs(expectedIGV-igvAmount) > 1.0 { // ±1 cent tolerance (SUNAT uses integer centavos, tolerance of 1)
		errs = append(errs, model.ValidationError{
			Code:    3103,
			Message: fmt.Sprintf("line %d: IGV amount %.2f exceeds tolerance (expected ~%.2f)", li.LineNumber, igvAmount, expectedIGV),
			Field:   fmt.Sprintf("items[%d].igvAmount", li.LineNumber),
		})
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
