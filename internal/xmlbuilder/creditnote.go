package xmlbuilder

import (
	"encoding/xml"
	"fmt"

	"github.com/perunio/perunio-facturador/internal/model"
)

// creditNote is the UBL 2.1 CreditNote XML root element.
type creditNote struct {
	XMLName              xml.Name                `xml:"CreditNote"`
	XMLNS                string                  `xml:"xmlns,attr"`
	XMLNSCAC             string                  `xml:"xmlns:cac,attr"`
	XMLNSCBC             string                  `xml:"xmlns:cbc,attr"`
	XMLNSEXT             string                  `xml:"xmlns:ext,attr"`
	XMLNSDS              string                  `xml:"xmlns:ds,attr"`
	UBLExtensions        ublExtensions
	UBLVersionID         string                  `xml:"cbc:UBLVersionID"`
	CustomizationID      string                  `xml:"cbc:CustomizationID"`
	ID                   string                  `xml:"cbc:ID"`
	IssueDate            string                  `xml:"cbc:IssueDate"`
	IssueTime            string                  `xml:"cbc:IssueTime,omitempty"`
	Notes                []noteElement
	DocumentCurrencyCode documentCurrencyCode
	DiscrepancyResponse  discrepancyResponse
	BillingReference     billingReference
	Signature            cacSignature
	SupplierParty        accountingSupplierParty
	CustomerParty        accountingCustomerParty
	TaxTotal             taxTotal
	LegalMonetaryTotal   legalMonetaryTotal
	CreditNoteLines      []creditNoteLine
}

// buildCreditNoteXML creates UBL 2.1 CreditNote XML bytes.
func buildCreditNoteXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	refID := fmt.Sprintf("%s-%08d", req.ReferenceDocSeries, req.ReferenceDocCorrelative)

	cn := creditNote{
		XMLNS:    NSCreditNote,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSEXT: NSEXT,
		XMLNSDS:  NSDS,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    UBLVersion21,
		CustomizationID: CustomizationID20,
		ID:              docID,
		IssueDate:       req.IssueDate,
		IssueTime:       req.IssueTime,

		DocumentCurrencyCode: newDocumentCurrencyCode(req.CurrencyCode),

		DiscrepancyResponse: discrepancyResponse{
			ReferenceID:  refID,
			ResponseCode: req.ReasonCode,
			Description:  req.ReasonDescription,
		},
		BillingReference: billingReference{
			InvoiceDocumentReference: invoiceDocumentReference{
				ID:               refID,
				DocumentTypeCode: req.ReferenceDocType,
			},
		},

		Signature:     newCACSignature(req.SupplierRUC, req.SupplierName),
		SupplierParty: newSupplierParty(req.SupplierRUC, req.SupplierName, req.SupplierAddress, req.EstablishmentCode),
		CustomerParty: newCustomerParty(req.CustomerDocType, req.CustomerDocNumber, req.CustomerName, req.CustomerAddress),
	}

	for _, n := range req.Notes {
		cn.Notes = append(cn.Notes, noteElement{
			Value:            n.Text,
			LanguageLocaleID: n.Code,
		})
	}

	cn.TaxTotal = buildDocumentTaxTotal(req)
	cn.LegalMonetaryTotal = buildLegalMonetaryTotal(req)

	for _, li := range req.Items {
		cn.CreditNoteLines = append(cn.CreditNoteLines, buildCreditNoteLine(li, req.CurrencyCode))
	}

	return marshalISO8859(&cn)
}

func buildCreditNoteLine(li model.LineItem, cur string) creditNoteLine {
	return creditNoteLine{
		ID:                  fmt.Sprint(li.LineNumber),
		CreditedQuantity:    quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(li.LineTotal, cur),
		PricingReference:    buildPricingReference(li, cur),
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(li.UnitPrice, cur)},
	}
}
