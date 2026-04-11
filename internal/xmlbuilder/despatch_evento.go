package xmlbuilder

import (
	"fmt"

	"github.com/perunio/perunio-facturador/internal/model"
)

// buildDespatchEventoXML creates a UBL 2.1 DespatchAdvice "por Eventos"
// used to supersede an in-transit GRE when a qualifying event happens
// (vehicle breakdown, transbordo, etc. — see Cat.59).
//
// The XML still carries a DespatchAdviceTypeCode of 09 or 31 — the
// base DocType on the Despatch model tells us which. A por-eventos
// GRE differs from the base flavor in that it must reference:
//
//  1. the original GRE being superseded, via cac:AdditionalDocumentReference
//  2. the Cat.59 event code, via cbc:DocumentTypeCode on that same reference
func buildDespatchEventoXML(d *model.Despatch, lines []model.DespatchLine, ruc, companyName, companyAddress string, baseDocType string) ([]byte, error) {
	if baseDocType == "" {
		baseDocType = "09"
	}
	if baseDocType != "09" && baseDocType != "31" {
		return nil, fmt.Errorf("por-eventos base doc type must be 09 or 31, got %q", baseDocType)
	}
	if d.OriginalGreID == nil || *d.OriginalGreID == "" {
		return nil, fmt.Errorf("por-eventos GRE requires OriginalGreID")
	}
	if d.EventCode == nil || *d.EventCode == "" {
		return nil, fmt.Errorf("por-eventos GRE requires EventCode (Cat.59)")
	}

	adv := newDespatchAdviceShell(d, lines, ruc, companyName)

	adv.DespatchAdviceTypeCode = newDespatchAdviceTypeCode(baseDocType)
	adv.DespatchSupplierParty = newDespatchSupplierParty(ruc, companyName, companyAddress)
	adv.DeliveryCustomerParty = newDeliveryCustomerParty(
		d.RecipientDocType, d.RecipientDocNumber, d.RecipientName, stringOrDefault(d.RecipientAddress, ""),
	)

	// The reference to the original GRE plus the event code. SUNAT's
	// guía por eventos pattern is: DocumentTypeCode = Cat.59 event
	// code, ID = <original GRE filename identifier>.
	adv.AdditionalDocumentReferences = []additionalDocumentReference{
		{
			ID:               *d.OriginalGreID,
			DocumentTypeCode: *d.EventCode,
		},
	}

	return marshalISO8859(&adv)
}
