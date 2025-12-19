package main

import (
	"context"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const MaxFileSizeBytes = 5 * 1024 * 1024 // 5MB

var AllowedMimeTypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s <file>", os.Args[0])
	}

	filePath := os.Args[1]
	info, err := os.Stat(filePath)
	if err != nil {
		log.Fatal(err)
	}
	if info.Size() <= 0 || info.Size() > MaxFileSizeBytes {
		log.Fatalf("file size %d bytes violates policy (max %d)", info.Size(), MaxFileSizeBytes)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	region := os.Getenv("DO_REGION")
	bucket := os.Getenv("EVT_S3_BUCKET")
	keyID := os.Getenv("DO_SPACES_KEY")
	secret := os.Getenv("DO_SPACES_SECRET")

	if region == "" || bucket == "" || keyID == "" || secret == "" {
		log.Fatal("DO_REGION, EVT_S3_BUCKET, DO_SPACES_KEY, DO_SPACES_SECRET must be set")
	}

	// Determine Content-Type (simple, good-enough for this test)
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		// Fallback: sniff first 512 bytes
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		_, _ = file.Seek(0, 0)
		contentType = http.DetectContentType(buf[:n])
	}
	if !AllowedMimeTypes[contentType] {
		log.Fatalf("content-type %q not allowed by policy", contentType)
	}

	endpoint := fmt.Sprintf("https://%s.digitaloceanspaces.com", region)

	cfg, err := config.LoadDefaultConfig(
	context.TODO(),
	config.WithRegion(region),
	config.WithCredentialsProvider(
		credentials.NewStaticCredentialsProvider(
			os.Getenv("DO_SPACES_KEY"),
			os.Getenv("DO_SPACES_SECRET"),
			"",
		),
	),
)
if err != nil {
	log.Fatal(err)
}

client := s3.NewFromConfig(cfg, func(o *s3.Options) {
	o.BaseEndpoint = aws.String(endpoint)
})

	key := fmt.Sprintf("debug/manual/%s", filepath.Base(filePath))

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Uploaded: s3://%s/%s (Content-Type=%s, Size=%d)\n", bucket, key, contentType, info.Size())
}
