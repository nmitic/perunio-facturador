package validation_test

import (
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/validation"
)

func newValidInvoice() model.IssueRequest {
	return model.IssueRequest{
		SupplierRUC:       "20100113612",
		SupplierName:      "EMPRESA TEST SAC",
		SupplierAddress:   "AV. TEST 123",
		EstablishmentCode: "0000",
		DocType:           "01",
		Series:            "F001",
		Correlative:       1,
		IssueDate:         "2024-01-15",
		IssueTime:         "15:20:30",
		CurrencyCode:      "PEN",
		OperationType:     "0101",
		CustomerDocType:   "6",
		CustomerDocNumber: "20601327318",
		CustomerName:      "CLIENTE TEST SRL",
		Subtotal:          "1000.00",
		TotalIGV:          "180.00",
		TotalISC:          "0.00",
		TotalOtherTaxes:   "0.00",
		TotalDiscount:     "0.00",
		TotalAmount:       "1180.00",
		TaxInclusiveAmount: "1180.00",
		Items: []model.LineItem{
			{
				LineNumber: 1, Description: "PRODUCTO TEST", Quantity: "10",
				UnitCode: "NIU", UnitPrice: "100.00", UnitPriceWithTax: "118.00",
				TaxExemptionReasonCode: "10", IGVAmount: "180.00",
				LineTotal: "1000.00", PriceTypeCode: "01",
			},
		},
	}
}

func TestValidate(t *testing.T) {
	t.Run("should pass for a valid factura", func(t *testing.T) {
		errs := validation.Validate(newValidInvoice())
		is.Equal(t, 0, len(errs))
	})

	t.Run("should fail when supplier RUC is invalid", func(t *testing.T) {
		req := newValidInvoice()
		req.SupplierRUC = "123"
		errs := validation.Validate(req)
		is.True(t, len(errs) > 0)
		is.True(t, hasErrorCode(errs, 1007))
	})

	t.Run("should fail when factura customer is not RUC type", func(t *testing.T) {
		req := newValidInvoice()
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2800))
	})

	t.Run("should fail when boleta over 700 has no customer identity", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "03"
		req.Series = "B001"
		req.TotalAmount = "800.00"
		req.CustomerDocType = ""
		req.CustomerDocNumber = ""
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2800))
	})

	t.Run("should fail when line items are empty", func(t *testing.T) {
		req := newValidInvoice()
		req.Items = nil
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2023))
	})

	t.Run("should fail for IGV amount outside tolerance", func(t *testing.T) {
		req := newValidInvoice()
		req.Items[0].IGVAmount = "999.99" // way off from expected 180.00
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 3103))
	})

	t.Run("should fail for NC with invalid reason code on boleta", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "BC01"
		req.ReasonCode = "04" // Descuento global — not allowed on boleta
		req.ReferenceDocType = "03"
		req.ReferenceDocSeries = "B001"
		req.ReferenceDocCorrelative = 1
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2800))
	})

	t.Run("should fail for ND with invalid reason code", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "08"
		req.Series = "FD01"
		req.ReasonCode = "03" // Invalid — only 01, 02 allowed
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2800))
	})
}

func hasErrorCode(errs []model.ValidationError, code int) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}
