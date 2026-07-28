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
	"neo-chat/mm-chat/backend/internal/providerfactory"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/ragsource"
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/voicejobs"
	"neo-chat/mm-chat/backend/internal/websearch"
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
	developmentSession   *auth.Session
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
	ragQueryEmbedder     ragproviders.QueryEmbedder
	ragReranker          ragproviders.Reranker
	runtimeConfigRepo    runtimeconfig.ProviderConfigRepository
	taskModelRepo        runtimeconfig.TaskModelSettingsRepository
	userMemoryRepo       usermemory.Repository
	memoryWakePublisher  chat.MemoryWakePublisher
	providerSecretVault  *providersecrets.Vault
	webSearchResolver    websearch.Resolver
}

type ragEvidenceCandidateFetcher interface {
	FetchQueryEvidenceCandidates(context.Context, knowledge.QueryEvidenceCandidatesInput) ([]knowledge.EvidenceCandidateReference, error)
	FetchHybridQueryEvidenceCandidates(context.Context, knowledge.HybridQueryEvidenceCandidatesInput) ([]knowledge.EvidenceCandidateReference, error)
}

type ragProfiledEvidenceCandidateFetcher interface {
	ResolveActiveRetrievalProfile(context.Context) (knowledge.RetrievalProfileBinding, error)
	FetchFencedHybridQueryEvidenceCandidates(context.Context, knowledge.FencedHybridQueryEvidenceCandidatesInput) ([]knowledge.EvidenceCandidateReference, error)
	FetchFencedQueryEvidenceCandidates(context.Context, knowledge.FencedQueryEvidenceCandidatesInput) ([]knowledge.EvidenceCandidateReference, error)
}

type ragGenerationRetrievalProfileResolver interface {
	ResolveGenerationRetrievalProfile(context.Context, string) (knowledge.RetrievalProfileBinding, error)
}

type ragRetrievalProfileGatewayFactory interface {
	ForRetrievalProfile(ragproviders.RetrievalProfileID) (*ragproviders.RetrievalProfileGateway, error)
}

type knowledgeRAGCandidateSource struct {
	candidates ragEvidenceCandidateFetcher
	embedder   ragproviders.QueryEmbedder
	queryGate  chat.RAGQueryEmbeddingGovernanceGate
}

type knowledgeRAGReranker struct {
	client   ragproviders.Reranker
	profiles ragGenerationRetrievalProfileResolver
}

func (reranker knowledgeRAGReranker) Rerank(
	ctx context.Context,
	indexGenerationID string,
	query string,
	documents []string,
) ([]chat.RAGRerankResult, error) {
	if reranker.client == nil {
		return nil, ragproviders.ErrRerankUnavailable
	}
	client := reranker.client
	if factory, ok := reranker.client.(ragRetrievalProfileGatewayFactory); ok &&
		reranker.profiles != nil {
		binding, err := reranker.profiles.ResolveGenerationRetrievalProfile(
			ctx,
			indexGenerationID,
		)
		if err != nil {
			return nil, ragproviders.ErrRerankUnavailable
		}
		profileClient, err := factory.ForRetrievalProfile(
			ragproviders.RetrievalProfileID(binding.RetrievalProfileID),
		)
		if err != nil {
			return nil, ragproviders.ErrRerankUnavailable
		}
		client = profileClient
	}
	results, err := client.Rerank(ctx, query, documents)
	if err != nil {
		return nil, err
	}
	converted := make([]chat.RAGRerankResult, 0, len(results))
	for _, result := range results {
		converted = append(converted, chat.RAGRerankResult{
			Index: result.Index, RelevanceScore: result.RelevanceScore,
		})
	}
	return converted, nil
}

type fileProviderAttachmentResolver struct {
	service  *files.Service
	maxBytes int64
}

type runtimeChatProviderResolver struct {
	service *runtimeconfig.Service
	timeout time.Duration
}

type runtimeToolCapabilityCache struct {
	service *runtimeconfig.Service
}

