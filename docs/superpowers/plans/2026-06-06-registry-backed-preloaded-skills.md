# Registry-Backed Preloaded Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move preloaded Agent Skill listing behind a persistent Skill registry while preserving current `SKILL.md`, Agent selection, and sandbox execution behavior.

**Architecture:** Add a small database-backed registry for built-in Skills, import `skills/preloaded` metadata into it, and make the existing Skill API read from the registry. Keep existing filesystem loading as the runtime source for `read_skill` and `execute_skill_script` in this phase to reduce risk.

**Tech Stack:** Go, Gin, GORM, SQL migrations, existing `internal/agent/skills` loader, existing DI container.

---

## File Structure

- Create `migrations/versioned/000059_skill_registry.up.sql`
  Creates `skills`, `tenant_skill_installs`, `agent_skill_bindings`, and `skill_execution_runs` tables for the first registry-backed slice.

- Create `migrations/versioned/000059_skill_registry.down.sql`
  Drops the new Skill registry tables in reverse dependency order.

- Create `internal/types/skill_registry.go`
  Defines persistent Skill registry entities and constants.

- Modify `internal/types/interfaces/skill.go`
  Adds repository methods and upgrades the service interface to expose registry-backed listing and preloaded import.

- Create `internal/application/repository/skill_repository.go`
  Implements GORM repository methods for upserting and listing Skill registry records.

- Create `internal/application/repository/skill_repository_test.go`
  Tests upsert idempotency and list ordering on SQLite.

- Modify `internal/application/service/skill_service.go`
  Injects the new repository, imports preloaded Skill metadata into the registry, and lists registry records with filesystem fallback.

- Create `internal/application/service/skill_service_test.go`
  Tests import behavior and fallback behavior with temporary Skill directories.

- Modify `internal/container/container.go`
  Registers `repository.NewSkillRepository` before `service.NewSkillService`.

- Modify `internal/handler/skill_handler.go`
  Keeps the same response shape but calls the registry-backed service method.

- Modify `docs/agent-skills.md`
  Notes that preloaded Skills are now registry-backed while script execution still uses the installed package files in Phase 1.

## Task 1: Add Skill Registry Migrations

**Files:**
- Create: `migrations/versioned/000059_skill_registry.up.sql`
- Create: `migrations/versioned/000059_skill_registry.down.sql`

- [ ] **Step 1: Add the up migration**

Create `migrations/versioned/000059_skill_registry.up.sql`:

```sql
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
```

- [ ] **Step 2: Add the down migration**

Create `migrations/versioned/000059_skill_registry.down.sql`:

```sql
-- ============================================================================
-- Migration 000059: Drop Skill registry tables
-- ============================================================================

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Dropping Skill registry tables...'; END $$;

DROP TABLE IF EXISTS skill_execution_runs;
DROP TABLE IF EXISTS agent_skill_bindings;
DROP TABLE IF EXISTS tenant_skill_installs;
DROP TABLE IF EXISTS skills;

DO $$ BEGIN RAISE NOTICE '[Migration 000059] Skill registry tables dropped'; END $$;
```

- [ ] **Step 3: Validate migration files are present**

Run:

```bash
ls migrations/versioned/000059_skill_registry.*.sql
```

Expected output includes:

```text
migrations/versioned/000059_skill_registry.down.sql
migrations/versioned/000059_skill_registry.up.sql
```

- [ ] **Step 4: Commit**

```bash
git add migrations/versioned/000059_skill_registry.up.sql migrations/versioned/000059_skill_registry.down.sql
git commit -m "feat: add skill registry migrations"
```

## Task 2: Add Skill Registry Types and Interfaces

**Files:**
- Create: `internal/types/skill_registry.go`
- Modify: `internal/types/interfaces/skill.go`

- [ ] **Step 1: Add persistent types**

Create `internal/types/skill_registry.go`:

```go
package types

import (
	"time"

	"gorm.io/datatypes"
)

const (
	SkillSourceTypePreloaded = "preloaded"
	SkillStatusActive        = "active"
	SkillStatusDisabled      = "disabled"
	DefaultSkillVersion      = "0.0.0"
)

type SkillRegistryEntry struct {
	ID          string         `gorm:"column:id;primaryKey" json:"id"`
	Name        string         `gorm:"column:name" json:"name"`
	Version     string         `gorm:"column:version" json:"version"`
	Description string         `gorm:"column:description" json:"description"`
	SourceType  string         `gorm:"column:source_type" json:"source_type"`
	SourceURI   string         `gorm:"column:source_uri" json:"source_uri"`
	Digest      string         `gorm:"column:digest" json:"digest"`
	Manifest    datatypes.JSON `gorm:"column:manifest" json:"manifest"`
	Status      string         `gorm:"column:status" json:"status"`
	IsBuiltin   bool           `gorm:"column:is_builtin" json:"is_builtin"`
	CreatedAt   time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

func (SkillRegistryEntry) TableName() string {
	return "skills"
}

type TenantSkillInstall struct {
	ID                  string         `gorm:"column:id;primaryKey" json:"id"`
	TenantID            uint64         `gorm:"column:tenant_id" json:"tenant_id"`
	SkillID             string         `gorm:"column:skill_id" json:"skill_id"`
	Enabled             bool           `gorm:"column:enabled" json:"enabled"`
	InstalledBy         string         `gorm:"column:installed_by" json:"installed_by"`
	ApprovedPermissions datatypes.JSON `gorm:"column:approved_permissions" json:"approved_permissions"`
	CreatedAt           time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

func (TenantSkillInstall) TableName() string {
	return "tenant_skill_installs"
}

type AgentSkillBinding struct {
	ID        string         `gorm:"column:id;primaryKey" json:"id"`
	TenantID  uint64         `gorm:"column:tenant_id" json:"tenant_id"`
	AgentID   string         `gorm:"column:agent_id" json:"agent_id"`
	SkillID   string         `gorm:"column:skill_id" json:"skill_id"`
	Enabled   bool           `gorm:"column:enabled" json:"enabled"`
	Config    datatypes.JSON `gorm:"column:config" json:"config"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

func (AgentSkillBinding) TableName() string {
	return "agent_skill_bindings"
}

type SkillExecutionRun struct {
	ID            string         `gorm:"column:id;primaryKey" json:"id"`
	TenantID      uint64         `gorm:"column:tenant_id" json:"tenant_id"`
	UserID        string         `gorm:"column:user_id" json:"user_id"`
	AgentID       string         `gorm:"column:agent_id" json:"agent_id"`
	SessionID     string         `gorm:"column:session_id" json:"session_id"`
	MessageID     string         `gorm:"column:message_id" json:"message_id"`
	ToolCallID    string         `gorm:"column:tool_call_id" json:"tool_call_id"`
	SkillID       string         `gorm:"column:skill_id" json:"skill_id"`
	ScriptPath    string         `gorm:"column:script_path" json:"script_path"`
	Status        string         `gorm:"column:status" json:"status"`
	DurationMS    int64          `gorm:"column:duration_ms" json:"duration_ms"`
	ResourceUsage datatypes.JSON `gorm:"column:resource_usage" json:"resource_usage"`
	ErrorSummary  string         `gorm:"column:error_summary" json:"error_summary"`
	CreatedAt     time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

func (SkillExecutionRun) TableName() string {
	return "skill_execution_runs"
}
```

- [ ] **Step 2: Update interfaces**

Replace `internal/types/interfaces/skill.go` with:

```go
package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/types"
)

// SkillService defines the interface for skill business logic.
type SkillService interface {
	// ListPreloadedSkills returns metadata for Skills available to the current deployment.
	ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error)

	// ImportPreloadedSkills scans preloaded Skill directories and upserts their metadata into the registry.
	ImportPreloadedSkills(ctx context.Context) error

	// GetSkillByName retrieves a Skill's instructions from the current package source.
	GetSkillByName(ctx context.Context, name string) (*skills.Skill, error)
}

