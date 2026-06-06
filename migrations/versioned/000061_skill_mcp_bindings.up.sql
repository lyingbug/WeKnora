DO $$ BEGIN RAISE NOTICE '[Migration 000061] Creating tenant_skill_mcp_bindings...'; END $$;

CREATE TABLE IF NOT EXISTS tenant_skill_mcp_bindings (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    skill_id VARCHAR(64) NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    mcp_name VARCHAR(128) NOT NULL,
    service_id VARCHAR(36) NOT NULL REFERENCES mcp_services(id) ON DELETE CASCADE,
    updated_by VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_mcp_bindings_tenant_skill_name
    ON tenant_skill_mcp_bindings(tenant_id, skill_id, mcp_name);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_mcp_bindings_tenant
    ON tenant_skill_mcp_bindings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_mcp_bindings_skill
    ON tenant_skill_mcp_bindings(skill_id);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_mcp_bindings_service
    ON tenant_skill_mcp_bindings(service_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000061] tenant_skill_mcp_bindings ready'; END $$;
