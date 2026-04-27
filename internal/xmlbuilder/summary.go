package xmlbuilder

import (
	"encoding/xml"
	"fmt"

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
	XMLName             xml.Name `xml:"cac:AccountingSupplierParty"`
	CustomerAssignedID  string   `xml:"cbc:CustomerAssignedAccountID"`
	AdditionalAccountID string   `xml:"cbc:AdditionalAccountID"`
	Party               summaryParty `xml:"cac:Party"`
}

type summaryParty struct {
	PartyLegalEntity summaryPartyLegalEntity `xml:"cac:PartyLegalEntity"`
}

type summaryPartyLegalEntity struct {
	RegistrationName string `xml:"cbc:RegistrationName"`
}

type summaryDocumentsLine struct {
	LineID                  string                `xml:"cbc:LineID"`
	DocumentTypeCode        string                `xml:"cbc:DocumentTypeCode"`
	DocumentSerialID        string                `xml:"sac:DocumentSerialID"`
	StartDocumentNumberID   string                `xml:"sac:StartDocumentNumberID"`
	EndDocumentNumberID     string                `xml:"sac:EndDocumentNumberID"`
	TotalAmount             currencyAmount        `xml:"sac:TotalAmount"`
	BillingPayment          []summaryBillingPayment `xml:"sac:BillingPayment"`
	AccountingCustomerParty *summaryCustomerParty `xml:"cac:AccountingCustomerParty,omitempty"`
	BillingReference        *summaryBillingRef    `xml:"cac:BillingReference,omitempty"`
	ConditionCode           string                `xml:"cac:Status>cbc:ConditionCode"`
	TotalTaxAmount          currencyAmount        `xml:"sac:TaxTotal>cbc:TaxAmount"`
}

type summaryBillingPayment struct {
	PaidAmount      currencyAmount `xml:"cbc:PaidAmount"`
	InstructionID   string         `xml:"cbc:InstructionID"`
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
	summaryID := fmt.Sprintf("%s-RC-%s-%05d",
		req.SupplierRUC,
		formatDateCompact(req.IssueDate),
		req.Correlative)

	doc := summaryDocuments{
		XMLNS:    NSSummaryDocuments,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSDS:  NSDS,
		XMLNSEXT: NSEXT,
		XMLNSSAC: NSSAC,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    UBLVersion20,
		CustomizationID: CustomizationIDRC,
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
	line := summaryDocumentsLine{
		LineID:                fmt.Sprint(item.LineNumber),
		DocumentTypeCode:     item.DocType,
		DocumentSerialID:     item.Series,
		StartDocumentNumberID: fmt.Sprint(item.StartCorrelative),
		EndDocumentNumberID:  fmt.Sprint(item.EndCorrelative),
		TotalAmount:          newCurrencyAmount(item.TotalAmount, "PEN"), // RC amounts always PEN
		ConditionCode:        item.ConditionCode,
		TotalTaxAmount:       newCurrencyAmount(item.TotalIGV, "PEN"),
	}

	// Billing payments (tax breakdowns)
	if item.TotalIGV != "" && item.TotalIGV != "0.00" {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalIGV, "PEN"),
			InstructionID: "01", // Gravado
		})
	}
	if item.TotalExonerated != "" && item.TotalExonerated != "0.00" {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalExonerated, "PEN"),
			InstructionID: "02", // Exonerado
		})
	}
	if item.TotalUnaffected != "" && item.TotalUnaffected != "0.00" {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalUnaffected, "PEN"),
			InstructionID: "03", // Inafecto
		})
	}
	if item.TotalFree != "" && item.TotalFree != "0.00" {
		line.BillingPayment = append(line.BillingPayment, summaryBillingPayment{
			PaidAmount:    newCurrencyAmount(item.TotalFree, "PEN"),
			InstructionID: "05", // Gratuito
		})
	}

	// Customer (if present)
	if item.CustomerDocNumber != "" {
		line.AccountingCustomerParty = &summaryCustomerParty{
			CustomerAssignedID:  item.CustomerDocNumber,
			AdditionalAccountID: item.CustomerDocType,
		}
	}

	// Billing reference (for NC/ND in summary)
	if item.ReferenceSeries != "" {
		refID := fmt.Sprintf("%s-%d", item.ReferenceSeries, item.ReferenceCorr)
		line.BillingReference = &summaryBillingRef{
			InvoiceDocumentReference: summaryInvoiceRef{
				ID:               refID,
				DocumentTypeCode: item.ReferenceDocType,
			},
		}
	}

	return line
}

// formatDateCompact converts "2024-01-15" to "20240115".
func formatDateCompact(date string) string {
	// Remove dashes from YYYY-MM-DD
	return fmt.Sprintf("%s%s%s", date[0:4], date[5:7], date[8:10])
}
