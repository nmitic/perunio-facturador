package xmlbuilder

import (
	"github.com/perunio/perunio-facturador/internal/model"
)

// buildDespatchTransportistaXML creates a UBL 2.1 DespatchAdvice for a
// Transportista (Cat.01 = 31) GRE. The issuing company is the third-
// party carrier, so DespatchSupplierParty holds the carrier's RUC and
// name. SellerSupplierParty references the remitente (the party whose
// goods are being carried). Transport mode is always public — driver
// and vehicle data are mandatory on the carrier side.
//
// The remitente fields are taken from the Despatch's recipient block
// when the carrier is the one issuing (the handler is responsible for
// populating RecipientDocNumber with the remitente's RUC).
func buildDespatchTransportistaXML(d *model.Despatch, lines []model.DespatchLine, carrierRUC, carrierName, carrierAddress string) ([]byte, error) {
	adv := newDespatchAdviceShell(d, lines, carrierRUC, carrierName)

	adv.DespatchAdviceTypeCode = newDespatchAdviceTypeCode("31")
	// The "supplier" in DespatchSupplierParty is the carrier itself.
	adv.DespatchSupplierParty = newDespatchSupplierParty(carrierRUC, carrierName, carrierAddress)
	// The "delivery customer" is the recipient of the goods.
	adv.DeliveryCustomerParty = newDeliveryCustomerParty(
		d.RecipientDocType, d.RecipientDocNumber, d.RecipientName, stringOrDefault(d.RecipientAddress, ""),
	)
	// The remitente (the party whose goods are being carried) is
	// referenced via SellerSupplierParty. The handler passes its
	// RUC/name through the CarrierRUC/CarrierName fields on the
	// Despatch model — on a Transportista guía those fields hold
	// the remitente, not the third-party carrier.
	if d.CarrierRUC != nil && *d.CarrierRUC != "" {
		remitenteName := ""
		if d.CarrierName != nil {
			remitenteName = *d.CarrierName
		}
		adv.SellerSupplierParty = newSellerSupplierParty(*d.CarrierRUC, remitenteName)
	}

	return marshalISO8859(&adv)
}
