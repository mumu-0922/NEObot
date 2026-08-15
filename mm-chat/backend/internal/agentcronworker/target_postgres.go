package agentcronworker

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func VerifyPostgresTarget(ctx context.Context, database *sql.DB, plan Plan) error {
	if database == nil || ValidatePlan(plan) != nil {
		return ErrInvalidPlan
	}
	var activationID, templateID, userID, revisionFingerprint, planFingerprint string
	var revision int64
	var enabled bool
	var validFrom, validUntil time.Time
	err := database.QueryRowContext(ctx, `
SELECT activation_id,template_id,user_id::text,revision,revision_fingerprint,
  plan_fingerprint,valid_from,valid_until,enabled
FROM agent_cron_worker_get_target($1)
`, plan.ActivationID).Scan(&activationID, &templateID, &userID, &revision,
		&revisionFingerprint, &planFingerprint, &validFrom, &validUntil, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidPlan
	}
	if err != nil || activationID != plan.ActivationID || templateID != plan.TemplateID ||
		userID != plan.UserID || revision != plan.Revision ||
		revisionFingerprint != plan.RevisionFingerprint ||
		planFingerprint != plan.DocumentFingerprint ||
		!validFrom.Equal(plan.ValidFrom) || !validUntil.Equal(plan.ValidUntil) || !enabled {
		return ErrInvalidPlan
	}
	return nil
}
