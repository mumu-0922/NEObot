package memorycapture

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/usermemory"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestNativeMemoryRegressionLivePostgres(t *testing.T) {
	adminDB, adminConfig, databaseName := openMemoryRegressionTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	runner := migration.NewRunner(adminDB, migrationfiles.FS)
	if _, err := runner.WithPhase15GovernanceMapping(migration.Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("apply Memory regression migrations: %v", err)
	}
	assertMemoryRegressionPostgresProfile(t, ctx, adminDB)

	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildFixtureIndex(pool)
	if err != nil {
		t.Fatal(err)
	}
	runID := "postgres-protocol-" + databaseName[len(ephemeralDatabasePrefix):]
	seed, err := SeedEphemeralDatabase(ctx, adminDB, pool, index, runID)
	if err != nil {
		t.Fatalf("seed ephemeral Memory regression database: %v", err)
	}
	if seed.DatabaseName != databaseName || len(seed.Cases) != 500 || seed.ProjectionCount < 1 {
		t.Fatalf("seed result = %#v", seed)
	}

	fake := NewFakeProtocolProvider()
	vectorCount, err := PopulateProjectionVectors(ctx, adminDB, runID, fake)
	if err != nil || vectorCount < 1 {
		t.Fatalf("populate fake fixture vectors = %d/%v", vectorCount, err)
	}
	assertSeededFixtureState(t, ctx, adminDB, pool, index, vectorCount)

	runtimeConfig := adminConfig.Copy()
	runtimeConfig.Database = databaseName
	runtimeConfig.RuntimeParams["role"] = "go_api_runtime"
	runtimeDB := stdlib.OpenDB(*runtimeConfig)
	defer runtimeDB.Close()
	if err := runtimeDB.PingContext(ctx); err != nil {
		t.Fatalf("open runtime role connection: %v", err)
	}
	assertHybridAdmissionFailClosed(t, ctx, adminDB, runtimeDB, seed, fake)

	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture"),
		CorpusRawSHA256:   sha256String("corpus"),
		AuditRawSHA256:    sha256String("audit"),
		ManifestRawSHA256: sha256String("manifest"),
	}
	cost := protocolCostBasis()
	costHash, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	baselineConfig, candidateConfig, err := BuildProfileConfigs(
		protected,
		costHash,
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := HashProfileConfigs(baselineConfig, candidateConfig)
	if err != nil {
		t.Fatal(err)
	}
	baseline, candidate, err := CaptureProfiles(
		ctx,
		adminDB,
		runtimeDB,
		runID,
		index,
		seed,
		fake,
		hashes,
		cost,
	)
	if err != nil {
		t.Fatalf("capture production reader profiles: %v", err)
	}
	if len(baseline.Cases) != 500 || len(candidate.Cases) != 500 ||
		candidate.Profile.ID != FakeCandidateProfileID {
		t.Fatalf("captured profile cardinality = baseline:%d candidate:%d id:%q",
			len(baseline.Cases), len(candidate.Cases), candidate.Profile.ID)
	}
	assertNoForbiddenFixtureSurfaces(t, pool, baseline.Cases)
	assertNoForbiddenFixtureSurfaces(t, pool, candidate.Cases)
}

func assertHybridAdmissionFailClosed(
	t *testing.T,
	ctx context.Context,
	adminDB *sql.DB,
	runtimeDB *sql.DB,
	seed SeedResult,
	provider *FakeProtocolProvider,
) {
	t.Helper()
	if len(seed.Cases) == 0 {
		t.Fatal("seeded regression has no cases")
	}
	item := seed.Cases[0]
	embedding, err := provider.EmbedQuery(ctx, item.Query)
	if err != nil {
		t.Fatal(err)
	}
	repository := usermemory.NewPostgresRepository(runtimeDB)
	userCtx := auth.WithUser(ctx, auth.User{ID: item.UserID})
	observationID := "97000000-0000-4000-8000-000000000001"
	prepared, err := repository.PrepareHybridShadow(userCtx, usermemory.HybridShadowPrepareInput{
		ObservationID: observationID, ConversationID: item.ConversationID,
		AssistantMessageID: item.AssistantMessageID, QueryHash: sha256String(item.Query),
		QueryText: item.Query, Baseline: []usermemory.LexicalShadowBaseline{},
		QueryEmbedding: embedding.Vector, QueryEmbeddingState: "ready",
	})
	if err != nil || prepared.Summary.Status != "pending" || len(prepared.Candidates) == 0 {
		t.Fatalf("prepare admission fixture = %#v/%v", prepared, err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(`DELETE FROM message_memory_hybrid_shadow_results WHERE observation_id = $1`, observationID)
		_, _ = adminDB.Exec(`DELETE FROM message_memory_hybrid_shadow_observations WHERE id = $1`, observationID)
	})
	input := usermemory.HybridShadowAdmissionInput{
		ObservationID: observationID, AssistantMessageID: item.AssistantMessageID,
		QueryHash: sha256String(item.Query), QueryEmbedding: embedding.Vector,
	}
	var observationCountBefore, resultCountBefore int
	if err := adminDB.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM message_memory_hybrid_shadow_observations WHERE id = $1),
  (SELECT count(*) FROM message_memory_hybrid_shadow_results WHERE observation_id = $1)
