package mcpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type fakeRepository struct {
	mu                   sync.Mutex
	private              map[string]Server
	credentials          map[string]Credential
	selections           map[string]Selection
	workspaceSelections  map[string]WorkspaceSelection
	scopes               map[string]ConversationScope
	oauthStates          map[string]OAuthState
	calls                map[string]CallRecord
	results              map[string][]Content
	snapshots            []RunSnapshot
	conversationObjects  map[string][]string
	deletedConversations map[string]bool
	expiredCalls         []ExpiredCall
	deletedExpired       []string
	pendingArtifacts     []PendingArtifact
}

type failingRetentionRepository struct {
	*fakeRepository
	err error
}

func (r *failingRetentionRepository) ListExpiredCalls(
	context.Context,
	time.Time,
	int,
) ([]ExpiredCall, error) {
	return nil, r.err
}

func (r *fakeRepository) ListPendingArtifacts(_ context.Context, limit int) ([]PendingArtifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.pendingArtifacts) {
		limit = len(r.pendingArtifacts)
	}
	return append([]PendingArtifact(nil), r.pendingArtifacts[:limit]...), nil
}

func (r *fakeRepository) DeletePendingArtifacts(_ context.Context, objectKeys []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	deleted := make(map[string]struct{}, len(objectKeys))
	for _, key := range objectKeys {
		deleted[key] = struct{}{}
	}
	retained := r.pendingArtifacts[:0]
	for _, artifact := range r.pendingArtifacts {
		if _, ok := deleted[artifact.ObjectKey]; !ok {
			retained = append(retained, artifact)
		}
	}
	r.pendingArtifacts = retained
	return nil
}

func (r *fakeRepository) ListExpiredCalls(_ context.Context, _ time.Time, limit int) ([]ExpiredCall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.expiredCalls) {
		limit = len(r.expiredCalls)
	}
	result := make([]ExpiredCall, limit)
	copy(result, r.expiredCalls[:limit])
	for index := range result {
		result[index].ObjectKeys = append([]string(nil), result[index].ObjectKeys...)
	}
	return result, nil
}

func (r *fakeRepository) DeleteExpiredData(_ context.Context, _ time.Time, callIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedExpired = append(r.deletedExpired, callIDs...)
	deleted := make(map[string]struct{}, len(callIDs))
	for _, callID := range callIDs {
		deleted[callID] = struct{}{}
	}
	retained := r.expiredCalls[:0]
	for _, call := range r.expiredCalls {
		if _, ok := deleted[call.ID]; !ok {
			retained = append(retained, call)
		}
	}
	r.expiredCalls = retained
	return nil
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		private: map[string]Server{}, credentials: map[string]Credential{},
		selections: map[string]Selection{}, workspaceSelections: map[string]WorkspaceSelection{},
		scopes: map[string]ConversationScope{}, oauthStates: map[string]OAuthState{},
		calls: map[string]CallRecord{}, results: map[string][]Content{},
		conversationObjects: map[string][]string{}, deletedConversations: map[string]bool{},
	}
}

func (r *fakeRepository) ListConversationObjectKeys(_ context.Context, userID, conversationID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.scopes[userID+":"+conversationID]; !ok {
		return nil, ErrSelectionInvalid
	}
	return append([]string(nil), r.conversationObjects[conversationID]...), nil
}

func (r *fakeRepository) DeleteConversationData(_ context.Context, userID, conversationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.scopes[userID+":"+conversationID]; !ok {
		return ErrSelectionInvalid
	}
	r.deletedConversations[conversationID] = true
	delete(r.conversationObjects, conversationID)
	return nil
}

func (r *fakeRepository) CountPrivateServers(_ context.Context, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for key := range r.private {
		if len(key) > len(userID)+1 && key[:len(userID)+1] == userID+":" {
			count++
		}
	}
	return count, nil
}

func (r *fakeRepository) CreatePrivateServer(_ context.Context, userID string, input CreateServerInput) (Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	transport := input.Transport
	if transport == "" {
		transport = TransportStreamableHTTP
	}
	server := Server{
		Ref:  ServerRef{Source: SourcePrivate, ID: uuid.NewString()},
		Name: input.Name, EndpointURL: input.EndpointURL,
		Transport: transport, AuthType: input.AuthType,
		Status: ServerStatusDraft, Metadata: cloneObject(input.Metadata),
	}
	server.Icon = boundedMarketplaceIcon(stringField(server.Metadata, "icon"))
	if input.AuthType == AuthHeader {
		server.HeaderAuth = &HeaderAuth{Name: input.HeaderName}
	}
	if input.AuthType == AuthOAuth {
		server.OAuthClient = &OAuthClient{ClientID: input.ClientID, Scopes: input.Scopes}
	}
	r.private[userID+":"+server.Ref.ID] = server
	return server, nil
}

func (r *fakeRepository) ListPrivateServers(_ context.Context, userID string) ([]Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []Server{}
	for key, server := range r.private {
		if len(key) > len(userID)+1 && key[:len(userID)+1] == userID+":" {
			result = append(result, server)
		}
	}
	return result, nil
}

func (r *fakeRepository) GetPrivateServer(_ context.Context, userID, serverID string) (Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	server, ok := r.private[userID+":"+serverID]
	if !ok {
		return Server{}, ErrServerNotFound
	}
	return server, nil
}

func (r *fakeRepository) UpdateServerValidation(_ context.Context, userID, serverID, status string, tools []Tool, _ string, code string, validated *time.Time) (Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := userID + ":" + serverID
	server, ok := r.private[key]
	if !ok {
		return Server{}, ErrServerNotFound
	}
	server.Status, server.Tools, server.LastErrorCode, server.ValidatedAt = status, tools, code, validated
	r.private[key] = server
	return server, nil
}

