package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/model"
)

// OccurrenceResolution mirrors the comprobante_occurrence_resolution enum owned by
// perunio-backend (src/db/schema.ts). An occurrence is one period of a
// recurrente/programado that came due; the facturador only ever moves it from
// abierta to emitida, inside the transaction that creates its comprobante.
type OccurrenceResolution string

const (
	OccurrenceAbierta OccurrenceResolution = "abierta"
	OccurrenceEmitida OccurrenceResolution = "emitida"
	OccurrenceOmitida OccurrenceResolution = "omitida"
)

// Reasons a comprobante cannot be emitted for an occurrence. Each maps to its own
// 4xx so the agenda can tell the user what to do next.
var (
	ErrOccurrenceNotFound = errors.New("schedule occurrence not found")
	// ErrOccurrenceEmitted: the period already has an accepted comprobante.
	ErrOccurrenceEmitted = errors.New("schedule occurrence already emitted")
	// ErrOccurrenceOmitted: the user skipped the period.
	ErrOccurrenceOmitted = errors.New("schedule occurrence was omitted")
	// ErrOccurrencePendingDocument: the period's comprobante exists but has no
	// verdict yet (draft, or an issue that failed before SUNAT answered). The user
	// retries issuing that document; a second one would burn a correlative and
	// could put two comprobantes for one period in front of SUNAT.
	ErrOccurrencePendingDocument = errors.New("schedule occurrence has a comprobante awaiting issue")
)

// checkOccurrenceEmittable decides whether a new comprobante may be created for
// an occurrence, given its resolution and the status of the comprobante already
// linked to it (nil when none).
//
// Only two cases allow it: an open period, and a period whose comprobante SUNAT
// rejected — the rejection consumed its correlative, so re-emitting means a new
// document, and the occurrence is relinked to it.
func checkOccurrenceEmittable(resolution OccurrenceResolution, linkedDocStatus *string) error {
	switch resolution {
	case OccurrenceAbierta:
		return nil
	case OccurrenceOmitida:
		return ErrOccurrenceOmitted
	case OccurrenceEmitida:
		// The resolution CHECK guarantees an emitida occurrence carries a document.
		if linkedDocStatus == nil {
			return fmt.Errorf("emitida occurrence without a document")
		}
		switch *linkedDocStatus {
		case "rejected":
			return nil
		case "accepted", "accepted_with_observations":
			return ErrOccurrenceEmitted
		default:
			return ErrOccurrencePendingDocument
		}
	default:
		return fmt.Errorf("unknown occurrence resolution %q", resolution)
	}
}

// lockOccurrenceForEmission row-locks an occurrence of companyID and checks that a
// comprobante may be created for it. Returns the origin to stamp on that
// comprobante — derived from which schedule the occurrence belongs to, never
// accepted from the client.
//
// Must run inside the transaction that inserts the comprobante: the lock is what
// serializes two concurrent emissions of the same period (two tabs, a double
// click), so the second sees the first's document and is refused.
func lockOccurrenceForEmission(ctx context.Context, tx pgx.Tx, occurrenceID, companyID string) (model.ScheduleOrigin, error) {
	var (
		resolution      OccurrenceResolution
		isRecurrente    bool
		linkedDocStatus *string
	)
	err := tx.QueryRow(ctx, `
		SELECT o.resolution::text, o.recurrente_id IS NOT NULL, d.status::text
		FROM comprobante_schedule_occurrences o
		LEFT JOIN issued_documents d ON d.id = o.document_id
		WHERE o.id = $1 AND o.company_id = $2
		FOR UPDATE OF o`, occurrenceID, companyID,
	).Scan(&resolution, &isRecurrente, &linkedDocStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOccurrenceNotFound
	}
	if err != nil {
		return "", err
	}

	if err := checkOccurrenceEmittable(resolution, linkedDocStatus); err != nil {
		return "", err
	}
	if isRecurrente {
		return model.ScheduleOriginRecurrente, nil
	}
	return model.ScheduleOriginProgramado, nil
}

// markOccurrenceEmitted links the comprobante just inserted to its occurrence.
// Runs in the same transaction as lockOccurrenceForEmission and the insert, so
// the period is never resolved without its document, nor the document left
// unlinked.
func markOccurrenceEmitted(ctx context.Context, tx pgx.Tx, occurrenceID, docID, userID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE comprobante_schedule_occurrences
		SET resolution = 'emitida', document_id = $2, omitted_reason = NULL,
		    resolved_at = now(), resolved_by_user_id = $3
		WHERE id = $1`, occurrenceID, docID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("link occurrence %s: %d rows updated", occurrenceID, tag.RowsAffected())
	}
	return nil
}

// reopenOccurrenceOfDraft returns the period a draft settled to abierta, so
// deleting the draft gives the user the period back instead of leaving it marked
// emitida with no comprobante. The resolution CHECK makes this mandatory: the
// document FK's ON DELETE SET NULL would otherwise violate it and fail the delete.
func reopenOccurrenceOfDraft(ctx context.Context, tx pgx.Tx, docID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE comprobante_schedule_occurrences
		SET resolution = 'abierta', document_id = NULL,
		    resolved_at = NULL, resolved_by_user_id = NULL
		WHERE document_id = $1 AND resolution = 'emitida'`, docID)
	return err
}
