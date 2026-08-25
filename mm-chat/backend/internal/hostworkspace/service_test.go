package hostworkspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/localskills"
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

type permissionCapabilityResolver struct {
	fakeResolver
	modes []agenthost.PermissionMode
}

func (resolver permissionCapabilityResolver) Capabilities(context.Context) (agenthost.Capabilities, error) {
	return agenthost.Capabilities{
		RunnerID: "wsl-test-runner",
		Features: agenthost.HostFeatures{
			Execution: true, PermissionModes: resolver.modes,
		},
	}, nil
}

func TestServiceEnforcesPermissionCapabilitiesAndFullAccessAcknowledgement(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, permissionCapabilityResolver{
		modes: []agenthost.PermissionMode{
			agenthost.PermissionReadOnly,
			agenthost.PermissionWorkspaceWrite,
			agenthost.PermissionFullAccess,
		},
	})
	if err := service.SetConversationPermission(
		context.Background(), testConversationID,
		agenthost.PermissionFullAccess, false,
	); !errors.Is(err, ErrPermissionAcknowledgement) {
		t.Fatalf("unacknowledged Full access error = %v", err)
	}
	if err := service.SetConversationPermission(
		context.Background(), testConversationID,
		agenthost.PermissionFullAccess, true,
	); err != nil || repo.permissionMode != agenthost.PermissionFullAccess {
		t.Fatalf("acknowledged Full access = %v, stored %q", err, repo.permissionMode)
	}

	service = NewService(repo, permissionCapabilityResolver{
		modes: []agenthost.PermissionMode{agenthost.PermissionReadOnly},
	})
	if err := service.SetConversationPermission(
		context.Background(), testConversationID,
		agenthost.PermissionWorkspaceWrite, false,
	); !errors.Is(err, ErrPermissionUnavailable) {
		t.Fatalf("unadvertised mode error = %v", err)
	}
}

type workspaceFileResolver struct {
	fakeResolver
	snapshot localskills.WorkspaceArtifactSnapshot
	err      error
	requests []agenthost.ToolExecuteRequest
}

func (resolver *workspaceFileResolver) ExecuteTool(
	_ context.Context,
	request agenthost.ToolExecuteRequest,
	output any,
) error {
	resolver.requests = append(resolver.requests, request)
	if resolver.err != nil {
		return resolver.err
	}
	target, ok := output.(*localskills.WorkspaceArtifactSnapshot)
	if !ok {
		return agenthost.ErrHostProtocol
	}
	*target = resolver.snapshot
	return nil
}

func TestServiceReadsBoundWorkspaceFileThroughPinnedReadOnlyHostAuthority(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{
		ID: testWorkspaceID, Revision: 2, Settings: validSettings(),
		RunnerID: "wsl-test-runner", CanonicalPath: "/home/user/project",
		DirectoryFingerprint: "sha256:" + strings.Repeat("a", 64),
	}
	resolver := &workspaceFileResolver{
		fakeResolver: fakeResolver{runnerID: "wsl-test-runner"},
		snapshot: localskills.WorkspaceArtifactSnapshot{
			Path: "reports/result.xlsx", Body: []byte("xlsx"),
			Version: "sha256:" + strings.Repeat("b", 64),
		},
	}
	snapshot, err := NewService(repo, resolver).ReadWorkspaceFile(
		context.Background(), testWorkspaceID, "reports/result.xlsx", 1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "reports/result.xlsx" || snapshot.FileName != "result.xlsx" ||
		snapshot.MimeType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		string(snapshot.Body) != "xlsx" || len(resolver.requests) != 1 {
		t.Fatalf("snapshot=%#v requests=%#v", snapshot, resolver.requests)
	}
	request := resolver.requests[0]
	if request.Tool != agenthost.ToolArtifactRead ||
		request.PermissionMode != agenthost.PermissionReadOnly ||
		request.Workspace.CanonicalPath != "/home/user/project" ||
		request.Workspace.DirectoryFingerprint != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("Host request=%#v", request)
	}
}

func TestServiceWorkspaceFileReadFailsBeforeHostWithoutOwnedBoundWorkspace(t *testing.T) {
	resolver := &workspaceFileResolver{fakeResolver: fakeResolver{runnerID: "wsl-test-runner"}}
	service := NewService(newFakeRepository(), resolver)
	if _, err := service.ReadWorkspaceFile(
		context.Background(), testWorkspaceID, "secret.txt", 1024,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Workspace error=%v", err)
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("unauthorized read reached Host: %#v", resolver.requests)
	}
}

func TestServiceWorkspaceFileReadRejectsTraversalBeforeHost(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{
		ID: testWorkspaceID, Settings: validSettings(), RunnerID: "wsl-test-runner",
		CanonicalPath:        "/home/user/project",
		DirectoryFingerprint: "sha256:" + strings.Repeat("a", 64),
	}
	resolver := &workspaceFileResolver{fakeResolver: fakeResolver{runnerID: "wsl-test-runner"}}
	service := NewService(repo, resolver)
	for _, filePath := range []string{"../secret.txt", "/etc/passwd", `reports\\secret.txt`, "a/../b.txt"} {
		if _, err := service.ReadWorkspaceFile(
			context.Background(), testWorkspaceID, filePath, 1024,
		); !errors.Is(err, ErrInvalid) {
			t.Fatalf("path=%q error=%v", filePath, err)
		}
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("traversal reached Host: %#v", resolver.requests)
	}
}
