package validation

import (
	"github.com/perunio/perunio-facturador/internal/model"
)

// Validate runs all pre-submission validation rules and returns any errors found.
func Validate(req model.IssueRequest) []model.ValidationError {
	var errs []model.ValidationError

	errs = append(errs, validateHeader(req)...)
	errs = append(errs, validateSupplier(req)...)
	errs = append(errs, validateCustomer(req)...)
	errs = append(errs, validateAmounts(req)...)
	errs = append(errs, validateLines(req)...)

	if req.DocType == model.DocTypeFactura || req.DocType == model.DocTypeBoleta {
		errs = append(errs, validatePaymentTerms(req)...)
	}
	if req.DocType == model.DocTypeNotaCredito {
		errs = append(errs, validateCreditNote(req)...)
	}
	if req.DocType == model.DocTypeNotaDebito {
		errs = append(errs, validateDebitNote(req)...)
	}

	return errs
}