func (cache runtimeToolCapabilityCache) LookupToolCapability(
	ctx context.Context,
	configHash string,
	modelID string,
) (chat.ToolCapabilityStatus, bool, error) {
	if cache.service == nil {
		return chat.ToolCapabilityUnknown, false, nil
	}
	entry, found, err := cache.service.LookupToolCapability(ctx, configHash, modelID)
	return chat.ToolCapabilityStatus(entry.Status), found, err
}

func (cache runtimeToolCapabilityCache) StoreToolCapability(
	ctx context.Context,
	configHash string,
	modelID string,
	status chat.ToolCapabilityStatus,
	category string,
) error {
	if cache.service == nil {
		return nil
	}
	return cache.service.StoreToolCapability(
		ctx,
		configHash,
		modelID,
		runtimeconfig.ToolCapabilityStatus(status),
		category,
	)
}

type runtimeModelBuiltInSearchTester struct {
	timeout time.Duration
}

func (t runtimeModelBuiltInSearchTester) TestModelBuiltInSearch(
	ctx context.Context,
	input runtimeconfig.ModelBuiltInSearchTestInput,
) (int, error) {
	if input.Protocol != runtimeconfig.ModelBuiltInSearchProtocolOpenAIResponses {
		return 0, runtimeconfig.ErrModelBuiltInSearchUnsupported
	}
	provider, err := chat.NewOpenAIProvider(chat.OpenAICompatibleProviderConfig{
		BaseURL: input.BaseURL, APIKey: input.APIKey,
		ProviderID: input.ProviderID, Timeout: t.timeout,
	})
	if err != nil {
		return 0, err
	}
	events, err := provider.StreamChatWithModelBuiltInSearch(ctx, chat.ProviderRequest{
		Prompt:   "Search the web for the OpenAI official documentation home page and answer briefly.",
		ModelRef: chat.ModelRef{ProviderID: input.ProviderID, ModelID: input.Model},
	})
	if err != nil {
		return 0, err
	}
	sourceCount := 0
	for event := range events {
		if event.Error != nil {
			return 0, event.Error
		}
		if event.Type == chat.ProviderEventSearch && event.Search != nil {
			sourceCount += len(event.Search.Sources)
		}
	}
	if sourceCount <= 0 {
		return 0, runtimeconfig.ErrModelBuiltInSearchTestFailed
	}
	return sourceCount, nil
}

type chatImageGenerator struct {
	service *imagejobs.Service
}

