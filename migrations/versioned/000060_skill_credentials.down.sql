-- ============================================================================
-- Migration 000060: Drop tenant Skill credentials
-- ============================================================================

DO $$ BEGIN RAISE NOTICE '[Migration 000060] Dropping tenant Skill credentials table...'; END $$;

DROP TABLE IF EXISTS tenant_skill_credentials;

DO $$ BEGIN RAISE NOTICE '[Migration 000060] Tenant Skill credentials table dropped'; END $$;
