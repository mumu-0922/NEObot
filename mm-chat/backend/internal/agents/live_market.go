package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultLiveMarketBaseURL = "https://lobehub.com"
	liveMarketPageSize       = 21
	maxLiveMarketBytes       = int64(2 << 20)
	maxLiveMarketValues      = 20000
)

var officialAssistantCategoryIDs = []string{
	"academic",
	"career",
	"copywriting",
	"design",
	"education",
	"emotions",
	"entertainment",
	"games",
	"general",
	"life",
	"marketing",
	"office",
	"programming",
	"translation",
}

func (service *Service) SearchMarket(
	ctx context.Context,
	userID string,
	input MarketSearchInput,
) (MarketSearchResult, error) {
	if service == nil || service.repository == nil {
		return MarketSearchResult{}, ErrRepositoryUnavailable
	}
	input.Query = strings.TrimSpace(input.Query)
	input.Category = strings.TrimSpace(input.Category)
	if input.Category == "all" {
		input.Category = ""
	}
	if input.Page < 1 || input.Page > 1000 || input.PageSize != 20 ||
		!validSearchText(input.Query, 200) || !validSearchText(input.Category, 80) {
		return MarketSearchResult{}, validationError("INVALID_ASSISTANT_SEARCH", "assistant search is invalid")
	}
	if !service.IsAdministrator(userID) {
		result, err := service.repository.ListAdmissions(ctx, input)
		if err != nil {
			return MarketSearchResult{}, err
		}
		service.markInstalled(ctx, userID, result.Agents)
		return result, nil
	}

	result, err := service.searchOfficialMarket(ctx, input)
	if err != nil {
		result, err = service.searchLiveMarket(ctx, input)
	}
	if err != nil {
		result, err = service.searchLegacyMarket(ctx, input)
		if err != nil {
			return MarketSearchResult{Agents: []Agent{}, Categories: []MarketCategory{},
				Page: input.Page, PageSize: input.PageSize, Source: "unavailable", Unavailable: true,
				CanReview: true}, nil
		}
	}
	result.CanReview = true
	for index := range result.Agents {
		admission, admissionErr := service.repository.GetAdmission(ctx, result.Agents[index].Identifier)
		result.Agents[index].Admitted = admissionErr == nil && admission.Status == AdmissionAdmitted &&
			admission.ContentFingerprint == result.Agents[index].Fingerprint
	}
	service.markInstalled(ctx, userID, result.Agents)
	return result, nil
}

func (service *Service) MarketDetail(
	ctx context.Context,
	userID, identifier string,
	locale Locale,
) (Agent, error) {
	if service == nil || service.repository == nil {
		return Agent{}, ErrRepositoryUnavailable
	}
	identifier = strings.TrimSpace(identifier)
	if !validIdentifier(identifier) {
		return Agent{}, validationError("INVALID_AGENT_IDENTIFIER", "agent identifier is invalid")
	}
	if !service.IsAdministrator(userID) {
		admission, err := service.repository.GetAdmission(ctx, identifier)
		if err != nil || admission.Status != AdmissionAdmitted {
			return Agent{}, ErrAdmissionNotFound
		}
		agent := agentFromSnapshot(admission.Snapshot, true)
		service.markInstalled(ctx, userID, []Agent{agent})
		entries, _ := service.repository.ListLibrary(ctx, userID)
		markAgentInstalled(&agent, entries)
		return agent, nil
	}

	agent, err := service.getMarketAgentDetail(ctx, identifier, locale)
	if err != nil {
		return Agent{}, err
	}
	admission, admissionErr := service.repository.GetAdmission(ctx, identifier)
	agent.Admitted = admissionErr == nil && admission.Status == AdmissionAdmitted &&
		admission.ContentFingerprint == agent.Fingerprint
	entries, _ := service.repository.ListLibrary(ctx, userID)
	markAgentInstalled(&agent, entries)
	return agent, nil
}

func (service *Service) markInstalled(ctx context.Context, userID string, agents []Agent) {
	entries, err := service.repository.ListLibrary(ctx, userID)
	if err != nil {
		return
	}
	for index := range agents {
		markAgentInstalled(&agents[index], entries)
	}
}

func markAgentInstalled(agent *Agent, entries []LibraryEntry) {
	if agent == nil {
		return
	}
	for _, entry := range entries {
		if entry.Source == SourceLobeHub && entry.SourceIdentifier == agent.Identifier {
			agent.Installed = true
			agent.LibraryID = entry.ID
			return
		}
	}
}

