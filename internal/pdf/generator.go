package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/skip2/go-qrcode"

	"github.com/perunio/perunio-facturador/internal/model"
)

// Generate creates a PDF representation of an invoice.
func Generate(req model.IssueRequest, qrData string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	// Document type title
	docTitle := documentTitle(req.DocType)
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)

	// Header: Company info (left) + Document box (right)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(100, 6, req.SupplierName)
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(100, 5, fmt.Sprintf("RUC: %s", req.SupplierRUC))
	pdf.Ln(5)
	if req.SupplierAddress != "" {
		pdf.Cell(100, 5, req.SupplierAddress)
		pdf.Ln(5)
	}

	// Document box (right side)
	pdf.SetXY(120, 10)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(240, 240, 240)
	pdf.Rect(115, 8, 80, 25, "FD")
	pdf.SetXY(115, 10)
	pdf.CellFormat(80, 6, fmt.Sprintf("RUC: %s", req.SupplierRUC), "", 0, "C", false, 0, "")
	pdf.SetXY(115, 17)
	pdf.CellFormat(80, 6, docTitle, "", 0, "C", false, 0, "")
	pdf.SetXY(115, 24)
	pdf.CellFormat(80, 6, docID, "", 0, "C", false, 0, "")

	pdf.SetY(42)

	// Customer info
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetFillColor(245, 245, 245)
	pdf.CellFormat(190, 6, "DATOS DEL CLIENTE", "1", 1, "L", true, 0, "")
	customerRow(pdf, "Cliente", req.CustomerName)
	customerRow(pdf, "Documento", fmt.Sprintf("%s: %s", docTypeLabel(req.CustomerDocType), req.CustomerDocNumber))
	if req.CustomerAddress != "" {
		customerRow(pdf, "Direccion", req.CustomerAddress)
	}
	customerRow(pdf, "Fecha Emision", req.IssueDate)
	customerRow(pdf, "Moneda", req.CurrencyCode)

	pdf.Ln(4)

	// Items table
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(50, 50, 50)
	pdf.SetTextColor(255, 255, 255)
	colWidths := []float64{12, 80, 20, 25, 25, 28}
	headers := []string{"#", "Descripcion", "Cant.", "P.Unit", "IGV", "Total"}
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	for _, item := range req.Items {
		pdf.CellFormat(colWidths[0], 6, fmt.Sprint(item.LineNumber), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colWidths[1], 6, truncate(item.Description, 45), "1", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[2], 6, item.Quantity, "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[3], 6, item.UnitPrice, "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[4], 6, item.IGVAmount, "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[5], 6, item.LineTotal, "1", 0, "R", false, 0, "")
		pdf.Ln(-1)
	}

	pdf.Ln(4)

	// Totals
	totalsX := 130.0
	pdf.SetFont("Helvetica", "", 9)
	totalRow(pdf, totalsX, "Subtotal", req.CurrencyCode, req.Subtotal)
	totalRow(pdf, totalsX, "IGV (18%)", req.CurrencyCode, req.TotalIGV)
	if req.TotalDiscount != "" && req.TotalDiscount != "0.00" {
		totalRow(pdf, totalsX, "Descuento", req.CurrencyCode, req.TotalDiscount)
	}
	pdf.SetFont("Helvetica", "B", 10)
	totalRow(pdf, totalsX, "TOTAL", req.CurrencyCode, req.TotalAmount)

	// Forma de pago + cuotas
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "B", 9)
	formaPagoLabel := "Contado"
	if strings.EqualFold(strings.TrimSpace(req.FormaPago), "credito") {
		formaPagoLabel = "Credito"
	}
	pdf.Cell(40, 5, "Forma de pago:")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(150, 5, formaPagoLabel)
	pdf.Ln(6)
	if formaPagoLabel == "Credito" && len(req.Cuotas) > 0 {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetFillColor(50, 50, 50)
		pdf.SetTextColor(255, 255, 255)
		cuotaCols := []float64{20, 40, 50}
		cuotaHdr := []string{"Cuota", "Vencimiento", "Monto"}
		for i, h := range cuotaHdr {
			pdf.CellFormat(cuotaCols[i], 6, h, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFillColor(255, 255, 255)
		for _, c := range req.Cuotas {
			pdf.CellFormat(cuotaCols[0], 5, fmt.Sprintf("%03d", c.Numero), "1", 0, "C", false, 0, "")
			pdf.CellFormat(cuotaCols[1], 5, c.FechaVencimiento, "1", 0, "C", false, 0, "")
			pdf.CellFormat(cuotaCols[2], 5, fmt.Sprintf("%s %s", req.CurrencyCode, c.Monto), "1", 0, "R", false, 0, "")
			pdf.Ln(-1)
		}
	}

	// Notes
	if len(req.Notes) > 0 {
		pdf.Ln(4)
		pdf.SetFont("Helvetica", "I", 8)
		for _, n := range req.Notes {
			pdf.Cell(190, 5, n.Text)
			pdf.Ln(5)
		}
	}

	// QR Code (bottom left)
	if qrData != "" {
		qrPNG, err := qrcode.Encode(qrData, qrcode.Medium, 120)
		if err == nil {
			reader := bytes.NewReader(qrPNG)
			opts := fpdf.ImageOptions{ImageType: "PNG"}
			pdf.RegisterImageOptionsReader("qr", opts, reader)
			pdf.ImageOptions("qr", 10, pdf.GetY()+5, 30, 30, false, opts, 0, "")
		}
	}

	// Footer
	pdf.SetY(-20)
	pdf.SetFont("Helvetica", "I", 7)
	pdf.Cell(190, 4, "Representacion impresa de la factura electronica")
	pdf.Ln(4)
	pdf.Cell(190, 4, "Emitido mediante Perunio - perunio.pe")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}

func documentTitle(docType string) string {
	switch docType {
	case "01":
		return "FACTURA ELECTRONICA"
	case "03":
		return "BOLETA DE VENTA ELECTRONICA"
	case "07":
		return "NOTA DE CREDITO ELECTRONICA"
	case "08":
		return "NOTA DE DEBITO ELECTRONICA"
	default:
		return "COMPROBANTE ELECTRONICO"
	}
}

func docTypeLabel(code string) string {
	switch code {
	case "6":
		return "RUC"
	case "1":
		return "DNI"
	case "4":
		return "CE"
	case "7":
		return "Pasaporte"
	default:
		return "Doc"
	}
}

func customerRow(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(30, 5, label+":", "LB", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.CellFormat(160, 5, value, "RB", 1, "L", false, 0, "")
}

func totalRow(pdf *fpdf.Fpdf, x float64, label, currency, amount string) {
	pdf.SetX(x)
	pdf.CellFormat(30, 6, label, "", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, fmt.Sprintf("%s %s", currency, amount), "", 1, "R", false, 0, "")
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
