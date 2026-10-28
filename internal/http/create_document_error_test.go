package http

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/perunio/perunio-facturador/internal/db"
)

// The agenda branches on these codes (retry the pending comprobante, show it as
// already emitted, ...), so each occurrence refusal needs its own stable code —
// collapsing them into a generic 409 or 500 would leave the user with no next step.
func TestCreateDocumentError(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{db.ErrSeriesInactive, http.StatusBadRequest, "SERIES_NOT_FOUND"},
		{db.ErrDuplicate, http.StatusConflict, "DOCUMENT_DUPLICATE"},
		{db.ErrOccurrenceNotFound, http.StatusNotFound, "OCCURRENCE_NOT_FOUND"},
		{db.ErrOccurrenceEmitted, http.StatusConflict, "OCCURRENCE_ALREADY_EMITTED"},
		{db.ErrOccurrenceOmitted, http.StatusConflict, "OCCURRENCE_OMITTED"},
		{db.ErrOccurrencePendingDocument, http.StatusConflict, "OCCURRENCE_HAS_PENDING_DOCUMENT"},
		// Wrapped errors still map: the db layer may add context with %w.
		{fmt.Errorf("create: %w", db.ErrOccurrenceEmitted), http.StatusConflict, "OCCURRENCE_ALREADY_EMITTED"},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			got := createDocumentError(tc.err)
			if got == nil {
				t.Fatalf("createDocumentError(%v) = nil, want %s", tc.err, tc.wantCode)
			}
			if got.status != tc.wantStatus || got.code != tc.wantCode {
				t.Errorf("createDocumentError(%v) = %d %s, want %d %s", tc.err, got.status, got.code, tc.wantStatus, tc.wantCode)
			}
		})
	}

	if got := createDocumentError(errors.New("connection reset")); got != nil {
		t.Errorf("unexpected error mapped to %d %s, want nil (a logged 500)", got.status, got.code)
	}
}
