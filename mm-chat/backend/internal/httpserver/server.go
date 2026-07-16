package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agents"
	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/browserimport"
	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/codejobs"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/health"
	"neo-chat/mm-chat/backend/internal/imagejobs"
	"neo-chat/mm-chat/backend/internal/jobcontrol"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/plugins"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/ragsource"
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
	"neo-chat/mm-chat/backend/internal/voicejobs"
)

const contentTypeJSON = "application/json; charset=utf-8"

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Option func(*options)

type SessionResolver interface {
	ResolveByTokenHash(ctx context.Context, tokenHash string) (auth.Session, error)
}

type options struct {
	readyChecks          []health.Check
	logger               *slog.Logger
	chatRepository       chat.Repository
	chatProvider         chat.Provider
	runCancellationStore chat.RunCancellationStore
	rateLimitStore       ratelimit.Store
	sessionResolver      SessionResolver
	authService          *auth.Service
	fileRepository       files.Repository
	objectStore          storage.ObjectStore
	metrics              *Metrics
	dbStatsProvider      DatabaseStatsProvider
	maxUploadBytes       int64
	importRepository     browserimport.Repository
	maxImportBytes       int64
	teamService          *teams.Service
	knowledgeService     *knowledge.Service
	agentService         *agents.Service
	pluginRegistry       plugins.Registry
	pluginAuditRecorder  plugins.AuditRecorder
	imageJobService      *imagejobs.Service
	voiceJobService      *voicejobs.Service
	ragSourceService     *ragsource.Service
}

type knowledgeRAGCandidateSource struct {
	service *knowledge.Service
}

type fileProviderAttachmentResolver struct {
	service  *files.Service
	maxBytes int64
}

func (r fileProviderAttachmentResolver) ResolveProviderAttachment(
	ctx context.Context,
	attachment chat.Attachment,
) (chat.ProviderAttachment, error) {
	if r.service == nil {
		return chat.ProviderAttachment{}, chat.ValidationError{
			Code:    "ATTACHMENT_CONTENT_UNAVAILABLE",
			Message: "image attachment content is not available for provider streaming",
		}
	}

	record, reader, err := r.service.GetContent(ctx, attachment.FileID)
	if err != nil {
		if errors.Is(err, files.ErrFileNotFound) {
			return chat.ProviderAttachment{}, chat.ErrFileNotFound
		}
		return chat.ProviderAttachment{}, err
	}
	defer reader.Close()

	maxBytes := r.maxBytes
	if maxBytes <= 0 {
		maxBytes = config.DefaultMaxUploadBytes
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return chat.ProviderAttachment{}, err
	}
	if int64(len(data)) > maxBytes {
		return chat.ProviderAttachment{}, chat.ValidationError{
			Code:    "ATTACHMENT_TOO_LARGE",
			Message: "image attachment is too large for provider streaming",
		}
	}

	mimeType := strings.TrimSpace(record.MimeType)
	if mimeType == "" {
		mimeType = attachment.MimeType
	}
	fileName := strings.TrimSpace(record.OriginalFilename)
	if fileName == "" {
		fileName = attachment.FileName
	}
	size := record.ByteSize
	if size == 0 {
		size = attachment.Size
	}
	sha256 := strings.TrimSpace(record.SHA256)
	if sha256 == "" {
		sha256 = attachment.SHA256
	}

	return chat.ProviderAttachment{
		FileID:   attachment.FileID,
		FileName: fileName,
		MimeType: mimeType,
		Size:     size,
		SHA256:   sha256,
		Purpose:  attachment.Purpose,
		Data:     data,
	}, nil
}

func (source knowledgeRAGCandidateSource) FetchEvidenceCandidates(
	ctx context.Context,
	query chat.RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	if source.service == nil {
		return nil, knowledge.ErrDatabaseRequired
	}
	return source.service.FetchQueryEvidenceCandidates(ctx, knowledge.QueryEvidenceCandidatesInput{
		CollectionIDs: query.CollectionIDs,
		QueryText:     query.QueryText,
		Limit:         query.Limit,
	})
}

func WithReadyChecker(checker health.ReadinessChecker) Option {
	return func(opts *options) {
		opts.readyChecks = append(opts.readyChecks, health.Check{Name: "database", Checker: checker})
	}
}

