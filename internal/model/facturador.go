package model

import "time"

// Series represents a row in document_series. The JSON tags are camelCase to
// match the shape returned by perunio-backend so the existing frontend works
// without changes.
type Series struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	CompanyID       string    `json:"companyId"`
	DocType         string    `json:"docType"`
	Series          string    `json:"series"`
	NextCorrelative int       `json:"nextCorrelative"`
	Description     *string   `json:"description"`
	IsActive        bool      `json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// IssuedDocument is a row in issued_documents (without items).
type IssuedDocument struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	CompanyID string `json:"companyId"`
	SeriesID string `json:"seriesId"`
	DocType  string `json:"docType"`
	Series   string `json:"series"`
	Correlative int    `json:"correlative"`
	Status      string `json:"status"`

	IssueDate time.Time `json:"issueDate"`
	IssueTime *string   `json:"issueTime"`
	DueDate   *time.Time `json:"dueDate"`

	CurrencyCode  string  `json:"currencyCode"`
	OperationType *string `json:"operationType"`

	CustomerDocType   string  `json:"customerDocType"`
	CustomerDocNumber string  `json:"customerDocNumber"`
	CustomerName      string  `json:"customerName"`
	CustomerAddress   *string `json:"customerAddress"`

	Subtotal           string  `json:"subtotal"`
	TotalIgv           string  `json:"totalIgv"`
	TotalIsc           *string `json:"totalIsc"`
	TotalOtherTaxes    *string `json:"totalOtherTaxes"`
	TotalDiscount      *string `json:"totalDiscount"`
	TotalAmount        string  `json:"totalAmount"`
	TaxInclusiveAmount *string `json:"taxInclusiveAmount"`
	Notes              *string `json:"notes"`

	ReferenceDocType        *string `json:"referenceDocType"`
	ReferenceDocSeries      *string `json:"referenceDocSeries"`
	ReferenceDocCorrelative *int    `json:"referenceDocCorrelative"`
	CreditDebitReasonCode   *string `json:"creditDebitReasonCode"`
	CreditDebitReasonDesc   *string `json:"creditDebitReasonDesc"`

	SunatResponseCode        *string `json:"sunatResponseCode"`
	SunatResponseDescription *string `json:"sunatResponseDescription"`
	SunatTicket              *string `json:"sunatTicket"`

	R2XmlKey       *string `json:"r2XmlKey"`
	R2SignedXmlKey *string `json:"r2SignedXmlKey"`
	R2ZipKey       *string `json:"r2ZipKey"`
	R2CdrKey       *string `json:"r2CdrKey"`
	R2PdfKey       *string `json:"r2PdfKey"`

	QrData *string `json:"qrData"`

	SentAt     *time.Time `json:"sentAt"`
	AcceptedAt *time.Time `json:"acceptedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// IssuedDocumentItem is one line of an issued document.
type IssuedDocumentItem struct {
	ID                     string    `json:"id"`
	DocumentID             string    `json:"documentId"`
	LineNumber             int       `json:"lineNumber"`
	Description            string    `json:"description"`
	Quantity               string    `json:"quantity"`
	UnitCode               string    `json:"unitCode"`
	UnitPrice              string    `json:"unitPrice"`
	UnitPriceWithTax       *string   `json:"unitPriceWithTax"`
	TaxExemptionReasonCode *string   `json:"taxExemptionReasonCode"`
	IgvAmount              string    `json:"igvAmount"`
	IscAmount              *string   `json:"iscAmount"`
	IscTierRange           *string   `json:"iscTierRange"`
	DiscountAmount         *string   `json:"discountAmount"`
	LineTotal              string    `json:"lineTotal"`
	PriceTypeCode          *string   `json:"priceTypeCode"`
	CreatedAt              time.Time `json:"createdAt"`
}

// DailySummary is a row in daily_summaries (without items).
type DailySummary struct {
	ID                       string     `json:"id"`
	TenantID                 string     `json:"tenantId"`
	CompanyID                string     `json:"companyId"`
	SummaryID                string     `json:"summaryId"`
	ReferenceDate            time.Time  `json:"referenceDate"`
	Status                   string     `json:"status"`
	SunatTicket              *string    `json:"sunatTicket"`
	SunatResponseCode        *string    `json:"sunatResponseCode"`
	SunatResponseDescription *string    `json:"sunatResponseDescription"`
	R2XmlKey                 *string    `json:"r2XmlKey"`
	R2SignedXmlKey           *string    `json:"r2SignedXmlKey"`
	R2CdrKey                 *string    `json:"r2CdrKey"`
	TotalDocuments           int        `json:"totalDocuments"`
	SentAt                   *time.Time `json:"sentAt"`
	AcceptedAt               *time.Time `json:"acceptedAt"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

// VoidedDocument is a row in voided_documents (without items).
type VoidedDocument struct {
	ID                       string     `json:"id"`
	TenantID                 string     `json:"tenantId"`
	CompanyID                string     `json:"companyId"`
	VoidID                   string     `json:"voidId"`
	VoidDate                 time.Time  `json:"voidDate"`
	Status                   string     `json:"status"`
	SunatTicket              *string    `json:"sunatTicket"`
	SunatResponseCode        *string    `json:"sunatResponseCode"`
	SunatResponseDescription *string    `json:"sunatResponseDescription"`
	R2XmlKey                 *string    `json:"r2XmlKey"`
	R2SignedXmlKey           *string    `json:"r2SignedXmlKey"`
	R2CdrKey                 *string    `json:"r2CdrKey"`
	SentAt                   *time.Time `json:"sentAt"`
	AcceptedAt               *time.Time `json:"acceptedAt"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

// VoidedDocumentItem represents one document inside a void communication.
type VoidedDocumentItem struct {
	ID          string    `json:"id"`
	VoidID      string    `json:"voidId"`
	DocumentID  string    `json:"documentId"`
	DocType     string    `json:"docType"`
	Series      string    `json:"series"`
	Correlative int       `json:"correlative"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"createdAt"`
}

// FacturadorUsage is the response shape for GET /api/facturador/usage.
type FacturadorUsage struct {
	Period        string `json:"period"`
	DocumentCount int    `json:"documentCount"`
	Limit         *int   `json:"limit"`
	Tier          string `json:"tier"`
}
