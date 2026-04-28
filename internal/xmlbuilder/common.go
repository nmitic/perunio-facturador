package xmlbuilder

import "encoding/xml"

const (
	nsDS       = "http://www.w3.org/2000/09/xmldsig#"
	algC14NWC  = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments"
	algRSASHA1 = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	algSHA1    = "http://www.w3.org/2000/09/xmldsig#sha1"
	algEnvSig  = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
)

// ublExtensions wraps the signature extension placeholder.
type ublExtensions struct {
	XMLName   xml.Name       `xml:"ext:UBLExtensions"`
	Extension []ublExtension `xml:"ext:UBLExtension"`
}

type ublExtension struct {
	ExtensionContent extensionContent `xml:"ext:ExtensionContent"`
}

// extensionContent holds the XMLDSig template that xmlsec1 will fill.
type extensionContent struct {
	Signature dsSignatureTemplate `xml:"ds:Signature"`
}

type dsSignatureTemplate struct {
	XMLNS_DS       string           `xml:"xmlns:ds,attr"`
	Id             string           `xml:"Id,attr"`
	SignedInfo     dsSignedInfo     `xml:"ds:SignedInfo"`
	SignatureValue string           `xml:"ds:SignatureValue"`
	KeyInfo        dsKeyInfo        `xml:"ds:KeyInfo"`
}

type dsSignedInfo struct {
	CanonicalizationMethod dsAlgorithm `xml:"ds:CanonicalizationMethod"`
	SignatureMethod        dsAlgorithm `xml:"ds:SignatureMethod"`
	Reference              dsReference `xml:"ds:Reference"`
}

type dsAlgorithm struct {
	Algorithm string `xml:"Algorithm,attr"`
}

type dsReference struct {
	URI          string       `xml:"URI,attr"`
	Transforms   dsTransforms `xml:"ds:Transforms"`
	DigestMethod dsAlgorithm  `xml:"ds:DigestMethod"`
	DigestValue  string       `xml:"ds:DigestValue"`
}

type dsTransforms struct {
	Transform dsAlgorithm `xml:"ds:Transform"`
}

type dsKeyInfo struct {
	X509Data dsX509Data `xml:"ds:X509Data"`
}

type dsX509Data struct {
	X509Certificate string `xml:"ds:X509Certificate"`
}

func newExtensionContent() extensionContent {
	return extensionContent{
		Signature: dsSignatureTemplate{
			XMLNS_DS: nsDS,
			Id:       "signatureKG",
			SignedInfo: dsSignedInfo{
				CanonicalizationMethod: dsAlgorithm{Algorithm: algC14NWC},
				SignatureMethod:        dsAlgorithm{Algorithm: algRSASHA1},
				Reference: dsReference{
					URI: "",
					Transforms: dsTransforms{
						Transform: dsAlgorithm{Algorithm: algEnvSig},
					},
					DigestMethod: dsAlgorithm{Algorithm: algSHA1},
				},
			},
		},
	}
}

// cacSignature is the cac:Signature reference element (metadata, not actual XMLDSig).
type cacSignature struct {
	XMLName                    xml.Name                   `xml:"cac:Signature"`
	ID                         string                     `xml:"cbc:ID"`
	SignatoryParty              signatoryParty             `xml:"cac:SignatoryParty"`
	DigitalSignatureAttachment digitalSignatureAttachment `xml:"cac:DigitalSignatureAttachment"`
}

type signatoryParty struct {
	PartyIdentification partyIdentification `xml:"cac:PartyIdentification"`
	PartyName           partyName           `xml:"cac:PartyName"`
}

type digitalSignatureAttachment struct {
	ExternalReference externalReference `xml:"cac:ExternalReference"`
}

type externalReference struct {
	URI string `xml:"cbc:URI"`
}

