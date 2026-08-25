package resourceorchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MutationOutcomeSuccess = "success"
	MutationOutcomeFailed  = "failed"
)

type MutationAudit struct {
	ID               string
	UserID           string
	ConversationID   string
	EntryPoint       string
	Kind             string
	Action           string
	CandidateID      string
	Version          string
	ExactRevision    string
	ResultResourceID string
	Outcome          string
	ErrorCode        string
	OccurredAt       time.Time
}

type MutationAuditor interface {
	RecordMutation(context.Context, MutationAudit) error
}

type PostgresMutationAuditor struct {
	db *sql.DB
}

func NewPostgresMutationAuditor(db *sql.DB) *PostgresMutationAuditor {
	return &PostgresMutationAuditor{db: db}
}

func (auditor *PostgresMutationAuditor) RecordMutation(
	ctx context.Context,
	audit MutationAudit,
) error {
	if auditor == nil || auditor.db == nil {
		return ErrAuditUnavailable
	}
	metadata, err := json.Marshal(map[string]string{
		"action":           strings.TrimSpace(audit.Action),
		"candidateId":      strings.TrimSpace(audit.CandidateID),
		"entryPoint":       strings.TrimSpace(audit.EntryPoint),
		"errorCode":        strings.TrimSpace(audit.ErrorCode),
		"exactRevision":    strings.TrimSpace(audit.ExactRevision),
		"resultResourceId": strings.TrimSpace(audit.ResultResourceID),
		"version":          strings.TrimSpace(audit.Version),
	})
	if err != nil {
		return fmt.Errorf("encode resource mutation audit: %w", err)
	}
	_, err = auditor.db.ExecContext(ctx, `
INSERT INTO audit_logs (
  id, actor_user_id, conversation_id, actor_type, action, resource_type,
  request_id, outcome, metadata, created_at, updated_at
) VALUES ($1,$2,$3,'user',$4,$5,$6,$7,$8,$9,$9)
`, audit.ID, audit.UserID, audit.ConversationID, "resource."+audit.Action,
		audit.Kind, audit.ID, audit.Outcome, metadata, audit.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("record resource mutation audit: %w", err)
	}
	return nil
}

func defaultMutationAuditID() string { return uuid.NewString() }
