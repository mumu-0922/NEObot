package agentcron

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/agentbroker"
)

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func(string) string
}

func (service *Service) GetTemplate(ctx context.Context, userID, templateID string) (Template, error) {
	if service == nil || service.repository == nil {
		return Template{}, ErrDatabaseRequired
	}
	if !uuidPattern.MatchString(userID) || !validID(templateID, "cron") {
		return Template{}, ErrInvalidInput
	}
	return service.repository.GetTemplate(ctx, userID, templateID)
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
		newID: func(prefix string) string {
			return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		},
	}
}

func (service *Service) CreateRevision(ctx context.Context, input CreateInput) (Template, bool, error) {
	if service == nil || service.repository == nil {
		return Template{}, false, ErrDatabaseRequired
	}
	if input.TemplateID == "" {
		input.TemplateID = service.newID("cron")
	}
	if input.Spec.Automation.ApprovalID == "" {
		input.Spec.Automation.ApprovalID = service.newID("approval")
	}
	if !validID(input.TemplateID, "cron") || input.ExpectedRevision < 0 ||
		!validID(input.Spec.Automation.ApprovalID, "approval") ||
		!validActor(input.Approval.ActorType, input.Approval.ActorID, true) || !validReason(input.Approval.ReasonCode) {
		return Template{}, false, ErrInvalidInput
	}
	now := service.now().UTC()
	spec, parsed, canonical, revisionFingerprint, err := prepareSpec(input.Spec, now)
	if err != nil {
		return Template{}, false, err
	}
	base := now
	if !input.ActivateAt.IsZero() && input.ActivateAt.After(base) {
		base = input.ActivateAt.UTC()
	}
	next := parsed.next(base)
	if next.IsZero() {
		return Template{}, false, ErrInvalidInput
	}
	prepared := preparedRevision{
		TemplateID: input.TemplateID, ExpectedRevision: input.ExpectedRevision,
		Spec: spec, CanonicalSpec: canonical, RevisionFingerprint: revisionFingerprint,
		NextTriggerAt: next, Approval: input.Approval, AuditEventID: service.newID("cron_event"),
	}
	return service.repository.CreateRevision(ctx, prepared)
}

func (service *Service) SetLifecycle(ctx context.Context, input LifecycleInput) (Template, error) {
	if service == nil || service.repository == nil {
		return Template{}, ErrDatabaseRequired
	}
	if !validID(input.TemplateID, "cron") || !uuidPattern.MatchString(input.UserID) || input.ExpectedRevision < 1 ||
		(input.To != TemplatePaused && input.To != TemplateDeleted) ||
		!validActor(input.ActorType, input.ActorID, false) || !validReason(input.ReasonCode) {
		return Template{}, ErrInvalidInput
	}
	return service.repository.SetLifecycle(ctx, preparedLifecycle{
		LifecycleInput: input, AuditEventID: service.newID("cron_event"),
	})
}

// Resume resumes a known frozen Template without claiming unrelated due work.
func (service *Service) Resume(ctx context.Context, template Template, approval Approval) (Template, error) {
	if service == nil || service.repository == nil {
		return Template{}, ErrDatabaseRequired
	}
	if template.State != TemplatePaused || template.CurrentRevision < 1 ||
		!validActor(approval.ActorType, approval.ActorID, false) || !validReason(approval.ReasonCode) {
		return Template{}, ErrInvalidInput
	}
	parsed, _, err := parseSchedule(template.Spec.Schedule)
	if err != nil {
		return Template{}, err
	}
	now := service.now().UTC()
	next := parsed.next(now)
	missed := 0
	if template.NextTriggerAt != nil && !template.NextTriggerAt.After(now) {
		missed = 1
	}
	return service.repository.SetLifecycle(ctx, preparedLifecycle{
		LifecycleInput: LifecycleInput{
			TemplateID: template.ID, UserID: template.UserID, ExpectedRevision: template.CurrentRevision,
			To: TemplateActive, ActorType: approval.ActorType, ActorID: approval.ActorID, ReasonCode: approval.ReasonCode,
		},
		NextTriggerAt: &next, MissedCount: missed, AuditEventID: service.newID("cron_event"),
	})
}

