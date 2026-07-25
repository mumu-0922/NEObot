package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

const evidenceSearchProfileID = "74000000-0000-4000-8000-000000000017"

type evidenceHydrationFixture struct {
	MemberUserID      string
	OutsiderUserID    string
	MemberSessionID   string
	OutsiderSessionID string
	MemberConvID      string
	OutsiderConvID    string
	CollectionID      string
	Reference         EvidenceCandidateReference
}

func TestPostgresReauthorizeAndHydrateEvidenceFencesReferences(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	fixture := seedEvidenceHydrationFixture(t, ctx, db)
	repo := NewPostgresRepository(db)

	validInput := ReauthorizeEvidenceInput{
		ActorUserID:           fixture.MemberUserID,
		SessionID:             fixture.MemberSessionID,
		ConversationID:        fixture.MemberConvID,
		SelectedCollectionIDs: []string{fixture.CollectionID},
		References:            []EvidenceCandidateReference{fixture.Reference},
	}
	missingSearch, err := repo.ReauthorizeAndHydrateEvidence(ctx, validInput)
	if !errors.Is(err, ErrEvidenceHydrationRejected) || len(missingSearch) != 0 {
		t.Fatalf("missing Child Search authority = %#v, %v", missingSearch, err)
	}
	seedEvidenceSearchProjection(t, ctx, db, fixture)
	hydrated, err := repo.ReauthorizeAndHydrateEvidence(ctx, validInput)
	if err != nil {
		t.Fatalf("valid hydration error = %v", err)
	}
	if len(hydrated) != 1 {
		t.Fatalf("hydrated count = %d, want 1", len(hydrated))
	}
	got := hydrated[0]
	if got.SourceName != "source.pdf" ||
		got.SourceText != "alpha evidence source" ||
		got.ChildTokenCount != 3 ||
		got.ParentSourceText != "alpha evidence parent" ||
		got.ParentTokenCount != 4 ||
		got.ContentHash != fixture.Reference.ContentHash ||
		got.SourceSpanHash != fixture.Reference.SourceSpanHash ||
		!strings.Contains(string(got.Locator), `"startLine": 2`) ||
		strings.Contains(string(got.Locator), `"endLine": 10`) ||
		got.RankScore != fixture.Reference.RankScore {
		t.Fatalf("hydrated evidence = %#v", got)
	}

	rejectCases := []struct {
		name  string
		input ReauthorizeEvidenceInput
	}{
		{
			name: "wrong team actor",
			input: ReauthorizeEvidenceInput{
				ActorUserID:           fixture.OutsiderUserID,
				SessionID:             fixture.OutsiderSessionID,
				ConversationID:        fixture.OutsiderConvID,
				SelectedCollectionIDs: []string{fixture.CollectionID},
				References:            []EvidenceCandidateReference{fixture.Reference},
			},
		},
		{
			name: "unselected collection",
			input: ReauthorizeEvidenceInput{
				ActorUserID:           fixture.MemberUserID,
				SessionID:             fixture.MemberSessionID,
				ConversationID:        fixture.MemberConvID,
				SelectedCollectionIDs: []string{"73000000-0000-4000-8000-000000000099"},
				References:            []EvidenceCandidateReference{fixture.Reference},
			},
		},
		{
			name: "stale materialization",
			input: withEvidenceReference(validInput, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
				reference.MaterializationID = "73000000-0000-4000-8000-000000000098"
				return reference
			}),
		},
		{
			name: "stale document version",
			input: withEvidenceReference(validInput, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
				reference.DocumentVersionID = "73000000-0000-4000-8000-000000000097"
				return reference
			}),
		},
		{
			name: "content hash mismatch",
			input: withEvidenceReference(validInput, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
				reference.ContentHash = strings.Repeat("9", 64)
				return reference
			}),
		},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			hydrated, err := repo.ReauthorizeAndHydrateEvidence(ctx, tc.input)
			if !errors.Is(err, ErrEvidenceHydrationRejected) {
				t.Fatalf("error = %v, want ErrEvidenceHydrationRejected", err)
			}
			if len(hydrated) != 0 {
				t.Fatalf("rejected hydration returned body = %#v", hydrated)
			}
		})
	}

	mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_child_search_projections
