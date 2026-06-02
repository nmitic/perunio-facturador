package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"strconv"

	"github.com/perunio/perunio-facturador/internal/model"
)

// debitNote is the UBL 2.1 DebitNote XML root element.
type debitNote struct {
	XMLName              xml.Name `xml:"DebitNote"`
	XMLNS                string   `xml:"xmlns,attr"`
	XMLNSCAC             string   `xml:"xmlns:cac,attr"`
	XMLNSCBC             string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT             string   `xml:"xmlns:ext,attr"`
	XMLNSDS              string   `xml:"xmlns:ds,attr"`
	UBLExtensions        ublExtensions
	UBLVersionID         string `xml:"cbc:UBLVersionID"`
	CustomizationID      string `xml:"cbc:CustomizationID"`
	ProfileID            profileIDElement
	ID                   string `xml:"cbc:ID"`
	IssueDate            string `xml:"cbc:IssueDate"`
	IssueTime            string `xml:"cbc:IssueTime,omitempty"`
	Notes                []noteElement
	DocumentCurrencyCode documentCurrencyCode
	DiscrepancyResponse  discrepancyResponse
	BillingReference     billingReference
	Signature            cacSignature
	SupplierParty        accountingSupplierParty
	CustomerParty        accountingCustomerParty
	TaxTotal             taxTotal
	LegalMonetaryTotal   legalMonetaryTotal
	DebitNoteLines       []debitNoteLine
}

// buildDebitNoteXML creates UBL 2.1 DebitNote XML bytes.
func buildDebitNoteXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	refID := fmt.Sprintf("%s-%08d", req.ReferenceDocSeries, req.ReferenceDocCorrelative)

	dn := debitNote{
		XMLNS:    NSDebitNote,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSEXT: NSEXT,
		XMLNSDS:  NSDS,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    UBLVersion21,
		CustomizationID: CustomizationID20,
		ProfileID:       newProfileID(req.OperationType),
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
		dn.Notes = append(dn.Notes, noteElement{
			Value:            n.Text,
			LanguageLocaleID: n.Code,
		})
	}

	dn.TaxTotal = buildDocumentTaxTotal(req)

	// DebitNote uses ChargeTotalAmount instead of AllowanceTotalAmount
	lmt := buildLegalMonetaryTotal(req)
	if lmt.AllowanceTotalAmount != nil {
		lmt.ChargeTotalAmount = lmt.AllowanceTotalAmount
		lmt.AllowanceTotalAmount = nil
	}
	dn.LegalMonetaryTotal = lmt

	for _, li := range req.Items {
		line, err := buildDebitNoteLine(li, req.CurrencyCode)
		if err != nil {
			return nil, err
		}
		dn.DebitNoteLines = append(dn.DebitNoteLines, line)
	}

	return marshalISO8859(&dn)
}

func buildDebitNoteLine(li model.LineItem, cur string) (debitNoteLine, error) {
	unitPrice := li.UnitPrice
	if isGratuitoCode(li.TaxExemptionReasonCode) {
		unitPrice = "0.00"
	}
	pr, err := buildPricingReference(li, cur)
	if err != nil {
		return debitNoteLine{}, err
	}
	return debitNoteLine{
		ID:                  strconv.Itoa(li.LineNumber),
		DebitedQuantity:     quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(lineExtensionAmountFor(li), cur),
		PricingReference:    pr,
		AllowanceCharge:     buildLineDiscount(li, cur),
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(unitPrice, cur)},
	}, nil
}
