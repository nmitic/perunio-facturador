package db

import "errors"

// Business-logic errors returned by the db/service layer. HTTP handlers
// pattern-match on these to produce the right status + error code envelope.
var (
	ErrNotFound           = errors.New("not found")
	ErrNotDraft           = errors.New("only draft documents can be modified")
	ErrSeriesInactive     = errors.New("series not found or inactive")
	ErrSeriesHasDocuments = errors.New("series has associated documents")
	ErrDuplicate          = errors.New("duplicate")
	ErrQuotaExceeded      = errors.New("document quota exceeded")
	ErrNoBoletas          = errors.New("no unsummarized accepted boletas found for this date")
	ErrInvalidDocStatus   = errors.New("document not in accepted state")
	ErrVoidWindowExpired  = errors.New("document past the 7-day void window")
)
