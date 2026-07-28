package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxProjects                = 200
	MaxProjectNameChars        = 200
	MaxProjectDescriptionChars = 4000
	MaxGovernanceActivities    = 20
)

var memoryPolicyModes = map[string]struct{}{
	"inherit": {}, "on": {}, "off": {},
}

type GovernanceRepository interface {
	GovernanceSnapshot(context.Context) (GovernanceSnapshot, error)
	CreateProject(context.Context, CreateProjectInput) (MemoryProject, error)
	UpdateProject(context.Context, UpdateProjectInput) (MemoryProject, error)
	GetConversationPolicy(context.Context, string) (ConversationMemoryPolicy, error)
	UpdateConversationPolicy(context.Context, UpdateConversationPolicyInput) (ConversationMemoryPolicy, error)
	CreateGovernanceMemory(context.Context, GovernanceMemoryMutationInput) (GovernanceMemory, error)
	UpdateGovernanceMemory(context.Context, GovernanceMemoryMutationInput) (GovernanceMemory, error)
	DeleteGovernanceMemory(context.Context, GovernanceMemoryDeleteInput) (MemoryDeletionProgress, error)
	GovernanceMemoryDetail(context.Context, string) (GovernanceMemoryDetail, error)
	DecideMemoryReview(context.Context, MemoryReviewDecisionInput) (MemoryReviewDecisionResult, error)
	ListMessageActivities(context.Context, string, int) ([]MemoryActivity, error)
}

type GovernanceSnapshot struct {
	Settings      Settings                   `json:"settings"`
	Projects      []MemoryProject            `json:"projects"`
	Conversations []ConversationMemoryPolicy `json:"conversations"`
	Memories      []GovernanceMemory         `json:"memories"`
	Reviews       []MemoryReviewSuggestion   `json:"reviews"`
	Deletions     []MemoryDeletionProgress   `json:"deletions"`
	Diagnostics   []MemorySearchDiagnostic   `json:"diagnostics"`
}

type MemoryProject struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	LifecycleStatus   string `json:"lifecycleStatus"`
	Revision          int64  `json:"revision"`
	ScopeGeneration   int64  `json:"scopeGeneration"`
	ConversationCount int    `json:"conversationCount"`
	MemoryCount       int    `json:"memoryCount"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
	ArchivedAt        *int64 `json:"archivedAt,omitempty"`
}

type CreateProjectInput struct {
	ID          string
	Name        string
	Description string
}

type UpdateProjectInput struct {
	ProjectID        string
	ExpectedRevision int64
	Name             string
	Description      string
	LifecycleStatus  string
}

type ConversationMemoryPolicy struct {
	ConversationID  string `json:"conversationId"`
	Title           string `json:"title"`
	ProjectID       string `json:"projectId,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	ProjectStatus   string `json:"projectStatus,omitempty"`
	UseMode         string `json:"useMode"`
	LearnMode       string `json:"learnMode"`
	EffectiveUse    bool   `json:"effectiveUse"`
	EffectiveLearn  bool   `json:"effectiveLearn"`
	LearnForcedOff  bool   `json:"learnForcedOff"`
	ScopeGeneration int64  `json:"scopeGeneration"`
	UpdatedAt       int64  `json:"updatedAt"`
}

type UpdateConversationPolicyInput struct {
	ConversationID          string
	ExpectedScopeGeneration int64
	ProjectID               string
	UseMode                 string
	LearnMode               string
}

type GovernanceMemory struct {
	ID                   string   `json:"id"`
	Type                 string   `json:"type"`
	Content              string   `json:"content"`
	Importance           int      `json:"importance"`
	Tags                 []string `json:"tags"`
	Source               string   `json:"source"`
	AuthorityKind        string   `json:"authorityKind"`
	Enabled              bool     `json:"enabled"`
	Revision             int64    `json:"revision"`
	ScopeType            string   `json:"scopeType"`
	ProjectID            string   `json:"projectId,omitempty"`
	ProjectName          string   `json:"projectName,omitempty"`
	ConversationID       string   `json:"conversationId,omitempty"`
	ConversationTitle    string   `json:"conversationTitle,omitempty"`
	LifecycleStatus      string   `json:"lifecycleStatus"`
	Sensitivity          string   `json:"sensitivity"`
	RecallStatus         string   `json:"recallStatus"`
	ValidFrom            *int64   `json:"validFrom,omitempty"`
	ValidTo              *int64   `json:"validTo,omitempty"`
	ExpiresAt            *int64   `json:"expiresAt,omitempty"`
	SupersededByMemoryID string   `json:"supersededByMemoryId,omitempty"`
	LastUsedAt           *int64   `json:"lastUsedAt,omitempty"`
	CreatedAt            int64    `json:"createdAt"`
	UpdatedAt            int64    `json:"updatedAt"`
}

