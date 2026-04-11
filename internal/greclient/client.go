package greclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the SUNAT Nueva Plataforma GRE REST API.
//
// It holds the per-environment base URLs and an in-memory OAuth2 token
// cache. One instance is shared across requests — tokens are keyed by
// (companyID, environment) so multi-tenant requests don't thrash each
// other's credentials.
type Client struct {
	securityURL string // https://api-seguridad.sunat.gob.pe
	cpeBetaURL  string // https://api-cpe.sunat.gob.pe (beta)
	cpeProdURL  string // https://api-cpe.sunat.gob.pe (prod)
	httpClient  *http.Client
	tokens      *tokenCache
}

// NewClient constructs a GRE client. timeoutSeconds governs both token
// and comprobante request timeouts.
func NewClient(securityURL, cpeBetaURL, cpeProductionURL string, timeoutSeconds int) *Client {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	return &Client{
		securityURL: strings.TrimRight(securityURL, "/"),
		cpeBetaURL:  strings.TrimRight(cpeBetaURL, "/"),
		cpeProdURL:  strings.TrimRight(cpeProductionURL, "/"),
		httpClient:  &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		tokens:      newTokenCache(),
	}
}

// Credentials bundles everything the OAuth2 password grant needs.
// SunatUsername must already be concatenated as "<RUC><SOL_user>".
type Credentials struct {
	ClientID      string
	ClientSecret  string
	SunatUsername string
	SunatPassword string
}

// cpeBaseURL returns the comprobante base URL for an environment.
func (c *Client) cpeBaseURL(environment string) string {
	if environment == "production" {
		return c.cpeProdURL
	}
	return c.cpeBetaURL
}

// tokenKey keys the token cache. Different environments get different
// cached tokens; the same company in beta and production must never
// share tokens.
func tokenKey(companyID, environment string) string {
	return companyID + "|" + environment
}

// getToken returns a valid access token for the company, fetching a new
// one from api-seguridad if the cache is empty or stale.
func (c *Client) getToken(ctx context.Context, companyID, environment string, creds Credentials) (string, error) {
	key := tokenKey(companyID, environment)
	if tok, ok := c.tokens.get(key); ok {
		return tok, nil
	}

	tok, err := c.requestToken(ctx, creds)
	if err != nil {
		return "", err
	}
	c.tokens.put(key, tok.AccessToken, tok.ExpiresIn)
	return tok.AccessToken, nil
}

// requestToken performs the OAuth2 password grant against
// POST /v1/clientessol/{client_id}/oauth2/token/.
func (c *Client) requestToken(ctx context.Context, creds Credentials) (*apiToken, error) {
	endpoint := fmt.Sprintf("%s/v1/clientessol/%s/oauth2/token/", c.securityURL, url.PathEscape(creds.ClientID))

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("scope", "https://api-cpe.sunat.gob.pe")
	form.Set("client_id", creds.ClientID)
	form.Set("client_secret", creds.ClientSecret)
	form.Set("username", creds.SunatUsername)
	form.Set("password", creds.SunatPassword)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.StatusCode, body, "oauth2 token")
	}

	var tok apiToken
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("oauth2 token response missing access_token")
	}
	return &tok, nil
}

// Send uploads a signed+zipped comprobante to the GRE endpoint and
// returns the async ticket. filename is the ZIP name WITHOUT the .zip
// extension (SUNAT uses it both in the URL and echoes it in nomArchivo
// with the extension appended).
func (c *Client) Send(ctx context.Context, companyID, environment string, creds Credentials, filename string, zipBytes []byte) (*CpeResponse, error) {
	token, err := c.getToken(ctx, companyID, environment, creds)
	if err != nil {
		return nil, err
	}

	resp, err := c.doSend(ctx, environment, token, filename, zipBytes)
	if err == nil {
		return resp, nil
	}

	// On 401, drop the cached token once and retry with a freshly
	// minted one — covers the case where SUNAT expired a token
	// before our refresh buffer kicked in.
	if isUnauthorized(err) {
		c.tokens.invalidate(tokenKey(companyID, environment))
		token, tErr := c.getToken(ctx, companyID, environment, creds)
		if tErr != nil {
			return nil, tErr
		}
		return c.doSend(ctx, environment, token, filename, zipBytes)
	}
	return nil, err
}

