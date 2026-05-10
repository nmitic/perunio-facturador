package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/skip2/go-qrcode"

	"github.com/perunio/perunio-facturador/internal/model"
)

// defaultBrandRGB is the slate (#1F2937) used when a company hasn't picked
// a brand color, so unbranded PDFs still look intentional rather than ugly.
var defaultBrandRGB = [3]int{31, 41, 55}

// parseHexColor decodes "#RRGGBB" into 0-255 RGB triplet. Returns
// defaultBrandRGB on any parse failure so callers don't need fallback logic.
func parseHexColor(hex string) (int, int, int) {
	hex = strings.TrimSpace(hex)
	if len(hex) != 7 || hex[0] != '#' {
		return defaultBrandRGB[0], defaultBrandRGB[1], defaultBrandRGB[2]
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return defaultBrandRGB[0], defaultBrandRGB[1], defaultBrandRGB[2]
	}
	return int((v >> 16) & 0xFF), int((v >> 8) & 0xFF), int(v & 0xFF)
}

// decodeLogoDataURI splits "data:image/<png|jpeg>;base64,<payload>" and
// returns (bytes, fpdf-image-type, ok). Anything else returns ok=false.
func decodeLogoDataURI(s string) ([]byte, string, bool) {
	const prefix = "data:image/"
	if !strings.HasPrefix(s, prefix) {
		return nil, "", false
	}
	semi := strings.Index(s, ";")
	comma := strings.Index(s, ",")
	if semi < 0 || comma < 0 || comma < semi {
		return nil, "", false
	}
	mime := strings.ToLower(s[len(prefix):semi])
	var imgType string
	switch mime {
	case "png":
		imgType = "PNG"
	case "jpeg", "jpg":
		imgType = "JPG"
	default:
		return nil, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(s[comma+1:])
	if err != nil || len(raw) == 0 {
		return nil, "", false
	}
	return raw, imgType, true
}

// isGratuitoCat07 reports whether a Cat.07 affectation code is gratuito
// (transferencia/retiro a título gratuito): 11-16, 21, 31-37.
func isGratuitoCat07(code string) bool {
	switch code {
	case "11", "12", "13", "14", "15", "16",
		"21",
		"31", "32", "33", "34", "35", "36", "37":
		return true
	}
	return false
}

// affectationLabel returns a short Spanish label for a Cat.07 code.
func affectationLabel(code string) string {
	if isGratuitoCat07(code) {
		return "Gratuito"
	}
	switch code {
	case "10":
		return "Gravado"
	case "17":
		return "IVAP"
	case "20":
		return "Exonerado"
	case "30":
		return "Inafecto"
	case "40":
		return "Exportación"
	}
	return code
}

func parseAmount(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// nonZero reports whether a decimal string represents a non-zero value.
func nonZero(s string) bool {
	return parseAmount(s) > 0.0001
}

// formatMoney renders a float as a fixed-2 decimal string.
func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// Generate creates a PDF representation of an invoice.
func Generate(req model.IssueRequest, qrData string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	br, bg, bb := parseHexColor(req.BrandColor)

	// Document type title
	docTitle := documentTitle(req.DocType)
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)

	// === Header band: full-width brand-coloured stripe with logo (left),
	// company info (centre), and document box (right). ===
	const headerH = 34.0
	pdf.SetFillColor(br, bg, bb)
	pdf.Rect(0, 0, 210, headerH, "F")

	// Logo (optional). Aspect-fit into a 24x24mm slot at the top-left.
	textX := 10.0
	if logoBytes, imgType, ok := decodeLogoDataURI(req.LogoBase64); ok {
		opts := fpdf.ImageOptions{ImageType: imgType}
		pdf.RegisterImageOptionsReader("brand_logo", opts, bytes.NewReader(logoBytes))
		// Height-constrained; fpdf preserves aspect when one of w/h is 0.
		pdf.ImageOptions("brand_logo", 8, 5, 0, 24, false, opts, 0, "")
		textX = 38
	}

	// Company info on the band (white text).
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(textX, 7)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(80, 6, truncate(req.SupplierName, 40), "", 0, "L", false, 0, "")
	pdf.SetXY(textX, 14)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(80, 5, fmt.Sprintf("RUC: %s", req.SupplierRUC), "", 0, "L", false, 0, "")
	if req.SupplierAddress != "" {
		pdf.SetXY(textX, 20)
		pdf.CellFormat(110, 5, truncate(req.SupplierAddress, 70), "", 0, "L", false, 0, "")
	}

	// Document box on the right: white card with brand-coloured text.
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(255, 255, 255)
	pdf.Rect(140, 5, 65, 24, "F")
	pdf.SetTextColor(br, bg, bb)
	pdf.SetXY(140, 7)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(65, 5, fmt.Sprintf("RUC: %s", req.SupplierRUC), "", 0, "C", false, 0, "")
	pdf.SetXY(140, 13)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(65, 5, docTitle, "", 0, "C", false, 0, "")
	pdf.SetXY(140, 20)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(65, 6, docID, "", 0, "C", false, 0, "")

	// Reset text/draw colours for the body.
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetY(headerH + 6)

	// Customer info — section header in brand color.
	pdf.SetFillColor(br, bg, bb)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(190, 6, "DATOS DEL CLIENTE", "1", 1, "L", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 9)
	customerRow(pdf, "Cliente", req.CustomerName)
	customerRow(pdf, "Documento", fmt.Sprintf("%s: %s", docTypeLabel(req.CustomerDocType), req.CustomerDocNumber))
	if req.CustomerAddress != "" {
		customerRow(pdf, "Direccion", req.CustomerAddress)
	}
	customerRow(pdf, "Fecha Emision", req.IssueDate)
	customerRow(pdf, "Moneda", req.CurrencyCode)

	pdf.Ln(4)

	// Detect which optional columns to show.
	showISC := false
	hasGratuito := false
	for _, it := range req.Items {
		if nonZero(it.ISCAmount) {
			showISC = true
		}
		if isGratuitoCat07(it.TaxExemptionReasonCode) {
			hasGratuito = true
		}
	}

	// Items table — column widths sum to 190mm (page width minus 20mm margins).
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(br, bg, bb)
	pdf.SetTextColor(255, 255, 255)
	var colWidths []float64
	var headers []string
	if showISC {
		colWidths = []float64{8, 60, 18, 22, 20, 18, 18, 26}
		headers = []string{"#", "Descripcion", "Afect.", "Cant.", "P.Unit", "ISC", "IGV", "Total"}
	} else {
		colWidths = []float64{8, 78, 22, 22, 22, 18, 20}
		headers = []string{"#", "Descripcion", "Afect.", "Cant.", "P.Unit", "IGV", "Total"}
	}
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	for _, item := range req.Items {
		afect := affectationLabel(item.TaxExemptionReasonCode)
		descCol := 1
		afectCol := 2
		cantCol := 3
		precioCol := 4
		pdf.CellFormat(colWidths[0], 6, fmt.Sprint(item.LineNumber), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colWidths[descCol], 6, truncate(item.Description, 38), "1", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[afectCol], 6, afect, "1", 0, "C", false, 0, "")
		pdf.CellFormat(colWidths[cantCol], 6, item.Quantity, "1", 0, "R", false, 0, "")
		// Para gratuito mostramos el valor referencial (UnitPriceWithTax) en
		// lugar de "0.00" para que el cliente vea el monto referencial.
		precio := item.UnitPrice
		if isGratuitoCat07(item.TaxExemptionReasonCode) && item.UnitPriceWithTax != "" {
			precio = item.UnitPriceWithTax
		}
		pdf.CellFormat(colWidths[precioCol], 6, precio, "1", 0, "R", false, 0, "")
		if showISC {
			pdf.CellFormat(colWidths[5], 6, item.ISCAmount, "1", 0, "R", false, 0, "")
			pdf.CellFormat(colWidths[6], 6, item.IGVAmount, "1", 0, "R", false, 0, "")
			pdf.CellFormat(colWidths[7], 6, item.LineTotal, "1", 0, "R", false, 0, "")
		} else {
			pdf.CellFormat(colWidths[5], 6, item.IGVAmount, "1", 0, "R", false, 0, "")
			pdf.CellFormat(colWidths[6], 6, item.LineTotal, "1", 0, "R", false, 0, "")
		}
		pdf.Ln(-1)
	}

	pdf.Ln(4)

	// Totals — agrupados por tipo de afectación a partir de los ítems.
	var sumGravado, sumIvap, sumExo, sumInaf, sumExp, sumGrat float64
	for _, it := range req.Items {
		base := parseAmount(it.LineTotal)
		switch {
		case isGratuitoCat07(it.TaxExemptionReasonCode):
			ref := parseAmount(it.UnitPriceWithTax) * parseAmount(it.Quantity)
			sumGrat += ref
		case it.TaxExemptionReasonCode == "10":
			sumGravado += base
		case it.TaxExemptionReasonCode == "17":
			sumIvap += base
		case it.TaxExemptionReasonCode == "20":
			sumExo += base
		case strings.HasPrefix(it.TaxExemptionReasonCode, "3"):
			sumInaf += base
		case it.TaxExemptionReasonCode == "40":
			sumExp += base
		default:
			sumGravado += base
		}
	}

	totalsX := 130.0
	pdf.SetFont("Helvetica", "", 9)
	if sumGravado > 0 {
		totalRow(pdf, totalsX, "Op. Gravadas", req.CurrencyCode, formatMoney(sumGravado))
	}
	if sumIvap > 0 {
		totalRow(pdf, totalsX, "Base IVAP", req.CurrencyCode, formatMoney(sumIvap))
	}
	if sumExo > 0 {
		totalRow(pdf, totalsX, "Op. Exoneradas", req.CurrencyCode, formatMoney(sumExo))
	}
	if sumInaf > 0 {
		totalRow(pdf, totalsX, "Op. Inafectas", req.CurrencyCode, formatMoney(sumInaf))
	}
	if sumExp > 0 {
		totalRow(pdf, totalsX, "Exportación", req.CurrencyCode, formatMoney(sumExp))
	}
	totalRow(pdf, totalsX, "Subtotal", req.CurrencyCode, req.Subtotal)
	if req.TotalDiscount != "" && req.TotalDiscount != "0.00" {
		totalRow(pdf, totalsX, "Descuento", req.CurrencyCode, req.TotalDiscount)
	}
	if nonZero(req.TotalISC) {
		totalRow(pdf, totalsX, "ISC", req.CurrencyCode, req.TotalISC)
	}
	// Etiqueta IGV/IVAP según los buckets detectados.
	switch {
	case sumIvap > 0 && sumGravado == 0:
		totalRow(pdf, totalsX, "IVAP (4%)", req.CurrencyCode, req.TotalIGV)
	case sumIvap > 0 && sumGravado > 0:
		totalRow(pdf, totalsX, "IGV + IVAP", req.CurrencyCode, req.TotalIGV)
	default:
		totalRow(pdf, totalsX, "IGV (18%)", req.CurrencyCode, req.TotalIGV)
	}
	if sumGrat > 0 {
		totalRow(pdf, totalsX, "Total Gratuita", req.CurrencyCode, formatMoney(sumGrat))
	}
	// TOTAL row — brand-filled bar with white bold text.
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetFillColor(br, bg, bb)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetX(totalsX)
	pdf.CellFormat(30, 8, "TOTAL", "", 0, "L", true, 0, "")
	pdf.CellFormat(30, 8, fmt.Sprintf("%s %s", req.CurrencyCode, req.TotalAmount), "", 1, "R", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	if hasGratuito {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetX(totalsX - 10)
		pdf.CellFormat(70, 4, "Transferencia gratuita o a titulo gratuito (leyenda 1002)", "", 1, "R", false, 0, "")
	}

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
		pdf.SetFillColor(br, bg, bb)
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

	// Footer — brand-coloured rule above the legal lines.
	pdf.SetY(-22)
	pdf.SetDrawColor(br, bg, bb)
	pdf.SetLineWidth(0.6)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
	pdf.SetY(-18)
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
