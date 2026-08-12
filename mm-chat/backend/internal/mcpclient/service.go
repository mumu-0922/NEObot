package mcpclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"neo-chat/mm-chat/backend/internal/providersecrets"
)

const (
	defaultPrivateServerLimit   = 20
	defaultConversationLimit    = 8
	defaultMaxExposedTools      = 32
	defaultMaxCallsPerRun       = 32
	defaultMaxRoundsPerRun      = 8
	defaultMaxConcurrentPerUser = 4
	defaultMaxOAuthFlows        = 5
	defaultCallTimeout          = 30 * time.Second
	defaultRunTimeout           = 120 * time.Second
	defaultAuditRetention       = 90 * 24 * time.Hour
	defaultCleanupInterval      = time.Hour
	defaultMaxInlineResultBytes = int64(256 << 10)
	defaultMaxResultItemBytes   = int64(25 << 20)
	defaultMaxResultCallBytes   = int64(50 << 20)
	defaultSnapshotLifetime     = 24 * time.Hour
	maxCredentialBytes          = 64 << 10
	maxPrivateServerNameBytes   = 256
	maxPrivateScopes            = 32
)

type ResultObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Delete(context.Context, string) error
}

type Service struct {
	config      Config
	repo        Repository
	connector   Connector
	vault       *providersecrets.Vault
	objects     ResultObjectStore
	catalog     map[string]Server
	manifest    map[string]Server
	marketplace Marketplace
	now         func() time.Time
	sharedMu    sync.RWMutex
	refresh     singleflight.Group
	limitMu     sync.Mutex
	userLimits  map[string]chan struct{}
	userWrites  map[string]chan struct{}
}

type ServiceOption func(*Service)

func WithMarketplace(marketplace Marketplace) ServiceOption {
	return func(service *Service) {
		service.marketplace = marketplace
	}
}

