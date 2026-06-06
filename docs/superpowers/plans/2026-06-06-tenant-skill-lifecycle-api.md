# Tenant Skill Lifecycle API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tenant-level Skill lifecycle APIs so admins can inspect installed Skills and enable or disable them without deleting registry records or package files.

**Architecture:** Keep `GET /api/v1/skills` as the compatibility endpoint that returns only enabled Skill metadata. Add a richer installed list endpoint backed by `tenant_skill_installs` joins, plus a patch endpoint that toggles the tenant install `enabled` flag. Runtime already filters by enabled tenant installs, so disabled Skills naturally disappear from Agent prompt and loader access.

**Tech Stack:** Go, Gin, GORM, existing Skill registry tables and service/repository interfaces.

---

### Task 1: Repository Lifecycle Queries

**Files:**
- Modify: `internal/types/skill_registry.go`
- Modify: `internal/types/interfaces/skill.go`
- Modify: `internal/application/repository/skill_repository.go`
- Modify: `internal/application/repository/skill_repository_test.go`

- [ ] **Step 1: Add tests**

Add repository tests for:
- listing installed Skill entries for a tenant, including disabled installs
- toggling `tenant_skill_installs.enabled`
- returning an error when toggling a missing tenant install

- [ ] **Step 2: Implement repository methods**

Add:
- `TenantSkillInstallInfo` read model in `types`
- `ListTenantSkillInstallEntries(ctx, tenantID)`
- `SetTenantSkillInstallEnabled(ctx, tenantID, skillID, enabled)`

- [ ] **Step 3: Verify repository**

Run:

```bash
go test ./internal/application/repository -run 'TestSkillRepository_.*TenantSkillInstall' -count=1
```

Expected: PASS.

### Task 2: Service Lifecycle Methods

**Files:**
- Modify: `internal/types/interfaces/skill.go`
- Modify: `internal/application/service/skill_service.go`
- Modify: `internal/application/service/skill_service_test.go`

- [ ] **Step 1: Add service tests**

Add tests that:
- ensure preloaded installs exist
- list installed Skills with enabled state
- disable one Skill
- verify compatibility list and runtime access no longer include the disabled Skill

- [ ] **Step 2: Implement service methods**

Add:
- `ListTenantSkillInstalls(ctx, tenantID)`
- `SetTenantSkillEnabled(ctx, tenantID, skillID, enabled)`

`ListTenantSkillInstalls` should ensure preloaded installs exist before listing, matching the compatibility list behavior.

- [ ] **Step 3: Verify service**

Run:

```bash
go test ./internal/application/service -run 'TestSkillService_.*TenantSkillLifecycle|TestSkillService_EnsureTenantPreloadedSkillInstalls' -count=1
```

Expected: PASS.

### Task 3: Handler and Routes

**Files:**
- Modify: `internal/handler/skill_handler.go`
- Modify: `internal/handler/skill_handler_test.go`
- Modify: `internal/router/router.go`
- Modify: `docs/agent-skills.md`

- [ ] **Step 1: Add handler tests**

Add tests for:
- `GET /skills/installed` returning installed Skill state
- `PATCH /skills/:skill_id` calling service with `enabled=false`
- invalid patch JSON returning `400`

- [ ] **Step 2: Implement handler**

Add request/response DTOs and methods:
- `ListInstalledSkills`
- `UpdateTenantSkillInstall`

- [ ] **Step 3: Register routes**

Add:
- `GET /skills/installed` as Viewer+
- `PATCH /skills/:skill_id` as Admin+

- [ ] **Step 4: Update docs**

Document the lifecycle endpoints and clarify that disabling a tenant install hides the Skill from compatibility listing and runtime access while preserving registry/package data.

- [ ] **Step 5: Verify API**

Run:

```bash
go test ./internal/handler -run 'TestSkillHandler_.*Skill' -count=1
```

Expected: PASS.

### Task 4: Final Verification

- [ ] **Step 1: Focused tests**

Run:

```bash
go test ./internal/application/repository ./internal/application/service ./internal/handler -run 'TestSkill|TestTenantSkill|TestLocal|TestAgent' -count=1
```

- [ ] **Step 2: Broader tests**

Run:

```bash
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -count=1
```

- [ ] **Step 3: Diff checks**

Run:

```bash
git status --short
git diff --check
```
