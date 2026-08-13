-- G20.1 Skill package supply chain and Store authority.
-- Candidate bytes are untrusted and never execute during ingestion. Tool,
-- Egress, and Secret declarations are metadata only and grant no authority.

CREATE TABLE skill_package_versions (
  package_fingerprint TEXT PRIMARY KEY,
  runtime_bundle_fingerprint TEXT,
  sbom_fingerprint TEXT NOT NULL,
  name TEXT NOT NULL,
  version TEXT NOT NULL,
  description TEXT NOT NULL,
  license TEXT NOT NULL DEFAULT '',
  compatibility TEXT NOT NULL DEFAULT '',
  allowed_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  capability_requests JSONB NOT NULL DEFAULT '[]'::jsonb,
  has_runtime BOOLEAN NOT NULL,
  file_count INTEGER NOT NULL,
  package_bytes BIGINT NOT NULL,
  expanded_bytes BIGINT NOT NULL,
  package_object_key TEXT NOT NULL,
  sbom_object_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT skill_package_fingerprint_check
    CHECK (package_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT skill_package_runtime_fingerprint_check CHECK (
    (has_runtime AND runtime_bundle_fingerprint ~ '^sha256:[0-9a-f]{64}$')
    OR (NOT has_runtime AND runtime_bundle_fingerprint IS NULL)
  ),
  CONSTRAINT skill_package_sbom_fingerprint_check
    CHECK (sbom_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT skill_package_name_check
    CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(name) <= 64),
  CONSTRAINT skill_package_version_not_blank CHECK (length(trim(version)) > 0),
  CONSTRAINT skill_package_description_not_blank CHECK (length(trim(description)) > 0),
  CONSTRAINT skill_package_metadata_arrays CHECK (
    jsonb_typeof(allowed_tools) = 'array'
    AND jsonb_typeof(capability_requests) = 'array'
  ),
  CONSTRAINT skill_package_file_count_check CHECK (file_count BETWEEN 1 AND 2048),
  CONSTRAINT skill_package_size_check CHECK (
    package_bytes BETWEEN 1 AND 134217728
    AND expanded_bytes BETWEEN 1 AND 134217728
  ),
  CONSTRAINT skill_package_object_key_check CHECK (
    package_object_key = 'skill-packages/sha256/'
      || substring(package_fingerprint FROM 8) || '.zip'
  ),
  CONSTRAINT skill_package_sbom_key_check CHECK (
    sbom_object_key = 'skill-sboms/sha256/'
      || substring(sbom_fingerprint FROM 8) || '.cdx.json'
  )
);

CREATE UNIQUE INDEX idx_skill_package_sbom_unique
  ON skill_package_versions(sbom_fingerprint);

CREATE TABLE skill_package_candidates (
  id UUID PRIMARY KEY,
  source_type TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  source_artifact_sha256 TEXT NOT NULL,
  source_object_key TEXT NOT NULL,
  package_fingerprint TEXT NOT NULL
    REFERENCES skill_package_versions(package_fingerprint) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'validated',
  admission_eligible BOOLEAN NOT NULL DEFAULT false,
  validation_summary TEXT NOT NULL DEFAULT 'validated_no_execute',
  reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  review_reason TEXT NOT NULL DEFAULT '',
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT skill_candidate_source_type_check
    CHECK (source_type IN ('official', 'lobehub', 'git', 'zip')),
  CONSTRAINT skill_candidate_source_ref_not_blank CHECK (length(trim(source_ref)) > 0),
  CONSTRAINT skill_candidate_source_hash_check
    CHECK (source_artifact_sha256 ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT skill_candidate_source_key_check CHECK (
    source_object_key = 'skill-quarantine/sha256/'
      || substring(source_artifact_sha256 FROM 8) || '.zip'
  ),
  CONSTRAINT skill_candidate_status_check
    CHECK (status IN ('validated', 'admitted', 'rejected')),
  CONSTRAINT skill_candidate_admission_check CHECK (
    status <> 'admitted' OR admission_eligible
  ),
  CONSTRAINT skill_candidate_review_actor_check CHECK (
    (status = 'validated' AND reviewed_by_user_id IS NULL)
    OR (status IN ('admitted', 'rejected') AND reviewed_by_user_id IS NOT NULL)
  ),
  CONSTRAINT skill_candidate_revision_positive CHECK (revision >= 1),
  CONSTRAINT skill_candidate_timestamps_order CHECK (updated_at >= created_at),
  UNIQUE (source_type, source_ref),
  UNIQUE (id, package_fingerprint)
);

CREATE INDEX idx_skill_candidate_store
  ON skill_package_candidates(status, updated_at DESC, id DESC);

CREATE TABLE skill_installations (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  admission_id UUID NOT NULL REFERENCES skill_package_candidates(id) ON DELETE RESTRICT,
  package_fingerprint TEXT NOT NULL
    REFERENCES skill_package_versions(package_fingerprint) ON DELETE RESTRICT,
  skill_name TEXT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT skill_installation_name_check
    CHECK (skill_name ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(skill_name) <= 64),
  CONSTRAINT skill_installation_revision_positive CHECK (revision >= 1),
  CONSTRAINT skill_installation_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT skill_installation_admission_package_fk
    FOREIGN KEY (admission_id, package_fingerprint)
    REFERENCES skill_package_candidates(id, package_fingerprint) ON DELETE RESTRICT,
  UNIQUE (user_id, skill_name),
  UNIQUE (user_id, admission_id)
);

CREATE INDEX idx_skill_installation_user_updated
  ON skill_installations(user_id, updated_at DESC, id DESC);

CREATE FUNCTION enforce_skill_installation_admission()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM skill_package_candidates candidate
    WHERE candidate.id = NEW.admission_id
      AND candidate.package_fingerprint = NEW.package_fingerprint
      AND candidate.status = 'admitted'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23514',
      MESSAGE = 'SKILL_INSTALLATION_ADMISSION_REQUIRED';
  END IF;
  RETURN NEW;
END
$$;

CREATE TRIGGER trg_skill_installation_admission
BEFORE INSERT ON skill_installations
FOR EACH ROW EXECUTE FUNCTION enforce_skill_installation_admission();

GRANT SELECT, INSERT ON TABLE skill_package_versions TO go_api_runtime;
GRANT SELECT, INSERT ON TABLE skill_package_candidates TO go_api_runtime;
GRANT UPDATE (status, reviewed_by_user_id, review_reason, revision, updated_at)
  ON TABLE skill_package_candidates TO go_api_runtime;
GRANT SELECT, INSERT, DELETE ON TABLE skill_installations TO go_api_runtime;
