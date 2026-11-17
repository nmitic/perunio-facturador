package http

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/perunio/perunio-facturador/internal/greclient"
)

// A document goes to the environment it was numbered in, whatever the request
// or the company's current setting says.
func TestStampedPipelineEnv(t *testing.T) {
	for _, tc := range []struct {
		override, stamped string
		want              string
		wantCode          string
	}{
		{"", "beta", "beta", ""},
		{"", "production", "production", ""},
		{"beta", "beta", "beta", ""},
		{"production", "production", "production", ""},
		// A stamp we do not recognise is treated as production, the value
		// every row had before the column existed.
		{"", "", "production", ""},
		// The dangerous one: a guía numbered in the sandbox sent to SUNAT.
		{"production", "beta", "", "ENVIRONMENT_MISMATCH"},
		{"beta", "production", "", "ENVIRONMENT_MISMATCH"},
	} {
		got, pErr := stampedPipelineEnv(tc.override, tc.stamped)
		switch {
		case tc.wantCode == "" && pErr != nil:
			t.Errorf("override=%q stamped=%q: unexpected error %s", tc.override, tc.stamped, pErr.code)
		case tc.wantCode != "" && (pErr == nil || pErr.code != tc.wantCode || pErr.status != http.StatusBadRequest):
			t.Errorf("override=%q stamped=%q: want 400 %s, got %+v", tc.override, tc.stamped, tc.wantCode, pErr)
		case got != tc.want:
			t.Errorf("override=%q stamped=%q: env = %q, want %q", tc.override, tc.stamped, got, tc.want)
		}
	}
}

func TestGRESendErrorMessage(t *testing.T) {
	answered := &greclient.APIErrorResponse{
		StatusCode: 500, Stage: "send comprobante",
		Err: &greclient.APIError{Cod: "500", Msg: "Error inesperado"},
	}
	unreachable := errors.New("send request failed: dial tcp: connection refused")

	if got := greSendErrorMessage("beta", answered); got != answered.Error() {
		t.Errorf("beta, server answered: %q, want the server's own message", got)
	}
	if got := greSendErrorMessage("beta", unreachable); !strings.Contains(got, "servidor de pruebas") {
		t.Errorf("beta, no answer: %q, want it to name the test server", got)
	}
	// In production the user must see what actually happened with SUNAT.
	if got := greSendErrorMessage("production", unreachable); got != unreachable.Error() {
		t.Errorf("production, no answer: %q, want the raw error", got)
	}
}

// Only a guía SUNAT accepted gets a PDF: it is what email and share links serve.
func TestDespatchPdfUploadable(t *testing.T) {
	for status, want := range map[string]bool{
		"accepted": true, "draft": false, "signed": false, "sent": false, "rejected": false, "error": false,
	} {
		if got := despatchPdfUploadable(status); got != want {
			t.Errorf("status %q: uploadable = %v, want %v", status, got, want)
		}
	}
}
