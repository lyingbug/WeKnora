# Local Skill Package Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a conservative tenant-scoped local Skill package install path that prepares WeKnora for Skill Hub installation without introducing remote download or sandbox execution yet.

**Architecture:** A Skill package is a directory under a configured packages root containing `skill.json` plus the instruction file declared by the manifest. The backend validates the manifest, upserts the Skill registry row, installs it for the tenant, and resolves runtime Skill search directories from installed registry metadata. Remote Hub install, signature verification, and runtime sandbox provisioning remain later phases.

**Tech Stack:** Go, Gin, GORM/PostgreSQL repository layer, existing `internal/agent/skills` loader and `SkillManager`.

---

### Task 1: Skill Package Manifest Parser

**Files:**
- Create: `internal/agent/skills/manifest.go`
- Create: `internal/agent/skills/manifest_test.go`

- [ ] **Step 1: Write manifest tests**

Create tests that build temporary package directories with `skill.json` and `SKILL.md`, then assert:
- valid packages load and expose manifest fields, raw JSON, and instruction path
- manifest name must match `SKILL.md` frontmatter name
- instruction entrypoint cannot escape the package directory

- [ ] **Step 2: Run focused tests**

Run: `go test ./internal/agent/skills -run TestLoadSkillPackageManifest -count=1`

Expected: FAIL before implementation because `LoadSkillPackageManifest` does not exist.

- [ ] **Step 3: Implement parser**

Add `SkillPackageManifest`, `SkillPackageEntrypoints`, `SkillPackageRuntime`, and `LoadedSkillPackageManifest` types.

Implement:
- `LoadSkillPackageManifest(packageDir string) (*LoadedSkillPackageManifest, error)`
- manifest JSON decoding
- default `entrypoints.instructions` to `SKILL.md`
- path containment validation
- `SKILL.md` parse and validation through existing `ParseSkillFile` and `Validate`
- name matching between manifest and `SKILL.md`
- version validation with `[A-Za-z0-9._-]+`

- [ ] **Step 4: Verify parser**

Run: `go test ./internal/agent/skills -run TestLoadSkillPackageManifest -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/skills/manifest.go internal/agent/skills/manifest_test.go
git commit -m "feat: validate local skill package manifests"
```

### Task 2: Local Package Install Service

**Files:**
- Modify: `internal/types/skill_registry.go`
- Modify: `internal/types/interfaces/skill.go`
- Modify: `internal/application/service/skill_service.go`
- Modify: `internal/application/service/skill_service_test.go`

- [ ] **Step 1: Write service tests**

Add tests that set `WEKNORA_SKILL_PACKAGES_DIR` to a temporary packages root and create `sample-skill/skill.json` plus `sample-skill/SKILL.md`.

Assert:
- `InstallLocalSkillPackage(ctx, tenantID, "sample-skill", installedBy)` upserts a `skills` row with `source_type=local`, `source_uri` pointing to the package directory, `manifest` containing the raw JSON, and a deterministic digest.
- it upserts an enabled `tenant_skill_installs` row with approved permissions from the manifest.
- absolute package paths outside the configured packages root are rejected.

- [ ] **Step 2: Run focused service tests**

Run: `go test ./internal/application/service -run TestSkillService_InstallLocalSkillPackage -count=1`

Expected: FAIL before implementation.

- [ ] **Step 3: Extend types and service interface**

Add:
- `SkillSourceTypeLocal = "local"`
- `InstallLocalSkillPackage(ctx context.Context, tenantID uint64, packagePath string, installedBy string) (*types.SkillRegistryEntry, error)`
- `ResolveAgentSkillAccess(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) ([]string, []string, error)`

- [ ] **Step 4: Implement local install**

In `skill_service.go`, implement:
- packages root lookup via `WEKNORA_SKILL_PACKAGES_DIR`, defaulting to `skills/packages`
- package path resolution constrained to the packages root
- deterministic package digest by sorted relative file path plus content
- registry entry ID using `skillRegistryID(types.SkillSourceTypeLocal, manifest.Name, manifest.Version)`
- tenant install ID using existing `tenantSkillInstallID`
- approved permissions marshaled from manifest permissions

