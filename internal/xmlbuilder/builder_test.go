package xmlbuilder_test

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
)

func newTestInvoice() model.IssueRequest {
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
		CustomerAddress:    "AV. CLIENTE 456",
		Subtotal:           "1000.00",
		TotalIGV:           "180.00",
		TotalISC:           "0.00",
		TotalOtherTaxes:    "0.00",
		TotalDiscount:      "0.00",
		TotalAmount:        "1180.00",
		TaxInclusiveAmount: "1180.00",
		Notes: []model.Note{
			{Code: "1000", Text: "MIL CIENTO OCHENTA CON 00/100 SOLES"},
		},
		Items: []model.LineItem{
			{
				LineNumber:             1,
				Description:            "PRODUCTO TEST",
				Quantity:               "10",
				UnitCode:               "NIU",
				UnitPrice:              "100.00",
				UnitPriceWithTax:       "118.00",
				TaxExemptionReasonCode: "10",
				IGVAmount:              "180.00",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "1000.00",
				PriceTypeCode:          "01",
			},
		},
	}
}

func TestBuildDocumentXML_Invoice(t *testing.T) {
	t.Run("should generate valid UBL 2.1 Invoice XML with ISO-8859-1 encoding", func(t *testing.T) {
		req := newTestInvoice()
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		xml := string(xmlBytes)

		// Verify XML declaration
		is.True(t, strings.HasPrefix(xml, `<?xml version="1.0" encoding="ISO-8859-1" standalone="no"?>`), "should have ISO-8859-1 declaration")

		// Verify root element and namespaces
		is.True(t, strings.Contains(xml, `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`), "should have Invoice root")
		is.True(t, strings.Contains(xml, `xmlns:cac="`), "should have cac namespace")
		is.True(t, strings.Contains(xml, `xmlns:cbc="`), "should have cbc namespace")
		is.True(t, strings.Contains(xml, `xmlns:ext="`), "should have ext namespace")
		is.True(t, strings.Contains(xml, `xmlns:ds="`), "should have ds namespace")

		// Verify UBL version
		is.True(t, strings.Contains(xml, `<cbc:UBLVersionID>2.1</cbc:UBLVersionID>`), "should have UBL 2.1")
		is.True(t, strings.Contains(xml, `<cbc:CustomizationID>2.0</cbc:CustomizationID>`), "should have CustomizationID 2.0")

		// Verify document ID
		is.True(t, strings.Contains(xml, `<cbc:ID>F001-00000001</cbc:ID>`), "should have document ID")

		// Verify dates
		is.True(t, strings.Contains(xml, `<cbc:IssueDate>2024-01-15</cbc:IssueDate>`), "should have issue date")
		is.True(t, strings.Contains(xml, `<cbc:IssueTime>15:20:30</cbc:IssueTime>`), "should have issue time")

		// Verify supplier RUC with scheme attributes
		is.True(t, strings.Contains(xml, `schemeID="6"`), "should have RUC scheme ID")
		is.True(t, strings.Contains(xml, `20100113612`), "should have supplier RUC")

		// Verify customer
		is.True(t, strings.Contains(xml, `20601327318`), "should have customer RUC")

		// Verify cac:Signature reference
		is.True(t, strings.Contains(xml, `<cbc:URI>#SignatureSP</cbc:URI>`), "should have signature URI reference")

		// Verify ext:UBLExtensions placeholder
		is.True(t, strings.Contains(xml, `<ext:UBLExtensions>`), "should have UBLExtensions")
		is.True(t, strings.Contains(xml, `<ext:ExtensionContent>`), "should have empty ExtensionContent")

		// Verify note
		is.True(t, strings.Contains(xml, `MIL CIENTO OCHENTA CON 00/100 SOLES`), "should have note text")

		// Verify line item
		is.True(t, strings.Contains(xml, `PRODUCTO TEST`), "should have item description")
		is.True(t, strings.Contains(xml, `<cac:InvoiceLine>`), "should have InvoiceLine element")
		is.True(t, strings.Contains(xml, `unitCode="NIU"`), "should have unit code")

		// Verify monetary amounts have currencyID
		is.True(t, strings.Contains(xml, `currencyID="PEN"`), "should have currency attribute")
	})
}

func TestBuildDocumentXML_Boleta(t *testing.T) {
	t.Run("should generate a boleta (03) for an anonymous consumidor final customer", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = model.IdentityDocTribNoRUC
		req.CustomerDocNumber = model.ConsumidorFinalDocNumber
		req.CustomerName = model.ConsumidorFinalName
		req.CustomerAddress = ""

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		xml := string(xmlBytes)

		// Boleta document type code (Cat.01 = 03).
		is.True(t, strings.Contains(xml, `>03</cbc:InvoiceTypeCode>`), "should carry boleta type code 03")
		is.True(t, strings.Contains(xml, `<cbc:ID>B001-00000001</cbc:ID>`), "should have boleta document ID")

		// Consumidor final customer: Cat.06 doc type 0, name CLIENTES VARIOS.
		is.True(t, strings.Contains(xml, `schemeID="0"`), "should have consumidor final scheme ID 0")
		is.True(t, strings.Contains(xml, model.ConsumidorFinalName), "should have consumidor final name")
	})
}

