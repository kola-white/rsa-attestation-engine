package storage

import (
	"context"

	appcfg "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/config"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func NewS3Store(ctx context.Context, sc appcfg.SpacesConfig) (*S3Store, error) {
	creds := credentials.NewStaticCredentialsProvider(sc.AccessKey, sc.SecretKey, "")

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(sc.Region),
		config.WithCredentialsProvider(creds),
		config.WithBaseEndpoint(sc.Endpoint),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return newS3Store(sc.Bucket, client), nil
}
