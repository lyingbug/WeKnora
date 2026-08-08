-- Migration: 000082_memory_anchors
-- Description: The bridge between a person's memory and the shared knowledge
--              base. Each row says "this memory relates to that wiki page, in
--              this way, this often, most recently then".
--
-- This one table is what makes the knowledge graph personal: rolled up per
-- user it produces the illumination overlay (unlit → touched → familiar →
-- mastered, plus flagged) and the mastery coverage metric; rolled up across
-- users and anonymised it tells knowledge-base owners which pages are asked
-- about but thin, and which have been contested.
--
-- Heat is deliberately NOT computed in SQL. The scoring needs power() and
-- log(), which would work on PostgreSQL and fail on Lite's SQLite, so the
-- repository only ever runs the portable projection query backed by
-- idx_memory_anchors_space_kb and the arithmetic happens in Go.

DO $$ BEGIN RAISE NOTICE '[Migration 000082] Applying memory anchors schema'; END $$;

CREATE TABLE IF NOT EXISTS memory_anchors (
    id                VARCHAR(36)  PRIMARY KEY,
    tenant_id         BIGINT       NOT NULL,
    space_id          VARCHAR(36)  NOT NULL,
    memory_page_id    VARCHAR(36)  NOT NULL DEFAULT '',
    knowledge_base_id VARCHAR(36)  NOT NULL,
    target_kind       VARCHAR(24)  NOT NULL,
    target_ref        VARCHAR(512) NOT NULL,
    relation          VARCHAR(24)  NOT NULL,
    strength          REAL         NOT NULL DEFAULT 0,
    hit_count         INT          NOT NULL DEFAULT 0,
    confidence        REAL         NOT NULL DEFAULT 0.5,
    source            VARCHAR(16)  NOT NULL DEFAULT 'pipeline',
    evidence          JSONB        NOT NULL DEFAULT '{}'::JSONB,
    first_seen_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

COMMENT ON TABLE memory_anchors IS 'Links a memory page to a knowledge-base target. Aggregating these per user lights up the wiki graph; aggregating them anonymously across users exposes knowledge gaps.';
COMMENT ON COLUMN memory_anchors.relation IS 'mentioned | asked_about | bookmarked | disagreed | learned | corrected | owns. Weights are configurable settings, not constants.';
COMMENT ON COLUMN memory_anchors.evidence IS 'Message ids, session ids and queries that justify this anchor, capped per list so a hot anchor cannot grow without bound.';

-- Reinforcement is an upsert on this key: the same relation from the same
-- memory to the same target accumulates hit_count instead of duplicating.
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_anchors_unique
    ON memory_anchors (space_id, knowledge_base_id, target_kind, target_ref, relation, memory_page_id)
    WHERE deleted_at IS NULL;

-- Overlay read path: pull one person's anchors for one knowledge base.
CREATE INDEX IF NOT EXISTS idx_memory_anchors_space_kb
    ON memory_anchors (space_id, knowledge_base_id, target_kind);

-- Insights read path: aggregate one target across every space.
CREATE INDEX IF NOT EXISTS idx_memory_anchors_kb_target
    ON memory_anchors (knowledge_base_id, target_kind, target_ref);

CREATE INDEX IF NOT EXISTS idx_memory_anchors_page
    ON memory_anchors (space_id, memory_page_id);

CREATE INDEX IF NOT EXISTS idx_memory_anchors_tenant
    ON memory_anchors (tenant_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000082] memory anchors schema applied successfully'; END $$;
