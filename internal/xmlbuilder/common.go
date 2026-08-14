package xmlbuilder

import (
	"encoding/xml"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// The XMLDSig algorithm identifiers. These are W3C protocol constants, not SUNAT
// códigos — they stay here. The xmldsig namespace itself is not: it lives in
// ubl-namespace.json and reaches this package as nsDS in ubl.go.
const (
	algC14N     = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	algRSASHA1  = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	algSHA1     = "http://www.w3.org/2000/09/xmldsig#sha1"
	algEnvSig   = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	signatureID = "SignatureSP"
)

// ublExtensions wraps the signature extension placeholder.
type ublExtensions struct {
	XMLName   xml.Name       `xml:"ext:UBLExtensions"`
	Extension []ublExtension `xml:"ext:UBLExtension"`
}

type ublExtension struct {
	ExtensionContent extensionContent `xml:"ext:ExtensionContent"`
}

// extensionContent is the body of an ext:UBLExtension. SUNAT uses two kinds:
// (1) sac:AdditionalInformation — carries AdditionalMonetaryTotals (e.g.
// ID=1004 for total gratuito) and AdditionalProperties.
// (2) ds:Signature — XMLDSig template that xmlsec1 fills in.
// Both fields are optional; an extension carries exactly one of them.
type extensionContent struct {
	AdditionalInformation *sacAdditionalInformation `xml:"sac:AdditionalInformation,omitempty"`
	Signature             *dsSignatureTemplate      `xml:"ds:Signature,omitempty"`
}

// sacAdditionalInformation is the SUNAT extension wrapper that holds
// AdditionalMonetaryTotals (and, in future, AdditionalProperties).
type sacAdditionalInformation struct {
	XMLName                  xml.Name                  `xml:"sac:AdditionalInformation"`
	AdditionalMonetaryTotals []additionalMonetaryTotal `xml:",omitempty"`
}

type dsSignatureTemplate struct {
	XMLNS_DS       string       `xml:"xmlns:ds,attr"`
	Id             string       `xml:"Id,attr"`
	SignedInfo     dsSignedInfo `xml:"ds:SignedInfo"`
	SignatureValue string       `xml:"ds:SignatureValue"`
	KeyInfo        dsKeyInfo    `xml:"ds:KeyInfo"`
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
		Signature: &dsSignatureTemplate{
			XMLNS_DS: nsDS,
			Id:       signatureID,
			SignedInfo: dsSignedInfo{
				CanonicalizationMethod: dsAlgorithm{Algorithm: algC14N},
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
	SignatoryParty             signatoryParty             `xml:"cac:SignatoryParty"`
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
		ID: signatureID,
		SignatoryParty: signatoryParty{
			PartyIdentification: partyIdentification{ID: schemeID{Value: ruc}},
			PartyName:           partyName{Name: companyName},
		},
		DigitalSignatureAttachment: digitalSignatureAttachment{
			ExternalReference: externalReference{URI: "#" + signatureID},
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
	AddressTypeCode  *addressTypeCode `xml:"cbc:AddressTypeCode,omitempty"`
	CityName         string           `xml:"cbc:CityName,omitempty"`
	CountrySubentity string           `xml:"cbc:CountrySubentity,omitempty"`
	District         string           `xml:"cbc:District,omitempty"`
	AddressLine      *addressLine     `xml:"cac:AddressLine,omitempty"`
	Country          *country         `xml:"cac:Country,omitempty"`
}

type addressTypeCode struct {
	Value          string `xml:",chardata"`
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
	XMLName     xml.Name       `xml:"cac:TaxTotal"`
	TaxAmount   currencyAmount `xml:"cbc:TaxAmount"`
	TaxSubtotal []taxSubtotal  `xml:"cac:TaxSubtotal"`
}

type taxSubtotal struct {
	TaxableAmount currencyAmount `xml:"cbc:TaxableAmount"`
	TaxAmount     currencyAmount `xml:"cbc:TaxAmount"`
	TaxCategory   taxCategory    `xml:"cac:TaxCategory"`
}

type taxCategory struct {
	ID                     taxCategoryID     `xml:"cbc:ID"`
	Percent                string            `xml:"cbc:Percent,omitempty"`
	TierRange              string            `xml:"cbc:TierRange,omitempty"` // ISC calculation system (Cat.08)
	TaxExemptionReasonCode *taxExemptionCode `xml:"cbc:TaxExemptionReasonCode,omitempty"`
	TaxScheme              taxSchemeXML      `xml:"cac:TaxScheme"`
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

// paymentTerms represents cac:PaymentTerms (forma de pago, Cat.SUNAT).
// Required on Factura/Boleta since 2018; SUNAT error 3244 fires when missing.
// The same element also carries the detracción block (ID="Detraccion"), which
// additionally sets PaymentPercent. Field order matters: SUNAT expects
// ID, PaymentMeansID, PaymentPercent, Amount.
type paymentTerms struct {
	XMLName        xml.Name        `xml:"cac:PaymentTerms"`
	ID             string          `xml:"cbc:ID"`
	PaymentMeansID string          `xml:"cbc:PaymentMeansID"`
	PaymentPercent string          `xml:"cbc:PaymentPercent,omitempty"`
	Amount         *currencyAmount `xml:"cbc:Amount,omitempty"`
	PaymentDueDate string          `xml:"cbc:PaymentDueDate,omitempty"`
}

// paymentMeans represents cac:PaymentMeans. SUNAT uses it only to carry the
// detracción's Banco de la Nación account (PaymentMeansCode 999 = "otros").
type paymentMeans struct {
	XMLName               xml.Name              `xml:"cac:PaymentMeans"`
	ID                    string                `xml:"cbc:ID"`
	PaymentMeansCode      string                `xml:"cbc:PaymentMeansCode"`
	PayeeFinancialAccount payeeFinancialAccount `xml:"cac:PayeeFinancialAccount"`
}

type payeeFinancialAccount struct {
	ID string `xml:"cbc:ID"`
}

// buildDetraccionPaymentMeans returns the cac:PaymentMeans carrying the cuenta
// de detracción, or nil when the document is not subject to detracción.
func buildDetraccionPaymentMeans(req model.IssueRequest) *paymentMeans {
	d := req.Detraccion
	if d == nil {
		return nil
	}
	return &paymentMeans{
		ID:                    "Detraccion",
		PaymentMeansCode:      "999",
		PayeeFinancialAccount: payeeFinancialAccount{ID: d.CuentaBN},
	}
}

// buildDetraccionPaymentTerms returns the extra cac:PaymentTerms declaring the
// detracción código, porcentaje and monto (always in PEN), or nil when the
// document is not subject to detracción.
func buildDetraccionPaymentTerms(req model.IssueRequest) *paymentTerms {
	d := req.Detraccion
	if d == nil {
		return nil
	}
	amt := newCurrencyAmount(d.Monto, "PEN")
	return &paymentTerms{
		ID:             "Detraccion",
		PaymentMeansID: d.Codigo,
		PaymentPercent: d.Porcentaje,
		Amount:         &amt,
	}
}

// appendDetraccionLegend appends the SUNAT leyenda 2006 ("Operación sujeta a
// detracción") when the document is subject to detracción and the note is not
// already present. Shared by Invoice, CreditNote and DebitNote builders.
func appendDetraccionLegend(notes []noteElement, req model.IssueRequest) []noteElement {
	if req.Detraccion == nil || hasNoteWithCode(notes, sunat.Cat52Detraccion) {
		return notes
	}
	return append(notes, noteElement{
		Value:            sunat.Cat52Texto(sunat.Cat52Detraccion),
		LanguageLocaleID: sunat.Cat52Detraccion,
	})
}

// prepaidPayment represents cac:PrepaidPayment — one prior anticipo deducted
// by a factura de regularización. ID is the 1-based row number pairing it with
// the cac:AdditionalDocumentReference whose cbc:DocumentStatusCode matches
// (schemeName "Anticipo", schemeAgencyName "PE:SUNAT" per the SUNAT anticipo
// guide); PaidAmount is the anticipo amount INCLUDING IGV.
type prepaidPayment struct {
	XMLName    xml.Name       `xml:"cac:PrepaidPayment"`
	ID         schemeID       `xml:"cbc:ID"`
	PaidAmount currencyAmount `xml:"cbc:PaidAmount"`
}

// docRefIssuerParty is the cac:IssuerParty of an anticipo document reference:
// the emitter of the referenced anticipo comprobante, identified by RUC
// (schemeID "6", Cat.06).
type docRefIssuerParty struct {
	PartyIdentification partyIdentification `xml:"cac:PartyIdentification"`
}

// legalMonetaryTotal represents the totals block. The element name is supplied
// by the parent field's tag (cac:LegalMonetaryTotal for Invoice/CreditNote,
// cac:RequestedMonetaryTotal for DebitNote) rather than hardcoded here.
// PrepaidAmount (UBL order: after ChargeTotalAmount, before PayableAmount) is
// the total of anticipos applied, con IGV; emitted only on regularizaciones.
type legalMonetaryTotal struct {
	LineExtensionAmount  currencyAmount  `xml:"cbc:LineExtensionAmount"`
	TaxInclusiveAmount   currencyAmount  `xml:"cbc:TaxInclusiveAmount"`
	AllowanceTotalAmount *currencyAmount `xml:"cbc:AllowanceTotalAmount,omitempty"`
	ChargeTotalAmount    *currencyAmount `xml:"cbc:ChargeTotalAmount,omitempty"`
	PrepaidAmount        *currencyAmount `xml:"cbc:PrepaidAmount,omitempty"`
	PayableAmount        currencyAmount  `xml:"cbc:PayableAmount"`
}

// additionalMonetaryTotal represents sac:AdditionalMonetaryTotal. Used to
// declare the total referential value of operaciones gratuitas (ID=1004).
type additionalMonetaryTotal struct {
	XMLName       xml.Name       `xml:"sac:AdditionalMonetaryTotal"`
	ID            string         `xml:"cbc:ID"`
	PayableAmount currencyAmount `xml:"cbc:PayableAmount"`
}

// invoiceLine represents a single line item (for Invoice).
type invoiceLine struct {
	XMLName             xml.Name          `xml:"cac:InvoiceLine"`
	ID                  string            `xml:"cbc:ID"`
	InvoicedQuantity    quantity          `xml:"cbc:InvoicedQuantity"`
	LineExtensionAmount currencyAmount    `xml:"cbc:LineExtensionAmount"`
	PricingReference    *pricingReference `xml:"cac:PricingReference,omitempty"`
	// UBL puts cac:Delivery between PricingReference and AllowanceCharge; only a
	// detracción de transporte de carga (1004) populates it.
	Delivery        *lineDelivery        `xml:"cac:Delivery,omitempty"`
	AllowanceCharge *lineAllowanceCharge `xml:"cac:AllowanceCharge,omitempty"`
	TaxTotal        taxTotal             `xml:"cac:TaxTotal"`
	Item            item                 `xml:"cac:Item"`
	Price           price                `xml:"cac:Price"`
}

// lineDelivery carries the transporte de carga (1004) trip data. Field order is
// the UBL DeliveryType sequence — DeliveryLocation, then Despatch, then the
// repeated DeliveryTerms — and reordering it fails XSD validation silently.
type lineDelivery struct {
	DeliveryLocation *deliveryLocation `xml:"cac:DeliveryLocation,omitempty"`
	Despatch         *despatch         `xml:"cac:Despatch,omitempty"`
	DeliveryTerms    []deliveryTerms   `xml:"cac:DeliveryTerms,omitempty"`
}

type deliveryLocation struct {
	Address ubigeoAddress `xml:"cac:Address"`
}

// despatch holds the punto de origen. cbc:Instructions (the detalle del viaje)
// precedes cac:DespatchAddress in UBL's DespatchType.
type despatch struct {
	Instructions    string        `xml:"cbc:Instructions,omitempty"`
	DespatchAddress ubigeoAddress `xml:"cac:DespatchAddress"`
}

// ubigeoAddress is a Cat.13 ubigeo (cbc:ID) plus the detailed street. The
// schemeName/schemeAgencyName attributes are optional (observations 4255/4256
// only fire when present and wrong), but SUNAT documents them, so emit them.
type ubigeoAddress struct {
	ID          ubigeoID    `xml:"cbc:ID"`
	AddressLine addressLine `xml:"cac:AddressLine"`
}

type ubigeoID struct {
	Value            string `xml:",chardata"`
	SchemeName       string `xml:"schemeName,attr,omitempty"`
	SchemeAgencyName string `xml:"schemeAgencyName,attr,omitempty"`
}

// deliveryTerms is one valor referencial: cbc:ID is the type ("01" servicio,
// "02" carga efectiva, "03" carga útil nominal) and cbc:Amount its PEN value.
type deliveryTerms struct {
	ID     string         `xml:"cbc:ID"`
	Amount currencyAmount `xml:"cbc:Amount"`
}

// creditNoteLine represents a single line item (for CreditNote). SUNAT's
// CreditNoteLine model has NO line-level cac:AllowanceCharge (unlike InvoiceLine):
// a descuento por ítem is baked into cac:Price/cbc:PriceAmount as the net valor
// unitario, since SUNAT computes LineExtensionAmount = CreditedQuantity × Price.
type creditNoteLine struct {
	XMLName             xml.Name          `xml:"cac:CreditNoteLine"`
	ID                  string            `xml:"cbc:ID"`
	CreditedQuantity    quantity          `xml:"cbc:CreditedQuantity"`
	LineExtensionAmount currencyAmount    `xml:"cbc:LineExtensionAmount"`
	PricingReference    *pricingReference `xml:"cac:PricingReference,omitempty"`
	TaxTotal            taxTotal          `xml:"cac:TaxTotal"`
	Item                item              `xml:"cac:Item"`
	Price               price             `xml:"cac:Price"`
}

// debitNoteLine represents a single line item (for DebitNote). Like
// CreditNoteLine, SUNAT's DebitNoteLine model has no line-level
// cac:AllowanceCharge; a descuento is baked into the net cac:Price/cbc:PriceAmount.
type debitNoteLine struct {
	XMLName             xml.Name          `xml:"cac:DebitNoteLine"`
	ID                  string            `xml:"cbc:ID"`
	DebitedQuantity     quantity          `xml:"cbc:DebitedQuantity"`
	LineExtensionAmount currencyAmount    `xml:"cbc:LineExtensionAmount"`
	PricingReference    *pricingReference `xml:"cac:PricingReference,omitempty"`
	TaxTotal            taxTotal          `xml:"cac:TaxTotal"`
	Item                item              `xml:"cac:Item"`
	Price               price             `xml:"cac:Price"`
}

// lineAllowanceCharge represents a cac:AllowanceCharge, used both on a line and
// at document level. SUNAT uses it to itemise a descuento por ítem (Cat.53 code
// "00", which affects the IGV base). The discount reconciles
// cbc:LineExtensionAmount with cac:Price: SUNAT rule 3271 computes
// LineExtensionAmount = Price.PriceAmount × Quantity − Amount, so the line price
// stays gross (valor unitario) and the discount is subtracted here.
// MultiplierFactorNumeric = Amount / BaseAmount.
//
// MultiplierFactorNumeric and BaseAmount are optional: the descuento global por
// anticipo (Cat.53 "04") carries only the amount already collected, with no base
// to prorate against (see buildAnticipoAllowances).
type lineAllowanceCharge struct {
	ChargeIndicator         bool            `xml:"cbc:ChargeIndicator"`
	AllowanceChargeReason   string          `xml:"cbc:AllowanceChargeReasonCode"`
	MultiplierFactorNumeric string          `xml:"cbc:MultiplierFactorNumeric,omitempty"`
	Amount                  currencyAmount  `xml:"cbc:Amount"`
	BaseAmount              *currencyAmount `xml:"cbc:BaseAmount,omitempty"`
}

type quantity struct {
	Value    string `xml:",chardata"`
	UnitCode string `xml:"unitCode,attr"`
}

type pricingReference struct {
	AlternativeConditionPrice []alternativeConditionPrice `xml:"cac:AlternativeConditionPrice"`
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

// invoiceTypeCode carries two different catálogos in one element, by SUNAT's
// design. The element value is catálogo 01 (document type, e.g. "01" Factura).
// The @listID attribute is a separate mandatory field holding catálogo 51
// (operation type, e.g. "0101" Venta interna). @listURI names catalogo01
// because it describes the *value*, not @listID.
//
// This reads like a código copied into the wrong attribute. It is not — it is
// what the wire accepts, and what greenter emits. SUNAT rejects with fault 3205
// when @listID is missing, 3206 when it is not a catálogo 51 code, and 3129
// when it contradicts the detracción's Cat.54 código.
//
// @listID must always equal cbc:ProfileID, which is why newInvoiceTypeCode and
// newProfileID both resolve it through sunatOperationType.
type invoiceTypeCode struct {
	XMLName        xml.Name `xml:"cbc:InvoiceTypeCode"`
	Value          string   `xml:",chardata"`
	ListID         string   `xml:"listID,attr"`
	ListAgencyName string   `xml:"listAgencyName,attr"`
	ListName       string   `xml:"listName,attr"`
	ListURI        string   `xml:"listURI,attr"`
}

// profileID carries the SUNAT operation type (catalog 17), e.g. "0101"
// for Venta interna. Lives between cbc:CustomizationID and cbc:ID.
type profileIDElement struct {
	XMLName          xml.Name `xml:"cbc:ProfileID"`
	Value            string   `xml:",chardata"`
	SchemeName       string   `xml:"schemeName,attr"`
	SchemeAgencyName string   `xml:"schemeAgencyName,attr"`
	SchemeURI        string   `xml:"schemeURI,attr"`
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

// detraccionOperationType returns the catálogo 51 code a document sujeto a
// detracción must declare. Three Cat.54 códigos have their own operación —
// SUNAT tracks them separately because each carries extra mandatory data — and
// the pairing is exact in both directions: declaring 1002/1003/1004 with the
// wrong código, or the código with plain 1001, is fault 3129.
//
//	Cat.54 004 (recursos hidrobiológicos) → 1002
//	Cat.54 028 (transporte de pasajeros)  → 1003
//	Cat.54 027 (transporte de carga)      → 1004
//	everything else                       → 1001
//
// Source: sheets Factura2_0 / Boleta2_0 of
// resources/AjustesValidacionesCPEv20260212.xlsx.
func detraccionOperationType(d *model.Detraccion) string {
	if d == nil {
		return sunat.Cat51DetraccionGeneral
	}
	switch d.Codigo {
	case sunat.Cat54RecursosHidrobiologicos:
		return sunat.Cat51DetraccionHidrobiologicos
	case sunat.Cat54TransporteDePasajeros:
		return sunat.Cat51DetraccionPasajeros
	case sunat.Cat54ServicioDeTransporteDeCarga:
		return sunat.Cat51DetraccionTransporteCarga
	default:
		return sunat.Cat51DetraccionGeneral
	}
}

// sunatOperationType maps an internally stored tipo de operación onto the code
// SUNAT's cat_51.xml actually accepts.
//
// "0104" (Venta interna – Anticipos) was retired from catálogo 51: SUNAT now
// rejects it outright with fault 3206 ("no se encuentra en el catalogo: 51").
// We still store it on the document as the marker that a factura *is* an
// anticipo — it drives the anticipos report, the historial picker and the UI
// badge — but it never reaches the wire. What makes the final factura a
// regularización is the PrepaidPayment / AllowanceCharge "04" block, not the
// operation type.
//
// A comprobante de anticipo can also be sujeto a detracción: the SPOT
// obligation arises per payment, computed on the monto of the comprobante that
// documents it (so the anticipo declares SPOT on what it collected, and the
// comprobante de regularización on its PayableAmount — the saldo). When that
// happens the anticipo goes out as an operación sujeta a detracción, since
// 1001-1004 is what pairs with the leyenda 2006 + cuenta BN block (fault 3127
// otherwise).
//
// It also RE-DERIVES the detracción operación from the Cat.54 código whenever
// one is attached, rather than trusting what the caller stored. The pairing is
// exact (fault 3129), and a draft can easily carry a stale 1001 — it was built
// before the código changed, or the row predates this mapping being correct.
// The código is the fact; the operación is a function of it.
func sunatOperationType(operationType string, d *model.Detraccion) string {
	if operationType == sunat.Cat51Anticipos {
		if d != nil {
			return detraccionOperationType(d)
		}
		return sunat.Cat51VentaInterna
	}
	if d != nil && isDetraccionOperationType(operationType) {
		return detraccionOperationType(d)
	}
	return operationType
}

func isDetraccionOperationType(operationType string) bool {
	switch operationType {
	case sunat.Cat51DetraccionGeneral, sunat.Cat51DetraccionHidrobiologicos,
		sunat.Cat51DetraccionPasajeros, sunat.Cat51DetraccionTransporteCarga:
		return true
	}
	return false
}

func newInvoiceTypeCode(code, operationType string, d *model.Detraccion) invoiceTypeCode {
	if operationType == "" {
		operationType = sunat.Cat51VentaInterna
	}
	return invoiceTypeCode{
		Value:          code,
		ListID:         sunatOperationType(operationType, d),
		ListAgencyName: "PE:SUNAT",
		ListName:       "Tipo de Documento",
		ListURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01",
	}
}

// newProfileID returns the SUNAT cbc:ProfileID element carrying the operation
// type. The @schemeURI names catalogo17, but the códigos themselves come from
// catálogo 51 — SUNAT's own inconsistency. Defaults to Venta interna, and
// always agrees with cbc:InvoiceTypeCode/@listID.
func newProfileID(operationType string, d *model.Detraccion) profileIDElement {
	if operationType == "" {
		operationType = sunat.Cat51VentaInterna
	}
	return profileIDElement{
		Value:            sunatOperationType(operationType, d),
		SchemeName:       "SUNAT:Identificador de Tipo de Operación",
		SchemeAgencyName: "PE:SUNAT",
		SchemeURI:        "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo17",
	}
}