SET locator_summary = '{}'::jsonb
WHERE child_chunk_id = $1
`, fixture.Reference.ChildChunkID)
	malformedLocator, err := repo.ReauthorizeAndHydrateEvidence(ctx, validInput)
	if !errors.Is(err, ErrEvidenceHydrationRejected) || len(malformedLocator) != 0 {
		t.Fatalf("malformed Child locator authority = %#v, %v", malformedLocator, err)
	}
}

func TestPostgresFetchQueryEvidenceCandidatesReturnsBoundedReferences(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	fixture := seedEvidenceHydrationFixture(t, ctx, db)
	seedEvidenceSearchProjection(t, ctx, db, fixture)
	repo := NewPostgresRepository(db)

	mustKnowledgeExec(t, ctx, db, `
SELECT * FROM knowledge_backfill_pgvector_shadow($1::uuid, $2::uuid)
`, fixture.Reference.IndexGenerationID, evidenceSearchProfileID)
	mustKnowledgeExec(t, ctx, db, `
SELECT * FROM knowledge_backfill_bm25_shadow($1::uuid, $2::uuid)
`, fixture.Reference.IndexGenerationID, evidenceSearchProfileID)
	var activeProfile string
	var retrievalRevision int64
	if err := db.QueryRowContext(ctx, `
SELECT active_profile, revision
FROM knowledge_set_retrieval_profile(
  'legacy', 'pg17_bm25_pgvector_v1', 1, 'integration test activation'
)
`).Scan(&activeProfile, &retrievalRevision); err != nil {
		t.Fatalf("activate pg17 retrieval profile: %v", err)
	}
	if activeProfile != "pg17_bm25_pgvector_v1" || retrievalRevision != 2 {
		t.Fatalf("retrieval profile = %q@%d", activeProfile, retrievalRevision)
	}

	candidates, err := repo.FetchQueryEvidenceCandidates(ctx, QueryEvidenceCandidatesInput{
		CollectionIDs: []string{fixture.CollectionID, fixture.CollectionID},
		QueryText:     "alpha evidence",
		Limit:         4,
	})
	if err != nil {
		t.Fatalf("fetch candidates error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(candidates), candidates)
	}
	got := candidates[0]
	want := fixture.Reference
	if got.CollectionID != want.CollectionID ||
		got.DocumentID != want.DocumentID ||
		got.DocumentVersionID != want.DocumentVersionID ||
		got.IndexGenerationID != want.IndexGenerationID ||
		got.MaterializationID != want.MaterializationID ||
		got.ParentChunkID != want.ParentChunkID ||
		got.ChildChunkID != want.ChildChunkID ||
		got.SourceSpanHash != want.SourceSpanHash ||
		got.ContentHash != want.ContentHash ||
		got.RankScore <= 0 {
		t.Fatalf("candidate = %#v, want reference %#v with positive rank", got, want)
	}

	_, err = repo.FetchQueryEvidenceCandidates(ctx, QueryEvidenceCandidatesInput{
		CollectionIDs: []string{fixture.CollectionID},
		QueryText:     " ",
		Limit:         4,
	})
	if !errors.Is(err, ErrEvidenceHydrationRejected) {
		t.Fatalf("blank query error = %v, want ErrEvidenceHydrationRejected", err)
	}

	hybrid, err := repo.FetchHybridQueryEvidenceCandidates(ctx, HybridQueryEvidenceCandidatesInput{
		CollectionIDs:  []string{fixture.CollectionID},
		QueryText:      "semantic paraphrase without lexical overlap",
		QueryEmbedding: repeatedEvidenceVector(0.001),
		Limit:          4,
	})
	if err != nil {
		t.Fatalf("fetch hybrid candidates error = %v", err)
	}
	if len(hybrid) != 1 || hybrid[0].ChildChunkID != fixture.Reference.ChildChunkID || hybrid[0].RankScore <= 0 {
		t.Fatalf("hybrid candidates = %#v, want BGE Dense reference", hybrid)
	}
	var diagnosticGenerationID string
	if err := db.QueryRowContext(ctx, `
