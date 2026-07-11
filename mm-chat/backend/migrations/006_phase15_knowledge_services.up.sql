ALTER TABLE knowledge_collections
  ADD COLUMN description TEXT NOT NULL DEFAULT '',
  ADD COLUMN icon TEXT NOT NULL DEFAULT 'Folder',
  ADD COLUMN color TEXT NOT NULL DEFAULT 'blue',
  ADD COLUMN created_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
  ADD COLUMN idempotency_key TEXT,
  ADD COLUMN create_request_hash TEXT;

ALTER TABLE knowledge_collections
  ADD CONSTRAINT knowledge_collections_name_bounded_check CHECK (
    octet_length(name) BETWEEN 1 AND 256
  ),
  ADD CONSTRAINT knowledge_collections_description_bounded_check CHECK (
    octet_length(description) <= 8192
  ),
  ADD CONSTRAINT knowledge_collections_icon_check CHECK (
    icon IN (
      'Folder',
      'Atom',
      'BookText',
      'Microscope',
      'Cat',
      'ChartLine',
      'ChessKnight',
      'CodeXml',
      'Coffee',
      'GraduationCap',
      'MessagesSquare',
      'Archive'
    )
  ),
  ADD CONSTRAINT knowledge_collections_color_check CHECK (
    color IN ('blue', 'purple', 'green', 'orange', 'red', 'pink', 'cyan', 'gray')
  ),
  ADD CONSTRAINT knowledge_collections_idempotency_shape_check CHECK (
    (
      idempotency_key IS NULL
      AND create_request_hash IS NULL
    )
    OR (
      idempotency_key IS NOT NULL
      AND created_by_user_id IS NOT NULL
      AND octet_length(idempotency_key) BETWEEN 1 AND 128
      AND length(trim(idempotency_key)) > 0
      AND create_request_hash ~ '^[0-9a-f]{64}$'
    )
  );

