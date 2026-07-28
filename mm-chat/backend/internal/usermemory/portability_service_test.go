package usermemory

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type portabilityTestRepository struct {
	*fakeRepository
	state        string
	resolution   ImportMemoryResolution
	applyCalls   int
	projects     []PortabilityApplyProject
	memories     []PortabilityApplyMemory
	revisions    []PortabilityApplyRevision
	completed    *PortabilityApplyResult
	metadata     PortabilityApplyMetadata
	resolveCalls int
	resolutions  map[string]ImportMemoryResolution
}

func (r *portabilityTestRepository) PortabilityAuthorityState(context.Context) (string, error) {
	return r.state, nil
}

func (r *portabilityTestRepository) ResolveImportProject(
	_ context.Context,
	projectID string,
) (MemoryProject, error) {
	return MemoryProject{ID: projectID, ScopeGeneration: 2}, nil
}

func (r *portabilityTestRepository) ResolveImportConversation(
	_ context.Context,
	conversationID string,
) (ConversationMemoryPolicy, error) {
	return ConversationMemoryPolicy{
		ConversationID: conversationID, ScopeGeneration: 3,
	}, nil
}

func (r *portabilityTestRepository) ResolveImportMemory(
	_ context.Context,
	input ImportMemoryResolutionInput,
) (ImportMemoryResolution, error) {
	r.resolveCalls++
	if resolution, ok := r.resolutions[input.NormalizedContent]; ok {
		return resolution, nil
	}
	return r.resolution, nil
}

func (r *portabilityTestRepository) CompletedPortabilityImport(
	_ context.Context,
	metadata PortabilityApplyMetadata,
) (PortabilityApplyResult, bool, error) {
	if r.completed == nil {
		return PortabilityApplyResult{}, false, nil
	}
	if metadata.ImportID != r.metadata.ImportID ||
		metadata.PackageSHA256 != r.metadata.PackageSHA256 ||
		metadata.ManifestSHA256 != r.metadata.ManifestSHA256 ||
		metadata.MappingsSHA256 != r.metadata.MappingsSHA256 ||
		metadata.PlanSHA256 != r.metadata.PlanSHA256 ||
		metadata.AuthorityStateHash != r.metadata.AuthorityStateHash {
		return PortabilityApplyResult{}, false, importPlanStaleError()
	}
	return *r.completed, true, nil
}

func (r *portabilityTestRepository) ApplyPortabilityImport(
	_ context.Context,
	metadata PortabilityApplyMetadata,
	apply func(PortabilityApplySink) error,
) (PortabilityApplyResult, error) {
	r.applyCalls++
	if metadata.AuthorityStateHash != r.state {
		return PortabilityApplyResult{}, importPlanStaleError()
	}
	if err := apply(portabilityTestSink{repository: r}); err != nil {
		return PortabilityApplyResult{}, err
	}
	result := PortabilityApplyResult{
		ImportID: metadata.ImportID, Status: "completed",
		AddedProjects: len(r.projects), AddedMemories: len(r.memories),
		ImportedAt: time.Now().UTC().UnixMilli(),
	}
	r.metadata = metadata
	r.completed = &result
	return result, nil
}

type portabilityTestSink struct {
	repository *portabilityTestRepository
}

func (s portabilityTestSink) CreateProject(input PortabilityApplyProject) error {
	s.repository.projects = append(s.repository.projects, input)
	return nil
}

func (s portabilityTestSink) AddMemory(input PortabilityApplyMemory) error {
	s.repository.memories = append(s.repository.memories, input)
	return nil
}

func (s portabilityTestSink) AddRevision(input PortabilityApplyRevision) error {
	s.repository.revisions = append(s.repository.revisions, input)
	return nil
}

func (s portabilityTestSink) FinalizeMemory(PortabilityApplyFinalState) error {
	return nil
}