func (r *fakeRepository) DeletePrivateServer(_ context.Context, userID, serverID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := userID + ":" + serverID
	if _, ok := r.private[key]; !ok {
		return ErrServerNotFound
	}
	delete(r.private, key)
	return nil
}

func credentialKey(userID string, ref ServerRef) string { return userID + ":" + ref.Key() }

func (r *fakeRepository) GetCredential(_ context.Context, userID string, ref ServerRef) (Credential, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	credential, ok := r.credentials[credentialKey(userID, ref)]
	return credential, ok, nil
}

func (r *fakeRepository) UpsertCredential(_ context.Context, credential Credential) (Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if credential.ID == "" {
		credential.ID = uuid.NewString()
	}
	r.credentials[credentialKey(credential.UserID, credential.ServerRef)] = credential
	return credential, nil
}

func (r *fakeRepository) DeleteCredential(_ context.Context, userID string, ref ServerRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.credentials, credentialKey(userID, ref))
	return nil
}

func (r *fakeRepository) ConversationScope(_ context.Context, userID, conversationID string) (ConversationScope, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	scope, ok := r.scopes[userID+":"+conversationID]
	if !ok {
		return ConversationScope{}, ErrSelectionInvalid
	}
	return scope, nil
}

func (r *fakeRepository) GetSelection(_ context.Context, userID, conversationID string) (Selection, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	selection, ok := r.selections[userID+":"+conversationID]
	return selection, ok, nil
}

func (r *fakeRepository) ReplaceSelection(_ context.Context, userID string, selection Selection) (Selection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := userID + ":" + selection.ConversationID
	selection.Revision = r.selections[key].Revision + 1
	r.selections[key] = selection
	return selection, nil
}

func (r *fakeRepository) GetWorkspaceSelection(_ context.Context, _ string, workspaceID string) (WorkspaceSelection, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	selection, ok := r.workspaceSelections[workspaceID]
	return selection, ok, nil
}

func (r *fakeRepository) ReplaceWorkspaceSelection(_ context.Context, _ string, selection WorkspaceSelection) (WorkspaceSelection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	selection.Revision = r.workspaceSelections[selection.WorkspaceID].Revision + 1
	r.workspaceSelections[selection.WorkspaceID] = selection
	return selection, nil
}

func (r *fakeRepository) CreateRunSnapshot(_ context.Context, snapshot RunSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots = append(r.snapshots, snapshot)
	return nil
}

func (r *fakeRepository) CountPendingOAuthStates(_ context.Context, userID string, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, state := range r.oauthStates {
		if state.UserID == userID && state.ConsumedAt == nil && state.ExpiresAt.After(now) {
			count++
		}
	}
	return count, nil
}

func (r *fakeRepository) CreateOAuthState(_ context.Context, state OAuthState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.oauthStates[state.StateHash] = state
	return nil
}

func (r *fakeRepository) ConsumeOAuthState(_ context.Context, hash string, now time.Time) (OAuthState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.oauthStates[hash]
	if !ok {
		return OAuthState{}, ErrOAuthStateInvalid
	}
	if state.ConsumedAt != nil {
		return OAuthState{}, ErrOAuthStateConsumed
	}
	if !now.Before(state.ExpiresAt) {
		return OAuthState{}, ErrOAuthStateExpired
	}
	state.ConsumedAt = &now
	r.oauthStates[hash] = state
	return state, nil
}

func (r *fakeRepository) CreateCall(_ context.Context, _ string, call CallRecord) (CallRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if call.ID == "" {
		call.ID = uuid.NewString()
	}
	r.calls[call.ID] = call
	return call, nil
}

func (r *fakeRepository) FinishCall(_ context.Context, _ string, call CallRecord, content []Content, _ []string, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[call.ID] = call
	r.results[call.ID] = content
	return nil
}

func (r *fakeRepository) ListCalls(_ context.Context, _ string, conversationID, runID string) ([]CallRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	calls := make([]CallRecord, 0)
	for _, call := range r.calls {
		if call.ConversationID == conversationID && (runID == "" || call.RunID == runID) {
			calls = append(calls, call)
		}
	}
	return calls, nil
}

type fakeConnector struct {
	mu       sync.Mutex
	connects int
	sessions []Session
	err      error
}

func (c *fakeConnector) Connect(_ context.Context, _ Server, _ string) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connects++
	if c.err != nil {
		return nil, c.err
	}
	if len(c.sessions) == 0 {
		return nil, ErrServerUnavailable
	}
	session := c.sessions[0]
	c.sessions = c.sessions[1:]
	return session, nil
}

type fakeSession struct {
	tools  []Tool
	result CallResult
	err    error
}

func (s fakeSession) ListTools(context.Context) ([]Tool, error) { return s.tools, s.err }
func (s fakeSession) CallTool(context.Context, string, map[string]any) (CallResult, error) {
	return s.result, s.err
}
func (s fakeSession) Close() error { return nil }

type memoryObjectStore struct {
	mu             sync.Mutex
	objects        map[string][]byte
	deleteFailures map[string]int
	deleteAttempts map[string]int
}

func (s *memoryObjectStore) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	data, err := io.ReadAll(body)
	if err != nil || int64(len(data)) != size {
		return errors.New("bad object")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = bytes.Clone(data)
	return nil
}

func (s *memoryObjectStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteAttempts == nil {
		s.deleteAttempts = map[string]int{}
	}
	s.deleteAttempts[key]++
	if s.deleteFailures[key] > 0 {
		s.deleteFailures[key]--
		return errors.New("delete unavailable")
	}
	delete(s.objects, key)
	return nil
}
