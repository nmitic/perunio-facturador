package validation_test

import (
	"strconv"
	"strings"
	"testing"

	"maragu.dev/is"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
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
		req.CustomerDocType = sunat.Cat06DocTribNoDomSinRUC
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
		req.CustomerDocType = sunat.Cat06DocTribNoDomSinRUC
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
		req.CustomerDocType = sunat.Cat06DNI
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
	// Otros servicios empresariales (Anexo 3): 12% of 1180.00 = 141.60
	return &model.Detraccion{Codigo: sunat.Cat54OtrosServiciosEmpresariales, Porcentaje: "12.00", Monto: "141.60", CuentaBN: "00-123-456789"}
}

// newInmuebleDetraccion is the venta de inmueble gravada (040): 4% at any
// amount and outside the Anexo 2/3 boleta exclusion, so it is what a boleta
// under S/700 can legally carry. total is the comprobante's importe.
func newInmuebleDetraccion(total float64) *model.Detraccion {
	return &model.Detraccion{Codigo: sunat.Cat54BienInmuebleGravadoConIgv, Porcentaje: "4.00", Monto: strconv.FormatFloat(total*0.04, 'f', 2, 64), CuentaBN: "00-123-456789"}
}

// newBoletaDetraccion is newInmuebleDetraccion over newBoletaUnder700's total:
// 4% of 118.00 = 4.72.
func newBoletaDetraccion() *model.Detraccion {
	return newInmuebleDetraccion(118)
}

// asConsumidorFinal strips the receptor down to the anonymous placeholder a
// boleta falls back to when nobody is entered.
func asConsumidorFinal(req model.IssueRequest) model.IssueRequest {
	req.CustomerDocType = sunat.Cat06DocTribNoDomSinRUC
	req.CustomerDocNumber = model.ConsumidorFinalDocNumber
	req.CustomerName = model.ConsumidorFinalName
	return req
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

	t.Run("boleta sujeta a detracción passes", func(t *testing.T) {
		// Catálogo 51 marks 1001-1004 "Factura, Boletas" and the Boleta2_0 sheet
		// of SUNAT's validation pack carries the same detracción rules as
		// Factura2_0 — see validateDetraccion.
		req := newBoletaUnder700()
		req.OperationType = "1001"
		req.Detraccion = newBoletaDetraccion()
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "should allow detracción on a boleta")
	})

	t.Run("detracción on a consumidor-final boleta is rejected", func(t *testing.T) {
		// The constancia de depósito is filed under the adquirente, so
		// "CLIENTES VARIOS" cannot carry a SPOT operation.
		req := asConsumidorFinal(newBoletaUnder700())
		req.OperationType = "1001"
		req.Detraccion = newBoletaDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "customerDocType"), "should require an identified adquirente")
	})

	t.Run("a boleta de anticipo (0104) sujeta a detracción is allowed", func(t *testing.T) {
		req := newBoletaUnder700()
		req.OperationType = "0104"
		req.Detraccion = newBoletaDetraccion()
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "operationType"), "0104 carries the detracción through")
		is.True(t, !hasErrorField(errs, "detraccion"), "should have no detracción errors")
	})

	t.Run("a consumidor-final boleta de anticipo (0104) is rejected", func(t *testing.T) {
		req := asConsumidorFinal(newBoletaUnder700())
		req.OperationType = "0104"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "customerDocType"), "an anticipo must name the buyer it will be reconciled against")
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

	t.Run("nota de crédito referencing a boleta is allowed", func(t *testing.T) {
		// A boleta can be sujeta a detracción, so a nota correcting one mirrors it.
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "BC01"
		req.OperationType = "1001"
		req.ReferenceDocType = "03"
		req.ReferenceDocSeries = "B001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Detraccion = newInmuebleDetraccion(1180)
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "should allow detracción on NC referencing a boleta")
	})

	t.Run("nota de débito referencing a boleta is allowed", func(t *testing.T) {
		// The half SUNAT's NotaDebito2_0 sheet actually validates.
		req := newValidInvoice()
		req.DocType = "08"
		req.Series = "BD01"
		req.OperationType = "1001"
		req.ReferenceDocType = "03"
		req.ReferenceDocSeries = "B001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Detraccion = newInmuebleDetraccion(1180)
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "should allow detracción on ND referencing a boleta")
	})

	t.Run("detracción on a nota referencing a guía is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "FC01"
		req.OperationType = "1001"
		req.ReferenceDocType = "09"
		req.ReferenceDocSeries = "T001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "only facturas and boletas can carry a detracción")
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

	t.Run("recursos hidrobiológicos (004) needs the embarcación and descarga", func(t *testing.T) {
		req := newHidrobiologicos("4.00", "47.20")
		req.Hidrobiologicos = nil
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "hidrobiologicos"), "3063: SUNAT rejects a 1002 line without 3001")
	})

	t.Run("recursos hidrobiológicos (004) at 4% and at 10% pass", func(t *testing.T) {
		for _, c := range [][2]string{{"4.00", "47.20"}, {"10.00", "118.00"}} {
			errs := validation.Validate(newHidrobiologicos(c[0], c[1]))
			is.Equal(t, 0, len(errs))
		}
	})

	t.Run("recursos hidrobiológicos (004) refuses a line without especie or cantidad", func(t *testing.T) {
		req := newHidrobiologicos("4.00", "47.20")
		req.Items[0].EspecieVendida = ""
		req.Items[0].CantidadEspecie = "0"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "items[0].especieVendida"), "3131 runs per line")
		is.True(t, hasErrorField(errs, "items[0].cantidadEspecie"), "the cantidad must be greater than 0 (4281)")
	})

	t.Run("recursos hidrobiológicos (004) refuses an over-long matrícula", func(t *testing.T) {
		req := newHidrobiologicos("4.00", "47.20")
		req.Hidrobiologicos.MatriculaEmbarcacion = "PL-1234567890-CM"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "hidrobiologicos.matriculaEmbarcacion"), "3001 is an..15")
	})

	t.Run("transporte de pasajeros (028) is refused: it is not a percentage", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1003"
		d := newDetraccion()
		d.Codigo = sunat.Cat54TransporteDePasajeros
		req.Detraccion = d
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.codigo"), "028 is a fixed amount per vehicle")
	})

	t.Run("a monto exceeding the total a pagar is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54OtrosServiciosEmpresariales, Porcentaje: "150.00", Monto: "1770.00", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.monto"), "cannot deposit more than the comprobante collects")
	})
}

