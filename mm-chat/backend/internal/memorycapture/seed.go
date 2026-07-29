package memorycapture

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	ephemeralDatabasePrefix = "mm_chat_memory_regression_"
	ephemeralGuardSchema    = "neo-chat.memory-regression-runtime-guard.v1"
)

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type SeedResult struct {
	RunID           string
	DatabaseName    string
	Cases           []RuntimeCase
	ProjectionCount int
}

type projectKey struct {
	userAlias    string
	projectAlias string
}

type conversationKey struct {
	userAlias         string
	conversationAlias string
}

type conversationSeed struct {
	key          conversationKey
	projectAlias string
}

// SeedEphemeralDatabase materializes only synthetic fixture state after
// proving the database is a fresh, fully migrated benchmark database.
func SeedEphemeralDatabase(
	ctx context.Context,
	db *sql.DB,
	pool memoryauthor.RegressionPool,
	index FixtureIndex,
	runID string,
) (SeedResult, error) {
	if db == nil || !runIDPattern.MatchString(runID) {
		return SeedResult{}, ErrCaptureInvalid
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SeedResult{}, errors.New("begin Memory regression seed")
	}
	defer func() { _ = tx.Rollback() }()
	databaseName, err := validateFreshEphemeralDatabase(ctx, tx)
	if err != nil {
		return SeedResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE memory_regression_runtime_guard (
  run_id TEXT PRIMARY KEY,
  schema_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT memory_regression_runtime_guard_run_id CHECK (
    run_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'
  ),
  CONSTRAINT memory_regression_runtime_guard_schema CHECK (
    schema_version = 'neo-chat.memory-regression-runtime-guard.v1'
  )
);
INSERT INTO memory_regression_runtime_guard(run_id, schema_version)
VALUES ($1, $2);
REVOKE ALL ON memory_regression_runtime_guard FROM PUBLIC;
GRANT SELECT ON memory_regression_runtime_guard TO go_api_runtime
`, runID, ephemeralGuardSchema); err != nil {
		return SeedResult{}, errors.New("create Memory regression runtime guard")
	}

	users := sortedKeys(index.UserToUUID)
	for _, alias := range users {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO users(id, display_name, created_at, updated_at)
VALUES ($1::uuid, $2, $3, $3)
`, index.UserToUUID[alias], "Memory regression fixture", regressionSeedTime()); err != nil {
			return SeedResult{}, errors.New("seed Memory regression user")
		}
	}
	for _, alias := range users {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO user_memory_state(user_id) VALUES ($1::uuid);
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled,
  sensitive_memory_enabled, l2_mode, l3_mode
) VALUES ($1::uuid, true, true, false, false, 'off', 'off');
`, index.UserToUUID[alias]); err != nil {
			return SeedResult{}, errors.New("seed Memory regression authority state")
		}
	}

	projects, conversations, err := collectScopeSeeds(pool)
	if err != nil {
		return SeedResult{}, err
	}
	for _, key := range sortedProjectKeys(projects) {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO projects(id, user_id, name, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $4)
`, projectUUID(key), index.UserToUUID[key.userAlias], "Memory regression Project", regressionSeedTime()); err != nil {
			return SeedResult{}, errors.New("seed Memory regression Project")
		}
	}
	for _, seed := range sortedConversationSeeds(conversations) {
		var projectID any
		if seed.projectAlias != "" {
			projectID = projectUUID(projectKey{userAlias: seed.key.userAlias, projectAlias: seed.projectAlias})
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO conversations(
  id, user_id, project_id, title, status, created_at, updated_at
) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Memory regression', 'active', $4, $4)
`, conversationUUID(seed.key), index.UserToUUID[seed.key.userAlias], projectID, regressionSeedTime()); err != nil {
			return SeedResult{}, errors.New("seed Memory regression Conversation")
		}
	}

	runtimeCases := make([]RuntimeCase, len(pool.Corpus.Cases))
	for position, item := range pool.Corpus.Cases {
		conversationAlias := caseConversationAlias(item)
		conversationID := conversationUUID(conversationKey{
			userAlias: item.Scope.UserAlias, conversationAlias: conversationAlias,
		})
		userMessageID := deterministicUUID("case-user-message", item.ID)
		assistantMessageID := deterministicUUID("case-assistant-message", item.ID)
		userID := index.UserToUUID[item.Scope.UserAlias]
		if _, err := tx.ExecContext(ctx, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content,
  completed_at, created_at, updated_at
) VALUES ($1::uuid, $2::uuid, $3::uuid, 1, 'user', 'completed', $4, $5, $5, $5);
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no, role,
  status, content, created_at, updated_at
) VALUES ($6::uuid, $2::uuid, $3::uuid, $1::uuid, 2, 'assistant',
  'streaming', '', $5, $5);
`, userMessageID, conversationID, userID, item.Query, regressionSeedTime(), assistantMessageID); err != nil {
			return SeedResult{}, errors.New("seed Memory regression messages")
		}
		runtimeCases[position] = RuntimeCase{
			CaseID: item.ID, Query: item.Query, UserID: userID,
			ConversationID: conversationID, AssistantMessageID: assistantMessageID,
		}
	}

	if err := seedFixtureMemories(ctx, tx, pool, index); err != nil {
		return SeedResult{}, err
	}
	var projectionCount int
	if err := tx.QueryRowContext(ctx, `
SELECT count(*) FROM user_memory_search_projections
`).Scan(&projectionCount); err != nil || projectionCount < 1 {
		return SeedResult{}, errors.New("verify Memory regression projection seed")
	}
	if err := tx.Commit(); err != nil {
		return SeedResult{}, errors.New("commit Memory regression seed")
	}
	return SeedResult{
		RunID: runID, DatabaseName: databaseName, Cases: runtimeCases,
		ProjectionCount: projectionCount,
	}, nil
}