SELECT index_generation_id
FROM knowledge_fetch_hybrid_shadow_diagnostics(
  $1::uuid[],
  'semantic paraphrase without lexical overlap',
  $2::real[]::vector(1024),
  4
)
LIMIT 1
`, evidenceUUIDArrayLiteral([]string{fixture.CollectionID}),
		evidenceRealArrayLiteral(repeatedEvidenceVector(0.001))).Scan(
		&diagnosticGenerationID,
	); err != nil {
		t.Fatalf("fetch active-profile diagnostics: %v", err)
	}
	if diagnosticGenerationID != fixture.Reference.IndexGenerationID {
		t.Fatalf("diagnostic generation = %s", diagnosticGenerationID)
	}

	profiledHybrid, err := repo.FetchHybridQueryEvidenceCandidates(ctx, HybridQueryEvidenceCandidatesInput{
		CollectionIDs:  []string{fixture.CollectionID},
		QueryText:      "semantic paraphrase without lexical overlap",
		QueryEmbedding: repeatedEvidenceVector(0.001),
		Limit:          4,
	})
	if err != nil || len(profiledHybrid) != 1 ||
		profiledHybrid[0].ChildChunkID != fixture.Reference.ChildChunkID {
		t.Fatalf("profiled hybrid candidates = %#v, %v", profiledHybrid, err)
	}
	profiledLexical, err := repo.FetchQueryEvidenceCandidates(ctx, QueryEvidenceCandidatesInput{
		CollectionIDs: []string{fixture.CollectionID},
		QueryText:     "alpha evidence",
		Limit:         4,
	})
	if err != nil || len(profiledLexical) != 1 ||
		profiledLexical[0].ChildChunkID != fixture.Reference.ChildChunkID {
		t.Fatalf("profiled lexical candidates = %#v, %v", profiledLexical, err)
	}

	binding, err := repo.ResolveActiveRetrievalProfile(ctx)
	if err != nil {
		t.Fatalf("resolve active retrieval profile: %v", err)
	}
	if binding.IndexGenerationID != fixture.Reference.IndexGenerationID ||
		binding.RetrievalProfileID != "siliconflow_bge_m3_v1" ||
		binding.EmbeddingModelID != "Pro/BAAI/bge-m3" {
		t.Fatalf("active retrieval binding = %#v", binding)
	}
	fenced, err := repo.FetchFencedHybridQueryEvidenceCandidates(
		ctx,
		FencedHybridQueryEvidenceCandidatesInput{
			Binding: binding, CollectionIDs: []string{fixture.CollectionID},
			QueryText:      "semantic paraphrase without lexical overlap",
			QueryEmbedding: repeatedEvidenceVector(0.001), Limit: 4,
		},
	)
	if err != nil || len(fenced) != 1 ||
		fenced[0].ChildChunkID != fixture.Reference.ChildChunkID {
		t.Fatalf("fenced hybrid candidates = %#v, %v", fenced, err)
	}
	fencedLexical, err := repo.FetchFencedQueryEvidenceCandidates(
		ctx,
		FencedQueryEvidenceCandidatesInput{
			Binding: binding, CollectionIDs: []string{fixture.CollectionID},
			QueryText: "alpha evidence", Limit: 4,
		},
	)
	if err != nil || len(fencedLexical) != 1 ||
		fencedLexical[0].ChildChunkID != fixture.Reference.ChildChunkID {
		t.Fatalf("fenced lexical candidates = %#v, %v", fencedLexical, err)
	}

	staleBinding := binding
	staleBinding.SearchProfileID = "74000000-0000-4000-8000-000000000099"
	_, err = repo.FetchFencedHybridQueryEvidenceCandidates(
		ctx,
		FencedHybridQueryEvidenceCandidatesInput{
			Binding: staleBinding, CollectionIDs: []string{fixture.CollectionID},
			QueryText:      "alpha evidence",
			QueryEmbedding: repeatedEvidenceVector(0.001), Limit: 4,
		},
	)
	if !errors.Is(err, ErrRetrievalProfileChanged) {
		t.Fatalf("stale profile error = %v, want ErrRetrievalProfileChanged", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT active_profile, revision
FROM knowledge_set_retrieval_profile(
  'pg17_bm25_pgvector_v1', 'legacy', 2, 'integration test rollback'
)
`).Scan(&activeProfile, &retrievalRevision); err != nil {
		t.Fatalf("restore legacy retrieval profile: %v", err)
	}
	_, err = repo.FetchFencedHybridQueryEvidenceCandidates(
		ctx,
		FencedHybridQueryEvidenceCandidatesInput{
			Binding: binding, CollectionIDs: []string{fixture.CollectionID},
			QueryText:      "alpha evidence",
			QueryEmbedding: repeatedEvidenceVector(0.001), Limit: 4,
		},
	)
	if !errors.Is(err, ErrRetrievalProfileChanged) {
		t.Fatalf("rolled-back profile error = %v, want ErrRetrievalProfileChanged", err)
	}
}