func (service *Service) RevokeApproval(ctx context.Context, input RevokeApprovalInput) (bool, error) {
	if service == nil || service.repository == nil {
		return false, ErrDatabaseRequired
	}
	if input.RevocationID == "" {
		input.RevocationID = service.newID("cron_revocation")
	}
	if input.AuditEventID == "" {
		input.AuditEventID = service.newID("cron_event")
	}
	if !validID(input.RevocationID, "cron_revocation") || !uuidPattern.MatchString(input.UserID) ||
		!validID(input.ApprovalID, "approval") || !validID(input.AuditEventID, "cron_event") ||
		!validActor(input.ActorType, input.ActorID, true) || !validReason(input.ReasonCode) {
		return false, ErrInvalidInput
	}
	return service.repository.RevokeApproval(ctx, input)
}

func (service *Service) RunCycle(ctx context.Context, request ClaimRequest) (CycleResult, error) {
	if service == nil || service.repository == nil {
		return CycleResult{}, ErrDatabaseRequired
	}
	if err := validateClaimRequest(request); err != nil {
		return CycleResult{}, err
	}
	request.Now = request.Now.UTC()
	claims, err := service.repository.ClaimDue(ctx, request)
	if err != nil {
		return CycleResult{}, err
	}
	result := CycleResult{ClaimedTemplates: len(claims)}
	for _, claim := range claims {
		decisions, next, planErr := planOccurrences(claim, request.Now)
		if planErr != nil {
			return result, planErr
		}
		for index := range decisions {
			decisions[index].TriggerID = service.newID("cron_trigger")
			decisions[index].AuditEventID = service.newID("cron_event")
			decisions[index].OccurrenceFingerprint = occurrenceFingerprint(
				claim.TemplateID, claim.Revision, decisions[index].ScheduledFor,
			)
		}
		materialized, advanceErr := service.repository.Advance(ctx, AdvanceInput{
			Claim: claim, ObservedAt: request.Now, NextTriggerAt: next, Decisions: decisions,
		})
		if advanceErr != nil {
			return result, advanceErr
		}
		result.Materialized += len(materialized)
	}
	triggers, err := service.repository.ClaimTriggers(ctx, request)
	if err != nil {
		return result, err
	}
	result.ClaimedTriggers = len(triggers)
	for _, trigger := range triggers {
		envelope, prepareErr := service.prepareEnqueue(trigger)
		if prepareErr != nil {
			return result, prepareErr
		}
		triggerResult, enqueueErr := service.repository.EnqueueTrigger(ctx, envelope)
		if enqueueErr != nil {
			retryAt := request.Now.Add(time.Duration(trigger.Spec.Policies.RetryBackoffSeconds) * time.Second)
			if releaseErr := service.repository.ReleaseTrigger(ctx, ReleaseInput{
				TriggerID: trigger.ID, ClaimOwner: trigger.ClaimOwner, ClaimGeneration: trigger.ClaimGeneration,
				ErrorCode: "SCHEDULER_UNAVAILABLE", RetryAt: retryAt, AuditEventID: service.newID("cron_event"),
			}); releaseErr != nil {
				return result, releaseErr
			}
			result.Failed++
			continue
		}
		switch triggerResult.State {
		case TriggerEnqueued:
			result.Enqueued++
		case TriggerSkipped:
			result.Skipped++
		case TriggerFailed:
			result.Failed++
		}
	}
	return result, nil
}

func (service *Service) Reconcile(ctx context.Context, now time.Time, limit int) (CleanupResult, error) {
	if service == nil || service.repository == nil {
		return CleanupResult{}, ErrDatabaseRequired
	}
	if now.IsZero() || limit < 1 || limit > 1000 {
		return CleanupResult{}, ErrInvalidInput
	}
	return service.repository.Reconcile(ctx, now.UTC(), limit)
}

func (service *Service) Prune(ctx context.Context, cutoff time.Time, limit int) (CleanupResult, error) {
	if service == nil || service.repository == nil {
		return CleanupResult{}, ErrDatabaseRequired
	}
	if cutoff.IsZero() || cutoff.After(service.now().Add(time.Minute)) || limit < 1 || limit > 1000 {
		return CleanupResult{}, ErrInvalidInput
	}
	return service.repository.Prune(ctx, cutoff.UTC(), limit)
}

