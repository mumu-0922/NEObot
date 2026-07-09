package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const defaultS3Region = "us-east-1"

type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	ForcePathStyle  bool
}

type S3Store struct {
	client *minio.Client
	bucket string
	region string
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	endpoint, secure, err := normalizeS3Endpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}

	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	if accessKeyID == "" {
		return nil, errors.New("s3 access key id is required")
	}
	secretAccessKey := strings.TrimSpace(cfg.SecretAccessKey)
	if secretAccessKey == "" {
		return nil, errors.New("s3 secret access key is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = defaultS3Region
	}

	options := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: secure,
		Region: region,
	}
	if cfg.ForcePathStyle {
		options.BucketLookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpoint, options)
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	return &S3Store{
		client: client,
		bucket: bucket,
		region: region,
	}, nil
}

func (s *S3Store) EnsureBucket(ctx context.Context) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check s3 bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{
		Region: s.region,
	}); err != nil {
		return fmt.Errorf("create s3 bucket: %w", err)
	}

	return nil
}

func (s *S3Store) CheckReady(ctx context.Context) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check s3 bucket: %w", err)
	}
	if !exists {
		return errors.New("s3 bucket is not ready")
	}

	return nil
}

func (s *S3Store) Put(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	contentType string,
) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if body == nil {
		return errors.New("object body is required")
	}
	if size < 0 {
		return errors.New("object size must be non-negative")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{
		ContentType: strings.TrimSpace(contentType),
	})
	if err != nil {
		return fmt.Errorf("put s3 object: %w", err)
	}

	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	if err := s.requireReady(); err != nil {
		return nil, ObjectInfo{}, err
	}
	if err := validateObjectKey(key); err != nil {
		return nil, ObjectInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, ObjectInfo{}, err
	}

	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, mapS3ObjectError(err, "stat s3 object")
	}

	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, mapS3ObjectError(err, "get s3 object")
	}

	return object, ObjectInfo{
		Key:         key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
		UpdatedAt:   stat.LastModified,
	}, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		if isS3ObjectMissing(err) {
			return nil
		}
		return fmt.Errorf("delete s3 object: %w", err)
	}

	return nil
}

func (s *S3Store) requireReady() error {
	if s == nil || s.client == nil || strings.TrimSpace(s.bucket) == "" {
		return errors.New("s3 store is not initialized")
	}

	return nil
}

func normalizeS3Endpoint(rawEndpoint string, defaultSecure bool) (string, bool, error) {
	rawEndpoint = strings.TrimSpace(rawEndpoint)
	if rawEndpoint == "" {
		return "", false, errors.New("s3 endpoint is required")
	}
	if rawEndpoint != strings.Trim(rawEndpoint, "/") && !strings.Contains(rawEndpoint, "://") {
		return "", false, errors.New("s3 endpoint must not include a path")
	}

	secure := defaultSecure
	if strings.Contains(rawEndpoint, "://") {
		parsed, err := url.Parse(rawEndpoint)
		if err != nil {
			return "", false, fmt.Errorf("parse s3 endpoint: %w", err)
		}
		switch parsed.Scheme {
		case "http":
			secure = false
		case "https":
			secure = true
		default:
			return "", false, fmt.Errorf("unsupported s3 endpoint scheme %q", parsed.Scheme)
		}
		if parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", false, errors.New("s3 endpoint must be host only")
		}
		rawEndpoint = parsed.Host
	}
	if rawEndpoint == "" || strings.Contains(rawEndpoint, "/") || strings.Contains(rawEndpoint, "\\") {
		return "", false, errors.New("s3 endpoint must be host only")
	}

	return rawEndpoint, secure, nil
}

func mapS3ObjectError(err error, operation string) error {
	if isS3ObjectMissing(err) {
		return ErrObjectNotFound
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func isS3ObjectMissing(err error) bool {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchBucket", "NotFound", "NoSuchObject":
		return true
	default:
		return false
	}
}
