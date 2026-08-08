-- Migration: 000083_memory_tenant_config
-- Description: Workspace-level memory settings.
--
-- Stored as a sparse patch rather than a full settings struct so "unset" stays
-- distinguishable from "set to the default". Without that distinction a
-- workspace could never tell whether a user had deliberately opted out of
-- something or simply never touched it.
--
-- The user layer needs no DDL: it lives in the existing users.preferences JSON.

DO $$ BEGIN RAISE NOTICE '[Migration 000083] Adding tenants.memory_config'; END $$;

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS memory_config JSONB;

COMMENT ON COLUMN tenants.memory_config IS 'Sparse workspace-level memory settings patch. Keys are validated against the descriptor catalogue in internal/types/memory_settings.go.';

DO $$ BEGIN RAISE NOTICE '[Migration 000083] tenants.memory_config added'; END $$;