type GovernanceMemoryMutationInput struct {
	MemoryID         string
	ExpectedRevision int64
	Candidate        Candidate
	ScopeType        string
	ProjectID        string
	ConversationID   string
	Sensitivity      string
}

type GovernanceMemoryDeleteInput struct {
	MemoryID         string
	ExpectedRevision int64
	EventID          string
	JobID            string
	TombstoneID      string
	ManifestID       string
}

type MemoryEvidence struct {
	MessageID         string `json:"messageId"`
	ConversationID    string `json:"conversationId"`
	ConversationTitle string `json:"conversationTitle,omitempty"`
	Role              string `json:"role"`
	SourceDeleted     bool   `json:"sourceDeleted"`
	SourceExcerpt     string `json:"sourceExcerpt,omitempty"`
	ObservedAt        int64  `json:"observedAt"`
}

type MemoryRevision struct {
	Revision     int64  `json:"revision"`
	Operation    string `json:"operation"`
	PriorContent string `json:"priorContent,omitempty"`
	ActorType    string `json:"actorType"`
	ResultCode   string `json:"resultCode,omitempty"`
	Purged       bool   `json:"purged"`
	CreatedAt    int64  `json:"createdAt"`
}

type MemoryUsageLink struct {
	AssistantMessageID string `json:"assistantMessageId"`
	MemoryRevision     int64  `json:"memoryRevision"`
	CreatedAt          int64  `json:"createdAt"`
}

type GovernanceMemoryDetail struct {
	Memory   GovernanceMemory  `json:"memory"`
	Evidence []MemoryEvidence  `json:"evidence"`
	History  []MemoryRevision  `json:"history"`
	Usages   []MemoryUsageLink `json:"usages"`
}

type MemoryReviewTarget struct {
	MemoryID  string `json:"memoryId"`
	Revision  int64  `json:"revision"`
	Type      string `json:"type,omitempty"`
	Content   string `json:"content,omitempty"`
	ScopeType string `json:"scopeType,omitempty"`
	Current   bool   `json:"current"`
}

type MemoryReviewSuggestion struct {
	ID                 string               `json:"id"`
	Type               string               `json:"type"`
	Content            string               `json:"content"`
	Importance         int                  `json:"importance"`
	Tags               []string             `json:"tags"`
	Sensitivity        string               `json:"sensitivity"`
	ProposedAction     string               `json:"proposedAction"`
	ReasonCode         string               `json:"reasonCode"`
	ScopeType          string               `json:"scopeType"`
	ProjectID          string               `json:"projectId,omitempty"`
	ConversationID     string               `json:"conversationId,omitempty"`
	Targets            []MemoryReviewTarget `json:"targets"`
	EvidenceMessageIDs []string             `json:"evidenceMessageIds"`
	ExpiresAt          int64                `json:"expiresAt"`
	CreatedAt          int64                `json:"createdAt"`
}

type MemoryReviewDecisionInput struct {
	SuggestionID      string
	DecisionID        string
	Decision          string
	MemoryID          string
	EditedContent     string
	NormalizedContent string
	DecisionHash      string
}

type MemoryReviewDecisionResult struct {
	SuggestionID   string `json:"suggestionId"`
	Decision       string `json:"decision"`
	Status         string `json:"status"`
	ResultCode     string `json:"resultCode"`
	MemoryID       string `json:"memoryId,omitempty"`
	MemoryRevision int64  `json:"memoryRevision,omitempty"`
}

type MemoryDeletionProgress struct {
	ManifestID         string `json:"manifestId"`
	MemoryID           string `json:"memoryId"`
	ImmediateHidden    bool   `json:"immediateHidden"`
	OnlinePurgeStatus  string `json:"onlinePurgeStatus"`
	BackupExpiryStatus string `json:"backupExpiryStatus"`
	BackupExpiresAt    int64  `json:"backupExpiresAt"`
	DeletedAt          int64  `json:"deletedAt"`
	PurgedAt           *int64 `json:"purgedAt,omitempty"`
}

