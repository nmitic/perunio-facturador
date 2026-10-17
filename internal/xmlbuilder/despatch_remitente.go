package xmlbuilder

import (
	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// buildDespatchRemitenteXML creates a UBL 2.1 DespatchAdvice for a
// Remitente (Cat.01 = 09) GRE. The issuing company is the transferor
// (DespatchSupplierParty); the consignee (DeliveryCustomerParty) is
// the destinatario. No SellerSupplierParty is emitted.
func buildDespatchRemitenteXML(d *model.Despatch, lines []model.DespatchLine, ruc, companyName, companyAddress string) ([]byte, error) {
	adv, err := newDespatchAdviceShell(d, lines, ruc, companyName)
	if err != nil {
		return nil, err
	}

	adv.DespatchAdviceTypeCode = newDespatchAdviceTypeCode("09")
	adv.DespatchSupplierParty = newDespatchSupplierParty(ruc, companyName, companyAddress)
	adv.DeliveryCustomerParty = newDeliveryCustomerParty(d.RecipientDocType, d.RecipientDocNumber, d.RecipientName, stringOrDefault(d.RecipientAddress, ""))

	// Optional related document (e.g. the factura covering the goods). GRE 2.0
	// reads it as cac:AdditionalDocumentReference: GRE Cat.61 code (2692),
	// SERIE-NÚMERO (3441), the code's name (OBSERV 4371) and its issuer's RUC
	// (3380/3382). cac:OrderReference, used before, is read by no GRE rule.
	if d.RelatedDocSeries != nil && *d.RelatedDocSeries != "" &&
		d.RelatedDocNumber != nil && *d.RelatedDocNumber != "" {
		docType := stringOrDefault(d.RelatedDocType, "")
		// Cat.61 shares its comprobante codes (01, 03, 04, 09, 12…) with Cat.01;
		// a code Cat.01 does not name goes without cbc:DocumentType (OBSERV only).
		name := ""
		if e, ok := sunat.Cat01(docType); ok {
			name = e.Label
		}
		adv.AdditionalDocumentReferences = []additionalDocumentReference{{
			ID:               *d.RelatedDocSeries + "-" + *d.RelatedDocNumber,
			DocumentTypeCode: docType,
			DocumentType:     name,
			// The model carries no issuer for the related document, so it is the
			// remitente's own (a venta). FOLLOWUPS.md FU-4.
			IssuerParty: &docRefIssuerParty{
				PartyIdentification: partyIdentification{
					ID: schemeID{
						Value:            ruc,
						SchemeID:         "6",
						SchemeName:       "Documento de Identidad",
						SchemeAgencyName: "PE:SUNAT",
						SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06",
					},
				},
			},
		}}
	}

	return marshalISO8859(&adv)
}