func TestBuildDocumentXML_PaymentTerms(t *testing.T) {
	t.Run("contado emits a single FormaPago/Contado entry", func(t *testing.T) {
		req := newTestInvoice()
		req.FormaPago = "contado"
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, strings.Contains(xml, `<cac:PaymentTerms><cbc:ID>FormaPago</cbc:ID><cbc:PaymentMeansID>Contado</cbc:PaymentMeansID></cac:PaymentTerms>`), "should emit single Contado entry")
		is.True(t, !strings.Contains(xml, `Cuota001`), "should not have any Cuota entries")
	})

	t.Run("credito emits Credito + one entry per cuota", func(t *testing.T) {
		req := newTestInvoice()
		req.FormaPago = "credito"
		req.Cuotas = []model.CuotaCredito{
			{Numero: 1, Monto: "590.00", FechaVencimiento: "2024-02-15"},
			{Numero: 2, Monto: "590.00", FechaVencimiento: "2024-03-15"},
		}
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Credito</cbc:PaymentMeansID>`), "should have Credito entry")
		// SUNAT err 3251: leading Credito entry must carry the net pending amount,
		// which without detracción is just TotalAmount.
		is.True(t, strings.Contains(xml, `<cac:PaymentTerms><cbc:ID>FormaPago</cbc:ID><cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="PEN">1180.00</cbc:Amount></cac:PaymentTerms>`), "Credito entry should include net pending Amount")
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Cuota001</cbc:PaymentMeansID>`), "should have Cuota001")
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Cuota002</cbc:PaymentMeansID>`), "should have Cuota002")
		is.True(t, strings.Contains(xml, `<cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate>`), "should have due date")
		is.True(t, strings.Contains(xml, `<cbc:Amount currencyID="PEN">590.00</cbc:Amount>`), "should have cuota amount with currency")
	})
}

func TestBuildDocumentXML_Detraccion(t *testing.T) {
	det := &model.Detraccion{
		Codigo:     "019",
		Porcentaje: "12.00",
		Monto:      "141.60", // 12% of 1180.00
		CuentaBN:   "00-123-456789",
	}

	assertDetraccion := func(t *testing.T, xml string) {
		t.Helper()
		// Legend text carries accents (valid ISO-8859-1 but not byte-equal to a
		// UTF-8 literal here), so assert only on the leyenda code.
		is.True(t, strings.Contains(xml, `<cbc:Note languageLocaleID="2006">`), "should emit leyenda 2006")
		is.True(t, strings.Contains(xml, `<cac:PaymentMeans><cbc:ID>Detraccion</cbc:ID><cbc:PaymentMeansCode>999</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>00-123-456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`), "should emit cuenta BN PaymentMeans")
		is.True(t, strings.Contains(xml, `<cac:PaymentTerms><cbc:ID>Detraccion</cbc:ID><cbc:PaymentMeansID>019</cbc:PaymentMeansID><cbc:PaymentPercent>12.00</cbc:PaymentPercent><cbc:Amount currencyID="PEN">141.60</cbc:Amount></cac:PaymentTerms>`), "should emit detracción PaymentTerms")
	}

	t.Run("factura sujeta a detracción emits PaymentMeans + PaymentTerms + leyenda 2006", func(t *testing.T) {
		req := newTestInvoice()
		req.OperationType = "1001"
		req.FormaPago = "contado"
		req.Detraccion = det
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		assertDetraccion(t, xml)
		// The forma de pago Contado entry must still be present alongside detracción.
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Contado</cbc:PaymentMeansID>`), "should keep Contado entry")
	})

	// Catálogo 51 marks 1001-1004 "Factura, Boletas", and the Boleta2_0 sheet of
	// SUNAT's validation pack carries the same detracción rules as Factura2_0.
	// buildInvoiceXML serves both doc types, so the markup must be identical bar
	// the InvoiceTypeCode value.
	t.Run("boleta sujeta a detracción emits the same detracción markup as a factura", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		req.OperationType = "1001"
		req.FormaPago = "contado"
		req.Detraccion = det
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		assertDetraccion(t, xml)
		is.True(t, strings.Contains(xml, `<cbc:InvoiceTypeCode listID="1001"`), "InvoiceTypeCode carries the detracción listID")
		is.True(t, strings.Contains(xml, `>03</cbc:InvoiceTypeCode>`), "still a boleta")
		// schemeName contains "Operación" — ISO-8859-1 encoded in the output, so
		// never byte-equal to a UTF-8 literal. Assert on the tail only.
		is.True(t, strings.Contains(xml, `catalogos:catalogo17">1001</cbc:ProfileID>`), "ProfileID carries 1001")
	})

	t.Run("nota de crédito de una factura sujeta a detracción mirrors the detracción", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "07"
		req.Series = "FC01"
		req.OperationType = "1001"
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.ReasonDescription = "Anulación de la operación"
		req.Detraccion = det
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		assertDetraccion(t, string(xmlBytes))
	})

	// The receptor deposits the detracción into the emisor's restricted BN cuenta
	// and owes only the rest on credit, so the monto neto pendiente de pago is the
	// payable MINUS the detracción and the cuotas settle that. 1180.00 − 141.60 =
	// 1038.40.
	t.Run("a crédito sujeto a detracción declares the neto pendiente, not the total", func(t *testing.T) {
		req := newTestInvoice()
		req.OperationType = "1001"
		req.Detraccion = det
		req.FormaPago = "credito"
		req.Cuotas = []model.CuotaCredito{
			{Numero: 1, Monto: "1038.40", FechaVencimiento: "2024-02-15"},
		}
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, strings.Contains(xml, `<cac:PaymentTerms><cbc:ID>FormaPago</cbc:ID><cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="PEN">1038.40</cbc:Amount></cac:PaymentTerms>`), "Credito Amount is the payable net of the detracción")
		is.True(t, !strings.Contains(xml, `<cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="PEN">1180.00</cbc:Amount>`), "the full payable is not what stays pending")
		assertDetraccion(t, xml)
	})

	// A USD document declares its detracción in soles, so the neto pendiente has
	// to convert back: 141.60 PEN ÷ 3.540 = 40.00 USD off a 1180.00 USD payable.
	t.Run("a USD crédito converts the detracción with its tipo de cambio", func(t *testing.T) {
		req := newTestInvoice()
		req.CurrencyCode = "USD"
		req.ExchangeRate = "3.540"
		req.OperationType = "1001"
		req.Detraccion = det
		req.FormaPago = "credito"
		req.Cuotas = []model.CuotaCredito{
			{Numero: 1, Monto: "1140.00", FechaVencimiento: "2024-02-15"},
		}
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="USD">1140.00</cbc:Amount>`), "neto pendiente is stated in the document's currency")
		is.True(t, strings.Contains(xml, `<cbc:Amount currencyID="PEN">141.60</cbc:Amount>`), "the detracción itself stays in soles")
	})

	t.Run("document without detracción emits no detracción markup", func(t *testing.T) {
		req := newTestInvoice()
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, !strings.Contains(xml, `<cbc:ID>Detraccion</cbc:ID>`), "should not emit detracción block")
		is.True(t, !strings.Contains(xml, `languageLocaleID="2006"`), "should not emit leyenda 2006")
	})
}

// Cat.54 código 027 declares tipo de operación 1004, which SUNAT only accepts
// with the trip block on cac:InvoiceLine/cac:Delivery. Every tag asserted here
// is an ERROR-level requirement (faults 3116-3126), not an observation.
func TestBuildDocumentXML_TransporteCarga(t *testing.T) {
	newReq := func() model.IssueRequest {
		req := newTestInvoice()
		req.OperationType = "1004"
		req.Detraccion = &model.Detraccion{
			Codigo: model.DetraccionTransporteCarga, Porcentaje: "4.00",
			Monto: "47.20", CuentaBN: "00-123-456789",
		}
		req.TransporteCarga = &model.TransporteCarga{
			OrigenUbigeo:                  "150101",
			OrigenDireccion:               "AV. ARGENTINA 1234",
			DestinoUbigeo:                 "040101",
			DestinoDireccion:              "AV. EJERCITO 500",
			DetalleViaje:                  "Lima - Arequipa",
			ValorReferencialServicio:      "1500.00",
			ValorReferencialCargaEfectiva: "1400.00",
			ValorReferencialCargaUtil:     "1600.00",
		}
		return req
	}

	t.Run("emits the full cac:Delivery trip block on the first line", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newReq())
		is.NotError(t, err)
		xml := string(xmlBytes)

		// UBL DeliveryType order: DeliveryLocation (destino), Despatch (origen,
		// with the detalle del viaje first), then the repeated DeliveryTerms.
		destino := `<cac:Delivery><cac:DeliveryLocation><cac:Address><cbc:ID schemeName="Ubigeos" schemeAgencyName="PE:INEI">040101</cbc:ID><cac:AddressLine><cbc:Line>AV. EJERCITO 500</cbc:Line></cac:AddressLine></cac:Address></cac:DeliveryLocation>`
		is.True(t, strings.Contains(xml, destino), "punto de destino: ubigeo + dirección (faults 3118/3119)")

		origen := `<cac:Despatch><cbc:Instructions>Lima - Arequipa</cbc:Instructions><cac:DespatchAddress><cbc:ID schemeName="Ubigeos" schemeAgencyName="PE:INEI">150101</cbc:ID><cac:AddressLine><cbc:Line>AV. ARGENTINA 1234</cbc:Line></cac:AddressLine></cac:DespatchAddress></cac:Despatch>`
		is.True(t, strings.Contains(xml, origen), "detalle del viaje + punto de origen (faults 3116/3117/3120)")

		terms := `<cac:DeliveryTerms><cbc:ID>01</cbc:ID><cbc:Amount currencyID="PEN">1500.00</cbc:Amount></cac:DeliveryTerms>` +
			`<cac:DeliveryTerms><cbc:ID>02</cbc:ID><cbc:Amount currencyID="PEN">1400.00</cbc:Amount></cac:DeliveryTerms>` +
			`<cac:DeliveryTerms><cbc:ID>03</cbc:ID><cbc:Amount currencyID="PEN">1600.00</cbc:Amount></cac:DeliveryTerms>`
		is.True(t, strings.Contains(xml, terms), "the three valores referenciales, in PEN (faults 3122-3126)")

		is.True(t, strings.Contains(xml, `listID="1004"`), "027 declares tipo de operación 1004, not 1002")
		is.True(t, strings.Contains(xml, `catalogos:catalogo17">1004</cbc:ProfileID>`), "ProfileID 1004")
	})

	t.Run("the trip block rides on the first line only", func(t *testing.T) {
		req := newReq()
		req.Items = append(req.Items, model.LineItem{
			LineNumber: 2, Description: "SEGUNDA LINEA", Quantity: "1",
			UnitCode: "NIU", UnitPrice: "100.00", UnitPriceWithTax: "118.00",
			TaxExemptionReasonCode: "10", IGVAmount: "18.00",
			LineTotal: "100.00", PriceTypeCode: "01",
		})
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		is.Equal(t, 1, strings.Count(string(xmlBytes), "<cac:Delivery>"))
	})

	t.Run("a comprobante without transporte de carga emits no Delivery block", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newTestInvoice())
		is.NotError(t, err)
		is.True(t, !strings.Contains(string(xmlBytes), "<cac:Delivery>"), "should not emit cac:Delivery")
	})
}

func TestBuildDocumentXML_CreditNote(t *testing.T) {
	t.Run("should generate valid UBL 2.1 CreditNote with discrepancy and billing reference", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "07"
		req.Series = "FC01"
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.ReasonDescription = "Anulación de la operación"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"`), "should have CreditNote root")
		is.True(t, strings.Contains(xml, `<cac:DiscrepancyResponse>`), "should have DiscrepancyResponse")
		is.True(t, strings.Contains(xml, `<cbc:ResponseCode>01</cbc:ResponseCode>`), "should have reason code")
		is.True(t, strings.Contains(xml, `<cac:BillingReference>`), "should have BillingReference")
		is.True(t, strings.Contains(xml, `<cac:CreditNoteLine>`), "should use CreditNoteLine")
		is.True(t, strings.Contains(xml, `<cbc:CreditedQuantity`), "should use CreditedQuantity")
	})
}

