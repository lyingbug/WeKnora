package types

import "time"

const (
	SkillSourceTypePreloaded = "preloaded"
	SkillSourceTypeLocal     = "local"
	SkillStatusActive        = "active"
	SkillStatusDisabled      = "disabled"
	DefaultSkillVersion      = "0.0.0"
)

type SkillRegistryEntry struct {
	ID          string    `gorm:"column:id;primaryKey" json:"id"`
	Name        string    `gorm:"column:name" json:"name"`
	Version     string    `gorm:"column:version" json:"version"`
	Description string    `gorm:"column:description" json:"description"`
	SourceType  string    `gorm:"column:source_type" json:"source_type"`
	SourceURI   string    `gorm:"column:source_uri" json:"source_uri"`
	Digest      string    `gorm:"column:digest" json:"digest"`
	Manifest    JSON      `gorm:"column:manifest;type:jsonb;default:'{}'" json:"manifest"`
	Status      string    `gorm:"column:status" json:"status"`
	IsBuiltin   bool      `gorm:"column:is_builtin" json:"is_builtin"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SkillRegistryEntry) TableName() string {
	return "skills"
}

type LocalSkillPackagePreview struct {
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Description          string `json:"description"`
	SourceType           string `json:"source_type"`
	SourceURI            string `json:"source_uri"`
	Digest               string `json:"digest"`
	Manifest             JSON   `json:"manifest"`
	RequestedPermissions JSON   `json:"requested_permissions"`
}

type TenantSkillInstall struct {
	ID                  string    `gorm:"column:id;primaryKey" json:"id"`
	TenantID            uint64    `gorm:"column:tenant_id" json:"tenant_id"`
	SkillID             string    `gorm:"column:skill_id" json:"skill_id"`
	Enabled             bool      `gorm:"column:enabled" json:"enabled"`
	InstalledBy         string    `gorm:"column:installed_by" json:"installed_by"`
	ApprovedPermissions JSON      `gorm:"column:approved_permissions;type:jsonb;default:'{}'" json:"approved_permissions"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TenantSkillInstall) TableName() string {
	return "tenant_skill_installs"
}

type TenantSkillInstallInfo struct {
	InstallID           string    `json:"install_id"`
	TenantID            uint64    `json:"tenant_id"`
	SkillID             string    `json:"skill_id"`
	Enabled             bool      `json:"enabled"`
	InstalledBy         string    `json:"installed_by"`
	ApprovedPermissions JSON      `json:"approved_permissions"`
	InstalledAt         time.Time `json:"installed_at"`
	InstallUpdatedAt    time.Time `json:"install_updated_at"`
	Name                string    `json:"name"`
	Version             string    `json:"version"`
	Description         string    `json:"description"`
	SourceType          string    `json:"source_type"`
	SourceURI           string    `json:"source_uri"`
	Digest              string    `json:"digest"`
	Manifest            JSON      `json:"manifest"`
	Status              string    `json:"status"`
	IsBuiltin           bool      `json:"is_builtin"`
}

type AgentSkillBinding struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	TenantID  uint64    `gorm:"column:tenant_id" json:"tenant_id"`
	AgentID   string    `gorm:"column:agent_id" json:"agent_id"`
	SkillID   string    `gorm:"column:skill_id" json:"skill_id"`
	Enabled   bool      `gorm:"column:enabled" json:"enabled"`
	Config    JSON      `gorm:"column:config;type:jsonb;default:'{}'" json:"config"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentSkillBinding) TableName() string {
	return "agent_skill_bindings"
}

type TenantSkillCredential struct {
	ID          string    `gorm:"column:id;primaryKey" json:"id"`
	TenantID    uint64    `gorm:"column:tenant_id" json:"tenant_id"`
	SkillID     string    `gorm:"column:skill_id" json:"skill_id"`
	Credentials JSON      `gorm:"column:credentials;type:jsonb;default:'{}'" json:"credentials"`
	UpdatedBy   string    `gorm:"column:updated_by" json:"updated_by"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TenantSkillCredential) TableName() string {
	return "tenant_skill_credentials"
}

type TenantSkillMCPBinding struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	TenantID  uint64    `gorm:"column:tenant_id" json:"tenant_id"`
	SkillID   string    `gorm:"column:skill_id" json:"skill_id"`
	MCPName   string    `gorm:"column:mcp_name" json:"mcp_name"`
	ServiceID string    `gorm:"column:service_id" json:"service_id"`
	UpdatedBy string    `gorm:"column:updated_by" json:"updated_by"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TenantSkillMCPBinding) TableName() string {
	return "tenant_skill_mcp_bindings"
}

type SkillExecutionRun struct {
	ID            string    `gorm:"column:id;primaryKey" json:"id"`
	TenantID      uint64    `gorm:"column:tenant_id" json:"tenant_id"`
	UserID        string    `gorm:"column:user_id" json:"user_id"`
	AgentID       string    `gorm:"column:agent_id" json:"agent_id"`
	SessionID     string    `gorm:"column:session_id" json:"session_id"`
	MessageID     string    `gorm:"column:message_id" json:"message_id"`
	ToolCallID    string    `gorm:"column:tool_call_id" json:"tool_call_id"`
	SkillID       string    `gorm:"column:skill_id" json:"skill_id"`
	ScriptPath    string    `gorm:"column:script_path" json:"script_path"`
	Status        string    `gorm:"column:status" json:"status"`
	DurationMS    int64     `gorm:"column:duration_ms" json:"duration_ms"`
	ResourceUsage JSON      `gorm:"column:resource_usage;type:jsonb;default:'{}'" json:"resource_usage"`
	ErrorSummary  string    `gorm:"column:error_summary" json:"error_summary"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SkillExecutionRun) TableName() string {
	return "skill_execution_runs"
}
