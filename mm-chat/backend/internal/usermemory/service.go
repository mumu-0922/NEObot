package usermemory

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var uuidRE = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-` +
		`[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

var memoryTypes = map[string]struct{}{
	"fact": {}, "preference": {}, "instruction": {}, "project": {},
	"warning": {}, "decision": {}, "context": {},
}

var searchStopWords = map[string]struct{}{
	"about": {}, "answer": {}, "chat": {}, "could": {}, "from": {},
	"have": {}, "how": {}, "please": {}, "project": {}, "tell": {},
	"that": {}, "the": {}, "this": {}, "what": {}, "when": {}, "with": {},
	"you": {}, "your": {}, "用户": {}, "回答": {}, "只回": {}, "请回": {},
	"问题": {}, "项目": {}, "什么": {}, "怎么": {}, "如何": {}, "请问": {},
	"一下": {}, "可以": {}, "我的": {}, "你的": {}, "他的": {}, "她的": {},
	"这个": {}, "那个": {}, "如果": {}, "以后": {}, "知道": {}, "不知": {},
}

type Service struct {
	repo             Repository
	hybridProvider   HybridShadowProvider
	portabilityCodec *PortabilityPlanCodec
	release          string
}

type ServiceOption func(*Service)

func WithHybridShadowProvider(provider HybridShadowProvider) ServiceOption {
	return func(service *Service) { service.hybridProvider = provider }
}

func WithPortabilityPlanCodec(codec *PortabilityPlanCodec) ServiceOption {
	return func(service *Service) { service.portabilityCodec = codec }
}

func WithPortabilityRelease(release string) ServiceOption {
	return func(service *Service) { service.release = strings.TrimSpace(release) }
}

func NewService(repo Repository, options ...ServiceOption) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func DefaultSettings() Settings {
	return Settings{SearchEnabled: true, L2Mode: "inherit", L3Mode: "inherit"}
}

func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	if err := s.requireRepository(); err != nil {
		return Settings{}, err
	}
	settings, ok, err := s.repo.GetSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	if !ok {
		settings = DefaultSettings()
	}
	return normalizeSettingsDefaults(settings), nil
}

func normalizeSettingsDefaults(settings Settings) Settings {
	if settings.L2Mode == "" {
		settings.L2Mode = "inherit"
	}
	if settings.L3Mode == "" {
		settings.L3Mode = "inherit"
	}
	return settings
}

func (s *Service) UpdateSettings(ctx context.Context, patch SettingsPatch) (Settings, error) {
	if err := s.requireRepository(); err != nil {
		return Settings{}, err
	}
	settings, found, err := s.repo.GetSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	if !found {
		settings = DefaultSettings()
	}
	settings = normalizeSettingsDefaults(settings)
	if patch.Enabled != nil {
		settings.Enabled = *patch.Enabled
		if *patch.Enabled && !found {
			settings.SearchEnabled = true
			settings.AutoRecordEnabled = true
		}
	}
	if patch.SearchEnabled != nil {
		settings.SearchEnabled = *patch.SearchEnabled
	}
	if patch.AutoRecordEnabled != nil {
		settings.AutoRecordEnabled = *patch.AutoRecordEnabled
	}
	if patch.SensitiveMemoryEnabled != nil {
		settings.SensitiveMemoryEnabled = *patch.SensitiveMemoryEnabled
	}
	if patch.L2Mode != nil {
		mode := strings.ToLower(strings.TrimSpace(*patch.L2Mode))
		if _, ok := memoryPolicyModes[mode]; !ok {
			return Settings{}, validation("INVALID_MEMORY_L2_MODE", "memory L2 mode is invalid")
		}
		settings.L2Mode = mode
	}
	if patch.L3Mode != nil {
		mode := strings.ToLower(strings.TrimSpace(*patch.L3Mode))
		if _, ok := memoryPolicyModes[mode]; !ok {
			return Settings{}, validation("INVALID_MEMORY_L3_MODE", "memory L3 mode is invalid")
		}
		settings.L3Mode = mode
	}
	return s.repo.UpsertSettings(ctx, settings)
}

func (s *Service) List(ctx context.Context) ([]Memory, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	memories, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if memories == nil {
		return []Memory{}, nil
	}
	return memories, nil
}

func (s *Service) CreateManual(ctx context.Context, input Candidate) (Memory, error) {
	normalized, err := normalizeCandidate(input)
	if err != nil {
		return Memory{}, err
	}
	sensitivity := ClassifyMemorySensitivity(normalized.Content)
	if sensitivity == SensitivitySecret {
		return Memory{}, validation("MEMORY_SECRET_REJECTED", "secrets cannot be stored as memory")
	}
	if sensitivity == SensitivitySensitive {
		if _, ok := s.repo.(GovernanceRepository); !ok {
			return Memory{}, ErrGovernanceRepositoryRequired
		}
		memory, createErr := s.CreateGovernanceMemory(ctx, GovernanceMemoryMutationInput{
			Candidate: normalized, ScopeType: "global", Sensitivity: sensitivity,
		})
		if createErr == nil {
			return governanceMemoryAsLegacy(memory), nil
		}
		if !errors.Is(createErr, ErrMemoryConflict) {
			return Memory{}, createErr
		}
		memories, listErr := s.List(ctx)
		if listErr != nil {
			return Memory{}, listErr
		}
		normalizedContent := normalizeSearchText(normalized.Content)
		for _, existing := range memories {
			if existing.ScopeType == "global" &&
				existing.NormalizedContent == normalizedContent {
				return existing, nil
			}
		}
		return Memory{}, createErr
	}
	return s.create(ctx, input, "manual", "", "")
}