// SkillRepository defines registry persistence for installed and built-in Skills.
type SkillRepository interface {
	UpsertSkill(ctx context.Context, skill *types.SkillRegistryEntry) error
	ListActiveSkills(ctx context.Context) ([]*types.SkillRegistryEntry, error)
	GetSkillByName(ctx context.Context, name string) (*types.SkillRegistryEntry, error)
	CountSkills(ctx context.Context) (int64, error)
}
```

- [ ] **Step 3: Run gofmt**

Run:

```bash
gofmt -w internal/types/skill_registry.go internal/types/interfaces/skill.go
```

Expected: command exits with status 0.

- [ ] **Step 4: Commit**

```bash
git add internal/types/skill_registry.go internal/types/interfaces/skill.go
git commit -m "feat: add skill registry types"
```

## Task 3: Implement Skill Repository

**Files:**
- Create: `internal/application/repository/skill_repository.go`
- Create: `internal/application/repository/skill_repository_test.go`

- [ ] **Step 1: Write repository tests**

Create `internal/application/repository/skill_repository_test.go`:

```go
package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&types.SkillRegistryEntry{}); err != nil {
		t.Fatalf("migrate skills: %v", err)
	}
	return db
}

func TestSkillRepositoryUpsertSkillIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)

	first := &types.SkillRegistryEntry{
		ID:          "preloaded-document-analyzer-0.0.0",
		Name:        "document-analyzer",
		Version:     types.DefaultSkillVersion,
		Description: "Old description",
		SourceType:  types.SkillSourceTypePreloaded,
		SourceURI:   "skills/preloaded/document-analyzer",
		Status:      types.SkillStatusActive,
		IsBuiltin:   true,
	}
	if err := repo.UpsertSkill(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := *first
	second.Description = "New description"
	if err := repo.UpsertSkill(ctx, &second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	count, err := repo.CountSkills(ctx)
	if err != nil {
		t.Fatalf("count skills: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 skill, got %d", count)
	}

	got, err := repo.GetSkillByName(ctx, "document-analyzer")
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Description != "New description" {
		t.Fatalf("expected updated description, got %q", got.Description)
	}
}

func TestSkillRepositoryListActiveSkillsOrdersByName(t *testing.T) {
	ctx := context.Background()
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)

	entries := []*types.SkillRegistryEntry{
		{
			ID:          "preloaded-zeta-0.0.0",
			Name:        "zeta",
			Version:     types.DefaultSkillVersion,
			Description: "Zeta",
			SourceType:  types.SkillSourceTypePreloaded,
			Status:      types.SkillStatusActive,
			IsBuiltin:   true,
		},
		{
			ID:          "preloaded-alpha-0.0.0",
			Name:        "alpha",
			Version:     types.DefaultSkillVersion,
			Description: "Alpha",
			SourceType:  types.SkillSourceTypePreloaded,
			Status:      types.SkillStatusActive,
			IsBuiltin:   true,
		},
		{
			ID:          "preloaded-disabled-0.0.0",
			Name:        "disabled",
			Version:     types.DefaultSkillVersion,
			Description: "Disabled",
			SourceType:  types.SkillSourceTypePreloaded,
			Status:      types.SkillStatusDisabled,
			IsBuiltin:   true,
		},
	}
	for _, entry := range entries {
		if err := repo.UpsertSkill(ctx, entry); err != nil {
			t.Fatalf("upsert %s: %v", entry.Name, err)
		}
	}

	got, err := repo.ListActiveSkills(ctx)
	if err != nil {
		t.Fatalf("list active skills: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 active skills, got %d", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("unexpected order: %s, %s", got[0].Name, got[1].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/application/repository -run TestSkillRepository -count=1
```

Expected: FAIL because `NewSkillRepository` does not exist.

- [ ] **Step 3: Implement repository**

Create `internal/application/repository/skill_repository.go`:

```go
package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type skillRepository struct {
	db *gorm.DB
}

func NewSkillRepository(db *gorm.DB) interfaces.SkillRepository {
	return &skillRepository{db: db}
}

func (r *skillRepository) UpsertSkill(ctx context.Context, skill *types.SkillRegistryEntry) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "version"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"description",
				"source_type",
				"source_uri",
				"digest",
				"manifest",
				"status",
				"is_builtin",
				"updated_at",
			}),
		}).
		Create(skill).Error
}

