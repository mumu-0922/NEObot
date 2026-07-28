package usermemory

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	governanceProjectID      = "41000000-0000-4000-8000-000000000001"
	governanceConversationID = "21000000-0000-4000-8000-000000000001"
	governanceMemoryID       = "51000000-0000-4000-8000-000000000001"
	governanceReviewID       = "61000000-0000-4000-8000-000000000001"
	governanceAssistantID    = "31000000-0000-4000-8000-000000000001"
)

func TestGovernanceServiceValidatesProjectsPoliciesAndSecrets(t *testing.T) {
	repo := &governanceTestRepository{fakeRepository: &fakeRepository{
		memories: []Memory{{
			ID: governanceMemoryID, Type: "fact", Content: "ordinary",
			NormalizedContent: "ordinary", Enabled: true, Revision: 1,
			ScopeType: "global",
		}},
	}}
	service := NewService(repo)

	project, err := service.CreateProject(context.Background(), "  Neo   Chat  ", " notes ")
	if err != nil || project.Name != "Neo Chat" || repo.createdProject.ID == "" {
		t.Fatalf("CreateProject() = %#v/%v input=%#v", project, err, repo.createdProject)
	}
	if _, err := service.UpdateConversationPolicy(context.Background(), UpdateConversationPolicyInput{
		ConversationID: governanceConversationID, ExpectedScopeGeneration: 1,
		ProjectID: governanceProjectID, UseMode: "off", LearnMode: "on",
	}); err != nil || repo.updatedPolicy.UseMode != "off" {
		t.Fatalf("UpdateConversationPolicy() err=%v input=%#v", err, repo.updatedPolicy)
	}
	if _, err := service.UpdateConversationPolicy(context.Background(), UpdateConversationPolicyInput{
		ConversationID: governanceConversationID, ExpectedScopeGeneration: 1,
		UseMode: "sometimes", LearnMode: "inherit",
	}); err == nil {
		t.Fatal("invalid policy mode accepted")
	}
	for _, secret := range []string{
		"API key: sk-secretvalue",
		"cookie: fixture-cookie-value",
		"Authorization: Bearer fixture-token-value",
		"cvv=1234",
	} {
		if _, err := service.CreateGovernanceMemory(context.Background(), GovernanceMemoryMutationInput{
			Candidate: Candidate{Type: "fact", Content: secret, Importance: 3},
			ScopeType: "global", Sensitivity: "normal",
		}); err == nil {
			t.Fatalf("secret governance memory accepted: %q", secret)
		}
	}
	memory, err := service.CreateGovernanceMemory(context.Background(), GovernanceMemoryMutationInput{
		Candidate: Candidate{Type: "project", Content: "Neo Chat uses Go", Importance: 4, Tags: []string{"Stack"}},
		ScopeType: "project", ProjectID: governanceProjectID, Sensitivity: "normal",
	})
	if err != nil || memory.ScopeType != "project" || repo.createdMemory.MemoryID == "" ||
		repo.createdMemory.Candidate.Tags[0] != "stack" {
		t.Fatalf("CreateGovernanceMemory() = %#v/%v input=%#v", memory, err, repo.createdMemory)
	}
	if _, err := service.CreateGovernanceMemory(context.Background(), GovernanceMemoryMutationInput{
		Candidate: Candidate{Type: "fact", Content: "我患有糖尿病", Importance: 4},
		ScopeType: "global", Sensitivity: "normal",
	}); err != nil || repo.createdMemory.Sensitivity != SensitivitySensitive {
		t.Fatalf("sensitive downgrade was not corrected: err=%v input=%#v", err, repo.createdMemory)
	}
	if _, err := service.CreateManual(context.Background(), Candidate{
		Type: "fact", Content: "我的家庭住址在测试路九号", Importance: 4,
	}); err != nil || repo.createdMemory.Sensitivity != SensitivitySensitive {
		t.Fatalf("legacy sensitive create bypassed governance: err=%v input=%#v", err, repo.createdMemory)
	}
	if _, err := service.Update(context.Background(), governanceMemoryID, Candidate{
		Type: "fact", Content: "我的工资是测试金额", Importance: 4,
	}); err != nil || repo.updatedMemory.Sensitivity != SensitivitySensitive ||
		repo.updatedMemory.ExpectedRevision != 1 {
		t.Fatalf("legacy sensitive update bypassed governance: err=%v input=%#v", err, repo.updatedMemory)
	}
	if _, err := service.CreateManual(context.Background(), Candidate{
		Type: "fact", Content: "password: fixture-secret", Importance: 4,
	}); err == nil {
		t.Fatal("legacy secret create was accepted")
	}
}

