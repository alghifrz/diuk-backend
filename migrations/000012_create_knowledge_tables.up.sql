-- JOB 14: tenant knowledge documents, pgvector chunks, RBAC, and ingest jobs.

CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN ('AI_GENERATE', 'MESSAGE_DELIVERY', 'KNOWLEDGE_INGEST'));

CREATE TABLE knowledge_documents (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id       UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    title             TEXT         NOT NULL CHECK (btrim(title) <> ''),
    description       TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    source_type       TEXT         NOT NULL CHECK (source_type IN ('TEXT', 'PDF', 'DOCX')),
    source_url        TEXT         CHECK (source_url IS NULL OR btrim(source_url) <> ''),
    storage_path      TEXT         CHECK (storage_path IS NULL OR btrim(storage_path) <> ''),
    mime_type         TEXT         CHECK (mime_type IS NULL OR btrim(mime_type) <> ''),
    status            TEXT         NOT NULL DEFAULT 'PENDING'
                                   CHECK (status IN ('PENDING', 'PROCESSING', 'READY', 'FAILED', 'ARCHIVED')),
    version           INTEGER      NOT NULL DEFAULT 1 CHECK (version >= 1),
    processing_error  TEXT,
    created_by        UUID         REFERENCES users (id) ON DELETE RESTRICT,
    updated_by        UUID         REFERENCES users (id) ON DELETE RESTRICT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_knowledge_documents_id_business UNIQUE (id, business_id)
);

CREATE INDEX idx_knowledge_documents_business_id_status
    ON knowledge_documents (business_id, status, updated_at DESC);

CREATE TRIGGER trg_knowledge_documents_updated_at
    BEFORE UPDATE ON knowledge_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Embedding dimension matches the default model text-embedding-3-small (1536).
-- Application config AI_EMBEDDING_DIMENSIONS must stay aligned with this column.
CREATE TABLE knowledge_chunks (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id   UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    document_id   UUID         NOT NULL,
    chunk_index   INTEGER      NOT NULL CHECK (chunk_index >= 0),
    content       TEXT         NOT NULL CHECK (btrim(content) <> ''),
    embedding     VECTOR(1536) NOT NULL,
    token_count   INTEGER      CHECK (token_count IS NULL OR token_count >= 0),
    metadata      JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_knowledge_chunks_document_index UNIQUE (document_id, chunk_index),
    CONSTRAINT fk_knowledge_chunks_document_business
        FOREIGN KEY (document_id, business_id)
        REFERENCES knowledge_documents (id, business_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_knowledge_chunks_business_id_document_id
    ON knowledge_chunks (business_id, document_id);

-- Cosine HNSW is valid for vector(1536) and does not require a lists estimate.
CREATE INDEX idx_knowledge_chunks_embedding
    ON knowledge_chunks
    USING hnsw (embedding vector_cosine_ops);

INSERT INTO permissions (code, resource, action, description) VALUES
    ('knowledge.read',   'knowledge', 'read',   'Read knowledge documents'),
    ('knowledge.create', 'knowledge', 'create', 'Create knowledge documents'),
    ('knowledge.update', 'knowledge', 'update', 'Update or reprocess knowledge documents'),
    ('knowledge.delete', 'knowledge', 'delete', 'Delete knowledge documents')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'knowledge.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('knowledge.read', 'knowledge.create', 'knowledge.update', 'knowledge.delete')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'knowledge.read'
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
