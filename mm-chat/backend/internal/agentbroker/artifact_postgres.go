package agentbroker

import (
	"context"
	"database/sql"
)

// PostgresArtifactRepository is the function-only persistence boundary added
// by migration 091. Its database session may inherit agent_artifact_control;
// no direct agent_artifacts DML is used.
type PostgresArtifactRepository struct{ db *sql.DB }

func NewPostgresArtifactRepository(db *sql.DB) *PostgresArtifactRepository {
	return &PostgresArtifactRepository{db: db}
}

func (repository *PostgresArtifactRepository) AuthorizeArtifact(ctx context.Context, candidate ArtifactCandidate) error {
	if repository == nil || repository.db == nil || !validArtifactRef(candidate) || candidate.Size < 0 ||
		!validFingerprint(candidate.Fingerprint) {
		return ErrArtifactDenied
	}
	var authorized bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_artifact_authorize(
  $1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
)`, candidate.IntentID, candidate.UserID, candidate.RunID, candidate.AttemptID,
		candidate.Generation, candidate.SnapshotFingerprint, candidate.GrantFingerprint,
		candidate.RegistryFingerprint, candidate.Name, candidate.MediaType, candidate.Size,
		candidate.Fingerprint, artifactObjectKey(candidate)).Scan(&authorized)
	if err != nil {
		return mapPostgresError("authorize agent Artifact", err)
	}
	if !authorized {
		return ErrArtifactDenied
	}
	return nil
}

func (repository *PostgresArtifactRepository) AttachArtifact(ctx context.Context, candidate ArtifactCandidate, objectKey string) error {
	if repository == nil || repository.db == nil || !validArtifactRef(candidate) || candidate.Size < 0 ||
		!validFingerprint(candidate.Fingerprint) || objectKey != artifactObjectKey(candidate) {
		return ErrArtifactDenied
	}
	var artifactID string
	err := repository.db.QueryRowContext(ctx, `
SELECT id FROM agent_artifact_attach(
  $1,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)`, candidate.ArtifactID, candidate.IntentID, candidate.UserID, candidate.RunID,
		candidate.AttemptID, candidate.Generation, candidate.SnapshotFingerprint,
		candidate.GrantFingerprint, candidate.RegistryFingerprint, candidate.Name,
		candidate.MediaType, candidate.Size, candidate.Fingerprint, objectKey).Scan(&artifactID)
	if err != nil {
		return mapPostgresError("attach agent Artifact", err)
	}
	if artifactID != candidate.ArtifactID {
		return ErrArtifactDenied
	}
	return nil
}