func prepareSpec(input TemplateSpec, now time.Time) (TemplateSpec, parsedSchedule, json.RawMessage, string, error) {
	input.SchemaVersion = SchemaVersion
	input.Schedule.Calculator = CalculatorVersion
	parsed, expression, err := parseSchedule(input.Schedule)
	if err != nil {
		return TemplateSpec{}, parsedSchedule{}, nil, "", err
	}
	input.Schedule.Expression = expression
	if !uuidPattern.MatchString(input.Owner.UserID) || !validID(input.Owner.ProjectID, "project") ||
		!validID(input.Owner.AssistantID, "assistant") || !inputRefPattern.MatchString(input.Input.Ref) ||
		!fingerprintPattern.MatchString(input.Input.Fingerprint) || !validModel(input.Model) ||
		!uuidPattern.MatchString(input.Skill.InstallationID) || !uuidPattern.MatchString(input.Skill.AdmissionID) ||
		!fingerprintPattern.MatchString(input.Skill.PackageFingerprint) ||
		!fingerprintPattern.MatchString(input.Skill.RuntimeBundleFingerprint) ||
		!validID(input.Grant.GrantID, "grant") || !fingerprintPattern.MatchString(input.Grant.GrantFingerprint) ||
		!fingerprintPattern.MatchString(input.Grant.RegistryFingerprint) ||
		input.Grant.IssuedAt.IsZero() || input.Grant.IssuedAt.After(now) ||
		!input.Grant.ExpiresAt.After(now) || !input.Grant.ExpiresAt.After(input.Grant.IssuedAt) ||
		len(input.Secrets) > 25 || len(input.Steps) < 1 || len(input.Steps) > 32 {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	probe := agentbroker.CapabilityGrant{
		SchemaVersion: agentbroker.GrantVersion, GrantID: input.Grant.GrantID, Subject: input.Owner,
		Run:                      agentbroker.RunBinding{RunID: "run_0000000000000000", Depth: 0},
		PackageFingerprint:       input.Skill.PackageFingerprint,
		RuntimeBundleFingerprint: input.Skill.RuntimeBundleFingerprint,
		IssuedAt:                 input.Grant.IssuedAt.UTC(), ExpiresAt: input.Grant.ExpiresAt.UTC(),
		Capabilities: input.Grant.Capabilities, Egress: input.Egress, Secrets: input.Secrets, Budget: input.Budget,
	}
	if err := agentbroker.ValidateGrant(probe, now); err != nil {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	if input.Automation.Class != AutomationReadOnly && input.Automation.Class != AutomationBrokeredEffect {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	if input.Automation.Class == AutomationReadOnly {
		for _, capability := range input.Grant.Capabilities {
			if capability.Approval != agentbroker.ApprovalAutomatic {
				return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
			}
		}
	}
	policies := input.Policies
	if policies.Missed != MissedSkip && policies.Missed != MissedFireOnce && policies.Missed != MissedCatchUp ||
		policies.Overlap != OverlapSkip && policies.Overlap != OverlapBufferOne && policies.Overlap != OverlapAllow ||
		policies.CatchupWindowSeconds < 10 || time.Duration(policies.CatchupWindowSeconds)*time.Second > maxCatchupWindow ||
		policies.MaxCatchupRuns < 1 || policies.MaxCatchupRuns > 100 ||
		policies.MaxAttempts < 1 || policies.MaxAttempts > 10 ||
		policies.RetryBackoffSeconds < 1 || policies.RetryBackoffSeconds > 3600 {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	seenSteps := map[string]struct{}{}
	for _, step := range input.Steps {
		if !identifierPattern.MatchString(step.Kind) || len(step.Kind) > 64 {
			return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
		}
		if _, duplicate := seenSteps[step.Kind]; duplicate {
			return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
		}
		seenSteps[step.Kind] = struct{}{}
	}
	input.Grant.IssuedAt = input.Grant.IssuedAt.UTC()
	input.Grant.ExpiresAt = input.Grant.ExpiresAt.UTC()
	input.ScopeKeys = scopeKeys(input)
	if len(input.ScopeKeys)+1 > 32 {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	canonical, err := json.Marshal(input)
	if err != nil || len(canonical) > 256<<10 {
		return TemplateSpec{}, parsedSchedule{}, nil, "", ErrInvalidInput
	}
	return input, parsed, canonical, fingerprint("neo-cron-template-revision-v1", canonical), nil
}

func (service *Service) prepareEnqueue(trigger Trigger) (EnqueueEnvelope, error) {
	if trigger.State != TriggerClaimed || trigger.ClaimOwner == "" || trigger.ClaimGeneration < 1 {
		return EnqueueEnvelope{}, ErrInvalidInput
	}
	snapshot := struct {
		SchemaVersion            string                   `json:"schemaVersion"`
		Source                   string                   `json:"source"`
		Cron                     map[string]any           `json:"cron"`
		Input                    InputBinding             `json:"input"`
		Model                    ModelBinding             `json:"model"`
		Budget                   agentbroker.Budget       `json:"budget"`
		PackageFingerprint       string                   `json:"packageFingerprint"`
		RuntimeBundleFingerprint string                   `json:"runtimeBundleFingerprint"`
		GrantFingerprint         string                   `json:"grantFingerprint"`
		RegistryFingerprint      string                   `json:"registryFingerprint"`
		Egress                   agentbroker.EgressPolicy `json:"egress"`
		SecretRefs               []string                 `json:"secretRefs"`
	}{
		SchemaVersion: "neo.agent-snapshot/v1", Source: "cron",
		Cron: map[string]any{
			"templateId": trigger.TemplateID, "revision": trigger.Revision,
			"scheduledFor":          trigger.ScheduledFor.UTC().Format(time.RFC3339Nano),
			"occurrenceFingerprint": trigger.OccurrenceFingerprint,
		},
		Input: trigger.Spec.Input, Model: trigger.Spec.Model, Budget: trigger.Spec.Budget,
		PackageFingerprint:       trigger.Spec.Skill.PackageFingerprint,
		RuntimeBundleFingerprint: trigger.Spec.Skill.RuntimeBundleFingerprint,
		GrantFingerprint:         trigger.Spec.Grant.GrantFingerprint,
		RegistryFingerprint:      trigger.Spec.Grant.RegistryFingerprint,
		Egress:                   trigger.Spec.Egress,
	}
	for _, secret := range trigger.Spec.Secrets {
		snapshot.SecretRefs = append(snapshot.SecretRefs, secret.BrokerRef)
	}
	canonicalSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return EnqueueEnvelope{}, ErrInvalidInput
	}
	steps := make([]map[string]string, len(trigger.Spec.Steps))
	stepKinds := make([]string, len(trigger.Spec.Steps))
	for index, step := range trigger.Spec.Steps {
		steps[index] = map[string]string{"stepId": service.newID("step"), "kind": step.Kind}
		stepKinds[index] = step.Kind
	}
	stepsJSON, _ := json.Marshal(steps)
	runID := service.newID("run")
	snapshotID := service.newID("snapshot")
	snapshotFingerprint := fingerprint("neo-agent-snapshot-v1", canonicalSnapshot)
	requestBody, _ := json.Marshal(struct {
		Snapshot json.RawMessage `json:"snapshot"`
		Steps    []string        `json:"steps"`
		Scopes   []string        `json:"scopes"`
	}{canonicalSnapshot, stepKinds, append([]string(nil), trigger.Spec.ScopeKeys...)})
	eventIDs := make([]string, 3+2*len(steps))
	for index := range eventIDs {
		eventIDs[index] = service.newID("event")
	}
	return EnqueueEnvelope{
		Trigger: trigger, RunID: runID, SnapshotID: snapshotID,
		SnapshotFingerprint: snapshotFingerprint,
		RequestFingerprint:  fingerprint("neo-agent-enqueue-v1", requestBody),
		CanonicalSnapshot:   canonicalSnapshot, Steps: stepsJSON, EventIDs: eventIDs,
		AuditEventID: service.newID("cron_event"),
	}, nil
}

func validateClaimRequest(request ClaimRequest) error {
	if request.Owner == "" || strings.TrimSpace(request.Owner) != request.Owner || len(request.Owner) > 128 ||
		request.Now.IsZero() || request.LeaseDuration < 5*time.Second || request.LeaseDuration > 5*time.Minute ||
		request.Limit < 1 || request.Limit > 1000 {
		return ErrInvalidInput
	}
	return nil
}
