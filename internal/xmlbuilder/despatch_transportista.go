package xmlbuilder

import (
	"github.com/perunio/perunio-facturador/internal/model"
)

// buildDespatchTransportistaXML creates a UBL 2.1 DespatchAdvice for a
// Transportista (Cat.01 = 31) GRE. The issuing company is the carrier, so
// DespatchSupplierParty holds the carrier's RUC and name, and the shipment
// always declares its vehicle and driver. The remitente (the party whose goods
// are being carried) is Shipment/Delivery/Despatch/DespatchParty, read from the
// Despatch's CarrierRUC/CarrierName, which on a 31 hold the remitente.
func buildDespatchTransportistaXML(d *model.Despatch, lines []model.DespatchLine, carrierRUC, carrierName, carrierAddress string) ([]byte, error) {
	adv, err := newDespatchAdviceShell(d, lines, carrierRUC, carrierName)
	if err != nil {
		return nil, err
	}

	adv.DespatchAdviceTypeCode = newDespatchAdviceTypeCode("31")
	// The "supplier" in DespatchSupplierParty is the carrier itself.
	adv.DespatchSupplierParty = newDespatchSupplierParty(carrierRUC, carrierName, carrierAddress)
	// The "delivery customer" is the recipient of the goods.
	adv.DeliveryCustomerParty = newDeliveryCustomerParty(
		d.RecipientDocType, d.RecipientDocNumber, d.RecipientName, stringOrDefault(d.RecipientAddress, ""),
	)
	// The remitente (the party whose goods are being carried) is
	// Shipment/Delivery/Despatch/DespatchParty; the Transportista sheet and
	// its XSL know no cac:SellerSupplierParty. The handler passes its RUC/name
	// through the CarrierRUC/CarrierName fields on the Despatch model — on a
	// Transportista guía those fields hold the remitente, not a third-party
	// carrier.
	if d.CarrierRUC != nil && *d.CarrierRUC != "" {
		remitente := party{
			PartyIdentification: partyIdentification{
				ID: schemeID{
					Value:            *d.CarrierRUC,
					SchemeID:         "6",
					SchemeName:       "Documento de Identidad",
					SchemeAgencyName: "PE:SUNAT",
					SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06",
				},
			},
		}
		if d.CarrierName != nil {
			remitente.PartyLegalEntity.RegistrationName = *d.CarrierName
		}
		adv.Shipment.Delivery.Despatch.DespatchParty = &remitente
	}
	adv.AdditionalDocumentReferences = buildRelatedDocumentReferences(d.RelatedDocuments)

	return marshalISO8859(&adv)
}
