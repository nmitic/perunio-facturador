package db

import "errors"

// Business-logic errors returned by the db/service layer. HTTP handlers
// pattern-match on these to produce the right status + error code envelope.
var (
	ErrNotFound           = errors.New("not found")
	ErrNotDraft           = errors.New("only draft documents can be modified")
	ErrSeriesInactive     = errors.New("series not found or inactive")
	ErrSeriesHasDocuments = errors.New("series has associated documents")
	ErrCorrelativeTooLow  = errors.New("correlative must be raised past the last number already used")
	ErrDuplicate          = errors.New("duplicate")
	ErrQuotaExceeded      = errors.New("document quota exceeded")
	ErrNoBoletas          = errors.New("no unsummarized accepted boletas found for this date")
	ErrInvalidDocStatus   = errors.New("document not in accepted state")
	ErrVoidWindowExpired  = errors.New("document past the 7-day void window")
	ErrVoidMixedDates     = errors.New("documents in one comunicación de baja must share their issue date")
	ErrVentaHasDocuments  = errors.New("venta con anticipos has associated documents")
	// ErrVentaNoAbierta: only an abierta venta can be cancelled.
	ErrVentaNoAbierta = errors.New("venta con anticipos is not abierta")
	// ErrCancelacionIncompleta: a cancellation must give a valid destino for
	// every anticipo in force, and for nothing else.
	ErrCancelacionIncompleta = errors.New("cancelacion must cover exactly the anticipos in force")
	// ErrVentaHasNotas: a nota de crédito already reverses one of the venta's
	// anticipos, so the cancellation cannot be undone.
	ErrVentaHasNotas = errors.New("venta con anticipos has notas de crédito")
)
