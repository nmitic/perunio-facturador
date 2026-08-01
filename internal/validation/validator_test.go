package validation_test

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/validation"
)

func newValidInvoice() model.IssueRequest {
	return model.IssueRequest{
		SupplierRUC:        "20100113612",
		SupplierName:       "EMPRESA TEST SAC",
		SupplierAddress:    "AV. TEST 123",
		EstablishmentCode:  "0000",
		DocType:            "01",
		Series:             "F001",
		Correlative:        1,
		IssueDate:          "2024-01-15",
		IssueTime:          "15:20:30",
		CurrencyCode:       "PEN",
		OperationType:      "0101",
		CustomerDocType:    "6",
		CustomerDocNumber:  "20601327318",
		CustomerName:       "CLIENTE TEST SRL",
		Subtotal:           "1000.00",
		TotalIGV:           "180.00",
		TotalISC:           "0.00",
		TotalOtherTaxes:    "0.00",
		TotalDiscount:      "0.00",
		TotalAmount:        "1180.00",
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

// newBoletaUnder700 returns a valid boleta (03) for S/118 with a single line,
// leaving the customer fields for the test to set.
func newBoletaUnder700() model.IssueRequest {
	req := newValidInvoice()
	req.DocType = "03"
	req.Series = "B001"
	req.Subtotal = "100.00"
	req.TotalIGV = "18.00"
	req.TotalAmount = "118.00"
	req.TaxInclusiveAmount = "118.00"
	req.Items = []model.LineItem{
		{
			LineNumber: 1, Description: "PRODUCTO TEST", Quantity: "1",
			UnitCode: "NIU", UnitPrice: "100.00", UnitPriceWithTax: "118.00",
			TaxExemptionReasonCode: "10", IGVAmount: "18.00",
			LineTotal: "100.00", PriceTypeCode: "01",
		},
	}
	return req
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

	t.Run("should pass for a consumidor final boleta at or under 700", func(t *testing.T) {
		req := newBoletaUnder700()
		req.CustomerDocType = model.IdentityDocTribNoRUC
		req.CustomerDocNumber = model.ConsumidorFinalDocNumber
		req.CustomerName = model.ConsumidorFinalName
		errs := validation.Validate(req)
		is.Equal(t, 0, len(errs))
	})

	t.Run("should fail when boleta over 700 uses consumidor final identity", func(t *testing.T) {
		req := newBoletaUnder700()
		req.Subtotal = "800.00"
		req.TotalIGV = "144.00"
		req.TotalAmount = "944.00"
		req.TaxInclusiveAmount = "944.00"
		req.Items[0].UnitPrice = "800.00"
		req.Items[0].UnitPriceWithTax = "944.00"
		req.Items[0].IGVAmount = "144.00"
		req.Items[0].LineTotal = "800.00"
		req.CustomerDocType = model.IdentityDocTribNoRUC
		req.CustomerDocNumber = model.ConsumidorFinalDocNumber
		req.CustomerName = model.ConsumidorFinalName
		errs := validation.Validate(req)
		is.True(t, hasErrorCode(errs, 2800))
	})

	t.Run("should pass when boleta over 700 carries a valid DNI", func(t *testing.T) {
		req := newBoletaUnder700()
		req.Subtotal = "800.00"
		req.TotalIGV = "144.00"
		req.TotalAmount = "944.00"
		req.TaxInclusiveAmount = "944.00"
		req.Items[0].UnitPrice = "800.00"
		req.Items[0].UnitPriceWithTax = "944.00"
		req.Items[0].IGVAmount = "144.00"
		req.Items[0].LineTotal = "800.00"
		req.CustomerDocType = model.IdentityDNI
		req.CustomerDocNumber = "12345678"
		req.CustomerName = "JUAN PEREZ"
		errs := validation.Validate(req)
		is.Equal(t, 0, len(errs))
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

	t.Run("should pass IGV tolerance when ISC is included in the base", func(t *testing.T) {
		req := newValidInvoice()
		// IGV applies on top of ISC: (100 + 20) * 18% = 21.60
		req.Items = []model.LineItem{
			{
				LineNumber: 1, Description: "PRODUCTO CON ISC", Quantity: "1",
				UnitCode: "NIU", UnitPrice: "100.00", UnitPriceWithTax: "141.60",
				TaxExemptionReasonCode: "10", IGVAmount: "21.60", ISCAmount: "20.00",
				LineTotal: "100.00", PriceTypeCode: "01",
			},
		}
		req.Subtotal = "100.00"
		req.TotalIGV = "21.60"
		req.TotalISC = "20.00"
		req.TotalAmount = "141.60"
		req.TaxInclusiveAmount = "141.60"
		errs := validation.Validate(req)
		is.True(t, !hasErrorCode(errs, 3103))
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

func hasErrorField(errs []model.ValidationError, field string) bool {
	for _, e := range errs {
		if strings.HasPrefix(e.Field, field) {
			return true
		}
	}
	return false
}

func newDetraccion() *model.Detraccion {
	// 12% of 1180.00 = 141.60
	return &model.Detraccion{Codigo: "019", Porcentaje: "12.00", Monto: "141.60", CuentaBN: "00-123-456789"}
}

func TestValidateDetraccion(t *testing.T) {
	t.Run("valid factura sujeta a detracción passes", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1001"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "should have no detracción errors")
	})

	t.Run("missing cuenta BN is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1001"
		d := newDetraccion()
		d.CuentaBN = ""
		req.Detraccion = d
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.cuentaBN"), "should require cuenta BN")
	})

	t.Run("detracción on a boleta is rejected", func(t *testing.T) {
		req := newBoletaUnder700()
		req.OperationType = "1001"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "should reject detracción on boleta")
	})

	t.Run("nota de crédito referencing a factura is allowed", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "FC01"
		req.OperationType = "1001"
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "should allow detracción on NC referencing a factura")
	})

	t.Run("nota de crédito referencing a boleta is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "BC01"
		req.OperationType = "1001"
		req.ReferenceDocType = "03"
		req.ReferenceDocSeries = "B001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "should reject detracción on NC referencing a boleta")
	})

	t.Run("monto not matching porcentaje × total is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1001"
		d := newDetraccion()
		d.Monto = "100.00" // should be 141.60
		req.Detraccion = d
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.monto"), "should reject mismatched monto")
	})

	t.Run("operationType not 1001/1002/0104 is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "0101"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "operationType"), "should require operationType 1001/1002/0104")
	})

	t.Run("a factura de anticipo (0104) sujeta a detracción is allowed", func(t *testing.T) {
		// The anticipo keeps its 0104 marker in the DB; xmlbuilder maps it onto
		// 1001/1002 on the wire, so validation must not demand it here.
		req := newValidInvoice()
		req.OperationType = "0104"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "operationType"), "0104 carries the detracción through")
		is.True(t, !hasErrorField(errs, "detraccion"), "should have no detracción errors")
	})

	t.Run("a monto exceeding the total a pagar is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: "019", Porcentaje: "150.00", Monto: "1770.00", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.monto"), "cannot deposit more than the comprobante collects")
	})
}

