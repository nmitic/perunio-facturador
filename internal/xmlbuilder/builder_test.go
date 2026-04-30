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
		CustomerAddress:   "AV. CLIENTE 456",
		Subtotal:          "1000.00",
		TotalIGV:          "180.00",
		TotalISC:          "0.00",
		TotalOtherTaxes:   "0.00",
		TotalDiscount:     "0.00",
		TotalAmount:       "1180.00",
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
		is.True(t, strings.HasPrefix(xml, `<?xml version="1.0" encoding="ISO-8859-1"?>`), "should have ISO-8859-1 declaration")

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
		is.True(t, strings.Contains(xml, `<cbc:URI>#signatureKG</cbc:URI>`), "should have signature URI reference")

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
