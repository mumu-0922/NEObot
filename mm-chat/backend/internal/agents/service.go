package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultRegistryBaseURL = "https://registry.npmmirror.com/@lobehub/agents-index/v1/files/public"
	maxListResponseBytes   = int64(5 << 20)
	maxDetailResponseBytes = int64(2 << 20)

	maxAgents                = 500
	maxAgentIdentifierChars  = 120
	maxAgentTitleChars       = 160
	maxAgentDescriptionChars = 1000
	maxAgentAvatarChars      = 4096
	maxAgentCategoryChars    = 80
	maxAgentAuthorChars      = 120
	maxAgentHomepageChars    = 4096
	maxAgentCreatedAtChars   = 80
	maxAgentTags             = 12
	maxAgentTagChars         = 60
	maxAgentSystemRoleChars  = 200000
)

var (
	agentIdentifierRE       = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	ErrRegistryUnavailable  = errors.New("agent registry unavailable")
	ErrInvalidRegistryEntry = errors.New("invalid agent registry entry")
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Service struct {
	registryBaseURL string
	httpClient      HTTPClient
}

type ServiceOption func(*Service)

func WithRegistryBaseURL(baseURL string) ServiceOption {
	return func(service *Service) {
		baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if baseURL != "" {
			service.registryBaseURL = baseURL
		}
	}
}

func WithHTTPClient(client HTTPClient) ServiceOption {
	return func(service *Service) {
		if client != nil {
			service.httpClient = client
		}
	}
}

func NewService(opts ...ServiceOption) *Service {
	service := &Service{
		registryBaseURL: DefaultRegistryBaseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (service *Service) ListAgents(ctx context.Context, locale Locale) ([]Agent, error) {
	if service == nil {
		service = NewService()
	}
	var payload struct {
		Agents []any `json:"agents"`
	}
	if err := service.fetchJSON(ctx, indexFile(locale), maxListResponseBytes, &payload); err != nil {
		return nil, err
	}
	return normalizeMarketAgents(payload.Agents), nil
}

func (service *Service) GetAgentDetail(ctx context.Context, identifier string, locale Locale) (Agent, error) {
	if service == nil {
		service = NewService()
	}
	identifier = strings.TrimSpace(identifier)
	if !validIdentifier(identifier) {
		return Agent{}, validationError("INVALID_AGENT_IDENTIFIER", "agent identifier is invalid")
	}

	var payload any
	if err := service.fetchJSON(ctx, detailFile(identifier, locale), maxDetailResponseBytes, &payload); err != nil {
		return Agent{}, err
	}
	agent, ok := normalizeAgentDetail(payload, identifier)
	if !ok {
		return Agent{}, ErrInvalidRegistryEntry
	}
	return agent, nil
}

func (service *Service) fetchJSON(ctx context.Context, file string, maxBytes int64, destination any) error {
	url := service.registryBaseURL + "/" + file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	response, err := service.httpClient.Do(req)
	if err != nil {
		return ErrRegistryUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return ErrRegistryUnavailable
	}

	reader := io.LimitReader(response.Body, maxBytes+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		return ErrRegistryUnavailable
	}
	if int64(len(payload)) > maxBytes {
		return ErrRegistryUnavailable
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return ErrInvalidRegistryEntry
	}
	return nil
}

func NormalizeLocale(value string) Locale {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "zh" || strings.HasPrefix(normalized, "zh-") {
		return LocaleChinese
	}
	if normalized == "ja" || strings.HasPrefix(normalized, "ja-") {
		return LocaleJapanese
	}
	return LocaleEnglish
}

func indexFile(locale Locale) string {
	switch locale {
	case LocaleChinese:
		return "index.zh-CN.json"
	case LocaleJapanese:
		return "index.ja-JP.json"
	default:
		return "index.json"
	}
}

func detailFile(identifier string, locale Locale) string {
	switch locale {
	case LocaleChinese:
		return identifier + ".zh-CN.json"
	case LocaleJapanese:
		return identifier + ".ja-JP.json"
	default:
		return identifier + ".json"
	}
}

func normalizeMarketAgents(values []any) []Agent {
	agents := make([]Agent, 0, min(len(values), maxAgents))
	seen := map[string]struct{}{}
	for _, value := range values {
		agent, ok := normalizeMarketAgent(value)
		if !ok {
			continue
		}
		if _, exists := seen[agent.Identifier]; exists {
			continue
		}
		agents = append(agents, agent)
		seen[agent.Identifier] = struct{}{}
		if len(agents) >= maxAgents {
			break
		}
	}
	if agents == nil {
		return []Agent{}
	}
	return agents
}

func normalizeMarketAgent(value any) (Agent, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return Agent{}, false
	}
	meta, _ := raw["meta"].(map[string]any)
	identifier := trimString(raw["identifier"], maxAgentIdentifierChars)
	if !validIdentifier(identifier) {
		return Agent{}, false
	}
	title := trimString(meta["title"], maxAgentTitleChars)
	if title == "" {
		title = identifier
	}
	category := trimString(meta["category"], maxAgentCategoryChars)
	if category == "" {
		category = "General"
	}
	avatar := trimString(meta["avatar"], maxAgentAvatarChars)
	if avatar == "" {
		avatar = "🤖"
	}

	return Agent{
		Identifier: identifier,
		Meta: AgentMeta{
			Avatar:      avatar,
			Description: trimString(meta["description"], maxAgentDescriptionChars),
			Tags:        normalizeTags(meta["tags"]),
			Title:       title,
			Category:    category,
		},
		CreatedAt: trimString(raw["createdAt"], maxAgentCreatedAtChars),
		Homepage:  trimString(raw["homepage"], maxAgentHomepageChars),
		Author:    trimString(raw["author"], maxAgentAuthorChars),
	}, true
}

func normalizeAgentDetail(value any, identifier string) (Agent, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return Agent{}, false
	}
	meta, _ := raw["meta"].(map[string]any)
	config, _ := raw["config"].(map[string]any)
	systemRole := trimString(config["systemRole"], maxAgentSystemRoleChars)
	if systemRole == "" {
		systemRole = trimString(meta["systemRole"], maxAgentSystemRoleChars)
	}

	withIdentifier := cloneMap(raw)
	withIdentifier["identifier"] = identifier
	metaCopy := cloneMap(meta)
	if systemRole != "" {
		metaCopy["systemRole"] = systemRole
	}
	withIdentifier["meta"] = metaCopy
	agent, ok := normalizeMarketAgent(withIdentifier)
	if !ok {
		return Agent{}, false
	}
	if systemRole != "" {
		agent.Meta.SystemRole = systemRole
		agent.Config = &AgentConfig{SystemRole: systemRole}
	}
	return agent, true
}

func normalizeTags(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return []string{}
	}
	tags := make([]string, 0, min(len(values), maxAgentTags))
	seen := map[string]struct{}{}
	for _, value := range values {
		tag := trimString(value, maxAgentTagChars)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		tags = append(tags, tag)
		seen[key] = struct{}{}
		if len(tags) >= maxAgentTags {
			break
		}
	}
	if tags == nil {
		return []string{}
	}
	return tags
}

func trimString(value any, maxRunes int) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes])
	}
	return text
}

func validIdentifier(identifier string) bool {
	return identifier != "" && len([]rune(identifier)) <= maxAgentIdentifierChars && agentIdentifierRE.MatchString(identifier)
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

type ValidationError struct {
	Code    string
	Message string
}

func (err ValidationError) Error() string { return err.Message }

func validationError(code string, message string) error {
	return ValidationError{Code: code, Message: message}
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func registryErrorMessage(err error) string {
	if errors.Is(err, ErrInvalidRegistryEntry) {
		return "agent registry response is invalid"
	}
	return "agent registry is unavailable"
}