type MemorySearchDiagnostic struct {
	AssistantMessageID string `json:"assistantMessageId"`
	Profile            string `json:"profile"`
	Status             string `json:"status"`
	ResultCode         string `json:"resultCode"`
	FallbackCode       string `json:"fallbackCode"`
	BaselineCount      int    `json:"baselineCount"`
	FinalCount         int    `json:"finalCount"`
	OverlapCount       int    `json:"overlapCount"`
	EstimatedTokens    int    `json:"estimatedTokens"`
	DurationMillis     int    `json:"durationMillis"`
	CreatedAt          int64  `json:"createdAt"`
}

func (s *Service) GovernanceSnapshot(ctx context.Context) (GovernanceSnapshot, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return GovernanceSnapshot{}, err
	}
	return repo.GovernanceSnapshot(ctx)
}

func (s *Service) ListProjects(ctx context.Context) ([]MemoryProject, error) {
	snapshot, err := s.GovernanceSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot.Projects == nil {
		return []MemoryProject{}, nil
	}
	return snapshot.Projects, nil
}

func (s *Service) CreateProject(ctx context.Context, name, description string) (MemoryProject, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return MemoryProject{}, err
	}
	name, description, err = normalizeProject(name, description)
	if err != nil {
		return MemoryProject{}, err
	}
	id, err := newUUID()
	if err != nil {
		return MemoryProject{}, err
	}
	return repo.CreateProject(ctx, CreateProjectInput{ID: id, Name: name, Description: description})
}

func (s *Service) UpdateProject(ctx context.Context, input UpdateProjectInput) (MemoryProject, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return MemoryProject{}, err
	}
	if !uuidRE.MatchString(strings.TrimSpace(input.ProjectID)) || input.ExpectedRevision < 1 {
		return MemoryProject{}, validation("INVALID_MEMORY_PROJECT", "project id or revision is invalid")
	}
	input.Name, input.Description, err = normalizeProject(input.Name, input.Description)
	if err != nil {
		return MemoryProject{}, err
	}
	input.LifecycleStatus = strings.ToLower(strings.TrimSpace(input.LifecycleStatus))
	if input.LifecycleStatus != "active" && input.LifecycleStatus != "archived" {
		return MemoryProject{}, validation("INVALID_MEMORY_PROJECT_STATUS", "project status is invalid")
	}
	return repo.UpdateProject(ctx, input)
}

