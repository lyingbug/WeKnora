ALTER TABLE tenants DROP COLUMN memory_config;

DROP TABLE IF EXISTS memory_anchors;
DROP TABLE IF EXISTS memory_page_revisions;
DROP TABLE IF EXISTS memory_pages;
DROP TABLE IF EXISTS memory_notes;
DROP TABLE IF EXISTS memory_spaces;