`, observationID).Scan(&observationCountBefore, &resultCountBefore); err != nil {
		t.Fatal(err)
	}
	admission, err := repository.AuthorizeHybridRerank(userCtx, input)
	if err != nil || admission.CandidateCount != len(prepared.Candidates) ||
		admission.VectorCandidateCount != admission.CandidateCount {
		t.Fatalf("authorize prepared hybrid surface = %#v/%v", admission, err)
	}
	var observationCountAfter, resultCountAfter int
	if err := adminDB.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM message_memory_hybrid_shadow_observations WHERE id = $1),
  (SELECT count(*) FROM message_memory_hybrid_shadow_results WHERE observation_id = $1)
`, observationID).Scan(&observationCountAfter, &resultCountAfter); err != nil {
		t.Fatal(err)
	}
	if observationCountAfter != observationCountBefore || resultCountAfter != resultCountBefore {
		t.Fatalf("admission persisted state: observations %d->%d results %d->%d",
			observationCountBefore, observationCountAfter, resultCountBefore, resultCountAfter)
	}

	if _, err := adminDB.ExecContext(ctx, `
UPDATE messages SET status = 'pending'
WHERE id = (SELECT parent_message_id FROM messages WHERE id = $1)
`, item.AssistantMessageID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AuthorizeHybridRerank(userCtx, input); err == nil {
		t.Fatal("stale source message authorized hybrid rerank")
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE messages SET status = 'completed'
WHERE id = (SELECT parent_message_id FROM messages WHERE id = $1)
`, item.AssistantMessageID); err != nil {
		t.Fatal(err)
	}

	memoryID := prepared.Candidates[0].MemoryID
	var vectorText string
	var vectorUpdatedAt time.Time
	if err := adminDB.QueryRowContext(ctx, `
SELECT embedding_vector::text, embedding_updated_at
FROM user_memory_search_projections
WHERE memory_id = $1
`, memoryID).Scan(&vectorText, &vectorUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE user_memory_search_projections
SET embedding_status = 'pending', embedding_vector = NULL,
    embedding_error_code = NULL, embedding_updated_at = NULL
WHERE memory_id = $1
`, memoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AuthorizeHybridRerank(userCtx, input); err == nil {
		t.Fatal("stale vector authorized hybrid rerank")
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE user_memory_search_projections
SET embedding_status = 'ready', embedding_vector = $2::vector(1024),
    embedding_error_code = NULL, embedding_updated_at = $3
WHERE memory_id = $1
`, memoryID, vectorText, vectorUpdatedAt); err != nil {
		t.Fatal(err)
	}

	var projectionGeneration int64
	if err := adminDB.QueryRowContext(ctx, `
SELECT active_projection_generation FROM user_memory_state WHERE user_id = $1
`, item.UserID).Scan(&projectionGeneration); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE user_memory_state
SET active_projection_generation = active_projection_generation + 1
WHERE user_id = $1
`, item.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AuthorizeHybridRerank(userCtx, input); err == nil {
		t.Fatal("stale projection generation authorized hybrid rerank")
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE user_memory_state SET active_projection_generation = $2 WHERE user_id = $1
`, item.UserID, projectionGeneration); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AuthorizeHybridRerank(userCtx, input); err != nil {
		t.Fatalf("restored admission authority = %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, `
DELETE FROM message_memory_hybrid_shadow_results WHERE observation_id = $1;
DELETE FROM message_memory_hybrid_shadow_observations WHERE id = $1;
`, observationID); err != nil {
		t.Fatal(err)
	}
}

func openMemoryRegressionTestDatabase(t *testing.T) (*sql.DB, *pgx.ConnConfig, string) {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	adminConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	controlDB := stdlib.OpenDB(*adminConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := controlDB.PingContext(ctx); err != nil {
		_ = controlDB.Close()
		t.Fatalf("ping Memory regression control database: %v", err)
	}
	databaseName := fmt.Sprintf("%s%d", ephemeralDatabasePrefix, time.Now().UnixNano())
	quotedDatabase := pgx.Identifier{databaseName}.Sanitize()
	if _, err := controlDB.ExecContext(ctx, `CREATE DATABASE `+quotedDatabase); err != nil {
		_ = controlDB.Close()
		t.Fatalf("create disposable Memory regression database: %v", err)
	}
	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	testDB := stdlib.OpenDB(*testConfig)
	if err := testDB.PingContext(ctx); err != nil {
		_ = testDB.Close()
		_, _ = controlDB.ExecContext(ctx, `DROP DATABASE `+quotedDatabase+` WITH (FORCE)`)
		_ = controlDB.Close()
		t.Fatalf("ping disposable Memory regression database: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		if _, err := controlDB.ExecContext(
			dropCtx,
			`DROP DATABASE IF EXISTS `+quotedDatabase+` WITH (FORCE)`,
		); err != nil {
			t.Errorf("drop disposable Memory regression database: %v", err)
		}
		_ = controlDB.Close()
	})
	return testDB, adminConfig, databaseName
}

func assertMemoryRegressionPostgresProfile(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var serverVersion int
	var textSearchVersion, vectorVersion string
	if err := db.QueryRowContext(ctx, `
SELECT current_setting('server_version_num')::integer,
       (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch'),
       (SELECT extversion FROM pg_extension WHERE extname = 'vector')
`).Scan(&serverVersion, &textSearchVersion, &vectorVersion); err != nil {
		t.Fatalf("inspect Memory regression PostgreSQL profile: %v", err)
	}
	if serverVersion/10000 != 17 || textSearchVersion != "1.3.1" || vectorVersion != "0.8.5" {
		t.Fatalf("PostgreSQL profile = %d pg_textsearch=%q vector=%q",
			serverVersion, textSearchVersion, vectorVersion)
	}
}

func assertSeededFixtureState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	pool memoryauthor.RegressionPool,
	index FixtureIndex,
	vectorCount int,
) {
	t.Helper()
	expectedCanonical := 0
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			if memory.State != memoryauthor.StateSecretRejected &&
				memory.State != memoryauthor.StateUntrustedRejected {
				expectedCanonical++
			}
		}
	}
	var users, projects, conversations, memories, readyVectors int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM projects),
  (SELECT count(*) FROM conversations),
  (SELECT count(*) FROM user_memories),
  (SELECT count(*) FROM user_memory_search_projections WHERE embedding_status = 'ready')
`).Scan(&users, &projects, &conversations, &memories, &readyVectors); err != nil {
		t.Fatalf("inspect seeded fixture state: %v", err)
	}
	if users != len(index.UserToUUID) || conversations < 500 ||
		projects < 1 || memories != expectedCanonical || readyVectors != vectorCount {
		t.Fatalf("seeded fixture counts = users:%d projects:%d conversations:%d memories:%d vectors:%d; expected users:%d memories:%d vectors:%d",
			users, projects, conversations, memories, readyVectors,
			len(index.UserToUUID), expectedCanonical, vectorCount)
	}
}

func assertNoForbiddenFixtureSurfaces(
	t *testing.T,
	pool memoryauthor.RegressionPool,
	cases []memoryeval.CaseObservation,
) {
	t.Helper()
	states := make(map[string]memoryauthor.MemoryState)
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			states[memory.ID] = memory.State
		}
	}
	for _, observed := range cases {
		for _, surface := range [][]string{
			observed.CandidateMemoryIDs,
			observed.FinalMemoryIDs,
			observed.InjectedMemoryIDs,
			observed.ProviderSentMemoryIDs,
		} {
			for _, memoryID := range surface {
				switch states[memoryID] {
				case memoryauthor.StateDeleted, memoryauthor.StateSuperseded,
					memoryauthor.StateSecretRejected, memoryauthor.StateUntrustedRejected,
					memoryauthor.StateOutOfScope:
					t.Fatalf("forbidden fixture Memory %q (%s) reached case %q",
						memoryID, states[memoryID], observed.CaseID)
				}
			}
		}
	}
}

func protocolCostBasis() CostBasis {
	return CostBasis{
		SchemaVersion: "neo-chat.memory-regression-cost-basis.v1",
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 1,
			ChatProviderCostMicrounits: 100,
		},
		Source:      "deterministic protocol fixture",
		EffectiveAt: "2026-07-29T13:00:00Z",
	}
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
