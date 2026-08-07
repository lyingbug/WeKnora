-- Rollback: 000082_memory_anchors

DO $$ BEGIN RAISE NOTICE '[Migration 000082] Rolling back memory anchors schema'; END $$;

DROP TABLE IF EXISTS memory_anchors;

DO $$ BEGIN RAISE NOTICE '[Migration 000082] memory anchors schema rolled back'; END $$;
