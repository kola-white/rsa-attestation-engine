package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestEvidenceObjectReadAndSeal(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		t.Run(map[bool]string{false: "content-length", true: "chunked"}[chunked], func(t *testing.T) {
			var sealed []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "GET":
					w.Header().Set("Content-Type", "application/pdf")
					if chunked {
						w.(http.Flusher).Flush()
					}
					io.WriteString(w, "abcdef")
				case "PUT":
					if r.URL.Path != "/test/sealed-key" || r.Header.Get("x-amz-acl") != "private" || r.Header.Get("Content-Type") != "application/pdf" {
						t.Error("incorrect sealed object")
					}
					var err error
					sealed, err = io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
				default:
					t.Error("unexpected storage request")
				}
			}))
			defer server.Close()
			client := s3.NewFromConfig(aws.Config{Region: "test", Credentials: credentials.NewStaticCredentialsProvider("fixture", "fixture", "")}, func(o *s3.Options) {
				o.BaseEndpoint = aws.String(server.URL)
				o.UsePathStyle = true
				o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			})
			store := newS3Store("test", client)
			if _, _, err := store.ReadEvidence(context.Background(), "staging", 5); err == nil {
				t.Fatal("oversize object accepted")
			}
			data, mime, err := store.ReadEvidence(context.Background(), "staging", 6)
			if err != nil || string(data) != "abcdef" || mime != "application/pdf" {
				t.Fatal("read failed", err)
			}
			if err = store.SealEvidence(context.Background(), "sealed-key", mime, data); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(sealed)) != "abcdef" {
				t.Fatal("sealed bytes differ")
			}
		})
	}
}