func TestMemoryImportDryRunConfirmAndStateDrift(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	repo := &portabilityTestRepository{
		fakeRepository: &fakeRepository{},
		state:          strings.Repeat("a", 64),
		resolution: ImportMemoryResolution{
			Result: "ADD", ReasonCode: "NEW_MEMORY",
		},
	}
	service := NewService(repo, WithPortabilityPlanCodec(codec))
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "00000000-0000-4000-8000-000000000001",
	})
	encrypted := encryptedTestMemoryPackage(t, validPortableMemory(1), false)
	mappings := ImportMappings{Projects: map[string]ImportProjectMapping{
		"project-000001": {Mode: "skip"},
	}}

	dryRun, err := service.DryRunMemoryImport(
		ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings,
	)
	if err != nil {
		t.Fatalf("DryRunMemoryImport() error = %v", err)
	}
	if dryRun.Counts["ADD"] != 1 || len(dryRun.ScopeRequirements) != 0 ||
		dryRun.SettingsSuggestion == nil || dryRun.PlanToken == "" {
		t.Fatalf("dry-run = %#v", dryRun)
	}
	confirmed, err := service.ConfirmMemoryImport(
		ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings, dryRun.PlanToken,
	)
	if err != nil {
		t.Fatalf("ConfirmMemoryImport() error = %v", err)
	}
	if confirmed.AddedMemories != 1 || len(repo.memories) != 1 ||
		repo.memories[0].Record.OriginalAuthority != "manual" {
		t.Fatalf("confirmed=%#v memories=%#v", confirmed, repo.memories)
	}
	replayed, err := service.ConfirmMemoryImport(
		ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings, dryRun.PlanToken,
	)
	if err != nil || replayed.ImportID != confirmed.ImportID || repo.applyCalls != 1 ||
		len(repo.memories) != 1 {
		t.Fatalf("replayed confirm=%#v/%v apply_calls=%d memories=%d",
			replayed, err, repo.applyCalls, len(repo.memories))
	}
	if _, err := service.ConfirmMemoryImport(
		ctx, bytes.NewReader(encrypted), "incorrect-passphrase", mappings, dryRun.PlanToken,
	); errorCode(err) != "MEMORY_PORTABILITY_DECRYPT_FAILED" {
		t.Fatalf("replayed confirm with wrong passphrase error = %v", err)
	}

	driftPlan, err := service.DryRunMemoryImport(
		ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings,
	)
	if err != nil {
		t.Fatalf("second DryRunMemoryImport() error = %v", err)
	}
	repo.state = strings.Repeat("b", 64)
	if _, err := service.ConfirmMemoryImport(
		ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings, driftPlan.PlanToken,
	); errorCode(err) != "MEMORY_IMPORT_PLAN_STALE" {
		t.Fatalf("drift ConfirmMemoryImport() error = %v", err)
	}
}

func TestMemoryImportMapsHistorySupersessionTargets(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "00000000-0000-4000-8000-000000000001",
	})
	target := validPortableMemory(1)
	target.Content = "Use short answers"
	target.ContentHash = contentSHA256(target.Content)
	source := validPortableMemory(2)
	source.Ref = "memory-000002"
	source.Content = "Keep responses direct"
	source.ContentHash = contentSHA256(source.Content)
	priorContent := "Prefer concise answers"
	prior := PortableMemorySnapshot{
		Type: "preference", Content: priorContent,
		ContentHash: contentSHA256(priorContent), Importance: 4,
		Tags: []string{"style"}, Enabled: true,
		Scope:           PortableMemoryScope{Type: "global"},
		LifecycleStatus: "superseded", SupersededByRef: target.Ref,
		Sensitivity: "normal", Confidence: 1,
		ObservedAt: "2026-07-28T00:00:00Z",
	}
	revision := PortableRevisionRecord{
		Kind: "revision", MemoryRef: source.Ref, Revision: 2,
		Operation: "restore", OldContentHash: prior.ContentHash,
		NewContentHash: source.ContentHash, Prior: &prior,
		CreatedAt: "2026-07-28T01:00:00Z",
	}
	encrypted := encryptedTestMemoryRecords(t, []any{
		target, source, revision,
	}, true)

	for _, testCase := range []struct {
		name                 string
		targetResolution     ImportMemoryResolution
		expectedAdded        int
		expectedSupersededID func(string) string
	}{
		{
			name: "added target uses its fresh local id",
			targetResolution: ImportMemoryResolution{
				Result: "ADD", ReasonCode: "NEW_MEMORY",
			},
			expectedAdded: 2,
			expectedSupersededID: func(importID string) string {
				id, idErr := deterministicImportUUID(importID, target.Ref, "memory")
				if idErr != nil {
					t.Fatalf("deterministicImportUUID() error = %v", idErr)
				}
				return id
			},
		},
		{
			name: "noop target uses its current local id",
			targetResolution: ImportMemoryResolution{
				Result: "NOOP", ReasonCode: "EXACT_DUPLICATE",
				CurrentMemoryID:    "00000000-0000-4000-8000-000000000099",
				CurrentRevision:    3,
				CurrentContentHash: target.ContentHash,
			},
			expectedAdded: 1,
			expectedSupersededID: func(string) string {
				return "00000000-0000-4000-8000-000000000099"
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &portabilityTestRepository{
				fakeRepository: &fakeRepository{}, state: strings.Repeat("e", 64),
				resolution: ImportMemoryResolution{
					Result: "ADD", ReasonCode: "NEW_MEMORY",
				},
				resolutions: map[string]ImportMemoryResolution{
					normalizeSearchText(target.Content): testCase.targetResolution,
				},
			}
			service := NewService(repo, WithPortabilityPlanCodec(codec))
			dryRun, dryRunErr := service.DryRunMemoryImport(
				ctx, bytes.NewReader(encrypted), "fixture-passphrase", ImportMappings{},
			)
			if dryRunErr != nil {
				t.Fatalf("DryRunMemoryImport() error = %v", dryRunErr)
			}
			if dryRun.Counts["ADD"] != testCase.expectedAdded ||
				dryRun.Counts["REJECT"] != 0 {
				t.Fatalf("dry-run = %#v", dryRun)
			}
			confirmed, confirmErr := service.ConfirmMemoryImport(
				ctx, bytes.NewReader(encrypted), "fixture-passphrase",
				ImportMappings{}, dryRun.PlanToken,
			)
			if confirmErr != nil {
				t.Fatalf("ConfirmMemoryImport() error = %v", confirmErr)
			}
			if confirmed.AddedMemories != testCase.expectedAdded || len(repo.revisions) != 1 {
				t.Fatalf("confirmed=%#v revisions=%#v", confirmed, repo.revisions)
			}
			expectedID := testCase.expectedSupersededID(dryRun.ImportID)
			if repo.revisions[0].SupersededByMemoryID != expectedID {
				t.Fatalf(
					"revision superseded target = %q, want %q",
					repo.revisions[0].SupersededByMemoryID, expectedID,
				)
			}
		})
	}
}

