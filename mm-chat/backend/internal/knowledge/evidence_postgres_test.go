package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

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
	hydrated, err := repo.ReauthorizeAndHydrateEvidence(ctx, validInput)
	if err != nil {
		t.Fatalf("valid hydration error = %v", err)
	}
	if len(hydrated) != 1 {
		t.Fatalf("hydrated count = %d, want 1", len(hydrated))
	}
	got := hydrated[0]
	if got.SourceText != "alpha evidence source" ||
		got.ContentHash != fixture.Reference.ContentHash ||
		got.SourceSpanHash != fixture.Reference.SourceSpanHash ||
		!strings.Contains(string(got.Locator), `"page": 1`) ||
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
  $10,1,'canonical-ir.v2','{}',$17,'{}',$19,'jina','hosted','jina-embeddings-v4',
  'v1','passage','jina','hosted','jina-reranker-v3','v1',$18
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
  ARRAY['Manual']::text[],'{"page": 1, "bbox": [0, 0, 10, 10]}'::jsonb
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
	const (
		searchProfileID = "74000000-0000-4000-8000-000000000017"
		profileHash     = "7777777777777777777777777777777777777777777777777777777777777777"
		vectorHash      = "8888888888888888888888888888888888888888888888888888888888888888"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO knowledge_search_profiles(
  id,index_profile_id,provider_profile_id,embedding_processor,
  embedding_model_id,embedding_dimensions,rerank_processor,rerank_model_id,
  lexical_config,exact_config,profile_hash
) SELECT
  $1,generation.index_profile_id,'mineru_jina_postgres_v1','jina',
  'jina-embeddings-v4',1024,'jina','jina-reranker-v3',
  '{}'::jsonb,'{}'::jsonb,$2
FROM knowledge_index_generations generation
WHERE generation.id=$3;
INSERT INTO knowledge_child_search_projections(
  child_chunk_id,parent_chunk_id,materialization_id,index_generation_id,
  collection_id,document_id,document_version_id,search_profile_id,
  embedding_model_id,embedding_dimensions,embedding_vector,
  embedding_vector_sha256,lexical_text,exact_terms,source_span_hash,
  chunk_profile_hash,content_hash,locator_summary,status,ready_at
) SELECT
  child.id,child.parent_chunk_id,child.materialization_id,child.index_generation_id,
  materialization.collection_id,child.document_id,child.document_version_id,$1,
  'jina-embeddings-v4',1024,
  ARRAY(SELECT 0.001::real FROM generate_series(1,1024)),
  $4,child.content,ARRAY['alpha','evidence']::text[],child.source_span_hash,
  child.chunk_profile_hash,child.content_hash,'{"page": 1}'::jsonb,'ready',
  clock_timestamp()
FROM knowledge_child_chunks child
JOIN knowledge_document_materializations materialization
  ON materialization.id=child.materialization_id
WHERE child.id=$5;
`, searchProfileID, profileHash, fixture.Reference.IndexGenerationID,
		vectorHash, fixture.Reference.ChildChunkID)
}
