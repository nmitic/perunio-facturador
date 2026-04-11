package model

import "time"

// Despatch type codes (Cat.01 for Remitente/Transportista; "EV" is a local
// discriminator for "Por Eventos" — the SUNAT XML still carries 09/31 in
// DespatchAdviceTypeCode, but "EV" lets the pipeline know to emit the
// event-specific references).
const (
	DespatchTypeRemitente     = "09"
	DespatchTypeTransportista = "31"
	DespatchTypeEvento        = "EV"
)

// Transport modality (Cat.18):
//
//	"01" = public (third-party carrier)
//	"02" = private (transferor's own vehicle)
const (
	TransportModalityPublic  = "01"
	TransportModalityPrivate = "02"
)

// DespatchStatus values mirror the lifecycle of summaries/voids: drafts
// until issued, then sent → accepted|rejected once SUNAT replies.
const (
	DespatchStatusDraft    = "draft"
	DespatchStatusSigned   = "signed"
	DespatchStatusSent     = "sent"
	DespatchStatusAccepted = "accepted"
	DespatchStatusRejected = "rejected"
	DespatchStatusError    = "error"
)

// Despatch is a row in the `despatches` table — a single Guía de Remisión
// Electrónica. Unlike invoices this carries goods metadata (weight,
// packages, addresses, transport details) instead of monetary totals.
type Despatch struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	CompanyID string `json:"companyId"`
	SeriesID  string `json:"seriesId"`

	// DocType is "09" (Remitente), "31" (Transportista) or "EV" (por
	// Eventos). "EV" means the XML carries 09 or 31 plus an event
	// reference node.
	DocType     string `json:"docType"`
	Series      string `json:"series"`
	Correlative int    `json:"correlative"`

	IssueDate time.Time `json:"issueDate"`
	IssueTime *string   `json:"issueTime"`

	// Recipient (destinatario). The transferor — DespatchSupplierParty
	// in the XML — is always the issuing company, pulled from the
	// `companies` row, so it isn't stored here.
	RecipientDocType   string  `json:"recipientDocType"`
	RecipientDocNumber string  `json:"recipientDocNumber"`
	RecipientName      string  `json:"recipientName"`
	RecipientAddress   *string `json:"recipientAddress"`

	// Transport.
	TransportModality string  `json:"transportModality"` // Cat.18
	TransferReason    string  `json:"transferReason"`    // Cat.20
	TransferReasonDesc *string `json:"transferReasonDesc"`
	StartDate         *time.Time `json:"startDate"`      // fecha de inicio del traslado
	TotalWeightKg     string  `json:"totalWeightKg"`
	WeightUnitCode    string  `json:"weightUnitCode"`    // "KGM" per SUNAT
	TotalPackages     *int    `json:"totalPackages"`

	// Addresses — both punto de partida (origin) and punto de llegada
	// (arrival) require 6-digit ubigeo codes.
	StartUbigeo     string  `json:"startUbigeo"`
	StartAddress    string  `json:"startAddress"`
	ArrivalUbigeo   string  `json:"arrivalUbigeo"`
	ArrivalAddress  string  `json:"arrivalAddress"`

	// Private transport only — required when TransportModality = "02"
	// (and always for Transportista). Driver license + at least one
	// vehicle plate are hard-required by SUNAT.
	DriverDocType    *string `json:"driverDocType"`    // Cat.06
	DriverDocNumber  *string `json:"driverDocNumber"`
	DriverLicense    *string `json:"driverLicense"`
	DriverName       *string `json:"driverName"`
	VehiclePlate     *string `json:"vehiclePlate"`
	VehiclePlates    []string `json:"vehiclePlates"` // additional (secundarios)

	// Public transport only — required when TransportModality = "01".
	// The third-party carrier.
	CarrierRUC  *string `json:"carrierRuc"`
	CarrierName *string `json:"carrierName"`
	// Optional MTC registration number for the carrier.
	CarrierMTC  *string `json:"carrierMtc"`

	// Por Eventos only.
	EventCode     *string `json:"eventCode"`     // Cat.59
	OriginalGreID *string `json:"originalGreId"` // FK to despatches.id

	// Related document reference (optional — e.g. the invoice/order).
	RelatedDocType    *string `json:"relatedDocType"`
	RelatedDocSeries  *string `json:"relatedDocSeries"`
	RelatedDocNumber  *string `json:"relatedDocNumber"`

	Status string `json:"status"`

	// SUNAT REST state.
	SunatTicket              *string `json:"sunatTicket"`
	SunatResponseCode        *string `json:"sunatResponseCode"`
	SunatResponseDescription *string `json:"sunatResponseDescription"`

	R2XmlKey       *string `json:"r2XmlKey"`
	R2SignedXmlKey *string `json:"r2SignedXmlKey"`
	R2ZipKey       *string `json:"r2ZipKey"`
	R2CdrKey       *string `json:"r2CdrKey"`

	SentAt     *time.Time `json:"sentAt"`
	AcceptedAt *time.Time `json:"acceptedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// DespatchLine is a single goods line on a Despatch — GRE carries only
// description/quantity/unit code (no prices, no taxes).
type DespatchLine struct {
	ID          string    `json:"id"`
	DespatchID  string    `json:"despatchId"`
	LineNumber  int       `json:"lineNumber"`
	Description string    `json:"description"`
	Quantity    string    `json:"quantity"`
	UnitCode    string    `json:"unitCode"` // UN/ECE rec 20 (e.g. NIU, KGM)
	ProductCode *string   `json:"productCode"`
	CreatedAt   time.Time `json:"createdAt"`
}
