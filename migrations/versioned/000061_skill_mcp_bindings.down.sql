DO $$ BEGIN RAISE NOTICE '[Migration 000061 DOWN] Dropping tenant_skill_mcp_bindings...'; END $$;

DROP INDEX IF EXISTS idx_tenant_skill_mcp_bindings_service;
DROP INDEX IF EXISTS idx_tenant_skill_mcp_bindings_skill;
DROP INDEX IF EXISTS idx_tenant_skill_mcp_bindings_tenant;
DROP INDEX IF EXISTS idx_tenant_skill_mcp_bindings_tenant_skill_name;
DROP TABLE IF EXISTS tenant_skill_mcp_bindings;
