package usermemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) GovernanceSnapshot(ctx context.Context) (GovernanceSnapshot, error) {
	snapshot, err := queryGovernanceJSON[GovernanceSnapshot](ctx, r, `
SELECT memory_governance_snapshot($1::uuid)
`, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return GovernanceSnapshot{}, err
	}
	l2Scene, err := queryGovernanceJSON[L2SceneGovernanceSnapshot](ctx, r, `
SELECT memory_governance_l2_scene_snapshot($1::uuid)
`, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return GovernanceSnapshot{}, err
	}
	if l2Scene.Scenes == nil {
		l2Scene.Scenes = []L2SceneGovernanceScene{}
	}
	snapshot.L2Scene = &l2Scene
	l3Persona, err := queryGovernanceJSON[L3PersonaGovernanceSnapshot](ctx, r, `
SELECT memory_governance_l3_persona_snapshot($1::uuid)
`, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return GovernanceSnapshot{}, err
	}
	snapshot.L3Persona = &l3Persona
	return snapshot, nil
}

func (r *PostgresRepository) CreateProject(ctx context.Context, input CreateProjectInput) (MemoryProject, error) {
	return queryGovernanceJSON[MemoryProject](ctx, r, `
SELECT memory_governance_create_project($1::uuid, $2::uuid, $3, $4)
`, auth.UserOrDevelopment(ctx).ID, input.ID, input.Name, input.Description)
}

func (r *PostgresRepository) UpdateProject(ctx context.Context, input UpdateProjectInput) (MemoryProject, error) {
	return queryGovernanceJSON[MemoryProject](ctx, r, `
SELECT memory_governance_update_project(
  $1::uuid, $2::uuid, $3::bigint, $4, $5, $6
)
`, auth.UserOrDevelopment(ctx).ID, input.ProjectID, input.ExpectedRevision,
		input.Name, input.Description, input.LifecycleStatus)
}

