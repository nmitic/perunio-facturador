package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/perunio/perunio-facturador/internal/model"
)

// isZeroAmount returns true for any decimal string that represents zero
// (e.g. "0", "0.0", "0.00") or is empty.
func isZeroAmount(s string) bool {
	s = strings.TrimLeft(s, "0")
	s = strings.TrimPrefix(s, ".")
	s = strings.TrimRight(s, "0")
	return s == "" || s == "."
}

// invoice is the UBL 2.1 Invoice XML root element.
type invoice struct {
	XMLName              xml.Name                `xml:"Invoice"`
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
	InvoiceTypeCode      invoiceTypeCode
	Notes                []noteElement
	DocumentCurrencyCode documentCurrencyCode
	Signature            cacSignature
	SupplierParty        accountingSupplierParty
	CustomerParty        accountingCustomerParty
	TaxTotal             taxTotal
	LegalMonetaryTotal   legalMonetaryTotal
	InvoiceLines         []invoiceLine
}

// buildInvoiceXML creates UBL 2.1 Invoice XML bytes from an issue request.
func buildInvoiceXML(req model.IssueRequest) ([]byte, error) {
	docID := fmt.Sprintf("%s-%08d", req.Series, req.Correlative)

	inv := invoice{
		XMLNS:    NSInvoice,
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
		InvoiceTypeCode: newInvoiceTypeCode(req.DocType),

		DocumentCurrencyCode: newDocumentCurrencyCode(req.CurrencyCode),
		Signature:            newCACSignature(req.SupplierRUC, req.SupplierName),
		SupplierParty:        newSupplierParty(req.SupplierRUC, req.SupplierName, req.SupplierAddress, req.EstablishmentCode),
		CustomerParty:        newCustomerParty(req.CustomerDocType, req.CustomerDocNumber, req.CustomerName, req.CustomerAddress),
	}

	// Notes
	for _, n := range req.Notes {
		inv.Notes = append(inv.Notes, noteElement{
			Value:            n.Text,
			LanguageLocaleID: n.Code,
		})
	}

	// Tax totals
	inv.TaxTotal = buildDocumentTaxTotal(req)

	// Monetary totals
	inv.LegalMonetaryTotal = buildLegalMonetaryTotal(req)

	// Lines
	for _, li := range req.Items {
		inv.InvoiceLines = append(inv.InvoiceLines, buildInvoiceLine(li, req.CurrencyCode))
	}

	return marshalISO8859(&inv)
}

func buildDocumentTaxTotal(req model.IssueRequest) taxTotal {
	cur := req.CurrencyCode
	tt := taxTotal{
		TaxAmount: newCurrencyAmount(req.TotalIGV, cur),
	}

	// IGV subtotal (always present)
	if !isZeroAmount(req.TotalIGV) {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount(req.TotalIGV, cur),
			TaxCategory:   newTaxCategory("S", "18.00", model.TaxIGV),
		})
	}

	// ISC subtotal (if present)
	if !isZeroAmount(req.TotalISC) {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount(req.TotalISC, cur),
			TaxCategory:   newTaxCategory("S", "", model.TaxISC),
		})
	}

	// Other taxes subtotal (if present)
	if !isZeroAmount(req.TotalOtherTaxes) {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount(req.TotalOtherTaxes, cur),
			TaxCategory:   newTaxCategory("S", "", model.TaxOtros),
		})
	}

	// If no subtotals were added, add a zero IGV subtotal
	if len(tt.TaxSubtotal) == 0 {
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(req.Subtotal, cur),
			TaxAmount:     newCurrencyAmount("0.00", cur),
			TaxCategory:   newTaxCategory("S", "18.00", model.TaxIGV),
		})
	}

	return tt
}

