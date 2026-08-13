package skillsupply

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	testSkillAdmin = "11111111-1111-4111-8111-111111111111"
	testSkillUser  = "22222222-2222-4222-8222-222222222222"
)

func TestServiceNoExecuteIngestReviewInstallLifecycle(t *testing.T) {
	marker := t.TempDir() + "/must-not-exist"
	archive := mustTestArchive(t, []packageFile{
		{path: "SKILL.md", data: []byte(validSkillMarkdown("no-execute"))},
		{path: "scripts/run.sh", data: []byte("#!/bin/sh\ntouch " + marker + "\n")},
	}, 0, time.Time{})
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects),
		WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs(
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	)
	service.now = func() time.Time { return time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC) }

	if _, err := service.IngestZIP(context.Background(), testSkillUser, archive); !errors.Is(err, ErrAdministratorNeeded) {
		t.Fatalf("ordinary-user ingest error = %v", err)
	}
	candidate, err := service.IngestZIP(context.Background(), testSkillAdmin, archive)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != StatusValidated || !candidate.AdmissionEligible || len(objects.objects) != 3 {
		t.Fatalf("candidate=%#v objects=%#v", candidate, objects.objects)
	}
	if _, err := service.ReviewCandidate(context.Background(), testSkillAdmin, candidate.ID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 2,
		PackageFingerprint: candidate.Package.PackageFingerprint,
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale review error = %v", err)
	}
	admitted, err := service.ReviewCandidate(context.Background(), testSkillAdmin, candidate.ID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 1,
		PackageFingerprint: candidate.Package.PackageFingerprint, Reason: "reviewed fixture",
	})
	if err != nil || admitted.Revision != 2 || admitted.Status != StatusAdmitted {
		t.Fatalf("admission=%#v error=%v", admitted, err)
	}
	installation, err := service.Install(context.Background(), testSkillUser, candidate.ID,
		candidate.Package.PackageFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), testSkillUser, candidate.ID,
		candidate.Package.PackageFingerprint); !errors.Is(err, ErrInstallationConflict) {
		t.Fatalf("duplicate install error = %v", err)
	}
	if err := service.Uninstall(context.Background(), testSkillUser, installation.ID, 2); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale uninstall error = %v", err)
	}
	if err := service.Uninstall(context.Background(), testSkillAdmin, installation.ID, 1); !errors.Is(err, ErrInstallationNotFound) {
		t.Fatalf("cross-user uninstall error = %v", err)
	}
	if err := service.Uninstall(context.Background(), testSkillUser, installation.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetStoreItem(context.Background(), candidate.ID); err != nil {
		t.Fatalf("uninstall deleted admission: %v", err)
	}
	if _, err := service.IngestZIP(context.Background(), testSkillAdmin, archive); err != nil {
		t.Fatalf("idempotent source ingest: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate script executed; stat error = %v", err)
	}
}