func TestGovernanceServiceReviewDecisionAndMessageActivities(t *testing.T) {
	repo := &governanceTestRepository{fakeRepository: &fakeRepository{}, activities: []MemoryActivity{{
		ID: governanceReviewID, AssistantMessageID: governanceAssistantID,
		Action: "review_required", Status: "completed",
	}}}
	service := NewService(repo)

	result, err := service.DecideMemoryReview(context.Background(), MemoryReviewDecisionInput{
		SuggestionID: governanceReviewID, Decision: "edit_merge",
		EditedContent: "  Prefer concise Chinese replies  ",
	})
	if err != nil || result.Status != "accepted" || repo.reviewDecision.DecisionID == "" ||
		repo.reviewDecision.MemoryID == "" || len(repo.reviewDecision.DecisionHash) != 64 ||
		repo.reviewDecision.EditedContent != "Prefer concise Chinese replies" {
		t.Fatalf("DecideMemoryReview() = %#v/%v input=%#v", result, err, repo.reviewDecision)
	}
	if _, err := service.DecideMemoryReview(context.Background(), MemoryReviewDecisionInput{
		SuggestionID: governanceReviewID, Decision: "reject", EditedContent: "not allowed",
	}); err == nil {
		t.Fatal("non-edit decision accepted edited content")
	}
	activities, err := service.ListMessageActivities(context.Background(), governanceAssistantID, 200)
	if err != nil || len(activities) != 1 || repo.activityLimit != MaxGovernanceActivities {
		t.Fatalf("ListMessageActivities() = %#v/%v limit=%d", activities, err, repo.activityLimit)
	}
}

func TestGovernanceMemoryActivityPayloadUsesEpochMillis(t *testing.T) {
	payload := governanceMemoryActivityPayload{
		ID: "activity-1", AssistantMessageID: "assistant-1", Ordinal: 1,
		SourceKind: "memory_job", ScopeType: "project",
		CreatedAt: 1_700_000_000_123, UpdatedAt: 1_700_000_001_456,
	}
	activity := payload.activity()
	if activity.SourceKind != "memory_job" || activity.ScopeType != "project" {
		t.Fatalf("activity metadata = %#v", activity)
	}
	if activity.CreatedAt.UnixMilli() != payload.CreatedAt ||
		activity.UpdatedAt.UnixMilli() != payload.UpdatedAt ||
		activity.CreatedAt.Location() != time.UTC {
		t.Fatalf("activity timestamps = %#v", activity)
	}
}

