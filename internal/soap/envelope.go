package soap

import (
	"encoding/base64"
	"fmt"
)

const (
	nsSoapenv = "http://schemas.xmlsoap.org/soap/envelope/"
	nsSer     = "http://service.sunat.gob.pe"
	nsWSSE    = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
)

// buildSendBillEnvelope creates a SOAP envelope for the sendBill method.
func buildSendBillEnvelope(username, password, filename string, zipContent []byte) []byte {
	b64Content := base64.StdEncoding.EncodeToString(zipContent)
	return buildEnvelope(username, password, "sendBill", fmt.Sprintf(
		"<fileName>%s</fileName><contentFile>%s</contentFile>",
		filename+".zip", b64Content,
	))
}

// buildSendSummaryEnvelope creates a SOAP envelope for the sendSummary method.
func buildSendSummaryEnvelope(username, password, filename string, zipContent []byte) []byte {
	b64Content := base64.StdEncoding.EncodeToString(zipContent)
	return buildEnvelope(username, password, "sendSummary", fmt.Sprintf(
		"<fileName>%s</fileName><contentFile>%s</contentFile>",
		filename+".zip", b64Content,
	))
}

// buildGetStatusEnvelope creates a SOAP envelope for the getStatus method.
func buildGetStatusEnvelope(username, password, ticket string) []byte {
	return buildEnvelope(username, password, "getStatus", fmt.Sprintf(
		"<ticket>%s</ticket>", ticket,
	))
}

// buildGetStatusCdrEnvelope creates a SOAP envelope for the getStatusCdr method.
func buildGetStatusCdrEnvelope(username, password, ruc, tipo, serie string, numero int) []byte {
	return buildEnvelope(username, password, "getStatusCdr", fmt.Sprintf(
		"<rucComprobante>%s</rucComprobante><tipoComprobante>%s</tipoComprobante><serieComprobante>%s</serieComprobante><numeroComprobante>%d</numeroComprobante>",
		ruc, tipo, serie, numero,
	))
}

func buildEnvelope(username, password, method, body string) []byte {
	xml := fmt.Sprintf(`<soapenv:Envelope xmlns:soapenv="%s" xmlns:ser="%s" xmlns:wsse="%s">
<soapenv:Header>
<wsse:Security>
<wsse:UsernameToken>
<wsse:Username>%s</wsse:Username>
<wsse:Password>%s</wsse:Password>
</wsse:UsernameToken>
</wsse:Security>
</soapenv:Header>
<soapenv:Body>
<ser:%s>%s</ser:%s>
</soapenv:Body>
</soapenv:Envelope>`, nsSoapenv, nsSer, nsWSSE, username, password, method, body, method)
	return []byte(xml)
}