func newAnticipo() model.Anticipo {
	return model.Anticipo{
		DocID: "F001-00000042", DocTypeCode: "02",
		TotalAmount: "118.00", BaseAmount: "100.00",
	}
}

func TestValidateAnticipos(t *testing.T) {
	// The regularización declares the sale in full (1000 gravado + 180 IGV =
	// 1180) and deducts the 118 anticipo only from the payable: 1062.
	newRegularizacion := func() model.IssueRequest {
		req := newValidInvoice()
		req.TotalAmount = "1062.00"
		req.Anticipos = []model.Anticipo{newAnticipo()}
		return req
	}

	t.Run("valid factura de regularización passes", func(t *testing.T) {
		errs := validation.Validate(newRegularizacion())
		is.True(t, !hasErrorField(errs, "anticipos"), "should have no anticipo errors")
	})

	t.Run("anticipos on a boleta are rejected", func(t *testing.T) {
		req := newBoletaUnder700()
		req.Anticipos = []model.Anticipo{newAnticipo()}
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos"), "should reject anticipos on a boleta")
	})

	t.Run("anticipos combined with detracción on the saldo pass", func(t *testing.T) {
		// SPOT arises per payment, over the comprobante that documents it: the
		// anticipo already declared its own share, so this factura's detracción
		// covers only the 1062.00 payable. 12% of that is 127.44.
		req := newRegularizacion()
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: "019", Porcentaje: "12.00", Monto: "127.44", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "anticipos"), "anticipos + detracción is a valid combination")
		is.True(t, !hasErrorField(errs, "detraccion.monto"), "12% of the payable is the right base")
	})

	t.Run("a detracción computed over the whole operación instead of the saldo is rejected", func(t *testing.T) {
		// 141.60 = 12% of the full 1180.00 sale, but only 1062.00 is being
		// collected here — the other 118.00 was deposited with the anticipo.
		req := newRegularizacion()
		req.OperationType = "1001"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.monto"), "the base is the payable, not the operación")
	})

	t.Run("a 0104 anticipo factura cannot itself apply anticipos", func(t *testing.T) {
		req := newRegularizacion()
		req.OperationType = "0104"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "operationType"), "should reject anticipos on a 0104 factura")
	})

	t.Run("docTypeCode outside 02/03 is rejected", func(t *testing.T) {
		req := newRegularizacion()
		req.Anticipos[0].DocTypeCode = "01"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos[0].docTypeCode"), "should reject a non-anticipo Cat.12 code")
	})

	t.Run("base not matching total minus 18% IGV is rejected", func(t *testing.T) {
		req := newRegularizacion()
		req.Anticipos[0].BaseAmount = "90.00"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos[0].baseAmount"), "should require total ≈ base × 1.18")
	})

	t.Run("zero total is rejected", func(t *testing.T) {
		req := newRegularizacion()
		req.Anticipos[0].TotalAmount = "0.00"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos[0].totalAmount"), "should require a positive monto")
	})

	t.Run("anticipos exceeding the total of the comprobante are rejected", func(t *testing.T) {
		req := newRegularizacion()
		req.Anticipos = []model.Anticipo{
			{DocID: "F001-00000042", DocTypeCode: "02", TotalAmount: "708.00", BaseAmount: "600.00"},
			{DocID: "F001-00000043", DocTypeCode: "02", TotalAmount: "708.00", BaseAmount: "600.00"},
		}
		req.TotalAmount = "-236.00"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos"), "1416 anticipado cannot come off a 1180 sale")
	})

	t.Run("anticipos exactly at the total, after a descuento global, pass", func(t *testing.T) {
		req := newRegularizacion()
		// 1000 − 100 discount = 900 base, IGV 162 → the sale totals 1062, all
		// of it already collected, so nothing is left to pay.
		req.GlobalDiscount = "100.00"
		req.TotalIGV = "162.00"
		req.TaxInclusiveAmount = "1062.00"
		req.TotalAmount = "0.00"
		req.Anticipos = []model.Anticipo{
			{DocID: "F001-00000042", DocTypeCode: "02", TotalAmount: "1062.00", BaseAmount: "900.00"},
		}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "anticipos"), "anticipos may cover the sale exactly")
	})
}
