package model

// IssueRequest is the payload the backend sends to issue a document.
type IssueRequest struct {
	// Supplier
	SupplierRUC       string `json:"supplierRuc"`
	SupplierName      string `json:"supplierName"`
	SupplierAddress   string `json:"supplierAddress"`
	EstablishmentCode string `json:"establishmentCode"` // "0000" default

	// Document identity
	DocType     string `json:"docType"`     // "01","03","07","08"
	Series      string `json:"series"`      // "F001","B001","FC01","FD01"
	Correlative int    `json:"correlative"`

	// Dates
	IssueDate string `json:"issueDate"` // YYYY-MM-DD
	IssueTime string `json:"issueTime"` // HH:mm:ss

	// Currency & operation
	CurrencyCode  string `json:"currencyCode"`  // ISO 4217
	OperationType string `json:"operationType"` // Cat.51

	// Customer
	CustomerDocType   string `json:"customerDocType"`   // Cat.06
	CustomerDocNumber string `json:"customerDocNumber"`
	CustomerName      string `json:"customerName"`
	CustomerAddress   string `json:"customerAddress"`

	// Totals (strings to preserve decimal precision)
	Subtotal           string `json:"subtotal"`
	TotalIGV           string `json:"totalIgv"`
	TotalISC           string `json:"totalIsc"`
	TotalOtherTaxes    string `json:"totalOtherTaxes"`
	TotalDiscount      string `json:"totalDiscount"`
	TotalAmount        string `json:"totalAmount"`
	TaxInclusiveAmount string `json:"taxInclusiveAmount"`

	// Notes
	Notes []Note `json:"notes"`

	// Credit/Debit note reference
	ReferenceDocType        string `json:"referenceDocType"`
	ReferenceDocSeries      string `json:"referenceDocSeries"`
	ReferenceDocCorrelative int    `json:"referenceDocCorrelative"`
	ReasonCode              string `json:"reasonCode"`
	ReasonDescription       string `json:"reasonDescription"`

	// Items
	Items []LineItem `json:"items"`

	// Certificate
	CertificateURL      string `json:"certificateUrl"`      // Presigned R2 URL
	CertificatePassword string `json:"certificatePassword"` // AES-256-GCM encrypted

	// SUNAT credentials
	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"` // "beta" or "production"
}

// Note is a legend/note attached to the document.
type Note struct {
	Code string `json:"code"` // Cat.52 legend code
	Text string `json:"text"`
}

// LineItem is a single invoice line.
type LineItem struct {
	LineNumber             int    `json:"lineNumber"`
	Description            string `json:"description"`
	Quantity               string `json:"quantity"`
	UnitCode               string `json:"unitCode"` // UN/ECE rec 20
	UnitPrice              string `json:"unitPrice"`
	UnitPriceWithTax       string `json:"unitPriceWithTax"`
	TaxExemptionReasonCode string `json:"taxExemptionReasonCode"` // Cat.07
	IGVAmount              string `json:"igvAmount"`
	ISCAmount              string `json:"iscAmount"`
	ISCTierRange           string `json:"iscTierRange"` // Cat.08: 01/02/03
	DiscountAmount         string `json:"discountAmount"`
	LineTotal              string `json:"lineTotal"`
	PriceTypeCode          string `json:"priceTypeCode"` // "01" or "02"
}

// IssueResponse is returned to the backend after processing.
type IssueResponse struct {
	Success      bool          `json:"success"`
	SignedXML    []byte        `json:"signedXml,omitempty"`
	ZipBytes     []byte        `json:"zipBytes,omitempty"`
	CDRBytes     []byte        `json:"cdrBytes,omitempty"`
	PDFBytes     []byte        `json:"pdfBytes,omitempty"`
	ResponseCode string        `json:"responseCode,omitempty"`
	Description  string        `json:"description,omitempty"`
	Observations []Observation `json:"observations,omitempty"`
	QRData       string        `json:"qrData,omitempty"`
	Filename     string        `json:"filename,omitempty"`
	ErrorCode    string        `json:"errorCode,omitempty"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
}

// Observation is a SUNAT observation (code 4000+).
type Observation struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateResponse is for dry-run validation.
type ValidateResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError carries a SUNAT error code.
type ValidationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}