func TestMemoryImportHistoryRequiresAvailablePriorScope(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "00000000-0000-4000-8000-000000000001",
	})
	memory := validPortableMemory(2)
	memory.Content = "Keep answers concise"
	memory.ContentHash = contentSHA256(memory.Content)
	priorContent := "Prefer concise answers"
	prior := PortableMemorySnapshot{
		Type: "preference", Content: priorContent,
		ContentHash: contentSHA256(priorContent), Importance: 4,
		Tags: []string{"style"}, Enabled: true,
		Scope:           PortableMemoryScope{Type: "project", ProjectRef: "project-000001"},
		LifecycleStatus: "active", Sensitivity: "normal", Confidence: 1,
		ObservedAt: "2026-07-28T00:00:00Z",
	}
	revision := PortableRevisionRecord{
		Kind: "revision", MemoryRef: memory.Ref, Revision: 2,
		Operation: "update", OldContentHash: prior.ContentHash,
		NewContentHash: memory.ContentHash, Prior: &prior,
		CreatedAt: "2026-07-28T01:00:00Z",
	}
	encrypted := encryptedTestMemoryRecords(t, []any{
		PortableProjectRecord{
			Kind: "project", Ref: "project-000001", Name: "Neo Chat",
			Description: "Memory portability", LifecycleStatus: "active",
		},
		memory,
		revision,
	}, true)

	for _, testCase := range []struct {
		name       string
		mappings   ImportMappings
		wantResult string
		wantReason string
	}{
		{
			name:       "missing mapping",
			mappings:   ImportMappings{},
			wantResult: "SCOPE_REQUIRED",
			wantReason: "HISTORY_SCOPE_MAPPING_REQUIRED",
		},
		{
			name: "skipped mapping",
			mappings: ImportMappings{Projects: map[string]ImportProjectMapping{
				"project-000001": {Mode: "skip"},
			}},
			wantResult: "REJECT",
			wantReason: "HISTORY_SCOPE_UNAVAILABLE",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &portabilityTestRepository{
				fakeRepository: &fakeRepository{}, state: strings.Repeat("f", 64),
				resolution: ImportMemoryResolution{Result: "ADD", ReasonCode: "NEW_MEMORY"},
			}
			service := NewService(repository, WithPortabilityPlanCodec(codec))
			dryRun, dryRunErr := service.DryRunMemoryImport(
				ctx, bytes.NewReader(encrypted), "fixture-passphrase", testCase.mappings,
			)
			if dryRunErr != nil {
				t.Fatalf("DryRunMemoryImport() error = %v", dryRunErr)
			}
			if len(dryRun.Items) != 1 || dryRun.Items[0].Result != testCase.wantResult ||
				dryRun.Items[0].ReasonCode != testCase.wantReason ||
				dryRun.Counts[testCase.wantResult] != 1 {
				t.Fatalf("dry-run = %#v", dryRun)
			}
		})
	}
}