func (g chatImageGenerator) GenerateImage(
	ctx context.Context,
	request chat.ImageGenerationRequest,
) (chat.ImageGenerationResult, error) {
	if g.service == nil {
		return chat.ImageGenerationResult{}, imagejobs.ErrImageJobsUnavailable
	}
	response, err := g.service.Generate(ctx, imagejobs.GenerateRequest{
		ModelRef: imagejobs.ModelRef{
			ProviderID: request.ModelRef.ProviderID,
			ModelID:    request.ModelRef.ModelID,
		},
		Prompt: request.Prompt,
		Size:   request.Size,
		Count:  1,
	})
	if err != nil {
		slog.WarnContext(
			ctx,
			"image_generation_failed",
			slog.String("reason", imagejobs.FailureReason(err)),
		)
		if imagejobs.IsContentPolicyViolation(err) {
			return chat.ImageGenerationResult{}, &chat.ImageGenerationError{
				Code: chat.ImageContentPolicyViolationCode,
				Err:  err,
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return chat.ImageGenerationResult{}, &chat.ImageGenerationError{
				Code: chat.ImageProviderTimeoutCode,
				Err:  err,
			}
		}
		if imagejobs.IsProviderConnectionFailure(err) {
			return chat.ImageGenerationResult{}, &chat.ImageGenerationError{
				Code: chat.ImageProviderConnectionCode,
				Err:  err,
			}
		}
		return chat.ImageGenerationResult{}, err
	}
	attachments := make([]chat.GeneratedImageAttachment, 0, len(response.Images))
	for _, generated := range response.Images {
		attachments = append(attachments, chat.GeneratedImageAttachment{
			FileID:  generated.FileID,
			Purpose: generated.Purpose,
		})
	}
	return chat.ImageGenerationResult{
		Attachments: attachments,
		Message:     response.Message,
	}, nil
}

func (r runtimeChatProviderResolver) ResolveRuntimeProvider(
	ctx context.Context,
	provider runtimeconfig.ProviderRuntimeConfig,
) (chat.RuntimeProviderResolution, error) {
	if r.service == nil {
		return chat.RuntimeProviderResolution{}, chat.ValidationError{
			Code:    "PROVIDER_CONFIG_UNSUPPORTED",
			Message: "runtime provider configuration is not supported",
		}
	}
	apiKey := ""
	modelBuiltInSearchProtocol := ""
	modelBuiltInSearchTestValid := false
	toolCapabilityPolicy := ""
	toolCapabilityModelOverrides := map[string]string{}
	toolCapabilityConfigHash := ""
	source := strings.TrimSpace(provider.Source)
	if strings.TrimSpace(provider.Source) == "server-default" {
		resolved, err := r.service.ResolveServerDefaultProvider(ctx)
		if err != nil {
			return chat.RuntimeProviderResolution{}, mapRuntimeProviderError(err)
		}
		apiKey = resolved.APIKey
		provider.ID = resolved.ID
		provider.Name = resolved.Name
		provider.Type = string(resolved.Type)
		provider.BaseURL = resolved.BaseURL
		modelBuiltInSearchProtocol = resolved.ModelBuiltInSearchProtocol
		modelBuiltInSearchTestValid = resolved.ModelBuiltInSearchTestValid
		toolCapabilityPolicy = string(resolved.ToolCapabilityDefault)
		for model, value := range resolved.ToolCapabilityModelOverrides {
			toolCapabilityModelOverrides[model] = string(value)
		}
		toolCapabilityConfigHash = resolved.ToolCapabilityConfigHash
	} else if strings.TrimSpace(provider.Source) == "server-stored" {
		resolved, err := r.service.ResolveStoredProvider(ctx, provider.ID)
		if err != nil {
			return chat.RuntimeProviderResolution{}, mapRuntimeProviderError(err)
		}
		apiKey = resolved.APIKey
		provider.ID = resolved.ID
		provider.Name = resolved.Name
		provider.Type = string(resolved.Type)
		provider.BaseURL = resolved.BaseURL
		modelBuiltInSearchProtocol = resolved.ModelBuiltInSearchProtocol
		modelBuiltInSearchTestValid = resolved.ModelBuiltInSearchTestValid
		toolCapabilityPolicy = string(resolved.ToolCapabilityDefault)
		for model, value := range resolved.ToolCapabilityModelOverrides {
			toolCapabilityModelOverrides[model] = string(value)
		}
		toolCapabilityConfigHash = resolved.ToolCapabilityConfigHash
	} else {
		var err error
		apiKey, err = r.service.ProviderAPIKey(provider)
		if err != nil {
			return chat.RuntimeProviderResolution{}, mapRuntimeProviderError(err)
		}
	}
	resolved, err := providerfactory.NewChatProvider(providerfactory.ChatConfig{
		ProviderID: strings.TrimSpace(provider.ID),
		Type:       runtimeconfig.ProviderType(provider.Type),
		BaseURL:    provider.BaseURL,
		APIKey:     apiKey,
		Timeout:    r.timeout,
		UseOpenAIResponses: modelBuiltInSearchTestValid &&
			modelBuiltInSearchProtocol == runtimeconfig.ModelBuiltInSearchProtocolOpenAIResponses,
	})
	if err != nil {
		return chat.RuntimeProviderResolution{}, chat.ValidationError{
			Code:    "PROVIDER_CONFIG_UNSUPPORTED",
			Message: "runtime provider configuration is unsupported",
		}
	}
	return runtimeChatProviderResolutionWithToolCapability(
		resolved,
		source,
		provider,
		toolCapabilityPolicy,
		toolCapabilityModelOverrides,
		toolCapabilityConfigHash,
	), nil
}

func runtimeChatProviderResolution(
	provider chat.Provider,
	source string,
	config runtimeconfig.ProviderRuntimeConfig,
) chat.RuntimeProviderResolution {
	return runtimeChatProviderResolutionWithToolCapability(
		provider,
		source,
		config,
		"",
		nil,
		"",
	)
}

func runtimeChatProviderResolutionWithToolCapability(
	provider chat.Provider,
	source string,
	config runtimeconfig.ProviderRuntimeConfig,
	toolCapabilityPolicy string,
	toolCapabilityModelOverrides map[string]string,
	toolCapabilityConfigHash string,
) chat.RuntimeProviderResolution {
	processor := ""
	switch source {
	case "server-default":
		processor = knowledge.CanonicalAnswerProcessor(config.Type)
	case "server-stored":
		processor = knowledge.CanonicalAnswerProcessor(config.ID)
	}
	return chat.RuntimeProviderResolution{
		Provider:                     provider,
		RAGAnswerProcessor:           processor,
		ToolCapabilityPolicy:         toolCapabilityPolicy,
		ToolCapabilityModelOverrides: toolCapabilityModelOverrides,
		ToolCapabilityConfigHash:     toolCapabilityConfigHash,
	}
}

func mapRuntimeProviderError(err error) error {
	if errors.Is(err, runtimeconfig.ErrPlaintextProviderSecret) {
		return chat.ValidationError{
			Code:    "PLAINTEXT_PROVIDER_SECRET_REJECTED",
			Message: "plaintext provider secrets are not accepted",
		}
	}
	if errors.Is(err, runtimeconfig.ErrProviderSecretRequired) ||
		errors.Is(err, runtimeconfig.ErrBYOKNotConfigured) {
		return chat.ValidationError{
			Code:    "PROVIDER_SECRET_REQUIRED",
			Message: "provider API key is required",
		}
	}
	if errors.Is(err, runtimeconfig.ErrProviderSecretVaultUnavailable) ||
		errors.Is(err, runtimeconfig.ErrProviderSecretInvalid) {
		return chat.ValidationError{
			Code:    "PROVIDER_SECRET_UNAVAILABLE",
			Message: "stored provider secret is unavailable",
		}
	}
	if errors.Is(err, runtimeconfig.ErrProviderDisabled) {
		return chat.ValidationError{
			Code:    "PROVIDER_DISABLED",
			Message: "provider is disabled",
		}
	}
	if errors.Is(err, runtimeconfig.ErrProviderActivationRequired) {
		return chat.ValidationError{
			Code:    "PROVIDER_ACTIVATION_REQUIRED",
			Message: "provider must pass connection testing before activation",
		}
	}
	return err
}

func (r fileProviderAttachmentResolver) ResolveProviderAttachment(
	ctx context.Context,
	attachment chat.Attachment,
) (chat.ProviderAttachment, error) {
	if r.service == nil {
		return chat.ProviderAttachment{}, chat.ValidationError{
			Code:    "ATTACHMENT_CONTENT_UNAVAILABLE",
			Message: "attachment content is not available for provider streaming",
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
	resolvedMIMEType := strings.TrimSpace(record.MimeType)
	if resolvedMIMEType == "" {
		resolvedMIMEType = attachment.MimeType
	}
	if !strings.HasPrefix(strings.ToLower(resolvedMIMEType), "image/") &&
		maxBytes > chat.MaxDirectAttachmentBytes {
		maxBytes = chat.MaxDirectAttachmentBytes
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return chat.ProviderAttachment{}, err
	}
	if int64(len(data)) > maxBytes {
		return chat.ProviderAttachment{}, chat.ValidationError{
			Code:    "ATTACHMENT_TOO_LARGE",
			Message: "attachment is too large for direct provider context",
		}
	}

	mimeType := resolvedMIMEType
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
	if source.candidates == nil {
		return nil, knowledge.ErrDatabaseRequired
	}
	if profiled, ok := source.candidates.(ragProfiledEvidenceCandidateFetcher); ok {
		if factory, ok := source.embedder.(ragRetrievalProfileGatewayFactory); ok {
			for attempt := 0; attempt < 2; attempt++ {
				binding, err := profiled.ResolveActiveRetrievalProfile(ctx)
				if errors.Is(err, knowledge.ErrActiveRetrievalProfileUnavailable) {
					return source.fetchLegacyEvidenceCandidates(ctx, query)
				}
				if err != nil {
					return nil, err
				}
				if source.queryGate == nil ||
					source.queryGate.AuthorizeRAGQueryEmbedding(ctx, binding) != nil {
					candidates, lexicalErr := source.fetchFencedLexical(
						ctx,
						profiled,
						binding,
						query,
					)
					if errors.Is(lexicalErr, knowledge.ErrRetrievalProfileChanged) {
						continue
					}
					return candidates, lexicalErr
				}
				profileGateway, err := factory.ForRetrievalProfile(
					ragproviders.RetrievalProfileID(binding.RetrievalProfileID),
				)
				if err != nil {
					candidates, lexicalErr := source.fetchFencedLexical(
						ctx,
						profiled,
						binding,
						query,
					)
					if errors.Is(lexicalErr, knowledge.ErrRetrievalProfileChanged) {
						continue
					}
					return candidates, lexicalErr
				}
				embedding, err := profileGateway.EmbedQuery(ctx, query.QueryText)
				if err != nil {
					candidates, lexicalErr := source.fetchFencedLexical(
						ctx,
						profiled,
						binding,
						query,
					)
					if errors.Is(lexicalErr, knowledge.ErrRetrievalProfileChanged) {
						continue
					}
					return candidates, lexicalErr
				}
				candidates, err := profiled.FetchFencedHybridQueryEvidenceCandidates(
					ctx,
					knowledge.FencedHybridQueryEvidenceCandidatesInput{
						Binding: binding, CollectionIDs: query.CollectionIDs,
						QueryText: query.QueryText, QueryEmbedding: embedding.Vector,
						Limit: query.Limit,
					},
				)
				if errors.Is(err, knowledge.ErrRetrievalProfileChanged) {
					continue
				}
				return candidates, err
			}
			return nil, knowledge.ErrRetrievalProfileChanged
		}
	}
	return source.fetchLegacyEvidenceCandidates(ctx, query)
}

func (source knowledgeRAGCandidateSource) fetchFencedLexical(
	ctx context.Context,
	profiled ragProfiledEvidenceCandidateFetcher,
	binding knowledge.RetrievalProfileBinding,
	query chat.RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	return profiled.FetchFencedQueryEvidenceCandidates(
		ctx,
		knowledge.FencedQueryEvidenceCandidatesInput{
			Binding:       binding,
			CollectionIDs: query.CollectionIDs,
			QueryText:     query.QueryText,
			Limit:         query.Limit,
		},
	)
}

func (source knowledgeRAGCandidateSource) fetchLegacyEvidenceCandidates(
	ctx context.Context,
	query chat.RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	if source.embedder != nil {
		embedding, err := source.embedder.EmbedQuery(ctx, query.QueryText)
		if err == nil {
			return source.candidates.FetchHybridQueryEvidenceCandidates(
				ctx,
				knowledge.HybridQueryEvidenceCandidatesInput{
					CollectionIDs:  query.CollectionIDs,
					QueryText:      query.QueryText,
					QueryEmbedding: embedding.Vector,
					Limit:          query.Limit,
				},
			)
		}
	}
	return source.candidates.FetchQueryEvidenceCandidates(ctx, knowledge.QueryEvidenceCandidatesInput{
		CollectionIDs: query.CollectionIDs,
		QueryText:     query.QueryText,
		Limit:         query.Limit,
	})
}

func WithRuntimeConfigRepository(repo runtimeconfig.ProviderConfigRepository) Option {
	return func(opts *options) {
		opts.runtimeConfigRepo = repo
	}
}

func WithTaskModelSettingsRepository(repo runtimeconfig.TaskModelSettingsRepository) Option {
	return func(opts *options) {
		opts.taskModelRepo = repo
	}
}

func WithUserMemoryRepository(repo usermemory.Repository) Option {
	return func(opts *options) {
		opts.userMemoryRepo = repo
	}
}

func WithMemoryWakePublisher(publisher chat.MemoryWakePublisher) Option {
	return func(opts *options) {
		opts.memoryWakePublisher = publisher
	}
}

func WithProviderSecretVault(vault *providersecrets.Vault) Option {
	return func(opts *options) {
		opts.providerSecretVault = vault
	}
}

func WithWebSearchResolver(resolver websearch.Resolver) Option {
	return func(opts *options) {
		opts.webSearchResolver = resolver
	}
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

func WithDevelopmentSession(session auth.Session) Option {
	return func(opts *options) {
		copy := session
		opts.developmentSession = &copy
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

func WithRAGQueryEmbedder(embedder ragproviders.QueryEmbedder) Option {
	return func(opts *options) {
		opts.ragQueryEmbedder = embedder
	}
}

func WithRAGReranker(reranker ragproviders.Reranker) Option {
	return func(opts *options) {
		opts.ragReranker = reranker
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
	webSearchService := websearch.NewService(resolvedOptions.webSearchResolver)
	var chatHandler *chat.Handler
	runtimeConfigService := runtimeconfig.NewService(
		cfg,
		runtimeconfig.WithProviderConfigRepository(resolvedOptions.runtimeConfigRepo),
		runtimeconfig.WithTaskModelSettingsRepository(resolvedOptions.taskModelRepo),
		runtimeconfig.WithToolCapabilityWarmupScheduler(func(
			ctx context.Context,
			request runtimeconfig.ToolCapabilityWarmupRequest,
		) {
			if chatHandler != nil {
				chatHandler.PrewarmToolCapabilities(ctx, request)
			}
		}),
		runtimeconfig.WithProviderSecretVault(resolvedOptions.providerSecretVault),
		runtimeconfig.WithModelBuiltInSearchTester(runtimeModelBuiltInSearchTester{
			timeout: cfg.Provider.Timeout,
		}),
		runtimeconfig.WithSearchAvailability(func(ctx context.Context) bool {
			_, err := webSearchService.ResolveActive(ctx)
			return err == nil
		}),
	)
	userMemoryService := usermemory.NewService(resolvedOptions.userMemoryRepo)
	chatOptions := []chat.HandlerOption{
		chat.WithProvider(resolvedOptions.chatProvider),
		chat.WithRunCancellationStore(resolvedOptions.runCancellationStore),
		chat.WithAttachmentResolver(fileProviderAttachmentResolver{
			service:  fileService,
			maxBytes: cfg.Storage.MaxUploadBytes,
		}),
		chat.WithRuntimeProviderResolver(runtimeChatProviderResolver{
			service: runtimeConfigService,
			timeout: cfg.Provider.Timeout,
		}),
		chat.WithToolCapabilityCache(runtimeToolCapabilityCache{
			service: runtimeConfigService,
		}),
		chat.WithImageGenerator(chatImageGenerator{
			service: resolvedOptions.imageJobService,
		}),
		chat.WithUserMemoryService(userMemoryService),
		chat.WithMemoryWakePublisher(resolvedOptions.memoryWakePublisher),
	}
	if webSearchService.Configured() {
		chatOptions = append(chatOptions, chat.WithWebSearchService(webSearchService))
	}
	if resolvedOptions.knowledgeService != nil {
		assemblerOptions := make([]chat.RAGAnswerAssemblerOption, 0, 1)
		if resolvedOptions.ragReranker != nil {
			assemblerOptions = append(
				assemblerOptions,
				chat.WithRAGEvidenceReranker(
					knowledgeRAGReranker{
						client:   resolvedOptions.ragReranker,
						profiles: resolvedOptions.knowledgeService,
					},
					chat.NewKnowledgeConsentRAGRerankGovernanceGate(
						resolvedOptions.knowledgeService,
					),
				),
			)
		}
		chatOptions = append(
			chatOptions,
			chat.WithRAGAnswerAssembler(
				chat.NewRAGAnswerAssembler(
					knowledgeRAGCandidateSource{
						candidates: resolvedOptions.knowledgeService,
						embedder:   resolvedOptions.ragQueryEmbedder,
						queryGate: chat.NewKnowledgeConsentRAGQueryEmbeddingGovernanceGate(
							resolvedOptions.knowledgeService,
						),
					},
					resolvedOptions.knowledgeService,
					assemblerOptions...,
				),
			),
			chat.WithRAGAnswerGovernanceGate(
				chat.NewKnowledgeConsentRAGAnswerGovernanceGate(resolvedOptions.knowledgeService),
			),
			chat.WithKnowledgeRoutingCatalog(
				resolvedOptions.knowledgeService,
				chat.NewKnowledgeConsentRoutingCatalogGovernanceGate(
					resolvedOptions.knowledgeService,
				),
			),
		)
	}
	chatHandler = chat.NewHandler(
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
	ragProviderHandler := ragproviders.NewHandler(runtimeConfigService.RAGProviderStatus)
	providerGatewayHandler := ragproviders.NewProviderGatewayHandler(
		cfg.RAG.SourceGatewayToken,
		ragproviders.NewProviderGateway(runtimeConfigService),
	)
	ragSourceHandler := ragsource.NewHandler(resolvedOptions.ragSourceService)
	pluginHandler := plugins.NewHandler(plugins.NewService(
		cfg,
		plugins.WithSecretDecrypter(runtimeConfigService.DecryptOptionalSecret),
		plugins.WithRegistry(resolvedOptions.pluginRegistry),
		plugins.WithAuditRecorder(resolvedOptions.pluginAuditRecorder),
	))
	runtimeConfigHandler := runtimeconfig.NewHandler(runtimeConfigService)
	webSearchHandler := websearch.NewHandler(webSearchService)
	userMemoryHandler := usermemory.NewHandler(userMemoryService)

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
	mux.Handle("/v1/admin/provider-config", runtimeConfigHandler)
	mux.Handle("/v1/admin/providers", runtimeConfigHandler)
	mux.Handle("/v1/admin/providers/", runtimeConfigHandler)
	mux.Handle("/v1/admin/task-models", runtimeConfigHandler)
	mux.Handle("/v1/admin/search/providers", runtimeConfigHandler)
	mux.Handle("/v1/admin/search/providers/", runtimeConfigHandler)
	mux.Handle("/v1/admin/voice/providers", runtimeConfigHandler)
	mux.Handle("/v1/admin/voice/providers/", runtimeConfigHandler)
	mux.Handle("/v1/admin/rag/providers", runtimeConfigHandler)
	mux.Handle("/v1/admin/rag/providers/", runtimeConfigHandler)
	mux.Handle("/v1/byok/public-key", runtimeConfigHandler)
	mux.Handle(websearch.SearchPath, webSearchHandler)
	mux.Handle("/v1/chat/conversations", chatHandler)
	mux.Handle("/v1/chat/conversations/", chatHandler)
	mux.Handle("/v1/chat/generate", chatHandler)
	mux.Handle("/v1/chat/runs/", chatHandler)
	mux.Handle("/v1/chat/tools/plan", chatHandler)
	mux.Handle("/v1/memories", userMemoryHandler)
	mux.Handle("/v1/memories/", userMemoryHandler)
	mux.Handle("/v1/memory-settings", userMemoryHandler)
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
	mux.Handle(ragproviders.InternalProviderPathPrefix, providerGatewayHandler)
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
		withSessionIdentity(
			resolvedOptions.sessionResolver,
			resolvedOptions.developmentSession,
			authRequired,
		),
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

func withSessionIdentity(
	resolver SessionResolver,
	developmentSession *auth.Session,
	requireAuth bool,
) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicWithoutAuthRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			// Development mode is the independent single-user deployment mode.
			// Never let a stale browser session switch or block its fixed owner;
			// inject the owner explicitly for services that reject implicit
			// repository fallbacks.
			if !requireAuth {
				developmentContext := auth.WithUser(r.Context(), auth.DevelopmentUser())
				if developmentSession != nil {
					developmentContext = auth.WithAuthenticatedSession(
						r.Context(),
						*developmentSession,
					)
				}
				next.ServeHTTP(w, r.WithContext(developmentContext))
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeAuthError(w, auth.ErrSessionNotFound)
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
		if r.Method == http.MethodPost &&
			strings.HasPrefix(r.URL.Path, ragproviders.InternalProviderPathPrefix) {
			return true
		}
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