func validateFreshEphemeralDatabase(ctx context.Context, tx *sql.Tx) (string, error) {
	var databaseName, currentUser string
	var migrationVersion sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
SELECT current_database(), current_user, max(version) FROM schema_migrations
`).Scan(&databaseName, &currentUser, &migrationVersion); err != nil {
		return "", fmt.Errorf("%w: inspect ephemeral database", ErrCaptureInvalid)
	}
	if !strings.HasPrefix(databaseName, ephemeralDatabasePrefix) ||
		currentUser == "go_api_runtime" || !migrationVersion.Valid || migrationVersion.Int64 < 63 {
		return "", fmt.Errorf("%w: database is not an eligible ephemeral migration target", ErrCaptureInvalid)
	}
	var users, projects, conversations, messages, memories, projections, providers, guards int
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM projects),
  (SELECT count(*) FROM conversations),
  (SELECT count(*) FROM user_memories),
  (SELECT count(*) FROM messages),
  (SELECT count(*) FROM user_memory_search_projections),
  (SELECT count(*) FROM provider_configs),
  CASE WHEN to_regclass('memory_regression_runtime_guard') IS NULL THEN 0
       ELSE 1 END
`).Scan(
		&users,
		&projects,
		&conversations,
		&memories,
		&messages,
		&projections,
		&providers,
		&guards,
	); err != nil {
		return "", fmt.Errorf("%w: inspect ephemeral database contents", ErrCaptureInvalid)
	}
	if users != 0 || projects != 0 || conversations != 0 || messages != 0 ||
		memories != 0 || projections != 0 || providers != 0 || guards != 0 {
		return "", fmt.Errorf("%w: ephemeral database is not empty", ErrCaptureInvalid)
	}
	return databaseName, nil
}

func VerifyRuntimeDatabase(ctx context.Context, db *sql.DB, runID string) error {
	if db == nil || !runIDPattern.MatchString(runID) {
		return ErrCaptureInvalid
	}
	var databaseName, currentUser, schemaVersion string
	if err := db.QueryRowContext(ctx, `
SELECT current_database(), current_user, schema_version
FROM memory_regression_runtime_guard WHERE run_id = $1
`, runID).Scan(&databaseName, &currentUser, &schemaVersion); err != nil {
		return fmt.Errorf("%w: runtime database guard", ErrCaptureInvalid)
	}
	if !strings.HasPrefix(databaseName, ephemeralDatabasePrefix) ||
		currentUser != "go_api_runtime" || schemaVersion != ephemeralGuardSchema {
		return fmt.Errorf("%w: runtime database authority", ErrCaptureInvalid)
	}
	return nil
}

