package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewS3StoreRequiresConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  S3Config
	}{
		{name: "endpoint", cfg: S3Config{Bucket: "bucket", AccessKeyID: "key", SecretAccessKey: "secret"}},
		{name: "bucket", cfg: S3Config{Endpoint: "localhost:9000", AccessKeyID: "key", SecretAccessKey: "secret"}},
		{name: "access key", cfg: S3Config{Endpoint: "localhost:9000", Bucket: "bucket", SecretAccessKey: "secret"}},
		{name: "secret key", cfg: S3Config{Endpoint: "localhost:9000", Bucket: "bucket", AccessKeyID: "key"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewS3Store(tt.cfg); err == nil {
				t.Fatal("NewS3Store() error = nil, want required config error")
			}
		})
	}
}

func TestNormalizeS3Endpoint(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		useSSL     bool
		wantHost   string
		wantSecure bool
	}{
		{name: "plain host", endpoint: "minio:9000", useSSL: false, wantHost: "minio:9000", wantSecure: false},
		{name: "plain host secure override", endpoint: "s3.example.com", useSSL: true, wantHost: "s3.example.com", wantSecure: true},
		{name: "http url", endpoint: "http://minio:9000", useSSL: true, wantHost: "minio:9000", wantSecure: false},
		{name: "https url", endpoint: "https://s3.example.com", useSSL: false, wantHost: "s3.example.com", wantSecure: true},
		{name: "https slash", endpoint: "https://s3.example.com/", useSSL: false, wantHost: "s3.example.com", wantSecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, secure, err := normalizeS3Endpoint(tt.endpoint, tt.useSSL)
			if err != nil {
				t.Fatalf("normalizeS3Endpoint() error = %v", err)
			}
			if host != tt.wantHost || secure != tt.wantSecure {
				t.Fatalf("normalizeS3Endpoint() = %q/%v, want %q/%v", host, secure, tt.wantHost, tt.wantSecure)
			}
		})
	}
}

func TestNormalizeS3EndpointRejectsPathLikeValues(t *testing.T) {
	badEndpoints := []string{
		"",
		"http://minio:9000/path",
		"https://s3.example.com?bucket=x",
		"ftp://s3.example.com",
		"minio:9000/path",
		`minio:9000\path`,
	}

	for _, endpoint := range badEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			if _, _, err := normalizeS3Endpoint(endpoint, false); err == nil {
				t.Fatal("normalizeS3Endpoint() error = nil, want error")
			}
		})
	}
}

func TestS3StoreRejectsUnsafeKeysBeforeNetwork(t *testing.T) {
	store, err := NewS3Store(S3Config{
		Endpoint:        "127.0.0.1:1",
		Bucket:          "bucket",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewS3Store() error = %v", err)
	}

	for _, key := range []string{"../escape", "/absolute", "safe/../escape", `safe\windows`, "C:/drive"} {
		t.Run("put "+key, func(t *testing.T) {
			err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "text/plain")
			if !errors.Is(err, ErrInvalidObjectKey) {
				t.Fatalf("Put(%q) error = %v, want ErrInvalidObjectKey", key, err)
			}
		})
		t.Run("get "+key, func(t *testing.T) {
			_, _, err := store.Get(context.Background(), key)
			if !errors.Is(err, ErrInvalidObjectKey) {
				t.Fatalf("Get(%q) error = %v, want ErrInvalidObjectKey", key, err)
			}
		})
		t.Run("delete "+key, func(t *testing.T) {
			err := store.Delete(context.Background(), key)
			if !errors.Is(err, ErrInvalidObjectKey) {
				t.Fatalf("Delete(%q) error = %v, want ErrInvalidObjectKey", key, err)
			}
		})
	}
}

func TestS3StorePutGetDeleteIntegration(t *testing.T) {
	cfg := s3IntegrationConfig(t)
	store, err := NewS3Store(cfg)
	if err != nil {
		t.Fatalf("NewS3Store() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket() error = %v", err)
	}

	key := "tests/" + strconv.FormatInt(time.Now().UnixNano(), 10)
	body := "hello minio object store"
	if err := store.Put(ctx, key, strings.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	reader, info, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if string(payload) != body {
		t.Fatalf("payload = %q, want %q", string(payload), body)
	}
	if info.Key != key || info.Size != int64(len(body)) || info.ContentType != "text/plain" {
		t.Fatalf("ObjectInfo = %#v", info)
	}
	if info.UpdatedAt.IsZero() {
		t.Fatal("ObjectInfo.UpdatedAt is zero")
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrObjectNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() missing object error = %v", err)
	}
}

func s3IntegrationConfig(t *testing.T) S3Config {
	t.Helper()

	endpoint := os.Getenv("MM_CHAT_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set MM_CHAT_TEST_S3_ENDPOINT to run S3/MinIO integration tests")
	}
	accessKeyID := os.Getenv("MM_CHAT_TEST_S3_ACCESS_KEY_ID")
	if accessKeyID == "" {
		t.Skip("set MM_CHAT_TEST_S3_ACCESS_KEY_ID to run S3/MinIO integration tests")
	}
	secretAccessKey := os.Getenv("MM_CHAT_TEST_S3_SECRET_ACCESS_KEY")
	if secretAccessKey == "" {
		t.Skip("set MM_CHAT_TEST_S3_SECRET_ACCESS_KEY to run S3/MinIO integration tests")
	}
	bucket := os.Getenv("MM_CHAT_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "mm-chat-test"
	}
	region := os.Getenv("MM_CHAT_TEST_S3_REGION")
	if region == "" {
		region = defaultS3Region
	}
	useSSL := false
	if value := os.Getenv("MM_CHAT_TEST_S3_USE_SSL"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			t.Fatalf("parse MM_CHAT_TEST_S3_USE_SSL: %v", err)
		}
		useSSL = parsed
	}

	return S3Config{
		Endpoint:        endpoint,
		Bucket:          bucket,
		Region:          region,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		UseSSL:          useSSL,
	}
}
