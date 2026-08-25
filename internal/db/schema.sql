-- GRC Portal schema (PostgreSQL). Idempotent: safe to run at every startup.
-- ponytail: CREATE TABLE IF NOT EXISTS at boot; add a schema_migrations table + numbered .sql when ALTERs are needed.

CREATE TABLE IF NOT EXISTS portal_users (
  id            BIGSERIAL PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name  TEXT,
  role          TEXT NOT NULL DEFAULT 'reviewer' CHECK (role IN ('admin','reviewer','readonly')),
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
  id             TEXT PRIMARY KEY,
  portal_user_id BIGINT NOT NULL REFERENCES portal_users(id) ON DELETE CASCADE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at     TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS applications (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  description TEXT,
  owner       TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Departemen pemilik aplikasi (keuangan/marketing/hr); nullable. Dipakai untuk
-- grouping audit & akses per departemen. Divalidasi di handler, bukan di DB.
ALTER TABLE applications ADD COLUMN IF NOT EXISTS department TEXT;

CREATE TABLE IF NOT EXISTS roles (
  id             BIGSERIAL PRIMARY KEY,
  application_id BIGINT REFERENCES applications(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  description    TEXT,
  UNIQUE (application_id, name)
);

CREATE TABLE IF NOT EXISTS subject_identities (
  id           BIGSERIAL PRIMARY KEY,
  email        TEXT,
  username     TEXT,
  display_name TEXT,
  type         TEXT NOT NULL DEFAULT 'user' CHECK (type IN ('user','service_account')),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_subject_email    ON subject_identities(lower(email))    WHERE email IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_subject_username ON subject_identities(lower(username)) WHERE email IS NULL;

CREATE TABLE IF NOT EXISTS access_bindings (
  id             BIGSERIAL PRIMARY KEY,
  application_id BIGINT NOT NULL REFERENCES applications(id)       ON DELETE CASCADE,
  subject_id     BIGINT NOT NULL REFERENCES subject_identities(id) ON DELETE CASCADE,
  role_id        BIGINT NOT NULL REFERENCES roles(id)             ON DELETE CASCADE,
  source         TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','csv')),
  status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
  granted_at     TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (application_id, subject_id, role_id)
);

CREATE TABLE IF NOT EXISTS review_campaigns (
  id           BIGSERIAL PRIMARY KEY,
  name         TEXT NOT NULL,
  period_start TEXT,
  period_end   TEXT,
  status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed')),
  created_by   BIGINT REFERENCES portal_users(id),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at    TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS campaign_applications (
  campaign_id    BIGINT NOT NULL REFERENCES review_campaigns(id) ON DELETE CASCADE,
  application_id BIGINT NOT NULL REFERENCES applications(id)     ON DELETE CASCADE,
  PRIMARY KEY (campaign_id, application_id)
);

CREATE TABLE IF NOT EXISTS review_items (
  id            BIGSERIAL PRIMARY KEY,
  campaign_id   BIGINT NOT NULL REFERENCES review_campaigns(id) ON DELETE CASCADE,
  binding_id    BIGINT REFERENCES access_bindings(id) ON DELETE SET NULL,
  snap_subject  TEXT NOT NULL,
  snap_app      TEXT NOT NULL,
  snap_role     TEXT NOT NULL,
  reviewer_id   BIGINT REFERENCES portal_users(id),
  decision      TEXT NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','certified','revoked')),
  justification TEXT,
  decided_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (campaign_id, binding_id)
);

CREATE TABLE IF NOT EXISTS audit_log (
  id          BIGSERIAL PRIMARY KEY,
  actor_id    BIGINT REFERENCES portal_users(id),
  action      TEXT NOT NULL,
  entity_type TEXT,
  entity_id   BIGINT,
  detail      TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Audit: index waktu untuk filter rentang tanggal (menjaga query cepat saat data tumbuh).
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_log (created_at DESC);

-- Modul SOP & Kebijakan (+ evidence): dokumen internal untuk pencarian + RAG (jawaban AI).
-- Retrieval pakai full-text search bawaan Postgres (config 'simple', cocok utk bahasa Indonesia
-- tanpa stemming keliru); tak butuh embedding/vector. `search` di-generate + GIN index.
CREATE TABLE IF NOT EXISTS documents (
  id         BIGSERIAL PRIMARY KEY,
  title      TEXT NOT NULL,
  category   TEXT NOT NULL DEFAULT 'sop' CHECK (category IN ('sop','kebijakan','evidence')),
  source     TEXT,
  body       TEXT NOT NULL,
  created_by BIGINT REFERENCES portal_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  search     tsvector GENERATED ALWAYS AS
             (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(body,''))) STORED
);
CREATE INDEX IF NOT EXISTS idx_documents_search ON documents USING GIN(search);

-- Modul Evidence Management (UC-01): request evidence berulang dari auditor/assessor.
-- Sistem menyarankan evidence lama yang cocok (reuse, via FTS di atas `documents`
-- kategori 'evidence') + mendeteksi request serupa (dedup, via `search` di sini);
-- manusia yang memutuskan: fulfill (tautkan dokumen = jejak sumber) atau gap (arahkan
-- ke owner). `document_id` = jejak sumber tunggal.
-- ponytail: satu evidence per request; kalau butuh banyak, tambah tabel join evidence_request_documents.
CREATE TABLE IF NOT EXISTS evidence_requests (
  id          BIGSERIAL PRIMARY KEY,
  title       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  requester   TEXT NOT NULL DEFAULT 'auditor'
              CHECK (requester IN ('auditor','pihak_ketiga','internal_audit','assessor')),
  status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','fulfilled','gap')),
  document_id BIGINT REFERENCES documents(id) ON DELETE SET NULL,
  owner       TEXT,   -- diarahkan ke siapa saat gap (mis. Infrastructure / Developer)
  note        TEXT,   -- keterangan saat fulfill/gap
  created_by  BIGINT REFERENCES portal_users(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  search      tsvector GENERATED ALWAYS AS
              (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,''))) STORED
);
CREATE INDEX IF NOT EXISTS idx_evidence_requests_search ON evidence_requests USING GIN(search);

-- Direktori HR (UC-02): sumber kebenaran status karyawan aktif/nonaktif. Access
-- review mencocokkan subject akses ke sini (by email, fallback username) untuk
-- menandai akun nonaktif yang masih punya akses / akun yang tak ada di HR (mismatch).
CREATE TABLE IF NOT EXISTS hr_employees (
  id           BIGSERIAL PRIMARY KEY,
  email        TEXT,
  username     TEXT,
  display_name TEXT,
  department   TEXT,
  status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_hr_email    ON hr_employees(lower(email))    WHERE email IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_hr_username ON hr_employees(lower(username)) WHERE email IS NULL;