func newCACSignature(ruc, companyName string) cacSignature {
	return cacSignature{
		ID: "IDSignKG",
		SignatoryParty: signatoryParty{
			PartyIdentification: partyIdentification{ID: schemeID{Value: ruc}},
			PartyName:           partyName{Name: companyName},
		},
		DigitalSignatureAttachment: digitalSignatureAttachment{
			ExternalReference: externalReference{URI: "#signatureKG"},
		},
	}
}

// accountingSupplierParty represents the supplier (emisor).
type accountingSupplierParty struct {
	XMLName xml.Name `xml:"cac:AccountingSupplierParty"`
	Party   party    `xml:"cac:Party"`
}

// accountingCustomerParty represents the customer (adquiriente).
type accountingCustomerParty struct {
	XMLName xml.Name `xml:"cac:AccountingCustomerParty"`
	Party   party    `xml:"cac:Party"`
}

type party struct {
	PartyIdentification partyIdentification `xml:"cac:PartyIdentification"`
	PartyName           *partyName          `xml:"cac:PartyName,omitempty"`
	PartyLegalEntity    partyLegalEntity    `xml:"cac:PartyLegalEntity"`
}

type partyIdentification struct {
	ID schemeID `xml:"cbc:ID"`
}

type partyName struct {
	Name string `xml:"cbc:Name"`
}

type partyLegalEntity struct {
	RegistrationName    string               `xml:"cbc:RegistrationName"`
	RegistrationAddress *registrationAddress `xml:"cac:RegistrationAddress,omitempty"`
}

type registrationAddress struct {
	AddressTypeCode *addressTypeCode `xml:"cbc:AddressTypeCode,omitempty"`
	CityName        string           `xml:"cbc:CityName,omitempty"`
	CountrySubentity string          `xml:"cbc:CountrySubentity,omitempty"`
	District        string           `xml:"cbc:District,omitempty"`
	AddressLine     *addressLine     `xml:"cac:AddressLine,omitempty"`
	Country         *country         `xml:"cac:Country,omitempty"`
}

type addressTypeCode struct {
	Value    string `xml:",chardata"`
	ListAgencyName string `xml:"listAgencyName,attr,omitempty"`
	ListName       string `xml:"listName,attr,omitempty"`
}

type addressLine struct {
	Line string `xml:"cbc:Line"`
}

type country struct {
	IdentificationCode string `xml:"cbc:IdentificationCode"`
}

// schemeID is a cbc element with scheme attributes (used for identity documents).
type schemeID struct {
	Value            string `xml:",chardata"`
	SchemeID         string `xml:"schemeID,attr,omitempty"`
	SchemeName       string `xml:"schemeName,attr,omitempty"`
	SchemeAgencyName string `xml:"schemeAgencyName,attr,omitempty"`
	SchemeURI        string `xml:"schemeURI,attr,omitempty"`
}

// cbcID is a simple cbc:ID element.
type cbcID struct {
	Value string `xml:",chardata"`
}

// currencyAmount is a numeric element with currencyID attribute.
type currencyAmount struct {
	Value      string `xml:",chardata"`
	CurrencyID string `xml:"currencyID,attr"`
}

// taxTotal represents a document-level or line-level tax total.
type taxTotal struct {
	XMLName     xml.Name      `xml:"cac:TaxTotal"`
	TaxAmount   currencyAmount `xml:"cbc:TaxAmount"`
	TaxSubtotal []taxSubtotal  `xml:"cac:TaxSubtotal"`
}

type taxSubtotal struct {
	TaxableAmount currencyAmount `xml:"cbc:TaxableAmount"`
	TaxAmount     currencyAmount `xml:"cbc:TaxAmount"`
	TaxCategory   taxCategory    `xml:"cac:TaxCategory"`
}

type taxCategory struct {
	ID        taxCategoryID `xml:"cbc:ID"`
	Percent   string        `xml:"cbc:Percent,omitempty"`
	TierRange string        `xml:"cbc:TierRange,omitempty"` // ISC calculation system (Cat.08)
	TaxExemptionReasonCode *taxExemptionCode `xml:"cbc:TaxExemptionReasonCode,omitempty"`
	TaxScheme taxSchemeXML  `xml:"cac:TaxScheme"`
}

