package model

// CDR represents a parsed Constancia de Recepcion from SUNAT.
type CDR struct {
	ResponseCode string   `json:"responseCode"` // "0"=accepted, "98"=observations, "99"=rejected
	Description  string   `json:"description"`
	Notes        []string `json:"notes,omitempty"` // Observations (4000+ codes)
	Accepted     bool     `json:"accepted"`
	RawBytes     []byte   `json:"-"` // Original CDR ZIP bytes
}

// CDRQueryRequest is for querying a CDR for a specific document.
type CDRQueryRequest struct {
	SupplierRUC   string `json:"supplierRuc"`
	DocType       string `json:"docType"`
	Series        string `json:"series"`
	Correlative   int    `json:"correlative"`
	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"`
}

// CDRQueryResponse is returned after querying a CDR.
type CDRQueryResponse struct {
	Success      bool          `json:"success"`
	CDRBytes     []byte        `json:"cdrBytes,omitempty"`
	ResponseCode string        `json:"responseCode,omitempty"`
	Description  string        `json:"description,omitempty"`
	Observations []Observation `json:"observations,omitempty"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}