func WithReadyCheck(name string, checker health.ReadinessChecker) Option {
	return func(opts *options) {
		opts.readyChecks = append(opts.readyChecks, health.Check{Name: name, Checker: checker})
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

func WithChatRepository(repo chat.Repository) Option {
	return func(opts *options) {
		opts.chatRepository = repo
	}
}

func WithChatProvider(provider chat.Provider) Option {
	return func(opts *options) {
		opts.chatProvider = provider
	}
}

func WithRunCancellationStore(store chat.RunCancellationStore) Option {
	return func(opts *options) {
		opts.runCancellationStore = store
	}
}

func WithRateLimitStore(store ratelimit.Store) Option {
	return func(opts *options) {
		opts.rateLimitStore = store
	}
}

func WithSessionResolver(resolver SessionResolver) Option {
	return func(opts *options) {
		opts.sessionResolver = resolver
	}
}

func WithAuthService(service *auth.Service) Option {
	return func(opts *options) {
		opts.authService = service
	}
}

func WithFileRepository(repo files.Repository) Option {
	return func(opts *options) {
		opts.fileRepository = repo
	}
}

func WithObjectStore(store storage.ObjectStore) Option {
	return func(opts *options) {
		opts.objectStore = store
	}
}

func WithMetrics(metrics *Metrics) Option {
	return func(opts *options) {
		opts.metrics = metrics
	}
}

func WithDatabaseStatsProvider(provider DatabaseStatsProvider) Option {
	return func(opts *options) {
		opts.dbStatsProvider = provider
	}
}

func WithMaxUploadBytes(maxUploadBytes int64) Option {
	return func(opts *options) {
		opts.maxUploadBytes = maxUploadBytes
	}
}

func WithBrowserImportRepository(repo browserimport.Repository) Option {
	return func(opts *options) {
		opts.importRepository = repo
	}
}

func WithMaxImportBytes(maxImportBytes int64) Option {
	return func(opts *options) {
		opts.maxImportBytes = maxImportBytes
	}
}

func WithTeamService(service *teams.Service) Option {
	return func(opts *options) {
		opts.teamService = service
	}
}

func WithKnowledgeService(service *knowledge.Service) Option {
	return func(opts *options) {
		opts.knowledgeService = service
	}
}

func WithAgentService(service *agents.Service) Option {
	return func(opts *options) {
		opts.agentService = service
	}
}

func WithPluginRegistry(registry plugins.Registry) Option {
	return func(opts *options) {
		opts.pluginRegistry = registry
	}
}

func WithPluginAuditRecorder(recorder plugins.AuditRecorder) Option {
	return func(opts *options) {
		opts.pluginAuditRecorder = recorder
	}
}

func WithImageJobService(service *imagejobs.Service) Option {
	return func(opts *options) {
		opts.imageJobService = service
	}
}

func WithVoiceJobService(service *voicejobs.Service) Option {
	return func(opts *options) {
		opts.voiceJobService = service
	}
}

func WithRAGSourceService(service *ragsource.Service) Option {
	return func(opts *options) {
		opts.ragSourceService = service
	}
}

func New(cfg config.Config, opts ...Option) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewHandler(cfg, opts...),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func NewHandler(cfg config.Config, opts ...Option) http.Handler {
	resolvedOptions := resolveOptions(opts...)
	logger := resolvedOptions.logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := resolvedOptions.metrics
	if metrics == nil {
		metrics = NewMetrics(cfg.Version, cfg.Storage.Backend)
	}
	metrics.SetReadyChecks(resolvedOptions.readyChecks)
	metrics.SetDBStatsProvider(resolvedOptions.dbStatsProvider)

	mux := http.NewServeMux()
	healthHandler := health.NewWithChecks(cfg.Version, resolvedOptions.readyChecks...)
	authHandler := auth.NewHandler(
		resolvedOptions.authService,
		auth.WithAuthRateLimitStore(resolvedOptions.rateLimitStore),
	)
	fileService := files.NewService(
		resolvedOptions.fileRepository,
		resolvedOptions.objectStore,
		files.WithStorageBackend(cfg.Storage.Backend),
	)
	chatOptions := []chat.HandlerOption{
		chat.WithProvider(resolvedOptions.chatProvider),
		chat.WithRunCancellationStore(resolvedOptions.runCancellationStore),
		chat.WithAttachmentResolver(fileProviderAttachmentResolver{
			service:  fileService,
			maxBytes: cfg.Storage.MaxUploadBytes,
		}),
	}
	if resolvedOptions.knowledgeService != nil {
		chatOptions = append(
			chatOptions,
			chat.WithRAGAnswerAssembler(
				chat.NewRAGAnswerAssembler(
					knowledgeRAGCandidateSource{service: resolvedOptions.knowledgeService},
					resolvedOptions.knowledgeService,
				),
			),
			chat.WithRAGAnswerGovernanceGate(
				chat.NewKnowledgeConsentRAGAnswerGovernanceGate(resolvedOptions.knowledgeService),
			),
		)
	}
	chatHandler := chat.NewHandler(
		chat.NewService(resolvedOptions.chatRepository),
		chatOptions...,
	)
	fileHandler := files.NewHandler(
		fileService,
		files.WithMaxUploadBytes(resolvedOptions.maxUploadBytes),
	)
	importHandler := browserimport.NewHandler(
		browserimport.NewService(
			resolvedOptions.importRepository,
			browserimport.WithMaxPackageBytes(resolvedOptions.maxImportBytes),
		),
		browserimport.WithHandlerMaxPackageBytes(resolvedOptions.maxImportBytes),
	)
	teamHandler := teams.NewHandler(resolvedOptions.teamService)
	knowledgeHandler := knowledge.NewHandler(resolvedOptions.knowledgeService)
	agentHandler := agents.NewHandler(resolvedOptions.agentService)
	codeJobHandler := codejobs.NewHandler(nil)
	imageJobHandler := imagejobs.NewHandler(resolvedOptions.imageJobService)
	jobControlHandler := jobcontrol.NewHandler(nil)
	voiceJobHandler := voicejobs.NewHandler(resolvedOptions.voiceJobService)
	ragProviderHandler := ragproviders.NewHandler(cfg.RAG)
	ragSourceHandler := ragsource.NewHandler(resolvedOptions.ragSourceService)
	runtimeConfigService := runtimeconfig.NewService(cfg)
	pluginHandler := plugins.NewHandler(plugins.NewService(
		cfg,
		plugins.WithSecretDecrypter(runtimeConfigService.DecryptOptionalSecret),
		plugins.WithRegistry(resolvedOptions.pluginRegistry),
		plugins.WithAuditRecorder(resolvedOptions.pluginAuditRecorder),
	))
	runtimeConfigHandler := runtimeconfig.NewHandler(runtimeConfigService)

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.Handle("/metrics", metrics)
	mux.HandleFunc("/v1/version", healthHandler.Version)
	mux.Handle("/v1/me", authHandler)
	mux.Handle("/v1/me/sessions", authHandler)
	mux.Handle("/v1/auth/login", authHandler)
	mux.Handle("/v1/auth/logout", authHandler)
	mux.Handle("/v1/auth/invites/accept", authHandler)
	mux.Handle("/v1/auth/recovery/request", authHandler)
	mux.Handle("/v1/auth/recovery/complete", authHandler)
	mux.Handle("/v1/config", runtimeConfigHandler)
	mux.Handle("/v1/providers/models", runtimeConfigHandler)
	mux.Handle("/v1/byok/public-key", runtimeConfigHandler)
	mux.Handle("/v1/chat/conversations", chatHandler)
	mux.Handle("/v1/chat/conversations/", chatHandler)
	mux.Handle("/v1/chat/runs/", chatHandler)
	mux.Handle("/v1/chat/tools/plan", chatHandler)
	mux.Handle("/v1/agents", agentHandler)
	mux.Handle("/v1/agents/", agentHandler)
	mux.Handle("/v1/plugins", pluginHandler)
	mux.Handle("/v1/plugins/", pluginHandler)
	mux.Handle("/v1/code/executions", codeJobHandler)
	mux.Handle("/v1/images/generations", imageJobHandler)
	mux.Handle("/v1/jobs/", jobControlHandler)
	mux.Handle("/v1/voice/transcribe", voiceJobHandler)
	mux.Handle("/v1/voice/synthesize", voiceJobHandler)
	mux.Handle("/v1/rag/provider-status", ragProviderHandler)
	mux.Handle(ragsource.InternalSourceObjectPath, ragSourceHandler)
	mux.Handle("/v1/files", fileHandler)
	mux.Handle("/v1/files/", fileHandler)
	mux.Handle("/v1/import/browser", importHandler)
	mux.Handle("/v1/import/browser/", importHandler)
	mux.Handle("/v1/teams", teamHandler)
	mux.Handle("/v1/teams/", teamHandler)
	mux.Handle("/v1/knowledge/collections", knowledgeHandler)
	mux.Handle("/v1/knowledge/collections/", knowledgeHandler)
	mux.Handle("/v1/knowledge/documents/", knowledgeHandler)
	mux.Handle("/v1/me/knowledge/query-consents", knowledgeHandler)
	mux.Handle("/v1/me/knowledge/query-consents/", knowledgeHandler)
	mux.HandleFunc("/", notFound)

	middlewares := []Middleware{
		withRequestID,
		withRequestMetrics(metrics),
		withRequestLogging(logger),
		withRecover(logger),
		withSecurityHeaders,
	}
	authRequired := cfg.Auth.RequireAuth()
	middlewares = append(
		middlewares,
		withSessionIdentity(resolvedOptions.sessionResolver, authRequired),
	)
	if cfg.Redis.RateLimitEnabled && resolvedOptions.rateLimitStore != nil {
		middlewares = append(middlewares, withRateLimit(resolvedOptions.rateLimitStore, cfg.Redis, nil))
	}

	return chain(mux, middlewares...)
}

func resolveOptions(opts ...Option) options {
	resolved := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}

	return resolved
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, ErrorResponse{
		Error: ErrorBody{
			Code:    "NOT_FOUND",
			Message: "route not found",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

func withSessionIdentity(resolver SessionResolver, requireAuth bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicWithoutAuthRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				if requireAuth || isIndependentIdentityAPIRequest(r) {
					writeAuthError(w, auth.ErrSessionNotFound)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if token == "" {
				writeAuthError(w, auth.ErrSessionNotFound)
				return
			}
			if resolver == nil {
				writeAuthError(w, auth.ErrDatabaseRequired)
				return
			}

			session, err := resolver.ResolveByTokenHash(r.Context(), auth.HashSessionToken(token))
			if err != nil {
				writeAuthError(w, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithAuthenticatedSession(r.Context(), session)))
		})
	}
}

func isIndependentIdentityAPIRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return r.URL.Path == "/v1/teams" || strings.HasPrefix(r.URL.Path, "/v1/teams/") ||
		r.URL.Path == "/v1/rag/provider-status" ||
		r.URL.Path == "/v1/knowledge/collections" ||
		strings.HasPrefix(r.URL.Path, "/v1/knowledge/collections/") ||
		strings.HasPrefix(r.URL.Path, "/v1/knowledge/documents/") ||
		r.URL.Path == "/v1/me/knowledge/query-consents" ||
		strings.HasPrefix(r.URL.Path, "/v1/me/knowledge/query-consents/")
}

func isPublicWithoutAuthRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	switch r.URL.Path {
	case "/health", "/ready", "/metrics", "/v1/version", "/v1/config", "/v1/byok/public-key":
		return r.Method == http.MethodGet
	case "/v1/agents":
		return r.Method == http.MethodGet
	case "/v1/plugins":
		return r.Method == http.MethodGet
	case "/v1/auth/login", "/v1/auth/invites/accept",
		"/v1/auth/recovery/request", "/v1/auth/recovery/complete":
		return r.Method == http.MethodPost
	case ragsource.InternalSourceObjectPath:
		return r.Method == http.MethodPost
	default:
		return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/agents/")
	}
}

func bearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", true
	}

	return parts[1], true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, auth.ErrDatabaseRequired):
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
			Error: ErrorBody{
				Code:    "DATABASE_REQUIRED",
				Message: "database is required for auth verification",
			},
		})
	case errors.Is(err, auth.ErrSessionNotFound),
		errors.Is(err, auth.ErrSessionExpired),
		errors.Is(err, auth.ErrSessionRevoked):
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: ErrorBody{
				Code:    "UNAUTHENTICATED",
				Message: "session is invalid or expired",
			},
		})
	default:
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: ErrorBody{
				Code:    "UNAUTHENTICATED",
				Message: "session could not be verified",
			},
		})
	}
}
