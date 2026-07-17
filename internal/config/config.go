package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the facturador service. Secret material
// (JWT key, encryption keys) is sourced from awssecrets.Service, not from this
// struct.
type Config struct {
	Port string

	// EncryptionKey is the AES-256 key (32 raw bytes) shared with the Node.js
	// backend for cert-password encryption. Populated by main.go from
	// awssecrets.Service.EncryptionKey() after Initialize, NOT loaded from env.
	EncryptionKey []byte

	// DatabaseURL is the PostgreSQL connection string for the shared DB.
	DatabaseURL string

	// DatabaseAdminURL connects as perunio_admin (BYPASSRLS). Used only by the
	// scheduler, which must enumerate due schedules across every tenant — RLS
	// would hide all but the current one, and a background tick has no current
	// tenant. Mirrors DATABASE_ADMIN_URL in perunio-backend (src/db/admin.ts).
	//
	// Deliberately has no fallback to DatabaseURL: the app role would find zero due
	// schedules and the service would look healthy while emitting nothing. Load
	// fails instead when the scheduler is enabled without it.
	DatabaseAdminURL string

	// SchedulerEnabled turns the comprobantes recurrentes/programados ticker on.
	// Off by default so local dev and one-off runs never emit to SUNAT unattended.
	SchedulerEnabled bool

	// AWS secrets bootstrap. AWSSecretName empty -> awssecrets falls back to
	// individual env vars (dev/CI mode).
	AWSSecretName string
	AWSRegion     string

	// Cloudflare R2 (S3-compatible) credentials and the documents bucket name.
	// Certificates are owned by perunio-backend (stored in the DB), so this
	// service only writes generated SUNAT artifacts to the documents bucket.
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2DocumentsBucket string

	SunatBetaURL       string
	SunatProductionURL string
	SunatConsultURL    string

	// GRE (Guía de Remisión Electrónica) REST API endpoints. Unlike the
	// SOAP billService used for Factura/Boleta/NC/ND, GRE uses OAuth2 +
	// REST under the "Plataforma Nueva GRE" (see
	// cpe.sunat.gob.pe/sites/default/files/inline-files/Manual_Servicios_GRE.pdf).
	// SunatGRESecurityURL issues tokens; SunatGREBetaURL / SunatGREProductionURL
	// host the /v1/contribuyente/gem/comprobantes/... endpoints.
	SunatGRESecurityURL   string
	SunatGREBetaURL       string
	SunatGREProductionURL string

	SunatTimeoutSeconds int

	// AllowedOrigins are the browser origins allowed to hit /api/facturador/*
	// with credentials. Loaded from ALLOWED_ORIGINS (comma-separated) with a
	// localhost-only dev default.
	AllowedOrigins []string
}

// Load reads configuration from environment variables and validates required
// fields. Secret values are NOT read here — see awssecrets.Service.
func Load() (Config, error) {
	_ = godotenv.Load() // dev only; no-ops when .env is absent or vars already set

	c := Config{
		Port:                  env("PORT", "3002"),
		DatabaseURL:           env("DATABASE_URL", ""),
		DatabaseAdminURL:      env("DATABASE_ADMIN_URL", ""),
		SchedulerEnabled:      env("SCHEDULER_ENABLED", "") == "true",
		AWSSecretName:         env("AWS_SECRET_NAME", ""),
		AWSRegion:             env("AWS_REGION", ""),
		R2AccountID:           env("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:         env("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:     env("R2_SECRET_ACCESS_KEY", ""),
		R2DocumentsBucket:     env("R2_DOCUMENTS_BUCKET", "perunio-facturador"),
		SunatBetaURL:          env("SUNAT_BETA_URL", "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService"),
		SunatProductionURL:    env("SUNAT_PRODUCTION_URL", "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService"),
		SunatConsultURL:       env("SUNAT_CONSULT_URL", "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"),
		SunatGRESecurityURL:   env("SUNAT_GRE_SECURITY_URL", "https://api-seguridad.sunat.gob.pe"),
		SunatGREBetaURL:       env("SUNAT_GRE_BETA_URL", "https://api-cpe.sunat.gob.pe"),
		SunatGREProductionURL: env("SUNAT_GRE_PRODUCTION_URL", "https://api-cpe.sunat.gob.pe"),
		SunatTimeoutSeconds:   30,
		AllowedOrigins:        parseAllowedOrigins(env("ALLOWED_ORIGINS", "http://localhost:5173")),
	}

	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if c.R2AccountID == "" {
		return Config{}, fmt.Errorf("R2_ACCOUNT_ID is required")
	}
	if c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" {
		return Config{}, fmt.Errorf("R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are required")
	}
	// The scheduler cannot see other tenants' rows through the RLS-bound app role,
	// so running it without an admin URL would silently emit nothing at all.
	if c.SchedulerEnabled && c.DatabaseAdminURL == "" {
		return Config{}, fmt.Errorf("DATABASE_ADMIN_URL is required when SCHEDULER_ENABLED=true")
	}

	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
