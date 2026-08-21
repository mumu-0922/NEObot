package hostworkspace

import (
	"context"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

type fakeRepository struct {
	items       map[string]Workspace
	bindings    map[string]ExecutionBinding
	setCalls    int
	lastPath    agenthost.WorkspaceDescriptor
	lastRunner  string
	returnError error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{items: map[string]Workspace{}, bindings: map[string]ExecutionBinding{}}
}

func (repo *fakeRepository) List(context.Context) ([]Workspace, error) {
	if repo.returnError != nil {
		return nil, repo.returnError
	}
	items := make([]Workspace, 0, len(repo.items))
	for _, item := range repo.items {
		items = append(items, item)
	}
	return items, nil
}

func (repo *fakeRepository) Get(_ context.Context, id string) (Workspace, error) {
	item, ok := repo.items[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	return item, nil
}

func (repo *fakeRepository) ImportLegacy(
	_ context.Context,
	id string,
	settings Settings,
	now time.Time,
) (Workspace, error) {
	if repo.returnError != nil {
		return Workspace{}, repo.returnError
	}
	item := repo.items[id]
	if item.ID == "" {
		item.ID, item.Revision, item.CreatedAt = id, 1, now
	} else {
		item.Revision++
	}
	item.Settings, item.UpdatedAt, item.LegacyImportedAt = settings, now, &now
	repo.items[id] = item
	return item, nil
}

func (repo *fakeRepository) UpdateSettings(
	_ context.Context,
	id string,
	revision int64,
	settings Settings,
) (Workspace, error) {
	item, ok := repo.items[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	if item.Revision != revision {
		return Workspace{}, ErrRevisionConflict
	}
	item.Settings, item.Revision = settings, item.Revision+1
	repo.items[id] = item
	return item, nil
}

func (repo *fakeRepository) Bind(
	_ context.Context,
	id string,
	revision int64,
	descriptor agenthost.WorkspaceDescriptor,
	runnerID string,
	now time.Time,
) (Workspace, error) {
	if repo.returnError != nil {
		return Workspace{}, repo.returnError
	}
	item, ok := repo.items[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	if item.Revision != revision {
		return Workspace{}, ErrRevisionConflict
	}
	repo.lastPath, repo.lastRunner = descriptor, runnerID
	item.RunnerID = runnerID
	item.CanonicalPath = descriptor.CanonicalPath
	item.DisplayPath = descriptor.DisplayPath
	item.PathKind = descriptor.PathKind
	item.DirectoryFingerprint = descriptor.DirectoryFingerprint
	item.BoundAt = &now
	item.Revision++
	repo.items[id] = item
	return item, nil
}

func (repo *fakeRepository) Delete(_ context.Context, id string, revision int64, _ time.Time) error {
	item, ok := repo.items[id]
	if !ok {
		return ErrNotFound
	}
	if item.Revision != revision {
		return ErrRevisionConflict
	}
	delete(repo.items, id)
	return nil
}

func (repo *fakeRepository) SetConversationWorkspace(context.Context, string, string) error {
	repo.setCalls++
	return repo.returnError
}

func (repo *fakeRepository) LockConversationExecutionWorkspace(
	_ context.Context,
	conversationID string,
	workspaceID string,
	now time.Time,
) (ExecutionBinding, error) {
	if repo.returnError != nil {
		return ExecutionBinding{}, repo.returnError
	}
	binding := ExecutionBinding{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		BoundAt:        now,
	}
	repo.bindings[conversationID] = binding
	return binding, nil
}

type fakeResolver struct {
	runnerID   string
	descriptor agenthost.WorkspaceDescriptor
	err        error
	calls      *int
}

func (resolver fakeResolver) ResolveWorkspace(context.Context, string) (agenthost.WorkspaceDescriptor, error) {
	if resolver.calls != nil {
		(*resolver.calls)++
	}
	return resolver.descriptor, resolver.err
}

func (resolver fakeResolver) RunnerID() string { return resolver.runnerID }
