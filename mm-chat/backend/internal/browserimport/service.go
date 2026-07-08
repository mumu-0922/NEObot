package browserimport

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repo            Repository
	maxPackageBytes int64
	now             func() time.Time
}

type ServiceOption func(*Service)

func WithMaxPackageBytes(maxPackageBytes int64) ServiceOption {
	return func(s *Service) {
		if maxPackageBytes > 0 {
			s.maxPackageBytes = maxPackageBytes
		}
	}
}

func WithNow(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(repo Repository, opts ...ServiceOption) *Service {
	service := &Service{
		repo:            repo,
		maxPackageBytes: defaultMaxPackageBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) Preview(_ context.Context, reader PackageInput) (PreviewResponse, error) {
	pkg, issues, err := s.readPackage(reader)
	if err != nil {
		return PreviewResponse{}, err
	}
	errors := filterIssues(issues, "error")
	warnings := filterIssues(issues, "warning")

	return PreviewResponse{
		Summary:       summaryFromManifest(pkg.Manifest),
		Warnings:      warnings,
		Errors:        errors,
		CommitAllowed: len(errors) == 0,
	}, nil
}

func (s *Service) Commit(ctx context.Context, reader PackageInput) (CommitResponse, error) {
	if err := s.requireRepository(); err != nil {
		return CommitResponse{}, err
	}
	pkg, issues, err := s.readPackage(reader)
	if err != nil {
		return CommitResponse{}, err
	}
	for _, issue := range issues {
		if issue.Severity == "error" {
			return CommitResponse{}, newValidationError(issue.Code, issue.Message)
		}
	}

	return s.repo.Commit(ctx, pkg)
}

func (s *Service) GetBatchStatus(ctx context.Context, batchID string) (BatchStatusResponse, error) {
	if err := s.requireRepository(); err != nil {
		return BatchStatusResponse{}, err
	}
	batchID = strings.TrimSpace(batchID)
	if !isUUID(batchID) {
		return BatchStatusResponse{}, newValidationError("INVALID_IMPORT_PAYLOAD", "batchId must be a UUID")
	}
	return s.repo.GetBatchStatus(ctx, batchID)
}

func (s *Service) Rollback(ctx context.Context, batchID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	batchID = strings.TrimSpace(batchID)
	if !isUUID(batchID) {
		return newValidationError("INVALID_IMPORT_PAYLOAD", "batchId must be a UUID")
	}
	return s.repo.Rollback(ctx, batchID)
}

func (s *Service) readPackage(input PackageInput) (Package, []Issue, error) {
	reader := PackageReader{MaxPackageBytes: s.maxPackageBytes, Now: s.now}
	return reader.Read(input.Reader)
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func summaryFromManifest(manifest Manifest) ImportSummary {
	return ImportSummary{
		Conversations: len(manifest.Conversations),
		Messages:      len(manifest.Messages),
		Files:         len(manifest.Files),
		Bytes:         manifest.Counts.Bytes,
	}
}

type PackageInput struct {
	Reader anyReader
}

type anyReader interface {
	Read(p []byte) (int, error)
}
