package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SubmissionLogEntry captures a single SUNAT call (sendBill, sendSummary,
// getStatus, getStatusCdr) for compliance audit. Matches the Node schema at
// perunio-backend/src/db/schema.ts:sunatSubmissionLog. All pointer fields are
// optional because failure modes leave some blank (e.g. a network error
// produces no responseCode).
type SubmissionLogEntry struct {
	CompanyID           string
	DocumentID          *string
	SummaryID           *string
	VoidID              *string
	Action              string // sendBill | sendSummary | getStatus | getStatusCdr
	RequestHash         *string
	ResponseCode        *string
	ResponseDescription *string
	HTTPStatus          *int
	DurationMs          *int
}

// InsertSubmissionLog appends a compliance audit row inside a tenant-scoped tx
// so RLS populates tenant_id from app.current_tenant_id.
func (p *Pool) InsertSubmissionLog(ctx context.Context, entry SubmissionLogEntry) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO sunat_submission_log (
				tenant_id, company_id, document_id, summary_id, void_id,
				action, request_hash, response_code, response_description,
				http_status, duration_ms
			)
			VALUES (
				NULLIF(current_setting('app.current_tenant_id', true), '')::uuid,
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			)
		`,
			entry.CompanyID,
			entry.DocumentID, entry.SummaryID, entry.VoidID,
			entry.Action, entry.RequestHash,
			entry.ResponseCode, entry.ResponseDescription,
			entry.HTTPStatus, entry.DurationMs,
		)
		return err
	})
}