CREATE UNIQUE INDEX idx_knowledge_collections_creator_idempotency
  ON knowledge_collections(created_by_user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_knowledge_collections_personal_page
  ON knowledge_collections(owner_user_id, created_at DESC, id DESC)
  WHERE scope = 'personal' AND deleted_at IS NULL;
CREATE INDEX idx_knowledge_collections_team_page
  ON knowledge_collections(team_id, created_at DESC, id DESC)
  WHERE scope = 'team' AND deleted_at IS NULL;

ALTER TABLE knowledge_documents
  ADD COLUMN visibility_epoch BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN created_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
  ADD COLUMN idempotency_key TEXT,
  ADD COLUMN create_request_hash TEXT;

ALTER TABLE knowledge_documents
  ADD CONSTRAINT knowledge_documents_visibility_epoch_positive CHECK (
    visibility_epoch >= 1
  ),
  ADD CONSTRAINT knowledge_documents_idempotency_shape_check CHECK (
    (
      idempotency_key IS NULL
      AND create_request_hash IS NULL
    )
    OR (
      idempotency_key IS NOT NULL
      AND created_by_user_id IS NOT NULL
      AND octet_length(idempotency_key) BETWEEN 1 AND 128
      AND length(trim(idempotency_key)) > 0
      AND create_request_hash ~ '^[0-9a-f]{64}$'
    )
  ),
  ADD CONSTRAINT knowledge_documents_collection_id_unique
    UNIQUE (collection_id, id);

CREATE UNIQUE INDEX idx_knowledge_documents_collection_creator_idempotency
  ON knowledge_documents(collection_id, created_by_user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_knowledge_documents_collection_page
  ON knowledge_documents(collection_id, created_at DESC, id DESC)
  WHERE deleted_at IS NULL;

ALTER TABLE knowledge_document_versions
  ADD COLUMN created_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
  ADD COLUMN idempotency_key TEXT,
  ADD COLUMN request_hash TEXT,
  ADD COLUMN error_code TEXT;

ALTER TABLE knowledge_document_versions
  ADD CONSTRAINT knowledge_document_versions_idempotency_shape_check CHECK (
    (
      idempotency_key IS NULL
      AND request_hash IS NULL
    )
    OR (
      idempotency_key IS NOT NULL
      AND created_by_user_id IS NOT NULL
      AND octet_length(idempotency_key) BETWEEN 1 AND 128
      AND length(trim(idempotency_key)) > 0
      AND request_hash ~ '^[0-9a-f]{64}$'
    )
  ),
  ADD CONSTRAINT knowledge_document_versions_error_code_check CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  ADD CONSTRAINT knowledge_document_versions_document_id_file_unique
    UNIQUE (document_id, id, file_id);

CREATE UNIQUE INDEX idx_knowledge_document_versions_document_creator_idempotency
  ON knowledge_document_versions(document_id, created_by_user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX idx_knowledge_document_versions_one_nonterminal
  ON knowledge_document_versions(document_id)
  WHERE status IN ('uploaded', 'processing');

ALTER TABLE processing_consents
  ADD CONSTRAINT processing_consents_collection_job_binding_unique UNIQUE (
    collection_id,
    id,
    processor,
    endpoint_id,
    governance_profile_id,
    governance_revision,
    governance_head_revision,
    consent_revision
  );

CREATE INDEX idx_processing_consents_current_lookup
  ON processing_consents(
    scope,
    processor,
    collection_id,
    user_id,
    decision,
    expires_at
  )
  WHERE superseded_at IS NULL;

CREATE TABLE knowledge_processing_jobs (
  id UUID PRIMARY KEY,
  collection_id UUID NOT NULL
    REFERENCES knowledge_collections(id) ON DELETE RESTRICT,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
  stage TEXT NOT NULL,
  operation TEXT NOT NULL,
  processor TEXT,
  endpoint_id TEXT,
  governance_profile_id UUID,
  governance_revision BIGINT,
  governance_head_revision BIGINT,
  collection_consent_id UUID,
  collection_consent_revision BIGINT,
  collection_acl_revision BIGINT NOT NULL,
  collection_visibility_epoch BIGINT NOT NULL,
  collection_processing_revision BIGINT NOT NULL,
  document_visibility_epoch BIGINT NOT NULL,
  requested_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
  caused_by_job_id UUID,
  idempotency_scope TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT knowledge_processing_jobs_document_version_fk
    FOREIGN KEY (document_id, document_version_id, file_id)
    REFERENCES knowledge_document_versions(document_id, id, file_id)
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_collection_document_fk
    FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id)
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_governance_profile_fk
    FOREIGN KEY (
      processor,
      endpoint_id,
      governance_profile_id,
      governance_revision
    )
    REFERENCES processor_governance_profiles(
      processor,
      endpoint_id,
      id,
      governance_revision
    )
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_governance_head_fk
    FOREIGN KEY (processor, endpoint_id)
    REFERENCES processor_governance_heads(processor, endpoint_id)
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_collection_consent_fk
    FOREIGN KEY (
      collection_id,
      collection_consent_id,
      processor,
      endpoint_id,
      governance_profile_id,
      governance_revision,
      governance_head_revision,
      collection_consent_revision
    )
    REFERENCES processing_consents(
      collection_id,
      id,
      processor,
      endpoint_id,
      governance_profile_id,
      governance_revision,
      governance_head_revision,
      consent_revision
    )
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_document_version_id_unique
    UNIQUE (document_id, document_version_id, id),
  CONSTRAINT knowledge_processing_jobs_caused_by_fk
    FOREIGN KEY (document_id, document_version_id, caused_by_job_id)
    REFERENCES knowledge_processing_jobs(document_id, document_version_id, id)
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_processing_jobs_stage_check
    CHECK (stage IN ('parse', 'passage_embedding', 'purge')),
  CONSTRAINT knowledge_processing_jobs_operation_check
    CHECK (operation IN ('initial', 'replace', 'reprocess', 'purge')),
  CONSTRAINT knowledge_processing_jobs_authority_shape_check CHECK (
    (
      stage IN ('parse', 'passage_embedding')
      AND operation IN ('initial', 'replace', 'reprocess')
      AND processor IS NOT NULL
      AND endpoint_id IS NOT NULL
      AND governance_profile_id IS NOT NULL
      AND governance_revision IS NOT NULL
      AND governance_head_revision IS NOT NULL
      AND collection_consent_id IS NOT NULL
      AND collection_consent_revision IS NOT NULL
    )
    OR (
      stage = 'purge'
      AND operation = 'purge'
      AND processor IS NULL
      AND endpoint_id IS NULL
      AND governance_profile_id IS NULL
      AND governance_revision IS NULL
      AND governance_head_revision IS NULL
      AND collection_consent_id IS NULL
      AND collection_consent_revision IS NULL
    )
  ),
  CONSTRAINT knowledge_processing_jobs_processor_not_blank CHECK (
    processor IS NULL OR length(trim(processor)) > 0
  ),
  CONSTRAINT knowledge_processing_jobs_endpoint_not_blank CHECK (
    endpoint_id IS NULL OR length(trim(endpoint_id)) > 0
  ),
  CONSTRAINT knowledge_processing_jobs_revision_positive CHECK (
    (governance_revision IS NULL OR governance_revision >= 1)
    AND (governance_head_revision IS NULL OR governance_head_revision >= 1)
    AND (collection_consent_revision IS NULL OR collection_consent_revision >= 1)
    AND collection_acl_revision >= 1
    AND collection_visibility_epoch >= 1
    AND collection_processing_revision >= 1
    AND document_visibility_epoch >= 1
  ),
  CONSTRAINT knowledge_processing_jobs_idempotency_check CHECK (
    octet_length(idempotency_scope) BETWEEN 1 AND 256
    AND length(trim(idempotency_scope)) > 0
    AND octet_length(idempotency_key) BETWEEN 1 AND 128
    AND length(trim(idempotency_key)) > 0
    AND request_hash ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT knowledge_processing_jobs_status_check CHECK (
    status IN ('pending', 'processing', 'succeeded', 'failed', 'cancelled')
  ),
  CONSTRAINT knowledge_processing_jobs_attempts_check CHECK (
    attempt_count >= 0
    AND max_attempts BETWEEN 1 AND 32
    AND attempt_count <= max_attempts
  ),
  CONSTRAINT knowledge_processing_jobs_state_shape_check CHECK (
    (
      status = 'pending'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count < max_attempts
    )
    OR (
      status = 'processing'
      AND lease_owner IS NOT NULL
      AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'succeeded'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'failed'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NOT NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'cancelled'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
    )
  ),
  CONSTRAINT knowledge_processing_jobs_error_code_check CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT knowledge_processing_jobs_available_after_created CHECK (
    available_at >= created_at
  ),
  CONSTRAINT knowledge_processing_jobs_lease_after_created CHECK (
    lease_expires_at IS NULL OR lease_expires_at >= created_at
  ),
  CONSTRAINT knowledge_processing_jobs_completed_after_created CHECK (
    completed_at IS NULL OR completed_at >= created_at
  ),
  CONSTRAINT knowledge_processing_jobs_timestamps_order CHECK (
    updated_at >= created_at
  ),
  CONSTRAINT knowledge_processing_jobs_idempotency_unique
    UNIQUE (idempotency_scope, idempotency_key)
);

CREATE INDEX idx_knowledge_processing_jobs_pending
  ON knowledge_processing_jobs(available_at, id)
  WHERE status = 'pending';
CREATE INDEX idx_knowledge_processing_jobs_processing
  ON knowledge_processing_jobs(lease_expires_at, id)
  WHERE status = 'processing';
CREATE INDEX idx_knowledge_processing_jobs_document
  ON knowledge_processing_jobs(document_id, created_at DESC, id DESC);
CREATE INDEX idx_knowledge_processing_jobs_version
  ON knowledge_processing_jobs(document_version_id, stage, created_at DESC);
