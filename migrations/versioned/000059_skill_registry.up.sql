-- ============================================================================
-- Migration 000059: Add Skill registry tables
-- ============================================================================

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Creating Skill registry tables...'; END $$;

CREATE TABLE IF NOT EXISTS skills (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    version VARCHAR(64) NOT NULL DEFAULT '0.0.0',
    description TEXT NOT NULL DEFAULT '',
    source_type VARCHAR(32) NOT NULL DEFAULT 'preloaded',
    source_uri TEXT NOT NULL DEFAULT '',
    digest VARCHAR(128) NOT NULL DEFAULT '',
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    is_builtin BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_version ON skills(name, version);
CREATE INDEX IF NOT EXISTS idx_skills_source_type ON skills(source_type);
CREATE INDEX IF NOT EXISTS idx_skills_status ON skills(status);
CREATE INDEX IF NOT EXISTS idx_skills_is_builtin ON skills(is_builtin);

CREATE TABLE IF NOT EXISTS tenant_skill_installs (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    skill_id VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    installed_by VARCHAR(64) NOT NULL DEFAULT '',
    approved_permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_tenant_skill_installs_skill FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_installs_tenant_skill ON tenant_skill_installs(tenant_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_tenant_skill_installs_tenant ON tenant_skill_installs(tenant_id);

CREATE TABLE IF NOT EXISTS agent_skill_bindings (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    agent_id VARCHAR(64) NOT NULL,
    skill_id VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_agent_skill_bindings_skill FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_skill_bindings_agent_skill ON agent_skill_bindings(agent_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_agent_skill_bindings_tenant_agent ON agent_skill_bindings(tenant_id, agent_id);

CREATE TABLE IF NOT EXISTS skill_execution_runs (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    user_id VARCHAR(64) NOT NULL DEFAULT '',
    agent_id VARCHAR(64) NOT NULL DEFAULT '',
    session_id VARCHAR(64) NOT NULL DEFAULT '',
    message_id VARCHAR(64) NOT NULL DEFAULT '',
    tool_call_id VARCHAR(128) NOT NULL DEFAULT '',
    skill_id VARCHAR(64) NOT NULL,
    script_path TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'started',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    resource_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_skill_execution_runs_skill FOREIGN KEY(skill_id) REFERENCES skills(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_skill_execution_runs_tenant_created ON skill_execution_runs(tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_skill_execution_runs_session ON skill_execution_runs(session_id);
CREATE INDEX IF NOT EXISTS idx_skill_execution_runs_skill ON skill_execution_runs(skill_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Skill registry tables created'; END $$;