type taxCategoryID struct {
	Value          string `xml:",chardata"`
	SchemeID       string `xml:"schemeID,attr,omitempty"`
	SchemeAgencyID string `xml:"schemeAgencyID,attr,omitempty"`
}

type taxExemptionCode struct {
	Value          string `xml:",chardata"`
	ListAgencyName string `xml:"listAgencyName,attr,omitempty"`
	ListName       string `xml:"listName,attr,omitempty"`
	ListURI        string `xml:"listURI,attr,omitempty"`
}

type taxSchemeXML struct {
	ID          taxSchemeID `xml:"cbc:ID"`
	Name        string      `xml:"cbc:Name,omitempty"`
	TaxTypeCode string      `xml:"cbc:TaxTypeCode,omitempty"`
}

type taxSchemeID struct {
	Value          string `xml:",chardata"`
	SchemeID       string `xml:"schemeID,attr,omitempty"`
	SchemeAgencyID string `xml:"schemeAgencyID,attr,omitempty"`
}

// legalMonetaryTotal represents the totals block.
type legalMonetaryTotal struct {
	XMLName              xml.Name       `xml:"cac:LegalMonetaryTotal"`
	LineExtensionAmount  currencyAmount `xml:"cbc:LineExtensionAmount"`
	TaxInclusiveAmount   currencyAmount `xml:"cbc:TaxInclusiveAmount"`
	AllowanceTotalAmount *currencyAmount `xml:"cbc:AllowanceTotalAmount,omitempty"`
	ChargeTotalAmount    *currencyAmount `xml:"cbc:ChargeTotalAmount,omitempty"`
	PayableAmount        currencyAmount `xml:"cbc:PayableAmount"`
}

// invoiceLine represents a single line item (for Invoice).
type invoiceLine struct {
	XMLName           xml.Name       `xml:"cac:InvoiceLine"`
	ID                string         `xml:"cbc:ID"`
	InvoicedQuantity  quantity       `xml:"cbc:InvoicedQuantity"`
	LineExtensionAmount currencyAmount `xml:"cbc:LineExtensionAmount"`
	PricingReference  pricingReference `xml:"cac:PricingReference"`
	TaxTotal          taxTotal       `xml:"cac:TaxTotal"`
	Item              item           `xml:"cac:Item"`
	Price             price          `xml:"cac:Price"`
}

// creditNoteLine represents a single line item (for CreditNote).
type creditNoteLine struct {
	XMLName             xml.Name         `xml:"cac:CreditNoteLine"`
	ID                  string           `xml:"cbc:ID"`
	CreditedQuantity    quantity         `xml:"cbc:CreditedQuantity"`
	LineExtensionAmount currencyAmount   `xml:"cbc:LineExtensionAmount"`
	PricingReference    pricingReference `xml:"cac:PricingReference"`
	TaxTotal            taxTotal         `xml:"cac:TaxTotal"`
	Item                item             `xml:"cac:Item"`
	Price               price            `xml:"cac:Price"`
}

// debitNoteLine represents a single line item (for DebitNote).
type debitNoteLine struct {
	XMLName             xml.Name         `xml:"cac:DebitNoteLine"`
	ID                  string           `xml:"cbc:ID"`
	DebitedQuantity     quantity         `xml:"cbc:DebitedQuantity"`
	LineExtensionAmount currencyAmount   `xml:"cbc:LineExtensionAmount"`
	PricingReference    pricingReference `xml:"cac:PricingReference"`
	TaxTotal            taxTotal         `xml:"cac:TaxTotal"`
	Item                item             `xml:"cac:Item"`
	Price               price            `xml:"cac:Price"`
}

type quantity struct {
	Value    string `xml:",chardata"`
	UnitCode string `xml:"unitCode,attr"`
}

type pricingReference struct {
	AlternativeConditionPrice alternativeConditionPrice `xml:"cac:AlternativeConditionPrice"`
}

