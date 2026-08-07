-- Rollback: 000081_memory_core

DO $$ BEGIN RAISE NOTICE '[Migration 000081] Rolling back memory core schema'; END $$;

DROP TABLE IF EXISTS memory_page_revisions;
DROP TABLE IF EXISTS memory_pages;
DROP TABLE IF EXISTS memory_notes;
DROP TABLE IF EXISTS memory_spaces;

DO $$ BEGIN RAISE NOTICE '[Migration 000081] memory core schema rolled back'; END $$;