func (service *Service) searchLiveMarket(ctx context.Context, input MarketSearchInput) (MarketSearchResult, error) {
	logicalStart := (input.Page - 1) * input.PageSize
	firstUpstreamPage := logicalStart/liveMarketPageSize + 1
	firstOffset := logicalStart % liveMarketPageSize
	needed := input.PageSize
	agents := make([]Agent, 0, input.PageSize)
	var first liveMarketPayload
	for upstreamPage := firstUpstreamPage; needed > 0 && upstreamPage <= firstUpstreamPage+1; upstreamPage++ {
		payload, err := service.fetchLiveMarketPage(ctx, input, upstreamPage)
		if err != nil {
			return MarketSearchResult{}, err
		}
		if upstreamPage == firstUpstreamPage {
			first = payload
		}
		start := 0
		if upstreamPage == firstUpstreamPage {
			start = firstOffset
		}
		if start >= len(payload.Items) {
			break
		}
		end := min(len(payload.Items), start+needed)
		agents = append(agents, payload.Items[start:end]...)
		needed -= end - start
		if end < len(payload.Items) || len(payload.Items) < liveMarketPageSize {
			break
		}
	}
	return MarketSearchResult{Agents: agents, Categories: first.Categories,
		Page: input.Page, PageSize: input.PageSize, TotalCount: first.TotalCount,
		TotalPages:       pageCount(first.TotalCount, input.PageSize),
		TotalMarketCount: first.TotalMarketCount, Source: "lobehub-live"}, nil
}