func TestBuildDocumentXML_DebitNote(t *testing.T) {
	t.Run("should generate valid UBL 2.1 DebitNote with debit-specific elements", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "08"
		req.Series = "FD01"
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "01"
		req.ReasonDescription = "Intereses por mora"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<DebitNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:DebitNote-2"`), "should have DebitNote root")
		is.True(t, strings.Contains(xml, `<cac:DebitNoteLine>`), "should use DebitNoteLine")
		is.True(t, strings.Contains(xml, `<cbc:DebitedQuantity`), "should use DebitedQuantity")
		// DebitNoteType requires cac:RequestedMonetaryTotal for the totals block,
		// NOT cac:LegalMonetaryTotal (which is what Invoice/CreditNote use). Emitting
		// the latter trips SUNAT fault soap-env:Client.0306 (cvc-particle 2.1).
		is.True(t, strings.Contains(xml, `<cac:RequestedMonetaryTotal>`), "should use RequestedMonetaryTotal")
		is.True(t, !strings.Contains(xml, `cac:LegalMonetaryTotal`), "must NOT use LegalMonetaryTotal on a DebitNote")
	})
}

func TestBuildDocumentXML_UnsupportedType(t *testing.T) {
	t.Run("should return error for unsupported document type", func(t *testing.T) {
		req := newTestInvoice()
		req.DocType = "99"

		_, err := xmlbuilder.BuildDocumentXML(req)
		is.True(t, err != nil, "should return error")
		is.True(t, strings.Contains(err.Error(), "unsupported"), "should mention unsupported type")
	})
}

func TestFilename(t *testing.T) {
	t.Run("should format filename per SUNAT spec", func(t *testing.T) {
		name := xmlbuilder.Filename("20100113612", "01", "F001", 1)
		is.Equal(t, "20100113612-01-F001-00000001", name)
	})
}

func TestBuildDocumentXML_IVAP(t *testing.T) {
	t.Run("IVAP line emits TaxScheme 1016 and 4% percent", func(t *testing.T) {
		req := newTestInvoice()
		req.Subtotal = "500.00"
		req.TotalIGV = "20.00"
		req.TotalAmount = "520.00"
		req.TaxInclusiveAmount = "520.00"
		req.Items = []model.LineItem{
			{
				LineNumber:             1,
				Description:            "ARROZ PILADO 50KG",
				Quantity:               "10",
				UnitCode:               "KGM",
				UnitPrice:              "50.00",
				UnitPriceWithTax:       "52.00",
				TaxExemptionReasonCode: "17",
				IGVAmount:              "20.00",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "500.00",
				PriceTypeCode:          "01",
			},
		}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// Document-level TaxSubtotal must reference IVAP scheme code 1016 at 4 %.
		is.True(t, strings.Contains(xml, "1016"), "should emit IVAP tax scheme code 1016")
		is.True(t, strings.Contains(xml, "<cbc:Percent>4.00</cbc:Percent>"), "should emit 4% percent for IVAP")
		// Line-level tax category should also carry IVAP scheme.
		is.True(t, strings.Contains(xml, `<cbc:Name>IVAP</cbc:Name>`), "should label tax scheme as IVAP")
	})
}

func TestBuildDocumentXML_ISC(t *testing.T) {
	t.Run("gravado-onerosa line with ISC: IGV TaxableAmount = LineTotal + ISC", func(t *testing.T) {
		req := newTestInvoice()
		req.Subtotal = "100.00"
		req.TotalISC = "20.00"
		req.TotalIGV = "21.60"
		req.TotalAmount = "141.60"
		req.TaxInclusiveAmount = "141.60"
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "PRODUCTO CON ISC",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "100.00",
			UnitPriceWithTax:       "141.60",
			TaxExemptionReasonCode: "10",
			IGVAmount:              "21.60",
			ISCAmount:              "20.00",
			DiscountAmount:         "0.00",
			LineTotal:              "100.00",
			PriceTypeCode:          "01",
			ISCTierRange:           "01",
		}}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// IGV TaxSubtotal must declare TaxableAmount = 120.00 (= 100 + 20 ISC).
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">120.00</cbc:TaxableAmount>`),
			"IGV TaxableAmount must include ISC: LineTotal + ISC = 120.00")
		is.True(t, strings.Contains(xml, `<cbc:TaxAmount currencyID="PEN">21.60</cbc:TaxAmount>`),
			"IGV TaxAmount must be 21.60")
		// ISC TaxSubtotal still uses bare valor_venta as its base.
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">100.00</cbc:TaxableAmount>`),
			"ISC TaxableAmount must remain 100.00 (valor_venta only)")
	})
}