// newServicio780 is factura F001-00000008 as it should have gone out: S/ 780
// of servicios de contabilidad (Cat.54 022, Anexo 3 numeral 5, 12%).
func newServicio780() model.IssueRequest {
	req := newValidInvoice()
	req.Subtotal = "661.02"
	req.TotalIGV = "118.98"
	req.TotalAmount = "780.00"
	req.TaxInclusiveAmount = "780.00"
	req.Items[0].Quantity = "3"
	req.Items[0].UnitPrice = "220.34"
	req.Items[0].UnitPriceWithTax = "260.00"
	req.Items[0].IGVAmount = "118.98"
	req.OperationType = "1001"
	req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54OtrosServiciosEmpresariales, Porcentaje: "12.00", Monto: "93.60", CuentaBN: "00-123-456789"}
	return req
}

func TestValidateDetraccionCatalogo(t *testing.T) {
	t.Run("servicios de contabilidad over S/700 at 12% pass", func(t *testing.T) {
		errs := validation.Validate(newServicio780())
		is.True(t, !hasErrorField(errs, "detraccion") && !hasErrorField(errs, "detraccion.porcentaje") && !hasErrorField(errs, "detraccion.monto"), "022 at 12% over S/780 is the correct detracción")
	})

	t.Run("a rate other than the catálogo's is rejected", func(t *testing.T) {
		req := newServicio780()
		req.Detraccion.Porcentaje = "10.00" // the pre-2018 rate
		req.Detraccion.Monto = "78.00"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.porcentaje"), "022 is 12% since R.S. 071-2018")
	})

	t.Run("a código outside Cat.54 is rejected", func(t *testing.T) {
		req := newServicio780()
		req.Detraccion.Codigo = "006" // derogated
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.codigo"), "SUNAT would answer 3033")
	})

	t.Run("an importe of exactly S/700 is not sujeto", func(t *testing.T) {
		req := newServicio780()
		req.TaxInclusiveAmount = "700.00"
		req.TotalAmount = "700.00"
		req.Detraccion.Monto = "84.00"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "the umbral is strictly greater than S/700")
	})

	t.Run("a comprobante of a larger venta is measured against the monto acordado", func(t *testing.T) {
		req := newServicio780()
		req.TaxInclusiveAmount = "590.00"
		req.TotalAmount = "590.00"
		req.Detraccion.Monto = "70.80"
		req.MontoOperacion = "2360.00"
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "Perunio measures the umbral of a venta con anticipos on the operación")
	})

	t.Run("Anexo 1 needs more than half a UIT", func(t *testing.T) {
		req := newValidInvoice() // 1180.00 < 2750
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54AzucarYMelazaDeCana, Porcentaje: "10.00", Monto: "118.00", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "001 applies above S/2,750 in 2026")
	})

	t.Run("Anexo 1 under the umbral is subject when one transport unit carries more than half a UIT", func(t *testing.T) {
		req := newValidInvoice() // 1180.00 < 2750
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54AzucarYMelazaDeCana, Porcentaje: "10.00", Monto: "118.00", CuentaBN: "00-123-456789"}
		req.Anexo1UnidadTransporte = true
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "R.S. 183-2004 art. 2.1 measures the transport unit, not the operación")
	})

	t.Run("the transport-unit answer does not lift the umbral of another anexo", func(t *testing.T) {
		req := newServicio780()
		req.TaxInclusiveAmount = "700.00"
		req.TotalAmount = "700.00"
		req.Detraccion.Monto = "84.00"
		req.Anexo1UnidadTransporte = true
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "art. 2.1 is an Anexo 1 rule")
	})

	t.Run("the transport-unit answer is refused above the Anexo 1 umbral", func(t *testing.T) {
		req := newValidInvoice()
		req.Subtotal = "2542.37"
		req.TotalIGV = "457.63"
		req.TotalAmount = "3000.00"
		req.TaxInclusiveAmount = "3000.00"
		req.Items[0].Quantity = "1"
		req.Items[0].UnitPrice = "2542.37"
		req.Items[0].UnitPriceWithTax = "3000.00"
		req.Items[0].IGVAmount = "457.63"
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54AzucarYMelazaDeCana, Porcentaje: "10.00", Monto: "300.00", CuentaBN: "00-123-456789"}
		req.Anexo1UnidadTransporte = true
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anexo1UnidadTransporte"), "above ½ UIT the buyer deposits; the answer would wrongly move the deposit to the proveedor")
	})

	t.Run("residuos (010) apply at any amount", func(t *testing.T) {
		req := newBoletaUnder700()
		req.DocType = "01"
		req.Series = "F001"
		req.CustomerDocType = "6"
		req.CustomerDocNumber = "20601327318"
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54ResiduosSubproductosDesechosRecortesYDesperd, Porcentaje: "15.00", Monto: "17.70", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion"), "Anexo 2 numeral 6 has no umbral")
	})

	t.Run("an Anexo 3 servicio on a boleta is rejected", func(t *testing.T) {
		req := newServicio780()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.codigo"), "a boleta grants no crédito fiscal (R.S. 183-2004 art. 13)")
	})

	t.Run("an Anexo 3 servicio on a boleta to a Sector Público entity is allowed", func(t *testing.T) {
		req := newServicio780()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "6"
		req.CustomerDocNumber = "20131312955"
		req.AdquirenteSectorPublico = true
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion.codigo"), "the exclusion no opera for an art. 18 a) LIR entity (R.S. 183-2004 art. 13)")
	})

	t.Run("an Anexo 1 bien on a boleta is allowed", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		req.Subtotal = "2542.37"
		req.TotalIGV = "457.63"
		req.TotalAmount = "3000.00"
		req.TaxInclusiveAmount = "3000.00"
		req.Items[0].Quantity = "1"
		req.Items[0].UnitPrice = "2542.37"
		req.Items[0].UnitPriceWithTax = "3000.00"
		req.Items[0].IGVAmount = "457.63"
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54AzucarYMelazaDeCana, Porcentaje: "10.00", Monto: "300.00", CuentaBN: "00-123-456789"}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion") && !hasErrorField(errs, "detraccion.codigo"), "Anexo 1 excludes only póliza de adjudicación and liquidación de compra")
	})

	t.Run("an Anexo 3 servicio for a no-domiciliado usuario is rejected", func(t *testing.T) {
		for _, docType := range []string{sunat.Cat06DocTribNoDomSinRUC, sunat.Cat06DocPaisResidencia} {
			req := newServicio780()
			req.CustomerDocType = docType
			req.CustomerDocNumber = "X1234567"
			errs := validation.Validate(req)
			is.True(t, hasErrorField(errs, "detraccion.codigo"), "R.S. 183-2004 art. 13 excludes a no-domiciliado usuario")
		}
	})

	t.Run("a venta de inmueble for a no-domiciliado buyer is allowed", func(t *testing.T) {
		req := newValidInvoice()
		req.CustomerDocType = sunat.Cat06DocPaisResidencia
		req.CustomerDocNumber = "X1234567"
		req.OperationType = "1001"
		req.Detraccion = newInmuebleDetraccion(1180)
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion.codigo"), "the no-domiciliado exclusion covers Anexo 3 and carga only")
	})

	t.Run("a dollar factura is checked in soles", func(t *testing.T) {
		req := newServicio780()
		req.CurrencyCode = "USD"
		req.ExchangeRate = "3.500"
		req.Detraccion.Monto = "327.60" // 12% of 780.00 × 3.5
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "detraccion.monto"), "the monto is 12% of the importe in soles")

		req.Detraccion.Monto = "93.60" // forgot to convert
		errs = validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.monto"), "an unconverted monto is rejected")
	})

	t.Run("a dollar factura under S/700 in soles is not sujeta", func(t *testing.T) {
		req := newServicio780()
		req.CurrencyCode = "USD"
		req.ExchangeRate = "0.800"     // contrived: 780 × 0.8 = S/ 624
		req.Detraccion.Monto = "74.88" // 12% of 624
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion"), "the umbral is in soles")
	})
}

