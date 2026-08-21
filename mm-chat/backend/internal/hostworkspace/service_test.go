package hostworkspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

const (
	testWorkspaceID    = "0198ca9a-81c6-7c8d-9444-b16da02de9b4"
	testConversationID = "0198ca9a-81c6-70ae-b06d-12ec330e5aa6"
)

func validSettings() Settings {
	return Settings{
		Name:  "Neo Chat",
		Files: []WorkspaceFile{{ID: "file-1", FileName: "notes.md", MimeType: "text/markdown"}},
	}
}

func TestServiceImportsLegacyWorkspaceInPlace(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	workspace, err := service.ImportLegacy(context.Background(), testWorkspaceID, validSettings())
	if err != nil {
		t.Fatalf("ImportLegacy() error = %v", err)
	}
	if workspace.ID != testWorkspaceID || workspace.Settings.Name != "Neo Chat" ||
		workspace.Bound() || workspace.LegacyImportedAt == nil {
		t.Fatalf("unexpected workspace: %+v", workspace)
	}
}

func TestServiceValidatesLegacySettings(t *testing.T) {
	service := NewService(newFakeRepository(), nil)
	tests := []Settings{
		{},
		{Name: strings.Repeat("x", maxNameBytes+1), Files: []WorkspaceFile{}},
		{Name: "test", Files: []WorkspaceFile{{ID: "", FileName: "missing-id"}}},
		{Name: "test", Files: []WorkspaceFile{{ID: "file", FileName: "test", SHA256: "invalid"}}},
	}
	for _, settings := range tests {
		if _, err := service.ImportLegacy(context.Background(), testWorkspaceID, settings); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ImportLegacy(%+v) error = %v, want %v", settings, err, ErrInvalid)
		}
	}
}

func TestServiceBindsOnlyRunnerValidatedDescriptor(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{ID: testWorkspaceID, Revision: 1, Settings: validSettings()}
	resolver := fakeResolver{
		runnerID: "wsl-test-runner",
		descriptor: agenthost.WorkspaceDescriptor{
			CanonicalPath:        "/home/user/project",
			DisplayPath:          `D:\project`,
			PathKind:             "windows-mounted",
			DirectoryFingerprint: "sha256:" + strings.Repeat("a", 64),
		},
	}
	service := NewService(repo, resolver)
	workspace, err := service.Bind(context.Background(), testWorkspaceID, 1, `D:\project`)
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if workspace.RunnerID != "wsl-test-runner" || repo.lastRunner != "wsl-test-runner" ||
		repo.lastPath.CanonicalPath != "/home/user/project" {
		t.Fatalf("unexpected binding: %+v", workspace)
	}
}

func TestServiceRejectsUntrustedResolverDescriptor(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{ID: testWorkspaceID, Revision: 1, Settings: validSettings()}
	resolver := fakeResolver{
		runnerID: "INVALID",
		descriptor: agenthost.WorkspaceDescriptor{
			CanonicalPath:        "/home/private\nleak",
			DisplayPath:          "/home/private",
			PathKind:             "wsl",
			DirectoryFingerprint: "sha256:" + strings.Repeat("z", 64),
		},
	}
	service := NewService(repo, resolver)
	if _, err := service.Bind(context.Background(), testWorkspaceID, 1, "/home/private"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Bind() error = %v, want %v", err, ErrInvalid)
	}
}

func TestServiceAuthorizesWorkspaceBeforeResolvingHostPath(t *testing.T) {
	repo := newFakeRepository()
	calls := 0
	service := NewService(repo, fakeResolver{calls: &calls})
	if _, err := service.Bind(
		context.Background(), testWorkspaceID, 1, "/home/private/project",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bind() error = %v, want %v", err, ErrNotFound)
	}
	if calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", calls)
	}
}

func TestServiceLocksConversationWorkspaceThroughRepository(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	binding, err := service.LockConversationExecutionWorkspace(
		context.Background(), testConversationID, testWorkspaceID,
	)
	if err != nil {
		t.Fatalf("LockConversationExecutionWorkspace() error = %v", err)
	}
	if binding.ConversationID != testConversationID || binding.WorkspaceID != testWorkspaceID {
		t.Fatalf("unexpected binding: %+v", binding)
	}
}
