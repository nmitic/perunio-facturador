package model

// SummaryRequest is the payload for issuing a Resumen Diario (RC).
type SummaryRequest struct {
	SupplierRUC  string `json:"supplierRuc"`
	SupplierName string `json:"supplierName"`

	IssueDate     string `json:"issueDate"`     // YYYY-MM-DD (when the summary is generated)
	ReferenceDate string `json:"referenceDate"` // YYYY-MM-DD (emission date of the boletas)
	Correlative   int    `json:"correlative"`   // Sequential within the day (1-99999)

	Items []SummaryItem `json:"items"`

	CertificateURL      string `json:"certificateUrl"`
	CertificatePassword string `json:"certificatePassword"`

	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"`
}

// SummaryItem is a single boleta/NC/ND in a Resumen Diario.
type SummaryItem struct {
	LineNumber        int    `json:"lineNumber"`
	DocType           string `json:"docType"`           // "03","07","08"
	Series            string `json:"series"`
	StartCorrelative  int    `json:"startCorrelative"`
	EndCorrelative    int    `json:"endCorrelative"`
	ConditionCode     string `json:"conditionCode"`     // "1"=add, "2"=modify, "3"=void
	CustomerDocType   string `json:"customerDocType"`
	CustomerDocNumber string `json:"customerDocNumber"`
	ReferenceDocType  string `json:"referenceDocType"`
	ReferenceSeries   string `json:"referenceSeries"`
	ReferenceCorr     int    `json:"referenceCorrelative"`
	CurrencyCode      string `json:"currencyCode"`      // Must be PEN
	TotalAmount       string `json:"totalAmount"`
	TotalIGV          string `json:"totalIgv"`
	TotalISC          string `json:"totalIsc"`
	TotalOtherTaxes   string `json:"totalOtherTaxes"`
	TotalExonerated   string `json:"totalExonerated"`
	TotalUnaffected   string `json:"totalUnaffected"`
	TotalFree         string `json:"totalFree"`
}

// SummaryResponse is returned after sending a Resumen Diario.
type SummaryResponse struct {
	Success   bool   `json:"success"`
	Ticket    string `json:"ticket,omitempty"`
	Filename  string `json:"filename,omitempty"`
	SignedXML []byte `json:"signedXml,omitempty"`
	ZipBytes  []byte `json:"zipBytes,omitempty"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// TicketStatusRequest is for polling the status of an async submission.
type TicketStatusRequest struct {
	Ticket        string `json:"ticket"`
	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"`
}

// TicketStatusResponse contains the result of a ticket poll.
type TicketStatusResponse struct {
	Success      bool          `json:"success"`
	StatusCode   string        `json:"statusCode"` // "0"=done, "98"=processing, "99"=error
	CDRBytes     []byte        `json:"cdrBytes,omitempty"`
	ResponseCode string        `json:"responseCode,omitempty"`
	Description  string        `json:"description,omitempty"`
	Observations []Observation `json:"observations,omitempty"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// VoidRequest is the payload for issuing a Comunicacion de Baja (RA).
type VoidRequest struct {
	SupplierRUC  string `json:"supplierRuc"`
	SupplierName string `json:"supplierName"`

	IssueDate   string `json:"issueDate"`
	Correlative int    `json:"correlative"`

	Items []VoidItem `json:"items"`

	CertificateURL      string `json:"certificateUrl"`
	CertificatePassword string `json:"certificatePassword"`

	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"`
}

// VoidItem is a single document to void in a Comunicacion de Baja.
type VoidItem struct {
	LineNumber  int    `json:"lineNumber"`
	DocType     string `json:"docType"` // "01","07","08"
	Series      string `json:"series"`
	Correlative int    `json:"correlative"`
	VoidReason  string `json:"voidReason"`
}

// VoidResponse is returned after sending a Comunicacion de Baja.
type VoidResponse = SummaryResponse
