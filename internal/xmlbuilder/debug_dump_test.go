package xmlbuilder_test

import (
	"strings"
	"testing"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
)

// TestDebugDumpUserPayload dumps the XML produced for the user's reported
// payload so we can inspect what we actually emit per line for the gratuito
// case (line 1, code 11). Run with: go test -run TestDebugDumpUserPayload -v
func TestDebugDumpUserPayload(t *testing.T) {
	req := model.IssueRequest{
		SupplierRUC:        "20100113612",
		SupplierName:       "EMPRESA TEST SAC",
		SupplierAddress:    "AV. TEST 123",
		EstablishmentCode:  "0000",
		DocType:            "01",
		Series:             "F001",
		Correlative:        40,
		IssueDate:          "2026-05-02",
		IssueTime:          "22:15:43",
		CurrencyCode:       "PEN",
		OperationType:      "0101",
		CustomerDocType:    "6",
		CustomerDocNumber:  "20546722687",
		CustomerName:       "ALIORE ASESORES S.A.C.",
		CustomerAddress:    "AV. RAFAEL ESCARDO NRO. 986",
		Subtotal:           "184.75",
		TotalIGV:           "36.85",
		TotalISC:           "20.00",
		TotalOtherTaxes:    "0.00",
		TotalDiscount:      "0.00",
		TotalAmount:        "241.60",
		TaxInclusiveAmount: "241.60",
		Items: []model.LineItem{
			{
				LineNumber:             1,
				Description:            "with_gratuito",
				Quantity:               "1.0",
				UnitCode:               "NIU",
				UnitPrice:              "0.00",
				UnitPriceWithTax:       "118.00",
				TaxExemptionReasonCode: "11",
				IGVAmount:              "18.00",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "0.00",
				PriceTypeCode:          "02",
			},
			{
				LineNumber:             2,
				Description:            "with_isc",
				Quantity:               "1.0",
				UnitCode:               "NIU",
				UnitPrice:              "100.00",
				UnitPriceWithTax:       "141.60",
				TaxExemptionReasonCode: "10",
				IGVAmount:              "21.60",
				ISCAmount:              "20.00",
				ISCTierRange:           "01",
				DiscountAmount:         "0.00",
				LineTotal:              "100.00",
				PriceTypeCode:          "01",
			},
			{
				LineNumber:             3,
				Description:            "with_igv",
				Quantity:               "1.0",
				UnitCode:               "NIU",
				UnitPrice:              "84.75",
				UnitPriceWithTax:       "100.00",
				TaxExemptionReasonCode: "10",
				IGVAmount:              "15.25",
				ISCAmount:              "0.00",
				DiscountAmount:         "0.00",
				LineTotal:              "84.75",
				PriceTypeCode:          "01",
			},
		},
	}

	xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(xmlBytes)

	// Print each <cac:InvoiceLine>...</cac:InvoiceLine> block.
	for {
		i := strings.Index(xml, "<cac:InvoiceLine>")
		if i < 0 {
			break
		}
		j := strings.Index(xml, "</cac:InvoiceLine>")
		if j < 0 {
			break
		}
		t.Logf("\n%s</cac:InvoiceLine>\n", xml[i:j])
		xml = xml[j+len("</cac:InvoiceLine>"):]
	}
}
