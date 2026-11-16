package config

import (
	"fmt"
	"net/url"
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
	// SunatGRESecurityURL issues production tokens; SunatGREProductionURL hosts
	// the production /v1/contribuyente/gem/comprobantes/... endpoints.
	//
	// SUNAT has no GRE sandbox. Beta guías go to Nubefact's free test server,
	// which copies SUNAT's paths and answers with SUNAT's error codes after a
	// basic validation (XSD and part of the XSL rules). It takes one shared
	// credential set, published in Nubefact's documentation, so a company's own
	// client_secret and clave SOL never leave for it: SunatGREBetaClientID /
	// SunatGREBetaClientSecret are used instead, with {RUC}MODDATOS / MODDATOS.
	SunatGRESecurityURL      string
	SunatGREProductionURL    string
	SunatGREBetaSecurityURL  string
	SunatGREBetaURL          string
	SunatGREBetaClientID     string
	SunatGREBetaClientSecret string

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
		SunatGREProductionURL: env("SUNAT_GRE_PRODUCTION_URL", "https://api-cpe.sunat.gob.pe"),
		SunatTimeoutSeconds:   30,
		AllowedOrigins:        parseAllowedOrigins(env("ALLOWED_ORIGINS", "http://localhost:5173")),

		SunatGREBetaSecurityURL:  env("SUNAT_GRE_BETA_SECURITY_URL", "https://gre-test.nubefact.com"),
		SunatGREBetaURL:          env("SUNAT_GRE_BETA_URL", "https://gre-test.nubefact.com"),
		SunatGREBetaClientID:     env("SUNAT_GRE_BETA_CLIENT_ID", "test-85e5b0ae-255c-4891-a595-0b98c65c9854"),
		SunatGREBetaClientSecret: env("SUNAT_GRE_BETA_CLIENT_SECRET", "test-Hty/M6QshYvPgItX2P0+Kw=="),
	}

	if err := c.checkGREBetaIsNotProduction(); err != nil {
		return Config{}, err
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

// checkGREBetaIsNotProduction refuses a configuration in which a beta guía
// would reach SUNAT. Until this check existed SUNAT_GRE_BETA_URL defaulted to
// the production host, so every "prueba" guía was a real one.
func (c Config) checkGREBetaIsNotProduction() error {
	productionHosts := map[string]bool{}
	for _, v := range []struct{ name, url string }{
		{"SUNAT_GRE_SECURITY_URL", c.SunatGRESecurityURL},
		{"SUNAT_GRE_PRODUCTION_URL", c.SunatGREProductionURL},
	} {
		host, err := urlHost(v.url)
		if err != nil {
			return fmt.Errorf("%s: %w", v.name, err)
		}
		productionHosts[host] = true
	}
	for _, v := range []struct{ name, url string }{
		{"SUNAT_GRE_BETA_SECURITY_URL", c.SunatGREBetaSecurityURL},
		{"SUNAT_GRE_BETA_URL", c.SunatGREBetaURL},
	} {
		host, err := urlHost(v.url)
		if err != nil {
			return fmt.Errorf("%s: %w", v.name, err)
		}
		if productionHosts[host] || host == "sunat.gob.pe" || strings.HasSuffix(host, ".sunat.gob.pe") {
			return fmt.Errorf("%s points at %s: SUNAT has no GRE sandbox, so beta guías sent there would be real ones", v.name, host)
		}
	}
	return nil
}

func urlHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%q is not an absolute URL", raw)
	}
	return strings.ToLower(u.Hostname()), nil
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
