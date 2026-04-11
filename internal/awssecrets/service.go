// Package awssecrets fetches application secrets from AWS Secrets Manager.
//
// Mirrors perunio-backend/src/services/aws-secrets.service.ts so the Go and
// Node.js services share the same source of truth: AWS Secrets Manager when
// AWS_SECRET_NAME is set, plain environment variables otherwise (for local dev
// and CI). Both backends point at the same AWS_SECRET_NAME in production.
package awssecrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// Secrets holds the four shared secrets used across the platform.
type Secrets struct {
	JWTSecret      string `json:"jwt_secret"`
	EncryptionKey  string `json:"encryption_key"`
	EncryptionKey2 string `json:"encryption_key_2"`
	PasswordPepper string `json:"password_pepper"`
}

// Service is a singleton holder for application secrets. Construct with New()
// then call Initialize() once at startup before any handler accesses secrets.
type Service struct {
	secrets  *Secrets
	usingAWS bool
}

// New returns an uninitialized Service. Call Initialize before reading secrets.
func New() *Service {
	return &Service{}
}

// envVarMap mirrors ENV_VAR_MAP in the Node.js implementation.
var envVarMap = map[string]string{
	"jwt_secret":       "JWT_SECRET",
	"encryption_key":   "ENCRYPTION_KEY",
	"encryption_key_2": "ENCRYPTION_KEY_2",
	"password_pepper":  "PASSWORD_PEPPER",
}

// Initialize loads secrets either from AWS Secrets Manager (when AWS_SECRET_NAME
// is set) or from individual environment variables. Fails fast if any required
// secret is missing or empty.
func (s *Service) Initialize(ctx context.Context) error {
	secretName := os.Getenv("AWS_SECRET_NAME")

	if secretName == "" {
		return s.loadFromEnv()
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		return fmt.Errorf("AWS_REGION is required when AWS_SECRET_NAME is set")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return fmt.Errorf("get secret %q: %w", secretName, err)
	}
	if out.SecretString == nil {
		return fmt.Errorf("secret %q has no string value", secretName)
	}

	var parsed Secrets
	if err := json.Unmarshal([]byte(*out.SecretString), &parsed); err != nil {
		return fmt.Errorf("parse secret %q JSON: %w", secretName, err)
	}

	if err := validate(&parsed); err != nil {
		return fmt.Errorf("secret %q: %w", secretName, err)
	}

	s.secrets = &parsed
	s.usingAWS = true
	return nil
}

func (s *Service) loadFromEnv() error {
	parsed := Secrets{
		JWTSecret:      os.Getenv("JWT_SECRET"),
		EncryptionKey:  os.Getenv("ENCRYPTION_KEY"),
		EncryptionKey2: os.Getenv("ENCRYPTION_KEY_2"),
		PasswordPepper: os.Getenv("PASSWORD_PEPPER"),
	}
	if err := validate(&parsed); err != nil {
		return err
	}
	s.secrets = &parsed
	s.usingAWS = false
	return nil
}

func validate(s *Secrets) error {
	missing := []string{}
	if s.JWTSecret == "" {
		missing = append(missing, envVarMap["jwt_secret"])
	}
	if s.EncryptionKey == "" {
		missing = append(missing, envVarMap["encryption_key"])
	}
	if s.EncryptionKey2 == "" {
		missing = append(missing, envVarMap["encryption_key_2"])
	}
	if s.PasswordPepper == "" {
		missing = append(missing, envVarMap["password_pepper"])
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required secrets: %v", missing)
	}
	return nil
}

// JWTSecret returns the HMAC secret used for JWT signing/verification.
func (s *Service) JWTSecret() string {
	return s.mustSecret().JWTSecret
}

// EncryptionKey returns the primary AES-256-GCM key (64-char hex string).
func (s *Service) EncryptionKey() string {
	return s.mustSecret().EncryptionKey
}

// EncryptionKey2 returns the secondary AES-256-GCM key for SUNAT API credentials.
func (s *Service) EncryptionKey2() string {
	return s.mustSecret().EncryptionKey2
}

// PasswordPepper returns the HMAC pepper applied before bcrypt hashing.
func (s *Service) PasswordPepper() string {
	return s.mustSecret().PasswordPepper
}

// IsUsingAWS reports whether secrets were loaded from AWS Secrets Manager.
func (s *Service) IsUsingAWS() bool {
	return s.usingAWS
}

func (s *Service) mustSecret() *Secrets {
	if s.secrets == nil {
		panic("awssecrets: Initialize() was not called")
	}
	return s.secrets
}
