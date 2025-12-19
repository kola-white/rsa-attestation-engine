package storage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Store struct {
	bucket    string
	client    *s3.Client
	presigner *s3.PresignClient
}

func newS3Store(bucket string, client *s3.Client) *S3Store {
	return &S3Store{
		bucket:    bucket,
		client:    client,
		presigner: s3.NewPresignClient(client),
	}
}

func (s *S3Store) PresignPut(ctx context.Context, key, mime string, expires time.Duration) (url string, headers map[string]string, err error) {
	in := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(mime),
		ACL:         s3types.ObjectCannedACLPrivate,
	}

	out, err := s.presigner.PresignPutObject(ctx, in, func(o *s3.PresignOptions) {
		o.Expires = expires
	})
	if err != nil {
		return "", nil, err
	}

	// Client MUST send this header (otherwise signature mismatch).
	return out.URL, map[string]string{"Content-Type": mime}, nil
}

type HeadMeta struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
}

func (s *S3Store) Head(ctx context.Context, key string) (HeadMeta, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return HeadMeta{}, err
	}

	return HeadMeta{
		Key:         key,
		Size:        aws.ToInt64(out.ContentLength),
		ContentType: aws.ToString(out.ContentType),
		ETag:        aws.ToString(out.ETag),
	}, nil
}
