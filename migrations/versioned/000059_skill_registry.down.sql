-- ============================================================================
-- Migration 000059: Drop Skill registry tables
-- ============================================================================

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Dropping Skill registry tables...'; END $$;

DROP TABLE IF EXISTS skill_execution_runs;
DROP TABLE IF EXISTS agent_skill_bindings;
DROP TABLE IF EXISTS tenant_skill_installs;
DROP TABLE IF EXISTS skills;

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Skill registry tables dropped'; END $$;
