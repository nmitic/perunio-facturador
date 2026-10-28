package db

import (
	"errors"
	"testing"
)

// The decision behind "may the user emit this scheduled period?". Getting it wrong
// either way is expensive: too strict strands a period the user must emit, too
// lenient puts two comprobantes for one period in front of SUNAT.
func TestCheckOccurrenceEmittable(t *testing.T) {
	s := func(v string) *string { return &v }

	cases := []struct {
		name       string
		resolution OccurrenceResolution
		docStatus  *string
		want       error
	}{
		{"open period", OccurrenceAbierta, nil, nil},
		{"rejected comprobante is re-emitted as a new one", OccurrenceEmitida, s("rejected"), nil},
		{"voided comprobante is re-emitted as a new one", OccurrenceEmitida, s("voided"), nil},
		{"accepted comprobante settles the period", OccurrenceEmitida, s("accepted"), ErrOccurrenceEmitted},
		{"accepted with observations settles the period", OccurrenceEmitida, s("accepted_with_observations"), ErrOccurrenceEmitted},
		// Every other facturador_doc_status value: no verdict yet.
		{"draft must be retried, not duplicated", OccurrenceEmitida, s("draft"), ErrOccurrencePendingDocument},
		{"pending must be retried, not duplicated", OccurrenceEmitida, s("pending"), ErrOccurrencePendingDocument},
		{"sending and awaiting a verdict", OccurrenceEmitida, s("sending"), ErrOccurrencePendingDocument},
		{"omitted by the user", OccurrenceOmitida, nil, ErrOccurrenceOmitted},
		{"omitted after a rejection", OccurrenceOmitida, s("rejected"), ErrOccurrenceOmitted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkOccurrenceEmittable(tc.resolution, tc.docStatus)
			if !errors.Is(got, tc.want) || (tc.want == nil && got != nil) {
				t.Errorf("checkOccurrenceEmittable(%q, %v) = %v, want %v", tc.resolution, tc.docStatus, got, tc.want)
			}
		})
	}
}

// States the database CHECK rules out must fail loudly rather than be read as
// "emittable" — they mean the schema and this code disagree.
func TestCheckOccurrenceEmittableRefusesImpossibleStates(t *testing.T) {
	sentinels := []error{ErrOccurrenceEmitted, ErrOccurrenceOmitted, ErrOccurrencePendingDocument, ErrOccurrenceNotFound}

	cases := []struct {
		name       string
		resolution OccurrenceResolution
	}{
		{"emitida without a document", OccurrenceEmitida},
		{"unknown resolution", OccurrenceResolution("vencida")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkOccurrenceEmittable(tc.resolution, nil)
			if err == nil {
				t.Fatal("got nil, want an error")
			}
			for _, sentinel := range sentinels {
				if errors.Is(err, sentinel) {
					t.Errorf("got business error %v, want an internal error", err)
				}
			}
		})
	}
}
