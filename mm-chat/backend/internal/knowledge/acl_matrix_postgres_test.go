package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

type knowledgeACLFixture struct {
	ownerID, teamID, personalID, teamCollectionID string
	personalDocumentID, teamDocumentID            string
}

func TestPostgresKnowledgeACLTwoUsersTwoTeamsAndDisabledActor(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const (
		userA    = "11000000-0000-4000-8000-000000000001"
		userB    = "11000000-0000-4000-8000-000000000002"
		memberA  = "11000000-0000-4000-8000-000000000003"
		removedA = "11000000-0000-4000-8000-000000000004"
		teamA    = "21000000-0000-4000-8000-000000000001"
		teamB    = "21000000-0000-4000-8000-000000000002"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users (id,email,display_name) VALUES
  ($1,'acl-a@example.test','ACL A'),
  ($2,'acl-b@example.test','ACL B'),
  ($5,'acl-member@example.test','ACL Member'),
  ($6,'acl-removed@example.test','ACL Removed');
INSERT INTO teams (id,name,created_by_user_id) VALUES
  ($3,'ACL Team A',$1),
  ($4,'ACL Team B',$2);
INSERT INTO team_memberships (team_id,user_id,role) VALUES
  ($3,$1,'admin'),
  ($4,$2,'admin'),
  ($3,$5,'member');
INSERT INTO team_memberships (team_id,user_id,role,status,removed_at)
VALUES ($3,$6,'member','removed',clock_timestamp())
`, userA, userB, teamA, teamB, memberA, removedA)

	fixtures := []knowledgeACLFixture{
		{
			ownerID: "11000000-0000-4000-8000-000000000001", teamID: "21000000-0000-4000-8000-000000000001",
			personalID: "31000000-0000-4000-8000-000000000001", teamCollectionID: "31000000-0000-4000-8000-000000000002",
			personalDocumentID: "41000000-0000-4000-8000-000000000001", teamDocumentID: "41000000-0000-4000-8000-000000000002",
		},
		{
			ownerID: "11000000-0000-4000-8000-000000000002", teamID: "21000000-0000-4000-8000-000000000002",
			personalID: "31000000-0000-4000-8000-000000000003", teamCollectionID: "31000000-0000-4000-8000-000000000004",
			personalDocumentID: "41000000-0000-4000-8000-000000000003", teamDocumentID: "41000000-0000-4000-8000-000000000004",
		},
	}
	for index, value := range fixtures {
		seedKnowledgeACLFixture(t, ctx, db, index, value)
	}

	repo := NewPostgresRepository(db)
	for index, actorID := range []string{userA, userB} {
		own := fixtures[index]
		other := fixtures[1-index]
		page, err := repo.ListCollections(ctx, ListCollectionsRepositoryInput{ActorUserID: actorID, Limit: 10})
		if err != nil {
			t.Fatalf("list actor %s: %v", actorID, err)
		}
		assertKnowledgeCollectionIDs(t, page.Items, own.personalID, own.teamCollectionID)

		for _, collectionID := range []string{own.personalID, own.teamCollectionID} {
			if _, err := repo.GetCollection(ctx, CollectionLookupInput{CollectionID: collectionID, ActorUserID: actorID}); err != nil {
				t.Fatalf("get own collection actor=%s collection=%s: %v", actorID, collectionID, err)
			}
			if _, err := repo.ListCollectionConsents(ctx, CollectionConsentLookupInput{CollectionID: collectionID, ActorUserID: actorID}); err != nil {
				t.Fatalf("list own consent actor=%s collection=%s: %v", actorID, collectionID, err)
			}
			documents, err := repo.ListDocuments(ctx, ListDocumentsRepositoryInput{CollectionID: collectionID, ActorUserID: actorID, Limit: 10})
			if err != nil || len(documents.Items) != 1 {
				t.Fatalf("list own documents actor=%s collection=%s page=%#v err=%v", actorID, collectionID, documents, err)
			}
		}
		for _, collectionID := range []string{other.personalID, other.teamCollectionID} {
			if _, err := repo.GetCollection(ctx, CollectionLookupInput{CollectionID: collectionID, ActorUserID: actorID}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope collection actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if _, err := repo.ListCollectionConsents(ctx, CollectionConsentLookupInput{CollectionID: collectionID, ActorUserID: actorID}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope consent actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			name := "cross-scope-denied"
			if _, err := repo.UpdateCollection(ctx, UpdateCollectionRepositoryInput{CollectionID: collectionID, ActorUserID: actorID, Name: &name}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope update actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if err := repo.DeleteCollection(ctx, DeleteCollectionRepositoryInput{CollectionID: collectionID, ActorUserID: actorID}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope delete actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if err := repo.RevokeCollectionConsent(ctx, CollectionConsentLookupInput{CollectionID: collectionID, ActorUserID: actorID, Processor: "mineru"}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope revoke actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if _, err := repo.ListDocuments(ctx, ListDocumentsRepositoryInput{CollectionID: collectionID, ActorUserID: actorID, Limit: 10}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope list documents actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if _, err := repo.CreateDocument(ctx, CreateDocumentRepositoryInput{
				DocumentID: "77000000-0000-4000-8000-000000000001", VersionID: "77000000-0000-4000-8000-000000000002",
				JobID: "77000000-0000-4000-8000-000000000003", CollectionID: collectionID, ActorUserID: actorID,
				FileID: "77000000-0000-4000-8000-000000000004", IdempotencyKey: "cross-bind",
				RequestHash: strings.Repeat("3", 64), ParseProcessor: "mineru",
			}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope bind actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
			if _, err := repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
				CollectionID: collectionID, ActorUserID: actorID, Processor: "mineru",
				Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1",
			}); !errors.Is(err, ErrCollectionNotFound) {
				t.Fatalf("cross-scope put consent actor=%s collection=%s error=%v", actorID, collectionID, err)
			}
		}
		if _, err := repo.CreateCollection(ctx, CreateCollectionRepositoryInput{
			ID: fmt.Sprintf("75000000-0000-4000-8000-%012d", index+1), ActorUserID: actorID,
			Name: "Cross Team", Scope: ScopeTeam, TeamID: other.teamID,
			IdempotencyKey: "cross-team-create", CreateRequestHash: strings.Repeat("f", 64),
		}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("cross-team create actor=%s team=%s error=%v", actorID, other.teamID, err)
		}

		for _, documentID := range []string{own.personalDocumentID, own.teamDocumentID} {
			if _, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: actorID}); err != nil {
				t.Fatalf("get own document actor=%s document=%s: %v", actorID, documentID, err)
			}
			if _, err := repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: actorID}); err != nil {
				t.Fatalf("get own content actor=%s document=%s: %v", actorID, documentID, err)
			}
		}
		for _, documentID := range []string{other.personalDocumentID, other.teamDocumentID} {
			if _, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: actorID}); !errors.Is(err, ErrDocumentNotFound) {
				t.Fatalf("cross-scope document actor=%s document=%s error=%v", actorID, documentID, err)
			}
			if _, err := repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: actorID}); !errors.Is(err, ErrDocumentNotFound) {
				t.Fatalf("cross-scope content actor=%s document=%s error=%v", actorID, documentID, err)
			}
			if _, err := repo.ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
				JobID: "76000000-0000-4000-8000-000000000001", DocumentID: documentID, ActorUserID: actorID,
				IdempotencyKey: "cross-reprocess", RequestHash: strings.Repeat("1", 64), ParseProcessor: "mineru",
			}); !errors.Is(err, ErrDocumentNotFound) {
				t.Fatalf("cross-scope reprocess actor=%s document=%s error=%v", actorID, documentID, err)
			}
			if err := repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{DocumentID: documentID, ActorUserID: actorID}); !errors.Is(err, ErrDocumentNotFound) {
				t.Fatalf("cross-scope delete actor=%s document=%s error=%v", actorID, documentID, err)
			}
			if _, err := repo.CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
				VersionID: "76000000-0000-4000-8000-000000000002", JobID: "76000000-0000-4000-8000-000000000003",
				DocumentID: documentID, ActorUserID: actorID, FileID: "76000000-0000-4000-8000-000000000004",
				IdempotencyKey: "cross-version", RequestHash: strings.Repeat("2", 64), ParseProcessor: "mineru",
			}); !errors.Is(err, ErrDocumentNotFound) {
				t.Fatalf("cross-scope replace actor=%s document=%s error=%v", actorID, documentID, err)
			}
		}
	}

	teamAPage, err := repo.ListCollections(ctx, ListCollectionsRepositoryInput{ActorUserID: memberA, Limit: 10})
	if err != nil {
		t.Fatalf("active member list: %v", err)
	}
	assertKnowledgeCollectionIDs(t, teamAPage.Items, fixtures[0].teamCollectionID)
	if _, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: fixtures[0].teamDocumentID, ActorUserID: memberA}); err != nil {
		t.Fatalf("active member document read: %v", err)
	}
	if _, err := repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{DocumentID: fixtures[0].teamDocumentID, ActorUserID: memberA}); err != nil {
		t.Fatalf("active member content read: %v", err)
	}
	if _, err := repo.ListCollectionConsents(ctx, CollectionConsentLookupInput{CollectionID: fixtures[0].teamCollectionID, ActorUserID: memberA}); err != nil {
		t.Fatalf("active member consent read: %v", err)
	}
	memberRename := "member-denied"
	if _, err := repo.UpdateCollection(ctx, UpdateCollectionRepositoryInput{CollectionID: fixtures[0].teamCollectionID, ActorUserID: memberA, Name: &memberRename}); !errors.Is(err, ErrTeamAdminRequired) {
		t.Fatalf("active member mutation error = %v", err)
	}
	removedPage, err := repo.ListCollections(ctx, ListCollectionsRepositoryInput{ActorUserID: removedA, Limit: 10})
	if err != nil || len(removedPage.Items) != 0 {
		t.Fatalf("removed member list = %#v, err=%v", removedPage, err)
	}
	if _, err := repo.GetCollection(ctx, CollectionLookupInput{CollectionID: fixtures[0].teamCollectionID, ActorUserID: removedA}); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("removed member collection error = %v", err)
	}

	mustKnowledgeExec(t, ctx, db, `UPDATE users SET account_status='disabled' WHERE id=$1`, userA)
	page, err := repo.ListCollections(ctx, ListCollectionsRepositoryInput{ActorUserID: userA, Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("disabled actor list = %#v, err=%v", page, err)
	}
	for _, collectionID := range []string{fixtures[0].personalID, fixtures[0].teamCollectionID} {
		if _, err := repo.GetCollection(ctx, CollectionLookupInput{CollectionID: collectionID, ActorUserID: userA}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor collection=%s error=%v", collectionID, err)
		}
		if _, err := repo.ListCollectionConsents(ctx, CollectionConsentLookupInput{CollectionID: collectionID, ActorUserID: userA}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor consent collection=%s error=%v", collectionID, err)
		}
		name := "denied"
		if _, err := repo.UpdateCollection(ctx, UpdateCollectionRepositoryInput{CollectionID: collectionID, ActorUserID: userA, Name: &name}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor update collection=%s error=%v", collectionID, err)
		}
		if _, err := repo.CreateDocument(ctx, CreateDocumentRepositoryInput{
			DocumentID: "71000000-0000-4000-8000-000000000001", VersionID: "71000000-0000-4000-8000-000000000002",
			JobID: "71000000-0000-4000-8000-000000000003", CollectionID: collectionID, ActorUserID: userA,
			FileID: "71000000-0000-4000-8000-000000000004", IdempotencyKey: "disabled-bind",
			RequestHash: strings.Repeat("a", 64), ParseProcessor: "mineru",
		}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor bind collection=%s error=%v", collectionID, err)
		}
		if _, err := repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
			CollectionID: collectionID, ActorUserID: userA, Processor: "mineru",
			Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1",
		}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor put consent collection=%s error=%v", collectionID, err)
		}
		if err := repo.RevokeCollectionConsent(ctx, CollectionConsentLookupInput{CollectionID: collectionID, ActorUserID: userA, Processor: "mineru"}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor revoke consent collection=%s error=%v", collectionID, err)
		}
		if err := repo.DeleteCollection(ctx, DeleteCollectionRepositoryInput{CollectionID: collectionID, ActorUserID: userA}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor delete collection=%s error=%v", collectionID, err)
		}
		if _, err := repo.ListDocuments(ctx, ListDocumentsRepositoryInput{CollectionID: collectionID, ActorUserID: userA, Limit: 10}); !errors.Is(err, ErrCollectionNotFound) {
			t.Fatalf("disabled actor list documents collection=%s error=%v", collectionID, err)
		}
	}
	if _, err := repo.CreateCollection(ctx, CreateCollectionRepositoryInput{
		ID: "73000000-0000-4000-8000-000000000001", ActorUserID: userA, Name: "Disabled Personal",
		Scope: ScopePersonal, IdempotencyKey: "disabled-personal", CreateRequestHash: strings.Repeat("c", 64),
	}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled actor create personal error = %v", err)
	}
	if _, err := repo.CreateCollection(ctx, CreateCollectionRepositoryInput{
		ID: "73000000-0000-4000-8000-000000000002", ActorUserID: userA, Name: "Disabled Team",
		Scope: ScopeTeam, TeamID: teamA, IdempotencyKey: "disabled-team", CreateRequestHash: strings.Repeat("d", 64),
	}); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("disabled actor create team error = %v", err)
	}
	for _, documentID := range []string{fixtures[0].personalDocumentID, fixtures[0].teamDocumentID} {
		if _, err := repo.GetDocument(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: userA}); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("disabled actor document=%s error=%v", documentID, err)
		}
		if _, err := repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: userA}); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("disabled actor content=%s error=%v", documentID, err)
		}
		if _, err := repo.ReprocessDocument(ctx, ReprocessDocumentRepositoryInput{
			JobID: "72000000-0000-4000-8000-000000000001", DocumentID: documentID, ActorUserID: userA,
			IdempotencyKey: "disabled-reprocess", RequestHash: strings.Repeat("b", 64), ParseProcessor: "mineru",
		}); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("disabled actor reprocess document=%s error=%v", documentID, err)
		}
		if err := repo.DeleteDocument(ctx, DeleteDocumentRepositoryInput{DocumentID: documentID, ActorUserID: userA}); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("disabled actor delete document=%s error=%v", documentID, err)
		}
		if _, err := repo.CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
			VersionID: "74000000-0000-4000-8000-000000000001", JobID: "74000000-0000-4000-8000-000000000002",
			DocumentID: documentID, ActorUserID: userA, FileID: "74000000-0000-4000-8000-000000000003",
			IdempotencyKey: "disabled-version", RequestHash: strings.Repeat("e", 64), ParseProcessor: "mineru",
		}); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("disabled actor replace document=%s error=%v", documentID, err)
		}
	}
	if _, err := repo.GetCollection(ctx, CollectionLookupInput{CollectionID: fixtures[1].personalID, ActorUserID: userB}); err != nil {
		t.Fatalf("disabling user A affected user B: %v", err)
	}
}

func seedKnowledgeACLFixture(t *testing.T, ctx context.Context, db *sql.DB, index int, value knowledgeACLFixture) {
	t.Helper()
	for collectionIndex, collection := range []struct {
		id, scope, documentID string
	}{
		{id: value.personalID, scope: ScopePersonal, documentID: value.personalDocumentID},
		{id: value.teamCollectionID, scope: ScopeTeam, documentID: value.teamDocumentID},
	} {
		fileID := fmt.Sprintf("51000000-0000-4000-8000-%012d", index*2+collectionIndex+1)
		versionID := fmt.Sprintf("61000000-0000-4000-8000-%012d", index*2+collectionIndex+1)
		owner, team := any(value.ownerID), any(nil)
		if collection.scope == ScopeTeam {
			owner, team = nil, value.teamID
		}
		mustKnowledgeExec(t, ctx, db, `
INSERT INTO knowledge_collections (id,name,scope,owner_user_id,team_id)
VALUES ($1,$2,$3,$4,$5);
INSERT INTO files (
  id,user_id,original_filename,mime_type,byte_size,sha256,upload_status,
  storage_backend,object_key,metadata
) VALUES ($6,$7,$8,'application/pdf',10,$9,'available','local',$10,'{"purpose":"knowledge"}');
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($11,$1,'processing');
INSERT INTO knowledge_document_versions (
  id,document_id,file_id,source_version,status,content_hash
) VALUES ($12,$11,$6,1,'active',$9);
UPDATE knowledge_documents SET status='active',current_version_id=$12 WHERE id=$11
`, collection.id, fmt.Sprintf("ACL %d %d", index, collectionIndex), collection.scope, owner, team,
			fileID, value.ownerID, fmt.Sprintf("acl-%d-%d.pdf", index, collectionIndex), strings.Repeat(fmt.Sprint(index+collectionIndex+1), 64)[:64],
			"users/"+value.ownerID+"/files/"+fileID, collection.documentID, versionID)
	}
}

func assertKnowledgeCollectionIDs(t *testing.T, values []Collection, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(values))
	for _, value := range values {
		got[value.ID] = true
	}
	if len(got) != len(want) {
		t.Fatalf("collection ids = %v, want %v", got, want)
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("collection ids = %v, missing %s", got, id)
		}
	}
}
