package xmlbuilder

import (
	"github.com/perunio/perunio-facturador/internal/model"
)

// buildDespatchRemitenteXML creates a UBL 2.1 DespatchAdvice for a
// Remitente (Cat.01 = 09) GRE. The issuing company is the transferor
// (DespatchSupplierParty); the consignee (DeliveryCustomerParty) is
// the destinatario. No SellerSupplierParty is emitted.
func buildDespatchRemitenteXML(d *model.Despatch, lines []model.DespatchLine, ruc, companyName, companyAddress string) ([]byte, error) {
	adv := newDespatchAdviceShell(d, lines, ruc, companyName)

	adv.DespatchAdviceTypeCode = newDespatchAdviceTypeCode("09")
	adv.DespatchSupplierParty = newDespatchSupplierParty(ruc, companyName, companyAddress)
	adv.DeliveryCustomerParty = newDeliveryCustomerParty(d.RecipientDocType, d.RecipientDocNumber, d.RecipientName, stringOrDefault(d.RecipientAddress, ""))

	// Optional reference to the commercial invoice or order.
	if d.RelatedDocSeries != nil && *d.RelatedDocSeries != "" &&
		d.RelatedDocNumber != nil && *d.RelatedDocNumber != "" {
		adv.OrderReference = &orderReference{
			ID: *d.RelatedDocSeries + "-" + *d.RelatedDocNumber,
		}
	}

	return marshalISO8859(&adv)
}