func TestMemoryImportSecretRejectsBeforeRepositoryResolution(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	codec, _ := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	repo := &portabilityTestRepository{
		fakeRepository: &fakeRepository{}, state: strings.Repeat("c", 64),
		resolution: ImportMemoryResolution{Result: "ADD", ReasonCode: "NEW_MEMORY"},
	}
	service := NewService(repo, WithPortabilityPlanCodec(codec))
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "00000000-0000-4000-8000-000000000001",
	})
	record := validPortableMemory(1)
	record.Content = "api_key=fixture-secret-value"
	record.ContentHash = contentSHA256(record.Content)
	encrypted := encryptedTestMemoryPackage(t, record, false)
	dryRun, err := service.DryRunMemoryImport(
		ctx,
		bytes.NewReader(encrypted),
		"fixture-passphrase",
		ImportMappings{Projects: map[string]ImportProjectMapping{
			"project-000001": {Mode: "skip"},
		}},
	)
	if err != nil {
		t.Fatalf("DryRunMemoryImport() error = %v", err)
	}
	if dryRun.Counts["REJECT"] != 1 || dryRun.Items[0].ReasonCode != "SECRET_REJECTED" {
		t.Fatalf("dry-run = %#v", dryRun)
	}
	if repo.resolveCalls != 0 {
		t.Fatalf("secret candidate reached repository resolution %d time(s)", repo.resolveCalls)
	}
}

