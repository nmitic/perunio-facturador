// Package r2 provides Cloudflare R2 (S3-compatible) access for the facturador
// service. R2 holds the generated SUNAT artifacts (signed XML, ZIP, CDR, PDF).
// Customer certificates are owned by perunio-backend and live in the DB, not
// R2.
package r2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client wraps the S3 SDK client wired to a Cloudflare R2 endpoint, plus the
// documents bucket name the service writes to.
type Client struct {
	s3              *s3.Client
	presigner       *s3.PresignClient
	documentsBucket string
}

// Config holds the R2 connection settings, normally sourced from
// internal/config.Config.
type Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	DocumentsBucket string
}

// New constructs an R2 client. R2 is S3-compatible and lives at
// https://{accountId}.r2.cloudflarestorage.com. Region is fixed to "auto" per
// Cloudflare's recommendation.
func New(ctx context.Context, c Config) (*Client, error) {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.AccessKeyID, c.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config for R2: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &Client{
		s3:              s3Client,
		presigner:       s3.NewPresignClient(s3Client),
		documentsBucket: c.DocumentsBucket,
	}, nil
}

// DocumentsBucket returns the bucket holding generated SUNAT artifacts.
func (c *Client) DocumentsBucket() string { return c.documentsBucket }