func (service *Service) searchOfficialMarket(
	ctx context.Context,
	input MarketSearchInput,
) (MarketSearchResult, error) {
	if service == nil || service.officialMarket == nil {
		return MarketSearchResult{}, ErrMarketUnavailable
	}
	query := url.Values{}
	query.Set("locale", officialMarketLocale(input.Locale))
	query.Set("order", "desc")
	query.Set("page", strconv.Itoa(input.Page))
	query.Set("pageSize", strconv.Itoa(input.PageSize))
	query.Set("sort", "recommended")
	query.Set("status", "published")
	query.Set("visibility", "public")
	if input.Query != "" {
		query.Set("q", input.Query)
	}
	if input.Category != "" {
		if !officialAssistantCategory(input.Category) {
			return MarketSearchResult{
				Agents: []Agent{}, Categories: officialAssistantCategories(),
				Page: input.Page, PageSize: input.PageSize, Source: "lobehub-market",
			}, nil
		}
		query.Set("category", input.Category)
	}
	data, err := service.officialMarket.FetchAgentMarketJSON(
		ctx,
		"/api/v1/agents?"+query.Encode(),
		maxLiveMarketBytes,
	)
	if err != nil {
		return MarketSearchResult{}, ErrMarketUnavailable
	}
	var payload struct {
		Items       []any `json:"items"`
		CurrentPage int   `json:"currentPage"`
		PageSize    int   `json:"pageSize"`
		TotalCount  int   `json:"totalCount"`
		TotalPages  int   `json:"totalPages"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.CurrentPage != input.Page ||
		payload.PageSize != input.PageSize || payload.TotalCount < 0 || payload.TotalPages < 0 ||
		len(payload.Items) > input.PageSize {
		return MarketSearchResult{}, ErrInvalidRegistryEntry
	}
	agents := make([]Agent, 0, len(payload.Items))
	for _, raw := range payload.Items {
		agent, valid := normalizeLiveAgent(raw)
		if !valid {
			continue
		}
		_ = service.applyLiveAgentProvenance(&agent, input.Locale)
		agents = append(agents, agent)
	}
	return MarketSearchResult{
		Agents:     agents,
		Categories: officialAssistantCategories(),
		Page:       payload.CurrentPage,
		PageSize:   payload.PageSize,
		TotalCount: boundedCount(payload.TotalCount),
		TotalPages: boundedCount(payload.TotalPages),
		Source:     "lobehub-market",
	}, nil
}

func (service *Service) fetchLiveMarketPage(
	ctx context.Context,
	input MarketSearchInput,
	upstreamPage int,
) (liveMarketPayload, error) {
	values := url.Values{}
	values.Set("page", strconv.Itoa(upstreamPage))
	if input.Query != "" {
		values.Set("q", input.Query)
	}
	if input.Category != "" {
		values.Set("category", input.Category)
	}
	endpoint := service.liveMarketBaseURL + liveMarketPath(input.Locale, "agent.data") + "?" + values.Encode()
	var serialized []any
	if err := service.fetchLiveJSON(ctx, endpoint, &serialized); err != nil {
		return liveMarketPayload{}, err
	}
	decoded, err := decodeLivePayload(serialized)
	if err != nil {
		return liveMarketPayload{}, err
	}
	payload, err := normalizeLiveMarketPayload(decoded)
	if err != nil {
		return liveMarketPayload{}, err
	}
	for index := range payload.Items {
		_ = service.applyLiveAgentProvenance(&payload.Items[index], input.Locale)
	}
	return payload, nil
}

func (service *Service) getLiveAgentDetail(ctx context.Context, identifier string, locale Locale) (Agent, error) {
	identifier = strings.TrimSpace(identifier)
	if !validIdentifier(identifier) {
		return Agent{}, validationError("INVALID_AGENT_IDENTIFIER", "agent identifier is invalid")
	}
	endpoint := service.liveMarketBaseURL + liveMarketPath(locale, "agent/"+url.PathEscape(identifier)+".data")
	var serialized []any
	if err := service.fetchLiveJSON(ctx, endpoint, &serialized); err != nil {
		return Agent{}, err
	}
	decoded, err := decodeLivePayload(serialized)
	if err != nil {
		return Agent{}, err
	}
	detail, ok := objectField(decoded, "detail")
	if !ok {
		return Agent{}, ErrInvalidRegistryEntry
	}
	agent, ok := normalizeLiveAgent(detail)
	if !ok || agent.Identifier != identifier {
		return Agent{}, ErrInvalidRegistryEntry
	}
	if err := service.applyLiveAgentProvenance(&agent, locale); err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (service *Service) applyLiveAgentProvenance(agent *Agent, locale Locale) error {
	if agent == nil {
		return ErrInvalidRegistryEntry
	}
	agent.Homepage = service.liveMarketBaseURL + liveMarketPath(
		locale,
		"agent/"+url.PathEscape(agent.Identifier),
	)
	snapshot, err := snapshotFromAgent(*agent)
	if err != nil {
		agent.Fingerprint = ""
		return err
	}
	agent.Fingerprint = snapshot.ContentFingerprint
	return nil
}

func (service *Service) getMarketAgentDetail(ctx context.Context, identifier string, locale Locale) (Agent, error) {
	agent, err := service.getOfficialAgentDetail(ctx, identifier, locale)
	if err == nil {
		return agent, nil
	}
	agent, err = service.getLiveAgentDetail(ctx, identifier, locale)
	if err == nil {
		return agent, nil
	}
	agent, err = service.GetAgentDetail(ctx, identifier, locale)
	if err != nil {
		return Agent{}, err
	}
	agent.Version = "legacy"
	snapshot, err := snapshotFromAgent(agent)
	if err != nil {
		return Agent{}, err
	}
	agent.Fingerprint = snapshot.ContentFingerprint
	return agent, nil
}

func (service *Service) getOfficialAgentDetail(
	ctx context.Context,
	identifier string,
	locale Locale,
) (Agent, error) {
	identifier = strings.TrimSpace(identifier)
	if service == nil || service.officialMarket == nil {
		return Agent{}, ErrMarketUnavailable
	}
	if !validIdentifier(identifier) {
		return Agent{}, validationError("INVALID_AGENT_IDENTIFIER", "agent identifier is invalid")
	}
	path := "/api/v1/agents/detail/" + url.PathEscape(identifier) +
		"?locale=" + url.QueryEscape(officialMarketLocale(locale))
	data, err := service.officialMarket.FetchAgentMarketJSON(ctx, path, maxLiveMarketBytes)
	if err != nil {
		return Agent{}, ErrMarketUnavailable
	}
	var raw any
	if json.Unmarshal(data, &raw) != nil {
		return Agent{}, ErrInvalidRegistryEntry
	}
	agent, ok := normalizeLiveAgent(raw)
	if !ok || agent.Identifier != identifier {
		return Agent{}, ErrInvalidRegistryEntry
	}
	if err := service.applyLiveAgentProvenance(&agent, locale); err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func officialAssistantCategories() []MarketCategory {
	categories := make([]MarketCategory, 0, len(officialAssistantCategoryIDs))
	for _, id := range officialAssistantCategoryIDs {
		categories = append(categories, MarketCategory{ID: id})
	}
	return categories
}

func officialAssistantCategory(candidate string) bool {
	for _, id := range officialAssistantCategoryIDs {
		if candidate == id {
			return true
		}
	}
	return false
}

func officialMarketLocale(locale Locale) string {
	switch locale {
	case LocaleChinese:
		return "zh-CN"
	case LocaleJapanese:
		return "ja-JP"
	default:
		return "en-US"
	}
}

func (service *Service) fetchLiveJSON(ctx context.Context, endpoint string, destination any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ErrMarketUnavailable
	}
	req.Header.Set("Accept", "text/x-script, application/json")
	req.Header.Set("User-Agent", "neo-chat-assistant-market/1")
	response, err := service.httpClient.Do(req)
	if err != nil {
		return ErrMarketUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return ErrMarketUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxLiveMarketBytes+1))
	if err != nil || int64(len(payload)) > maxLiveMarketBytes {
		return ErrMarketUnavailable
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return ErrInvalidRegistryEntry
	}
	return nil
}

func (service *Service) searchLegacyMarket(ctx context.Context, input MarketSearchInput) (MarketSearchResult, error) {
	agents, err := service.ListAgents(ctx, input.Locale)
	if err != nil {
		return MarketSearchResult{}, err
	}
	filtered := make([]Agent, 0, len(agents))
	query := strings.ToLower(input.Query)
	categoryCounts := map[string]int{}
	for _, agent := range agents {
		categoryCounts[agent.Meta.Category]++
		if input.Category != "" && !strings.EqualFold(agent.Meta.Category, input.Category) {
			continue
		}
		haystack := strings.ToLower(agent.Meta.Title + " " + agent.Meta.Description + " " + strings.Join(agent.Meta.Tags, " "))
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		filtered = append(filtered, agent)
	}
	categories := make([]MarketCategory, 0, len(categoryCounts))
	for category, count := range categoryCounts {
		categories = append(categories, MarketCategory{ID: category, Count: count})
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i].ID < categories[j].ID })
	start := min(len(filtered), (input.Page-1)*input.PageSize)
	end := min(len(filtered), start+input.PageSize)
	return MarketSearchResult{Agents: filtered[start:end], Categories: categories,
		Page: input.Page, PageSize: input.PageSize, TotalCount: len(filtered),
		TotalPages: pageCount(len(filtered), input.PageSize), Source: "legacy-registry"}, nil
}

type liveMarketPayload struct {
	Items            []Agent
	Categories       []MarketCategory
	TotalCount       int
	TotalMarketCount int
}

func normalizeLiveMarketPayload(payload map[string]any) (liveMarketPayload, error) {
	itemsRaw, ok := payload["items"].([]any)
	if !ok {
		return liveMarketPayload{}, ErrInvalidRegistryEntry
	}
	items := make([]Agent, 0, min(len(itemsRaw), liveMarketPageSize))
	for _, raw := range itemsRaw {
		agent, valid := normalizeLiveAgent(raw)
		if valid {
			items = append(items, agent)
		}
		if len(items) == liveMarketPageSize {
			break
		}
	}
	categories := []MarketCategory{}
	if rawCategories, ok := payload["categories"].([]any); ok {
		for _, raw := range rawCategories {
			category, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id := stringValue(category["category"])
			if officialAssistantCategory(id) {
				categories = append(categories, MarketCategory{ID: id, Count: intValue(category["count"])})
			}
		}
	}
	total := boundedCount(intValue(payload["totalCount"]))
	return liveMarketPayload{Items: items, Categories: categories, TotalCount: total,
		TotalMarketCount: boundedCount(intValue(payload["totalMarketCount"]))}, nil
}

func normalizeLiveAgent(value any) (Agent, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return Agent{}, false
	}
	config, _ := raw["config"].(map[string]any)
	authorMap, _ := raw["author"].(map[string]any)
	identifier := trimText(stringValue(raw["identifier"]), maxAgentIdentifierChars)
	if !validIdentifier(identifier) {
		return Agent{}, false
	}
	agent := Agent{Identifier: identifier, Meta: AgentMeta{
		Avatar:      trimText(stringValue(raw["avatar"]), maxAgentAvatarChars),
		Title:       trimText(stringValue(raw["name"]), maxAgentTitleChars),
		Description: trimText(firstString(raw["description"], config["description"]), maxAgentDescriptionChars),
		Category:    trimText(stringValue(raw["category"]), maxAgentCategoryChars),
		Tags:        stringSlice(raw["tags"]),
		SystemRole:  trimText(stringValue(config["systemRole"]), maxAgentSystemRoleChars),
	}, CreatedAt: trimText(stringValue(raw["createdAt"]), 80),
		UpdatedAt:     trimText(stringValue(raw["updatedAt"]), 80),
		Author:        trimText(firstString(authorMap["name"], authorMap["userName"]), maxAgentAuthorChars),
		Version:       strconv.Itoa(intValue(raw["versionNumber"])),
		InstallCount:  boundedCount(intValue(raw["installCount"])),
		IsValidated:   boolValue(raw["isValidated"]),
		SafetyCheck:   trimText(stringValue(raw["safetyCheck"]), 40),
		RequiredTools: stringSlice(config["plugins"]),
	}
	if agent.Meta.Avatar == "" {
		agent.Meta.Avatar = "🤖"
	}
	if agent.Meta.Title == "" {
		agent.Meta.Title = identifier
	}
	if agent.Meta.Category == "" {
		agent.Meta.Category = "general"
	}
	if agent.Meta.SystemRole != "" {
		agent.Config = &AgentConfig{SystemRole: agent.Meta.SystemRole}
	}
	return agent, true
}

// React Router's Single Fetch payload is a bounded reference table. This
// decoder supports only its JSON object/array/string/number/bool subset and
// fails closed on shape drift.
func decodeLivePayload(values []any) (map[string]any, error) {
	if len(values) == 0 || len(values) > maxLiveMarketValues {
		return nil, ErrInvalidRegistryEntry
	}
	for index, value := range values {
		candidate, ok := value.(map[string]any)
		if !ok {
			continue
		}
		keys := map[string]bool{}
		for key := range candidate {
			nameIndex, err := strconv.Atoi(strings.TrimPrefix(key, "_"))
			if err == nil && nameIndex >= 0 && nameIndex < len(values) {
				keys[stringValue(values[nameIndex])] = true
			}
		}
		if !keys["items"] && !keys["detail"] {
			continue
		}
		decoded, err := decodeLiveValue(values, index, map[int]bool{}, 0)
		if err != nil {
			return nil, err
		}
		payload, ok := decoded.(map[string]any)
		if ok {
			return payload, nil
		}
	}
	return nil, ErrInvalidRegistryEntry
}

func decodeLiveValue(values []any, index int, stack map[int]bool, depth int) (any, error) {
	if index < 0 {
		return nil, nil
	}
	if index >= len(values) || depth > 80 || stack[index] {
		return nil, ErrInvalidRegistryEntry
	}
	stack[index] = true
	defer delete(stack, index)
	switch value := values[index].(type) {
	case map[string]any:
		output := make(map[string]any, len(value))
		for encodedKey, encodedValue := range value {
			keyIndex, err := strconv.Atoi(strings.TrimPrefix(encodedKey, "_"))
			valueIndex, valueOK := numberIndex(encodedValue)
			if err != nil || keyIndex < 0 || keyIndex >= len(values) || !valueOK {
				return nil, ErrInvalidRegistryEntry
			}
			key := stringValue(values[keyIndex])
			if key == "" {
				return nil, ErrInvalidRegistryEntry
			}
			decoded, err := decodeLiveValue(values, valueIndex, stack, depth+1)
			if err != nil {
				return nil, err
			}
			output[key] = decoded
		}
		return output, nil
	case []any:
		output := make([]any, 0, len(value))
		for _, encoded := range value {
			itemIndex, ok := numberIndex(encoded)
			if !ok {
				return nil, ErrInvalidRegistryEntry
			}
			decoded, err := decodeLiveValue(values, itemIndex, stack, depth+1)
			if err != nil {
				return nil, err
			}
			output = append(output, decoded)
		}
		return output, nil
	default:
		return value, nil
	}
}

func numberIndex(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func objectField(value map[string]any, key string) (any, bool) {
	field, ok := value[key]
	return field, ok
}

func liveMarketPath(locale Locale, resource string) string {
	resource = "/" + strings.TrimLeft(strings.TrimSpace(resource), "/")
	switch locale {
	case LocaleChinese:
		return "/zh" + resource
	case LocaleJapanese:
		return "/ja" + resource
	default:
		return resource
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func intValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func boolValue(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func stringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return []string{}
	}
	values := make([]string, 0, min(len(raw), maxAgentTags))
	for _, item := range raw {
		values = append(values, stringValue(item))
	}
	return normalizeStringTags(values)
}

func boundedCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000 {
		return 1_000_000_000
	}
	return value
}