func TestNormalizeReauthorizeEvidenceRejectsUnsafeCandidateShapes(t *testing.T) {
	base := ReauthorizeEvidenceInput{
		ActorUserID:           "74000000-0000-4000-8000-000000000001",
		SessionID:             "74000000-0000-4000-8000-000000000002",
		ConversationID:        "74000000-0000-4000-8000-000000000003",
		SelectedCollectionIDs: []string{"74000000-0000-4000-8000-000000000004"},
		References: []EvidenceCandidateReference{{
			CollectionID:      "74000000-0000-4000-8000-000000000004",
			DocumentID:        "74000000-0000-4000-8000-000000000005",
			DocumentVersionID: "74000000-0000-4000-8000-000000000006",
			IndexGenerationID: "74000000-0000-4000-8000-000000000007",
			MaterializationID: "74000000-0000-4000-8000-000000000008",
			ParentChunkID:     "74000000-0000-4000-8000-000000000009",
			ChildChunkID:      "74000000-0000-4000-8000-000000000010",
			SourceSpanHash:    strings.Repeat("a", 64),
			ContentHash:       strings.Repeat("b", 64),
			RankScore:         0.25,
		}},
	}

	cases := []struct {
		name  string
		input ReauthorizeEvidenceInput
	}{
		{name: "zero uuid", input: withEvidenceReference(base, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
			reference.ChildChunkID = "00000000-0000-0000-0000-000000000000"
			return reference
		})},
		{name: "bad hash", input: withEvidenceReference(base, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
			reference.SourceSpanHash = "not-a-hash"
			return reference
		})},
		{name: "negative rank", input: withEvidenceReference(base, func(reference EvidenceCandidateReference) EvidenceCandidateReference {
			reference.RankScore = -0.1
			return reference
		})},
		{name: "duplicate reference", input: func() ReauthorizeEvidenceInput {
			input := base
			input.References = []EvidenceCandidateReference{base.References[0], base.References[0]}
			return input
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := normalizeReauthorizeEvidenceInput(tc.input); !errors.Is(err, ErrEvidenceHydrationRejected) {
				t.Fatalf("error = %v, want ErrEvidenceHydrationRejected", err)
			}
		})
	}
}