type storedCredential struct {
	HeaderValue        string    `json:"headerValue,omitempty"`
	AccessToken        string    `json:"accessToken,omitempty"`
	RefreshToken       string    `json:"refreshToken,omitempty"`
	TokenType          string    `json:"tokenType,omitempty"`
	Scope              string    `json:"scope,omitempty"`
	ExpiresAt          time.Time `json:"expiresAt,omitempty"`
	TokenEndpoint      string    `json:"tokenEndpoint,omitempty"`
	RevocationEndpoint string    `json:"revocationEndpoint,omitempty"`
	ClientID           string    `json:"clientId,omitempty"`
	ClientSecret       string    `json:"clientSecret,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		PrivateServerLimit:   defaultPrivateServerLimit,
		ConversationLimit:    defaultConversationLimit,
		MaxExposedTools:      defaultMaxExposedTools,
		MaxCallsPerRun:       defaultMaxCallsPerRun,
		MaxRoundsPerRun:      defaultMaxRoundsPerRun,
		MaxConcurrentPerUser: defaultMaxConcurrentPerUser,
		MaxOAuthFlows:        defaultMaxOAuthFlows,
		CallTimeout:          defaultCallTimeout,
		RunTimeout:           defaultRunTimeout,
		AuditRetention:       defaultAuditRetention,
		CleanupInterval:      defaultCleanupInterval,
		MaxInlineResultBytes: defaultMaxInlineResultBytes,
		MaxResultItemBytes:   defaultMaxResultItemBytes,
		MaxResultCallBytes:   defaultMaxResultCallBytes,
	}
}

func NewService(
	config Config,
	repo Repository,
	connector Connector,
	vault *providersecrets.Vault,
	objects ResultObjectStore,
	catalog Catalog,
	manifest []Server,
	options ...ServiceOption,
) (*Service, error) {
	config = normalizeConfig(config)
	if config.Enabled && repo == nil {
		return nil, ErrRepositoryRequired
	}
	if connector == nil {
		connector = NewDirectConnector("neo-chat", "dev")
	}
	service := &Service{
		config:     config,
		repo:       repo,
		connector:  connector,
		vault:      vault,
		objects:    objects,
		catalog:    make(map[string]Server, len(catalog.Servers)),
		manifest:   make(map[string]Server, len(manifest)),
		now:        time.Now,
		userLimits: make(map[string]chan struct{}),
		userWrites: make(map[string]chan struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	for _, server := range catalog.Servers {
		if server.Ref.Source == "" {
			server.Ref.Source = SourceCatalog
		}
		if server.Ref.Source != SourceCatalog || !validServerRef(server.Ref) {
			return nil, fmt.Errorf("%w: catalog server identity", ErrManifestInvalid)
		}
		if _, exists := service.catalog[server.Ref.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate catalog server", ErrManifestInvalid)
		}
		service.catalog[server.Ref.ID] = cloneServer(server)
	}
	for _, server := range manifest {
		if server.Ref.Source != SourceManifest || !validServerRef(server.Ref) {
			return nil, fmt.Errorf("%w: manifest server identity", ErrManifestInvalid)
		}
		if _, exists := service.manifest[server.Ref.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate manifest server", ErrManifestInvalid)
		}
		service.manifest[server.Ref.ID] = cloneServer(server)
	}
	return service, nil
}

func (s *Service) Config() Config {
	if s == nil {
		return normalizeConfig(Config{})
	}
	return s.config
}

// ValidateSharedServers validates shipped and deployment-managed definitions
// without making their availability a global API readiness dependency.
func (s *Service) ValidateSharedServers(ctx context.Context) error {
	if err := s.available(); err != nil {
		return err
	}
	var failures []error
	for _, source := range []string{SourceCatalog, SourceManifest} {
		for _, id := range s.sharedServerIDs(source) {
			server, err := s.sharedServer(ServerRef{Source: source, ID: id})
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if _, hidden := marketplaceArtifactFromServer(server); hidden {
				continue
			}
			if server.AuthType == AuthOAuth {
				server.Status = ServerStatusNeedsAuth
				s.setSharedServer(server)
				continue
			}
			credential := ""
			if server.AuthType == AuthHeader && server.HeaderAuth != nil {
				credential = server.HeaderAuth.EncryptedSecret
			}
			discovered, discoverErr := s.discoverTools(ctx, server, credential)
			if discoverErr != nil {
				server.Status = ServerStatusUnavailable
				server.LastErrorCode = "preflight_failed"
				s.setSharedServer(server)
				failures = append(failures, ErrServerUnavailable)
				continue
			}
			s.setSharedServer(discovered)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) ListServers(ctx context.Context, userID, conversationID string) ([]Server, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	private, err := s.repo.ListPrivateServers(ctx, userID)
	if err != nil {
		return nil, err
	}
	scope := ConversationScope{}
	if conversationID != "" {
		scope, err = s.repo.ConversationScope(ctx, userID, conversationID)
		if err != nil {
			return nil, err
		}
	}
	s.sharedMu.RLock()
	scopeServers := make([]Server, 0, len(s.catalog)+len(s.manifest)+len(private))
	for _, server := range s.catalog {
		scopeServers = append(scopeServers, cloneServer(server))
	}
	for _, server := range s.manifest {
		if _, hidden := marketplaceArtifactFromServer(server); !hidden && manifestGranted(server, scope) {
			scopeServers = append(scopeServers, cloneServer(server))
		}
	}
	s.sharedMu.RUnlock()
	for index := range private {
		s.bindPrivateServerDisplay(&private[index])
	}
	scopeServers = append(scopeServers, private...)
	for index := range scopeServers {
		server := &scopeServers[index]
		server.HasCredential = server.AuthType == AuthNone ||
			(server.AuthType == AuthHeader && server.HeaderAuth != nil && server.HeaderAuth.EncryptedSecret != "")
		if !server.HasCredential && server.AuthType != AuthNone {
			_, found, credentialErr := s.repo.GetCredential(ctx, userID, server.Ref)
			if credentialErr != nil {
				return nil, credentialErr
			}
			server.HasCredential = found
		}
	}
	sort.SliceStable(scopeServers, func(i, j int) bool {
		return scopeServers[i].Ref.Key() < scopeServers[j].Ref.Key()
	})
	return scopeServers, nil
}

func (s *Service) CreatePrivateServer(
	ctx context.Context,
	userID string,
	input CreateServerInput,
) (Server, error) {
	if err := s.remoteAvailable(); err != nil {
		return Server{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.EndpointURL = strings.TrimSpace(input.EndpointURL)
	input.Transport = TransportStreamableHTTP
	input.AuthType = strings.TrimSpace(input.AuthType)
	input.HeaderName = strings.TrimSpace(input.HeaderName)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.Scopes = normalizeStrings(input.Scopes, maxPrivateScopes, 256)
	if input.Name == "" || len(input.Name) > maxPrivateServerNameBytes {
		return Server{}, ErrSelectionInvalid
	}
	if _, err := parseEndpoint(input.EndpointURL, true); err != nil {
		return Server{}, ErrURLBlocked
	}
	switch input.AuthType {
	case "", AuthNone:
		input.AuthType = AuthNone
		input.HeaderName = ""
		input.ClientID = ""
		input.Scopes = nil
	case AuthHeader:
		if !validHeaderName(input.HeaderName) || forbiddenCredentialHeader(input.HeaderName) {
			return Server{}, ErrCredentialInvalid
		}
		input.ClientID = ""
		input.Scopes = nil
	case AuthOAuth:
		if input.ClientID == "" || len(input.ClientID) > 2048 {
			return Server{}, ErrCredentialInvalid
		}
		input.HeaderName = ""
	default:
		return Server{}, ErrCredentialInvalid
	}
	count, err := s.repo.CountPrivateServers(ctx, userID)
	if err != nil {
		return Server{}, err
	}
	if count >= s.config.PrivateServerLimit {
		return Server{}, ErrServerLimit
	}
	return s.repo.CreatePrivateServer(ctx, userID, input)
}

func (s *Service) createPrivateRunnerServer(
	ctx context.Context,
	userID string,
	name string,
	artifact Server,
	metadata map[string]any,
) (Server, error) {
	if err := s.available(); err != nil {
		return Server{}, err
	}
	if !s.config.StdioEnabled {
		return Server{}, ErrStdioDisabled
	}
	name = strings.TrimSpace(name)
	approved, ok := marketplaceArtifactFromServer(artifact)
	if name == "" || len(name) > maxPrivateServerNameBytes || !ok ||
		artifact.Ref.Source != SourceManifest || !validServerRef(artifact.Ref) ||
		artifact.Transport != TransportStdio || artifact.Command == nil ||
		approved.DeploymentHash == "" {
		return Server{}, ErrMarketplaceIncompatible
	}
	count, err := s.repo.CountPrivateServers(ctx, userID)
	if err != nil {
		return Server{}, err
	}
	if count >= s.config.PrivateServerLimit {
		return Server{}, ErrServerLimit
	}
	return s.repo.CreatePrivateServer(ctx, userID, CreateServerInput{
		Name: name, EndpointURL: "runner://" + artifact.Ref.ID,
		Transport: TransportStdio, AuthType: AuthNone, Metadata: metadata,
	})
}

func (s *Service) DeletePrivateServer(ctx context.Context, userID, serverID string) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.repo.DeletePrivateServer(ctx, userID, serverID)
}

func (s *Service) SetHeaderCredential(
	ctx context.Context,
	userID string,
	conversationID string,
	ref ServerRef,
	value string,
) (Server, error) {
	if err := s.available(); err != nil {
		return Server{}, err
	}
	scope := ConversationScope{}
	var err error
	if conversationID != "" {
		scope, err = s.repo.ConversationScope(ctx, userID, conversationID)
		if err != nil {
			return Server{}, err
		}
	}
	server, err := s.serverForUser(ctx, userID, ref, scope)
	if err != nil {
		return Server{}, err
	}
	if server.AuthType != AuthHeader || server.HeaderAuth == nil {
		return Server{}, ErrCredentialInvalid
	}
	if value == "" || len(value) > maxCredentialBytes || strings.ContainsAny(value, "\r\n") {
		return Server{}, ErrCredentialInvalid
	}
	if err := s.storeCredential(ctx, userID, ref, AuthHeader, storedCredential{HeaderValue: value}, nil); err != nil {
		return Server{}, err
	}
	server.HasCredential = true
	return server, nil
}

func (s *Service) DeleteCredential(ctx context.Context, userID, conversationID string, ref ServerRef) error {
	if err := s.available(); err != nil {
		return err
	}
	scope := ConversationScope{}
	var err error
	if conversationID != "" {
		scope, err = s.repo.ConversationScope(ctx, userID, conversationID)
		if err != nil {
			return err
		}
	}
	if _, err := s.serverForUser(ctx, userID, ref, scope); err != nil {
		return err
	}
	return s.repo.DeleteCredential(ctx, userID, ref)
}

func (s *Service) ValidatePrivateServer(
	ctx context.Context,
	userID string,
	serverID string,
) (Server, error) {
	if err := s.available(); err != nil {
		return Server{}, err
	}
	server, err := s.repo.GetPrivateServer(ctx, userID, serverID)
	if err != nil {
		return Server{}, err
	}
	if err := s.requireTransportEnabled(server); err != nil {
		return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "transport_disabled", err)
	}
	var runnerArtifact Server
	if server.Transport == TransportStreamableHTTP {
		if _, err := ValidateEndpoint(ctx, server.EndpointURL, NetworkPolicy{RequireHTTPS: true}); err != nil {
			return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "url_blocked", ErrURLBlocked)
		}
	} else {
		var artifactErr error
		runnerArtifact, artifactErr = s.privateRunnerArtifact(server)
		if artifactErr != nil {
			return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "runner_artifact_invalid", artifactErr)
		}
	}
	credential, credentialErr := s.connectionCredential(ctx, userID, server)
	if credentialErr != nil {
		return s.recordValidationFailure(ctx, userID, server, ServerStatusNeedsAuth, "credential_required", credentialErr)
	}
	session, err := s.connector.Connect(ctx, server, credential)
	if err != nil {
		return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "connect_failed", err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx)
	if err != nil {
		return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "tools_list_failed", err)
	}
	if server.Transport == TransportStdio {
		tools = bindPrivateRunnerToolPolicy(tools, runnerArtifact)
	}
	hash, err := toolSnapshotHash(tools)
	if err != nil {
		return s.recordValidationFailure(ctx, userID, server, ServerStatusUnavailable, "tool_schema_invalid", err)
	}
	now := s.now().UTC()
	validated, err := s.repo.UpdateServerValidation(
		ctx, userID, server.Ref.ID, ServerStatusReady, tools, hash, "", &now,
	)
	if err != nil {
		return Server{}, err
	}
	if server.Transport == TransportStdio {
		validated.Icon = boundedMarketplaceIcon(runnerArtifact.Icon)
	}
	return validated, nil
}

func (s *Service) privateRunnerArtifact(server Server) (Server, error) {
	if server.Ref.Source != SourcePrivate || server.Transport != TransportStdio ||
		server.AuthType != AuthNone || server.Metadata == nil {
		return Server{}, ErrServerUnavailable
	}
	artifactID, _ := server.Metadata["runnerArtifactId"].(string)
	artifactID = strings.TrimSpace(artifactID)
	if !manifestIDPattern.MatchString(artifactID) || server.EndpointURL != "runner://"+artifactID {
		return Server{}, ErrServerUnavailable
	}
	artifact, err := s.sharedServer(ServerRef{Source: SourceManifest, ID: artifactID})
	if err != nil || artifact.Transport != TransportStdio || artifact.Command == nil {
		return Server{}, ErrServerUnavailable
	}
	approved, ok := marketplaceArtifactFromServer(artifact)
	provenance, _ := server.Metadata["marketplace"].(map[string]any)
	provider, _ := provenance["provider"].(string)
	identifier, _ := provenance["identifier"].(string)
	version, _ := provenance["version"].(string)
	deploymentHash, _ := provenance["deploymentHash"].(string)
	if !ok || approved.DeploymentHash == "" ||
		provider != approved.Provider || identifier != approved.Identifier ||
		version != approved.Version || deploymentHash != approved.DeploymentHash {
		return Server{}, ErrServerUnavailable
	}
	return artifact, nil
}

// bindPrivateRunnerToolPolicy is intentionally narrower than normalizeTool:
// ordinary private Servers remain unknown because remote annotations are not
// authorization. A private stdio Server reaches this helper only after its
// exact Marketplace provenance has been rebound to the current reviewed
// manifest artifact, so that artifact's local policy is authoritative.
func bindPrivateRunnerToolPolicy(tools []Tool, artifact Server) []Tool {
	policy, _ := artifact.Metadata["toolPolicy"].(map[string]string)
	bound := append([]Tool(nil), tools...)
	for index := range bound {
		bound[index].Classification = normalizeClassification(policy[bound[index].Name])
	}
	return bound
}

func (s *Service) bindPrivateServerDisplay(server *Server) {
	if server == nil {
		return
	}
	server.Icon = boundedMarketplaceIcon(server.Icon)
	if server.Transport != TransportStdio {
		return
	}
	server.Icon = ""
	artifact, err := s.privateRunnerArtifact(*server)
	if err != nil {
		return
	}
	server.Icon = boundedMarketplaceIcon(artifact.Icon)
	server.Tools = bindPrivateRunnerToolPolicy(server.Tools, artifact)
}

func (s *Service) GetSelection(
	ctx context.Context,
	userID string,
	conversationID string,
) (Selection, error) {
	if err := s.available(); err != nil {
		return Selection{}, err
	}
	selection, found, err := s.repo.GetSelection(ctx, userID, conversationID)
	if err != nil {
		return Selection{}, err
	}
	if found {
		return selection, nil
	}
	if _, err := s.repo.ConversationScope(ctx, userID, conversationID); err != nil {
		return Selection{}, err
	}
	return Selection{
		ConversationID: conversationID,
		Mode:           SelectionModeInherit,
		Revision:       0,
		Servers:        []SelectionServer{},
	}, nil
}

func (s *Service) ReplaceSelection(
	ctx context.Context,
	userID string,
	selection Selection,
) (Selection, error) {
	if err := s.available(); err != nil {
		return Selection{}, err
	}
	scope, err := s.repo.ConversationScope(ctx, userID, selection.ConversationID)
	if err != nil {
		return Selection{}, err
	}
	if selection.Mode != SelectionModeInherit && selection.Mode != SelectionModeCustom {
		return Selection{}, ErrSelectionInvalid
	}
	if selection.Mode == SelectionModeInherit && len(selection.Servers) != 0 {
		return Selection{}, ErrSelectionInvalid
	}
	if err := s.validateSelectionServers(ctx, userID, scope, selection.Servers); err != nil {
		return Selection{}, err
	}
	selection.Servers = normalizeSelectionServers(selection.Servers)
	return s.repo.ReplaceSelection(ctx, userID, selection)
}

func (s *Service) GetWorkspaceSelection(
	ctx context.Context,
	userID string,
	workspaceID string,
) (WorkspaceSelection, error) {
	if err := s.available(); err != nil {
		return WorkspaceSelection{}, err
	}
	selection, found, err := s.repo.GetWorkspaceSelection(ctx, userID, workspaceID)
	if err != nil {
		return WorkspaceSelection{}, err
	}
	if !found {
		return WorkspaceSelection{WorkspaceID: workspaceID, Servers: []SelectionServer{}}, nil
	}
	return selection, nil
}

func (s *Service) ReplaceWorkspaceSelection(
	ctx context.Context,
	userID string,
	selection WorkspaceSelection,
) (WorkspaceSelection, error) {
	if err := s.available(); err != nil {
		return WorkspaceSelection{}, err
	}
	scope := ConversationScope{WorkspaceID: selection.WorkspaceID}
	if err := s.validateSelectionServers(ctx, userID, scope, selection.Servers); err != nil {
		return WorkspaceSelection{}, err
	}
	selection.Servers = normalizeSelectionServers(selection.Servers)
	return s.repo.ReplaceWorkspaceSelection(ctx, userID, selection)
}

func (s *Service) PrepareRun(
	ctx context.Context,
	userID string,
	conversationID string,
	messageID string,
	runID string,
) (PreparedRun, error) {
	if !validUUID(runID) {
		return PreparedRun{}, ErrSelectionInvalid
	}
	return s.prepareRun(ctx, userID, conversationID, messageID, runID, true)
}

// Preflight resolves and reauthorizes the effective conversation selection
// without accepting a chat message or persisting a run snapshot.
func (s *Service) Preflight(
	ctx context.Context,
	userID string,
	conversationID string,
) (PreparedRun, error) {
	return s.prepareRun(ctx, userID, conversationID, "", "", false)
}

func (s *Service) prepareRun(
	ctx context.Context,
	userID string,
	conversationID string,
	messageID string,
	runID string,
	persist bool,
) (PreparedRun, error) {
	if err := s.available(); err != nil {
		return PreparedRun{}, err
	}
	if !validUUID(userID) || !validUUID(conversationID) ||
		(messageID != "" && !validUUID(messageID)) {
		return PreparedRun{}, ErrSelectionInvalid
	}
	scope, err := s.repo.ConversationScope(ctx, userID, conversationID)
	if err != nil {
		return PreparedRun{}, err
	}
	selection, found, err := s.repo.GetSelection(ctx, userID, conversationID)
	if err != nil {
		return PreparedRun{}, err
	}
	if !found {
		selection = Selection{ConversationID: conversationID, Mode: SelectionModeInherit}
	}
	effectiveServers := selection.Servers
	effectiveRevision := selection.Revision
	if selection.Mode == SelectionModeInherit {
		effectiveServers = nil
		if scope.WorkspaceID != "" {
			workspace, workspaceFound, workspaceErr := s.repo.GetWorkspaceSelection(ctx, userID, scope.WorkspaceID)
			if workspaceErr != nil {
				return PreparedRun{}, workspaceErr
			}
			if workspaceFound {
				effectiveServers = workspace.Servers
				effectiveRevision = workspace.Revision
			}
		}
	}
	if len(effectiveServers) == 0 {
		return PreparedRun{}, nil
	}
	if err := s.validateSelectionServers(ctx, userID, scope, effectiveServers); err != nil {
		return PreparedRun{}, err
	}
	prepared := PreparedRun{
		servers: make(map[string]Server, len(effectiveServers)),
		aliases: map[string]Tool{},
	}
	for _, selected := range normalizeSelectionServers(effectiveServers) {
		server, err := s.serverForUser(ctx, userID, selected.Ref, scope)
		if err != nil {
			return PreparedRun{}, err
		}
		if err := s.requireTransportEnabled(server); err != nil {
			return PreparedRun{}, err
		}
		if server.Ref.Source != SourcePrivate && len(server.Tools) == 0 {
			credential, credentialErr := s.connectionCredential(ctx, userID, server)
			if credentialErr != nil {
				return PreparedRun{}, credentialErr
			}
			server, err = s.discoverTools(ctx, server, credential)
			if err != nil {
				return PreparedRun{}, err
			}
		}
		if server.Status != ServerStatusReady {
			return PreparedRun{}, ErrServerNotReady
		}
		if _, err := s.connectionCredential(ctx, userID, server); err != nil {
			return PreparedRun{}, err
		}
		disabled := make(map[string]struct{}, len(selected.DisabledTools))
		for _, name := range selected.DisabledTools {
			disabled[name] = struct{}{}
		}
		enabledTools := make([]Tool, 0, len(server.Tools))
		for _, tool := range sortedTools(server.Tools) {
			if !tool.Supported {
				continue
			}
			if _, off := disabled[tool.Name]; off {
				continue
			}
			if existing, exists := prepared.aliases[tool.Alias]; exists &&
				existing.ServerRef.Key() != tool.ServerRef.Key() {
				return PreparedRun{}, ErrToolUnsupported
			}
			prepared.aliases[tool.Alias] = tool
			enabledTools = append(enabledTools, tool)
		}
		server.Tools = enabledTools
		prepared.servers[server.Ref.Key()] = server
		prepared.Snapshot.Servers = append(prepared.Snapshot.Servers, SnapshotServer{
			Ref: server.Ref, Name: server.Name, Transport: server.Transport, Tools: enabledTools,
		})
	}
	sort.Slice(prepared.Snapshot.Servers, func(i, j int) bool {
		return prepared.Snapshot.Servers[i].Ref.Key() < prepared.Snapshot.Servers[j].Ref.Key()
	})
	now := s.now().UTC()
	prepared.Snapshot.RunID = runID
	prepared.Snapshot.UserID = userID
	prepared.Snapshot.ConversationID = conversationID
	prepared.Snapshot.MessageID = messageID
	prepared.Snapshot.SelectionRevision = max(effectiveRevision, 1)
	prepared.Snapshot.CreatedAt = now
	prepared.Snapshot.ExpiresAt = now.Add(defaultSnapshotLifetime)
	if !persist {
		return prepared, nil
	}
	hash, err := runSnapshotHash(prepared.Snapshot)
	if err != nil {
		return PreparedRun{}, err
	}
	prepared.Snapshot.Hash = hash
	if err := s.repo.CreateRunSnapshot(ctx, prepared.Snapshot); err != nil {
		return PreparedRun{}, err
	}
	return prepared, nil
}

func (s *Service) ToolsForProvider(run PreparedRun, query string) ([]Tool, bool) {
	tools := make([]Tool, 0, len(run.aliases))
	for _, tool := range run.aliases {
		tools = append(tools, tool)
	}
	ranked := rankTools(tools, query)
	limit := s.config.MaxExposedTools
	if len(ranked) <= limit {
		return ranked, false
	}
	return ranked[:max(limit-1, 1)], true
}

func (s *Service) SearchTools(run PreparedRun, query string) []Tool {
	tools := make([]Tool, 0, len(run.aliases))
	for _, tool := range run.aliases {
		tools = append(tools, tool)
	}
	ranked := rankTools(tools, query)
	limit := s.config.MaxExposedTools
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func (s *Service) ToolForAlias(run PreparedRun, alias string) (Tool, bool) {
	tool, ok := run.aliases[strings.TrimSpace(alias)]
	return tool, ok
}

func (s *Service) ListCalls(
	ctx context.Context,
	userID string,
	conversationID string,
	runID string,
) ([]CallRecord, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if !validUUID(userID) || !validUUID(conversationID) ||
		(runID != "" && !validUUID(runID)) {
		return nil, ErrSelectionInvalid
	}
	if _, err := s.repo.ConversationScope(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	return s.repo.ListCalls(ctx, userID, conversationID, runID)
}

// DeleteConversationData removes MCP artifacts and durable execution state
// before the owning conversation is soft-deleted. It intentionally remains
// available while the MCP execution kill switch is off.
func (s *Service) DeleteConversationData(
	ctx context.Context,
	userID string,
	conversationID string,
) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if !validUUID(userID) || !validUUID(conversationID) {
		return ErrSelectionInvalid
	}
	lifecycle, ok := s.repo.(ConversationLifecycleRepository)
	if !ok {
		return ErrRepositoryRequired
	}
	objectKeys, err := lifecycle.ListConversationObjectKeys(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	prefix := "mcp-results/" + conversationID + "/"
	for _, key := range objectKeys {
		if !strings.HasPrefix(key, prefix) || len(key) > 2048 {
			return ErrServerUnavailable
		}
		if s.objects == nil {
			return ErrServerUnavailable
		}
		if err := s.objects.Delete(ctx, key); err != nil {
			return ErrServerUnavailable
		}
	}
	return lifecycle.DeleteConversationData(ctx, userID, conversationID)
}

// PruneExpiredData removes expired MCP result artifacts before acknowledging
// their durable call rows. Repeating the method after a partial object-store
// failure is safe because object deletion is required to be idempotent and no
// row is removed until every object for the selected call batch succeeds.
func (s *Service) PruneExpiredData(ctx context.Context, limit int) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	retention, ok := s.repo.(RetentionRepository)
	if !ok {
		return 0, ErrRepositoryRequired
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	now := s.now().UTC()
	removed := 0
	if cleanup, ok := s.repo.(ArtifactCleanupRepository); ok {
		artifacts, err := cleanup.ListPendingArtifacts(ctx, limit)
		if err != nil {
			return 0, err
		}
		keys := make([]string, 0, len(artifacts))
		for _, artifact := range artifacts {
			if !validUUID(artifact.CallID) || !validUUID(artifact.ConversationID) ||
				!validMCPResultObjectKey(artifact.ObjectKey, artifact.ConversationID, artifact.CallID) ||
				s.objects == nil {
				return 0, ErrServerUnavailable
			}
			if err := s.objects.Delete(ctx, artifact.ObjectKey); err != nil {
				return 0, ErrServerUnavailable
			}
			keys = append(keys, artifact.ObjectKey)
		}
		if err := cleanup.DeletePendingArtifacts(ctx, keys); err != nil {
			return 0, err
		}
		removed += len(keys)
	}
	calls, err := retention.ListExpiredCalls(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	completed := make([]string, 0, len(calls))
	for _, call := range calls {
		if !validUUID(call.ID) || !validUUID(call.ConversationID) {
			return 0, ErrServerUnavailable
		}
		for _, key := range call.ObjectKeys {
			if !validMCPResultObjectKey(key, call.ConversationID, call.ID) || s.objects == nil {
				return 0, ErrServerUnavailable
			}
			if err := s.objects.Delete(ctx, key); err != nil {
				return 0, ErrServerUnavailable
			}
		}
		completed = append(completed, call.ID)
	}
	if err := retention.DeleteExpiredData(ctx, now, completed); err != nil {
		return 0, err
	}
	return removed + len(completed), nil
}

// RunRetention performs one startup sweep and then bounded periodic sweeps.
// It remains active when MCP execution is disabled so kill switches do not
// suspend privacy and lifecycle cleanup. Sweep failures are reported without
// stopping later retries; callers own the redaction policy for diagnostics.
func (s *Service) RunRetention(ctx context.Context, onError func(error)) {
	if s == nil || s.repo == nil {
		return
	}
	s.runRetentionSweep(ctx, onError)
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRetentionSweep(ctx, onError)
		}
	}
}

func (s *Service) runRetentionSweep(ctx context.Context, onError func(error)) {
	if _, err := s.PruneExpiredData(ctx, 100); err != nil && onError != nil {
		onError(err)
	}
}

func (s *Service) acquireUserCall(ctx context.Context, userID string) (func(), error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrSelectionInvalid
	}
	s.limitMu.Lock()
	semaphore := s.userLimits[userID]
	if semaphore == nil {
		semaphore = make(chan struct{}, s.config.MaxConcurrentPerUser)
		s.userLimits[userID] = semaphore
	}
	s.limitMu.Unlock()
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) acquireUserWrite(ctx context.Context, userID string) (func(), error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrSelectionInvalid
	}
	s.limitMu.Lock()
	semaphore := s.userWrites[userID]
	if semaphore == nil {
		semaphore = make(chan struct{}, 1)
		s.userWrites[userID] = semaphore
	}
	s.limitMu.Unlock()
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func validMCPResultObjectKey(key, conversationID, callID string) bool {
	prefix := "mcp-results/" + conversationID + "/" + callID + "/"
	if !strings.HasPrefix(key, prefix) || len(key) <= len(prefix) || len(key) > 2048 {
		return false
	}
	remainder := strings.TrimPrefix(key, prefix)
	return !strings.Contains(remainder, "..") && !strings.ContainsAny(remainder, "\\\x00")
}

func (s *Service) available() error {
	if s == nil || !s.config.Enabled {
		return ErrDisabled
	}
	if s.repo == nil {
		return ErrRepositoryRequired
	}
	return nil
}

func (s *Service) remoteAvailable() error {
	if err := s.available(); err != nil {
		return err
	}
	if !s.config.RemoteEnabled {
		return ErrRemoteDisabled
	}
	return nil
}

func (s *Service) requireTransportEnabled(server Server) error {
	switch server.Transport {
	case TransportStreamableHTTP:
		if !s.config.RemoteEnabled {
			return ErrRemoteDisabled
		}
	case TransportStdio:
		if !s.config.StdioEnabled {
			return ErrStdioDisabled
		}
	default:
		return ErrServerUnavailable
	}
	return nil
}

func (s *Service) validateSelectionServers(
	ctx context.Context,
	userID string,
	scope ConversationScope,
	servers []SelectionServer,
) error {
	if len(servers) > s.config.ConversationLimit {
		return ErrSelectionLimit
	}
	seen := map[string]struct{}{}
	for _, selected := range servers {
		if !validServerRef(selected.Ref) {
			return ErrSelectionInvalid
		}
		if _, duplicate := seen[selected.Ref.Key()]; duplicate {
			return ErrSelectionInvalid
		}
		seen[selected.Ref.Key()] = struct{}{}
		server, err := s.serverForUser(ctx, userID, selected.Ref, scope)
		if err != nil {
			return err
		}
		toolNames := make(map[string]struct{}, len(server.Tools))
		for _, tool := range server.Tools {
			toolNames[tool.Name] = struct{}{}
		}
		for _, name := range normalizeStrings(selected.DisabledTools, 512, maxToolNameBytes) {
			if _, exists := toolNames[name]; !exists {
				return ErrToolNotFound
			}
		}
	}
	return nil
}

func (s *Service) serverForUser(
	ctx context.Context,
	userID string,
	ref ServerRef,
	scope ConversationScope,
) (Server, error) {
	if !validServerRef(ref) {
		return Server{}, ErrServerNotFound
	}
	switch ref.Source {
	case SourceCatalog:
		return s.sharedServer(ref)
	case SourceManifest:
		server, err := s.sharedServer(ref)
		if err != nil || !manifestGranted(server, scope) {
			return Server{}, ErrServerNotFound
		}
		return server, nil
	case SourcePrivate:
		server, err := s.repo.GetPrivateServer(ctx, userID, ref.ID)
		if err != nil {
			return Server{}, err
		}
		if server.Transport == TransportStdio {
			artifact, err := s.privateRunnerArtifact(server)
			if err != nil {
				return Server{}, err
			}
			server.Icon = boundedMarketplaceIcon(artifact.Icon)
			server.Tools = bindPrivateRunnerToolPolicy(server.Tools, artifact)
		}
		return server, nil
	default:
		return Server{}, ErrServerNotFound
	}
}

func (s *Service) discoverTools(ctx context.Context, server Server, credential string) (Server, error) {
	if err := s.requireTransportEnabled(server); err != nil {
		return Server{}, err
	}
	session, err := s.connector.Connect(ctx, server, credential)
	if err != nil {
		return Server{}, ErrServerUnavailable
	}
	defer session.Close()
	tools, err := session.ListTools(ctx)
	if err != nil {
		return Server{}, ErrServerUnavailable
	}
	server.Tools = tools
	server.ToolCount = 0
	server.UnsupportedCount = 0
	for _, tool := range tools {
		if tool.Supported {
			server.ToolCount++
		} else {
			server.UnsupportedCount++
		}
	}
	server.Status = ServerStatusReady
	server.LastErrorCode = ""
	now := s.now().UTC()
	server.ValidatedAt = &now
	return server, nil
}

func (s *Service) sharedServer(ref ServerRef) (Server, error) {
	s.sharedMu.RLock()
	defer s.sharedMu.RUnlock()
	var (
		server Server
		ok     bool
	)
	switch ref.Source {
	case SourceCatalog:
		server, ok = s.catalog[ref.ID]
	case SourceManifest:
		server, ok = s.manifest[ref.ID]
	}
	if !ok {
		return Server{}, ErrServerNotFound
	}
	return cloneServer(server), nil
}

func (s *Service) setSharedServer(server Server) {
	s.sharedMu.Lock()
	defer s.sharedMu.Unlock()
	switch server.Ref.Source {
	case SourceCatalog:
		s.catalog[server.Ref.ID] = cloneServer(server)
	case SourceManifest:
		s.manifest[server.Ref.ID] = cloneServer(server)
	}
}

func (s *Service) sharedServerIDs(source string) []string {
	s.sharedMu.RLock()
	defer s.sharedMu.RUnlock()
	servers := s.manifest
	if source == SourceCatalog {
		servers = s.catalog
	}
	ids := make([]string, 0, len(servers))
	for id := range servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *Service) connectionCredential(ctx context.Context, userID string, server Server) (string, error) {
	switch server.AuthType {
	case AuthNone:
		return "", nil
	case AuthHeader:
		if server.HeaderAuth != nil && server.HeaderAuth.EncryptedSecret != "" {
			return server.HeaderAuth.EncryptedSecret, nil
		}
		stored, err := s.loadCredential(ctx, userID, server.Ref)
		if err != nil {
			return "", err
		}
		if stored.HeaderValue == "" {
			return "", ErrCredentialRequired
		}
		return stored.HeaderValue, nil
	case AuthOAuth:
		stored, err := s.loadCredential(ctx, userID, server.Ref)
		if err != nil {
			return "", err
		}
		if stored.AccessToken == "" {
			return "", ErrCredentialRequired
		}
		if !stored.ExpiresAt.IsZero() && !s.now().Add(oauthExpirySafetyWindow).Before(stored.ExpiresAt) {
			if stored.RefreshToken == "" {
				return "", ErrCredentialRequired
			}
			return s.refreshOAuthAccessToken(ctx, userID, server)
		}
		return stored.AccessToken, nil
	default:
		return "", ErrCredentialInvalid
	}
}

func (s *Service) storeCredential(
	ctx context.Context,
	userID string,
	ref ServerRef,
	kind string,
	payload storedCredential,
	expiresAt *time.Time,
) error {
	if s.vault == nil {
		return ErrCredentialInvalid
	}
	plaintext, err := json.Marshal(payload)
	if err != nil || len(plaintext) == 0 || len(plaintext) > maxCredentialBytes {
		return ErrCredentialInvalid
	}
	defer clear(plaintext)
	envelope, err := s.vault.Encrypt(plaintext, credentialContext(userID, ref))
	if err != nil {
		return ErrCredentialInvalid
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return ErrCredentialInvalid
	}
	_, err = s.repo.UpsertCredential(ctx, Credential{
		UserID: userID, ServerRef: ref, Kind: kind,
		EncryptedSecretRef: string(encoded), Metadata: map[string]any{}, ExpiresAt: expiresAt,
	})
	return err
}

func (s *Service) loadCredential(
	ctx context.Context,
	userID string,
	ref ServerRef,
) (storedCredential, error) {
	credential, found, err := s.repo.GetCredential(ctx, userID, ref)
	if err != nil {
		return storedCredential{}, err
	}
	if !found || s.vault == nil {
		return storedCredential{}, ErrCredentialRequired
	}
	envelope, err := providersecrets.ParseEnvelope(credential.EncryptedSecretRef)
	if err != nil {
		return storedCredential{}, ErrCredentialInvalid
	}
	plaintext, err := s.vault.Decrypt(envelope, credentialContext(userID, ref))
	if err != nil {
		return storedCredential{}, ErrCredentialInvalid
	}
	defer clear(plaintext)
	var payload storedCredential
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return storedCredential{}, ErrCredentialInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return storedCredential{}, ErrCredentialInvalid
	}
	return payload, nil
}

func (s *Service) recordValidationFailure(
	ctx context.Context,
	userID string,
	server Server,
	status string,
	code string,
	cause error,
) (Server, error) {
	now := s.now().UTC()
	hash, _ := toolSnapshotHash(server.Tools)
	updated, err := s.repo.UpdateServerValidation(
		ctx, userID, server.Ref.ID, status, server.Tools, hash, code, &now,
	)
	if err != nil {
		return Server{}, err
	}
	return updated, cause
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.PrivateServerLimit <= 0 {
		config.PrivateServerLimit = defaults.PrivateServerLimit
	}
	if config.ConversationLimit <= 0 {
		config.ConversationLimit = defaults.ConversationLimit
	}
	if config.MaxExposedTools <= 1 {
		config.MaxExposedTools = defaults.MaxExposedTools
	}
	if config.MaxCallsPerRun <= 0 {
		config.MaxCallsPerRun = defaults.MaxCallsPerRun
	}
	if config.MaxRoundsPerRun <= 0 {
		config.MaxRoundsPerRun = defaults.MaxRoundsPerRun
	}
	if config.MaxConcurrentPerUser <= 0 {
		config.MaxConcurrentPerUser = defaults.MaxConcurrentPerUser
	}
	if config.MaxOAuthFlows <= 0 {
		config.MaxOAuthFlows = defaults.MaxOAuthFlows
	}
	if config.CallTimeout <= 0 {
		config.CallTimeout = defaults.CallTimeout
	}
	if config.RunTimeout <= 0 {
		config.RunTimeout = defaults.RunTimeout
	}
	if config.AuditRetention <= 0 {
		config.AuditRetention = defaults.AuditRetention
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = defaults.CleanupInterval
	}
	if config.MaxInlineResultBytes <= 0 {
		config.MaxInlineResultBytes = defaults.MaxInlineResultBytes
	}
	if config.MaxResultItemBytes <= 0 {
		config.MaxResultItemBytes = defaults.MaxResultItemBytes
	}
	if config.MaxResultCallBytes <= 0 {
		config.MaxResultCallBytes = defaults.MaxResultCallBytes
	}
	return config
}

func validServerRef(ref ServerRef) bool {
	if strings.TrimSpace(ref.ID) == "" || len(ref.ID) > 128 {
		return false
	}
	switch ref.Source {
	case SourceCatalog, SourceManifest:
		return manifestIDPattern.MatchString(ref.ID)
	case SourcePrivate:
		return validUUID(ref.ID)
	default:
		return false
	}
}

func forbiddenCredentialHeader(name string) bool {
	for _, forbidden := range []string{"host", "cookie", "content-length", "connection", "transfer-encoding", "proxy-authorization"} {
		if strings.EqualFold(strings.TrimSpace(name), forbidden) {
			return true
		}
	}
	return false
}

func manifestGranted(server Server, scope ConversationScope) bool {
	if len(server.Grants) == 0 {
		return false
	}
	for _, grant := range server.Grants {
		switch grant.ScopeType {
		case "global":
			return true
		case "workspace":
			if scope.WorkspaceID != "" && grant.ScopeID == scope.WorkspaceID {
				return true
			}
		case "team":
			if scope.TeamID != "" && grant.ScopeID == scope.TeamID {
				return true
			}
		}
	}
	return false
}

func normalizeSelectionServers(input []SelectionServer) []SelectionServer {
	result := append([]SelectionServer(nil), input...)
	for index := range result {
		result[index].DisabledTools = normalizeStrings(result[index].DisabledTools, 512, maxToolNameBytes)
		sort.Strings(result[index].DisabledTools)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Ref.Key() < result[j].Ref.Key() })
	return result
}

func cloneServer(server Server) Server {
	server.Tools = append([]Tool(nil), server.Tools...)
	server.Grants = append([]Grant(nil), server.Grants...)
	metadata := make(map[string]any, len(server.Metadata))
	for key, value := range server.Metadata {
		metadata[key] = value
	}
	server.Metadata = metadata
	if server.Command != nil {
		command := *server.Command
		command.Argv = append([]string(nil), server.Command.Argv...)
		command.Env = cloneStringMap(server.Command.Env)
		server.Command = &command
	}
	return server
}

func toolSnapshotHash(tools []Tool) (string, error) {
	encoded, err := json.Marshal(sortedTools(tools))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func runSnapshotHash(snapshot RunSnapshot) (string, error) {
	snapshot.Hash = ""
	snapshot.UserID = ""
	snapshot.CreatedAt = time.Time{}
	snapshot.ExpiresAt = time.Time{}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func credentialContext(userID string, ref ServerRef) string {
	return "mcp:credential:" + userID + ":" + ref.Key()
}

func rankTools(tools []Tool, query string) []Tool {
	tokens := strings.Fields(strings.ToLower(query))
	type rankedTool struct {
		tool  Tool
		score int
	}
	ranked := make([]rankedTool, 0, len(tools))
	for _, tool := range tools {
		haystack := strings.ToLower(tool.Name + " " + tool.Title + " " + tool.Description)
		score := 0
		for _, token := range tokens {
			token = strings.Trim(token, ".,!?;:()[]{}\"'")
			if len(token) >= 2 && strings.Contains(haystack, token) {
				score++
			}
		}
		ranked = append(ranked, rankedTool{tool: tool, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].tool.Alias < ranked[j].tool.Alias
	})
	result := make([]Tool, len(ranked))
	for index, item := range ranked {
		result[index] = item.tool
	}
	return result
}
