-- Migration: 000028_create_skills_table
-- Description: Add skills and skill_files tables for online skill management with CRUD API
DO $$ BEGIN RAISE NOTICE '[Migration 000028] Creating skills and skill_files tables'; END $$;

CREATE TABLE IF NOT EXISTS skills (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(1024) NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active',  -- pending_review, active, disabled
    created_by BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS skill_files (
    id BIGSERIAL PRIMARY KEY,
    skill_id BIGINT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(512) NOT NULL,
    content TEXT,
    is_script BOOLEAN DEFAULT FALSE,
    file_size BIGINT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(skill_id, file_name)
);

DO $$ BEGIN RAISE NOTICE '[Migration 000028] skills and skill_files tables created successfully'; END $$;
