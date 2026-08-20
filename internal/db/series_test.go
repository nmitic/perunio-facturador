package db

import (
	"errors"
	"strings"
	"testing"
)

// The HTTP layer branches on errors.Is(err, ErrCorrelativeTooLow) to turn a
// rejected correlative into a 409 with the minimum named. Drop the Unwrap and
// that branch silently stops matching, so every too-low correlative starts
// returning a 500 instead — with `go build` perfectly happy. Pin it.
func TestCorrelativeTooLowErrorUnwraps(t *testing.T) {
	var err error = &CorrelativeTooLowError{Floor: 4312}

	if !errors.Is(err, ErrCorrelativeTooLow) {
		t.Fatal("CorrelativeTooLowError must unwrap to ErrCorrelativeTooLow")
	}

	var typed *CorrelativeTooLowError
	if !errors.As(err, &typed) {
		t.Fatal("errors.As must recover the concrete error")
	}
	if typed.Floor != 4312 {
		t.Fatalf("Floor = %d, want 4312", typed.Floor)
	}
	if !strings.Contains(err.Error(), "4312") {
		t.Fatalf("message must name the floor, got %q", err.Error())
	}
}
