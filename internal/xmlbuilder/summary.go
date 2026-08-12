package xmlbuilder

import (
	"encoding/xml"
	"fmt"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// summaryDocuments is the UBL 2.0 SummaryDocuments root element (Resumen Diario - RC).
type summaryDocuments struct {
	XMLName         xml.Name `xml:"SummaryDocuments"`
	XMLNS           string   `xml:"xmlns,attr"`
	XMLNSCAC        string   `xml:"xmlns:cac,attr"`
	XMLNSCBC        string   `xml:"xmlns:cbc,attr"`
	XMLNSDS         string   `xml:"xmlns:ds,attr"`
	XMLNSEXT        string   `xml:"xmlns:ext,attr"`
	XMLNSSAC        string   `xml:"xmlns:sac,attr"`
	UBLExtensions   ublExtensions
	UBLVersionID    string `xml:"cbc:UBLVersionID"`
	CustomizationID string `xml:"cbc:CustomizationID"`
	ID              string `xml:"cbc:ID"`
	ReferenceDate   string `xml:"cbc:ReferenceDate"`
	IssueDate       string `xml:"cbc:IssueDate"`
	Signature       cacSignature
	SupplierParty   summarySupplierParty
	SummaryLines    []summaryDocumentsLine `xml:"sac:SummaryDocumentsLine"`
}

type summarySupplierParty struct {
	XMLName             xml.Name     `xml:"cac:AccountingSupplierParty"`
	CustomerAssignedID  string       `xml:"cbc:CustomerAssignedAccountID"`
	AdditionalAccountID string       `xml:"cbc:AdditionalAccountID"`
	Party               summaryParty `xml:"cac:Party"`
}

type summaryParty struct {
	PartyLegalEntity summaryPartyLegalEntity `xml:"cac:PartyLegalEntity"`
}

type summaryPartyLegalEntity struct {
	RegistrationName string `xml:"cbc:RegistrationName"`
}

// summaryDocumentsLine follows SUNAT's SummaryDocumentsLineType sequence order
// exactly: customer + billing reference + cac:Status all precede sac:TotalAmount,
// then sac:BillingPayment, then cac:TaxTotal. Emitting them out of order triggers
// schema fault "cvc-complex-type 2.4 ... next item should be end-element".
type summaryDocumentsLine struct {
	LineID                  string                  `xml:"cbc:LineID"`
	DocumentTypeCode        string                  `xml:"cbc:DocumentTypeCode"`
	DocumentSerialID        string                  `xml:"sac:DocumentSerialID"`
	StartDocumentNumberID   string                  `xml:"sac:StartDocumentNumberID"`
	EndDocumentNumberID     string                  `xml:"sac:EndDocumentNumberID"`
	AccountingCustomerParty *summaryCustomerParty   `xml:"cac:AccountingCustomerParty,omitempty"`
	BillingReference        *summaryBillingRef      `xml:"cac:BillingReference,omitempty"`
	ConditionCode           string                  `xml:"cac:Status>cbc:ConditionCode"`
	TotalAmount             currencyAmount          `xml:"sac:TotalAmount"`
	BillingPayment          []summaryBillingPayment `xml:"sac:BillingPayment"`
	TaxTotal                []summaryTaxTotal       `xml:"cac:TaxTotal"`
}

type summaryBillingPayment struct {
	PaidAmount    currencyAmount `xml:"cbc:PaidAmount"`
	InstructionID string         `xml:"cbc:InstructionID"`
}

// summaryTaxTotal is a per-tax cac:TaxTotal on an RC line (IGV, ISC, …), matching
// greenter's accepted Resumen Diario output.
type summaryTaxTotal struct {
	TaxAmount   currencyAmount     `xml:"cbc:TaxAmount"`
	TaxSubtotal summaryTaxSubtotal `xml:"cac:TaxSubtotal"`
}

type summaryTaxSubtotal struct {
	TaxAmount   currencyAmount     `xml:"cbc:TaxAmount"`
	TaxCategory summaryTaxCategory `xml:"cac:TaxCategory"`
}

type summaryTaxCategory struct {
	TaxScheme summaryTaxScheme `xml:"cac:TaxScheme"`
}

type summaryTaxScheme struct {
	ID          string `xml:"cbc:ID"`
	Name        string `xml:"cbc:Name"`
	TaxTypeCode string `xml:"cbc:TaxTypeCode"`
}

type summaryCustomerParty struct {
	CustomerAssignedID  string `xml:"cbc:CustomerAssignedAccountID"`
	AdditionalAccountID string `xml:"cbc:AdditionalAccountID"`
}

type summaryBillingRef struct {
	InvoiceDocumentReference summaryInvoiceRef `xml:"cac:InvoiceDocumentReference"`
}

type summaryInvoiceRef struct {
	ID               string `xml:"cbc:ID"`
	DocumentTypeCode string `xml:"cbc:DocumentTypeCode"`
}

// BuildSummaryXML creates UBL 2.0 SummaryDocuments XML bytes.
func BuildSummaryXML(req model.SummaryRequest) ([]byte, error) {
	// cbc:ID carries NO RUC. SUNAT rebuilds the expected filename as
	// "{RUC}-{cbc:ID}", so prefixing the RUC here duplicates it and triggers
	// fault 2220 ("El ID debe coincidir con el nombre del archivo"). The RUC
	// lives only in SummaryFilename. Mirrors voidDocumentID for the RA path.
	summaryID := fmt.Sprintf("RC-%s-%05d",
		formatDateCompact(req.IssueDate),
		req.Correlative)

	root := ublRootFor(sunat.UblDocumentSummaryDocuments)

	doc := summaryDocuments{
		XMLNS:    root.NS,
		XMLNSCAC: nsCAC,
		XMLNSCBC: nsCBC,
		XMLNSDS:  nsDS,
		XMLNSEXT: nsEXT,
		XMLNSSAC: nsSAC,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    root.UBLVersionID,
		CustomizationID: root.CustomizationID,
		ID:              summaryID,
		ReferenceDate:   req.ReferenceDate,
		IssueDate:       req.IssueDate,

		Signature: newCACSignature(req.SupplierRUC, req.SupplierName),

		SupplierParty: summarySupplierParty{
			CustomerAssignedID:  req.SupplierRUC,
			AdditionalAccountID: "6",
			Party: summaryParty{
				PartyLegalEntity: summaryPartyLegalEntity{
					RegistrationName: req.SupplierName,
				},
			},
		},
	}

	for _, item := range req.Items {
		doc.SummaryLines = append(doc.SummaryLines, buildSummaryLine(item))
	}

	return marshalISO8859(&doc)
}

// SummaryFilename returns the filename for a Resumen Diario.
func SummaryFilename(ruc, issueDate string, correlative int) string {
	return fmt.Sprintf("%s-RC-%s-%05d", ruc, formatDateCompact(issueDate), correlative)
}

func buildSummaryLine(item model.SummaryItem) summaryDocumentsLine {
	// Each RC line carries the currency of its underlying boleta. SUNAT's RC
	// schema allows per-line currency, so a USD/EUR boleta summarizes in its own
	// moneda. Fall back to PEN for legacy rows with an empty currency.
	cur := item.CurrencyCode
	if cur == "" {
		cur = "PEN"
	}
	// Field order below is irrelevant (Go marshals in struct-definition order);
	// the schema-critical sequence lives in the summaryDocumentsLine type.
	line := summaryDocumentsLine{
		LineID:                fmt.Sprint(item.LineNumber),
		DocumentTypeCode:      item.DocType,
		DocumentSerialID:      item.Series,
		StartDocumentNumberID: fmt.Sprint(item.StartCorrelative),
		EndDocumentNumberID:   fmt.Sprint(item.EndCorrelative),
		ConditionCode:         item.ConditionCode,
		TotalAmount:           newCurrencyAmount(item.TotalAmount, cur),
	}

	// Customer (optional) — must precede cac:Status per the schema sequence.
	if item.CustomerDocNumber != "" {
		line.AccountingCustomerParty = &summaryCustomerParty{
			CustomerAssignedID:  item.CustomerDocNumber,
			AdditionalAccountID: item.CustomerDocType,
		}
	}

	// Billing reference (NC/ND in summary) — also precedes cac:Status.
	if item.ReferenceSeries != "" {
		refID := fmt.Sprintf("%s-%d", item.ReferenceSeries, item.ReferenceCorr)
		line.BillingReference = &summaryBillingRef{
			InvoiceDocumentReference: summaryInvoiceRef{
				ID:               refID,
				DocumentTypeCode: item.ReferenceDocType,
			},
		}
	}

	// BillingPayment: valor de venta por tipo de operación (Cat. via InstructionID).
	// These are IGV-exclusive bases, not the tax amounts.
	if !isZeroAmount(item.TotalGravada) {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalGravada, cur),
			InstructionID: "01", // Gravado
		})
	}
	if !isZeroAmount(item.TotalExonerated) {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalExonerated, cur),
			InstructionID: "02", // Exonerado
		})
	}
	if !isZeroAmount(item.TotalUnaffected) {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalUnaffected, cur),
			InstructionID: "03", // Inafecto
		})
	}
	if !isZeroAmount(item.TotalFree) {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalFree, cur),
			InstructionID: "05", // Gratuito
		})
	}

	// TaxTotal: one cac:TaxTotal per tributo present (IGV, ISC, otros).
	if !isZeroAmount(item.TotalIGV) {
		line.TaxTotal = append(line.TaxTotal, newSummaryTax(item.TotalIGV, cur, sunat.Cat05IGV))
	}
	if !isZeroAmount(item.TotalISC) {
		line.TaxTotal = append(line.TaxTotal, newSummaryTax(item.TotalISC, cur, sunat.Cat05ISC))
	}
	if !isZeroAmount(item.TotalOtherTaxes) {
		line.TaxTotal = append(line.TaxTotal, newSummaryTax(item.TotalOtherTaxes, cur, sunat.Cat05Otros))
	}

	return line
}

// newSummaryTax builds a cac:TaxTotal for an RC line from a tax amount, its line
// currency and its Cat.05 código. The scheme's name and cbc:TaxTypeCode come from
// the catálogo, so an RC line and the equivalent Invoice line cannot describe the
// same tributo differently.
func newSummaryTax(amount, currency, taxCode string) summaryTaxTotal {
	return summaryTaxTotal{
		TaxAmount: newCurrencyAmount(amount, currency),
		TaxSubtotal: summaryTaxSubtotal{
			TaxAmount: newCurrencyAmount(amount, currency),
			TaxCategory: summaryTaxCategory{
				TaxScheme: summaryTaxScheme{
					ID:          taxCode,
					Name:        sunat.Cat05Name(taxCode),
					TaxTypeCode: sunat.Cat05TaxTypeCode(taxCode),
				},
			},
		},
	}
}

// formatDateCompact converts "2024-01-15" to "20240115".
func formatDateCompact(date string) string {
	// Remove dashes from YYYY-MM-DD
	return fmt.Sprintf("%s%s%s", date[0:4], date[5:7], date[8:10])
}
