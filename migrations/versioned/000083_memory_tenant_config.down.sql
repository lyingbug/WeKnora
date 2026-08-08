-- Rollback: 000083_memory_tenant_config

DO $$ BEGIN RAISE NOTICE '[Migration 000083] Dropping tenants.memory_config'; END $$;

ALTER TABLE tenants DROP COLUMN IF EXISTS memory_config;

DO $$ BEGIN RAISE NOTICE '[Migration 000083] tenants.memory_config dropped'; END $$;
