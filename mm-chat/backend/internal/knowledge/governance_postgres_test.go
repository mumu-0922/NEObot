package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresGovernanceApplyDisableIsAtomicAndIdempotent(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	manifest := GovernanceManifest{Processor: "mineru", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"parse"}, AllowedDataTypes: []string{"application/pdf"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	service := NewGovernanceService(NewPostgresRepository(db))
	head, err := service.Apply(ctx, manifest)
	if err != nil || head.HeadRevision != 1 || head.ActiveGovernanceRevision != 1 || head.Status != "active" {
		t.Fatalf("first apply = %#v, err=%v", head, err)
	}
	replayed, err := service.Apply(ctx, manifest)
	if err != nil || replayed.ActiveProfileID != head.ActiveProfileID || replayed.HeadRevision != 1 {
		t.Fatalf("replay = %#v, err=%v", replayed, err)
	}
	manifest.ModelAPIVersion = "v2"
	head, err = service.Apply(ctx, manifest)
	if err != nil || head.HeadRevision != 2 || head.ActiveGovernanceRevision != 2 {
		t.Fatalf("changed apply = %#v, err=%v", head, err)
	}
	head, err = service.Disable(ctx, "mineru", "default")
	if err != nil || head.Status != "disabled" || head.HeadRevision != 3 || head.ActiveProfileID != "" {
		t.Fatalf("disable = %#v, err=%v", head, err)
	}
	replayed, err = service.Disable(ctx, "mineru", "default")
	if err != nil || replayed.HeadRevision != 3 {
		t.Fatalf("disable replay = %#v, err=%v", replayed, err)
	}
	var profiles, events int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='mineru' AND endpoint_id='default'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_type='processor_governance_head' AND aggregate_key='mineru/default'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if profiles != 2 || events != 3 {
		t.Fatalf("profiles/events = %d/%d", profiles, events)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processor_governance_profiles SET model_api_version='mutated' WHERE processor='mineru'`); err == nil {
		t.Fatal("governance profile update unexpectedly succeeded")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM processor_governance_profiles WHERE processor='mineru'`); err == nil {
		t.Fatal("governance profile delete unexpectedly succeeded")
	}
	var leaked int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_outbox WHERE aggregate_key='mineru/default' AND (payload ? 'retentionPolicy' OR payload ? 'deletionContract' OR payload ? 'trainingUse')`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("policy fields leaked into outbox: %d", leaked)
	}

	var existingEventID string
	if err := db.QueryRowContext(ctx, `SELECT event_id FROM knowledge_outbox WHERE aggregate_key='mineru/default' LIMIT 1`).Scan(&existingEventID); err != nil {
		t.Fatal(err)
	}
	failing := NewPostgresRepository(db)
	generated := 0
	failing.newEventID = func() (string, error) {
		generated++
		if generated == 2 {
			return existingEventID, nil
		}
		return fmt.Sprintf("91000000-0000-4000-8000-%012d", generated), nil
	}
	manifest.ModelAPIVersion = "v3"
	if _, err := NewGovernanceService(failing).Apply(ctx, manifest); err == nil {
		t.Fatal("failed outbox apply error = nil")
	}
	var status string
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT status,head_revision FROM processor_governance_heads WHERE processor='mineru' AND endpoint_id='default'`).Scan(&status, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" || revision != 3 {
		t.Fatalf("failed apply committed head = %s/%d", status, revision)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='mineru' AND endpoint_id='default'`).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if profiles != 2 {
		t.Fatalf("failed apply committed profile count = %d", profiles)
	}
}

func TestPostgresGovernanceConcurrentFirstApplySerializes(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatal(err)
	}
	base := GovernanceManifest{Processor: "jina", EndpointID: "default", ModelAPIVersion: "v1",
		AllowedPurposes: []string{"passage_embedding"}, AllowedDataTypes: []string{"text/plain"}, Region: "global",
		RetentionPolicy: "none", DeletionContract: "delete", TrainingUse: "disabled"}
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for _, version := range []string{"v1", "v2"} {
		wait.Add(1)
		go func(version string) {
			defer wait.Done()
			manifest := base
			manifest.ModelAPIVersion = version
			_, err := NewGovernanceService(NewPostgresRepository(db)).Apply(ctx, manifest)
			errorsOut <- err
		}(version)
	}
	wait.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatal(err)
		}
	}
	var profileCount int
	var headRevision int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processor_governance_profiles WHERE processor='jina'`).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT head_revision FROM processor_governance_heads WHERE processor='jina'`).Scan(&headRevision); err != nil {
		t.Fatal(err)
	}
	if profileCount != 2 || headRevision != 2 {
		t.Fatalf("concurrent profiles/head revision = %d/%d", profileCount, headRevision)
	}
	var hashes int
	if err := db.QueryRowContext(ctx, `SELECT count(DISTINCT manifest_hash) FROM processor_governance_profiles WHERE processor='jina' AND manifest_hash<>$1`, strings.Repeat("0", 64)).Scan(&hashes); err != nil {
		t.Fatal(err)
	}
	if hashes != 2 {
		t.Fatalf("distinct manifests = %d", hashes)
	}
}
