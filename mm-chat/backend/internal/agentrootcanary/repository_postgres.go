package agentrootcanary

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type PostgresTerminalRepository struct{ db *sql.DB }

func NewPostgresTerminalRepository(db *sql.DB) *PostgresTerminalRepository {
	return &PostgresTerminalRepository{db: db}
}

// FinalizeCanceled terminalizes the durable Runner projection and appends the
// Attempt, Step and Run cancellation facts in one transaction. This removes
// the restart window in which the memory-only lease token could be lost after
// only part of the terminal chain committed.
func (repository *PostgresTerminalRepository) FinalizeCanceled(ctx context.Context, input TerminalInput) error {
	if repository == nil || repository.db == nil || input.SandboxID == "" {
		return ErrUnavailable
	}
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ErrUnavailable
	}
	defer func() { _ = tx.Rollback() }()
	for _, transition := range []struct{ expected, to, terminal string }{
		{"running", "stopping", ""}, {"stopping", "terminal", "canceled"},
	} {
		var accepted bool
		if err := tx.QueryRowContext(ctx, `SELECT agent_runner_update_sandbox($1,$2,$3,$4,$5,NULLIF($6,''))`,
			input.SandboxID, input.AttemptID, input.Generation, transition.expected, transition.to, transition.terminal,
		).Scan(&accepted); err != nil || !accepted {
			return fmt.Errorf("finalize Root canary Sandbox: %w", ErrUnavailable)
		}
	}
	tokenDigest := sha256.Sum256([]byte(input.LeaseToken))
	tokenHash := hex.EncodeToString(tokenDigest[:])
	for _, transition := range []struct {
		entity, stepID, attemptID, expected, to, reason string
		generation                                      int64
	}{
		{"attempt", input.StepID, input.AttemptID, "running", "canceled", "ROOT_CANARY_CANCELED", input.Generation},
		{"step", input.StepID, input.AttemptID, "running", "canceled", "ROOT_CANARY_CANCELED", input.Generation},
		{"run", "", "", "running", "canceled", "ROOT_CANARY_CANCELED", 0},
	} {
		var accepted bool
		eventID := "event_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err := tx.QueryRowContext(ctx, `
SELECT agent_orchestrator_transition(
  $1,$2::uuid,$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,''),NULLIF($8,''),
  $9,$10,$11,'orchestrator','g21.1-root-canary',$12,'{}'::jsonb
)`, eventID, input.UserID, input.RunID, transition.stepID, transition.attemptID,
			transition.generation, input.LeaseOwner, tokenHash, transition.entity,
			transition.expected, transition.to, transition.reason).Scan(&accepted); err != nil || !accepted {
			return fmt.Errorf("finalize Root canary %s: %w", transition.entity, ErrUnavailable)
		}
	}
	if err := tx.Commit(); err != nil {
		return ErrUnavailable
	}
	return nil
}
