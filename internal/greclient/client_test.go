package greclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGRE stands in for one environment's servers (security + comprobantes on
// one host, as the beta test server is). It records every request it gets.
type fakeGRE struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	forms    []string
	// tokenBody, when set, replaces the token response.
	tokenBody string
	// sendStatuses are consumed one per send; empty means 200.
	sendStatuses []int
}

func newFakeGRE(t *testing.T, name string) *fakeGRE {
	t.Helper()
	f := &fakeGRE{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		defer f.mu.Unlock()

		switch {
		case strings.Contains(r.URL.Path, "/oauth2/token"):
			f.forms = append(f.forms, string(body))
			if f.tokenBody != "" {
				_, _ = io.WriteString(w, f.tokenBody)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"`+name+`-token","token_type":"JWT","expires_in":3600}`)
		case r.Method == http.MethodPost:
			if len(f.sendStatuses) > 0 {
				status := f.sendStatuses[0]
				f.sendStatuses = f.sendStatuses[1:]
				if status != http.StatusOK {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"cod":"401","msg":"token vencido"}`)
					return
				}
			}
			if got := r.Header.Get("Authorization"); got != "Bearer "+name+"-token" {
				t.Errorf("%s send: Authorization = %q", name, got)
			}
			_, _ = io.WriteString(w, `{"numTicket":"`+name+`-ticket","fecRecepcion":"2026-11-16T10:00:00"}`)
		default:
			_, _ = io.WriteString(w, `{"codRespuesta":"98"}`)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGRE) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func newTestClient(beta, production *fakeGRE) *Client {
	return NewClient(
		Endpoints{SecurityURL: beta.URL, CpeURL: beta.URL + "/"},
		Endpoints{SecurityURL: production.URL, CpeURL: production.URL},
		5,
	)
}

var testCreds = Credentials{
	ClientID:      "client-1",
	ClientSecret:  "secret-1",
	SunatUsername: "20000000001MODDATOS",
	SunatPassword: "MODDATOS",
}

// The invariant this package exists to hold: a beta guía is never seen by the
// production servers, token request included, and the reverse.
func TestEnvironmentsNeverCross(t *testing.T) {
	for _, tc := range []struct {
		environment string
		used        string
	}{
		{"beta", "beta"},
		{"production", "production"},
		// Anything that is not exactly "production" must stay in the sandbox.
		{"", "beta"},
		{"prod", "beta"},
	} {
		t.Run("env="+tc.environment, func(t *testing.T) {
			servers := map[string]*fakeGRE{
				"beta":       newFakeGRE(t, "beta"),
				"production": newFakeGRE(t, "production"),
			}
			c := newTestClient(servers["beta"], servers["production"])
			ctx := context.Background()

			sent, err := c.Send(ctx, "company-1", tc.environment, testCreds, "20000000001-09-T001-00000001", []byte("zip"))
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if sent.NumTicket != tc.used+"-ticket" {
				t.Errorf("ticket = %q, want it from the %s server", sent.NumTicket, tc.used)
			}
			if _, err := c.Poll(ctx, "company-1", tc.environment, testCreds, sent.NumTicket); err != nil {
				t.Fatalf("Poll: %v", err)
			}

			want := []string{
				"POST /v1/clientessol/client-1/oauth2/token/",
				"POST /v1/contribuyente/gem/comprobantes/20000000001-09-T001-00000001",
				"GET /v1/contribuyente/gem/comprobantes/envios/" + tc.used + "-ticket",
			}
			if got := servers[tc.used].seen(); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s server saw\n%s\nwant\n%s", tc.used, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			for name, srv := range servers {
				if name != tc.used && len(srv.seen()) != 0 {
					t.Errorf("%s server must see nothing, saw %v", name, srv.seen())
				}
			}
		})
	}
}

// One company in both environments must not reuse the other's token.
func TestTokenIsCachedPerEnvironment(t *testing.T) {
	beta, production := newFakeGRE(t, "beta"), newFakeGRE(t, "production")
	c := newTestClient(beta, production)
	ctx := context.Background()

	for _, environment := range []string{"beta", "production", "beta", "production"} {
		if _, err := c.Send(ctx, "company-1", environment, testCreds, "f", []byte("zip")); err != nil {
			t.Fatalf("Send(%s): %v", environment, err)
		}
	}
	for name, srv := range map[string]*fakeGRE{"beta": beta, "production": production} {
		tokens := 0
		for _, r := range srv.seen() {
			if strings.Contains(r, "/oauth2/token") {
				tokens++
			}
		}
		if tokens != 1 {
			t.Errorf("%s: %d token requests, want 1 (cached after the first)", name, tokens)
		}
	}
}

func TestSendRetriesOnceOnUnauthorized(t *testing.T) {
	beta, production := newFakeGRE(t, "beta"), newFakeGRE(t, "production")
	beta.sendStatuses = []int{http.StatusUnauthorized}
	c := newTestClient(beta, production)

	if _, err := c.Send(context.Background(), "company-1", "beta", testCreds, "f", []byte("zip")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := "token,send,token,send"
	var got []string
	for _, r := range beta.seen() {
		if strings.Contains(r, "/oauth2/token") {
			got = append(got, "token")
		} else {
			got = append(got, "send")
		}
	}
	if strings.Join(got, ",") != want {
		t.Errorf("requests = %s, want %s", strings.Join(got, ","), want)
	}
}

// The beta test server answers a refused grant with HTTP 200 and an error
// envelope. Its message is the only thing that tells the user what is wrong.
func TestTokenRefusalInA200IsAnError(t *testing.T) {
	beta, production := newFakeGRE(t, "beta"), newFakeGRE(t, "production")
	beta.tokenBody = `{"cod":"500","msg":"RUC incorrecto, el último dígito debería ser 0","exc":[{},{}]}`
	c := newTestClient(beta, production)

	_, err := c.Send(context.Background(), "company-1", "beta", testCreds, "f", []byte("zip"))
	var apiErr *APIErrorResponse
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want an *APIErrorResponse", err)
	}
	if !strings.Contains(err.Error(), "RUC incorrecto") {
		t.Errorf("error %q lost the server's message", err)
	}
}

func TestAPIErrorAcceptsBothServersShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want APIError
	}{
		"SUNAT: strings":             {`{"cod":"4001","msg":"mensaje","exc":"detalle"}`, APIError{"4001", "mensaje", "detalle"}},
		"test server: numeric cod":   {`{"cod":500,"msg":"Error inesperado","exc":"detalle"}`, APIError{"500", "Error inesperado", "detalle"}},
		"test server: exc is a list": {`{"cod":"500","msg":"'client_secret' incorrecto","exc":[{},{}]}`, APIError{"500", "'client_secret' incorrecto", ""}},
		"no exc":                     {`{"cod":404,"msg":"no existe"}`, APIError{"404", "no existe", ""}},
	} {
		t.Run(name, func(t *testing.T) {
			var got APIError
			if err := json.Unmarshal([]byte(tc.body), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
