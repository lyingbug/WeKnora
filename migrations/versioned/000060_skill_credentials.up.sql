-- ============================================================================
-- Migration 000060: Add tenant Skill credentials
-- ============================================================================

DO $$ BEGIN RAISE NOTICE '[Migration 000060] Creating tenant Skill credentials table...'; END $$;

CREATE TABLE IF NOT EXISTS tenant_skill_credentials (
    id VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    tenant_id INTEGER NOT NULL,
    skill_id VARCHAR(64) NOT NULL,
    credentials JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_by VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_tenant_skill_credentials_skill FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_credentials_tenant_skill ON tenant_skill_credentials(tenant_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_credentials_tenant ON tenant_skill_credentials(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_credentials_skill ON tenant_skill_credentials(skill_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000060] Tenant Skill credentials table created'; END $$;