func collectScopeSeeds(pool memoryauthor.RegressionPool) (
	map[projectKey]struct{},
	map[conversationKey]conversationSeed,
	error,
) {
	projects := make(map[projectKey]struct{})
	conversations := make(map[conversationKey]conversationSeed)
	addScope := func(userAlias string, scope memoryevalScope, fallbackConversation string) error {
		if scope.projectAlias != "" {
			projects[projectKey{userAlias: userAlias, projectAlias: scope.projectAlias}] = struct{}{}
		}
		conversationAlias := scope.conversationAlias
		if conversationAlias == "" {
			conversationAlias = fallbackConversation
		}
		if conversationAlias == "" {
			return nil
		}
		key := conversationKey{userAlias: userAlias, conversationAlias: conversationAlias}
		if existing, ok := conversations[key]; ok && existing.projectAlias != scope.projectAlias {
			return fmt.Errorf("%w: Conversation Project binding conflicts", ErrCaptureInvalid)
		}
		conversations[key] = conversationSeed{key: key, projectAlias: scope.projectAlias}
		return nil
	}
	for _, item := range pool.Corpus.Cases {
		if err := addScope(item.Scope.UserAlias, scopeValue(item.Scope), caseConversationAlias(item)); err != nil {
			return nil, nil, err
		}
	}
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			if memory.Scope.ConversationAlias == "" {
				if memory.Scope.ProjectAlias != "" {
					projects[projectKey{userAlias: memory.UserAlias, projectAlias: memory.Scope.ProjectAlias}] = struct{}{}
				}
				continue
			}
			if err := addScope(memory.UserAlias, scopeValue(memory.Scope), ""); err != nil {
				return nil, nil, err
			}
		}
	}
	return projects, conversations, nil
}

type memoryevalScope struct {
	projectAlias      string
	conversationAlias string
}

func scopeValue(scope memoryeval.Scope) memoryevalScope {
	return memoryevalScope{
		projectAlias: scope.ProjectAlias, conversationAlias: scope.ConversationAlias,
	}
}