// newHidrobiologicos is the 1180.00 factura sold as recursos hidrobiológicos
// (004 → 1002), with every Cat.55 concept filled in.
func newHidrobiologicos(porcentaje, monto string) model.IssueRequest {
	req := newValidInvoice()
	req.OperationType = "1002"
	req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54RecursosHidrobiologicos, Porcentaje: porcentaje, Monto: monto, CuentaBN: "00-123-456789"}
	req.Hidrobiologicos = &model.Hidrobiologicos{
		MatriculaEmbarcacion: "PL-12345-CM",
		NombreEmbarcacion:    "DON LUCHO II",
		LugarDescarga:        "Muelle artesanal de Chimbote",
		FechaDescarga:        "2024-01-14",
	}
	for i := range req.Items {
		req.Items[i].EspecieVendida = "Anchoveta"
		req.Items[i].CantidadEspecie = "10.00"
	}
	return req
}

func newTransporteCarga() *model.TransporteCarga {
	return &model.TransporteCarga{
		OrigenUbigeo:                  "150101",
		OrigenDireccion:               "AV. ARGENTINA 1234, CERCADO DE LIMA",
		DestinoUbigeo:                 "040101",
		DestinoDireccion:              "AV. EJERCITO 500, AREQUIPA",
		DetalleViaje:                  "Lima - Arequipa, carga general",
		ValorReferencialServicio:      "1500.00",
		ValorReferencialCargaEfectiva: "1400.00",
		ValorReferencialCargaUtil:     "1600.00",
	}
}

