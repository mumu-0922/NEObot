package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresRoutingCatalogRanksVisibleActiveMetadataOnly(t *testing.T) {
	db := openKnowledgeTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	const (
		actorID        = "19000000-0000-4000-8000-000000000001"
		outsiderID     = "19000000-0000-4000-8000-000000000002"
		visibleID      = "29000000-0000-4000-8000-000000000001"
		emptyID        = "29000000-0000-4000-8000-000000000002"
		unauthorizedID = "29000000-0000-4000-8000-000000000003"
		deletedID      = "29000000-0000-4000-8000-000000000004"
	)
	mustKnowledgeExec(t, ctx, db, `
INSERT INTO users (id,email,display_name) VALUES
 ($1,'routing-owner@example.test','Owner'),
 ($2,'routing-outsider@example.test','Outsider');
INSERT INTO knowledge_collections (id,name,scope,owner_user_id) VALUES
 ($3,'LINUX DO 资料','personal',$1),
 ($4,'Empty visible','personal',$1),
 ($5,'Unauthorized secrets','personal',$2),
 ($6,'Deleted secrets','personal',$1);
UPDATE knowledge_collections SET deleted_at=now() WHERE id=$6;
`, actorID, outsiderID, visibleID, emptyID, unauthorizedID, deletedID)

	type documentFixture struct {
		documentID  string
		versionID   string
		fileID      string
		collection  string
		title       string
		docActive   bool
		version     string
		upload      string
		fileDeleted bool
	}
	fixtures := []documentFixture{
		{
			"39000000-0000-4000-8000-000000000001",
			"49000000-0000-4000-8000-000000000001",
			"59000000-0000-4000-8000-000000000001",
			visibleID, "LINUX DO 注册申请小作文模板.md", true, "active", "available", false,
		},
		{
			"39000000-0000-4000-8000-000000000002",
			"49000000-0000-4000-8000-000000000002",
			"59000000-0000-4000-8000-000000000002",
			visibleID, "English registration handbook.pdf", true, "active", "available", false,
		},
		{
			"39000000-0000-4000-8000-000000000003",
			"49000000-0000-4000-8000-000000000003",
			"59000000-0000-4000-8000-000000000003",
			visibleID, "pending-secret.txt", false, "processing", "available", false,
		},
		{
			"39000000-0000-4000-8000-000000000004",
			"49000000-0000-4000-8000-000000000004",
			"59000000-0000-4000-8000-000000000004",
			visibleID, "unavailable-secret.txt", true, "active", "failed", false,
		},
		{
			"39000000-0000-4000-8000-000000000005",
			"49000000-0000-4000-8000-000000000005",
			"59000000-0000-4000-8000-000000000005",
			visibleID, "deleted-file-secret.txt", true, "active", "deleted", true,
		},
		{
			"39000000-0000-4000-8000-000000000006",
			"49000000-0000-4000-8000-000000000006",
			"59000000-0000-4000-8000-000000000006",
			unauthorizedID, "unauthorized-template.md", true, "active", "available", false,
		},
		{
			"39000000-0000-4000-8000-000000000007",
			"49000000-0000-4000-8000-000000000007",
			"59000000-0000-4000-8000-000000000007",
			deletedID, "deleted-collection-template.md", true, "active", "available", false,
		},
	}
	for index, fixture := range fixtures {
		mustKnowledgeExec(t, ctx, db, `
INSERT INTO files (
 id,user_id,original_filename,mime_type,byte_size,sha256,storage_backend,
 object_key,upload_status,metadata,deleted_at
) VALUES (
 $1,$2,$3,'text/plain',10,$4,'local',$5,$6,'{"purpose":"knowledge"}',
 CASE WHEN $7::boolean THEN clock_timestamp() ELSE NULL END
);
INSERT INTO knowledge_documents (id,collection_id,status) VALUES ($8,$9,'processing');
INSERT INTO knowledge_document_versions (
 id,document_id,file_id,source_version,status,content_hash
) VALUES ($10,$8,$1,1,$11,$4);
`, fixture.fileID, actorID, fixture.title,
			strings.Repeat(string("abcdef0"[index]), 64), "routing/"+fixture.fileID,
			fixture.upload, fixture.fileDeleted, fixture.documentID, fixture.collection,
			fixture.versionID, fixture.version)
		if fixture.docActive {
			mustKnowledgeExec(t, ctx, db, `
UPDATE knowledge_documents SET status='active',current_version_id=$1 WHERE id=$2
`, fixture.versionID, fixture.documentID)
		}
	}

	service := NewService(NewPostgresRepository(db))
	actorCtx := auth.WithUser(ctx, auth.User{ID: actorID})
	selection := []string{visibleID, emptyID, unauthorizedID, deletedID}
	catalog, err := service.BuildRoutingCatalog(actorCtx, RoutingCatalogInput{
		CollectionIDs: selection,
		QueryText:     "有小作文模板嘛",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.HasLexicalMatch || len(catalog.Collections) != 2 {
		t.Fatalf("CJK catalog = %#v", catalog)
	}
	visible := catalog.Collections[0]
	if visible.ID != visibleID || visible.ActiveDocumentCount != 2 ||
		len(visible.Documents) == 0 ||
		visible.Documents[0].Title != "LINUX DO 注册申请小作文模板.md" ||
		visible.Documents[0].RelevanceScore <= 0 {
		t.Fatalf("visible catalog = %#v", visible)
	}
	if catalog.Collections[1].ID != emptyID ||
		catalog.Collections[1].ActiveDocumentCount != 0 ||
		len(catalog.Collections[1].Documents) != 0 {
		t.Fatalf("empty catalog = %#v", catalog.Collections[1])
	}
	for _, collection := range catalog.Collections {
		for _, document := range collection.Documents {
			if strings.Contains(document.Title, "secret") {
				t.Fatalf("inactive or unauthorized title leaked: %#v", document)
			}
		}
	}

	english, err := service.BuildRoutingCatalog(actorCtx, RoutingCatalogInput{
		CollectionIDs: []string{visibleID}, QueryText: "English registration",
	})
	if err != nil || !english.HasLexicalMatch ||
		english.Collections[0].Documents[0].Title != "English registration handbook.pdf" {
		t.Fatalf("English catalog = %#v, err=%v", english, err)
	}
}
