package agentcron

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
)

const (
	testUserID         = "123e4567-e89b-42d3-a456-426614174000"
	testInstallationID = "223e4567-e89b-42d3-a456-426614174000"
	testAdmissionID    = "323e4567-e89b-42d3-a456-426614174000"
	testPackage        = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testRuntime        = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	testGrant          = "sha256:5555555555555555555555555555555555555555555555555555555555555555"
	testRegistry       = "sha256:8888888888888888888888888888888888888888888888888888888888888888"
)

func testSpec(now time.Time) TemplateSpec {
	return TemplateSpec{
		Owner:    agentbroker.Subject{UserID: testUserID, ProjectID: "project_01234567", AssistantID: "assistant_01234567"},
		Input:    InputBinding{Ref: "input_ref_0123456789abcdef", Fingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Schedule: ScheduleBinding{Expression: "*/5 * * * *", Timezone: "Etc/UTC"},
		Model:    ModelBinding{Provider: "openai", ModelID: "gpt-5.4"},
		Budget:   agentbroker.Budget{MaxWallSeconds: 300, MaxModelTokens: 20_000, MaxToolCalls: 100, MaxArtifactBytes: 8 << 20},
		Skill: SkillBinding{
			InstallationID: testInstallationID, AdmissionID: testAdmissionID,
			PackageFingerprint: testPackage, RuntimeBundleFingerprint: testRuntime,
		},
		Grant: GrantBinding{
			GrantID: "grant_0123456789abcdef", GrantFingerprint: testGrant, RegistryFingerprint: testRegistry,
			IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
			Capabilities: []agentbroker.Capability{{
				Capability: "workspace.read", Actions: []string{"read"},
				Resources: agentbroker.Selector{Kind: "prefix", Values: []string{"project/"}},
				Approval:  agentbroker.ApprovalAutomatic, MaxCalls: 100,
			}},
		},
		Egress:     agentbroker.EgressPolicy{Mode: "none", Rules: []agentbroker.EgressRule{}},
		Secrets:    []agentbroker.SecretGrant{},
		Automation: AutomationBinding{Class: AutomationReadOnly, ApprovalID: "approval_0123456789abcdef"},
		Policies: SchedulingPolicies{
			Missed: MissedFireOnce, CatchupWindowSeconds: 3600, MaxCatchupRuns: 10,
			Overlap: OverlapBufferOne, MaxAttempts: 3, RetryBackoffSeconds: 30,
		},
		Steps: []StepBinding{{Kind: "skill.execute"}},
	}
}

func TestPrepareSpecStrictScheduleAndFingerprint(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	input := testSpec(now)
	input.Schedule.Expression = "  */5   * * * * "
	prepared, _, canonical, first, err := prepareSpec(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Schedule.Expression != "*/5 * * * *" || prepared.Schedule.Calculator != CalculatorVersion {
		t.Fatalf("schedule was not canonicalized: %+v", prepared.Schedule)
	}
	if len(prepared.ScopeKeys) != 6 || first != fingerprint("neo-cron-template-revision-v1", canonical) {
		t.Fatalf("frozen scope/fingerprint mismatch: %+v %s", prepared.ScopeKeys, first)
	}
	changed := input
	changed.Model.ModelID = "gpt-5.5"
	_, _, _, second, err := prepareSpec(changed, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("model edit did not create a new revision fingerprint")
	}
}

func TestPrepareSpecRejectsAmbiguousOrWidenedAutomation(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*TemplateSpec){
		"embedded timezone": func(spec *TemplateSpec) { spec.Schedule.Expression = "CRON_TZ=UTC */5 * * * *" },
		"seconds":           func(spec *TemplateSpec) { spec.Schedule.Expression = "0 */5 * * * *" },
		"descriptor":        func(spec *TemplateSpec) { spec.Schedule.Expression = "@daily" },
		"local timezone":    func(spec *TemplateSpec) { spec.Schedule.Timezone = "Local" },
		"unbounded catchup": func(spec *TemplateSpec) { spec.Policies.MaxCatchupRuns = 101 },
		"buffer all":        func(spec *TemplateSpec) { spec.Policies.Overlap = "buffer_all" },
		"interactive read": func(spec *TemplateSpec) {
			spec.Grant.Capabilities[0].Approval = agentbroker.ApprovalOnce
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := testSpec(now)
			mutate(&spec)
			if _, _, _, _, err := prepareSpec(spec, now); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v", err)
			}
		})
	}
	brokered := testSpec(now)
	brokered.Automation.Class = AutomationBrokeredEffect
	brokered.Grant.Capabilities[0].Approval = agentbroker.ApprovalOnce
	if _, _, _, _, err := prepareSpec(brokered, now); err != nil {
		t.Fatalf("brokered-effect automation was rejected: %v", err)
	}
}