func TestNormalizeQueryEvidenceCandidatesInputRejectsUnsafeShapes(t *testing.T) {
	base := QueryEvidenceCandidatesInput{
		CollectionIDs: []string{"74000000-0000-4000-8000-000000000004"},
		QueryText:     "alpha evidence",
		Limit:         8,
	}
	cases := []struct {
		name  string
		input QueryEvidenceCandidatesInput
	}{
		{name: "missing collection", input: QueryEvidenceCandidatesInput{QueryText: base.QueryText, Limit: base.Limit}},
		{name: "zero collection uuid", input: QueryEvidenceCandidatesInput{
			CollectionIDs: []string{"00000000-0000-0000-0000-000000000000"},
			QueryText:     base.QueryText,
			Limit:         base.Limit,
		}},
		{name: "blank query", input: QueryEvidenceCandidatesInput{CollectionIDs: base.CollectionIDs, QueryText: " ", Limit: base.Limit}},
		{name: "bad limit", input: QueryEvidenceCandidatesInput{CollectionIDs: base.CollectionIDs, QueryText: base.QueryText, Limit: 51}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := normalizeQueryEvidenceCandidatesInput(tc.input); !errors.Is(err, ErrEvidenceHydrationRejected) {
				t.Fatalf("error = %v, want ErrEvidenceHydrationRejected", err)
			}
		})
	}

	normalized, err := normalizeQueryEvidenceCandidatesInput(QueryEvidenceCandidatesInput{
		CollectionIDs: []string{base.CollectionIDs[0], base.CollectionIDs[0]},
		QueryText:     " alpha evidence ",
	})
	if err != nil {
		t.Fatalf("normalize valid input error = %v", err)
	}
	if normalized.Limit != defaultEvidenceCandidateLimit ||
		normalized.QueryText != base.QueryText ||
		len(normalized.CollectionIDs) != 1 {
		t.Fatalf("normalized input = %#v", normalized)
	}
}

func TestNormalizeHybridQueryEvidenceCandidatesRejectsUnsafeVectors(t *testing.T) {
	base := HybridQueryEvidenceCandidatesInput{
		CollectionIDs:  []string{"74000000-0000-4000-8000-000000000004"},
		QueryText:      "semantic question",
		QueryEmbedding: repeatedEvidenceVector(0.001),
		Limit:          8,
	}
	cases := []struct {
		name   string
		vector []float32
	}{
		{name: "short", vector: []float32{0.1}},
		{name: "zero norm", vector: make([]float32, evidenceQueryEmbeddingDimensions)},
		{name: "nan", vector: func() []float32 {
			vector := repeatedEvidenceVector(0.001)
			vector[10] = float32(math.NaN())
			return vector
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.QueryEmbedding = tc.vector
			if _, err := normalizeHybridQueryEvidenceCandidatesInput(input); !errors.Is(err, ErrEvidenceHydrationRejected) {
				t.Fatalf("error = %v, want ErrEvidenceHydrationRejected", err)
			}
		})
	}

	normalized, err := normalizeHybridQueryEvidenceCandidatesInput(HybridQueryEvidenceCandidatesInput{
		CollectionIDs:  append(base.CollectionIDs, base.CollectionIDs[0]),
		QueryText:      " semantic question ",
		QueryEmbedding: base.QueryEmbedding,
	})
	if err != nil {
		t.Fatalf("normalize valid hybrid input error = %v", err)
	}
	if normalized.Limit != defaultEvidenceCandidateLimit ||
		normalized.QueryText != base.QueryText ||
		len(normalized.CollectionIDs) != 1 ||
		len(normalized.QueryEmbedding) != evidenceQueryEmbeddingDimensions {
		t.Fatalf("normalized hybrid input = %#v", normalized)
	}
}

func TestOnlySiliconFlowBindingIsExecutableForDenseRetrieval(t *testing.T) {
	jina := RetrievalProfileBinding{
		IndexGenerationID:   "74000000-0000-4000-8000-000000000001",
		SearchProfileID:     "74000000-0000-4000-8000-000000000002",
		RetrievalProfileID:  "jina_v4_v3",
		ProviderID:          "jina",
		EmbeddingModelID:    "jina-embeddings-v4",
		EmbeddingDimensions: 1024,
		RerankModelID:       "jina-reranker-v3",
	}
	if _, err := normalizeRetrievalProfileBinding(jina); err != nil {
		t.Fatalf("historical Jina binding must remain decodable: %v", err)
	}
	if isExecutableSiliconFlowRetrievalBinding(jina) {
		t.Fatal("historical Jina binding is executable for Dense retrieval")
	}

	bge := jina
	bge.RetrievalProfileID = "siliconflow_bge_m3_v1"
	bge.ProviderID = "siliconflow"
	bge.EmbeddingModelID = "Pro/BAAI/bge-m3"
	bge.RerankModelID = "Pro/BAAI/bge-reranker-v2-m3"
	if normalized, err := normalizeRetrievalProfileBinding(bge); err != nil ||
		!isExecutableSiliconFlowRetrievalBinding(normalized) {
		t.Fatalf("BGE binding is not executable: %#v, %v", normalized, err)
	}
}

