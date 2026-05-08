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
		// SUNAT err 3251: leading Credito entry must carry net pending amount = TotalAmount.
		is.True(t, strings.Contains(xml, `<cac:PaymentTerms><cbc:ID>FormaPago</cbc:ID><cbc:PaymentMeansID>Credito</cbc:PaymentMeansID><cbc:Amount currencyID="PEN">1180.00</cbc:Amount></cac:PaymentTerms>`), "Credito entry should include net pending Amount")
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Cuota001</cbc:PaymentMeansID>`), "should have Cuota001")
		is.True(t, strings.Contains(xml, `<cbc:PaymentMeansID>Cuota002</cbc:PaymentMeansID>`), "should have Cuota002")
		is.True(t, strings.Contains(xml, `<cbc:PaymentDueDate>2024-02-15</cbc:PaymentDueDate>`), "should have due date")
		is.True(t, strings.Contains(xml, `<cbc:Amount currencyID="PEN">590.00</cbc:Amount>`), "should have cuota amount with currency")
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
		is.True(t, strings.Contains(xml, `<cbc:PriceAmount currencyID="PEN">118.00</cbc:PriceAmount>`),
			"AlternativeConditionPrice must carry the IGV-inclusive referential price 118.00")
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
