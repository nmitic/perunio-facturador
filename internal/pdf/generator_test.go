package pdf_test

import (
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/pdf"
)

func TestGenerate(t *testing.T) {
	t.Run("should generate a valid PDF with QR code", func(t *testing.T) {
		req := model.IssueRequest{
			SupplierRUC:       "20100113612",
			SupplierName:      "EMPRESA TEST SAC",
			SupplierAddress:   "AV TEST 123",
			DocType:           "01",
			Series:            "F001",
			Correlative:       1,
			IssueDate:         "2024-01-15",
			CurrencyCode:      "PEN",
			CustomerDocType:   "6",
			CustomerDocNumber: "20601327318",
			CustomerName:      "CLIENTE TEST SRL",
			Subtotal:          "1000.00",
			TotalIGV:          "180.00",
			TotalAmount:       "1180.00",
			TaxInclusiveAmount: "1180.00",
			Notes: []model.Note{
				{Code: "1000", Text: "MIL CIENTO OCHENTA CON 00/100 SOLES"},
			},
			Items: []model.LineItem{
				{LineNumber: 1, Description: "PRODUCTO TEST", Quantity: "10",
					UnitPrice: "100.00", IGVAmount: "180.00", LineTotal: "1000.00"},
			},
		}

		qrData := "20100113612|01|F001|00000001|180.00|1180.00|2024-01-15|6|20601327318|abc123|"

		pdfBytes, err := pdf.Generate(req, qrData)
		is.NotError(t, err)
		is.True(t, len(pdfBytes) > 100, "PDF should have substantial content")

		// Verify it starts with PDF magic bytes
		is.Equal(t, "%PDF", string(pdfBytes[:4]))
	})

	t.Run("should generate PDF without QR code", func(t *testing.T) {
		req := model.IssueRequest{
			SupplierRUC: "20100113612", SupplierName: "TEST",
			DocType: "03", Series: "B001", Correlative: 1,
			IssueDate: "2024-01-15", CurrencyCode: "PEN",
			CustomerDocType: "1", CustomerDocNumber: "12345678",
			CustomerName: "CLIENTE", Subtotal: "100.00",
			TotalIGV: "18.00", TotalAmount: "118.00",
			Items: []model.LineItem{
				{LineNumber: 1, Description: "ITEM", Quantity: "1",
					UnitPrice: "100.00", IGVAmount: "18.00", LineTotal: "100.00"},
			},
		}

		pdfBytes, err := pdf.Generate(req, "")
		is.NotError(t, err)
		is.True(t, len(pdfBytes) > 100)
	})
}
