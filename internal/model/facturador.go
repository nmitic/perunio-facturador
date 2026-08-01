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

// VentaAnticipoEstado is a venta con anticipos' lifecycle, always DERIVED from
// the comprobantes pointing at it (plus the manual Cancelada flag) so it can
// never disagree with the historial.
type VentaAnticipoEstado string

const (
	// VentaAbierta still has balance to collect, or has collected everything but
	// has not been regularized by a factura final yet.
	VentaAbierta VentaAnticipoEstado = "abierta"
	// VentaRegularizada has an accepted factura final deducting its anticipos.
	VentaRegularizada VentaAnticipoEstado = "regularizada"
	// VentaCancelada was abandoned by the user. The anticipos already emitted
	// stay valid comprobantes — cancelling only stops the deal being offered.
	VentaCancelada VentaAnticipoEstado = "cancelada"
)

// VentaAnticipo is one sale collected through advance payments: the agreed total
// plus the comprobantes that chip away at it.
//
// SUNAT never sees this record. It exists so the product can answer "how much is
// still left to collect", which needs the agreed total stored somewhere — the
// individual anticipo facturas carry no notion of the sale they belong to.
type VentaAnticipo struct {
	ID               string `json:"id"`
	TenantID         string `json:"tenantId"`
	CompanyID        string `json:"companyId"`
	SunatEnvironment string `json:"sunatEnvironment"`

	Nombre      string  `json:"nombre"`
	Descripcion *string `json:"descripcion"`

	CustomerDocType   string `json:"customerDocType"`
	CustomerDocNumber string `json:"customerDocNumber"`
	CustomerName      string `json:"customerName"`

	CurrencyCode string `json:"currencyCode"`
	// MontoAcordado is the line total of FormData, denormalised at save time so
	// listing deals stays a single query.
	MontoAcordado string         `json:"montoAcordado"`
	FormData      map[string]any `json:"formData"`

	Cancelada bool      `json:"cancelada"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Derived from the issued_documents pointing here — never stored, so voiding
	// a comprobante self-heals the numbers. Cobrado covers every accepted
	// comprobante of the deal, the factura final included: its total is the
	// saldo, so a regularizada venta reads cobrado = acordado, saldo 0.
	// AnticipoCount, in contrast, counts only the anticipos (0104).
	Cobrado string              `json:"cobrado"`
	Saldo   string              `json:"saldo"`
	Estado  VentaAnticipoEstado `json:"estado"`
	// DetraccionDeclarada is the SPOT total across the deal's accepted
	// comprobantes. Each one declares detracción on its own importe (the
	// anticipo on what it collected, the factura final on the saldo), so a
	// complete venta sums to porcentaje × MontoAcordado — which is what lets the
	// detail page show whether the deposits add up.
	DetraccionDeclarada string `json:"detraccionDeclarada"`
	AnticipoCount       int    `json:"anticipoCount"`
	// FinalDocumentID/Code identify the accepted factura final that regularized
	// this venta; both nil while it is still abierta.
	FinalDocumentID   *string `json:"finalDocumentId"`
	FinalDocumentCode *string `json:"finalDocumentCode"`
}

// ScheduleOrigin identifies which kind of schedule emitted a comprobante.
//
// Single source of the vocabulary: the scheduler uses it to say which table a due
// run came from, and the historial uses it to report a comprobante's provenance.
// Lives in model so both the db and http layers can name it without an import
// cycle — db imports model, never the reverse.
type ScheduleOrigin string

const (
	// ScheduleOriginManual is the stored origin of a hand-issued comprobante. It is
	// never surfaced on the wire — the historial reports manual emissions as a nil
	// ScheduleOrigin (see issuedDocumentColumns) so the UI renders no badge.
	ScheduleOriginManual     ScheduleOrigin = "manual"
	ScheduleOriginRecurrente ScheduleOrigin = "recurrente"
	ScheduleOriginProgramado ScheduleOrigin = "programado"
)

// IssuedDocument is a row in issued_documents (without items).
type IssuedDocument struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenantId"`
	CompanyID   string `json:"companyId"`
	SeriesID    string `json:"seriesId"`
	DocType     string `json:"docType"`
	Series      string `json:"series"`
	Correlative int    `json:"correlative"`
	Status      string `json:"status"`
	// SunatEnvironment ("beta" | "production") this document belongs to, stamped
	// at draft creation from the company's environment. Listings are filtered to
	// the company's current environment so sandbox/test docs don't pollute the
	// production historial.
	SunatEnvironment string `json:"sunatEnvironment"`

	IssueDate time.Time  `json:"issueDate"`
	IssueTime *string    `json:"issueTime"`
	DueDate   *time.Time `json:"dueDate"`

	CurrencyCode string `json:"currencyCode"`
	// ExchangeRate is the tipo de cambio to soles for foreign-currency documents
	// (decimal as string); nil for PEN.
	ExchangeRate  *string `json:"exchangeRate,omitempty"`
	OperationType *string `json:"operationType"`

	CustomerDocType   string  `json:"customerDocType"`
	CustomerDocNumber string  `json:"customerDocNumber"`
	CustomerName      string  `json:"customerName"`
	CustomerAddress   *string `json:"customerAddress"`

	// ClienteID is the id of the saved cliente_facturador matching this document's
	// customer (company + doc type + doc number), or nil when the customer isn't a
	// saved cliente (e.g. consumidor final). Populated only in the historial list
	// so the UI can link the customer to their cliente detail page; nil elsewhere.
	ClienteID *string `json:"clienteId,omitempty"`

	Subtotal        string  `json:"subtotal"`
	TotalIgv        string  `json:"totalIgv"`
	TotalIsc        *string `json:"totalIsc"`
	TotalOtherTaxes *string `json:"totalOtherTaxes"`
	TotalDiscount   *string `json:"totalDiscount"`
	GlobalDiscount  *string `json:"globalDiscount"`
	// BillingPeriodMonths is the periodo de facturación this comprobante bills for.
	// Reporting metadata: the line prices and descriptions already have it baked in,
	// so it must never be fed back into a form as a multiplier. Nil = none.
	BillingPeriodMonths *int    `json:"billingPeriodMonths"`
	TotalAmount         string  `json:"totalAmount"`
	TaxInclusiveAmount  *string `json:"taxInclusiveAmount"`
	Notes               *string `json:"notes"`

	FormaPago *string        `json:"formaPago,omitempty"`
	Cuotas    []CuotaCredito `json:"cuotas,omitempty"`

	// Anticipos applied by this document (factura de regularización). Nil for
	// ordinary comprobantes AND for the anticipo facturas themselves — whether
	// an anticipo has been applied is derived by searching these arrays.
	Anticipos []Anticipo `json:"anticipos,omitempty"`

	// VentaAnticipoID is the venta con anticipos (deal) this comprobante belongs
	// to — set on the anticipo facturas and on the factura final that closes it.
	// Nil for every ordinary comprobante.
	VentaAnticipoID *string `json:"ventaAnticipoId,omitempty"`

	// Manual payment status for CONTADO documents. PaidAt present = "pagado".
	// Orthogonal to SUNAT Status and to the cuotas system (crédito docs derive
	// their paid state from document_installment_payments, never from these).
	PaidAt           *time.Time `json:"paidAt"`
	PaymentMethod    *string    `json:"paymentMethod"`
	PaymentReference *string    `json:"paymentReference"`
	PaymentNotes     *string    `json:"paymentNotes"`

	// Unified payment status, populated only in the historial list (nil in the
	// detail view, which derives crédito state live in the CuotasPanel). For
	// contado docs it reflects PaidAt; for crédito docs it's derived from the
	// cuotas schedule + recorded installment payments. PaymentStatus is
	// "pagado"|"parcial"|"pendiente"; PaymentOverdue flags any past-due cuota with
	// an outstanding balance (crédito only). Mirrors GetInstallmentsReport.
	PaymentStatus  *string `json:"paymentStatus,omitempty"`
	PaymentOverdue bool    `json:"paymentOverdue,omitempty"`

	// Detracción (SPOT). All nil = not subject to detracción. DetraccionCuentaBN
	// is the per-document override; when empty the company default is used at
	// issue time.
	DetraccionCodigo     *string `json:"detraccionCodigo"`
	DetraccionPorcentaje *string `json:"detraccionPorcentaje"`
	DetraccionMonto      *string `json:"detraccionMonto"`
	DetraccionCuentaBN   *string `json:"detraccionCuentaBn"`

	ReferenceDocType        *string `json:"referenceDocType"`
	ReferenceDocSeries      *string `json:"referenceDocSeries"`
	ReferenceDocCorrelative *int    `json:"referenceDocCorrelative"`
	CreditDebitReasonCode   *string `json:"creditDebitReasonCode"`
	CreditDebitReasonDesc   *string `json:"creditDebitReasonDesc"`

	SunatResponseCode        *string       `json:"sunatResponseCode"`
	SunatResponseDescription *string       `json:"sunatResponseDescription"`
	SunatTicket              *string       `json:"sunatTicket"`
	SunatObservations        []Observation `json:"sunatObservations,omitempty"`

	R2XmlKey       *string `json:"r2XmlKey"`
	R2SignedXmlKey *string `json:"r2SignedXmlKey"`
	R2ZipKey       *string `json:"r2ZipKey"`
	R2CdrKey       *string `json:"r2CdrKey"`
	R2PdfKey       *string `json:"r2PdfKey"`

	QrData *string `json:"qrData"`

	// ScheduleOrigin says which kind of schedule emitted this comprobante; nil when
	// a user issued it by hand. Derived from comprobante_schedule_runs at read time
	// — there is no column on issued_documents, so it can't drift from the run
	// history that is the actual record of what emitted what.
	ScheduleOrigin *ScheduleOrigin `json:"scheduleOrigin"`

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
	ProductoID             *string   `json:"productoId"`
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

// ComprobanteAttachment is a supporting file attached to an issued comprobante
// (a signed order, a payment voucher, a contract). Internal-only — never exposed
// on the public comprobante share link. The bytes live in R2 under r2Key.
type ComprobanteAttachment struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"documentId"`
	FileName   string    `json:"fileName"`
	MimeType   string    `json:"mimeType"`
	FileSize   int64     `json:"fileSize"`
	CreatedAt  time.Time `json:"createdAt"`
}

// InstallmentPayment is one payment recorded against a credit document's cuota.
// CuotaNumero matches the cuotas[].numero on the parent issued_documents row.
type InstallmentPayment struct {
	ID          string    `json:"id"`
	DocumentID  string    `json:"documentId"`
	CuotaNumero int       `json:"cuotaNumero"`
	Amount      string    `json:"amount"`
	PaidAt      time.Time `json:"paidAt"`
	Method      *string   `json:"method"`
	Reference   *string   `json:"reference"`
	Notes       *string   `json:"notes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	// DocumentIDs is populated only by the list query (aggregated from
	// voided_document_items). It's empty on single-get, where Items is used.
	DocumentIDs []string `json:"documentIds,omitempty"`
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