type alternativeConditionPrice struct {
	PriceAmount   currencyAmount `xml:"cbc:PriceAmount"`
	PriceTypeCode priceTypeCode  `xml:"cbc:PriceTypeCode"`
}

type priceTypeCode struct {
	Value          string `xml:",chardata"`
	ListName       string `xml:"listName,attr,omitempty"`
	ListAgencyName string `xml:"listAgencyName,attr,omitempty"`
	ListURI        string `xml:"listURI,attr,omitempty"`
}

type item struct {
	Description string `xml:"cbc:Description"`
}

type price struct {
	PriceAmount currencyAmount `xml:"cbc:PriceAmount"`
}

// noteElement represents a cbc:Note with languageLocaleID.
type noteElement struct {
	XMLName          xml.Name `xml:"cbc:Note"`
	Value            string   `xml:",chardata"`
	LanguageLocaleID string   `xml:"languageLocaleID,attr,omitempty"`
}

// invoiceTypeCode with SUNAT-required attributes.
type invoiceTypeCode struct {
	XMLName        xml.Name `xml:"cbc:InvoiceTypeCode"`
	Value          string   `xml:",chardata"`
	ListAgencyName string   `xml:"listAgencyName,attr"`
	ListName       string   `xml:"listName,attr"`
	ListURI        string   `xml:"listURI,attr"`
}

// documentCurrencyCode with required attributes.
type documentCurrencyCode struct {
	XMLName        xml.Name `xml:"cbc:DocumentCurrencyCode"`
	Value          string   `xml:",chardata"`
	ListID         string   `xml:"listID,attr"`
	ListName       string   `xml:"listName,attr"`
	ListAgencyName string   `xml:"listAgencyName,attr"`
}

// discrepancyResponse for NC/ND.
type discrepancyResponse struct {
	XMLName      xml.Name `xml:"cac:DiscrepancyResponse"`
	ReferenceID  string   `xml:"cbc:ReferenceID"`
	ResponseCode string   `xml:"cbc:ResponseCode"`
	Description  string   `xml:"cbc:Description"`
}

// billingReference for NC/ND.
type billingReference struct {
	XMLName                  xml.Name                 `xml:"cac:BillingReference"`
	InvoiceDocumentReference invoiceDocumentReference `xml:"cac:InvoiceDocumentReference"`
}

type invoiceDocumentReference struct {
	ID               string `xml:"cbc:ID"`
	DocumentTypeCode string `xml:"cbc:DocumentTypeCode"`
}

// Helper constructors.

func newSupplierParty(ruc, name, address, establishmentCode string) accountingSupplierParty {
	p := accountingSupplierParty{
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

	if address != "" || establishmentCode != "" {
		regAddr := &registrationAddress{
			Country: &country{IdentificationCode: "PE"},
		}
		if establishmentCode != "" {
			regAddr.AddressTypeCode = &addressTypeCode{
				Value:          establishmentCode,
				ListAgencyName: "PE:SUNAT",
				ListName:       "Establecimientos anexos",
			}
		}
		if address != "" {
			regAddr.AddressLine = &addressLine{Line: address}
		}
		p.Party.PartyLegalEntity.RegistrationAddress = regAddr
	}

	return p
}

func newCustomerParty(docType, docNumber, name, address string) accountingCustomerParty {
	p := accountingCustomerParty{
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

func newCurrencyAmount(amount, currency string) currencyAmount {
	return currencyAmount{Value: amount, CurrencyID: currency}
}

func newDocumentCurrencyCode(code string) documentCurrencyCode {
	return documentCurrencyCode{
		Value:          code,
		ListID:         "ISO 4217 Alpha",
		ListName:       "Currency",
		ListAgencyName: "United Nations Economic Commission for Europe",
	}
}

func newInvoiceTypeCode(code string) invoiceTypeCode {
	return invoiceTypeCode{
		Value:          code,
		ListAgencyName: "PE:SUNAT",
		ListName:       "Tipo de Documento",
		ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01",
	}
}
