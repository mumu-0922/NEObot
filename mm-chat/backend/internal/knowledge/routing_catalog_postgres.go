package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func (r *PostgresRepository) FetchRoutingCatalog(
	ctx context.Context,
	input RoutingCatalogRepositoryInput,
) ([]RoutingCatalogCollection, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	encodedTerms, err := json.Marshal(input.QueryTerms)
	if err != nil {
		return nil, fmt.Errorf("encode routing catalog terms: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
WITH selected AS (
  SELECT id, ordinality
  FROM unnest($1::uuid[]) WITH ORDINALITY AS item(id, ordinality)
), authorized AS (
  SELECT c.id, c.name, c.description, selected.ordinality
  FROM selected
  JOIN knowledge_collections c ON c.id = selected.id AND c.deleted_at IS NULL
  JOIN users actor
    ON actor.id = $2
   AND actor.account_status = 'active'
   AND actor.deleted_at IS NULL
  LEFT JOIN team_memberships membership
    ON membership.team_id = c.team_id
   AND membership.user_id = $2
   AND membership.status = 'active'
  LEFT JOIN teams team ON team.id = c.team_id AND team.deleted_at IS NULL
  WHERE (c.scope = 'personal' AND c.owner_user_id = $2)
     OR (c.scope = 'team' AND membership.user_id IS NOT NULL AND team.id IS NOT NULL)
), active_documents AS (
  SELECT
    authorized.id AS collection_id,
    file.original_filename,
    document.updated_at,
    count(*) OVER (PARTITION BY authorized.id)::integer AS active_document_count,
    (
      CASE
        WHEN char_length(lower($3)) >= 2 AND (
          position(lower($3) IN lower(file.original_filename)) > 0 OR
          position(lower($3) IN lower(authorized.name)) > 0 OR
          position(lower($3) IN lower(authorized.description)) > 0
        ) THEN 8 ELSE 0
      END +
      COALESCE((
        SELECT sum(
          CASE
            WHEN position(term.value IN lower(file.original_filename)) > 0 THEN 2
            WHEN position(term.value IN lower(authorized.name)) > 0 OR
                 position(term.value IN lower(authorized.description)) > 0 THEN 1
            ELSE 0
          END
        )::integer
        FROM jsonb_array_elements_text($4::jsonb) AS term(value)
      ), 0)
    )::integer AS relevance_score,
    row_number() OVER (
      PARTITION BY authorized.id
      ORDER BY document.updated_at DESC, document.id DESC
    ) AS representative_rank
  FROM authorized
  JOIN knowledge_documents document
    ON document.collection_id = authorized.id
   AND document.status = 'active'
   AND document.deleted_at IS NULL
  JOIN knowledge_document_versions version
    ON version.id = document.current_version_id
   AND version.document_id = document.id
   AND version.status = 'active'
  JOIN files file
    ON file.id = version.file_id
   AND file.upload_status = 'available'
   AND file.deleted_at IS NULL
), ranked_documents AS (
  SELECT active_documents.*,
    row_number() OVER (
      PARTITION BY collection_id
      ORDER BY relevance_score DESC, updated_at DESC, original_filename
    ) AS relevance_rank
  FROM active_documents
), candidates AS (
  SELECT *
  FROM ranked_documents
  WHERE (relevance_score > 0 AND relevance_rank <= 5)
     OR representative_rank <= 3
)
SELECT
  authorized.id::text,
  authorized.name,
  authorized.description,
  COALESCE(candidates.active_document_count, 0),
  candidates.original_filename,
  COALESCE(candidates.relevance_score, 0),
  COALESCE(candidates.representative_rank <= 3, false),
  candidates.updated_at
FROM authorized
LEFT JOIN candidates ON candidates.collection_id = authorized.id
ORDER BY authorized.ordinality,
  CASE WHEN candidates.relevance_score > 0 THEN 0 ELSE 1 END,
  candidates.relevance_score DESC,
  candidates.updated_at DESC,
  candidates.original_filename
`, evidenceUUIDArrayLiteral(input.CollectionIDs), input.ActorUserID, input.QueryText, string(encodedTerms))
	if err != nil {
		return nil, fmt.Errorf("fetch routing catalog: %w", err)
	}
	defer rows.Close()

	collections := make([]RoutingCatalogCollection, 0, len(input.CollectionIDs))
	indexByID := make(map[string]int, len(input.CollectionIDs))
	for rows.Next() {
		var collectionID, name, description string
		var activeCount, score int
		var title sql.NullString
		var representative bool
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&collectionID,
			&name,
			&description,
			&activeCount,
			&title,
			&score,
			&representative,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan routing catalog: %w", err)
		}
		index, ok := indexByID[collectionID]
		if !ok {
			index = len(collections)
			indexByID[collectionID] = index
			collections = append(collections, RoutingCatalogCollection{
				ID:                  collectionID,
				Name:                name,
				Description:         description,
				ActiveDocumentCount: activeCount,
				Documents:           []RoutingCatalogDocument{},
			})
		}
		if !title.Valid || !updatedAt.Valid {
			continue
		}
		collections[index].Documents = append(
			collections[index].Documents,
			RoutingCatalogDocument{
				Title:          title.String,
				RelevanceScore: score,
				Representative: representative,
				UpdatedAt:      updatedAt.Time.UTC(),
			},
		)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing catalog: %w", err)
	}
	return collections, nil
}