func TestGovernanceHandlerRoutesAndErrors(t *testing.T) {
	repo := &governanceTestRepository{
		fakeRepository: &fakeRepository{},
		snapshot: GovernanceSnapshot{Projects: []MemoryProject{{
			ID: governanceProjectID, Name: "Neo Chat", LifecycleStatus: "active",
		}}},
	}
	handler := NewHandler(NewService(repo))

	response := serveMemoryRequest(t, handler, http.MethodGet, memoryGovernancePath, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), governanceProjectID) {
		t.Fatalf("snapshot response = %d %s", response.Code, response.Body.String())
	}
	response = serveMemoryRequest(t, handler, http.MethodGet, memoryProjectsPath, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), governanceProjectID) {
		t.Fatalf("project list response = %d %s", response.Code, response.Body.String())
	}
	response = serveMemoryRequest(t, handler, http.MethodPost, memoryProjectsPath,
		`{"name":"Neo Chat","description":"Memory"}`)
	if response.Code != http.StatusCreated || repo.createdProject.ID == "" {
		t.Fatalf("project response = %d %s input=%#v", response.Code, response.Body.String(), repo.createdProject)
	}
	response = serveMemoryRequest(t, handler, http.MethodPost,
		memoryReviewsPath+"/"+governanceReviewID+"/decision",
		`{"decision":"reject","editedContent":""}`)
	if response.Code != http.StatusOK || repo.reviewDecision.Decision != "reject" {
		t.Fatalf("review response = %d %s input=%#v", response.Code, response.Body.String(), repo.reviewDecision)
	}
	response = serveMemoryRequest(t, handler, http.MethodPost,
		memoryGovernancePath+"/memories", `{"type":"fact","content":"x","unknown":true}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown governance field = %d %s", response.Code, response.Body.String())
	}
	repo.updateProjectError = validation(
		"MEMORY_GOVERNANCE_REVISION_STALE",
		"memory governance state changed; reload and retry",
	)
	response = serveMemoryRequest(t, handler, http.MethodPatch,
		memoryProjectsPath+"/"+governanceProjectID,
		`{"name":"Neo Chat","description":"Memory","expectedRevision":1,"lifecycleStatus":"active"}`)
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "MEMORY_GOVERNANCE_REVISION_STALE") {
		t.Fatalf("stale project response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationMemoryUseAllowedPreservesNonGovernanceDoubles(t *testing.T) {
	service := NewService(&fakeRepository{})
	allowed, err := service.ConversationMemoryUseAllowed(context.Background(), governanceConversationID)
	if err != nil || !allowed {
		t.Fatalf("non-governance policy = %v/%v", allowed, err)
	}
	repo := &governanceTestRepository{fakeRepository: &fakeRepository{}, policy: ConversationMemoryPolicy{EffectiveUse: false}}
	allowed, err = NewService(repo).ConversationMemoryUseAllowed(context.Background(), governanceConversationID)
	if err != nil || allowed {
		t.Fatalf("governance policy = %v/%v", allowed, err)
	}
	repo.policyError = ErrConversationPolicyNotFound
	if _, err = NewService(repo).ConversationMemoryUseAllowed(context.Background(), governanceConversationID); !errors.Is(err, ErrConversationPolicyNotFound) {
		t.Fatalf("policy error = %v", err)
	}
}

type governanceTestRepository struct {
	*fakeRepository
	snapshot           GovernanceSnapshot
	policy             ConversationMemoryPolicy
	policyError        error
	createdProject     CreateProjectInput
	updatedProject     UpdateProjectInput
	updateProjectError error
	updatedPolicy      UpdateConversationPolicyInput
	createdMemory      GovernanceMemoryMutationInput
	updatedMemory      GovernanceMemoryMutationInput
	deletedMemory      GovernanceMemoryDeleteInput
	reviewDecision     MemoryReviewDecisionInput
	activities         []MemoryActivity
	activityLimit      int
}

func (r *governanceTestRepository) GovernanceSnapshot(context.Context) (GovernanceSnapshot, error) {
	return r.snapshot, nil
}
func (r *governanceTestRepository) CreateProject(_ context.Context, input CreateProjectInput) (MemoryProject, error) {
	r.createdProject = input
	return MemoryProject{ID: input.ID, Name: input.Name, Description: input.Description, LifecycleStatus: "active", Revision: 1}, nil
}
func (r *governanceTestRepository) UpdateProject(_ context.Context, input UpdateProjectInput) (MemoryProject, error) {
	r.updatedProject = input
	if r.updateProjectError != nil {
		return MemoryProject{}, r.updateProjectError
	}
	return MemoryProject{ID: input.ProjectID, Name: input.Name, Description: input.Description, LifecycleStatus: input.LifecycleStatus, Revision: input.ExpectedRevision + 1}, nil
}
func (r *governanceTestRepository) GetConversationPolicy(context.Context, string) (ConversationMemoryPolicy, error) {
	return r.policy, r.policyError
}
func (r *governanceTestRepository) UpdateConversationPolicy(_ context.Context, input UpdateConversationPolicyInput) (ConversationMemoryPolicy, error) {
	r.updatedPolicy = input
	return ConversationMemoryPolicy{ConversationID: input.ConversationID, ProjectID: input.ProjectID, UseMode: input.UseMode, LearnMode: input.LearnMode, ScopeGeneration: input.ExpectedScopeGeneration + 1}, nil
}
func (r *governanceTestRepository) CreateGovernanceMemory(_ context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	r.createdMemory = input
	return GovernanceMemory{ID: input.MemoryID, Type: input.Candidate.Type, Content: input.Candidate.Content, Tags: input.Candidate.Tags, ScopeType: input.ScopeType, ProjectID: input.ProjectID, Sensitivity: input.Sensitivity}, nil
}
func (r *governanceTestRepository) UpdateGovernanceMemory(_ context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	r.updatedMemory = input
	return GovernanceMemory{ID: input.MemoryID, Revision: input.ExpectedRevision + 1}, nil
}
func (r *governanceTestRepository) DeleteGovernanceMemory(_ context.Context, input GovernanceMemoryDeleteInput) (MemoryDeletionProgress, error) {
	r.deletedMemory = input
	return MemoryDeletionProgress{ManifestID: input.ManifestID, MemoryID: input.MemoryID, ImmediateHidden: true}, nil
}
func (r *governanceTestRepository) GovernanceMemoryDetail(_ context.Context, memoryID string) (GovernanceMemoryDetail, error) {
	return GovernanceMemoryDetail{Memory: GovernanceMemory{ID: memoryID}}, nil
}
func (r *governanceTestRepository) DecideMemoryReview(_ context.Context, input MemoryReviewDecisionInput) (MemoryReviewDecisionResult, error) {
	r.reviewDecision = input
	status := "rejected"
	if input.Decision != "keep_current" && input.Decision != "reject" {
		status = "accepted"
	}
	return MemoryReviewDecisionResult{SuggestionID: input.SuggestionID, Decision: input.Decision, Status: status}, nil
}
func (r *governanceTestRepository) ListMessageActivities(_ context.Context, _ string, limit int) ([]MemoryActivity, error) {
	r.activityLimit = limit
	return append([]MemoryActivity(nil), r.activities...), nil
}

var _ GovernanceRepository = (*governanceTestRepository)(nil)
