package model

import "time"

// CertificateValidateRequest is for validating a certificate.
type CertificateValidateRequest struct {
	CertificateURL      string `json:"certificateUrl"`
	CertificatePassword string `json:"certificatePassword"`
}

// CertificateInfo holds parsed certificate metadata.
type CertificateInfo struct {
	SerialNumber string    `json:"serialNumber"`
	Issuer       string    `json:"issuer"`
	Subject      string    `json:"subject"`
	ValidFrom    time.Time `json:"validFrom"`
	ValidTo      time.Time `json:"validTo"`
	IsExpired    bool      `json:"isExpired"`
	DaysUntilExp int       `json:"daysUntilExpiry"`
}

// CertificateValidateResponse is returned after validating a certificate.
type CertificateValidateResponse struct {
	Valid bool             `json:"valid"`
	Info  *CertificateInfo `json:"info,omitempty"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}
