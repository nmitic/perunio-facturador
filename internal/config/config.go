package config

import (
	"encoding/hex"
	"fmt"
	"os"
)

// Config holds all configuration for the facturador service.
type Config struct {
	Port          string
	APIKey        string
	EncryptionKey []byte // 32 bytes, decoded from 64-char hex

	SunatBetaURL       string
	SunatProductionURL string
	SunatConsultURL    string

	SunatTimeoutSeconds int
}

// Load reads configuration from environment variables and validates required fields.
func Load() (Config, error) {
	c := Config{
		Port:                env("PORT", "8080"),
		APIKey:              env("API_KEY", ""),
		SunatBetaURL:       env("SUNAT_BETA_URL", "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService"),
		SunatProductionURL: env("SUNAT_PRODUCTION_URL", "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService"),
		SunatConsultURL:    env("SUNAT_CONSULT_URL", "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"),
		SunatTimeoutSeconds: 30,
	}

	if c.APIKey == "" {
		return Config{}, fmt.Errorf("API_KEY is required")
	}

	encKeyHex := env("ENCRYPTION_KEY", "")
	if encKeyHex == "" {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY is required")
	}

	keyBytes, err := hex.DecodeString(encKeyHex)
	if err != nil {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be 64 hex chars (32 bytes), got %d bytes", len(keyBytes))
	}
	c.EncryptionKey = keyBytes

	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
