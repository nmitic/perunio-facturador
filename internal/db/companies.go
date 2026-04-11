package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Company is the view of the companies row used by the issue pipeline. The
// username/encryptedPassword fields hold SUNAT SOL credentials (AES-GCM
// encrypted by the backend with the same key the facturador uses).
type Company struct {
	ID                string
	TenantID          string
	RUC               string
	CompanyName       string
	Username          *string
	EncryptedPassword *string
	IsActive          bool
}

// GetCompany loads the company + SUNAT credentials for the issue pipeline,
// scoped to the tenant from context. Returns nil when not found.
func (p *Pool) GetCompany(ctx context.Context, companyID string) (*Company, error) {
	var c *Company
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, ruc, COALESCE(company_name, ''), username, password,
			       COALESCE(is_active, true)
			FROM companies
			WHERE id = $1
			LIMIT 1
		`, companyID)
		var got Company
		if err := row.Scan(&got.ID, &got.TenantID, &got.RUC, &got.CompanyName,
			&got.Username, &got.EncryptedPassword, &got.IsActive); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		c = &got
		return nil
	})
	return c, err
}

// GetFiscalAddressByRUC looks up a RUC in the public ssco_entries table and
// returns its registered fiscal address. Returns an empty string when not
// found — the caller should fall back to a placeholder so the pipeline can
// still run during onboarding. ssco_entries is a public/global table so no
// tenant context is required.
func (p *Pool) GetFiscalAddressByRUC(ctx context.Context, ruc string) (string, error) {
	var address string
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(domicilio_fiscal, '')
		FROM ssco_entries
		WHERE ruc = $1
		LIMIT 1
	`, ruc).Scan(&address)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return address, nil
}
