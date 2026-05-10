package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Company is the view of the companies row used by the issue pipeline. The
// username/encryptedPassword fields hold SUNAT SOL credentials (AES-GCM
// encrypted by the backend with the same key the facturador uses).
//
// EncryptedClientID and EncryptedClientSecret are the GRE API credentials
// (scope https://api-cpe.sunat.gob.pe), scraped by perunio-backend during
// company creation and used by the GRE REST pipeline for OAuth2 token
// exchange against api-seguridad.sunat.gob.pe. They are encrypted with the
// same AES-GCM key as the SOL password.
type Company struct {
	ID                    string
	TenantID              string
	RUC                   string
	CompanyName           string
	FiscalAddress         string
	Username              *string
	EncryptedPassword     *string
	EncryptedClientID     *string
	EncryptedClientSecret *string
	IsActive              bool
	// SunatEnvironment is the per-company default ("beta" | "production") the
	// issue pipeline uses when the request body omits it. Mirrors the
	// `companies.sunat_environment` column on the shared Postgres DB.
	SunatEnvironment string
	// PDF branding — empty strings mean "no override; use defaults".
	BrandColor string // "#RRGGBB"
	LogoBase64 string // data URI ("data:image/png;base64,...")
}

// GetCompany loads the company + SUNAT credentials for the issue pipeline,
// scoped to the tenant from context. Returns nil when not found.
func (p *Pool) GetCompany(ctx context.Context, companyID string) (*Company, error) {
	var c *Company
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, ruc, COALESCE(company_name, ''),
			       COALESCE(fiscal_address, ''),
			       username, password,
			       client_id, client_secret,
			       COALESCE(is_active, true),
			       COALESCE(sunat_environment::text, 'beta'),
			       COALESCE(brand_color, ''),
			       COALESCE(logo_base64, '')
			FROM companies
			WHERE id = $1
			LIMIT 1
		`, companyID)
		var got Company
		if err := row.Scan(&got.ID, &got.TenantID, &got.RUC, &got.CompanyName,
			&got.FiscalAddress,
			&got.Username, &got.EncryptedPassword,
			&got.EncryptedClientID, &got.EncryptedClientSecret,
			&got.IsActive, &got.SunatEnvironment,
			&got.BrandColor, &got.LogoBase64); err != nil {
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