// newTransporteCargaInvoice is a factura sujeta a detracción por transporte de
// carga (Cat.54 027 → tipo de operación 1004), with the trip block attached.
func newTransporteCargaInvoice() model.IssueRequest {
	req := newValidInvoice()
	req.OperationType = "1004"
	req.Detraccion = &model.Detraccion{
		Codigo: sunat.Cat54ServicioDeTransporteDeCarga, Porcentaje: "4.00",
		// 4% of the valor referencial (1500.00), which beats the 1180.00
		// importe: R.S. 073-2006 art. 4.
		Monto: "60.00", CuentaBN: "00-123-456789",
	}
	req.TransporteCarga = newTransporteCarga()
	return req
}

func TestValidateTransporteCarga(t *testing.T) {
	t.Run("a complete transporte de carga passes", func(t *testing.T) {
		errs := validation.Validate(newTransporteCargaInvoice())
		for _, e := range errs {
			t.Errorf("unexpected error: %d %s %s", e.Code, e.Field, e.Message)
		}
	})

	t.Run("transporte de carga for a no-domiciliado usuario is rejected", func(t *testing.T) {
		req := newTransporteCargaInvoice()
		req.CustomerDocType = sunat.Cat06DocTribNoDomSinRUC
		req.CustomerDocNumber = "X1234567"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.codigo"), "R.S. 073-2006 art. 3 excludes a no-domiciliado usuario")
	})

	t.Run("código 027 without the trip block is rejected", func(t *testing.T) {
		req := newTransporteCargaInvoice()
		req.TransporteCarga = nil
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "transporteCarga"), "1004 requires the trip data")
	})

	t.Run("each mandatory trip field is required", func(t *testing.T) {
		fields := map[string]func(*model.TransporteCarga){
			"transporteCarga.origenUbigeo":     func(t *model.TransporteCarga) { t.OrigenUbigeo = "" },
			"transporteCarga.origenDireccion":  func(t *model.TransporteCarga) { t.OrigenDireccion = "" },
			"transporteCarga.destinoUbigeo":    func(t *model.TransporteCarga) { t.DestinoUbigeo = "" },
			"transporteCarga.destinoDireccion": func(t *model.TransporteCarga) { t.DestinoDireccion = "" },
			"transporteCarga.detalleViaje":     func(t *model.TransporteCarga) { t.DetalleViaje = "" },
		}
		for field, blank := range fields {
			req := newTransporteCargaInvoice()
			blank(req.TransporteCarga)
			errs := validation.Validate(req)
			is.True(t, hasErrorField(errs, field), "should require "+field)
		}
	})

	t.Run("a malformed ubigeo is rejected", func(t *testing.T) {
		req := newTransporteCargaInvoice()
		req.TransporteCarga.DestinoUbigeo = "4010"
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "transporteCarga.destinoUbigeo"), "ubigeo must be 6 digits")
	})

	t.Run("each valor referencial must be present and positive", func(t *testing.T) {
		fields := map[string]func(*model.TransporteCarga){
			"transporteCarga.valorReferencialServicio":      func(t *model.TransporteCarga) { t.ValorReferencialServicio = "" },
			"transporteCarga.valorReferencialCargaEfectiva": func(t *model.TransporteCarga) { t.ValorReferencialCargaEfectiva = "0.00" },
			"transporteCarga.valorReferencialCargaUtil":     func(t *model.TransporteCarga) { t.ValorReferencialCargaUtil = "-5.00" },
		}
		for field, break_ := range fields {
			req := newTransporteCargaInvoice()
			break_(req.TransporteCarga)
			errs := validation.Validate(req)
			is.True(t, hasErrorField(errs, field), "should require "+field)
		}
	})

	t.Run("transporte de carga on a boleta is rejected", func(t *testing.T) {
		req := newBoletaUnder700()
		req.OperationType = "1004"
		req.Detraccion = &model.Detraccion{
			Codigo: sunat.Cat54ServicioDeTransporteDeCarga, Porcentaje: "4.00",
			Monto: "60.00", CuentaBN: "00-123-456789", // 4% of the 1500.00 valor referencial
		}
		req.TransporteCarga = newTransporteCarga()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "detraccion.codigo"), "R.S. 073-2006 art. 3 excludes comprobantes without crédito fiscal")
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

	// A boleta de regularización: the sale is 100 + 18 IGV = 118 in full, and a
	// 59.00 anticipo (50.00 base) comes off the payable only, leaving 59.00.
	newBoletaRegularizacion := func() model.IssueRequest {
		req := newBoletaUnder700()
		req.TotalAmount = "59.00"
		req.Anticipos = []model.Anticipo{
			{DocID: "B001-00000007", DocTypeCode: "03", TotalAmount: "59.00", BaseAmount: "50.00"},
		}
		return req
	}

	t.Run("boleta de regularización applying a boleta de anticipo passes", func(t *testing.T) {
		errs := validation.Validate(newBoletaRegularizacion())
		is.True(t, !hasErrorField(errs, "anticipos"), "should allow anticipos on a boleta")
	})

	t.Run("a boleta may apply a factura de anticipo", func(t *testing.T) {
		// Cat.12 admits both 02 and 03 regardless of the regularizing document.
		req := newBoletaRegularizacion()
		req.Anticipos[0] = model.Anticipo{DocID: "F001-00000042", DocTypeCode: "02", TotalAmount: "59.00", BaseAmount: "50.00"}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "anticipos"), "docTypeCode 02 on a boleta is valid")
	})

	t.Run("anticipos on a consumidor-final boleta are rejected", func(t *testing.T) {
		req := asConsumidorFinal(newBoletaRegularizacion())
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "customerDocType"), "cannot reconcile an anticipo against CLIENTES VARIOS")
	})

	t.Run("anticipos on a nota are rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.DocType = "07"
		req.Series = "FC01"
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.Anticipos = []model.Anticipo{newAnticipo()}
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos"), "a nota regularizes nothing")
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

// newCreditoInvoice is a factura a crédito settled in one cuota. The cuota
// carries the whole payable, which is what "no detracción" looks like.
func newCreditoInvoice() model.IssueRequest {
	req := newValidInvoice()
	req.FormaPago = "credito"
	req.Cuotas = []model.CuotaCredito{
		{Numero: 1, Monto: "1180.00", FechaVencimiento: "2024-02-15"},
	}
	return req
}

func TestValidatePaymentTerms(t *testing.T) {
	t.Run("cuotas adding up to the total pass when there is no detracción", func(t *testing.T) {
		errs := validation.Validate(newCreditoInvoice())
		is.True(t, !hasErrorField(errs, "cuotas"), "one cuota for the whole payable is valid")
	})

	// With detracción the cuotas settle the monto neto pendiente de pago: the
	// receptor deposits the 141.60 into the Banco de la Nación and owes the
	// emisor only 1038.40, so a cuota for the full 1180.00 is wrong.
	tests := []struct {
		name     string
		cuota    string
		rejected bool
	}{
		{name: "a cuota for the neto pendiente (total menos detracción) passes", cuota: "1038.40", rejected: false},
		{name: "a cuota for the full total is rejected when there is detracción", cuota: "1180.00", rejected: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := newCreditoInvoice()
			req.OperationType = "1001"
			req.Detraccion = newDetraccion()
			req.Cuotas = []model.CuotaCredito{
				{Numero: 1, Monto: test.cuota, FechaVencimiento: "2024-02-15"},
			}
			errs := validation.Validate(req)
			is.Equal(t, test.rejected, hasErrorField(errs, "cuotas"))
		})
	}

	t.Run("a USD comprobante con detracción converts the neto with its tipo de cambio", func(t *testing.T) {
		// 1180.00 USD with a 141.60 PEN detracción at 3.540 → 40.00 USD off the
		// payable, so 1140.00 USD is what the receptor still owes the emisor.
		req := newCreditoInvoice()
		req.CurrencyCode = "USD"
		req.ExchangeRate = "3.540"
		req.OperationType = "1001"
		req.Detraccion = newDetraccion()
		req.Cuotas = []model.CuotaCredito{
			{Numero: 1, Monto: "1140.00", FechaVencimiento: "2024-02-15"},
		}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "cuotas"), "the detracción converts at the declared rate")
		is.True(t, !hasErrorField(errs, "exchangeRate"), "a rate was declared")
	})

	// A venta de inmueble on a boleta: the seller is the obligated party
	// (R.S. 183-2004 art. 10.3 b.i), the buyer pays it the whole total, so the
	// neto pendiente is the full payable (R.S. 193-2020). On a factura the
	// buyer deposits and the detracción comes off as usual.
	t.Run("a boleta de inmueble a crédito settles the whole total", func(t *testing.T) {
		req := newCreditoInvoice()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		req.OperationType = "1001"
		req.Detraccion = newInmuebleDetraccion(1180) // 47.20
		is.Equal(t, 1180.0, req.NetoPendientePago())
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "cuotas"), "a cuota for the full 1180.00 is the neto")

		req.Cuotas[0].Monto = "1132.80"
		errs = validation.Validate(req)
		is.True(t, hasErrorField(errs, "cuotas"), "nothing comes off when the seller deposits")
	})

	t.Run("a factura de inmueble a crédito deducts the buyer's deposit", func(t *testing.T) {
		req := newCreditoInvoice()
		req.OperationType = "1001"
		req.Detraccion = newInmuebleDetraccion(1180)
		is.Equal(t, 1132.8, req.NetoPendientePago())
	})

	// An Anexo 1 bien under ½ UIT that applies only by the transport-unit rule
	// (R.S. 183-2004 art. 2.1): the proveedor carries and delivers it and
	// deposits (art. 5.1 b.1), so nothing comes off the neto.
	t.Run("an Anexo 1 bien subject by the transport unit settles the whole total", func(t *testing.T) {
		req := newCreditoInvoice()
		req.OperationType = "1001"
		req.Detraccion = &model.Detraccion{Codigo: sunat.Cat54AzucarYMelazaDeCana, Porcentaje: "10.00", Monto: "118.00", CuentaBN: "00-123-456789"}
		req.Anexo1UnidadTransporte = true
		is.Equal(t, 1180.0, req.NetoPendientePago())
	})

	t.Run("a foreign-currency detracción without tipo de cambio is rejected", func(t *testing.T) {
		req := newValidInvoice()
		req.CurrencyCode = "USD"
		req.OperationType = "1001"
		req.Detraccion = newDetraccion()
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "exchangeRate"), "soles cannot be netted off a USD total without a rate")
	})
}

