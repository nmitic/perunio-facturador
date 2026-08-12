package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"strconv"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
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
	// Detracción (cac:PaymentMeans + cac:PaymentTerms) — present only when the
	// nota references a factura sujeta a detracción. nil/empty otherwise.
	PaymentMeans *paymentMeans
	PaymentTerms []paymentTerms
	TaxTotal     taxTotal
	// UBL 2.1 asymmetry: Invoice and CreditNote name the totals block
	// cac:LegalMonetaryTotal, but DebitNoteType requires it to be
	// cac:RequestedMonetaryTotal in that exact slot. Emitting LegalMonetaryTotal
	// trips SUNAT fault soap-env:Client.0306 (cvc-particle 2.1: next item should
	// be RequestedMonetaryTotal).
	RequestedMonetaryTotal legalMonetaryTotal `xml:"cac:RequestedMonetaryTotal"`
	DebitNoteLines         []debitNoteLine
}

// buildDebitNoteXML creates UBL 2.1 DebitNote XML bytes.
func buildDebitNoteXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)
	refID := fmt.Sprintf("%s-%08d", req.ReferenceDocSeries, req.ReferenceDocCorrelative)

	root := ublRootFor(sunat.UblDocumentDebitNote)

	dn := debitNote{
		XMLNS:    root.NS,
		XMLNSCAC: nsCAC,
		XMLNSCBC: nsCBC,
		XMLNSEXT: nsEXT,
		XMLNSDS:  nsDS,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    root.UBLVersionID,
		CustomizationID: root.CustomizationID,
		ProfileID:       newProfileID(req.OperationType, req.Detraccion),
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

	// Detracción (SPOT) — leyenda 2006 + cuenta BN + código/porcentaje/monto,
	// mirrored from the referenced factura.
	dn.Notes = appendDetraccionLegend(dn.Notes, req)
	dn.PaymentMeans = buildDetraccionPaymentMeans(req)
	if pt := buildDetraccionPaymentTerms(req); pt != nil {
		dn.PaymentTerms = append(dn.PaymentTerms, *pt)
	}

	dn.TaxTotal = buildDocumentTaxTotal(req)

	// DebitNote uses ChargeTotalAmount instead of AllowanceTotalAmount
	lmt := buildLegalMonetaryTotal(req)
	if lmt.AllowanceTotalAmount != nil {
		lmt.ChargeTotalAmount = lmt.AllowanceTotalAmount
		lmt.AllowanceTotalAmount = nil
	}
	dn.RequestedMonetaryTotal = lmt

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
	pr, err := buildPricingReference(li, cur)
	if err != nil {
		return debitNoteLine{}, err
	}
	// Like CreditNoteLine, SUNAT's DebitNoteLine has no line-level
	// cac:AllowanceCharge; the descuento is baked into the net valor unitario.
	return debitNoteLine{
		ID:                  strconv.Itoa(li.LineNumber),
		DebitedQuantity:     quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(lineExtensionAmountFor(li), cur),
		PricingReference:    pr,
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(noteLineUnitPrice(li), cur)},
	}, nil
}
