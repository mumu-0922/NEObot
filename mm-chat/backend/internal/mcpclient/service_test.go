package mcpclient

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/providersecrets"
)

func TestPrepareRunUsesExplicitEmptyAndWorkspaceInheritance(t *testing.T) {
	t.Parallel()
	userID, conversationID, workspaceID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{
		ConversationID: conversationID, UserID: userID, WorkspaceID: workspaceID,
	}
	server := testManifestServer("weather", "global")
	repo.workspaceSelections[workspaceID] = WorkspaceSelection{
		WorkspaceID: workspaceID, Revision: 4,
		Servers: []SelectionServer{{Ref: server.Ref}},
	}
	service, err := NewService(testMCPConfig(), repo, &fakeConnector{}, nil, nil, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareRun(context.Background(), userID, conversationID, "", runID)
	if err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	if !prepared.Enabled() || prepared.Snapshot.SelectionRevision != 4 || len(repo.snapshots) != 1 {
		t.Fatalf("prepared = %#v, snapshots=%d", prepared.Snapshot, len(repo.snapshots))
	}

	repo.selections[userID+":"+conversationID] = Selection{
		ConversationID: conversationID, Mode: SelectionModeCustom, Revision: 2,
		Servers: []SelectionServer{},
	}
	prepared, err = service.PrepareRun(context.Background(), userID, conversationID, "", uuid.NewString())
	if err != nil {
		t.Fatalf("explicit empty PrepareRun() error = %v", err)
	}
	if prepared.Enabled() || len(repo.snapshots) != 1 {
		t.Fatalf("explicit empty prepared = %#v, snapshots=%d", prepared, len(repo.snapshots))
	}
}

func TestAdministratorDefinitionIsSharedWithoutCopyingCredential(t *testing.T) {
	t.Parallel()
	administratorID := uuid.NewString()
	ordinaryUserID := uuid.NewString()
	conversationID := uuid.NewString()
	serverRef := ServerRef{Source: SourcePrivate, ID: uuid.NewString()}
	tool := normalizeTool(
		serverRef, "lookup", "Lookup", "", map[string]any{"type": "object"}, ClassificationRead,
	)
	server := Server{
		Ref: serverRef, Name: "Shared search", Transport: TransportStreamableHTTP,
		EndpointURL: "https://1.1.1.1/mcp", AuthType: AuthHeader,
		HeaderAuth: &HeaderAuth{Name: "Authorization", Prefix: "Bearer "},
		Status:     ServerStatusReady, OwnerUserID: administratorID, Tools: []Tool{tool},
	}
	repo := newFakeRepository()
	repo.private[administratorID+":"+serverRef.ID] = server
	repo.scopes[ordinaryUserID+":"+conversationID] = ConversationScope{
		ConversationID: conversationID, UserID: ordinaryUserID,
	}
	connector := &fakeConnector{sessions: []Session{fakeSession{
		result: CallResult{Content: []Content{{Type: "text", Text: "shared-ok"}}},
	}}}
	service, err := NewService(
		testMCPAdminConfig(administratorID), repo, connector, testVault(t),
		&memoryObjectStore{}, Catalog{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetHeaderCredential(
		context.Background(), administratorID, "", serverRef, "owner-secret",
	); err != nil {
		t.Fatalf("SetHeaderCredential() error = %v", err)
	}

	servers, err := service.ListServers(context.Background(), ordinaryUserID, conversationID)
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Ref != serverRef || !servers[0].HasCredential ||
		servers[0].CanManage {
		t.Fatalf("ordinary shared servers = %#v", servers)
	}
	selection, err := service.ReplaceSelection(context.Background(), ordinaryUserID, Selection{
		ConversationID: conversationID, Mode: SelectionModeCustom,
		Servers: []SelectionServer{{Ref: serverRef}},
	})
	if err != nil || len(selection.Servers) != 1 || selection.Servers[0].Ref != serverRef {
		t.Fatalf("ReplaceSelection() selection=%#v error=%v", selection, err)
	}
	prepared, err := service.PrepareRun(
		context.Background(), ordinaryUserID, conversationID, "", uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	result, err := service.Execute(context.Background(), ordinaryUserID, prepared, ExecuteInput{
		Alias: tool.Alias, Arguments: map[string]any{}, Round: 1, Call: 1,
	}, nil)
	if err != nil || result.ModelContent == "" {
		t.Fatalf("Execute() result=%#v error=%v", result, err)
	}
	connector.mu.Lock()
	defer connector.mu.Unlock()
	if len(connector.credentials) != 1 || connector.credentials[0] != "owner-secret" {
		t.Fatalf("connector credentials = %#v", connector.credentials)
	}
	if _, found, err := repo.GetCredential(context.Background(), ordinaryUserID, serverRef); err != nil || found {
		t.Fatalf("ordinary credential copy found=%v error=%v", found, err)
	}
}

func TestAdministratorCheckFailsClosedWithoutConfiguredOwner(t *testing.T) {
	t.Parallel()
	service, err := NewService(
		testMCPConfig(), newFakeRepository(), nil, nil, nil, Catalog{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.IsAdministrator(uuid.NewString()) {
		t.Fatal("unconfigured administrator identity granted management access")
	}
}

func TestPreflightReauthorizesSelectionWithoutPersistingSnapshot(t *testing.T) {
	t.Parallel()
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{
		ConversationID: conversationID,
		UserID:         userID,
	}
	server := testManifestServer("weather", "global")
	repo.selections[userID+":"+conversationID] = Selection{
		ConversationID: conversationID,
		Mode:           SelectionModeCustom,
		Revision:       3,
		Servers:        []SelectionServer{{Ref: server.Ref}},
	}
	service, err := NewService(
		testMCPConfig(), repo, &fakeConnector{}, nil, nil, Catalog{}, []Server{server},
	)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := service.Preflight(context.Background(), userID, conversationID)
	if err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}
	if !prepared.Enabled() || prepared.Snapshot.SelectionRevision != 3 {
		t.Fatalf("Preflight() prepared = %#v", prepared.Snapshot)
	}
	if prepared.Snapshot.RunID != "" || prepared.Snapshot.Hash != "" || len(repo.snapshots) != 0 {
		t.Fatalf("Preflight() persisted run state: snapshot=%#v repo=%d", prepared.Snapshot, len(repo.snapshots))
	}
}

func TestPrepareRunReauthorizesManifestWorkspaceGrant(t *testing.T) {
	t.Parallel()
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{ConversationID: conversationID, UserID: userID, WorkspaceID: uuid.NewString()}
	server := testManifestServer("private-team-tool", "workspace")
	server.Grants[0].ScopeID = uuid.NewString()
	repo.selections[userID+":"+conversationID] = Selection{
		ConversationID: conversationID, Mode: SelectionModeCustom, Revision: 1,
		Servers: []SelectionServer{{Ref: server.Ref}},
	}
	service, err := NewService(testMCPConfig(), repo, &fakeConnector{}, nil, nil, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.PrepareRun(context.Background(), userID, conversationID, "", uuid.NewString())
	if !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("PrepareRun() error = %v, want authorization denial", err)
	}
}

func TestExecutePersistsRedactedTimelineAndExternalizesLargeResult(t *testing.T) {
	t.Parallel()
	userID, conversationID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	tool := normalizeTool(
		ServerRef{Source: SourceManifest, ID: "files"}, "read_file", "", "",
		map[string]any{
			"type": "object", "additionalProperties": false,
			"required":   []any{"path"},
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
		}, ClassificationRead,
	)
	server := Server{Ref: tool.ServerRef, Name: "Files", Transport: TransportStreamableHTTP, AuthType: AuthNone, Status: ServerStatusReady, Tools: []Tool{tool}, Grants: []Grant{{ScopeType: "global"}}}
	connector := &fakeConnector{sessions: []Session{fakeSession{result: CallResult{Content: []Content{{Type: "text", Text: strings.Repeat("x", 64)}}}}}}
	objects := &memoryObjectStore{}
	config := testMCPConfig()
	config.MaxInlineResultBytes = 16
	service, err := NewService(config, repo, connector, nil, objects, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	run := PreparedRun{
		Snapshot: RunSnapshot{RunID: runID, UserID: userID, ConversationID: conversationID},
		servers:  map[string]Server{server.Ref.Key(): server},
		aliases:  map[string]Tool{tool.Alias: tool},
	}
	var events []ExecutionEvent
	result, err := service.Execute(context.Background(), userID, run, ExecuteInput{
		Alias: tool.Alias, Arguments: map[string]any{"path": "/secret/value"}, Round: 1, Call: 1,
	}, func(_ context.Context, event ExecutionEvent) bool {
		events = append(events, event)
		return true
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].ObjectKey == "" || result.Content[0].Text != "" || len(objects.objects) != 1 {
		t.Fatalf("bounded result = %#v, objects=%d", result, len(objects.objects))
	}
	for _, call := range repo.calls {
		if call.ArgumentsSummary["path"] != "string" || strings.Contains(call.ResultSummary, "secret") {
			t.Fatalf("persisted call leaked arguments: %#v", call)
		}
	}
	if len(events) != 3 || events[0].ServerName != "Files" ||
		events[2].ServerName != "Files" {
		t.Fatalf("execution events lost display name: %#v", events)
	}
}

func TestExecutionEventBoundsServerDisplayName(t *testing.T) {
	event := eventFromCall(
		CallRecord{ServerRef: ServerRef{Source: SourcePrivate, ID: "fixture"}},
		"  "+strings.Repeat("界", maxPrivateServerNameBytes)+"  ",
		nil,
	)

	if event.ServerName == "" || len(event.ServerName) > maxPrivateServerNameBytes ||
		!utf8.ValidString(event.ServerName) || !strings.HasSuffix(event.ServerName, "…") {
		t.Fatalf("bounded Server display name = %q (%d bytes)", event.ServerName, len(event.ServerName))
	}
}

func TestExecuteRetriesTrustedReadButMarksWriteOutcomeUnknown(t *testing.T) {
	t.Parallel()
	userID, conversationID := uuid.NewString(), uuid.NewString()
	for _, test := range []struct {
		name           string
		classification string
		wantConnects   int
		wantUnknown    bool
	}{
		{name: "read retries", classification: ClassificationRead, wantConnects: 2},
		{name: "write never retries", classification: ClassificationWrite, wantConnects: 1, wantUnknown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepository()
			ref := ServerRef{Source: SourceManifest, ID: "remote"}
			tool := normalizeTool(ref, "act", "", "", map[string]any{"type": "object"}, test.classification)
			server := Server{Ref: ref, Transport: TransportStreamableHTTP, AuthType: AuthNone, Status: ServerStatusReady, Tools: []Tool{tool}}
			connector := &fakeConnector{sessions: []Session{
				fakeSession{err: ErrServerUnavailable},
				fakeSession{result: CallResult{Content: []Content{{Type: "text", Text: "ok"}}}},
			}}
			service, err := NewService(testMCPConfig(), repo, connector, nil, &memoryObjectStore{}, Catalog{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			run := PreparedRun{
				Snapshot: RunSnapshot{RunID: uuid.NewString(), UserID: userID, ConversationID: conversationID},
				servers:  map[string]Server{ref.Key(): server}, aliases: map[string]Tool{tool.Alias: tool},
			}
			result, err := service.Execute(context.Background(), userID, run, ExecuteInput{Alias: tool.Alias, Arguments: map[string]any{}, Round: 1, Call: 1}, nil)
			if test.wantUnknown {
				if !errors.Is(err, ErrOutcomeUnknown) || !result.OutcomeUnknown {
					t.Fatalf("write result=%#v err=%v", result, err)
				}
			} else if err != nil || result.ModelContent == "" {
				t.Fatalf("read result=%#v err=%v", result, err)
			}
			if connector.connects != test.wantConnects {
				t.Fatalf("connects=%d want=%d", connector.connects, test.wantConnects)
			}
		})
	}
}

func TestDeleteConversationDataRemovesArtifactsBeforeDurableReferencesWhileDisabled(t *testing.T) {
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{
		UserID: userID, ConversationID: conversationID,
	}
	key := "mcp-results/" + conversationID + "/call/001"
	repo.conversationObjects[conversationID] = []string{key}
	objects := &memoryObjectStore{objects: map[string][]byte{key: []byte("fixture")}}
	service, err := NewService(DefaultConfig(), repo, nil, nil, objects, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if service.Config().Enabled {
		t.Fatal("cleanup fixture unexpectedly enabled MCP execution")
	}
	if err := service.DeleteConversationData(context.Background(), userID, conversationID); err != nil {
		t.Fatalf("DeleteConversationData() error = %v", err)
	}
	if len(objects.objects) != 0 || !repo.deletedConversations[conversationID] {
		t.Fatalf("cleanup objects=%#v deleted=%v", objects.objects, repo.deletedConversations)
	}
}

func TestDeleteConversationDataRejectsObjectKeyOutsideConversationPrefix(t *testing.T) {
	userID, conversationID := uuid.NewString(), uuid.NewString()
	repo := newFakeRepository()
	repo.scopes[userID+":"+conversationID] = ConversationScope{
		UserID: userID, ConversationID: conversationID,
	}
	repo.conversationObjects[conversationID] = []string{"files/unrelated"}
	objects := &memoryObjectStore{objects: map[string][]byte{"files/unrelated": []byte("keep")}}
	service, err := NewService(DefaultConfig(), repo, nil, nil, objects, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteConversationData(context.Background(), userID, conversationID); !errors.Is(err, ErrServerUnavailable) {
		t.Fatalf("DeleteConversationData() error = %v", err)
	}
	if len(objects.objects) != 1 || repo.deletedConversations[conversationID] {
		t.Fatalf("unsafe cleanup mutated state: objects=%#v deleted=%v", objects.objects, repo.deletedConversations)
	}
}

func TestPruneExpiredDataRetriesPartialObjectCleanupBeforeDeletingRows(t *testing.T) {
	userID, conversationID := uuid.NewString(), uuid.NewString()
	_ = userID
	firstCall, secondCall := uuid.NewString(), uuid.NewString()
	firstKey := "mcp-results/" + conversationID + "/" + firstCall + "/001"
	secondKey := "mcp-results/" + conversationID + "/" + secondCall + "/001"
	repo := newFakeRepository()
	repo.expiredCalls = []ExpiredCall{
		{ID: firstCall, ConversationID: conversationID, ObjectKeys: []string{firstKey}},
		{ID: secondCall, ConversationID: conversationID, ObjectKeys: []string{secondKey}},
	}
	objects := &memoryObjectStore{
		objects:        map[string][]byte{firstKey: []byte("one"), secondKey: []byte("two")},
		deleteFailures: map[string]int{secondKey: 1},
	}
	service, err := NewService(DefaultConfig(), repo, nil, nil, objects, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := service.PruneExpiredData(context.Background(), 100); count != 0 || !errors.Is(err, ErrServerUnavailable) {
		t.Fatalf("first prune count=%d error=%v", count, err)
	}
	if len(repo.deletedExpired) != 0 || len(repo.expiredCalls) != 2 {
		t.Fatalf("partial cleanup deleted rows: deleted=%v remaining=%d", repo.deletedExpired, len(repo.expiredCalls))
	}
	count, err := service.PruneExpiredData(context.Background(), 100)
	if err != nil || count != 2 {
		t.Fatalf("retry prune count=%d error=%v", count, err)
	}
	if len(objects.objects) != 0 || len(repo.expiredCalls) != 0 || objects.deleteAttempts[firstKey] != 2 {
		t.Fatalf("retry state objects=%d calls=%d attempts=%v", len(objects.objects), len(repo.expiredCalls), objects.deleteAttempts)
	}
}

func TestRunRetentionReportsStartupSweepFailureAndStopsOnCancellation(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("retention list failed")
	repo := &failingRetentionRepository{
		fakeRepository: newFakeRepository(),
		err:            wantErr,
	}
	service, err := NewService(DefaultConfig(), repo, nil, nil, nil, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	reported := make(chan error, 1)
	go func() {
		defer close(done)
		service.RunRetention(ctx, func(err error) {
			reported <- err
		})
	}()

	select {
	case gotErr := <-reported:
		if !errors.Is(gotErr, wantErr) {
			t.Fatalf("reported error = %v, want %v", gotErr, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("startup retention failure was not reported")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not stop after cancellation")
	}
}

func TestExecuteSerializesWritesForSameUserAcrossRuns(t *testing.T) {
	userID, conversationID := uuid.NewString(), uuid.NewString()
	ref := ServerRef{Source: SourceManifest, ID: "write-serial"}
	tool := normalizeTool(ref, "mutate", "", "", map[string]any{"type": "object"}, ClassificationWrite)
	server := Server{Ref: ref, Transport: TransportStreamableHTTP, AuthType: AuthNone, Status: ServerStatusReady, Tools: []Tool{tool}}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	newSession := func() Session {
		return serialWriteSession{entered: entered, release: release, active: &active, maximum: &maximum}
	}
	connector := &fakeConnector{sessions: []Session{newSession(), newSession()}}
	service, err := NewService(testMCPConfig(), newFakeRepository(), connector, nil, &memoryObjectStore{}, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := func() PreparedRun {
		return PreparedRun{
			Snapshot: RunSnapshot{RunID: uuid.NewString(), UserID: userID, ConversationID: conversationID},
			servers:  map[string]Server{ref.Key(): server}, aliases: map[string]Tool{tool.Alias: tool},
		}
	}
	errorsSeen := make(chan error, 2)
	go func() {
		_, err := service.Execute(context.Background(), userID, run(), ExecuteInput{Alias: tool.Alias, Arguments: map[string]any{}, Round: 1, Call: 1}, nil)
		errorsSeen <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first write did not start")
	}
	go func() {
		_, err := service.Execute(context.Background(), userID, run(), ExecuteInput{Alias: tool.Alias, Arguments: map[string]any{}, Round: 1, Call: 2}, nil)
		errorsSeen <- err
	}()
	select {
	case <-entered:
		t.Fatal("second write started before the first completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range 2 {
		select {
		case err := <-errorsSeen:
			if err != nil {
				t.Fatalf("write execution error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("write execution did not finish")
		}
	}
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent writes = %d, want 1", maximum.Load())
	}
}

type serialWriteSession struct {
	entered chan<- struct{}
	release <-chan struct{}
	active  *atomic.Int32
	maximum *atomic.Int32
}

func (s serialWriteSession) ListTools(context.Context) ([]Tool, error) { return nil, nil }
func (s serialWriteSession) Close() error                              { return nil }
func (s serialWriteSession) CallTool(context.Context, string, map[string]any) (CallResult, error) {
	current := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		maximum := s.maximum.Load()
		if current <= maximum || s.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	s.entered <- struct{}{}
	<-s.release
	return CallResult{Content: []Content{{Type: "text", Text: "ok"}}}, nil
}

func testManifestServer(id, grant string) Server {
	ref := ServerRef{Source: SourceManifest, ID: id}
	tool := normalizeTool(ref, "lookup", "", "", map[string]any{"type": "object"}, ClassificationRead)
	return Server{
		Ref: ref, Name: id, Transport: TransportStreamableHTTP,
		AuthType: AuthNone, Status: ServerStatusReady, Tools: []Tool{tool},
		Grants: []Grant{{ScopeType: grant}},
	}
}

func testMCPConfig() Config {
	config := DefaultConfig()
	config.Enabled = true
	config.RemoteEnabled = true
	return config
}

func testMCPAdminConfig(userID string) Config {
	config := testMCPConfig()
	config.AdministratorUserID = userID
	return config
}

func testVault(t *testing.T) *providersecrets.Vault {
	t.Helper()
	key := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	vault, err := providersecrets.NewVault(providersecrets.KeyringConfig{
		V: providersecrets.KeyringVersion, ActiveKID: "test",
		Keys: []providersecrets.KeyConfig{{KID: "test", Key: key}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return vault
}