func TestPostgresHybridCandidatesRejectShortDenseOnlyQuery(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	fixture := seedEvidenceHydrationFixture(t, ctx, db)
	seedEvidenceSearchProjection(t, ctx, db, fixture)
	repo := NewPostgresRepository(db)

	candidates, err := repo.FetchHybridQueryEvidenceCandidates(ctx, HybridQueryEvidenceCandidatesInput{
		CollectionIDs:  []string{fixture.CollectionID},
		QueryText:      "今天天气如何？",
		QueryEmbedding: repeatedEvidenceVector(0.001),
		Limit:          4,
	})
	if err != nil {
		t.Fatalf("fetch short-query hybrid candidates error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("short Dense-only candidates = %#v, want none", candidates)
	}
}

func repeatedEvidenceVector(value float32) []float32 {
	vector := make([]float32, evidenceQueryEmbeddingDimensions)
	for index := range vector {
		vector[index] = value
	}
	return vector
}

func withEvidenceReference(
	input ReauthorizeEvidenceInput,
	mutate func(EvidenceCandidateReference) EvidenceCandidateReference,
) ReauthorizeEvidenceInput {
	input.References = append([]EvidenceCandidateReference(nil), input.References...)
	input.References[0] = mutate(input.References[0])
	return input
}

func seedEvidenceHydrationFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) evidenceHydrationFixture {
	t.Helper()
	const (
		memberUserID      = "74000000-0000-4000-8000-000000000001"
		outsiderUserID    = "74000000-0000-4000-8000-000000000002"
		teamID            = "74000000-0000-4000-8000-000000000003"
		memberSessionID   = "74000000-0000-4000-8000-000000000004"
		outsiderSessionID = "74000000-0000-4000-8000-000000000005"
		memberConvID      = "74000000-0000-4000-8000-000000000006"
		outsiderConvID    = "74000000-0000-4000-8000-000000000007"
		collectionID      = "74000000-0000-4000-8000-000000000008"
		fileID            = "74000000-0000-4000-8000-000000000009"
		indexProfileID    = "74000000-0000-4000-8000-000000000010"
		indexGenerationID = "74000000-0000-4000-8000-000000000011"
		documentID        = "74000000-0000-4000-8000-000000000012"
		versionID         = "74000000-0000-4000-8000-000000000013"
		materializationID = "74000000-0000-4000-8000-000000000014"
		parentChunkID     = "74000000-0000-4000-8000-000000000015"
		childChunkID      = "74000000-0000-4000-8000-000000000016"
	)
	const (
		sourceContentHash = "1111111111111111111111111111111111111111111111111111111111111111"
		baseProfileHash   = "2222222222222222222222222222222222222222222222222222222222222222"
		chunkProfileHash  = "3333333333333333333333333333333333333333333333333333333333333333"
		sourceSpanHash    = "4444444444444444444444444444444444444444444444444444444444444444"
		parentContentHash = "5555555555555555555555555555555555555555555555555555555555555555"
		childContentHash  = "6666666666666666666666666666666666666666666666666666666666666666"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users(id,email,display_name) VALUES
  ($1,'member@example.test','Member'),
  ($2,'outsider@example.test','Outsider');
INSERT INTO teams(id,name,created_by_user_id) VALUES ($3,'RAG Team',$1);
INSERT INTO team_memberships(team_id,user_id,role,status) VALUES
  ($3,$1,'member','active');
INSERT INTO sessions(id,user_id,token_hash,expires_at) VALUES
  ($4,$1,'member-token-hash',clock_timestamp() + interval '1 hour'),
  ($5,$2,'outsider-token-hash',clock_timestamp() + interval '1 hour');
INSERT INTO conversations(id,user_id,title,status) VALUES
  ($6,$1,'member chat','active'),
  ($7,$2,'outsider chat','active');
INSERT INTO knowledge_collections(id,name,scope,team_id)
  VALUES ($8,'Team Knowledge','team',$3);
INSERT INTO files(id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,object_key,metadata)
  VALUES ($9,$1,'source.pdf','application/pdf',10,$17,'local','knowledge/source.pdf','{"purpose":"knowledge"}');
INSERT INTO knowledge_index_profiles(
  id,contract_version,canonical_schema_version,parser_manifest,parser_manifest_hash,
  chunk_manifest,chunk_profile_hash,embedding_processor,embedding_endpoint_id,
  embedding_model_id,embedding_api_version,embedding_role,rerank_processor,
  rerank_endpoint_id,rerank_model_id,rerank_api_version,base_profile_hash
) VALUES (
  $10,1,'canonical-ir.v2','{}',$17,'{}',$19,'siliconflow','siliconflow-cn-v1',
  'Pro/BAAI/bge-m3','v1','passage','siliconflow','siliconflow-cn-v1',
  'Pro/BAAI/bge-reranker-v2-m3','v1',$18
);
INSERT INTO knowledge_search_profiles(
  id,index_profile_id,provider_profile_id,embedding_processor,
  embedding_model_id,embedding_dimensions,rerank_processor,rerank_model_id,
  lexical_config,exact_config,profile_hash
) VALUES (
  '74000000-0000-4000-8000-000000000017',$10,
  'siliconflow_bge_m3_v1','siliconflow','Pro/BAAI/bge-m3',1024,
  'siliconflow','Pro/BAAI/bge-reranker-v2-m3','{}','{}',repeat('7',64)
);
INSERT INTO knowledge_index_generations(
  id,index_profile_id,generation_seq,status,build_snapshot,build_snapshot_hash,
  artifact_manifest_hash,verified_at,activated_at
) VALUES ($11,$10,1,'active','{}',$17,$17,clock_timestamp(),clock_timestamp());
INSERT INTO knowledge_projection_state(
  index_generation_id,readiness,projection_revision,required_outbox_floor,
  contiguous_applied_outbox_id,manifest_hash,verified_at
) VALUES ($11,'ready',1,0,0,$17,clock_timestamp());
UPDATE knowledge_corpus_projection_head
SET active_index_generation_id=$11, updated_at=clock_timestamp()
WHERE singleton_id=1;
INSERT INTO knowledge_documents(id,collection_id,status)
  VALUES ($12,$8,'processing');
INSERT INTO knowledge_document_versions(id,document_id,file_id,source_version,status,content_hash)
  VALUES ($13,$12,$9,1,'active',$17);
UPDATE knowledge_documents
SET status='active', current_version_id=$13, updated_at=clock_timestamp()
WHERE id=$12;
INSERT INTO knowledge_document_materializations(
  id,index_generation_id,collection_id,document_id,document_version_id,file_id,
  materialization_seq,source_content_hash,base_profile_hash,
  collection_acl_revision,collection_visibility_epoch,collection_processing_revision,
  document_visibility_epoch,status,manifest_hash,result_hash,verified_at,published_at
) VALUES (
  $14,$11,$8,$12,$13,$9,1,$17,$18,1,1,1,1,'published',$17,$17,
  clock_timestamp(),clock_timestamp()
);
INSERT INTO knowledge_document_projection_heads(
  index_generation_id,document_id,active_materialization_id,
  document_projection_revision,last_corpus_projection_revision
) VALUES ($11,$12,$14,1,1);
INSERT INTO knowledge_parent_chunks(
  id,materialization_id,index_generation_id,document_id,document_version_id,
  ordinal,chunk_profile_hash,source_span_hash,content_hash,content,token_count,
  heading_path,locator_summary
	) VALUES (
	  $15,$14,$11,$12,$13,0,$19,$20,$21,'alpha evidence parent',4,
	  ARRAY['Manual']::text[],
	  '{
	    "schemaVersion":"g7.4-locator-summary.v1",
	    "primary":{
	      "kind":"line_range",
	      "locator":{"kind":"line_range","startLine":0,"endLine":10},
	      "locatorAggregateHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	    },
	    "fragments":[{
	      "kind":"line_range",
	      "locator":{"kind":"line_range","startLine":0,"endLine":10},
	      "locatorAggregateHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	    }],
	    "locatorAggregateHashes":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
	  }'::jsonb
);
INSERT INTO knowledge_child_chunks(
  id,parent_chunk_id,materialization_id,index_generation_id,document_id,
  document_version_id,ordinal,chunk_profile_hash,source_span_hash,content_hash,
  content,token_count
) VALUES (
  $16,$15,$14,$11,$12,$13,0,$19,$20,$22,'alpha evidence source',3
);
`, memberUserID, outsiderUserID, teamID, memberSessionID, outsiderSessionID,
		memberConvID, outsiderConvID, collectionID, fileID, indexProfileID,
		indexGenerationID, documentID, versionID, materializationID,
		parentChunkID, childChunkID, sourceContentHash,
		baseProfileHash, chunkProfileHash, sourceSpanHash, parentContentHash,
		childContentHash)
	return evidenceHydrationFixture{
		MemberUserID:      memberUserID,
		OutsiderUserID:    outsiderUserID,
		MemberSessionID:   memberSessionID,
		OutsiderSessionID: outsiderSessionID,
		MemberConvID:      memberConvID,
		OutsiderConvID:    outsiderConvID,
		CollectionID:      collectionID,
		Reference: EvidenceCandidateReference{
			CollectionID:      collectionID,
			DocumentID:        documentID,
			DocumentVersionID: versionID,
			IndexGenerationID: indexGenerationID,
			MaterializationID: materializationID,
			ParentChunkID:     parentChunkID,
			ChildChunkID:      childChunkID,
			SourceSpanHash:    sourceSpanHash,
			ContentHash:       childContentHash,
			RankScore:         0.75,
		},
	}
}

func seedEvidenceSearchProjection(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	fixture evidenceHydrationFixture,
) {
	t.Helper()
	const vectorHash = "8888888888888888888888888888888888888888888888888888888888888888"
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO knowledge_child_search_projections(
  child_chunk_id,parent_chunk_id,materialization_id,index_generation_id,
  collection_id,document_id,document_version_id,search_profile_id,
  embedding_model_id,embedding_dimensions,embedding_vector,
  embedding_vector_sha256,lexical_text,exact_terms,source_span_hash,
  chunk_profile_hash,content_hash,locator_summary,status,ready_at
) SELECT
  child.id,child.parent_chunk_id,child.materialization_id,child.index_generation_id,
  materialization.collection_id,child.document_id,child.document_version_id,$1,
  'Pro/BAAI/bge-m3',1024,
  ARRAY(SELECT 0.001::real FROM generate_series(1,1024)),
  $2,child.content,ARRAY['alpha','evidence']::text[],child.source_span_hash,
	  child.chunk_profile_hash,child.content_hash,
	  '{
	    "schemaVersion":"g7.4-locator-summary.v1",
	    "primary":{
	      "kind":"line_range",
	      "locator":{"kind":"line_range","startLine":2,"endLine":3},
	      "locatorAggregateHash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	    },
	    "fragments":[{
	      "kind":"line_range",
	      "locator":{"kind":"line_range","startLine":2,"endLine":3},
	      "locatorAggregateHash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	    }],
	    "locatorAggregateHashes":["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]
	  }'::jsonb,'ready',
  clock_timestamp()
FROM knowledge_child_chunks child
JOIN knowledge_document_materializations materialization
  ON materialization.id=child.materialization_id
WHERE child.id=$3;
`, evidenceSearchProfileID, vectorHash, fixture.Reference.ChildChunkID)
}