func (r *skillRepository) ListActiveSkills(ctx context.Context) ([]*types.SkillRegistryEntry, error) {
	var entries []*types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Where("status = ?", types.SkillStatusActive).
		Order("name ASC").
		Order("version ASC").
		Find(&entries).Error
	return entries, err
}

func (r *skillRepository) GetSkillByName(ctx context.Context, name string) (*types.SkillRegistryEntry, error) {
	var entry types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Where("name = ? AND status = ?", name, types.SkillStatusActive).
		Order("version DESC").
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *skillRepository) CountSkills(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.SkillRegistryEntry{}).Count(&count).Error
	return count, err
}
```

- [ ] **Step 4: Run gofmt**

Run:

```bash
gofmt -w internal/application/repository/skill_repository.go internal/application/repository/skill_repository_test.go
```

Expected: command exits with status 0.

- [ ] **Step 5: Run repository tests**

Run:

```bash
go test ./internal/application/repository -run TestSkillRepository -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/repository/skill_repository.go internal/application/repository/skill_repository_test.go
git commit -m "feat: add skill registry repository"
```

## Task 4: Make Skill Service Import and List Registry Entries

**Files:**
- Modify: `internal/application/service/skill_service.go`
- Create: `internal/application/service/skill_service_test.go`

- [ ] **Step 1: Write service tests**

Create `internal/application/service/skill_service_test.go`:

```go
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&types.SkillRegistryEntry{}); err != nil {
		t.Fatalf("migrate skills: %v", err)
	}
	return db
}

func writeTestSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()

	skillDir := filepath.Join(root, dir)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func TestSkillServiceImportsPreloadedSkillsIntoRegistry(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha", "alpha", "Alpha skill")

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, tempDir)

	if err := svc.ImportPreloadedSkills(ctx); err != nil {
		t.Fatalf("import preloaded skills: %v", err)
	}

	got, err := svc.ListPreloadedSkills(ctx)
	if err != nil {
		t.Fatalf("list preloaded skills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(got))
	}
	if got[0].Name != "alpha" || got[0].Description != "Alpha skill" {
		t.Fatalf("unexpected skill metadata: %#v", got[0])
	}
}

