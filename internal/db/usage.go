package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
)

// monthlyDocumentLimits mirrors monthlyIssuedDocuments in
// perunio-backend/src/config/subscription-limits.ts. nil means unlimited.
var monthlyDocumentLimits = map[string]*int{
	"free":             intPtr(5),
	"accounting_basic": intPtr(50),
	"accounting":       intPtr(500),
	"accounting_pro":   nil,
}

func intPtr(i int) *int { return &i }

// MonthlyDocumentLimit returns the issued-document quota for the given
// subscription tier (nil = unlimited). Unknown tiers fall back to the free
// limit so we err on the side of restriction.
func MonthlyDocumentLimit(tier string) *int {
	if v, ok := monthlyDocumentLimits[tier]; ok {
		return v
	}
	return monthlyDocumentLimits["free"]
}

func currentPeriod() string {
	return time.Now().UTC().Format("200601")
}

// GetTenantTier reads the subscription_tier of the user that owns the tenant.
// Used by quota checks. Runs WITHOUT a tenant context because users/tenants
// are not RLS-scoped.
func (p *Pool) GetTenantTier(ctx context.Context, tenantID string) (string, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT u.subscription_tier
		FROM tenants t
		JOIN users u ON u.id = t.user_id
		WHERE t.id = $1
		LIMIT 1
	`, tenantID)
	var tier string
	if err := row.Scan(&tier); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "free", nil
		}
		return "", err
	}
	return tier, nil
}

// QuotaResult mirrors checkDocumentQuota in perunio-backend/src/db/index.ts.
type QuotaResult struct {
	Allowed   bool
	Used      int
	Remaining int
	Limit     *int // nil = unlimited
	Tier      string
	Period    string
}

// CheckDocumentQuota returns whether the tenant can issue `documentCount` more
// documents this period.
func (p *Pool) CheckDocumentQuota(ctx context.Context, tenantID string, documentCount int) (QuotaResult, error) {
	tier, err := p.GetTenantTier(ctx, tenantID)
	if err != nil {
		return QuotaResult{}, err
	}
	limit := MonthlyDocumentLimit(tier)
	period := currentPeriod()

	if limit == nil {
		return QuotaResult{
			Allowed: true, Used: 0, Remaining: 0, Limit: nil, Tier: tier, Period: period,
		}, nil
	}

	used, err := p.documentUsage(ctx, tenantID, period)
	if err != nil {
		return QuotaResult{}, err
	}
	remaining := *limit - used
	if remaining < 0 {
		remaining = 0
	}
	return QuotaResult{
		Allowed:   *limit-used >= documentCount,
		Used:      used,
		Remaining: remaining,
		Limit:     limit,
		Tier:      tier,
		Period:    period,
	}, nil
}

// IncrementDocumentUsage atomically bumps the period's document_count for the
// tenant. Mirrors incrementDocumentUsage in perunio-backend.
func (p *Pool) IncrementDocumentUsage(ctx context.Context, documentCount int) error {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return ErrTenantContextMissing
	}
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO facturador_usage (tenant_id, period, document_count)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, period)
			DO UPDATE SET document_count = facturador_usage.document_count + EXCLUDED.document_count,
			              updated_at = now()
		`, tenantID, currentPeriod(), documentCount)
		return err
	})
}

// GetDocumentUsage returns the current period's count + tier metadata for the
// /api/facturador/usage endpoint.
func (p *Pool) GetDocumentUsage(ctx context.Context, tenantID string) (QuotaResult, error) {
	tier, err := p.GetTenantTier(ctx, tenantID)
	if err != nil {
		return QuotaResult{}, err
	}
	limit := MonthlyDocumentLimit(tier)
	period := currentPeriod()
	used, err := p.documentUsage(ctx, tenantID, period)
	if err != nil {
		return QuotaResult{}, err
	}
	remaining := 0
	if limit != nil {
		remaining = *limit - used
		if remaining < 0 {
			remaining = 0
		}
	}
	return QuotaResult{
		Allowed: limit == nil || used < *limit,
		Used:    used, Remaining: remaining, Limit: limit, Tier: tier, Period: period,
	}, nil
}

func (p *Pool) documentUsage(ctx context.Context, tenantID, period string) (int, error) {
	var used int
	err := p.WithExplicitTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT document_count FROM facturador_usage
			WHERE tenant_id = $1 AND period = $2
			LIMIT 1
		`, tenantID, period)
		if scanErr := row.Scan(&used); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				used = 0
				return nil
			}
			return scanErr
		}
		return nil
	})
	return used, err
}