func buildLegalMonetaryTotal(req model.IssueRequest) legalMonetaryTotal {
	cur := req.CurrencyCode
	lmt := legalMonetaryTotal{
		LineExtensionAmount: newCurrencyAmount(req.Subtotal, cur),
		TaxInclusiveAmount:  newCurrencyAmount(req.TaxInclusiveAmount, cur),
		PayableAmount:       newCurrencyAmount(req.TotalAmount, cur),
	}

	if req.TotalDiscount != "" && req.TotalDiscount != "0.00" {
		amt := newCurrencyAmount(req.TotalDiscount, cur)
		lmt.AllowanceTotalAmount = &amt
	}

	return lmt
}

func buildInvoiceLine(li model.LineItem, cur string) invoiceLine {
	return invoiceLine{
		ID:                  fmt.Sprint(li.LineNumber),
		InvoicedQuantity:    quantity{Value: li.Quantity, UnitCode: li.UnitCode},
		LineExtensionAmount: newCurrencyAmount(li.LineTotal, cur),
		PricingReference:    buildPricingReference(li, cur),
		TaxTotal:            buildLineTaxTotal(li, cur),
		Item:                item{Description: li.Description},
		Price:               price{PriceAmount: newCurrencyAmount(li.UnitPrice, cur)},
	}
}

func buildPricingReference(li model.LineItem, cur string) pricingReference {
	return pricingReference{
		AlternativeConditionPrice: alternativeConditionPrice{
			PriceAmount: newCurrencyAmount(li.UnitPriceWithTax, cur),
			PriceTypeCode: priceTypeCode{
				Value:          li.PriceTypeCode,
				ListName:       "Tipo de Precio",
				ListAgencyName: "PE:SUNAT",
				ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo16",
			},
		},
	}
}

func buildLineTaxTotal(li model.LineItem, cur string) taxTotal {
	taxCode := model.TaxCodeForAffectation(li.TaxExemptionReasonCode)
	ts, ok := model.TaxSchemeByCode(taxCode)
	if !ok {
		ts = model.TaxIGV // fallback
	}

	tt := taxTotal{
		TaxAmount: newCurrencyAmount(li.IGVAmount, cur),
		TaxSubtotal: []taxSubtotal{
			{
				TaxableAmount: newCurrencyAmount(li.LineTotal, cur),
				TaxAmount:     newCurrencyAmount(li.IGVAmount, cur),
				TaxCategory:   newLineTaxCategory(li.TaxExemptionReasonCode, ts),
			},
		},
	}

	// ISC subtotal if present
	if !isZeroAmount(li.ISCAmount) {
		iscCat := newTaxCategory("S", "", model.TaxISC)
		iscCat.TierRange = li.ISCTierRange
		tt.TaxSubtotal = append(tt.TaxSubtotal, taxSubtotal{
			TaxableAmount: newCurrencyAmount(li.LineTotal, cur),
			TaxAmount:     newCurrencyAmount(li.ISCAmount, cur),
			TaxCategory:   iscCat,
		})
	}

	return tt
}

func newTaxCategory(categoryID, percent string, ts model.TaxSchemeType) taxCategory {
	tc := taxCategory{
		ID: taxCategoryID{
			Value:          categoryID,
			SchemeID:       "UN/ECE 5305",
			SchemeAgencyID: "6",
		},
		TaxScheme: taxSchemeXML{
			ID: taxSchemeID{
				Value:          ts.Code,
				SchemeID:       "UN/ECE 5153",
				SchemeAgencyID: "6",
			},
			Name:        ts.Name,
			TaxTypeCode: ts.TaxTypeCode,
		},
	}
	if percent != "" {
		tc.Percent = percent
	}
	return tc
}

func newLineTaxCategory(exemptionReasonCode string, ts model.TaxSchemeType) taxCategory {
	tc := newTaxCategory(ts.TaxCategoryID, "", ts)

	// Set percent based on tax type
	switch ts.Code {
	case "1000": // IGV
		tc.Percent = "18.00"
	case "1016": // IVAP
		tc.Percent = "4.00"
	}

	if exemptionReasonCode != "" {
		tc.TaxExemptionReasonCode = &taxExemptionCode{
			Value:          exemptionReasonCode,
			ListAgencyName: "PE:SUNAT",
			ListName:       "Tipo de Afectacion del IGV",
			ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo07",
		}
	}

	return tc
}
