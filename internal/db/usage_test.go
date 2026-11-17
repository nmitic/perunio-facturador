package db

import (
	"testing"
	"time"
)

// The allowances mirror FACTURADOR_PLANS in perunio-backend/src/config/facturador-plans.ts.
func TestMonthlyDocumentLimit(t *testing.T) {
	cases := map[string]*int{
		"free":     intPtr(10),
		"emprende": intPtr(150),
		"negocio":  intPtr(600),
		"empresa":  nil,
		// A Perunio tier (or anything unknown) is not a facturador plan: Gratis.
		"accounting_pro": intPtr(10),
		"":               intPtr(10),
	}
	for plan, want := range cases {
		got := MonthlyDocumentLimit(plan)
		switch {
		case want == nil && got != nil:
			t.Errorf("%q: want unlimited, got %d", plan, *got)
		case want != nil && (got == nil || *got != *want):
			t.Errorf("%q: want %d, got %v", plan, *want, got)
		}
	}
}

func TestEffectivePlan(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	if got := effectivePlan("empresa", &past, now); got != "free" {
		t.Errorf("expired plan: got %q, want free", got)
	}
	if got := effectivePlan("empresa", &now, now); got != "free" {
		t.Errorf("plan expiring now: got %q, want free", got)
	}
	if got := effectivePlan("negocio", &future, now); got != "negocio" {
		t.Errorf("future expiry: got %q, want negocio", got)
	}
	if got := effectivePlan("emprende", nil, now); got != "emprende" {
		t.Errorf("no expiry: got %q, want emprende", got)
	}
}
