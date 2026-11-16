// Package greclient is a REST client for SUNAT's Nueva Plataforma GRE
// (Guía de Remisión Electrónica). Unlike the SOAP billService used for
// Factura/Boleta/NC/ND, GRE uses OAuth2 (password grant) against
// api-seguridad.sunat.gob.pe and JSON/REST under api-cpe.sunat.gob.pe.
//
// See the official reference:
//
//	https://cpe.sunat.gob.pe/sites/default/files/inline-files/Manual_Servicios_GRE.pdf
package greclient

import "encoding/json"

// apiToken is the OAuth2 token response returned by
// POST /v1/clientessol/{client_id}/oauth2/token/.
type apiToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// cpeDocument is the request body for sending a comprobante.
// Shape per the SUNAT manual section "Enviar comprobante":
//
//	{ "archivo": { "nomArchivo": "...", "arcGreZip": "<b64>", "hashZip": "..." } }
type cpeDocument struct {
	Archivo cpeArchivo `json:"archivo"`
}

type cpeArchivo struct {
	NomArchivo string `json:"nomArchivo"`
	ArcGreZip  string `json:"arcGreZip"`
	HashZip    string `json:"hashZip"`
}

// CpeResponse is the response from the send endpoint.
type CpeResponse struct {
	NumTicket    string `json:"numTicket"`
	FecRecepcion string `json:"fecRecepcion"`
}

// StatusResponse is the response from the ticket query endpoint.
type StatusResponse struct {
	// CodRespuesta: "0" accepted, "98" in process, "99" rejected.
	CodRespuesta string `json:"codRespuesta"`
	// ArcCdr is the base64-encoded CDR ZIP (present on codRespuesta="0").
	ArcCdr string `json:"arcCdr"`
	// IndCdrGenerado is "1" when a CDR was generated, "0" otherwise.
	IndCdrGenerado string       `json:"indCdrGenerado"`
	Error          *StatusError `json:"error,omitempty"`
}

// StatusError is the error node on codRespuesta="99".
type StatusError struct {
	NumError string `json:"numError"`
	DesError string `json:"desError"`
}

// APIError is the generic non-validation error envelope returned by the
// GRE REST API. HTTP 5xx responses typically use this shape.
type APIError struct {
	Cod string
	Msg string
	Exc string
}

// UnmarshalJSON accepts what both servers send: SUNAT writes cod and exc as
// strings, the beta test server writes cod as a number and exc as a string or
// an array. A strict decode would drop the whole envelope, message included.
func (e *APIError) UnmarshalJSON(data []byte) error {
	var raw struct {
		Cod json.RawMessage `json:"cod"`
		Msg string          `json:"msg"`
		Exc json.RawMessage `json:"exc"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Cod = scalarText(raw.Cod)
	e.Msg = raw.Msg
	e.Exc = scalarText(raw.Exc)
	return nil
}

// scalarText returns a JSON string's value, a number's digits, and "" for
// anything else (null, object, array).
func scalarText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// APIValidationError is the HTTP 422 response shape.
type APIValidationError struct {
	Cod    string     `json:"cod"`
	Msg    string     `json:"msg"`
	Errors []APIError `json:"errors,omitempty"`
}
