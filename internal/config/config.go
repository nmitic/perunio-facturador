package config

import (
	"fmt"
	"os"
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

	SunatTimeoutSeconds int
}

// Load reads configuration from environment variables and validates required
// fields. Secret values are NOT read here — see awssecrets.Service.
func Load() (Config, error) {
	c := Config{
		Port:                env("PORT", "8080"),
		DatabaseURL:         env("DATABASE_URL", ""),
		AWSSecretName:       env("AWS_SECRET_NAME", ""),
		AWSRegion:           env("AWS_REGION", ""),
		R2AccountID:         env("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:       env("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:   env("R2_SECRET_ACCESS_KEY", ""),
		R2CertificatesBucket: env("R2_CERTIFICATES_BUCKET", "perunio-certificates"),
		R2DocumentsBucket:    env("R2_DOCUMENTS_BUCKET", "perunio-facturador"),
		SunatBetaURL:        env("SUNAT_BETA_URL", "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService"),
		SunatProductionURL:  env("SUNAT_PRODUCTION_URL", "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService"),
		SunatConsultURL:     env("SUNAT_CONSULT_URL", "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"),
		SunatTimeoutSeconds: 30,
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
