# Tenant and Agent Skill Bindings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tenant-level Skill installation records and Agent-level Skill bindings while preserving the existing Agent editor contract based on `skills_selection_mode` and `selected_skills`.

**Architecture:** Build on the Phase 1 Skill registry. Preloaded Skills are imported into the global `skills` table, then each tenant can receive install rows in `tenant_skill_installs`. Agent bindings use `agent_skill_bindings` internally, but the public Agent config remains name-based for compatibility. Runtime skill selection resolves the current Agent config against tenant-installed Skills before exposing Skills to the Agent.

**Tech Stack:** Go, Gin, GORM, existing Skill registry repository/service, existing CustomAgent config JSON, existing RBAC guards.

---

## Scope

This phase does not add external Skill Hub installation or cloud sandbox runtime changes. It creates the backend foundation:

- tenant install rows for registry Skills;
- API listing from tenant-installed Skills;
- Agent binding repository/service methods;
- compatibility sync between `CustomAgent.Config.SelectedSkills` and binding rows;
- runtime filtering so selected Skills must be installed in the Agent's tenant.

## File Structure

- Modify `internal/types/interfaces/skill.go`
  Add tenant install and Agent binding repository/service methods.

- Modify `internal/application/repository/skill_repository.go`
  Add GORM methods for tenant installs and Agent bindings.

- Modify `internal/application/repository/skill_repository_test.go`
  Cover tenant install upsert/listing and Agent binding replacement/listing.

- Modify `internal/application/service/skill_service.go`
  Add tenant install helpers, tenant-scoped listing, binding sync, and selected-name resolution.

- Modify `internal/application/service/skill_service_test.go`
  Cover tenant install listing and Agent binding sync/resolve behavior.

- Modify `internal/handler/skill_handler.go`
  Make `ListSkills` tenant-aware and return tenant-installed Skills, preserving the response shape.

- Modify `internal/application/service/custom_agent.go`
  After create/update, sync Agent Skill bindings from `SkillsSelectionMode` and `SelectedSkills`.

- Modify `internal/application/service/session_agent_qa.go`
  Resolve selected Skills against tenant-installed Skill bindings before setting `AgentConfig.AllowedSkills`.

- Modify `internal/container/container.go`
  Inject `SkillService` into services that need it if not already available.

- Modify `internal/router/router.go`
  Update route comments only if needed; do not add new public routes in this phase.

- Modify `docs/agent-skills.md`
  Document tenant-installed Skills and Agent bindings.

## Task 1: Extend Skill Repository Interface

**Files:**
- Modify: `internal/types/interfaces/skill.go`

- [ ] **Step 1: Update `SkillRepository`**

Add these methods:

```go
	UpsertTenantSkillInstall(ctx context.Context, install *types.TenantSkillInstall) error
	ListTenantInstalledSkills(ctx context.Context, tenantID uint64) ([]*types.SkillRegistryEntry, error)
	ListTenantInstalledSkillNames(ctx context.Context, tenantID uint64) (map[string]*types.SkillRegistryEntry, error)
	ReplaceAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, skillIDs []string) error
	ListAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string) ([]*types.SkillRegistryEntry, error)
```

- [ ] **Step 2: Update `SkillService`**

Add these methods:

```go
	EnsureTenantPreloadedSkillInstalls(ctx context.Context, tenantID uint64) error
	ListTenantSkills(ctx context.Context, tenantID uint64) ([]*skills.SkillMetadata, error)
	SyncAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) error
	ResolveAgentSelectedSkills(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) ([]string, error)
```

- [ ] **Step 3: Run compile check**

Run:

```bash
go test ./internal/types/... -count=1
```

Expected: `internal/types` passes and `internal/types/interfaces` compiles.

- [ ] **Step 4: Commit**

```bash
git add internal/types/interfaces/skill.go
git commit -m "feat: extend skill binding interfaces"
```

## Task 2: Implement Tenant Install Repository Methods

**Files:**
- Modify: `internal/application/repository/skill_repository.go`
- Modify: `internal/application/repository/skill_repository_test.go`

- [ ] **Step 1: Add tests**

Add tests that:

- create two active Skill registry entries;
- upsert enabled tenant install rows for tenant `10`;
- assert `ListTenantInstalledSkills(ctx, 10)` returns only installed enabled Skills ordered by name/version;
- assert a disabled install is not returned;
- assert `ListTenantInstalledSkillNames(ctx, 10)` returns a map keyed by Skill name.

Use `AutoMigrate(&types.SkillRegistryEntry{}, &types.TenantSkillInstall{})` and create the same unique indexes used by the migration.

- [ ] **Step 2: Implement methods**

Implement:

```go
func (r *skillRepository) UpsertTenantSkillInstall(ctx context.Context, install *types.TenantSkillInstall) error
func (r *skillRepository) ListTenantInstalledSkills(ctx context.Context, tenantID uint64) ([]*types.SkillRegistryEntry, error)
func (r *skillRepository) ListTenantInstalledSkillNames(ctx context.Context, tenantID uint64) (map[string]*types.SkillRegistryEntry, error)
```

`ListTenantInstalledSkills` should join `tenant_skill_installs` to `skills`, filter `enabled = true` on install rows and `status = active` on Skill rows, and order by `skills.name ASC, skills.version ASC`.

- [ ] **Step 3: Run tests**

Run:

```bash
go test ./internal/application/repository -run 'TestSkillRepository_(Tenant|Upsert|List)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/application/repository/skill_repository.go internal/application/repository/skill_repository_test.go
git commit -m "feat: add tenant skill install repository"
```

## Task 3: Implement Agent Binding Repository Methods

**Files:**
- Modify: `internal/application/repository/skill_repository.go`
- Modify: `internal/application/repository/skill_repository_test.go`

- [ ] **Step 1: Add tests**

Add tests that:

- create active Skill registry entries;
- call `ReplaceAgentSkillBindings(ctx, 10, "agent-a", []string{skillID1, skillID2})`;
- assert `ListAgentSkillBindings(ctx, 10, "agent-a")` returns those Skills;
- call replacement again with only `skillID2`;
- assert the old binding is removed and only `skillID2` remains;
- assert tenant `11` with the same `agentID` has no bindings.

Use `AutoMigrate(&types.SkillRegistryEntry{}, &types.AgentSkillBinding{})` and create the unique index from migration.

- [ ] **Step 2: Implement methods**

Implement:

```go
func (r *skillRepository) ReplaceAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, skillIDs []string) error
func (r *skillRepository) ListAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string) ([]*types.SkillRegistryEntry, error)
```

Use a DB transaction. Delete existing rows for `(tenant_id, agent_id)`, then insert the new bindings when `skillIDs` is non-empty.

- [ ] **Step 3: Run tests**

Run:

```bash
go test ./internal/application/repository -run 'TestSkillRepository_(Agent|Tenant|Upsert|List)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/application/repository/skill_repository.go internal/application/repository/skill_repository_test.go
git commit -m "feat: add agent skill binding repository"
```

## Task 4: Add Skill Service Tenant Install and Binding Logic

**Files:**
- Modify: `internal/application/service/skill_service.go`
- Modify: `internal/application/service/skill_service_test.go`

- [ ] **Step 1: Add tests**

Add tests that cover:

- `EnsureTenantPreloadedSkillInstalls` imports preloaded Skills and creates tenant install rows.
- `ListTenantSkills` returns tenant-installed Skills and falls back to global preloaded listing when `tenantID == 0` or repo is nil.
- `SyncAgentSkillBindings` with mode `selected` writes bindings for selected installed Skill names and ignores unknown names.
- `SyncAgentSkillBindings` with mode `none` or `all` clears explicit bindings.
- `ResolveAgentSelectedSkills` with mode `selected` returns the intersection of selected names and tenant-installed names.
- `ResolveAgentSelectedSkills` with mode `all` returns all tenant-installed Skill names.

- [ ] **Step 2: Implement service methods**

Implementation rules:

- `EnsureTenantPreloadedSkillInstalls(ctx, tenantID)` calls `ImportPreloadedSkills`, lists active registry Skills, and upserts enabled install rows for the tenant.
- tenant install IDs should be deterministic and bounded to 64 chars using a helper similar to `skillRegistryID`.
- `ListTenantSkills(ctx, tenantID)` ensures tenant installs, then lists tenant-installed Skills. If no repo or tenant is zero, fall back to `ListPreloadedSkills`.
- `SyncAgentSkillBindings` resolves selected names to installed Skill IDs. It replaces binding rows only for `selected` mode; for `all` and `none`, it clears explicit binding rows.
- `ResolveAgentSelectedSkills` preserves compatibility by returning Skill names for `AgentConfig.AllowedSkills`.

- [ ] **Step 3: Run tests**

Run:

```bash
go test ./internal/application/service -run 'TestSkillService' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/application/service/skill_service.go internal/application/service/skill_service_test.go
git commit -m "feat: add tenant and agent skill service logic"
```

## Task 5: Make Skill Handler Tenant-Aware

**Files:**
- Modify: `internal/handler/skill_handler.go`

- [ ] **Step 1: Update handler logic**

In `ListSkills`, read tenant ID with:

```go
tenantID := c.GetUint64(types.TenantIDContextKey.String())
```

Call:

```go
skillsMetadata, err := h.skillService.ListTenantSkills(ctx, tenantID)
```

If tenant ID is zero, keep current behavior by falling back through service logic.

- [ ] **Step 2: Run tests**

Run:

```bash
go test ./internal/handler ./internal/application/service ./internal/application/repository -run 'TestSkill|TestNonExistent' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/handler/skill_handler.go
git commit -m "feat: list tenant installed skills"
```

## Task 6: Sync Agent Bindings on Agent Create and Update

**Files:**
- Modify: `internal/application/service/custom_agent.go`
- Modify: `internal/container/container.go` if constructor injection needs updating.

- [ ] **Step 1: Inspect `NewCustomAgentService` constructor**

Confirm whether adding `interfaces.SkillService` to the service struct is required and update DI if needed.

- [ ] **Step 2: Add sync helper**

After successful create/update, call:

```go
if s.skillService != nil {
	err := s.skillService.SyncAgentSkillBindings(
		ctx,
		agent.TenantID,
		agent.ID,
		agent.Config.SkillsSelectionMode,
		agent.Config.SelectedSkills,
	)
	if err != nil {
		return err
	}
}
```

Do this after persistence so binding rows do not point at an Agent create that failed.

- [ ] **Step 3: Run focused compile**

Run:

```bash
go test ./internal/application/service -run 'TestSkill|TestAgent|TestCustomAgent|TestNonExistent' -count=1
```

Expected: PASS or no matching tests, no compile errors.

- [ ] **Step 4: Commit**

```bash
git add internal/application/service/custom_agent.go internal/container/container.go
git commit -m "feat: sync agent skill bindings"
```

## Task 7: Use Tenant Installs During Runtime Skill Resolution

**Files:**
- Modify: `internal/application/service/session_agent_qa.go`

- [ ] **Step 1: Inject SkillService into session service if needed**

If `sessionService` does not already have `interfaces.SkillService`, add it to the constructor and struct through DI.

- [ ] **Step 2: Update `configureSkillsFromAgent`**

For mode `all`, use `ResolveAgentSelectedSkills(ctx, customAgent.TenantID, customAgent.ID, "all", nil)` to populate `AllowedSkills`.

For mode `selected`, use `ResolveAgentSelectedSkills(ctx, customAgent.TenantID, customAgent.ID, "selected", customAgent.Config.SelectedSkills)`.

If resolution returns no names, disable Skills.

Keep `SkillDirs` as the preloaded dir for Phase 2.

- [ ] **Step 3: Preserve sandbox gating**

Keep the existing sandbox disabled behavior unchanged.

- [ ] **Step 4: Run focused tests**

Run:

```bash
go test ./internal/application/service -run 'TestSkill|TestSession|TestAgent|TestNonExistent' -count=1
```

Expected: PASS or no matching tests, no compile errors.

- [ ] **Step 5: Commit**

```bash
git add internal/application/service/session_agent_qa.go
git commit -m "feat: resolve runtime skills from tenant installs"
```

## Task 8: Update Docs

**Files:**
- Modify: `docs/agent-skills.md`

- [ ] **Step 1: Add Phase 2 note**

Extend the registry-backed section with:

```markdown
第二阶段开始，预装 Skill 会为租户生成安装记录，Agent 编辑页仍使用 `skills_selection_mode` 和 `selected_skills` 配置，但后端会同步到 `agent_skill_bindings`。运行时会根据当前 Agent 所属租户的已安装 Skill 过滤可用 Skill，避免 Agent 暴露未安装或已禁用的 Skill。
```

- [ ] **Step 2: Commit**

```bash
git add docs/agent-skills.md
git commit -m "docs: describe tenant agent skill bindings"
```

## Task 9: Final Verification

**Files:**
- No new files.

- [ ] **Step 1: Run focused Skill tests**

Run:

```bash
go test ./internal/application/repository ./internal/application/service ./internal/handler -run 'TestSkill|TestAgent|TestSession|TestNonExistent' -count=1
```

Expected: PASS or no matching tests, no compile errors.

- [ ] **Step 2: Run broader related packages**

Run:

```bash
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -count=1
```

Expected: PASS.

- [ ] **Step 3: Run final diff review**

Review:

```bash
git diff --stat HEAD~8..HEAD
git diff HEAD~8..HEAD -- internal/types/interfaces/skill.go internal/application/repository/skill_repository.go internal/application/service/skill_service.go internal/handler/skill_handler.go internal/application/service/custom_agent.go internal/application/service/session_agent_qa.go
```

Expected: no Critical or Important issues.

## Self-Review

- Spec coverage: This plan implements tenant install rows, Agent binding rows, compatibility sync from existing Agent config, tenant-aware Skill listing, and runtime filtering. It intentionally does not add public install/upload APIs or Skill Hub.
- Placeholder scan: No placeholder markers or vague implementation steps remain.
- Type consistency: The plan consistently uses `SkillRepository`, `SkillService`, `TenantSkillInstall`, `AgentSkillBinding`, `SkillRegistryEntry`, `skills_selection_mode`, and `selected_skills`.
