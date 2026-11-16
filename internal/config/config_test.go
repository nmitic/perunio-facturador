package config

import (
	"strings"
	"testing"
)

func greConfig(betaSecurity, beta string) Config {
	return Config{
		SunatGRESecurityURL:     "https://api-seguridad.sunat.gob.pe",
		SunatGREProductionURL:   "https://api-cpe.sunat.gob.pe",
		SunatGREBetaSecurityURL: betaSecurity,
		SunatGREBetaURL:         beta,
	}
}

// SUNAT has no GRE sandbox. A beta URL that resolves to SUNAT turns every
// "prueba" guía into a real one, which is what the old default did.
func TestGREBetaMustNotBeSunat(t *testing.T) {
	const sandbox = "https://gre-test.nubefact.com"
	for name, tc := range map[string]struct {
		cfg     Config
		wantErr string
	}{
		"sandbox":                      {greConfig(sandbox, sandbox), ""},
		"local mock":                   {greConfig("http://localhost:9099", "http://localhost:9099"), ""},
		"beta is the production host":  {greConfig(sandbox, "https://api-cpe.sunat.gob.pe"), "SUNAT_GRE_BETA_URL"},
		"same host, other case, path":  {greConfig(sandbox, "https://API-CPE.sunat.gob.pe/v1/"), "SUNAT_GRE_BETA_URL"},
		"beta tokens from production":  {greConfig("https://api-seguridad.sunat.gob.pe", sandbox), "SUNAT_GRE_BETA_SECURITY_URL"},
		"another SUNAT host":           {greConfig(sandbox, "https://e-beta.sunat.gob.pe"), "SUNAT_GRE_BETA_URL"},
		"not a URL":                    {greConfig(sandbox, "gre-test.nubefact.com"), "SUNAT_GRE_BETA_URL"},
		"production moved, beta there": {Config{SunatGRESecurityURL: "https://sec.example", SunatGREProductionURL: "https://cpe.example", SunatGREBetaSecurityURL: sandbox, SunatGREBetaURL: "https://cpe.example"}, "SUNAT_GRE_BETA_URL"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.checkGREBetaIsNotProduction()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("accepted, want an error naming %s", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error %q does not name %s", err, tc.wantErr)
			}
		})
	}
}