func TestBuildDocumentXML_Gratuito(t *testing.T) {
	t.Run("mixed onerosa+gratuito invoice carries leyenda 1002 and gratuita tax scheme", func(t *testing.T) {
		req := newTestInvoice()
		// Onerosa line stays as in newTestInvoice (1000 base, 180 IGV, 1180 total).
		// Add a gratuito line whose referential value is 50 with declared IGV 9
		// (gravado-gratuito, code 11). It should NOT contribute to PayableAmount.
		req.Items = append(req.Items, model.LineItem{
			LineNumber:             2,
			Description:            "MUESTRA PROMOCIONAL",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "50.00",
			TaxExemptionReasonCode: "11",
			IGVAmount:              "9.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		})

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// Leyenda 1002 must be auto-injected.
		is.True(t, strings.Contains(xml, `languageLocaleID="1002"`), "should inject leyenda 1002")
		is.True(t, strings.Contains(xml, "TRANSFERENCIA GRATUITA"), "should carry leyenda text")

		// FreeOfChargeIndicator must NOT be emitted — SUNAT keys gratuito
		// behavior on TaxScheme/ID=9996, not on this indicator (greenter
		// reference: never emits it). Including it has caused validator drift.
		is.True(t, !strings.Contains(xml, "FreeOfChargeIndicator"),
			"should not emit FreeOfChargeIndicator on gratuito line")

		// Document must include a Gratuita TaxSubtotal (tax scheme 9996) at line level.
		is.True(t, strings.Contains(xml, "9996"), "should emit gratuita tax scheme 9996")

		// SUNAT only allows gratuita taxes at line level — the document-level
		// cac:TaxTotal must not include a Gratuita TaxScheme/ID=9996 subtotal.
		// (We assert the regular IGV subtotal is present and rely on the line
		//  level for the gratuita declaration.)
		is.True(t, strings.Contains(xml, `<cbc:ID schemeID="UN/ECE 5305" schemeAgencyID="6">S</cbc:ID>`),
			"should keep regular IGV subtotal at document level")
	})

	t.Run("gravado-gratuito line declares IGV at 18% with IGV-exclusive base (rules 3103 + 3111)", func(t *testing.T) {
		req := newTestInvoice()
		// Replace items with a single gravado-gratuito line: gross=118, IGV=18,
		// base must be 100 so that 100 × 18% = 18 satisfies SUNAT rule 3103,
		// and TaxAmount must be != 0 to satisfy rule 3111.
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "MUESTRA PROMOCIONAL",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "118.00",
			TaxExemptionReasonCode: "11",
			IGVAmount:              "18.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		}}
		req.Subtotal = "0.00"
		req.TotalIGV = "0.00"
		req.TotalISC = "0.00"
		req.TotalAmount = "0.00"
		req.TaxInclusiveAmount = "0.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// Line-level: TaxableAmount=100.00, TaxAmount=18.00, Percent=18.00.
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">100.00</cbc:TaxableAmount>`),
			"line TaxableAmount must be IGV-exclusive base 100.00")
		is.True(t, strings.Contains(xml, `<cbc:TaxAmount currencyID="PEN">18.00</cbc:TaxAmount>`),
			"line TaxAmount must be the would-be IGV 18.00 (rule 3111)")
		is.True(t, strings.Contains(xml, "<cbc:Percent>18.00</cbc:Percent>"),
			"gravado-gratuito must declare Percent=18.00 (rule 3103)")
	})

	t.Run("gravado-gratuito (11) emits PricingReference with PriceTypeCode=02 and AdditionalMonetaryTotal 1004 (SUNAT fault 2028)", func(t *testing.T) {
		req := newTestInvoice()
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "with_gratuito",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "118.00",
			TaxExemptionReasonCode: "11",
			IGVAmount:              "18.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		}}
		req.Subtotal = "0.00"
		req.TotalIGV = "0.00"
		req.TotalISC = "0.00"
		req.TotalAmount = "0.00"
		req.TaxInclusiveAmount = "0.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `xmlns:sac="urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1"`),
			"sac namespace must be declared on root")
		is.True(t, strings.Contains(xml, "<cac:AlternativeConditionPrice>"),
			"gratuito line must emit cac:AlternativeConditionPrice (SUNAT fault 2028)")
		// For gravado-gratuito the referencial (PriceTypeCode 02) is the
		// IGV-EXCLUSIVE per-unit base; SUNAT cross-checks it against the line
		// TaxableAmount (fault 3272). (118 − 18) / 1 = 100.00.
		is.True(t, strings.Contains(xml, `<cbc:PriceAmount currencyID="PEN">100.00</cbc:PriceAmount>`),
			"AlternativeConditionPrice must carry the IGV-exclusive referential base 100.00")
		is.True(t, strings.Contains(xml, ">02</cbc:PriceTypeCode>"),
			"PriceTypeCode must be 02 (referencial gratuita) on gratuito lines")
		is.True(t, strings.Contains(xml, "<sac:AdditionalMonetaryTotal>"),
			"document must emit sac:AdditionalMonetaryTotal when gratuito lines exist")
		is.True(t, strings.Contains(xml, "<cbc:ID>1004</cbc:ID>"),
			"AdditionalMonetaryTotal ID must be 1004 (total gratuito)")
		// SUNAT XSD requires sac:AdditionalMonetaryTotal to live inside
		// ext:UBLExtensions/ext:UBLExtension/ext:ExtensionContent/sac:AdditionalInformation,
		// NOT as a sibling of cac:LegalMonetaryTotal (fault soap-env:Client.0306,
		// "cvc-particle 2.1: ... next item should be cac:InvoiceLine").
		is.True(t, strings.Contains(xml, "<sac:AdditionalInformation>"),
			"AdditionalMonetaryTotal must be wrapped in sac:AdditionalInformation")
		amtIdx := strings.Index(xml, "<sac:AdditionalMonetaryTotal>")
		linesIdx := strings.Index(xml, "<cac:InvoiceLine>")
		extEnd := strings.Index(xml, "</ext:UBLExtensions>")
		is.True(t, amtIdx > 0 && extEnd > 0 && amtIdx < extEnd,
			"AdditionalMonetaryTotal must appear inside ext:UBLExtensions")
		is.True(t, linesIdx > extEnd,
			"InvoiceLine must come after ext:UBLExtensions, with no sac:AdditionalMonetaryTotal between LegalMonetaryTotal and InvoiceLine")
		// LegalMonetaryTotal/PayableAmount must remain 0 (the free amount is in 1004).
		is.True(t, strings.Contains(xml, "<cbc:PayableAmount currencyID=\"PEN\">100.00</cbc:PayableAmount>"),
			"AdditionalMonetaryTotal/PayableAmount must equal the IGV-exclusive referential base 100.00")
	})

	t.Run("exonerado-gratuito (21) aggregates UnitPriceWithTax×Qty into AdditionalMonetaryTotal 1004", func(t *testing.T) {
		req := newTestInvoice()
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "muestra exonerada",
			Quantity:               "2",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "50.00",
			TaxExemptionReasonCode: "21",
			IGVAmount:              "0.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		}}
		req.Subtotal = "0.00"
		req.TotalIGV = "0.00"
		req.TotalISC = "0.00"
		req.TotalAmount = "0.00"
		req.TaxInclusiveAmount = "0.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, "<cac:AlternativeConditionPrice>"),
			"exonerado-gratuito line must also emit AlternativeConditionPrice")
		is.True(t, strings.Contains(xml, "<sac:AdditionalMonetaryTotal>"),
			"AdditionalMonetaryTotal must be emitted for exonerado-gratuito")
		// 50 × 2 = 100, IGV=0, so total free base = 100.00.
		is.True(t, strings.Contains(xml,
			"<sac:AdditionalMonetaryTotal><cbc:ID>1004</cbc:ID><cbc:PayableAmount currencyID=\"PEN\">100.00</cbc:PayableAmount></sac:AdditionalMonetaryTotal>"),
			"AdditionalMonetaryTotal must total 100.00 (UnitPriceWithTax×Qty for exonerado)")
	})

	t.Run("inafecto-gratuito (31) aggregates UnitPriceWithTax×Qty into AdditionalMonetaryTotal 1004", func(t *testing.T) {
		req := newTestInvoice()
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "donacion inafecta",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "75.00",
			TaxExemptionReasonCode: "31",
			IGVAmount:              "0.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		}}
		req.Subtotal = "0.00"
		req.TotalIGV = "0.00"
		req.TotalISC = "0.00"
		req.TotalAmount = "0.00"
		req.TaxInclusiveAmount = "0.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml,
			"<sac:AdditionalMonetaryTotal><cbc:ID>1004</cbc:ID><cbc:PayableAmount currencyID=\"PEN\">75.00</cbc:PayableAmount></sac:AdditionalMonetaryTotal>"),
			"AdditionalMonetaryTotal must total 75.00 for inafecto-gratuito")
	})

	t.Run("mixed onerosa+gratuito: AdditionalMonetaryTotal contains only the gratuito base", func(t *testing.T) {
		req := newTestInvoice() // onerosa line: 1000 base, 180 IGV
		req.Items = append(req.Items, model.LineItem{
			LineNumber:             2,
			Description:            "MUESTRA",
			Quantity:               "1",
			UnitCode:               "NIU",
			UnitPrice:              "0.00",
			UnitPriceWithTax:       "59.00",
			TaxExemptionReasonCode: "11",
			IGVAmount:              "9.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "0.00",
			LineTotal:              "0.00",
			PriceTypeCode:          "02",
		})

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// Onerosa line still emits its own PriceTypeCode=01 PricingReference.
		is.True(t, strings.Contains(xml, ">01</cbc:PriceTypeCode>"),
			"onerosa line must keep PriceTypeCode=01")
		is.True(t, strings.Contains(xml, ">02</cbc:PriceTypeCode>"),
			"gratuito line must emit PriceTypeCode=02")
		// 59 - 9 = 50 from the gratuito line; onerosa contributes nothing.
		is.True(t, strings.Contains(xml,
			"<sac:AdditionalMonetaryTotal><cbc:ID>1004</cbc:ID><cbc:PayableAmount currencyID=\"PEN\">50.00</cbc:PayableAmount></sac:AdditionalMonetaryTotal>"),
			"AdditionalMonetaryTotal must aggregate only gratuito lines (50.00)")
		// LegalMonetaryTotal/PayableAmount stays at the onerosa total.
		is.True(t, strings.Contains(xml, "<cbc:PayableAmount currencyID=\"PEN\">1180.00</cbc:PayableAmount>"),
			"LegalMonetaryTotal/PayableAmount must reflect only the onerosa total")
	})

	t.Run("pure onerosa invoice does not emit AdditionalMonetaryTotal", func(t *testing.T) {
		req := newTestInvoice()
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, !strings.Contains(xml, "<sac:AdditionalMonetaryTotal"),
			"non-gratuito invoices must not emit sac:AdditionalMonetaryTotal")
	})
}