func (s *Service) Update(ctx context.Context, memoryID string, input Candidate) (Memory, error) {
	if err := s.requireRepository(); err != nil {
		return Memory{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if !uuidRE.MatchString(memoryID) {
		return Memory{}, validation("INVALID_MEMORY_ID", "memory id must be a UUID")
	}
	normalized, err := normalizeCandidate(input)
	if err != nil {
		return Memory{}, err
	}
	sensitivity := ClassifyMemorySensitivity(normalized.Content)
	if sensitivity == SensitivitySecret {
		return Memory{}, validation("MEMORY_SECRET_REJECTED", "secrets cannot be stored as memory")
	}
	if sensitivity == SensitivitySensitive {
		if _, ok := s.repo.(GovernanceRepository); !ok {
			return Memory{}, ErrGovernanceRepositoryRequired
		}
		memories, listErr := s.List(ctx)
		if listErr != nil {
			return Memory{}, listErr
		}
		for _, existing := range memories {
			if existing.ID != memoryID || existing.ScopeType != "global" {
				continue
			}
			updated, updateErr := s.UpdateGovernanceMemory(ctx, GovernanceMemoryMutationInput{
				MemoryID: memoryID, ExpectedRevision: existing.Revision,
				Candidate: normalized, ScopeType: "global", Sensitivity: sensitivity,
			})
			if updateErr != nil {
				return Memory{}, updateErr
			}
			return governanceMemoryAsLegacy(updated), nil
		}
		return Memory{}, ErrMemoryNotFound
	}
	return s.repo.Update(ctx, memoryID, UpdateInput{
		Type:              normalized.Type,
		Content:           normalized.Content,
		NormalizedContent: normalizeSearchText(normalized.Content),
		Importance:        normalized.Importance,
		Tags:              normalized.Tags,
		Enabled:           true,
	})
}

func (s *Service) Delete(ctx context.Context, memoryID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	memoryID = strings.TrimSpace(memoryID)
	if !uuidRE.MatchString(memoryID) {
		return validation("INVALID_MEMORY_ID", "memory id must be a UUID")
	}
	return s.repo.Delete(ctx, memoryID)
}

func (s *Service) StoreExtracted(ctx context.Context, input ExtractionInput) ([]Memory, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	if !uuidRE.MatchString(strings.TrimSpace(input.ConversationID)) ||
		!uuidRE.MatchString(strings.TrimSpace(input.MessageID)) {
		return nil, validation("INVALID_MEMORY_SOURCE", "memory source ids must be UUIDs")
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !settings.AutoRecordEnabled {
		return []Memory{}, nil
	}

	created := make([]Memory, 0, min(len(input.Candidates), MaxExtractedItems))
	for _, candidate := range input.Candidates {
		if len(created) >= MaxExtractedItems {
			break
		}
		memory, err := s.create(
			ctx,
			candidate,
			"ai",
			input.ConversationID,
			input.MessageID,
		)
		if err != nil {
			var invalid ValidationError
			if !asValidationError(err, &invalid) {
				return created, err
			}
			continue
		}
		created = append(created, memory)
	}
	return created, nil
}

func (s *Service) SearchRelevant(ctx context.Context, query string, limit int) ([]Memory, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !settings.SearchEnabled {
		return []Memory{}, nil
	}
	query = normalizeSearchText(query)
	queryTerms := searchTerms(query)
	if query == "" || len(queryTerms) == 0 {
		return []Memory{}, nil
	}
	memories, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	type scoredMemory struct {
		memory Memory
		score  float64
	}
	scored := make([]scoredMemory, 0, len(memories))
	for _, memory := range memories {
		if !memory.Enabled || memory.DeletedAt != nil {
			continue
		}
		sensitivity := ClassifyMemorySensitivity(memory.Content)
		if sensitivity == SensitivitySecret ||
			(sensitivity == SensitivitySensitive && !settings.SensitiveMemoryEnabled) {
			continue
		}
		score := relevanceScore(memory, query, queryTerms)
		if score < 2.5 {
			continue
		}
		scored = append(scored, scoredMemory{memory: memory, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].memory.Importance != scored[j].memory.Importance {
			return scored[i].memory.Importance > scored[j].memory.Importance
		}
		return scored[i].memory.UpdatedAt.After(scored[j].memory.UpdatedAt)
	})
	limit = max(0, min(limit, MaxSearchResults))
	if len(scored) > limit {
		scored = scored[:limit]
	}
	result := make([]Memory, 0, len(scored))
	ids := make([]string, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.memory)
		ids = append(ids, item.memory.ID)
	}
	if len(ids) > 0 {
		_ = s.repo.MarkUsed(ctx, ids, time.Now().UTC())
	}
	return result, nil
}

func (s *Service) create(
	ctx context.Context,
	input Candidate,
	source string,
	conversationID string,
	messageID string,
) (Memory, error) {
	if err := s.requireRepository(); err != nil {
		return Memory{}, err
	}
	normalized, err := normalizeCandidate(input)
	if err != nil {
		return Memory{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Memory{}, err
	}
	return s.repo.Create(ctx, CreateInput{
		ID:                   id,
		Type:                 normalized.Type,
		Content:              normalized.Content,
		NormalizedContent:    normalizeSearchText(normalized.Content),
		Importance:           normalized.Importance,
		Tags:                 normalized.Tags,
		Source:               source,
		SourceConversationID: strings.TrimSpace(conversationID),
		SourceMessageID:      strings.TrimSpace(messageID),
		Enabled:              true,
	})
}

// NormalizeCandidateForStorage applies the canonical Memory validation and
// normalization contract without performing a write. The durable Memory
// worker uses this before submitting an untrusted proposal batch to the
// lease-fenced PostgreSQL capability.
func NormalizeCandidateForStorage(input Candidate) (Candidate, string, error) {
	normalized, err := normalizeCandidate(input)
	if err != nil {
		return Candidate{}, "", err
	}
	return normalized, normalizeSearchText(normalized.Content), nil
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func normalizeCandidate(input Candidate) (Candidate, error) {
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	if _, ok := memoryTypes[input.Type]; !ok {
		return Candidate{}, validation("INVALID_MEMORY_TYPE", "memory type is invalid")
	}
	input.Content = strings.Join(strings.Fields(input.Content), " ")
	if input.Content == "" || utf8.RuneCountInString(input.Content) > MaxContentChars {
		return Candidate{}, validation("INVALID_MEMORY_CONTENT", "memory content must be between 1 and 2000 characters")
	}
	if input.Importance == 0 {
		input.Importance = 3
	}
	if input.Importance < 1 || input.Importance > 5 {
		return Candidate{}, validation("INVALID_MEMORY_IMPORTANCE", "memory importance must be between 1 and 5")
	}
	input.Tags = normalizeTags(input.Tags)
	return input, nil
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, min(len(tags), MaxTags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.Join(strings.Fields(tag), " "))
		if tag == "" || utf8.RuneCountInString(tag) > MaxTagChars {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) == MaxTags {
			break
		}
	}
	return result
}

func relevanceScore(memory Memory, query string, queryTerms map[string]struct{}) float64 {
	content := memory.NormalizedContent
	if content == "" {
		content = normalizeSearchText(memory.Content)
	}
	contentTerms := searchTerms(content)
	tagTerms := searchTerms(strings.Join(memory.Tags, " "))
	score := 0.0
	if content == query {
		score += 100
	} else if utf8.RuneCountInString(query) >= 4 &&
		(strings.Contains(content, query) || strings.Contains(query, content)) {
		score += 12
	}
	matched := 0
	for term := range queryTerms {
		if _, ok := tagTerms[term]; ok {
			score += 4
			matched++
			continue
		}
		if _, ok := contentTerms[term]; ok {
			score += 2
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	score += float64(memory.Importance) * 0.2
	if memory.Type == "preference" || memory.Type == "instruction" || memory.Type == "decision" {
		score += 0.5
	}
	return score
}

func searchTerms(value string) map[string]struct{} {
	terms := make(map[string]struct{})
	latin := make([]rune, 0, 16)
	han := make([]rune, 0, 16)
	flushLatin := func() {
		if len(latin) > 1 {
			addSearchTerm(terms, string(latin))
		}
		latin = latin[:0]
	}
	flushHan := func() {
		if len(han) == 1 {
			addSearchTerm(terms, string(han))
		} else if len(han) > 1 {
			if len(han) <= 6 {
				addSearchTerm(terms, string(han))
			}
			for i := 0; i+1 < len(han); i++ {
				addSearchTerm(terms, string(han[i:i+2]))
			}
		}
		han = han[:0]
	}
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			latin = append(latin, r)
		default:
			flushLatin()
			flushHan()
		}
	}
	flushLatin()
	flushHan()
	return terms
}

func addSearchTerm(terms map[string]struct{}, term string) {
	if _, stopped := searchStopWords[term]; stopped {
		return
	}
	terms[term] = struct{}{}
}

func normalizeSearchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func validation(code string, message string) ValidationError {
	return ValidationError{Code: code, Message: message}
}

func asValidationError(err error, target *ValidationError) bool {
	if value, ok := err.(ValidationError); ok {
		*target = value
		return true
	}
	return false
}
