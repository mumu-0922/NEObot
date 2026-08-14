package agentlearning

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/skillsupply"
	"neo-chat/mm-chat/backend/internal/storage"
)

func TestPostgresAgentLearningDraftChecksPromotionCleanup(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	database := stdlib.OpenDB(*config)
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	userID := uuid.NewString()
	mustLearningExec(t, ctx, database,
		`INSERT INTO users(id,email,display_name) VALUES($1,$2,'Agent Learning fixture')`,
		userID, "agent-learning-"+userID+"@example.test")
	objects, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	baseArchive := learningArchive(t, "1.2.3", "Base instructions.", "func TestBase() {}", "")
	base := seedLearningBase(t, ctx, database, objects, userID, baseArchive)

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(database))
	runResult, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{
		UserID: userID, IdempotencyKey: "learning-source-" + uuid.NewString(),
		Snapshot: json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","depth":0,"packageFingerprint":"` +
			base.PackageFingerprint + `"}`),
		Steps: []agentorchestrator.StepPlan{{Kind: "learn"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustLearningExec(t, ctx, database, `UPDATE agent_runs SET state='succeeded',terminal_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, runResult.Run.ID)
	var eventID string
	mustLearningQuery(t, ctx, database,
		`SELECT id FROM agent_run_events WHERE run_id=$1 ORDER BY sequence LIMIT 1`, runResult.Run.ID).Scan(&eventID)

	proposedArchive := learningArchive(t, "1.2.4", "Improved instructions.", "func TestImproved() {}", "")
	baseValidated := mustValidateLearning(t, baseArchive, "integration-base")
	proposedValidated := mustValidateLearning(t, proposedArchive, "integration-proposed")
	changed, err := changedPaths(baseValidated.CanonicalArchive, proposedValidated.CanonicalArchive)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []EvidenceRef{
		{Kind: EvidenceSourcePackage, Ref: base.PackageFingerprint,
			Fingerprint: base.PackageFingerprint, Paths: []string{"*"}},
		{Kind: EvidenceRunEvent, Ref: eventID,
			Fingerprint: "sha256:" + repeatHex("7"), Paths: changed},
	}
	repository := NewPostgresRepository(database)
	service := NewService(WithRepository(repository), WithObjectStore(objects),
		WithAdministratorUserID(userID), WithLearningEnabled(true),
		WithCheckers(StaticChecker{}, passChecker{suite: "8"}, passChecker{suite: "9"}))
	draft, created, err := service.Propose(ctx, ProposeInput{UserID: userID,
		SourceRunID: runResult.Run.ID, BasePackageFingerprint: base.PackageFingerprint,
		Archive: proposedArchive, Evidence: evidence})
	if err != nil || !created || draft.State != StateQuarantined {
		t.Fatalf("Propose = %#v created=%v err=%v", draft, created, err)
	}
	replayed, replayCreated, err := service.Propose(ctx, ProposeInput{UserID: userID,
		SourceRunID: runResult.Run.ID, BasePackageFingerprint: base.PackageFingerprint,
		Archive: proposedArchive, Evidence: evidence})
	if err != nil || replayCreated || replayed.ID != draft.ID {
		t.Fatalf("Propose replay = %#v created=%v err=%v", replayed, replayCreated, err)
	}

	claimRequest := ClaimRequest{Owner: "learning-checker-a", Now: time.Now().UTC(),
		LeaseDuration: 30 * time.Second, Limit: 10}
	checked, err := service.RunChecks(ctx, claimRequest)
	if err != nil || len(checked) != 1 || checked[0].State != StateReviewable || checked[0].Revision != 2 {
		t.Fatalf("RunChecks = %#v err=%v", checked, err)
	}
	var checkCount, candidateCount int
	mustLearningQuery(t, ctx, database,
		`SELECT count(*) FROM agent_learning_check_results WHERE draft_id=$1`, draft.ID).Scan(&checkCount)
	mustLearningQuery(t, ctx, database,
		`SELECT count(*) FROM skill_package_candidates WHERE source_type='learning'`).Scan(&candidateCount)
	if checkCount != 3 || candidateCount != 0 {
		t.Fatalf("check count=%d candidate count=%d", checkCount, candidateCount)
	}

	diff, err := service.GetDiff(ctx, userID, draft.ID)
	if err != nil || len(diff) != 3 {
		t.Fatalf("GetDiff = %#v err=%v", diff, err)
	}
	review := ReviewInput{DraftID: draft.ID, ExpectedRevision: 2,
		DraftFingerprint:           draft.DraftFingerprint,
		ProposedPackageFingerprint: draft.Spec.ProposedPackageFingerprint,
		ReasonCode:                 "LEARNING_DRAFT_APPROVED"}
	promotion, err := service.Promote(ctx, userID, review)
	if err != nil || !promotion.Created || promotion.PackageFingerprint != draft.Spec.ProposedPackageFingerprint {
		t.Fatalf("Promote = %#v err=%v", promotion, err)
	}
	promotionReplay, err := service.Promote(ctx, userID, review)
	if err != nil || promotionReplay.Created || promotionReplay.AdmissionID != promotion.AdmissionID {
		t.Fatalf("Promote replay = %#v err=%v", promotionReplay, err)
	}
	mustLearningQuery(t, ctx, database,
		`SELECT count(*) FROM skill_package_candidates WHERE id=$1::uuid AND status='admitted' AND source_type='learning'`,
		promotion.AdmissionID).Scan(&candidateCount)
	if candidateCount != 1 {
		t.Fatalf("promoted candidate count=%d", candidateCount)
	}
	var baseVersion string
	mustLearningQuery(t, ctx, database,
		`SELECT version FROM skill_package_versions WHERE package_fingerprint=$1`, base.PackageFingerprint).Scan(&baseVersion)
	if baseVersion != "1.2.3" {
		t.Fatalf("base version mutated to %s", baseVersion)
	}

	// Stale check generations cannot publish after reclaim.
	secondRunID, secondEventID := seedLearningRun(t, ctx, database, orchestrator, userID, base.PackageFingerprint)
	second, _, err := service.Propose(ctx, ProposeInput{UserID: userID, SourceRunID: secondRunID,
		BasePackageFingerprint: base.PackageFingerprint,
		Archive:                learningArchive(t, "1.2.5", "Second improvement.", "func TestSecond() {}", ""),
		Evidence: []EvidenceRef{
			{Kind: EvidenceSourcePackage, Ref: base.PackageFingerprint, Fingerprint: base.PackageFingerprint, Paths: []string{"*"}},
			{Kind: EvidenceRunEvent, Ref: secondEventID, Fingerprint: "sha256:" + repeatHex("6"),
				Paths: []string{"SKILL.md", "neo.runtime.json", "tests/learning_test.go"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldClaims, err := repository.ClaimChecks(ctx, ClaimRequest{Owner: "old-checker", Now: time.Now().UTC(), LeaseDuration: 5 * time.Second, Limit: 10})
	if err != nil || len(oldClaims) != 1 || oldClaims[0].ID != second.ID {
		t.Fatalf("old claim = %#v err=%v", oldClaims, err)
	}
	mustLearningExec(t, ctx, database, `UPDATE agent_learning_drafts SET check_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, second.ID)
	reconciled, err := repository.Reconcile(ctx, time.Now().UTC(), 10)
	if err != nil || reconciled.ChecksReclaimed != 1 {
		t.Fatalf("Reconcile stale check = %#v err=%v", reconciled, err)
	}
	newClaims, err := repository.ClaimChecks(ctx, ClaimRequest{Owner: "new-checker", Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 10})
	if err != nil || len(newClaims) != 1 || newClaims[0].ClaimGeneration <= oldClaims[0].ClaimGeneration {
		t.Fatalf("new claim = %#v err=%v", newClaims, err)
	}
	_, err = repository.CompleteChecks(ctx, oldClaims[0], passingReceipts(oldClaims[0], "a"))
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale CompleteChecks error=%v", err)
	}
	_, err = repository.ReleaseCheck(ctx, oldClaims[0], "CHECK_UNAVAILABLE", time.Now().Add(time.Minute))
	if !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale ReleaseCheck error=%v", err)
	}
	currentClaim := newClaims[0]
	for expectedAttempts := 2; expectedAttempts <= 3; expectedAttempts++ {
		mustLearningExec(t, ctx, database,
			`UPDATE agent_learning_drafts SET check_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, second.ID)
		reconciled, err = repository.Reconcile(ctx, time.Now().UTC(), 10)
		if err != nil || reconciled.ChecksReclaimed != 1 {
			t.Fatalf("Reconcile attempt %d = %#v err=%v", expectedAttempts, reconciled, err)
		}
		current, err := repository.GetDraft(ctx, userID, second.ID)
		if err != nil || current.CheckAttempts != expectedAttempts {
			t.Fatalf("reconciled Draft attempt %d = %#v err=%v", expectedAttempts, current, err)
		}
		if expectedAttempts == 3 {
			if current.State != StateCheckFailed {
				t.Fatalf("exhausted Draft state=%s", current.State)
			}
			claims, claimErr := repository.ClaimChecks(ctx, ClaimRequest{Owner: "forbidden-fourth-checker",
				Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 10})
			if claimErr != nil || len(claims) != 0 {
				t.Fatalf("claim after exhaustion = %#v err=%v", claims, claimErr)
			}
			continue
		}
		claims, claimErr := repository.ClaimChecks(ctx, ClaimRequest{Owner: "bounded-checker",
			Now: time.Now().UTC(), LeaseDuration: 30 * time.Second, Limit: 10})
		if claimErr != nil || len(claims) != 1 || claims[0].ClaimGeneration <= currentClaim.ClaimGeneration {
			t.Fatalf("bounded reclaim attempt %d = %#v err=%v", expectedAttempts, claims, claimErr)
		}
		currentClaim = claims[0]
	}

	// Cleanup remains available independently from Learning execution.
	corruptDraft := []byte("corrupt Draft quarantine")
	if err := objects.Put(ctx, draft.DraftObjectKey, bytes.NewReader(corruptDraft),
		int64(len(corruptDraft)), "application/zip"); err != nil {
		t.Fatal(err)
	}
	cleanup, err := service.Cleanup(ctx, ClaimRequest{Owner: "learning-cleaner", Now: time.Now().UTC().Add(8 * 24 * time.Hour), LeaseDuration: 30 * time.Second, Limit: 10})
	if err != nil || cleanup.Completed != 0 || cleanup.Failed != 1 {
		t.Fatalf("Cleanup drift = %#v err=%v", cleanup, err)
	}
	if _, info, err := objects.Get(ctx, draft.DraftObjectKey); err != nil || info.Size != int64(len(corruptDraft)) {
		t.Fatalf("drifted Draft object was deleted: info=%#v err=%v", info, err)
	}
	if err := objects.Put(ctx, draft.DraftObjectKey, bytes.NewReader(proposedValidated.CanonicalArchive),
		int64(len(proposedValidated.CanonicalArchive)), "application/zip"); err != nil {
		t.Fatal(err)
	}
	cleanup, err = service.Cleanup(ctx, ClaimRequest{Owner: "learning-cleaner", Now: time.Now().UTC().Add(8 * 24 * time.Hour), LeaseDuration: 30 * time.Second, Limit: 10})
	if err != nil || cleanup.Completed != 1 || cleanup.Failed != 0 {
		t.Fatalf("Cleanup repaired object = %#v err=%v", cleanup, err)
	}
	if _, _, err := objects.Get(ctx, draft.DraftObjectKey); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("Draft object still exists: %v", err)
	}
	promotionReplay, err = service.Promote(ctx, userID, review)
	if err != nil || promotionReplay.Created || promotionReplay.AdmissionID != promotion.AdmissionID {
		t.Fatalf("Promote replay after cleanup = %#v err=%v", promotionReplay, err)
	}
	mismatchedReplay := review
	mismatchedReplay.ExpectedRevision++
	if _, err := service.Promote(ctx, userID, mismatchedReplay); !errors.Is(err, ErrPromotionDenied) {
		t.Fatalf("mismatched Promote replay error=%v", err)
	}

	if os.Getenv("MM_CHAT_AGENT_LEARNING_RETAIN_FIXTURE") == "1" {
		return
	}
	// Cleanup the uncompleted stale-claim Draft and user-owned source facts.
	mustLearningExec(t, ctx, database, `DELETE FROM agent_learning_drafts WHERE id=$1`, second.ID)
	mustLearningExec(t, ctx, database, `DELETE FROM agent_learning_drafts WHERE id=$1`, draft.ID)
	mustLearningExec(t, ctx, database, `DELETE FROM skill_installations WHERE user_id=$1`, userID)
	mustLearningExec(t, ctx, database, `DELETE FROM skill_package_candidates WHERE reviewed_by_user_id=$1`, userID)
	mustLearningExec(t, ctx, database, `DELETE FROM skill_package_versions WHERE package_fingerprint IN ($1,$2)`,
		base.PackageFingerprint, promotion.PackageFingerprint)
	mustLearningExec(t, ctx, database, `DELETE FROM users WHERE id=$1`, userID)
}

type passChecker struct{ suite string }

func (checker passChecker) Check(context.Context, CheckInput) (CheckResult, error) {
	return CheckResult{Status: CheckPassed, ReasonCode: "CHECK_PASSED",
		SuiteFingerprint: "sha256:" + repeatHex(checker.suite), Metrics: map[string]int64{"cases": 1}}, nil
}

func passingReceipts(claim CheckClaim, value string) []CheckReceipt {
	result := make([]CheckReceipt, 0, 3)
	for _, kind := range []string{CheckStatic, CheckIsolation, CheckEvaluation} {
		result = append(result, CheckReceipt{ID: "draft_check_" + repeatN(value, 32), Kind: kind,
			Status: CheckPassed, ReasonCode: "CHECK_PASSED", SuiteFingerprint: "sha256:" + repeatHex(value),
			EvidenceFingerprint: "sha256:" + repeatHex(value), Metrics: map[string]int64{}})
		value = nextHex(value)
	}
	return result
}

func seedLearningBase(t *testing.T, ctx context.Context, database *sql.DB,
	objects storage.ObjectStore, userID string, archive []byte,
) skillsupply.PackageVersion {
	t.Helper()
	validated := mustValidateLearning(t, archive, "seed-base")
	packageKey := "skill-packages/sha256/" + validated.Package.PackageFingerprint[7:] + ".zip"
	sbomKey := "skill-sboms/sha256/" + validated.Package.SBOMFingerprint[7:] + ".cdx.json"
	sourceKey := "skill-quarantine/sha256/" + validated.SourceArtifactSHA256[7:] + ".zip"
	for _, object := range []struct {
		key, contentType string
		data             []byte
	}{
		{sourceKey, "application/zip", archive},
		{packageKey, "application/zip", validated.CanonicalArchive},
		{sbomKey, "application/vnd.cyclonedx+json", validated.SBOM},
	} {
		if err := objects.Put(ctx, object.key, bytes.NewReader(object.data), int64(len(object.data)), object.contentType); err != nil {
			t.Fatal(err)
		}
	}
	validated.Package.PackageObjectKey, validated.Package.SBOMObjectKey = packageKey, sbomKey
	candidate := skillsupply.Candidate{ID: uuid.NewString(), SourceType: skillsupply.SourceOfficial,
		SourceRef: "learning-base:" + uuid.NewString(), SourceArtifactSHA256: validated.SourceArtifactSHA256,
		SourceObjectKey: sourceKey, Package: validated.Package, Status: skillsupply.StatusValidated,
		AdmissionEligible: true, ValidationSummary: "validated_no_execute", Revision: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	supplyRepository := skillsupply.NewPostgresRepository(database)
	created, err := supplyRepository.CreateCandidate(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := supplyRepository.ReviewCandidate(ctx, created.ID, userID, skillsupply.ReviewInput{
		Status: skillsupply.StatusAdmitted, ExpectedRevision: 1,
		PackageFingerprint: created.Package.PackageFingerprint, Reason: "LEARNING_BASE_ADMITTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := supplyRepository.Install(ctx, userID, admitted.ID, admitted.Package.PackageFingerprint); err != nil {
		t.Fatal(err)
	}
	return admitted.Package
}

func seedLearningRun(t *testing.T, ctx context.Context, database *sql.DB,
	orchestrator *agentorchestrator.Service, userID, packageFingerprint string,
) (string, string) {
	t.Helper()
	result, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{UserID: userID,
		IdempotencyKey: "learning-source-" + uuid.NewString(),
		Snapshot:       json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","depth":0,"packageFingerprint":"` + packageFingerprint + `"}`),
		Steps:          []agentorchestrator.StepPlan{{Kind: "learn"}}})
	if err != nil {
		t.Fatal(err)
	}
	mustLearningExec(t, ctx, database, `UPDATE agent_runs SET state='succeeded',terminal_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, result.Run.ID)
	var eventID string
	mustLearningQuery(t, ctx, database, `SELECT id FROM agent_run_events WHERE run_id=$1 ORDER BY sequence LIMIT 1`, result.Run.ID).Scan(&eventID)
	return result.Run.ID, eventID
}

func repeatHex(value string) string { return repeatN(value, 64) }
func repeatN(value string, count int) string {
	result := ""
	for len(result) < count {
		result += value
	}
	return result[:count]
}
func nextHex(value string) string {
	if value == "f" {
		return "0"
	}
	return string(value[0] + 1)
}

func mustLearningQuery(t *testing.T, ctx context.Context, database *sql.DB,
	query string, arguments ...any,
) *sql.Row {
	t.Helper()
	return database.QueryRowContext(ctx, query, arguments...)
}

func mustLearningExec(t *testing.T, ctx context.Context, database *sql.DB,
	query string, arguments ...any,
) {
	t.Helper()
	if _, err := database.ExecContext(ctx, query, arguments...); err != nil {
		t.Fatal(err)
	}
}