func TestBuildDocumentXML_LineDiscount(t *testing.T) {
	// Mirrors the real SUNAT fault 3271 case: a 100.00 unit price line with a
	// 50.00 descuento por ítem (net valor de venta 50.00, IGV 9.00, total 59.00).
	newDiscountInvoice := func() model.IssueRequest {
		req := newTestInvoice()
		req.Subtotal = "50.00"
		req.TotalIGV = "9.00"
		req.TotalDiscount = "50.00"
		req.TotalAmount = "59.00"
		req.TaxInclusiveAmount = "59.00"
		req.Notes = nil
		req.Items = []model.LineItem{{
			LineNumber:             1,
			Description:            "Contabilidad basica",
			Quantity:               "1.0",
			UnitCode:               "NIU",
			UnitPrice:              "100.00",
			UnitPriceWithTax:       "59.00",
			TaxExemptionReasonCode: "10",
			IGVAmount:              "9.00",
			ISCAmount:              "0.00",
			DiscountAmount:         "50.00",
			LineTotal:              "50.00",
			PriceTypeCode:          "01",
		}}
		return req
	}

	t.Run("emits a line-level cac:AllowanceCharge reconciling LineExtensionAmount with the gross price", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newDiscountInvoice())
		is.NotError(t, err)
		xml := string(xmlBytes)

		// SUNAT 3271 reconciles LineExtensionAmount = Price.PriceAmount × Qty − Amount.
		is.True(t, strings.Contains(xml, "<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>00</cbc:AllowanceChargeReasonCode><cbc:MultiplierFactorNumeric>0.50000</cbc:MultiplierFactorNumeric><cbc:Amount currencyID=\"PEN\">50.00</cbc:Amount><cbc:BaseAmount currencyID=\"PEN\">100.00</cbc:BaseAmount></cac:AllowanceCharge>"),
			"line discount must emit a Cat.53 \"00\" AllowanceCharge with the gross base")
		// Gross unit price stays in cac:Price; the discount lives in AllowanceCharge.
		is.True(t, strings.Contains(xml, "<cac:Price><cbc:PriceAmount currencyID=\"PEN\">100.00</cbc:PriceAmount></cac:Price>"),
			"cac:Price/PriceAmount must remain the gross valor unitario (100.00)")
		is.True(t, strings.Contains(xml, "<cbc:LineExtensionAmount currencyID=\"PEN\">50.00</cbc:LineExtensionAmount>"),
			"LineExtensionAmount must be the net line total (50.00)")
	})

	t.Run("does not double-count per-line discounts as a document AllowanceTotalAmount", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newDiscountInvoice())
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, !strings.Contains(xml, "<cbc:AllowanceTotalAmount"),
			"per-line discounts must not surface as a document-level AllowanceTotalAmount")
		// Total identity: TaxInclusiveAmount = LineExtensionAmount + TaxAmount = 59.00.
		is.True(t, strings.Contains(xml, "<cbc:TaxInclusiveAmount currencyID=\"PEN\">59.00</cbc:TaxInclusiveAmount>"),
			"TaxInclusiveAmount must stay 59.00")
	})

	// SUNAT's CreditNoteLine/DebitNoteLine model has NO line-level
	// cac:AllowanceCharge (unlike InvoiceLine): per the official guide (punto 29)
	// it computes the valor de venta del ítem as Quantity × cac:Price with the
	// descuento already deducted. A discounted note line must therefore bake the
	// discount into a NET cac:Price and omit the AllowanceCharge, or SUNAT ignores
	// the discount and rejects with fault 3271. Regression for the motive-05 NC.
	for _, tc := range []struct {
		name    string
		docType string
		series  string
		lineTag string
	}{
		{"credit note (07)", "07", "FC01", "cac:CreditNoteLine"},
		{"debit note (08)", "08", "FD01", "cac:DebitNoteLine"},
	} {
		t.Run(tc.name+" bakes the discount into a net cac:Price with no line AllowanceCharge", func(t *testing.T) {
			req := newDiscountInvoice()
			req.DocType = tc.docType
			req.Series = tc.series
			req.ReferenceDocType = "01"
			req.ReferenceDocSeries = "F001"
			req.ReferenceDocCorrelative = 1
			req.ReasonCode = "05"
			req.ReasonDescription = "Descuento por ítem"

			xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
			is.NotError(t, err)
			xml := string(xmlBytes)

			start := strings.Index(xml, "<"+tc.lineTag+">")
			is.True(t, start != -1, "should emit "+tc.lineTag)
			line := xml[start : strings.Index(xml, "</"+tc.lineTag+">")+len("</"+tc.lineTag+">")]

			is.True(t, !strings.Contains(line, "<cac:AllowanceCharge>"),
				"SUNAT note line must NOT carry a line-level cac:AllowanceCharge")
			// Net valor unitario: LineExtensionAmount(50.00) / Qty(1) = 50.00, so
			// SUNAT's Qty × Price == LineExtensionAmount reconciles.
			is.True(t, strings.Contains(line, "<cac:Price><cbc:PriceAmount currencyID=\"PEN\">50.00</cbc:PriceAmount></cac:Price>"),
				"cac:Price must be the NET valor unitario (50.00), discount baked in")
			is.True(t, strings.Contains(line, "<cbc:LineExtensionAmount currencyID=\"PEN\">50.00</cbc:LineExtensionAmount>"),
				"LineExtensionAmount must be the net line total (50.00)")
		})
	}
}

func TestBuildDocumentXML_GlobalDiscount(t *testing.T) {
	// Descuento global Cat.53 code 02 (afecta base imponible). One gravado line
	// of 1000 base; a 100 global discount reduces the gravado base to 900, so
	// IGV = 162. SUNAT factura guide: //Invoice/cac:AllowanceCharge + a
	// cbc:AllowanceTotalAmount, with the LegalMonetaryTotal LineExtensionAmount
	// reported NET (900) and PayableAmount = net LineExtension + IGV = 900 + 162
	// = 1062. (Lines keep their gross 1000; SUNAT reads the discount as the delta.)
	newGlobalDiscountInvoice := func(docType, series string) model.IssueRequest {
		req := newTestInvoice()
		req.DocType = docType
		req.Series = series
		req.Notes = nil
		req.Subtotal = "1000.00"
		req.TotalIGV = "162.00"
		req.GlobalDiscount = "100.00"
		req.TotalAmount = "1062.00"
		req.TaxInclusiveAmount = "1062.00"
		req.Items = []model.LineItem{{
			LineNumber: 1, Description: "Servicio", Quantity: "1", UnitCode: "NIU",
			UnitPrice: "1000.00", UnitPriceWithTax: "1180.00", TaxExemptionReasonCode: "10",
			IGVAmount: "180.00", ISCAmount: "0.00", DiscountAmount: "0.00",
			LineTotal: "1000.00", PriceTypeCode: "01",
		}}
		return req
	}

	for _, tc := range []struct{ name, docType, series string }{
		{"factura (01)", "01", "F001"},
		{"boleta (03)", "03", "B001"},
	} {
		t.Run(tc.name+" emits a document AllowanceCharge (code 02) and reduces the IGV base", func(t *testing.T) {
			xmlBytes, err := xmlbuilder.BuildDocumentXML(newGlobalDiscountInvoice(tc.docType, tc.series))
			is.NotError(t, err)
			xml := string(xmlBytes)

			is.True(t, strings.Contains(xml, "<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>02</cbc:AllowanceChargeReasonCode><cbc:MultiplierFactorNumeric>0.10000</cbc:MultiplierFactorNumeric><cbc:Amount currencyID=\"PEN\">100.00</cbc:Amount><cbc:BaseAmount currencyID=\"PEN\">1000.00</cbc:BaseAmount></cac:AllowanceCharge>"),
				"must emit a document-level Cat.53 '02' AllowanceCharge with the gravado base")
			// IGV base reduced: 1000 − 100 = 900, IGV 162.
			is.True(t, strings.Contains(xml, "<cbc:TaxableAmount currencyID=\"PEN\">900.00</cbc:TaxableAmount><cbc:TaxAmount currencyID=\"PEN\">162.00</cbc:TaxAmount>"),
				"gravado TaxSubtotal must reduce the base to 900 with IGV 162")
			// SUNAT fault 3300: a Cat.53 '02' (afecta base) discount must NOT be
			// reported in cbc:AllowanceTotalAmount — that node carries only NON-afecta
			// (01/03) discounts, so it stays absent. The discount is realised by the
			// reduced base above and the NET LineExtensionAmount (1000 − 100 = 900).
			is.True(t, !strings.Contains(xml, "<cbc:AllowanceTotalAmount"),
				"an afecta-base global discount must NOT emit AllowanceTotalAmount (fault 3300)")
			is.True(t, strings.Contains(xml, "<cbc:LineExtensionAmount currencyID=\"PEN\">900.00</cbc:LineExtensionAmount><cbc:TaxInclusiveAmount currencyID=\"PEN\">1062.00</cbc:TaxInclusiveAmount>"),
				"LineExtensionAmount must be net of the global discount (900); TaxInclusive = 900 + 162 = 1062")
		})
	}

	t.Run("nota de crédito (07) motivo 04 bakes the discount into the line, no doc AllowanceCharge", func(t *testing.T) {
		// NC con descuento global (motivo 04): SUNAT does NOT honour a doc-level
		// cac:AllowanceCharge on a CreditNote — it reconciles Σ line valor de venta
		// against the document gravado total (fault 3277). So the 100 discount is
		// baked into the line (1000 → 900) and there is NO document AllowanceCharge.
		req := newGlobalDiscountInvoice("07", "FC01")
		req.ReferenceDocType = "01"
		req.ReferenceDocSeries = "F001"
		req.ReferenceDocCorrelative = 1
		req.ReasonCode = "04"
		req.ReasonDescription = "Descuento global"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"`), "should be a CreditNote")
		is.True(t, strings.Contains(xml, `<cac:CreditNoteLine>`), "should keep the referenced line(s)")
		// The discount lives in the lines, never as a document-level AllowanceCharge.
		is.True(t, !strings.Contains(xml, "<cac:AllowanceCharge>"),
			"a CreditNote must NOT emit a document-level AllowanceCharge (SUNAT ignores it → fault 3277)")
		// Line valor de venta is net (900) and matches the document gravado total
		// (900/162) so Σ line == total — the cure for fault 3277.
		is.True(t, strings.Contains(xml, "<cbc:CreditedQuantity unitCode=\"NIU\">1</cbc:CreditedQuantity><cbc:LineExtensionAmount currencyID=\"PEN\">900.00</cbc:LineExtensionAmount>"),
			"line valor de venta must be net of the global discount (900)")
		is.True(t, strings.Contains(xml, "<cbc:TaxableAmount currencyID=\"PEN\">900.00</cbc:TaxableAmount><cbc:TaxAmount currencyID=\"PEN\">162.00</cbc:TaxAmount>"),
			"gravado TaxSubtotal must be the reduced base 900 with IGV 162")
		is.True(t, strings.Contains(xml, "<cbc:LineExtensionAmount currencyID=\"PEN\">900.00</cbc:LineExtensionAmount><cbc:TaxInclusiveAmount currencyID=\"PEN\">1062.00</cbc:TaxInclusiveAmount>"),
			"document LineExtensionAmount must be net (900); TaxInclusive = 900 + 162 = 1062")
	})

	t.Run("no AllowanceCharge or AllowanceTotalAmount when there is no global discount", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newTestInvoice())
		is.NotError(t, err)
		xml := string(xmlBytes)
		is.True(t, !strings.Contains(xml, "<cbc:AllowanceTotalAmount"),
			"a document without a global discount must not emit AllowanceTotalAmount")
	})
}

