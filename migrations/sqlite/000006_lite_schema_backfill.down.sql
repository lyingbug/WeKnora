DROP TABLE IF EXISTS knowledge_processing_spans;
DROP TABLE IF EXISTS knowledge_tag_relations;
DROP INDEX IF EXISTS idx_mcp_oauth_tokens_principal;

ALTER TABLE mcp_oauth_tokens DROP COLUMN principal_id;
ALTER TABLE mcp_oauth_tokens DROP COLUMN principal_type;
ALTER TABLE tenant_invitations DROP COLUMN accepted_count;
ALTER TABLE tenant_invitations DROP COLUMN token;
ALTER TABLE messages DROP COLUMN attachments;
ALTER TABLE knowledges DROP COLUMN pending_subtasks_count;

DROP TABLE IF EXISTS system_settings;

ALTER TABLE users DROP COLUMN is_system_admin;
ALTER TABLE tenants DROP COLUMN api_principal_config;
