package xmlbuilder

import (
	"encoding/xml"
	"fmt"

	"github.com/perunio/perunio-facturador/internal/model"
)

// UBL 2.1 DespatchAdvice namespace (distinct from Invoice/CreditNote).
const NSDespatchAdvice = "urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"

// despatchAdvice is the UBL 2.1 DespatchAdvice root used by all three
// GRE flavors (Remitente 09, Transportista 31, por Eventos). Unlike
// Invoice there is no TaxTotal, LegalMonetaryTotal, PricingReference
// or Price — GRE carries goods metadata only.
type despatchAdvice struct {
	XMLName         xml.Name `xml:"DespatchAdvice"`
	XMLNS           string   `xml:"xmlns,attr"`
	XMLNSCAC        string   `xml:"xmlns:cac,attr"`
	XMLNSCBC        string   `xml:"xmlns:cbc,attr"`
	XMLNSEXT        string   `xml:"xmlns:ext,attr"`
	XMLNSDS         string   `xml:"xmlns:ds,attr"`
	XMLNSSAC        string   `xml:"xmlns:sac,attr"`

	UBLExtensions   ublExtensions
	UBLVersionID    string `xml:"cbc:UBLVersionID"`
	CustomizationID string `xml:"cbc:CustomizationID"`
	ID              string `xml:"cbc:ID"`
	IssueDate       string `xml:"cbc:IssueDate"`
	IssueTime       string `xml:"cbc:IssueTime,omitempty"`

	DespatchAdviceTypeCode despatchAdviceTypeCode

	Notes []noteElement `xml:"cbc:Note,omitempty"`

	OrderReference              *orderReference              `xml:"cac:OrderReference,omitempty"`
	AdditionalDocumentReferences []additionalDocumentReference `xml:"cac:AdditionalDocumentReference,omitempty"`

	Signature cacSignature

	DespatchSupplierParty despatchSupplierParty
	DeliveryCustomerParty deliveryCustomerParty
	SellerSupplierParty   *sellerSupplierParty `xml:"cac:SellerSupplierParty,omitempty"`

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

// orderReference points to a related sales document (commonly the
// invoice backing the shipment). Optional per SUNAT.
type orderReference struct {
	ID string `xml:"cbc:ID"`
}

// additionalDocumentReference carries references to related documents
// (used heavily by por Eventos to point at the original GRE, and by
// Remitente to reference the commercial invoice).
type additionalDocumentReference struct {
	ID               string `xml:"cbc:ID"`
	DocumentTypeCode string `xml:"cbc:DocumentTypeCode,omitempty"`
	DocumentType     string `xml:"cbc:DocumentType,omitempty"`
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

// sellerSupplierParty is the extra party used by Transportista (31):
// references the remitente (the party whose goods are being carried).
// For por Eventos with a type-31 issuer this is likewise the remitente.
type sellerSupplierParty struct {
	XMLName         xml.Name              `xml:"cac:SellerSupplierParty"`
	CustomerAssignedAccountID *schemeID   `xml:"cbc:CustomerAssignedAccountID,omitempty"`
	Party           party                 `xml:"cac:Party"`
}

// shipment carries the transport metadata: weight, packages, mode,
// origin/destination addresses, driver and vehicle.
type shipment struct {
	XMLName                           xml.Name               `xml:"cac:Shipment"`
	ID                                string                 `xml:"cbc:ID"`
	HandlingCode                      handlingCode           `xml:"cbc:HandlingCode"`
	Information                       string                 `xml:"cbc:Information,omitempty"`
	GrossWeightMeasure                weightMeasure          `xml:"cbc:GrossWeightMeasure"`
	TotalTransportHandlingUnitQuantity string                `xml:"cbc:TotalTransportHandlingUnitQuantity,omitempty"`
	SplitConsignmentIndicator         string                 `xml:"cbc:SplitConsignmentIndicator,omitempty"`
	ShipmentStage                     shipmentStage          `xml:"cac:ShipmentStage"`
	Delivery                          delivery               `xml:"cac:Delivery"`
	OriginAddress                     *despatchAddress       `xml:"cac:OriginAddress,omitempty"`
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
	XMLName           xml.Name           `xml:"cac:ShipmentStage"`
	TransportModeCode string             `xml:"cbc:TransportModeCode"`
	TransitPeriod     *transitPeriod     `xml:"cac:TransitPeriod,omitempty"`
	CarrierParty      *carrierParty      `xml:"cac:CarrierParty,omitempty"`
	TransportMeans    *transportMeans    `xml:"cac:TransportMeans,omitempty"`
	DriverPersons     []driverPerson     `xml:"cac:DriverPerson,omitempty"`
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

// transportMeans (transporte privado) — vehicle plates.
type transportMeans struct {
	RoadTransport     *roadTransport     `xml:"cac:RoadTransport,omitempty"`
	MeasurementDimensions []measurementDimension `xml:"cac:MeasurementDimension,omitempty"`
}

type roadTransport struct {
	LicensePlateID string `xml:"cbc:LicensePlateID"`
}

type measurementDimension struct {
	AttributeID     string        `xml:"cbc:AttributeID"`
	Measure         weightMeasure `xml:"cbc:Measure,omitempty"`
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

// delivery carries the arrival address (punto de llegada).
type delivery struct {
	XMLName         xml.Name         `xml:"cac:Delivery"`
	DeliveryAddress despatchAddress  `xml:"cac:DeliveryAddress"`
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
	XMLName            xml.Name          `xml:"cac:DespatchLine"`
	ID                 string            `xml:"cbc:ID"`
	DeliveredQuantity  quantity          `xml:"cbc:DeliveredQuantity"`
	OrderLineReference *orderLineReference `xml:"cac:OrderLineReference,omitempty"`
	Item               despatchItem      `xml:"cac:Item"`
}

type orderLineReference struct {
	LineID string `xml:"cbc:LineID"`
}

// despatchItem is a GRE line item — description + optional seller code.
type despatchItem struct {
	Description              string                    `xml:"cbc:Description"`
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

// newSellerSupplierParty — the remitente reference on type 31 despatches.
func newSellerSupplierParty(ruc, name string) *sellerSupplierParty {
	return &sellerSupplierParty{
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
		},
		OriginAddress: addrPtr(newDespatchAddress(d.StartUbigeo, d.StartAddress)),
	}
	if d.TransferReasonDesc != nil && *d.TransferReasonDesc != "" {
		ship.Information = *d.TransferReasonDesc
	}
	if d.TotalPackages != nil {
		ship.TotalTransportHandlingUnitQuantity = itoa(*d.TotalPackages)
	}
	if d.StartDate != nil {
		ship.ShipmentStage.TransitPeriod = &transitPeriod{
			StartDate: d.StartDate.Format("2006-01-02"),
		}
	}

	// Public transport → CarrierParty (third-party RUC + name).
	// Private transport → TransportMeans (plate) + DriverPerson (license).
	if d.TransportModality == model.TransportModalityPublic {
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
			ship.ShipmentStage.TransportMeans = &transportMeans{
				RoadTransport: &roadTransport{LicensePlateID: *d.VehiclePlate},
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
			if d.DriverName != nil {
				dp.FirstName = *d.DriverName
			}
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
			ID:                itoa(l.LineNumber),
			DeliveredQuantity: quantity{Value: l.Quantity, UnitCode: l.UnitCode},
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
func newDespatchAdviceShell(d *model.Despatch, lines []model.DespatchLine, ruc, companyName string) despatchAdvice {
	docID := fmt.Sprintf("%s-%08d", d.Series, d.Correlative)
	adv := despatchAdvice{
		XMLNS:    NSDespatchAdvice,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSEXT: NSEXT,
		XMLNSDS:  NSDS,
		XMLNSSAC: NSSAC,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    UBLVersion21,
		CustomizationID: CustomizationID20,
		ID:              docID,
		IssueDate:       d.IssueDate.Format("2006-01-02"),

		Signature: newCACSignature(ruc, companyName),
		Shipment:  buildShipment(d),
	}
	if d.IssueTime != nil && *d.IssueTime != "" {
		adv.IssueTime = *d.IssueTime
	}
	adv.DespatchLines = buildDespatchLines(lines)
	return adv
}

// -----------------------------------------------------------------------------
// Small internal helpers
// -----------------------------------------------------------------------------

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

func addrPtr(a despatchAddress) *despatchAddress { return &a }

func itoa(n int) string { return fmt.Sprintf("%d", n) }