func (s *Service) GetConversationPolicy(ctx context.Context, conversationID string) (ConversationMemoryPolicy, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return ConversationMemoryPolicy{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !uuidRE.MatchString(conversationID) {
		return ConversationMemoryPolicy{}, validation("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	return repo.GetConversationPolicy(ctx, conversationID)
}

func (s *Service) ConversationMemoryUseAllowed(ctx context.Context, conversationID string) (bool, error) {
	if s == nil || s.repo == nil {
		return true, nil
	}
	if _, ok := s.repo.(GovernanceRepository); !ok {
		return true, nil
	}
	policy, err := s.GetConversationPolicy(ctx, conversationID)
	if err != nil {
		return false, err
	}
	return policy.EffectiveUse, nil
}

func (s *Service) UpdateConversationPolicy(ctx context.Context, input UpdateConversationPolicyInput) (ConversationMemoryPolicy, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return ConversationMemoryPolicy{}, err
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.UseMode = strings.ToLower(strings.TrimSpace(input.UseMode))
	input.LearnMode = strings.ToLower(strings.TrimSpace(input.LearnMode))
	if !uuidRE.MatchString(input.ConversationID) || input.ExpectedScopeGeneration < 1 {
		return ConversationMemoryPolicy{}, validation("INVALID_MEMORY_POLICY", "conversation policy identity is invalid")
	}
	if input.ProjectID != "" && !uuidRE.MatchString(input.ProjectID) {
		return ConversationMemoryPolicy{}, validation("INVALID_MEMORY_PROJECT", "project id must be a UUID")
	}
	if _, ok := memoryPolicyModes[input.UseMode]; !ok {
		return ConversationMemoryPolicy{}, validation("INVALID_MEMORY_USE_MODE", "memory use mode is invalid")
	}
	if _, ok := memoryPolicyModes[input.LearnMode]; !ok {
		return ConversationMemoryPolicy{}, validation("INVALID_MEMORY_LEARN_MODE", "memory learn mode is invalid")
	}
	return repo.UpdateConversationPolicy(ctx, input)
}

func (s *Service) CreateGovernanceMemory(ctx context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return GovernanceMemory{}, err
	}
	input, err = normalizeGovernanceMemoryMutation(input, false)
	if err != nil {
		return GovernanceMemory{}, err
	}
	id, err := newUUID()
	if err != nil {
		return GovernanceMemory{}, err
	}
	input.MemoryID = id
	return repo.CreateGovernanceMemory(ctx, input)
}

func (s *Service) UpdateGovernanceMemory(ctx context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return GovernanceMemory{}, err
	}
	input, err = normalizeGovernanceMemoryMutation(input, true)
	if err != nil {
		return GovernanceMemory{}, err
	}
	return repo.UpdateGovernanceMemory(ctx, input)
}

func (s *Service) DeleteGovernanceMemory(ctx context.Context, memoryID string, expectedRevision int64) (MemoryDeletionProgress, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return MemoryDeletionProgress{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if !uuidRE.MatchString(memoryID) || expectedRevision < 1 {
		return MemoryDeletionProgress{}, validation("INVALID_MEMORY_REVISION", "memory id or revision is invalid")
	}
	ids := make([]string, 4)
	for index := range ids {
		ids[index], err = newUUID()
		if err != nil {
			return MemoryDeletionProgress{}, err
		}
	}
	return repo.DeleteGovernanceMemory(ctx, GovernanceMemoryDeleteInput{
		MemoryID: memoryID, ExpectedRevision: expectedRevision,
		EventID: ids[0], JobID: ids[1], TombstoneID: ids[2], ManifestID: ids[3],
	})
}

func (s *Service) GovernanceMemoryDetail(ctx context.Context, memoryID string) (GovernanceMemoryDetail, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return GovernanceMemoryDetail{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if !uuidRE.MatchString(memoryID) {
		return GovernanceMemoryDetail{}, validation("INVALID_MEMORY_ID", "memory id must be a UUID")
	}
	return repo.GovernanceMemoryDetail(ctx, memoryID)
}

func (s *Service) DecideMemoryReview(ctx context.Context, input MemoryReviewDecisionInput) (MemoryReviewDecisionResult, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return MemoryReviewDecisionResult{}, err
	}
	input.SuggestionID = strings.TrimSpace(input.SuggestionID)
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	if !uuidRE.MatchString(input.SuggestionID) {
		return MemoryReviewDecisionResult{}, validation("INVALID_MEMORY_REVIEW_ID", "review id must be a UUID")
	}
	switch input.Decision {
	case "keep_current", "accept_new", "edit_merge", "keep_both", "reject":
	default:
		return MemoryReviewDecisionResult{}, validation("INVALID_MEMORY_REVIEW_DECISION", "review decision is invalid")
	}
	if input.Decision == "edit_merge" {
		input.EditedContent = strings.Join(strings.Fields(input.EditedContent), " ")
		if input.EditedContent == "" || utf8.RuneCountInString(input.EditedContent) > MaxContentChars {
			return MemoryReviewDecisionResult{}, validation("INVALID_MEMORY_CONTENT", "memory content must be between 1 and 2000 characters")
		}
		if ClassifyMemorySensitivity(input.EditedContent) == SensitivitySecret {
			return MemoryReviewDecisionResult{}, validation("MEMORY_SECRET_REJECTED", "secrets cannot be stored as memory")
		}
		input.NormalizedContent = normalizeSearchText(input.EditedContent)
	} else if strings.TrimSpace(input.EditedContent) != "" {
		return MemoryReviewDecisionResult{}, validation("INVALID_MEMORY_REVIEW_CONTENT", "edited content is only allowed for edit merge")
	}
	input.DecisionID, err = newUUID()
	if err != nil {
		return MemoryReviewDecisionResult{}, err
	}
	if input.Decision != "keep_current" && input.Decision != "reject" {
		input.MemoryID, err = newUUID()
		if err != nil {
			return MemoryReviewDecisionResult{}, err
		}
	}
	input.DecisionHash = hashReviewDecision(input)
	return repo.DecideMemoryReview(ctx, input)
}

func (s *Service) ListMessageActivities(ctx context.Context, assistantMessageID string, limit int) ([]MemoryActivity, error) {
	repo, err := s.governanceRepository()
	if err != nil {
		return nil, err
	}
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(assistantMessageID) {
		return nil, validation("INVALID_ASSISTANT_MESSAGE_ID", "assistant message id must be a UUID")
	}
	limit = max(1, min(limit, MaxGovernanceActivities))
	return repo.ListMessageActivities(ctx, assistantMessageID, limit)
}

func (s *Service) governanceRepository() (GovernanceRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(GovernanceRepository)
	if !ok {
		return nil, ErrGovernanceRepositoryRequired
	}
	return repo, nil
}

func normalizeProject(name, description string) (string, string, error) {
	name = strings.Join(strings.Fields(name), " ")
	description = strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > MaxProjectNameChars {
		return "", "", validation("INVALID_MEMORY_PROJECT_NAME", "project name is invalid")
	}
	if utf8.RuneCountInString(description) > MaxProjectDescriptionChars {
		return "", "", validation("INVALID_MEMORY_PROJECT_DESCRIPTION", "project description is too long")
	}
	return name, description, nil
}

func normalizeGovernanceMemoryMutation(input GovernanceMemoryMutationInput, update bool) (GovernanceMemoryMutationInput, error) {
	var err error
	input.Candidate, _, err = NormalizeCandidateForStorage(input.Candidate)
	if err != nil {
		return GovernanceMemoryMutationInput{}, err
	}
	detectedSensitivity := ClassifyMemorySensitivity(input.Candidate.Content)
	if detectedSensitivity == SensitivitySecret {
		return GovernanceMemoryMutationInput{}, validation("MEMORY_SECRET_REJECTED", "secrets cannot be stored as memory")
	}
	input.ScopeType = strings.ToLower(strings.TrimSpace(input.ScopeType))
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.Sensitivity = strings.ToLower(strings.TrimSpace(input.Sensitivity))
	if input.Sensitivity == "" {
		input.Sensitivity = detectedSensitivity
	}
	if detectedSensitivity == SensitivitySensitive {
		input.Sensitivity = SensitivitySensitive
	}
	if input.Sensitivity != SensitivityNormal && input.Sensitivity != SensitivitySensitive {
		return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_SENSITIVITY", "memory sensitivity is invalid")
	}
	switch input.ScopeType {
	case "global":
		if input.ProjectID != "" || input.ConversationID != "" {
			return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_SCOPE", "global memory cannot bind a scope id")
		}
	case "project":
		if !uuidRE.MatchString(input.ProjectID) || input.ConversationID != "" {
			return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_SCOPE", "project memory requires one project id")
		}
	case "conversation":
		if !uuidRE.MatchString(input.ConversationID) || input.ProjectID != "" {
			return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_SCOPE", "conversation memory requires one conversation id")
		}
	default:
		return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_SCOPE", "memory scope is invalid")
	}
	if update {
		input.MemoryID = strings.TrimSpace(input.MemoryID)
		if !uuidRE.MatchString(input.MemoryID) || input.ExpectedRevision < 1 {
			return GovernanceMemoryMutationInput{}, validation("INVALID_MEMORY_REVISION", "memory id or revision is invalid")
		}
	}
	return input, nil
}

func hashReviewDecision(input MemoryReviewDecisionInput) string {
	digest := sha256.New()
	for _, value := range []string{
		"memory-review-v1", input.SuggestionID, input.Decision,
		input.EditedContent, input.NormalizedContent,
	} {
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(value))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func governanceMemoryAsLegacy(memory GovernanceMemory) Memory {
	result := Memory{
		ID: memory.ID, Type: memory.Type, Content: memory.Content,
		NormalizedContent: normalizeSearchText(memory.Content),
		Importance:        memory.Importance, Tags: append([]string(nil), memory.Tags...),
		Source: memory.Source, Enabled: memory.Enabled,
		Revision: memory.Revision, ScopeType: memory.ScopeType,
		CreatedAt: time.UnixMilli(memory.CreatedAt).UTC(),
		UpdatedAt: time.UnixMilli(memory.UpdatedAt).UTC(),
	}
	if memory.LastUsedAt != nil {
		value := time.UnixMilli(*memory.LastUsedAt).UTC()
		result.LastUsedAt = &value
	}
	return result
}