func (r *PostgresRepository) GetConversationPolicy(ctx context.Context, conversationID string) (ConversationMemoryPolicy, error) {
	return queryGovernanceJSON[ConversationMemoryPolicy](ctx, r, `
SELECT memory_governance_get_conversation_policy($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, conversationID)
}

func (r *PostgresRepository) UpdateConversationPolicy(ctx context.Context, input UpdateConversationPolicyInput) (ConversationMemoryPolicy, error) {
	return queryGovernanceJSON[ConversationMemoryPolicy](ctx, r, `
SELECT memory_governance_update_conversation_policy(
  $1::uuid, $2::uuid, $3::bigint, NULLIF($4, '')::uuid, $5, $6
)
`, auth.UserOrDevelopment(ctx).ID, input.ConversationID,
		input.ExpectedScopeGeneration, input.ProjectID, input.UseMode, input.LearnMode)
}

func (r *PostgresRepository) CreateGovernanceMemory(ctx context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	return queryGovernanceJSON[GovernanceMemory](ctx, r, `
SELECT memory_governance_create_memory(
  $1::uuid, $2::uuid, $3, $4, $5, $6::smallint, $7,
  $8, NULLIF($9, '')::uuid, NULLIF($10, '')::uuid, $11
)
`, auth.UserOrDevelopment(ctx).ID, input.MemoryID, input.Candidate.Type,
		input.Candidate.Content, normalizeSearchText(input.Candidate.Content),
		input.Candidate.Importance, input.Candidate.Tags, input.ScopeType,
		input.ProjectID, input.ConversationID, input.Sensitivity)
}

func (r *PostgresRepository) UpdateGovernanceMemory(ctx context.Context, input GovernanceMemoryMutationInput) (GovernanceMemory, error) {
	return queryGovernanceJSON[GovernanceMemory](ctx, r, `
SELECT memory_governance_update_memory(
  $1::uuid, $2::uuid, $3::bigint, $4, $5, $6, $7::smallint, $8,
  $9, NULLIF($10, '')::uuid, NULLIF($11, '')::uuid, $12
)
`, auth.UserOrDevelopment(ctx).ID, input.MemoryID, input.ExpectedRevision,
		input.Candidate.Type, input.Candidate.Content,
		normalizeSearchText(input.Candidate.Content), input.Candidate.Importance,
		input.Candidate.Tags, input.ScopeType, input.ProjectID,
		input.ConversationID, input.Sensitivity)
}

func (r *PostgresRepository) DeleteGovernanceMemory(ctx context.Context, input GovernanceMemoryDeleteInput) (MemoryDeletionProgress, error) {
	return queryGovernanceJSON[MemoryDeletionProgress](ctx, r, `
SELECT memory_governance_delete_memory(
  $1::uuid, $2::uuid, $3::bigint, $4::uuid, $5::uuid, $6::uuid, $7::uuid
)
`, auth.UserOrDevelopment(ctx).ID, input.MemoryID, input.ExpectedRevision,
		input.EventID, input.JobID, input.TombstoneID, input.ManifestID)
}

func (r *PostgresRepository) GovernanceMemoryDetail(ctx context.Context, memoryID string) (GovernanceMemoryDetail, error) {
	return queryGovernanceJSON[GovernanceMemoryDetail](ctx, r, `
SELECT memory_governance_memory_detail($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, memoryID)
}

func (r *PostgresRepository) GovernanceL2SceneDetail(
	ctx context.Context,
	sceneID string,
) (L2SceneGovernanceDetail, error) {
	detail, err := queryGovernanceJSON[L2SceneGovernanceDetail](ctx, r, `
SELECT memory_governance_l2_scene_detail($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, sceneID)
	if err != nil {
		return L2SceneGovernanceDetail{}, err
	}
	if detail.Members == nil {
		detail.Members = []L2SceneGovernanceMember{}
	}
	for index := range detail.Members {
		if detail.Members[index].Evidence == nil {
			detail.Members[index].Evidence = []MemoryEvidence{}
		}
	}
	return detail, nil
}

func (r *PostgresRepository) SetGovernanceL2SceneEnabled(
	ctx context.Context,
	input L2SceneEnabledInput,
) (L2SceneGovernanceScene, error) {
	return queryGovernanceJSON[L2SceneGovernanceScene](ctx, r, `
SELECT memory_governance_set_l2_scene_enabled(
  $1::uuid, $2::uuid, $3::bigint, $4
)
`, auth.UserOrDevelopment(ctx).ID, input.SceneID, input.ExpectedRevision,
		input.Enabled)
}

func (r *PostgresRepository) RebuildGovernanceL2Scene(
	ctx context.Context,
	sceneID string,
) (L2SceneRebuildResult, error) {
	return queryGovernanceJSON[L2SceneRebuildResult](ctx, r, `
SELECT memory_governance_rebuild_l2_scene($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, sceneID)
}

func (r *PostgresRepository) RebuildGovernanceL2Scenes(
	ctx context.Context,
) (L2SceneRebuildResult, error) {
	return queryGovernanceJSON[L2SceneRebuildResult](ctx, r, `
SELECT memory_governance_rebuild_l2_scenes($1::uuid)
`, auth.UserOrDevelopment(ctx).ID)
}

func (r *PostgresRepository) GovernanceL3PersonaDetail(
	ctx context.Context,
	personaID string,
) (L3PersonaGovernanceDetail, error) {
	detail, err := queryGovernanceJSON[L3PersonaGovernanceDetail](ctx, r, `
SELECT memory_governance_l3_persona_detail($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, personaID)
	if err != nil {
		return L3PersonaGovernanceDetail{}, err
	}
	if detail.Members == nil {
		detail.Members = []L3PersonaGovernanceMember{}
	}
	for index := range detail.Members {
		if detail.Members[index].Evidence == nil {
			detail.Members[index].Evidence = []MemoryEvidence{}
		}
	}
	return detail, nil
}

func (r *PostgresRepository) SetGovernanceL3PersonaEnabled(
	ctx context.Context,
	input L3PersonaEnabledInput,
) (L3PersonaGovernancePersona, error) {
	return queryGovernanceJSON[L3PersonaGovernancePersona](ctx, r, `
SELECT memory_governance_set_l3_persona_enabled(
  $1::uuid, $2::uuid, $3::bigint, $4
)
`, auth.UserOrDevelopment(ctx).ID, input.PersonaID, input.ExpectedRevision,
		input.Enabled)
}

func (r *PostgresRepository) RebuildGovernanceL3Persona(
	ctx context.Context,
	personaID string,
) (L3PersonaRebuildResult, error) {
	return queryGovernanceJSON[L3PersonaRebuildResult](ctx, r, `
SELECT memory_governance_rebuild_l3_persona($1::uuid, $2::uuid)
`, auth.UserOrDevelopment(ctx).ID, personaID)
}

func (r *PostgresRepository) RebuildGovernanceL3Personas(
	ctx context.Context,
) (L3PersonaRebuildResult, error) {
	return queryGovernanceJSON[L3PersonaRebuildResult](ctx, r, `
SELECT memory_governance_rebuild_l3_personas($1::uuid)
`, auth.UserOrDevelopment(ctx).ID)
}

func (r *PostgresRepository) DecideMemoryReview(ctx context.Context, input MemoryReviewDecisionInput) (MemoryReviewDecisionResult, error) {
	return queryGovernanceJSON[MemoryReviewDecisionResult](ctx, r, `
SELECT memory_governance_decide_review(
  $1::uuid, $2::uuid, $3::uuid, $4, NULLIF($5, '')::uuid,
  NULLIF($6, ''), NULLIF($7, ''), $8
)
`, auth.UserOrDevelopment(ctx).ID, input.SuggestionID, input.DecisionID,
		input.Decision, input.MemoryID, input.EditedContent,
		input.NormalizedContent, input.DecisionHash)
}

func (r *PostgresRepository) ListMessageActivities(ctx context.Context, assistantMessageID string, limit int) ([]MemoryActivity, error) {
	payloads, err := queryGovernanceJSON[[]governanceMemoryActivityPayload](ctx, r, `
	SELECT memory_governance_list_message_activities(
	  $1::uuid, $2::uuid, $3::integer
	)
	`, auth.UserOrDevelopment(ctx).ID, assistantMessageID, limit)
	if err != nil {
		return nil, err
	}
	activities := make([]MemoryActivity, 0, len(payloads))
	for _, payload := range payloads {
		activities = append(activities, payload.activity())
	}
	return activities, nil
}

type governanceMemoryActivityPayload struct {
	ID                 string `json:"id"`
	AssistantMessageID string `json:"assistantMessageId"`
	Ordinal            int    `json:"ordinal"`
	SubjectType        string `json:"subjectType"`
	SubjectID          string `json:"subjectId"`
	SubjectRevision    *int64 `json:"subjectRevision"`
	Action             string `json:"action"`
	Status             string `json:"status"`
	ReasonCode         string `json:"reasonCode"`
	UndoKind           string `json:"undoKind"`
	UndoStatus         string `json:"undoStatus"`
	SourceKind         string `json:"sourceKind"`
	ScopeType          string `json:"scopeType"`
	MemoryType         string `json:"memoryType"`
	MemoryContent      string `json:"memoryContent"`
	MemoryRevision     *int64 `json:"memoryRevision"`
	MemoryDeleted      bool   `json:"memoryDeleted"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

func (payload governanceMemoryActivityPayload) activity() MemoryActivity {
	return MemoryActivity{
		ID: payload.ID, AssistantMessageID: payload.AssistantMessageID,
		Ordinal: payload.Ordinal, SubjectType: payload.SubjectType,
		SubjectID: payload.SubjectID, SubjectRevision: payload.SubjectRevision,
		Action: payload.Action, Status: payload.Status, ReasonCode: payload.ReasonCode,
		UndoKind: payload.UndoKind, UndoStatus: payload.UndoStatus,
		SourceKind: payload.SourceKind, ScopeType: payload.ScopeType,
		MemoryType: payload.MemoryType, MemoryContent: payload.MemoryContent,
		MemoryRevision: payload.MemoryRevision, MemoryDeleted: payload.MemoryDeleted,
		CreatedAt: time.UnixMilli(payload.CreatedAt).UTC(),
		UpdatedAt: time.UnixMilli(payload.UpdatedAt).UTC(),
	}
}

func queryGovernanceJSON[T any](
	ctx context.Context,
	repository *PostgresRepository,
	query string,
	args ...any,
) (T, error) {
	var zero T
	if err := repository.requireDB(); err != nil {
		return zero, err
	}
	var encoded []byte
	if err := repository.db.QueryRowContext(ctx, query, args...).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, ErrMemoryNotFound
		}
		return zero, mapGovernancePostgresError(err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		return zero, fmt.Errorf("decode memory governance response: %w", err)
	}
	return result, nil
}

func mapGovernancePostgresError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("memory governance query: %w", err)
	}
	switch strings.TrimSpace(postgresError.Message) {
	case "MEMORY_L3_PERSONA_NOT_FOUND":
		return ErrMemoryL3PersonaNotFound
	case "MEMORY_L3_PERSONA_REVISION_STALE":
		return validation(postgresError.Message, "memory L3 Persona changed; reload and retry")
	case "MEMORY_L3_PERSONA_MUTATION_INVALID":
		return validation(postgresError.Message, "memory L3 Persona input is invalid")
	case "MEMORY_L2_SCENE_NOT_FOUND":
		return ErrMemoryL2SceneNotFound
	case "MEMORY_L2_SCENE_REVISION_STALE":
		return validation(postgresError.Message, "memory L2 Scene changed; reload and retry")
	case "MEMORY_L2_SCENE_MUTATION_INVALID":
		return validation(postgresError.Message, "memory L2 Scene input is invalid")
	case "MEMORY_GOVERNANCE_PROJECT_NOT_FOUND":
		return ErrMemoryProjectNotFound
	case "MEMORY_GOVERNANCE_CONVERSATION_NOT_FOUND":
		return ErrConversationPolicyNotFound
	case "MEMORY_GOVERNANCE_MEMORY_NOT_FOUND":
		return ErrMemoryNotFound
	case "MEMORY_GOVERNANCE_REVIEW_NOT_FOUND":
		return ErrMemoryReviewNotFound
	case "MEMORY_GOVERNANCE_REVISION_STALE", "MEMORY_GOVERNANCE_SCOPE_STALE",
		"MEMORY_GOVERNANCE_REVIEW_STALE", "MEMORY_GOVERNANCE_REPLAY_CONFLICT":
		return validation(postgresError.Message, "memory governance state changed; reload and retry")
	case "MEMORY_GOVERNANCE_EXACT_CONFLICT":
		return ErrMemoryConflict
	case "MEMORY_GOVERNANCE_PROJECT_INVALID", "MEMORY_GOVERNANCE_PROJECT_LIMIT",
		"MEMORY_GOVERNANCE_POLICY_INVALID", "MEMORY_GOVERNANCE_MEMORY_INVALID",
		"MEMORY_GOVERNANCE_SENSITIVE_DISABLED", "MEMORY_GOVERNANCE_ACTIVITY_INVALID":
		return validation(postgresError.Message, "memory governance input is invalid")
	default:
		if postgresError.Code == "23505" {
			return ErrMemoryConflict
		}
		return fmt.Errorf("memory governance query: %w", err)
	}
}

var _ GovernanceRepository = (*PostgresRepository)(nil)
var _ L2SceneGovernanceRepository = (*PostgresRepository)(nil)
var _ L3PersonaGovernanceRepository = (*PostgresRepository)(nil)