func TestServiceRejectsSourceDriftAndIneligibleAdmission(t *testing.T) {
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects), WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	archiveOne := mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(validSkillMarkdown("demo-skill"))}}, 0, time.Time{})
	archiveTwo := mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(validSkillMarkdown("demo-skill") + "changed\n")}}, 0, time.Time{})
	first, err := ValidateArchive(ArchiveSource{Type: SourceLobeHub, Ref: "lobehub:demo-skill@1.2.3",
		Identifier: "demo-skill", ExpectedName: "demo-skill", Version: "1.2.3", Data: archiveOne})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first.Package.PackageObjectKey = digestObjectKey("skill-packages", first.Package.PackageFingerprint, ".zip")
	first.Package.SBOMObjectKey = digestObjectKey("skill-sboms", first.Package.SBOMFingerprint, ".cdx.json")
	_, err = repository.CreateCandidate(context.Background(), Candidate{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", SourceType: SourceLobeHub,
		SourceRef: "lobehub:demo-skill@1.2.3", SourceArtifactSHA256: first.SourceArtifactSHA256,
		SourceObjectKey: digestObjectKey("skill-quarantine", first.SourceArtifactSHA256, ".zip"),
		Package:         first.Package, Status: StatusValidated, Revision: 1, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ingest(context.Background(), ArchiveSource{Type: SourceLobeHub,
		Ref: "lobehub:demo-skill@1.2.3", Identifier: "demo-skill", ExpectedName: "demo-skill",
		Version: "1.2.3", Data: archiveTwo}); !errors.Is(err, ErrSourceDrift) {
		t.Fatalf("source drift error = %v", err)
	}

	runtimeArchive := mustTestArchive(t, []packageFile{
		{path: "SKILL.md", data: []byte(validSkillMarkdown("runtime-skill"))},
		{path: "neo.runtime.json", data: []byte(validRuntimeManifestJSON("runtime-skill"))},
	}, 0, time.Time{})
	service.lobehub = LobeHubSource{Fetcher: &fakeLobeHubFetcher{data: runtimeArchive}}
	service.newID = sequenceIDs("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	candidate, err := service.IngestLobeHub(context.Background(), testSkillAdmin, "runtime-skill", "1.2.3")
	if err != nil || candidate.AdmissionEligible {
		t.Fatalf("runtime candidate=%#v error=%v", candidate, err)
	}
	_, err = service.ReviewCandidate(context.Background(), testSkillAdmin, candidate.ID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 1, PackageFingerprint: candidate.Package.PackageFingerprint,
	})
	if !errors.Is(err, ErrAdmissionIneligible) {
		t.Fatalf("ineligible review error = %v", err)
	}
}

type memoryRepository struct {
	candidates    map[string]Candidate
	bySource      map[string]string
	installations map[string]Installation
	newID         func() string
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{candidates: map[string]Candidate{}, bySource: map[string]string{},
		installations: map[string]Installation{}, newID: sequenceIDs("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")}
}

func (repository *memoryRepository) CreateCandidate(_ context.Context, candidate Candidate) (Candidate, error) {
	key := candidate.SourceType + ":" + candidate.SourceRef
	if existingID := repository.bySource[key]; existingID != "" {
		existing := repository.candidates[existingID]
		if existing.SourceArtifactSHA256 != candidate.SourceArtifactSHA256 {
			return Candidate{}, ErrSourceDrift
		}
		return existing, nil
	}
	repository.candidates[candidate.ID] = candidate
	repository.bySource[key] = candidate.ID
	return candidate, nil
}

func (repository *memoryRepository) GetCandidate(_ context.Context, id string) (Candidate, error) {
	item, ok := repository.candidates[id]
	if !ok {
		return Candidate{}, ErrCandidateNotFound
	}
	return item, nil
}

func (repository *memoryRepository) GetCandidateBySource(_ context.Context, sourceType, sourceRef string) (Candidate, error) {
	id := repository.bySource[sourceType+":"+sourceRef]
	if id == "" {
		return Candidate{}, ErrCandidateNotFound
	}
	return repository.candidates[id], nil
}

func (repository *memoryRepository) ReviewCandidate(_ context.Context, id, reviewerID string, input ReviewInput) (Candidate, error) {
	item, ok := repository.candidates[id]
	if !ok {
		return Candidate{}, ErrCandidateNotFound
	}
	if item.Revision != input.ExpectedRevision || item.Package.PackageFingerprint != input.PackageFingerprint {
		return Candidate{}, ErrRevisionConflict
	}
	item.Status, item.ReviewedByUserID, item.ReviewReason = input.Status, reviewerID, input.Reason
	item.Revision++
	repository.candidates[id] = item
	return item, nil
}

func (repository *memoryRepository) ListStore(_ context.Context, page, pageSize int) (StoreResult, error) {
	items := []Candidate{}
	for _, item := range repository.candidates {
		if item.Status == StatusAdmitted {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return StoreResult{Items: items, Page: page, PageSize: pageSize,
		TotalCount: len(items), TotalPages: pageCount(len(items), pageSize)}, nil
}

func (repository *memoryRepository) GetStoreItem(_ context.Context, id string) (Candidate, error) {
	item, ok := repository.candidates[id]
	if !ok || item.Status != StatusAdmitted {
		return Candidate{}, ErrAdmissionDenied
	}
	return item, nil
}

func (repository *memoryRepository) Install(_ context.Context, userID, admissionID, fingerprint string) (Installation, error) {
	candidate, ok := repository.candidates[admissionID]
	if !ok || candidate.Status != StatusAdmitted || candidate.Package.PackageFingerprint != fingerprint {
		return Installation{}, ErrAdmissionDenied
	}
	for _, item := range repository.installations {
		if item.UserID == userID && item.Name == candidate.Package.Name {
			return Installation{}, ErrInstallationConflict
		}
	}
	now := time.Now().UTC()
	item := Installation{ID: repository.newID(), UserID: userID, AdmissionID: admissionID,
		PackageFingerprint: fingerprint, Name: candidate.Package.Name, Version: candidate.Package.Version,
		Description: candidate.Package.Description, AllowedTools: candidate.Package.AllowedTools,
		Revision: 1, CreatedAt: now, UpdatedAt: now}
	repository.installations[item.ID] = item
	return item, nil
}

func (repository *memoryRepository) ListLibrary(_ context.Context, userID string) ([]Installation, error) {
	items := []Installation{}
	for _, item := range repository.installations {
		if item.UserID == userID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *memoryRepository) Uninstall(_ context.Context, userID, id string, revision int64) error {
	item, ok := repository.installations[id]
	if !ok || item.UserID != userID {
		return ErrInstallationNotFound
	}
	if item.Revision != revision {
		return ErrRevisionConflict
	}
	delete(repository.installations, id)
	return nil
}

type memoryObjectStore struct{ objects map[string][]byte }

func newMemoryObjectStore() *memoryObjectStore {
	return &memoryObjectStore{objects: map[string][]byte{}}
}

func (store *memoryObjectStore) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	data, err := io.ReadAll(body)
	if err != nil || int64(len(data)) != size {
		return errors.New("invalid object write")
	}
	store.objects[key] = append([]byte(nil), data...)
	return nil
}

func (store *memoryObjectStore) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	data, ok := store.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), storage.ObjectInfo{Key: key, Size: int64(len(data))}, nil
}

func (store *memoryObjectStore) Delete(_ context.Context, key string) error {
	delete(store.objects, key)
	return nil
}

func sequenceIDs(ids ...string) func() string {
	index := 0
	return func() string {
		if index >= len(ids) {
			return ids[len(ids)-1]
		}
		id := ids[index]
		index++
		return id
	}
}