func TestBuildDocumentXML_Anticipos(t *testing.T) {
	// Factura de regularización: the sale is declared IN FULL in the totales
	// (LineExtension 1000.00, TaxInclusive 1180.00) and the prior anticipo of
	// 118.00 comes off the payable once, as cbc:PrepaidAmount → PayableAmount
	// 1062.00. Its base (100.00) additionally reduces the gravado TaxSubtotal
	// through a Cat.53 "04" AllowanceCharge, because the factura de anticipo
	// already declared the IGV of that 100.
	//
	// Both halves are SUNAT-enforced: deducting the anticipo from the base AND
	// from the payable total is fault 3280, dropping the "04" allowance is
	// fault 3287.
	newRegularizacion := func() model.IssueRequest {
		req := newTestInvoice()
		req.Notes = nil
		req.TotalAmount = "1062.00"
		req.Anticipos = []model.Anticipo{{
			DocID: "F001-00000042", DocTypeCode: "02",
			TotalAmount: "118.00", BaseAmount: "100.00",
		}}
		return req
	}

	t.Run("declares the sale in full, taxes only the remainder and deducts the anticipo as PrepaidAmount", func(t *testing.T) {
		xmlBytes, err := xmlbuilder.BuildDocumentXML(newRegularizacion())
		is.NotError(t, err)
		xml := string(xmlBytes)

		ref := `<cac:AdditionalDocumentReference><cbc:ID>F001-00000042</cbc:ID><cbc:DocumentTypeCode>02</cbc:DocumentTypeCode><cbc:DocumentStatusCode>1</cbc:DocumentStatusCode><cac:IssuerParty><cac:PartyIdentification><cbc:ID schemeID="6">20100113612</cbc:ID></cac:PartyIdentification></cac:IssuerParty></cac:AdditionalDocumentReference>`
		is.True(t, strings.Contains(xml, ref),
			"must reference the anticipo doc with Cat.12 type 02, row 1 and the emitter RUC")
		is.True(t, strings.Index(xml, "<cac:AdditionalDocumentReference>") < strings.Index(xml, "<cac:Signature>"),
			"UBL order: AdditionalDocumentReference must precede cac:Signature")

		prepaid := `<cac:PrepaidPayment><cbc:ID schemeName="Anticipo" schemeAgencyName="PE:SUNAT">1</cbc:ID><cbc:PaidAmount currencyID="PEN">118.00</cbc:PaidAmount></cac:PrepaidPayment>`
		is.True(t, strings.Contains(xml, prepaid),
			"must emit cac:PrepaidPayment row 1 with the IGV-inclusive monto anticipado")
		is.True(t, strings.Index(xml, "<cac:PrepaidPayment>") > strings.Index(xml, "</cac:PaymentTerms>"),
			"UBL order: PrepaidPayment must follow cac:PaymentTerms")
		// Fault 3287 regression guard: a declared PrepaidAmount needs its
		// Cat.53 "04" allowance, carrying the anticipo base and nothing else.
		allowance := `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>04</cbc:AllowanceChargeReasonCode><cbc:Amount currencyID="PEN">100.00</cbc:Amount></cac:AllowanceCharge>`
		is.True(t, strings.Contains(xml, allowance),
			"must emit a Cat.53 '04' AllowanceCharge with the anticipo base, no BaseAmount/MultiplierFactor (fault 3287)")
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">900.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="PEN">162.00</cbc:TaxAmount>`),
			"gravado TaxSubtotal must net out the anticipo base (1000 − 100), IGV recomputed at 18%")
		is.True(t, strings.Contains(xml, `<cbc:LineExtensionAmount currencyID="PEN">1000.00</cbc:LineExtensionAmount><cbc:TaxInclusiveAmount currencyID="PEN">1180.00</cbc:TaxInclusiveAmount><cbc:PrepaidAmount currencyID="PEN">118.00</cbc:PrepaidAmount><cbc:PayableAmount currencyID="PEN">1062.00</cbc:PayableAmount>`),
			"LegalMonetaryTotal: gross LineExtension 1000, TaxInclusive 1180 (the full sale, not LineExtension + the reduced TaxAmount), Prepaid 118, Payable = 1180 − 118")
		is.True(t, !strings.Contains(xml, "<cbc:AllowanceTotalAmount"),
			"no global discount here, so no AllowanceTotalAmount (fault 3300)")
	})

	// The Boleta2_0 sheet of SUNAT's validation pack carries the same anticipo
	// rules as Factura2_0, fault 3287 included, and Cat.12 code 03 is
	// "Boleta de Venta – emitida por anticipos". buildInvoiceXML serves both.
	t.Run("boleta de regularización emits the same anticipo markup with Cat.12 code 03", func(t *testing.T) {
		req := newRegularizacion()
		req.DocType = "03"
		req.Series = "B001"
		req.CustomerDocType = "1"
		req.CustomerDocNumber = "12345678"
		req.Anticipos = []model.Anticipo{{
			DocID: "B001-00000007", DocTypeCode: "03",
			TotalAmount: "118.00", BaseAmount: "100.00",
		}}
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		ref := `<cac:AdditionalDocumentReference><cbc:ID>B001-00000007</cbc:ID><cbc:DocumentTypeCode>03</cbc:DocumentTypeCode><cbc:DocumentStatusCode>1</cbc:DocumentStatusCode><cac:IssuerParty><cac:PartyIdentification><cbc:ID schemeID="6">20100113612</cbc:ID></cac:PartyIdentification></cac:IssuerParty></cac:AdditionalDocumentReference>`
		is.True(t, strings.Contains(xml, ref),
			"must reference the boleta de anticipo with Cat.12 type 03")
		prepaid := `<cac:PrepaidPayment><cbc:ID schemeName="Anticipo" schemeAgencyName="PE:SUNAT">1</cbc:ID><cbc:PaidAmount currencyID="PEN">118.00</cbc:PaidAmount></cac:PrepaidPayment>`
		is.True(t, strings.Contains(xml, prepaid), "must emit cac:PrepaidPayment on a boleta too")
		// Fault 3287 applies to boletas exactly as it does to facturas.
		allowance := `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>04</cbc:AllowanceChargeReasonCode><cbc:Amount currencyID="PEN">100.00</cbc:Amount></cac:AllowanceCharge>`
		is.True(t, strings.Contains(xml, allowance),
			"must emit the Cat.53 '04' allowance on a boleta (fault 3287)")
		is.True(t, strings.Contains(xml, `<cbc:PrepaidAmount currencyID="PEN">118.00</cbc:PrepaidAmount><cbc:PayableAmount currencyID="PEN">1062.00</cbc:PayableAmount>`),
			"hybrid totals hold on a boleta")
		is.True(t, strings.Contains(xml, `>03</cbc:InvoiceTypeCode>`), "still a boleta")
	})

	t.Run("two anticipos are row-paired 1/2 and their totals summed", func(t *testing.T) {
		req := newRegularizacion()
		req.Anticipos = append(req.Anticipos, model.Anticipo{
			DocID: "B002-00000007", DocTypeCode: "03",
			TotalAmount: "236.00", BaseAmount: "200.00",
		})
		// Both bases come off the taxable (1000 − 300 = 700, IGV 126); the
		// totales stay gross and only the payable moves: 1180 − 354.
		req.TotalAmount = "826.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<cbc:ID>B002-00000007</cbc:ID><cbc:DocumentTypeCode>03</cbc:DocumentTypeCode><cbc:DocumentStatusCode>2</cbc:DocumentStatusCode>`),
			"second anticipo must be row 2 with Cat.12 type 03")
		is.True(t, strings.Contains(xml, `<cbc:ID schemeName="Anticipo" schemeAgencyName="PE:SUNAT">2</cbc:ID><cbc:PaidAmount currencyID="PEN">236.00</cbc:PaidAmount>`),
			"second PrepaidPayment must be row 2 with its own monto")
		is.True(t, strings.Count(xml, "<cbc:AllowanceChargeReasonCode>04</cbc:AllowanceChargeReasonCode>") == 2,
			"one Cat.53 '04' allowance per anticipo")
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">700.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="PEN">126.00</cbc:TaxAmount>`),
			"gravado base must net out both anticipo bases (1000 − 100 − 200)")
		is.True(t, strings.Contains(xml, `<cbc:LineExtensionAmount currencyID="PEN">1000.00</cbc:LineExtensionAmount><cbc:TaxInclusiveAmount currencyID="PEN">1180.00</cbc:TaxInclusiveAmount>`),
			"the totales stay the full sale however many anticipos are applied")
		is.True(t, strings.Contains(xml, `<cbc:PrepaidAmount currencyID="PEN">354.00</cbc:PrepaidAmount><cbc:PayableAmount currencyID="PEN">826.00</cbc:PayableAmount>`),
			"PrepaidAmount must be the sum of both anticipos con IGV (118 + 236), Payable = 1180 − 354")
	})

	t.Run("a descuento global reduces the totales too; the anticipo only the taxable and the payable", func(t *testing.T) {
		req := newRegularizacion()
		req.GlobalDiscount = "100.00"
		// The 02 discount moves the totales as well: valor venta 1000 − 100 =
		// 900, IGV 162, TaxInclusive 1062, and the anticipo comes off that:
		// 1062 − 118. The taxable drops by both (1000 − 100 − 100 = 800).
		req.TotalIGV = "162.00"
		req.TaxInclusiveAmount = "1062.00"
		req.TotalAmount = "944.00"

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, "<cbc:AllowanceChargeReasonCode>02</cbc:AllowanceChargeReasonCode>"),
			"must keep the descuento global 02 entry")
		is.True(t, strings.Contains(xml, "<cbc:AllowanceChargeReasonCode>04</cbc:AllowanceChargeReasonCode>"),
			"and the anticipo 04 entry alongside it")
		is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">800.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="PEN">144.00</cbc:TaxAmount>`),
			"gravado base must net out both the discount and the anticipo base (1000 − 100 − 100)")
		is.True(t, strings.Contains(xml, `<cbc:LineExtensionAmount currencyID="PEN">900.00</cbc:LineExtensionAmount><cbc:TaxInclusiveAmount currencyID="PEN">1062.00</cbc:TaxInclusiveAmount><cbc:PrepaidAmount currencyID="PEN">118.00</cbc:PrepaidAmount><cbc:PayableAmount currencyID="PEN">944.00</cbc:PayableAmount>`),
			"LegalMonetaryTotal: discount in the base, anticipo in Prepaid, Payable = 1062 − 118")
	})

	t.Run("crédito regularización: the leading Credito entry carries the net pending amount", func(t *testing.T) {
		req := newRegularizacion()
		req.FormaPago = "credito"
		req.Cuotas = []model.CuotaCredito{{Numero: 1, Monto: "1062.00", FechaVencimiento: "2024-02-15"}}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<cbc:ID>FormaPago</cbc:ID><cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="PEN">1062.00</cbc:Amount>`),
			"the Credito net pending (err 3251) must be the total NET of anticipos")
	})

	// SUNAT retired 0104 from catálogo 51 and rejects it with fault 3206, so the
	// internal anticipo marker must never reach the wire — it degrades to 0101.
	t.Run("a factura de anticipo emits 0101, never the retired 0104", func(t *testing.T) {
		req := newTestInvoice()
		req.OperationType = model.OpAnticipos

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, !strings.Contains(xml, "0104"), "the retired 0104 must not appear anywhere in the XML")
		is.True(t, strings.Contains(xml, ">0101</cbc:ProfileID>"), "cbc:ProfileID must fall back to 0101")
		is.True(t, strings.Contains(xml, `listID="0101"`), "InvoiceTypeCode listID must fall back to 0101")
		is.True(t, !strings.Contains(xml, "<cac:PrepaidPayment>"),
			"an anticipo factura itself deducts nothing — no PrepaidPayment")
	})

	t.Run("a regularización factura keeps its own operation type on the wire", func(t *testing.T) {
		req := newTestInvoice()
		req.OperationType = model.OpVentaInterna
		req.Anticipos = []model.Anticipo{
			{DocID: "F001-00000042", DocTypeCode: "02", TotalAmount: "118.00", BaseAmount: "100.00"},
		}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `listID="0101"`), "InvoiceTypeCode listID must stay 0101")
		is.True(t, strings.Contains(xml, "<cac:PrepaidPayment>"), "the regularización must declare its anticipos")
	})
}

// TestBuildDocumentXML_AnticipoConDetraccion covers the tipo de operación an
// anticipo declares once SPOT enters the picture. The 0104 marker still never
// reaches the wire, but a factura de anticipo sujeta a detracción goes out as
// an operación sujeta a detracción (1001/1002) rather than a venta interna,
// because that is the code SUNAT pairs with the leyenda 2006 + cuenta BN block.
func TestBuildDocumentXML_AnticipoConDetraccion(t *testing.T) {
	// The anticipo collects 1180.00 and deposits 12% of exactly that — SPOT is
	// computed on the comprobante documenting the payment, not on the eventual
	// operación.
	newDet := func(codigo string) *model.Detraccion {
		return &model.Detraccion{Codigo: codigo, Porcentaje: "12.00", Monto: "141.60", CuentaBN: "00-123-456789"}
	}

	tests := []struct {
		name          string
		operationType string
		detraccion    *model.Detraccion
		expected      string
	}{
		{name: "an anticipo without detracción stays a venta interna", operationType: model.OpAnticipos, detraccion: nil, expected: "0101"},
		{name: "an anticipo sujeto a detracción declares 1001", operationType: model.OpAnticipos, detraccion: newDet("019"), expected: "1001"},
		{name: "a plain factura sujeta a detracción is unaffected", operationType: model.OpDetraccion, detraccion: newDet("019"), expected: "1001"},
		// The Cat.54 → Cat.51 pairings SUNAT enforces with fault 3129.
		{name: "recursos hidrobiológicos (004) declares 1002", operationType: model.OpDetraccion, detraccion: newDet(model.DetraccionHidrobiologicos), expected: "1002"},
		{name: "transporte de pasajeros (028) declares 1003", operationType: model.OpDetraccion, detraccion: newDet(model.DetraccionTransportePasaj), expected: "1003"},
		{name: "transporte de carga (027) declares 1004, not 1002", operationType: model.OpDetraccion, detraccion: newDet(model.DetraccionTransporteCarga), expected: "1004"},
		{name: "an anticipo de transporte de carga (027) declares 1004", operationType: model.OpAnticipos, detraccion: newDet(model.DetraccionTransporteCarga), expected: "1004"},
		{name: "an explicit 1004 is unaffected", operationType: model.OpDetraccionTransporteCarga, detraccion: newDet(model.DetraccionTransporteCarga), expected: "1004"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := newTestInvoice()
			req.OperationType = test.operationType
			req.Detraccion = test.detraccion

			xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
			is.NotError(t, err)
			xml := string(xmlBytes)

			is.True(t, strings.Contains(xml, ">"+test.expected+"</cbc:ProfileID>"), "cbc:ProfileID must be "+test.expected)
			is.True(t, strings.Contains(xml, `listID="`+test.expected+`"`), "InvoiceTypeCode listID must be "+test.expected)
			is.True(t, !strings.Contains(xml, "0104"), "the retired 0104 must never reach the wire")
		})
	}

	t.Run("an anticipo sujeto a detracción emits the full SPOT block", func(t *testing.T) {
		req := newTestInvoice()
		req.OperationType = model.OpAnticipos
		req.Detraccion = newDet("019")

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		is.True(t, strings.Contains(xml, `<cbc:Note languageLocaleID="2006">`), "should emit leyenda 2006")
		is.True(t, strings.Contains(xml, `<cac:PaymentMeans><cbc:ID>Detraccion</cbc:ID><cbc:PaymentMeansCode>999</cbc:PaymentMeansCode><cac:PayeeFinancialAccount><cbc:ID>00-123-456789</cbc:ID></cac:PayeeFinancialAccount></cac:PaymentMeans>`), "should emit cuenta BN PaymentMeans")
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>019</cbc:PaymentMeansID><cbc:PaymentPercent>12.00</cbc:PaymentPercent><cbc:Amount currencyID="PEN">141.60</cbc:Amount>`), "should emit detracción PaymentTerms")
	})

	// The hybrid amount model (gross totales + one Cat.53 "04" AllowanceCharge
	// per anticipo) is what SUNAT accepted; the detracción block rides alongside
	// it without disturbing any of those figures.
	t.Run("a regularización sujeta a detracción keeps the hybrid anticipo shape", func(t *testing.T) {
		req := newTestInvoice()
		req.Notes = nil
		req.OperationType = model.OpDetraccion
		req.TotalAmount = "1062.00" // 1180.00 sale − 118.00 already collected
		req.Anticipos = []model.Anticipo{{
			DocID: "F001-00000042", DocTypeCode: "02",
			TotalAmount: "118.00", BaseAmount: "100.00",
		}}
		// 12% of the 1062.00 payable — the anticipo deposited its own 14.16, so
		// together they come to 141.60 = 12% of the 1180.00 operación.
		req.Detraccion = &model.Detraccion{Codigo: "019", Porcentaje: "12.00", Monto: "127.44", CuentaBN: "00-123-456789"}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// The sale is still declared in full, reduced only by the Cat.53 "04"
		// allowance, and the anticipo still comes off exactly once as Prepaid.
		is.True(t, strings.Contains(xml, `<cac:AllowanceCharge><cbc:ChargeIndicator>false</cbc:ChargeIndicator><cbc:AllowanceChargeReasonCode>04</cbc:AllowanceChargeReasonCode><cbc:Amount currencyID="PEN">100.00</cbc:Amount></cac:AllowanceCharge>`), "should keep the Cat.53 04 allowance")
		is.True(t, strings.Contains(xml, `<cbc:TaxInclusiveAmount currencyID="PEN">1180.00</cbc:TaxInclusiveAmount>`), "TaxInclusiveAmount stays gross")
		is.True(t, strings.Contains(xml, `<cbc:PrepaidAmount currencyID="PEN">118.00</cbc:PrepaidAmount>`), "should deduct the anticipo as PrepaidAmount")
		is.True(t, strings.Contains(xml, `<cbc:PayableAmount currencyID="PEN">1062.00</cbc:PayableAmount>`), "PayableAmount is the saldo")
		// …and the SPOT sits on that saldo.
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>019</cbc:PaymentMeansID><cbc:PaymentPercent>12.00</cbc:PaymentPercent><cbc:Amount currencyID="PEN">127.44</cbc:Amount>`), "detracción is 12% of the payable")
	})
}

func TestBuildDocumentXML_GravadoGratuitoReferential(t *testing.T) {
	// SUNAT fault 3272: for gravado-gratuito (Cat.07 codes 11-16) the
	// cac:PricingReference referencial (PriceTypeCode 02) is the IGV-EXCLUSIVE
	// per-unit base and must equal cac:TaxSubtotal/cbc:TaxableAmount. Mirrors the
	// real failing payload: gross 118.00, IGV 18.00, base 100.00, qty 1.
	for _, code := range []string{"11", "12", "13", "14", "15", "16"} {
		t.Run("code "+code+": referencial equals the IGV-exclusive base (100.00), not the gross price (118.00)", func(t *testing.T) {
			req := newTestInvoice()
			req.Items = []model.LineItem{{
				LineNumber:             1,
				Description:            "Contabilidad",
				Quantity:               "1.0",
				UnitCode:               "ZZ",
				UnitPrice:              "0.00",
				UnitPriceWithTax:       "118.00",
				TaxExemptionReasonCode: code,
				IGVAmount:              "18.00",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "0.00",
				PriceTypeCode:          "02",
			}}
			req.Subtotal = "0.00"
			req.TotalIGV = "0.00"
			req.TotalISC = "0.00"
			req.TotalAmount = "0.00"
			req.TaxInclusiveAmount = "0.00"
			req.Notes = nil

			xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
			is.NotError(t, err)
			xml := string(xmlBytes)

			is.True(t, strings.Contains(xml, `<cbc:PriceAmount currencyID="PEN">100.00</cbc:PriceAmount>`),
				"PricingReference must carry the IGV-exclusive referential base 100.00")
			is.True(t, strings.Contains(xml, `<cbc:TaxableAmount currencyID="PEN">100.00</cbc:TaxableAmount>`),
				"line TaxableAmount must be 100.00, matching the referencial")
			// The IGV-inclusive price must no longer leak into the referencial.
			is.True(t, !strings.Contains(xml, `>118.00</cbc:PriceAmount>`),
				"the IGV-inclusive 118.00 must not appear as a referencial price")

			// SUNAT fault 3272: the line value (cbc:LineExtensionAmount) must
			// equal the base imponible — greenter emits both as the referential.
			// The line value must equal the base imponible (greenter emits both
			// as the referential). The document LegalMonetaryTotal still carries
			// LineExtensionAmount 0.00 — the customer pays nothing.
			is.True(t, strings.Contains(xml, `<cbc:LineExtensionAmount currencyID="PEN">100.00</cbc:LineExtensionAmount>`),
				"line LineExtensionAmount must equal the referential base 100.00, not 0.00")

			// The document must consign the gratuita under the GRA (9996) scheme
			// so SUNAT can reconcile it with the line, while the document
			// cbc:TaxAmount stays 0.00 (no tax is payable on gratuitas).
			is.True(t, strings.Contains(xml, ">9996</cbc:ID>"),
				"document TaxTotal must include a 9996 (GRA) gratuita subtotal")
			is.True(t, strings.Contains(xml, `<cac:TaxTotal><cbc:TaxAmount currencyID="PEN">0.00</cbc:TaxAmount>`),
				"document-level TaxAmount must remain 0.00 for a pure-gratuito invoice")
		})
	}
}

func TestBuildDocumentXML_ExoneradoInafectoGratuito(t *testing.T) {
	// SUNAT fault 3224: a line with a referential price > 0 (PriceTypeCode 02)
	// must be declared under tributo 9996 at BOTH line and document level —
	// otherwise the document's fallback IGV (1000) subtotal makes the operation
	// look onerosa. Covers exonerado-gratuito (21) and inafecto-gratuito (31-37),
	// which carry a referential base but zero IGV.
	for _, code := range []string{"21", "31", "32", "33", "34", "35", "36", "37"} {
		t.Run("code "+code+": referencial>0 is declared as gratuita 9996 with no IGV fallback subtotal", func(t *testing.T) {
			req := newTestInvoice()
			req.Items = []model.LineItem{{
				LineNumber:             1,
				Description:            "Muestra",
				Quantity:               "1.0",
				UnitCode:               "ZZ",
				UnitPrice:              "0.00",
				UnitPriceWithTax:       "100.00",
				TaxExemptionReasonCode: code,
				IGVAmount:              "0.00",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "0.00",
				PriceTypeCode:          "02",
			}}
			req.Subtotal = "0.00"
			req.TotalIGV = "0.00"
			req.TotalISC = "0.00"
			req.TotalAmount = "0.00"
			req.TaxInclusiveAmount = "0.00"
			req.Notes = nil

			xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
			is.NotError(t, err)
			xml := string(xmlBytes)

			// Line: value == base imponience == referential, scheme 9996.
			is.True(t, strings.Contains(xml, `<cbc:LineExtensionAmount currencyID="PEN">100.00</cbc:LineExtensionAmount>`),
				"line LineExtensionAmount must equal the referential base 100.00")
			is.True(t, strings.Contains(xml, ">9996</cbc:ID>"),
				"line/doc tributo must be 9996 (GRA) when a referential price > 0 exists")
			// Document must NOT fall back to an IGV (1000) subtotal — that is what
			// triggers fault 3224 for gratuitas.
			is.True(t, !strings.Contains(xml, ">1000</cbc:ID>"),
				"a pure-gratuito document must not emit a 1000 (IGV) tax subtotal")
			// The rate tag must be present (fault 2992) but zero — never 18%.
			is.True(t, strings.Contains(xml, "<cbc:Percent>0.00</cbc:Percent>"),
				"the 9996 tributo must carry a 0.00 rate tag (SUNAT fault 2992)")
			is.True(t, !strings.Contains(xml, "<cbc:Percent>18.00</cbc:Percent>"),
				"exonerado/inafecto gratuito must not declare an 18% rate")
		})
	}
}