func (c *Client) doSend(ctx context.Context, environment, token, filename string, zipBytes []byte) (*CpeResponse, error) {
	hash := sha256.Sum256(zipBytes)
	body := cpeDocument{
		Archivo: cpeArchivo{
			NomArchivo: filename + ".zip",
			ArcGreZip:  base64.StdEncoding.EncodeToString(zipBytes),
			HashZip:    hex.EncodeToString(hash[:]),
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal cpe request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/contribuyente/gem/comprobantes/%s", c.cpeBaseURL(environment), url.PathEscape(filename))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read send response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.StatusCode, respBody, "send comprobante")
	}

	var out CpeResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode send response: %w", err)
	}
	if out.NumTicket == "" {
		return nil, fmt.Errorf("send response missing numTicket")
	}
	return &out, nil
}

// Poll queries the status of a previously-sent comprobante by ticket.
func (c *Client) Poll(ctx context.Context, companyID, environment string, creds Credentials, numTicket string) (*StatusResponse, error) {
	token, err := c.getToken(ctx, companyID, environment, creds)
	if err != nil {
		return nil, err
	}

	status, err := c.doPoll(ctx, environment, token, numTicket)
	if err == nil {
		return status, nil
	}
	if isUnauthorized(err) {
		c.tokens.invalidate(tokenKey(companyID, environment))
		token, tErr := c.getToken(ctx, companyID, environment, creds)
		if tErr != nil {
			return nil, tErr
		}
		return c.doPoll(ctx, environment, token, numTicket)
	}
	return nil, err
}

func (c *Client) doPoll(ctx context.Context, environment, token, numTicket string) (*StatusResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/contribuyente/gem/comprobantes/envios/%s", c.cpeBaseURL(environment), url.PathEscape(numTicket))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build poll request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read poll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.StatusCode, respBody, "poll ticket")
	}

	var out StatusResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode poll response: %w", err)
	}
	return &out, nil
}

// APIErrorResponse wraps HTTP non-200 responses with the decoded error
// envelope so callers can distinguish 422 validation failures from
// generic 5xx errors.
type APIErrorResponse struct {
	StatusCode int
	Stage      string
	Err        *APIError
	Validation *APIValidationError
}

func (e *APIErrorResponse) Error() string {
	if e.Validation != nil {
		return fmt.Sprintf("gre %s failed [%d %s]: %s", e.Stage, e.StatusCode, e.Validation.Cod, e.Validation.Msg)
	}
	if e.Err != nil {
		return fmt.Sprintf("gre %s failed [%d %s]: %s", e.Stage, e.StatusCode, e.Err.Cod, e.Err.Msg)
	}
	return fmt.Sprintf("gre %s failed: HTTP %d", e.Stage, e.StatusCode)
}

func parseAPIError(status int, body []byte, stage string) error {
	out := &APIErrorResponse{StatusCode: status, Stage: stage}
	if status == http.StatusUnprocessableEntity {
		var v APIValidationError
		if err := json.Unmarshal(body, &v); err == nil && (v.Cod != "" || v.Msg != "") {
			out.Validation = &v
			return out
		}
	}
	var e APIError
	if err := json.Unmarshal(body, &e); err == nil && (e.Cod != "" || e.Msg != "") {
		out.Err = &e
		return out
	}
	return out
}

func isUnauthorized(err error) bool {
	if e, ok := err.(*APIErrorResponse); ok {
		return e.StatusCode == http.StatusUnauthorized
	}
	return false
}