func seedFixtureMemories(
	ctx context.Context,
	tx *sql.Tx,
	pool memoryauthor.RegressionPool,
	index FixtureIndex,
) error {
	type pendingMemory struct {
		fixture memoryauthor.FixtureMemory
		caseDef memoryeval.GoldenCase
	}
	ordinary := make([]pendingMemory, 0, len(index.MemoryToUUID))
	superseded := make([]pendingMemory, 0, 50)
	for _, fixture := range pool.Fixtures.Fixtures {
		caseDef, ok := index.CasesByFixture[fixture.Alias]
		if !ok {
			return fmt.Errorf("%w: fixture case binding missing", ErrCaptureInvalid)
		}
		for _, memory := range fixture.Memories {
			item := pendingMemory{fixture: memory, caseDef: caseDef}
			switch memory.State {
			case memoryauthor.StateSecretRejected, memoryauthor.StateUntrustedRejected:
				continue
			case memoryauthor.StateSuperseded:
				superseded = append(superseded, item)
			case memoryauthor.StateActive, memoryauthor.StateDeleted,
				memoryauthor.StateIrrelevant, memoryauthor.StateOutOfScope:
				ordinary = append(ordinary, item)
			default:
				return fmt.Errorf("%w: unsupported fixture Memory state", ErrCaptureInvalid)
			}
		}
	}
	insert := func(item pendingMemory, successorID string) error {
		memory := item.fixture
		memoryType := fixtureMemoryType(item.caseDef)
		normalized, normalizedContent, err := usermemory.NormalizeCandidateForStorage(
			usermemory.Candidate{
				Type: memoryType, Content: memory.CanonicalContent,
				Importance: 3, Tags: []string{},
			},
		)
		if err != nil {
			return fmt.Errorf("%w: normalize fixture Memory", ErrCaptureInvalid)
		}
		occurredAt, err := time.Parse(time.RFC3339, memory.OccurredAt)
		if err != nil {
			return fmt.Errorf("%w: fixture Memory timestamp", ErrCaptureInvalid)
		}
		scopeType := "global"
		var projectID, conversationID any
		switch {
		case memory.Scope.ConversationAlias != "":
			scopeType = "conversation"
			conversationID = conversationUUID(conversationKey{
				userAlias: memory.UserAlias, conversationAlias: memory.Scope.ConversationAlias,
			})
		case memory.Scope.ProjectAlias != "":
			scopeType = "project"
			projectID = projectUUID(projectKey{
				userAlias: memory.UserAlias, projectAlias: memory.Scope.ProjectAlias,
			})
		}
		enabled := true
		lifecycle := "active"
		var deletedAt any
		var successor any
		switch memory.State {
		case memoryauthor.StateDeleted:
			enabled = false
			deletedAt = occurredAt.UTC()
		case memoryauthor.StateSuperseded:
			enabled = false
			lifecycle = "superseded"
			if successorID == "" {
				return fmt.Errorf("%w: superseded fixture has no current successor", ErrCaptureInvalid)
			}
			successor = successorID
		}
		digest := sha256.Sum256([]byte(normalized.Content))
		if _, err := tx.ExecContext(ctx, `
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, importance, tags,
  source, enabled, created_at, updated_at, deleted_at, scope_type, project_id,
  scope_conversation_id, scope_generation, revision, visibility_epoch,
  content_hash, authority_kind, lifecycle_status, observed_at,
  superseded_by_memory_id, sensitivity, temporal_basis
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5, $6::smallint, $7::text[],
  'manual', $8, $9, $9, $10, $11, $12::uuid,
  $13::uuid, 1, 1, 1, $14, 'manual', $15, $9,
  $16::uuid, 'normal', 'source_timestamp'
)
`,
			index.MemoryToUUID[memory.ID], index.UserToUUID[memory.UserAlias],
			normalized.Type, normalized.Content, normalizedContent,
			normalized.Importance, normalized.Tags, enabled, occurredAt.UTC(),
			deletedAt, scopeType, projectID, conversationID,
			hex.EncodeToString(digest[:]), lifecycle, successor,
		); err != nil {
			return fmt.Errorf("seed Memory regression canonical Memory: %w", err)
		}
		return nil
	}
	for _, item := range ordinary {
		if err := insert(item, ""); err != nil {
			return err
		}
	}
	for _, item := range superseded {
		successor := ""
		for _, opaqueID := range item.caseDef.ExpectedCurrentMemoryIDs {
			if opaqueID != item.fixture.ID {
				if candidate, ok := index.MemoryToUUID[opaqueID]; ok {
					successor = candidate
					break
				}
			}
		}
		if err := insert(item, successor); err != nil {
			return err
		}
	}
	return nil
}

func fixtureMemoryType(item memoryeval.GoldenCase) string {
	for _, slice := range item.Slices {
		switch slice {
		case "preference_instruction":
			return "preference"
		case "project_decision":
			return "decision"
		}
	}
	return "fact"
}

func caseConversationAlias(item memoryeval.GoldenCase) string {
	if item.Scope.ConversationAlias != "" {
		return item.Scope.ConversationAlias
	}
	return "case-" + item.ID
}

func projectUUID(key projectKey) string {
	return deterministicUUID("project", key.userAlias+"\x00"+key.projectAlias)
}

func conversationUUID(key conversationKey) string {
	return deterministicUUID("conversation", key.userAlias+"\x00"+key.conversationAlias)
}

func regressionSeedTime() time.Time {
	return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedProjectKeys(values map[projectKey]struct{}) []projectKey {
	keys := make([]projectKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].userAlias != keys[j].userAlias {
			return keys[i].userAlias < keys[j].userAlias
		}
		return keys[i].projectAlias < keys[j].projectAlias
	})
	return keys
}

func sortedConversationSeeds(values map[conversationKey]conversationSeed) []conversationSeed {
	seeds := make([]conversationSeed, 0, len(values))
	for _, seed := range values {
		seeds = append(seeds, seed)
	}
	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].key.userAlias != seeds[j].key.userAlias {
			return seeds[i].key.userAlias < seeds[j].key.userAlias
		}
		return seeds[i].key.conversationAlias < seeds[j].key.conversationAlias
	})
	return seeds
}
