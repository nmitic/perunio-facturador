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

	// AWS secrets bootstrap. AWSSecretName empty -> awssecrets falls back to
	// individual env vars (dev/CI mode).
	AWSSecretName string
	AWSRegion     string

	// Cloudflare R2 (S3-compatible) credentials and bucket names. Both buckets
	// are shared with the Node.js backend.
	R2AccountID          string
	R2AccessKeyID        string
	R2SecretAccessKey    string
	R2CertificatesBucket string
	R2DocumentsBucket    string

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
		Port:                env("PORT", "3002"),
		DatabaseURL:         env("DATABASE_URL", ""),
		AWSSecretName:       env("AWS_SECRET_NAME", ""),
		AWSRegion:           env("AWS_REGION", ""),
		R2AccountID:         env("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:       env("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:   env("R2_SECRET_ACCESS_KEY", ""),
		R2CertificatesBucket: env("R2_CERTIFICATES_BUCKET", "perunio-certificates"),
		R2DocumentsBucket:    env("R2_DOCUMENTS_BUCKET", "perunio-facturador"),
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
