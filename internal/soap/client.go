package soap

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with SUNAT SOAP web services.
type Client struct {
	BillServiceURL    string
	ConsultServiceURL string
	HTTPClient        *http.Client
}

// NewClient creates a SOAP client for the given environment.
func NewClient(environment, betaURL, productionURL, consultURL string, timeoutSeconds int) *Client {
	serviceURL := betaURL
	if environment == "production" {
		serviceURL = productionURL
	}

	return &Client{
		BillServiceURL:    serviceURL,
		ConsultServiceURL: consultURL,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// SendBillResult holds the response from a sendBill call.
type SendBillResult struct {
	ApplicationResponse []byte // Base64-decoded ZIP containing CDR
}

// SendBill sends a signed ZIP to SUNAT (sync, for Factura/NC/ND).
func (c *Client) SendBill(username, password, filename string, zipContent []byte) (*SendBillResult, error) {
	envelope := buildSendBillEnvelope(username, password, filename, zipContent)

	respBody, err := c.doSOAP(c.BillServiceURL, envelope)
	if err != nil {
		return nil, err
	}

	// Parse applicationResponse from SOAP body
	b64Content, err := extractElement(respBody, "applicationResponse")
	if err != nil {
		return nil, fmt.Errorf("parse sendBill response: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Content)
	if err != nil {
		return nil, fmt.Errorf("decode applicationResponse: %w", err)
	}

	return &SendBillResult{ApplicationResponse: decoded}, nil
}

// SendSummaryResult holds the response from a sendSummary call.
type SendSummaryResult struct {
	Ticket string
}

// SendSummary sends a signed ZIP to SUNAT (async, for RC/RA).
func (c *Client) SendSummary(username, password, filename string, zipContent []byte) (*SendSummaryResult, error) {
	envelope := buildSendSummaryEnvelope(username, password, filename, zipContent)

	respBody, err := c.doSOAP(c.BillServiceURL, envelope)
	if err != nil {
		return nil, err
	}

	ticket, err := extractElement(respBody, "ticket")
	if err != nil {
		return nil, fmt.Errorf("parse sendSummary response: %w", err)
	}

	return &SendSummaryResult{Ticket: ticket}, nil
}

// GetStatusResult holds the response from a getStatus call.
type GetStatusResult struct {
	StatusCode string // "0"=done, "98"=processing, "99"=error
	Content    []byte // Base64-decoded CDR ZIP (when statusCode="0")
}

// GetStatus polls for the result of an async submission.
func (c *Client) GetStatus(username, password, ticket string) (*GetStatusResult, error) {
	envelope := buildGetStatusEnvelope(username, password, ticket)

	respBody, err := c.doSOAP(c.BillServiceURL, envelope)
	if err != nil {
		return nil, err
	}

	statusCode, err := extractElement(respBody, "statusCode")
	if err != nil {
		return nil, fmt.Errorf("parse getStatus response: %w", err)
	}

	result := &GetStatusResult{StatusCode: statusCode}

	if statusCode == "0" {
		content, err := extractElement(respBody, "content")
		if err == nil && content != "" {
			decoded, err := base64.StdEncoding.DecodeString(content)
			if err == nil {
				result.Content = decoded
			}
		}
	}

	return result, nil
}

// GetStatusCdrResult holds the response from a getStatusCdr call.
type GetStatusCdrResult struct {
	StatusCode string
	Content    []byte // CDR ZIP bytes
}

// GetStatusCdr queries the CDR for a specific document.
func (c *Client) GetStatusCdr(username, password, ruc, tipo, serie string, numero int) (*GetStatusCdrResult, error) {
	envelope := buildGetStatusCdrEnvelope(username, password, ruc, tipo, serie, numero)

	respBody, err := c.doSOAP(c.ConsultServiceURL, envelope)
	if err != nil {
		return nil, err
	}

	statusCode, _ := extractElement(respBody, "statusCode")
	result := &GetStatusCdrResult{StatusCode: statusCode}

	content, err := extractElement(respBody, "statusCdr")
	if err == nil && content != "" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err == nil {
			result.Content = decoded
		}
	}

	return result, nil
}

func (c *Client) doSOAP(url string, envelope []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(envelope))
	if err != nil {
		return nil, fmt.Errorf("create SOAP request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SOAP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SOAP response: %w", err)
	}

	// Check for SOAP faults
	if fault, err := extractElement(body, "faultstring"); err == nil && fault != "" {
		faultCode, _ := extractElement(body, "faultcode")
		return nil, &SOAPFault{Code: faultCode, Message: fault}
	}

	return body, nil
}

// SOAPFault represents a SUNAT SOAP fault.
type SOAPFault struct {
	Code    string
	Message string
}

func (f *SOAPFault) Error() string {
	return fmt.Sprintf("SOAP fault [%s]: %s", f.Code, f.Message)
}

// extractElement finds the first occurrence of a simple XML element in raw bytes.
func extractElement(data []byte, localName string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("element %q not found", localName)
		}
		if se, ok := token.(xml.StartElement); ok && se.Name.Local == localName {
			var content string
			if err := decoder.DecodeElement(&content, &se); err != nil {
				return "", err
			}
			return content, nil
		}
	}
}
