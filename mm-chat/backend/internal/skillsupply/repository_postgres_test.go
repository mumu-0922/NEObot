package skillsupply

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestSkillPostgresRepositoryAuthorityDriftOwnershipAndCAS(t *testing.T) {
	database := openSkillPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminID, userOne, userTwo := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, userID := range []string{adminID, userOne, userTwo} {
		mustExecSkill(t, ctx, database, `
INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Skill integration')
`, userID, "skill-"+userID+"@example.test")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM users WHERE id IN ($1, $2, $3)`, adminID, userOne, userTwo)
	})

	repository := NewPostgresRepository(database)
	validated, err := ValidateArchive(ArchiveSource{Type: SourceZIP, Ref: "zip:integration",
		Data: mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(validSkillMarkdown("integration-skill"))}}, 0, time.Time{})})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	validated.Package.PackageObjectKey = digestObjectKey("skill-packages", validated.Package.PackageFingerprint, ".zip")
	validated.Package.SBOMObjectKey = digestObjectKey("skill-sboms", validated.Package.SBOMFingerprint, ".cdx.json")
	candidate := Candidate{ID: uuid.NewString(), SourceType: SourceZIP, SourceRef: "zip:integration",
		SourceArtifactSHA256: validated.SourceArtifactSHA256,
		SourceObjectKey:      digestObjectKey("skill-quarantine", validated.SourceArtifactSHA256, ".zip"),
		Package:              validated.Package, Status: StatusValidated, AdmissionEligible: true,
		ValidationSummary: "validated_no_execute", Revision: 1, CreatedAt: now, UpdatedAt: now}
	created, err := repository.CreateCandidate(ctx, candidate)
	if err != nil || created.ID != candidate.ID || created.Package.PackageFingerprint != candidate.Package.PackageFingerprint {
		t.Fatalf("create candidate=%#v error=%v", created, err)
	}
	replayed, err := repository.CreateCandidate(ctx, candidate)
	if err != nil || replayed.ID != candidate.ID {
		t.Fatalf("idempotent candidate=%#v error=%v", replayed, err)
	}
	drifted := candidate
	drifted.ID = uuid.NewString()
	drifted.SourceArtifactSHA256 = "sha256:" + strings.Repeat("f", 64)
	drifted.SourceObjectKey = digestObjectKey("skill-quarantine", drifted.SourceArtifactSHA256, ".zip")
	if _, err := repository.CreateCandidate(ctx, drifted); !errors.Is(err, ErrSourceDrift) {
		t.Fatalf("source drift error=%v", err)
	}
	if _, err := repository.ReviewCandidate(ctx, candidate.ID, adminID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 2, PackageFingerprint: candidate.Package.PackageFingerprint,
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale review error=%v", err)
	}
	admitted, err := repository.ReviewCandidate(ctx, candidate.ID, adminID, ReviewInput{
		Status: StatusAdmitted, ExpectedRevision: 1, PackageFingerprint: candidate.Package.PackageFingerprint,
		Reason: "integration review",
	})
	if err != nil || admitted.Revision != 2 || admitted.Status != StatusAdmitted {
		t.Fatalf("admitted=%#v error=%v", admitted, err)
	}
	installed, err := repository.Install(ctx, userOne, candidate.ID, candidate.Package.PackageFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Install(ctx, userOne, candidate.ID, candidate.Package.PackageFingerprint); !errors.Is(err, ErrInstallationConflict) {
		t.Fatalf("duplicate install error=%v", err)
	}
	if _, err := repository.Install(ctx, userTwo, candidate.ID, candidate.Package.PackageFingerprint); err != nil {
		t.Fatalf("other owner install error=%v", err)
	}
	if items, err := repository.ListLibrary(ctx, userOne); err != nil || len(items) != 1 || items[0].ID != installed.ID {
		t.Fatalf("owner library=%#v error=%v", items, err)
	}
	if items, err := repository.ListLibrary(ctx, adminID); err != nil || len(items) != 0 {
		t.Fatalf("cross-user library=%#v error=%v", items, err)
	}
	if err := repository.Uninstall(ctx, userTwo, installed.ID, 1); !errors.Is(err, ErrInstallationNotFound) {
		t.Fatalf("cross-user uninstall error=%v", err)
	}
	if err := repository.Uninstall(ctx, userOne, installed.ID, 2); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale uninstall error=%v", err)
	}
	if err := repository.Uninstall(ctx, userOne, installed.ID, 1); err != nil {
		t.Fatal(err)
	}
}

func openSkillPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Skill Postgres integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	database := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("ping Skill integration database: %v", err)
	}
	return database
}

func mustExecSkill(t *testing.T, ctx context.Context, database *sql.DB, query string, arguments ...any) {
	t.Helper()
	if _, err := database.ExecContext(ctx, query, arguments...); err != nil {
		t.Fatalf("execute Skill fixture: %v", err)
	}
}