// newReducidoInvoice is newValidInvoice taxed at the IGV reducido: 1000.00 + 105.00.
func newReducidoInvoice() model.IssueRequest {
	req := newValidInvoice()
	req.IGVReducido = true
	req.TotalIGV = "105.00"
	req.TotalAmount = "1105.00"
	req.TaxInclusiveAmount = "1105.00"
	req.Items[0].UnitPriceWithTax = "110.50"
	req.Items[0].IGVAmount = "105.00"
	return req
}

func TestValidateIGVReducido(t *testing.T) {
	t.Run("an issuer under the IGV reducido taxes at 10.5%", func(t *testing.T) {
		errs := validation.Validate(newReducidoInvoice())
		is.Equal(t, 0, len(errs))
	})

	t.Run("10.5% without the company setting is refused", func(t *testing.T) {
		req := newReducidoInvoice()
		req.IGVReducido = false
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "items[1].igvAmount"), "3103: an 18% issuer's line at 10.5%")
	})

	t.Run("an 18% line on a 10.5% document is refused", func(t *testing.T) {
		req := newReducidoInvoice()
		req.Items = append(req.Items, model.LineItem{
			LineNumber: 2, Description: "OTRO", Quantity: "1", UnitCode: "NIU",
			UnitPrice: "100.00", UnitPriceWithTax: "118.00", TaxExemptionReasonCode: "10",
			IGVAmount: "18.00", LineTotal: "100.00", PriceTypeCode: "01",
		})
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "items[2].igvAmount"), "3462: one IGV rate per document")
	})

	t.Run("a gravado gratuito at 18% on a 10.5% document is refused", func(t *testing.T) {
		req := newReducidoInvoice()
		req.Items = append(req.Items, model.LineItem{
			LineNumber: 2, Description: "CORTESIA", Quantity: "1", UnitCode: "NIU",
			UnitPrice: "0.00", UnitPriceWithTax: "11.80", TaxExemptionReasonCode: "11",
			IGVAmount: "1.80", LineTotal: "0.00", PriceTypeCode: "02",
		})
		errs := validation.Validate(req)
		is.True(t, hasErrorField(errs, "items[2].igvAmount"), "3462 compares the gravado gratuitos too")

		req.Items[1].UnitPriceWithTax = "11.05"
		req.Items[1].IGVAmount = "1.05"
		errs = validation.Validate(req)
		is.True(t, !hasErrorField(errs, "items[2].igvAmount"), "a gratuito at 10.5% matches the document")
	})

	t.Run("an anticipo of a 10.5% issuer is its base plus 10.5%", func(t *testing.T) {
		req := newReducidoInvoice()
		req.TotalAmount = "994.50"
		req.Anticipos = []model.Anticipo{{DocID: "F001-00000042", DocTypeCode: "02", TotalAmount: "110.50", BaseAmount: "100.00"}}
		errs := validation.Validate(req)
		is.True(t, !hasErrorField(errs, "anticipos[0].baseAmount"), "100 + 10.5% = 110.50")

		req.Anticipos[0].TotalAmount = "118.00"
		errs = validation.Validate(req)
		is.True(t, hasErrorField(errs, "anticipos[0].baseAmount"), "an 18% anticipo does not match a 10.5% issuer")
	})
}
