package validation

import (
	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
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
	errs = append(errs, validateGlobalDiscount(req)...)
	errs = append(errs, validateAnticipos(req)...)
	errs = append(errs, validateDetraccion(req)...)

	if req.DocType == sunat.Cat01Factura || req.DocType == sunat.Cat01Boleta {
		errs = append(errs, validatePaymentTerms(req)...)
	}
	if req.DocType == sunat.Cat01NotaCredito {
		errs = append(errs, validateCreditNote(req)...)
	}
	if req.DocType == sunat.Cat01NotaDebito {
		errs = append(errs, validateDebitNote(req)...)
	}

	return errs
}