func TestSkillServiceFallsBackToFilesystemWhenRegistryIsEmpty(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "beta", "beta", "Beta skill")

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, tempDir)

	got, err := svc.ListPreloadedSkills(ctx)
	if err != nil {
		t.Fatalf("list preloaded skills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 fallback skill, got %d", len(got))
	}
	if got[0].Name != "beta" {
		t.Fatalf("expected beta, got %s", got[0].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/application/service -run TestSkillService -count=1
```

Expected: FAIL because `NewSkillServiceWithRepository` does not exist.

- [ ] **Step 3: Replace service implementation**

Update `internal/application/service/skill_service.go` so its imports and implementation match this:

```go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/datatypes"
)

// DefaultPreloadedSkillsDir is the default directory for preloaded skills.
const DefaultPreloadedSkillsDir = "skills/preloaded"

type skillService struct {
	loader       *skills.Loader
	repo         interfaces.SkillRepository
	preloadedDir string
	mu           sync.RWMutex
	initialized  bool
}

func NewSkillService(repo interfaces.SkillRepository) interfaces.SkillService {
	return NewSkillServiceWithRepository(repo, getPreloadedSkillsDir())
}

func NewSkillServiceWithRepository(repo interfaces.SkillRepository, preloadedDir string) interfaces.SkillService {
	return &skillService{
		repo:         repo,
		preloadedDir: preloadedDir,
		initialized:  false,
	}
}

func getPreloadedSkillsDir() string {
	if dir := os.Getenv("WEKNORA_SKILLS_DIR"); dir != "" {
		return dir
	}

	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		skillsDir := filepath.Join(execDir, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	cwd, err := os.Getwd()
	if err == nil {
		skillsDir := filepath.Join(cwd, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	return DefaultPreloadedSkillsDir
}

func (s *skillService) ensureInitialized(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	if _, err := os.Stat(s.preloadedDir); os.IsNotExist(err) {
		logger.Warnf(ctx, "Preloaded skills directory does not exist: %s", s.preloadedDir)
		if err := os.MkdirAll(s.preloadedDir, 0755); err != nil {
			logger.Warnf(ctx, "Failed to create preloaded skills directory: %v", err)
		}
	}

	s.loader = skills.NewLoader([]string{s.preloadedDir})
	s.initialized = true

	logger.Infof(ctx, "Skill service initialized with preloaded directory: %s", s.preloadedDir)

	return nil
}

func (s *skillService) ImportPreloadedSkills(ctx context.Context) error {
	if err := s.ensureInitialized(ctx); err != nil {
		return fmt.Errorf("failed to initialize skill service: %w", err)
	}
	if s.repo == nil {
		return nil
	}

	s.mu.RLock()
	metadata, err := s.loader.DiscoverSkills()
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to discover preloaded skills: %w", err)
	}

	for _, meta := range metadata {
		entry := preloadedSkillRegistryEntry(s.preloadedDir, meta)
		if err := s.repo.UpsertSkill(ctx, entry); err != nil {
			return fmt.Errorf("failed to upsert preloaded skill %s: %w", meta.Name, err)
		}
	}

	logger.Infof(ctx, "Imported %d preloaded skills into registry", len(metadata))
	return nil
}

func (s *skillService) ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	if s.repo != nil {
		entries, err := s.repo.ListActiveSkills(ctx)
		if err != nil {
			logger.Warnf(ctx, "Failed to list skills from registry, falling back to filesystem: %v", err)
		} else if len(entries) > 0 {
			return registryEntriesToMetadata(entries), nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, err := s.loader.DiscoverSkills()
	if err != nil {
		logger.Errorf(ctx, "Failed to discover preloaded skills: %v", err)
		return nil, fmt.Errorf("failed to discover skills: %w", err)
	}

	logger.Infof(ctx, "Discovered %d preloaded skills from filesystem fallback", len(metadata))
	return metadata, nil
}

func (s *skillService) GetSkillByName(ctx context.Context, name string) (*skills.Skill, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.loader.LoadSkillInstructions(name)
	if err != nil {
		logger.Errorf(ctx, "Failed to load skill %s: %v", name, err)
		return nil, fmt.Errorf("failed to load skill: %w", err)
	}

	return skill, nil
}

func (s *skillService) GetPreloadedDir() string {
	return s.preloadedDir
}

func registryEntriesToMetadata(entries []*types.SkillRegistryEntry) []*skills.SkillMetadata {
	result := make([]*skills.SkillMetadata, 0, len(entries))
	for _, entry := range entries {
		result = append(result, &skills.SkillMetadata{
			Name:        entry.Name,
			Description: entry.Description,
		})
	}
	return result
}

func preloadedSkillRegistryEntry(preloadedDir string, meta *skills.SkillMetadata) *types.SkillRegistryEntry {
	version := types.DefaultSkillVersion
	id := skillRegistryID(types.SkillSourceTypePreloaded, meta.Name, version)

	return &types.SkillRegistryEntry{
		ID:          id,
		Name:        meta.Name,
		Version:     version,
		Description: meta.Description,
		SourceType:  types.SkillSourceTypePreloaded,
		SourceURI:   filepath.Join(preloadedDir, meta.Name),
		Digest:      skillRegistryDigest(meta.Name, version, meta.Description),
		Manifest:    datatypes.JSON([]byte("{}")),
		Status:      types.SkillStatusActive,
		IsBuiltin:   true,
	}
}

func skillRegistryID(sourceType, name, version string) string {
	clean := regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(sourceType+"-"+name+"-"+version, "-")
	return strings.Trim(clean, "-")
}

func skillRegistryDigest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
```

- [ ] **Step 4: Run gofmt**

Run:

```bash
gofmt -w internal/application/service/skill_service.go internal/application/service/skill_service_test.go
```

Expected: command exits with status 0.

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./internal/application/service -run TestSkillService -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/service/skill_service.go internal/application/service/skill_service_test.go
git commit -m "feat: import preloaded skills into registry"
```

## Task 5: Wire Repository and Startup Import

**Files:**
- Modify: `internal/container/container.go`

- [ ] **Step 1: Register the Skill repository**

In `internal/container/container.go`, add this line in the repository registration block after `repository.NewMCPToolApprovalRepository`:

```go
	must(container.Provide(repository.NewSkillRepository))
```

- [ ] **Step 2: Add startup import helper**

Add this helper near other small container helpers in `internal/container/container.go`:

```go
func importPreloadedSkillsOnStartup(skillSvc interfaces.SkillService) error {
	return skillSvc.ImportPreloadedSkills(context.Background())
}
```

- [ ] **Step 3: Invoke startup import**

After service registration includes `service.NewSkillService`, add:

```go
	must(container.Invoke(importPreloadedSkillsOnStartup))
```

Keep `service.NewSkillService` registered once. Do not leave the old zero-argument constructor call if the signature has changed.

- [ ] **Step 4: Run gofmt**

Run:

```bash
gofmt -w internal/container/container.go
```

Expected: command exits with status 0.

- [ ] **Step 5: Run compile check**

Run:

```bash
go test ./internal/container ./internal/application/service ./internal/application/repository -run 'TestSkill|TestNonExistent' -count=1
```

Expected: PASS or no tests for `internal/container`; no compile errors.

- [ ] **Step 6: Commit**

```bash
git add internal/container/container.go
git commit -m "feat: wire skill registry startup import"
```

## Task 6: Update Handler and Docs

**Files:**
- Modify: `internal/handler/skill_handler.go`
- Modify: `docs/agent-skills.md`

- [ ] **Step 1: Update handler comments**

In `internal/handler/skill_handler.go`, update the `ListSkills` comments to say registry-backed Skills instead of only preloaded filesystem Skills:

```go
// ListSkills godoc
// @Summary      获取可用 Skills 列表
// @Description  获取当前部署中可用的 Agent Skills 元数据。预装 Skills 会在启动时同步到注册表。
```

Do not change the JSON response shape in this phase.

- [ ] **Step 2: Update docs**

In `docs/agent-skills.md`, add this section after the "预加载技能（Preloaded Skills）" heading:

```markdown
### Registry-backed preloaded Skills

从 Skill Registry 重构的第一阶段开始，`skills/preloaded` 仍然是预装 Skill 的包来源，但应用启动时会把其 metadata 同步到数据库注册表。前端和 `/api/v1/skills` 从注册表读取可用 Skill 列表；`read_skill` 和 `execute_skill_script` 在该阶段仍沿用本地包文件读取和沙箱执行路径。

这为后续租户级安装、Agent 级绑定、版本管理、权限审批和云端沙箱运行做准备，同时保持现有 Agent 配置兼容。
```

- [ ] **Step 3: Run gofmt**

Run:

```bash
gofmt -w internal/handler/skill_handler.go
```

Expected: command exits with status 0.

- [ ] **Step 4: Run focused tests**

Run:

```bash
go test ./internal/handler ./internal/application/service ./internal/application/repository -run 'TestSkill|TestNonExistent' -count=1
```

Expected: PASS or no tests for `internal/handler`; no compile errors.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/skill_handler.go docs/agent-skills.md
git commit -m "docs: describe registry-backed skills"
```

## Task 7: Final Verification

**Files:**
- No new files.

- [ ] **Step 1: Run all focused Skill tests**

Run:

```bash
go test ./internal/application/repository ./internal/application/service ./internal/handler -run 'TestSkill|TestNonExistent' -count=1
```

Expected: PASS or no tests for packages without Skill tests; no compile errors.

- [ ] **Step 2: Run broader compile check**

Run:

```bash
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler -count=1
```

Expected: PASS.

- [ ] **Step 3: Check worktree**

Run:

```bash
git status --short
```

Expected: no unstaged changes.

## Self-Review

- Spec coverage: This plan covers Phase 1 from the design doc: registry-backed preloaded Skills, startup import, API listing from registry, compatibility fallback, and documentation. Tenant install APIs, Agent binding APIs, package installation, and cloud runtime executor are intentionally left for later plans.
- Placeholder scan: No placeholder markers or vague implementation steps remain.
- Type consistency: The plan consistently uses `SkillRegistryEntry`, `SkillRepository`, `NewSkillRepository`, `NewSkillServiceWithRepository`, `ImportPreloadedSkills`, and `ListPreloadedSkills`.
