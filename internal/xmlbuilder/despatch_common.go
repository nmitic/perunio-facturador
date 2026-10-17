package xmlbuilder

import (
	"encoding/xml"
	"fmt"
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// despatchAdvice is the UBL 2.1 DespatchAdvice root used by all three
// GRE flavors (Remitente 09, Transportista 31, por Eventos). Unlike
// Invoice there is no TaxTotal, LegalMonetaryTotal, PricingReference
// or Price — GRE carries goods metadata only.
type despatchAdvice struct {
	XMLName  xml.Name `xml:"DespatchAdvice"`
	XMLNS    string   `xml:"xmlns,attr"`
	XMLNSCAC string   `xml:"xmlns:cac,attr"`
	XMLNSCBC string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT string   `xml:"xmlns:ext,attr"`
	XMLNSDS  string   `xml:"xmlns:ds,attr"`
	XMLNSSAC string   `xml:"xmlns:sac,attr"`

	UBLExtensions   ublExtensions
	UBLVersionID    string `xml:"cbc:UBLVersionID"`
	CustomizationID string `xml:"cbc:CustomizationID"`
	ID              string `xml:"cbc:ID"`
	IssueDate       string `xml:"cbc:IssueDate"`
	IssueTime       string `xml:"cbc:IssueTime,omitempty"`

	DespatchAdviceTypeCode despatchAdviceTypeCode

	Notes []noteElement `xml:"cbc:Note,omitempty"`

	AdditionalDocumentReferences []additionalDocumentReference `xml:"cac:AdditionalDocumentReference,omitempty"`

	Signature cacSignature

	DespatchSupplierParty despatchSupplierParty
	DeliveryCustomerParty deliveryCustomerParty

	Shipment      shipment
	DespatchLines []despatchLineXML
}

// despatchAdviceTypeCode is the Cat.01 type code for the GRE (09 or 31).
type despatchAdviceTypeCode struct {
	XMLName        xml.Name `xml:"cbc:DespatchAdviceTypeCode"`
	Value          string   `xml:",chardata"`
	ListAgencyName string   `xml:"listAgencyName,attr,omitempty"`
	ListName       string   `xml:"listName,attr,omitempty"`
	ListURI        string   `xml:"listURI,attr,omitempty"`
}

func newDespatchAdviceTypeCode(code string) despatchAdviceTypeCode {
	return despatchAdviceTypeCode{
		Value:          code,
		ListAgencyName: "PE:SUNAT",
		ListName:       "Tipo de Documento",
		ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01",
	}
}

// additionalDocumentReference carries references to related documents
// (used heavily by por Eventos to point at the original GRE, by Remitente to
// reference the commercial invoice, and by Invoice regularizaciones to
// reference facturas/boletas de anticipo). DocumentStatusCode and IssuerParty
// are the anticipo-only fields: the row number pairing the reference with its
// cac:PrepaidPayment, and the RUC of the anticipo's emitter. Field order
// follows the UBL DocumentReference sequence.
type additionalDocumentReference struct {
	ID                 string             `xml:"cbc:ID"`
	DocumentTypeCode   string             `xml:"cbc:DocumentTypeCode,omitempty"`
	DocumentType       string             `xml:"cbc:DocumentType,omitempty"`
	DocumentStatusCode string             `xml:"cbc:DocumentStatusCode,omitempty"`
	IssuerParty        *docRefIssuerParty `xml:"cac:IssuerParty,omitempty"`
}

// despatchSupplierParty is the shipper (remitente for type 09, carrier
// for type 31). Uses the same `party` structure as AccountingSupplierParty.
type despatchSupplierParty struct {
	XMLName xml.Name `xml:"cac:DespatchSupplierParty"`
	Party   party    `xml:"cac:Party"`
}

// deliveryCustomerParty is the recipient (destinatario).
type deliveryCustomerParty struct {
	XMLName xml.Name `xml:"cac:DeliveryCustomerParty"`
	Party   party    `xml:"cac:Party"`
}

// shipment carries the transport metadata: weight, packages, mode,
// origin/destination addresses, driver and vehicle.
type shipment struct {
	XMLName      xml.Name     `xml:"cac:Shipment"`
	ID           string       `xml:"cbc:ID"`
	HandlingCode handlingCode `xml:"cbc:HandlingCode"`
	// HandlingInstructions is the description of the motivo de traslado,
	// required for motivo 13 Otros (ERROR 3457). cbc:Information is reserved
	// for motivos 08/09/19 (ERROR 3418 on any other) and is not emitted.
	HandlingInstructions               string        `xml:"cbc:HandlingInstructions,omitempty"`
	GrossWeightMeasure                 weightMeasure `xml:"cbc:GrossWeightMeasure"`
	TotalTransportHandlingUnitQuantity string        `xml:"cbc:TotalTransportHandlingUnitQuantity,omitempty"`
	SplitConsignmentIndicator          string        `xml:"cbc:SplitConsignmentIndicator,omitempty"`
	ShipmentStage                      shipmentStage `xml:"cac:ShipmentStage"`
	Delivery                           delivery      `xml:"cac:Delivery"`
	// TransportHandlingUnit carries the vehicle plate on GRE 2.0
	// (TransportEquipment/cbc:ID, ERROR 2566).
	TransportHandlingUnit *transportHandlingUnit `xml:"cac:TransportHandlingUnit,omitempty"`
}

// transportHandlingUnit holds the vehicle(s): GRE 2.0 reads the plate from
// TransportEquipment/cbc:ID. ShipmentStage/TransportMeans/RoadTransport is the
// GRE 1.0 shape, which no GRE 2.0 rule or validator reads.
type transportHandlingUnit struct {
	TransportEquipment []transportEquipment `xml:"cac:TransportEquipment"`
}

type transportEquipment struct {
	ID string `xml:"cbc:ID"`
}

// handlingCode carries the Cat.20 transfer reason code.
type handlingCode struct {
	Value          string `xml:",chardata"`
	ListAgencyName string `xml:"listAgencyName,attr,omitempty"`
	ListName       string `xml:"listName,attr,omitempty"`
	ListURI        string `xml:"listURI,attr,omitempty"`
}

func newHandlingCode(code string) handlingCode {
	return handlingCode{
		Value:          code,
		ListAgencyName: "PE:SUNAT",
		ListName:       "Motivo de traslado",
		ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo20",
	}
}

// weightMeasure is `<cbc:GrossWeightMeasure unitCode="KGM">125.00</>`.
type weightMeasure struct {
	Value    string `xml:",chardata"`
	UnitCode string `xml:"unitCode,attr"`
}

// shipmentStage carries the transport mode (Cat.18), transit period,
// and either a carrier (public transport) or driver+vehicle (private).
type shipmentStage struct {
	XMLName           xml.Name       `xml:"cac:ShipmentStage"`
	TransportModeCode string         `xml:"cbc:TransportModeCode"`
	TransitPeriod     *transitPeriod `xml:"cac:TransitPeriod,omitempty"`
	CarrierParty      *carrierParty  `xml:"cac:CarrierParty,omitempty"`
	DriverPersons     []driverPerson `xml:"cac:DriverPerson,omitempty"`
}

type transitPeriod struct {
	StartDate string `xml:"cbc:StartDate,omitempty"`
}

// carrierParty (transporte público) — RUC + name of the third-party carrier.
type carrierParty struct {
	XMLName             xml.Name            `xml:"cac:CarrierParty"`
	PartyIdentification partyIdentification `xml:"cac:PartyIdentification"`
	PartyName           *partyName          `xml:"cac:PartyName,omitempty"`
	PartyLegalEntity    *partyLegalEntity   `xml:"cac:PartyLegalEntity,omitempty"`
}

// driverPerson (transporte privado) — driver identity + license.
type driverPerson struct {
	XMLName            xml.Name `xml:"cac:DriverPerson"`
	ID                 schemeID `xml:"cbc:ID"`
	FirstName          string   `xml:"cbc:FirstName,omitempty"`
	FamilyName         string   `xml:"cbc:FamilyName,omitempty"`
	JobTitle           string   `xml:"cbc:JobTitle,omitempty"`
	IdentificationCard string   `xml:"cac:IdentityDocumentReference>cbc:ID,omitempty"`
}

// delivery carries the arrival address (punto de llegada) and, in
// cac:Despatch, the punto de partida. GRE 2.0 reads the origin from
// Delivery/Despatch/DespatchAddress (ERROR 2775/2577); Shipment/OriginAddress
// is the GRE 1.0 shape.
type delivery struct {
	XMLName         xml.Name         `xml:"cac:Delivery"`
	DeliveryAddress despatchAddress  `xml:"cac:DeliveryAddress"`
	Despatch        deliveryDespatch `xml:"cac:Despatch"`
}

type deliveryDespatch struct {
	DespatchAddress despatchAddress `xml:"cac:DespatchAddress"`
	// DespatchParty is the remitente on a GRE transportista (31): the party
	// whose goods travel (ERROR 3383 / 2541 / 3387).
	DespatchParty *party `xml:"cac:DespatchParty,omitempty"`
}

// despatchAddress is a 6-digit ubigeo + street for both origin and
// arrival points. GRE-specific structure: <cbc:ID> is the ubigeo.
type despatchAddress struct {
	ID          string       `xml:"cbc:ID"`
	StreetName  string       `xml:"cbc:StreetName,omitempty"`
	AddressLine *addressLine `xml:"cac:AddressLine,omitempty"`
	Country     *country     `xml:"cac:Country,omitempty"`
}

// despatchLineXML is the cac:DespatchLine element — GRE carries only
// id, delivered quantity and item description (no prices).
type despatchLineXML struct {
	XMLName            xml.Name            `xml:"cac:DespatchLine"`
	ID                 string              `xml:"cbc:ID"`
	DeliveredQuantity  quantity            `xml:"cbc:DeliveredQuantity"`
	OrderLineReference *orderLineReference `xml:"cac:OrderLineReference,omitempty"`
	Item               despatchItem        `xml:"cac:Item"`
}

type orderLineReference struct {
	LineID string `xml:"cbc:LineID"`
}

// despatchItem is a GRE line item — description + optional seller code.
type despatchItem struct {
	Description               string                     `xml:"cbc:Description"`
	SellersItemIdentification *sellersItemIdentification `xml:"cac:SellersItemIdentification,omitempty"`
}

type sellersItemIdentification struct {
	ID string `xml:"cbc:ID"`
}

// -----------------------------------------------------------------------------
// Shared helpers used by the three despatch builders.
// -----------------------------------------------------------------------------

// newDespatchSupplierParty builds the DespatchSupplierParty block
// (the transferor — or the carrier on type 31).
func newDespatchSupplierParty(ruc, name, address string) despatchSupplierParty {
	p := despatchSupplierParty{
		Party: party{
			PartyIdentification: partyIdentification{
				ID: schemeID{
					Value:            ruc,
					SchemeID:         "6",
					SchemeName:       "Documento de Identidad",
					SchemeAgencyName: "PE:SUNAT",
					SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06",
				},
			},
			PartyLegalEntity: partyLegalEntity{
				RegistrationName: name,
			},
		},
	}
	if address != "" {
		p.Party.PartyLegalEntity.RegistrationAddress = &registrationAddress{
			AddressLine: &addressLine{Line: address},
			Country:     &country{IdentificationCode: "PE"},
		}
	}
	return p
}

// newDeliveryCustomerParty builds the DeliveryCustomerParty block
// (the recipient — destinatario).
func newDeliveryCustomerParty(docType, docNumber, name, address string) deliveryCustomerParty {
	p := deliveryCustomerParty{
		Party: party{
			PartyIdentification: partyIdentification{
				ID: schemeID{
					Value:            docNumber,
					SchemeID:         docType,
					SchemeName:       "Documento de Identidad",
					SchemeAgencyName: "PE:SUNAT",
					SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06",
				},
			},
			PartyLegalEntity: partyLegalEntity{
				RegistrationName: name,
			},
		},
	}
	if address != "" {
		p.Party.PartyLegalEntity.RegistrationAddress = &registrationAddress{
			AddressLine: &addressLine{Line: address},
			Country:     &country{IdentificationCode: "PE"},
		}
	}
	return p
}

// newDespatchAddress builds an origin/arrival address block with a
// 6-digit ubigeo (cbc:ID) and free-form street.
func newDespatchAddress(ubigeo, street string) despatchAddress {
	return despatchAddress{
		ID:          ubigeo,
		StreetName:  street,
		AddressLine: &addressLine{Line: street},
		Country:     &country{IdentificationCode: "PE"},
	}
}

// buildShipment assembles the cac:Shipment block from the Despatch
// model. Handles the public/private transport branching.
func buildShipment(d *model.Despatch) shipment {
	ship := shipment{
		ID:           "SUNAT_Envio",
		HandlingCode: newHandlingCode(d.TransferReason),
		GrossWeightMeasure: weightMeasure{
			Value:    d.TotalWeightKg,
			UnitCode: weightUnitOrDefault(d.WeightUnitCode),
		},
		ShipmentStage: shipmentStage{
			TransportModeCode: d.TransportModality,
		},
		Delivery: delivery{
			DeliveryAddress: newDespatchAddress(d.ArrivalUbigeo, d.ArrivalAddress),
			Despatch: deliveryDespatch{
				DespatchAddress: newDespatchAddress(d.StartUbigeo, d.StartAddress),
			},
		},
	}
	if d.TransferReasonDesc != nil && *d.TransferReasonDesc != "" {
		// One line, as SUNAT's XSL wants it (no line breaks; 4190 otherwise).
		ship.HandlingInstructions = strings.Join(strings.Fields(*d.TransferReasonDesc), " ")
	}
	if d.TotalPackages != nil {
		ship.TotalTransportHandlingUnitQuantity = itoa(*d.TotalPackages)
	}
	if d.StartDate != nil {
		ship.ShipmentStage.TransitPeriod = &transitPeriod{
			StartDate: d.StartDate.Format("2006-01-02"),
		}
	}

	// Remitente (09) with transporte público → CarrierParty (the third-party
	// carrier's RUC + name). Otherwise → the vehicle plate + DriverPerson. A
	// GRE transportista (31) always declares its own vehicle and driver and
	// never a CarrierParty (ERROR 2566, 3357): the carrier is the issuer, so a
	// transporte público modalidad saved on its draft changes nothing.
	if d.DocType != sunat.GreDocTypeTransportista && d.TransportModality == model.TransportModalityPublic {
		if d.CarrierRUC != nil && *d.CarrierRUC != "" {
			cp := &carrierParty{
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
			if d.CarrierName != nil && *d.CarrierName != "" {
				cp.PartyLegalEntity = &partyLegalEntity{RegistrationName: *d.CarrierName}
			}
			ship.ShipmentStage.CarrierParty = cp
		}
	} else {
		// Private transport — emit driver + vehicle when present.
		if d.VehiclePlate != nil && *d.VehiclePlate != "" {
			ship.TransportHandlingUnit = &transportHandlingUnit{
				TransportEquipment: []transportEquipment{{ID: *d.VehiclePlate}},
			}
		}
		if d.DriverDocNumber != nil && *d.DriverDocNumber != "" {
			dp := driverPerson{
				ID: schemeID{
					Value:            *d.DriverDocNumber,
					SchemeID:         stringOrDefault(d.DriverDocType, "1"),
					SchemeName:       "Documento de Identidad",
					SchemeAgencyName: "PE:SUNAT",
					SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06",
				},
				JobTitle: "Principal",
			}
			dp.FirstName, dp.FamilyName = driverNames(d)
			if d.DriverLicense != nil {
				dp.IdentificationCard = *d.DriverLicense
			}
			ship.ShipmentStage.DriverPersons = []driverPerson{dp}
		}
	}

	return ship
}

func buildDespatchLines(lines []model.DespatchLine) []despatchLineXML {
	out := make([]despatchLineXML, 0, len(lines))
	for _, l := range lines {
		line := despatchLineXML{
			ID:                 itoa(l.LineNumber),
			DeliveredQuantity:  quantity{Value: l.Quantity, UnitCode: l.UnitCode},
			OrderLineReference: &orderLineReference{LineID: itoa(l.LineNumber)},
			Item: despatchItem{
				Description: l.Description,
			},
		}
		if l.ProductCode != nil && *l.ProductCode != "" {
			line.Item.SellersItemIdentification = &sellersItemIdentification{ID: *l.ProductCode}
		}
		out = append(out, line)
	}
	return out
}

// newDespatchAdviceShell constructs a DespatchAdvice XML struct with
// all the fields common to every GRE flavor populated — the three
// concrete builders overlay the variant-specific fields (type code,
// supplier/customer parties, seller party, references).
//
// cbc:IssueTime is mandatory on every GRE (ERROR 3437), so a despatch without
// one is refused rather than sent without the tag.
func newDespatchAdviceShell(d *model.Despatch, lines []model.DespatchLine, ruc, companyName string) (despatchAdvice, error) {
	if d.IssueTime == nil || *d.IssueTime == "" {
		return despatchAdvice{}, fmt.Errorf("despatch %s-%d: issueTime is required (cbc:IssueTime, SUNAT 3437)", d.Series, d.Correlative)
	}
	docID := fmt.Sprintf("%s-%08d", d.Series, d.Correlative)
	root := ublRootFor(sunat.UblDocumentDespatchAdvice)

	adv := despatchAdvice{
		XMLNS:    root.NS,
		XMLNSCAC: nsCAC,
		XMLNSCBC: nsCBC,
		XMLNSEXT: nsEXT,
		XMLNSDS:  nsDS,
		XMLNSSAC: nsSAC,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    root.UBLVersionID,
		CustomizationID: root.CustomizationID,
		ID:              docID,
		IssueDate:       d.IssueDate.Format("2006-01-02"),

		IssueTime: *d.IssueTime,

		Signature: newCACSignature(ruc, companyName),
		Shipment:  buildShipment(d),
	}
	adv.DespatchLines = buildDespatchLines(lines)
	return adv, nil
}

// -----------------------------------------------------------------------------
// Small internal helpers
// -----------------------------------------------------------------------------

// driverNames returns the principal driver's nombres (cbc:FirstName, ERROR
// 3360) and apellidos (cbc:FamilyName, ERROR 3361). Drafts saved before
// driverFamilyName existed hold the whole name in driverName; those are split
// at the first space, the Peruvian "nombres apellidos" order for a single
// given name ("JUAN PEREZ QUISPE" → "JUAN" / "PEREZ QUISPE").
func driverNames(d *model.Despatch) (first, family string) {
	first = strings.Join(strings.Fields(stringOrDefault(d.DriverName, "")), " ")
	family = strings.Join(strings.Fields(stringOrDefault(d.DriverFamilyName, "")), " ")
	if family == "" {
		if i := strings.Index(first, " "); i > 0 {
			first, family = first[:i], first[i+1:]
		}
	}
	return first, family
}

func weightUnitOrDefault(u string) string {
	if u == "" {
		return "KGM"
	}
	return u
}

func stringOrDefault(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