- [ ] **Step 5: Verify service install**

Run: `go test ./internal/application/service -run TestSkillService_InstallLocalSkillPackage -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/types/skill_registry.go internal/types/interfaces/skill.go internal/application/service/skill_service.go internal/application/service/skill_service_test.go
git commit -m "feat: install local skill packages"
```

### Task 3: Runtime Skill Directory Resolution

**Files:**
- Modify: `internal/application/service/skill_service.go`
- Modify: `internal/application/service/skill_service_test.go`
- Modify: `internal/application/service/session_agent_qa.go`

- [ ] **Step 1: Write resolution tests**

Add service tests that install two Skills with different `source_uri` directories and assert:
- mode `selected` returns only selected installed names
- mode `all` returns all enabled installed names
- returned Skill dirs contain each package parent directory plus the preloaded directory when preloaded Skills are used
- duplicate parent directories are de-duplicated

- [ ] **Step 2: Implement access resolver**

Implement `ResolveAgentSkillAccess` by loading tenant installed registry entries, applying the same mode and selected-name filtering as `ResolveAgentSelectedSkills`, and returning both names and loader search directories. Keep `ResolveAgentSelectedSkills` as a compatibility wrapper that returns only names.

- [ ] **Step 3: Wire session runtime**

Update `configureSkillsFromAgent` so session runtime uses `ResolveAgentSkillAccess`, assigns `AgentConfig.AllowedSkills` from the returned names, and assigns `AgentConfig.SkillDirs` from the returned dirs.

- [ ] **Step 4: Verify runtime resolution**

Run: `go test ./internal/application/service -run 'TestSkillService_SyncAndResolveAgentSelectedSkills|TestSkillService_ResolveAgentSkillAccess' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/service/skill_service.go internal/application/service/skill_service_test.go internal/application/service/session_agent_qa.go
git commit -m "feat: resolve installed skill runtime dirs"
```

### Task 4: Tenant Install API

**Files:**
- Modify: `internal/handler/skill_handler.go`
- Modify: `internal/handler/skill_handler_test.go`
- Modify: `internal/router/router.go`
- Modify: `docs/agent-skills.md`

- [ ] **Step 1: Write handler tests**

Add a handler test for `POST /skills/install-local` behavior at the handler method level:
- valid JSON body `{ "package_path": "sample-skill" }` calls service with the tenant ID and returns installed Skill metadata
- missing `package_path` returns `400`

- [ ] **Step 2: Implement handler**

Add `InstallLocalSkillPackage` to `SkillHandler`, request/response DTOs, tenant ID extraction, and user ID extraction fallback to an empty string if unavailable.

- [ ] **Step 3: Register route**

Add `POST /skills/install-local` behind the admin RBAC guard in `RegisterSkillRoutes`.

- [ ] **Step 4: Update docs**

Document local package layout, `WEKNORA_SKILL_PACKAGES_DIR`, the install endpoint, and the current limits: no remote Hub download, no signature verification, no execution sandbox yet.

- [ ] **Step 5: Verify API layer**

Run: `go test ./internal/handler ./internal/router -run 'TestSkill|TestRegisterSkillRoutes' -count=1`

Expected: PASS or no router tests if absent; handler tests must pass.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/skill_handler.go internal/handler/skill_handler_test.go internal/router/router.go docs/agent-skills.md
git commit -m "feat: expose local skill install api"
```

### Task 5: Final Verification

**Files:**
- Verify all modified files.

- [ ] **Step 1: Focused verification**

Run:

```bash
go test ./internal/agent/skills ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -run 'TestSkill|TestManifest|TestLocal|TestAgent|TestSession|TestNonExistent' -count=1
```

Expected: PASS.

- [ ] **Step 2: Broader verification**

Run:

```bash
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -count=1
```

Expected: PASS.

- [ ] **Step 3: Review diff**

Run:

```bash
git status --short
git diff --stat HEAD
git diff --check
```

Expected: only intended files changed, and no whitespace errors.

- [ ] **Step 4: Commit final fixes if needed**

If final verification requires fixes, commit them with a focused message.