func TestScheduleDSTGapAndOverlap(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	spring, _, err := parseSchedule(ScheduleBinding{
		Expression: "30 2 * * *", Timezone: "America/New_York", Calculator: CalculatorVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	springStart := time.Date(2026, 3, 7, 23, 0, 0, 0, location)
	springNext := spring.next(springStart)
	if local := springNext.In(location); local.Year() != 2026 || local.Month() != time.March || local.Day() != 9 || local.Hour() != 2 || local.Minute() != 30 {
		t.Fatalf("spring gap was not skipped: %s", local)
	}

	fall, _, err := parseSchedule(ScheduleBinding{
		Expression: "30 1 * * *", Timezone: "America/New_York", Calculator: CalculatorVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := fall.next(time.Date(2026, 10, 31, 23, 0, 0, 0, location))
	second := fall.next(first)
	if first.UTC() != time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC) ||
		second.UTC() != time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC) {
		t.Fatalf("fall overlap did not produce two UTC instants: %s %s", first, second)
	}
}

func TestPlanOccurrencesPoliciesAreBounded(t *testing.T) {
	observed := time.Date(2026, 8, 14, 10, 5, 0, 0, time.UTC)
	base := DueClaim{
		TemplateID: "cron_0123456789abcdef", Revision: 1,
		NextTriggerAt: observed.Add(-5 * time.Minute), Spec: testSpec(observed),
	}
	base.Spec.Schedule = ScheduleBinding{Expression: "* * * * *", Timezone: "Etc/UTC", Calculator: CalculatorVersion}
	base.Spec.Policies.CatchupWindowSeconds = 600

	base.Spec.Policies.Missed = MissedSkip
	decisions, next, err := planOccurrences(base, observed)
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionCounts(t, decisions, 1, 1)
	if !next.Equal(observed.Add(time.Minute)) {
		t.Fatalf("unexpected next cursor: %s", next)
	}

	base.Spec.Policies.Missed = MissedFireOnce
	decisions, _, err = planOccurrences(base, observed)
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionCounts(t, decisions, 1, 1)

	base.Spec.Policies.Missed = MissedCatchUp
	base.Spec.Policies.MaxCatchupRuns = 3
	decisions, _, err = planOccurrences(base, observed)
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionCounts(t, decisions, 3, 1)
	var pending []time.Time
	for _, decision := range decisions {
		if decision.State == TriggerPending {
			pending = append(pending, decision.ScheduledFor)
		}
	}
	if !pending[0].Equal(observed.Add(-2*time.Minute)) || !pending[2].Equal(observed) {
		t.Fatalf("catchup did not retain latest bounded occurrences: %+v", pending)
	}
}

func TestPlanOccurrencesSummarizesExpiredWindow(t *testing.T) {
	observed := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	claim := DueClaim{
		TemplateID: "cron_0123456789abcdef", Revision: 1,
		NextTriggerAt: observed.Add(-24 * time.Hour), Spec: testSpec(observed),
	}
	claim.Spec.Schedule = ScheduleBinding{Expression: "* * * * *", Timezone: "Etc/UTC", Calculator: CalculatorVersion}
	claim.Spec.Policies.Missed = MissedFireOnce
	claim.Spec.Policies.CatchupWindowSeconds = 600
	decisions, _, err := planOccurrences(claim, observed)
	if err != nil {
		t.Fatal(err)
	}
	if !decisions[0].CountTruncated || decisions[0].ReasonCode != "MISSED_WINDOW" {
		t.Fatalf("old outage was not summarized: %+v", decisions)
	}
}

func assertDecisionCounts(t *testing.T, decisions []OccurrenceDecision, pending, skipped int) {
	t.Helper()
	var gotPending, gotSkipped int
	for _, decision := range decisions {
		switch decision.State {
		case TriggerPending:
			gotPending++
		case TriggerSkipped:
			gotSkipped++
		}
	}
	if gotPending != pending || gotSkipped != skipped {
		body, _ := json.Marshal(decisions)
		t.Fatalf("got pending=%d skipped=%d, want %d/%d: %s", gotPending, gotSkipped, pending, skipped, body)
	}
}

func TestServiceRequiresRepository(t *testing.T) {
	service := NewService(nil)
	if _, _, err := service.CreateRevision(context.Background(), CreateInput{}); !errors.Is(err, ErrDatabaseRequired) {
		t.Fatalf("got %v", err)
	}
}