func TestMemoryImportPreflightRejectsSecretsAcrossPortablePlaintext(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	codec, _ := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	newService := func() (*Service, *portabilityTestRepository) {
		repository := &portabilityTestRepository{
			fakeRepository: &fakeRepository{}, state: strings.Repeat("d", 64),
			resolution: ImportMemoryResolution{Result: "ADD", ReasonCode: "NEW_MEMORY"},
		}
		return NewService(repository, WithPortabilityPlanCodec(codec)), repository
	}
	ctx := auth.WithUser(context.Background(), auth.User{
		ID: "00000000-0000-4000-8000-000000000001",
	})
	mappings := ImportMappings{Projects: map[string]ImportProjectMapping{
		"project-000001": {Mode: "skip"},
	}}

	t.Run("secret tag rejects the memory before resolution", func(t *testing.T) {
		service, repository := newService()
		record := validPortableMemory(1)
		record.Tags = []string{"api_key=fixture-secret-value"}
		dryRun, err := service.DryRunMemoryImport(
			ctx, bytes.NewReader(encryptedTestMemoryPackage(t, record, false)),
			"fixture-passphrase", mappings,
		)
		if err != nil || dryRun.Counts["REJECT"] != 1 ||
			dryRun.Items[0].ReasonCode != "SECRET_REJECTED" || repository.resolveCalls != 0 {
			t.Fatalf("dry-run=%#v error=%v resolve_calls=%d", dryRun, err, repository.resolveCalls)
		}
	})

	t.Run("oversize fact key rejects the memory before resolution", func(t *testing.T) {
		service, repository := newService()
		record := validPortableMemory(1)
		record.FactKey = strings.Repeat("f", 257)
		dryRun, err := service.DryRunMemoryImport(
			ctx, bytes.NewReader(encryptedTestMemoryPackage(t, record, false)),
			"fixture-passphrase", mappings,
		)
		if err != nil || dryRun.Counts["REJECT"] != 1 ||
			dryRun.Items[0].ReasonCode != "INVALID_MEMORY" || repository.resolveCalls != 0 {
			t.Fatalf("dry-run=%#v error=%v resolve_calls=%d", dryRun, err, repository.resolveCalls)
		}
	})

	t.Run("secret revision rejects its canonical memory before resolution", func(t *testing.T) {
		service, repository := newService()
		record := validPortableMemory(2)
		record.Content = "Keep answers concise"
		record.ContentHash = contentSHA256(record.Content)
		prior := PortableMemorySnapshot{
			Type: "preference", Content: "api_key=fixture-history-secret",
			ContentHash: contentSHA256("api_key=fixture-history-secret"),
			Importance:  4, Tags: []string{"history"}, Enabled: true,
			Scope: PortableMemoryScope{Type: "global"}, LifecycleStatus: "active",
			Sensitivity: "normal", Confidence: 1, ObservedAt: "2026-07-28T00:00:00Z",
		}
		revision := PortableRevisionRecord{
			Kind: "revision", MemoryRef: record.Ref, Revision: 2,
			Operation: "update", OldContentHash: prior.ContentHash,
			NewContentHash: record.ContentHash, Prior: &prior,
			CreatedAt: "2026-07-28T01:00:00Z",
		}
		encrypted := encryptedTestMemoryRecords(t, []any{
			PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
			PortableProjectRecord{
				Kind: "project", Ref: "project-000001", Name: "Neo Chat",
				Description: "Memory portability", LifecycleStatus: "active",
			},
			record,
			revision,
		}, true)
		dryRun, err := service.DryRunMemoryImport(
			ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings,
		)
		if err != nil || dryRun.Counts["REJECT"] != 1 ||
			dryRun.Items[0].ReasonCode != "SECRET_REJECTED" || repository.resolveCalls != 0 {
			t.Fatalf("dry-run=%#v error=%v resolve_calls=%d", dryRun, err, repository.resolveCalls)
		}
	})

	t.Run("secret Project metadata rejects the package", func(t *testing.T) {
		service, repository := newService()
		encrypted := encryptedTestMemoryRecords(t, []any{
			PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
			PortableProjectRecord{
				Kind: "project", Ref: "project-000001", Name: "Neo Chat",
				Description: "password=fixture-project-secret", LifecycleStatus: "active",
			},
			validPortableMemory(1),
		}, false)
		_, err := service.DryRunMemoryImport(
			ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings,
		)
		if errorCode(err) != "MEMORY_PACKAGE_SECRET_REJECTED" || repository.resolveCalls != 0 {
			t.Fatalf("error=%v resolve_calls=%d", err, repository.resolveCalls)
		}
	})

	t.Run("secret Project metadata does not mask ciphertext tamper", func(t *testing.T) {
		service, repository := newService()
		encrypted := encryptedTestMemoryRecords(t, []any{
			PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
			PortableProjectRecord{
				Kind: "project", Ref: "project-000001", Name: "Neo Chat",
				Description: "password=fixture-project-secret", LifecycleStatus: "active",
			},
			validPortableMemory(1),
		}, false)
		encrypted[len(encrypted)-1] ^= 1
		_, err := service.DryRunMemoryImport(
			ctx, bytes.NewReader(encrypted), "fixture-passphrase", mappings,
		)
		if errorCode(err) != "MEMORY_PACKAGE_AUTHENTICATION_FAILED" || repository.resolveCalls != 0 {
			t.Fatalf("error=%v resolve_calls=%d", err, repository.resolveCalls)
		}
	})
}

func encryptedTestMemoryPackage(
	t *testing.T,
	record PortableMemoryRecord,
	history bool,
) []byte {
	t.Helper()
	records := []any{
		PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
		PortableProjectRecord{
			Kind: "project", Ref: "project-000001", Name: "Neo Chat",
			Description: "Memory portability", LifecycleStatus: "active",
		},
		record,
	}
	return encryptedTestMemoryRecords(t, records, history)
}

func encryptedTestMemoryRecords(t *testing.T, records []any, history bool) []byte {
	t.Helper()
	counts := PortableRecordCounts{}
	for _, record := range records {
		switch record.(type) {
		case PortableSettingsRecord:
			counts.Settings++
		case PortableProjectRecord:
			counts.Projects++
		case PortableMemoryRecord:
			counts.Memories++
		case PortableRevisionRecord:
			counts.Revisions++
		}
	}
	manifest := MemoryPackageManifest{
		CreatedAt: "2026-07-28T02:00:00Z", ExporterRelease: "test",
		IncludeHistory: history,
		Counts:         counts,
	}
	var plaintext bytes.Buffer
	if err := WriteMemoryPackage(&plaintext, manifest, records); err != nil {
		t.Fatalf("WriteMemoryPackage() error = %v", err)
	}
	var encrypted bytes.Buffer
	if err := EncryptPortabilityStream(
		&encrypted,
		"fixture-passphrase",
		func(writer io.Writer) error {
			_, err := writer.Write(plaintext.Bytes())
			return err
		},
	); err != nil {
		t.Fatalf("EncryptPortabilityStream() error = %v", err)
	}
	return encrypted.Bytes()
}

var _ PortabilityRepository = (*portabilityTestRepository)(nil)
