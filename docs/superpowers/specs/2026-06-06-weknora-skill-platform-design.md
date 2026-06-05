# WeKnora Skill Platform Design

## Background

WeKnora currently supports Agent Skills as local directories under preloaded skill paths. A Skill is primarily a `SKILL.md` instruction file with optional resources and scripts. Agents receive lightweight Skill metadata in the system prompt, call `read_skill` to load detailed instructions, and call `execute_skill_script` to run bundled scripts through the sandbox manager.

This model works for built-in capabilities and single-instance deployments, but it is too weak for a web-based, multi-tenant WeKnora service. It lacks lifecycle management, tenant-scoped installation, versioning, permission review, runtime quota, and strong isolation.

The target design upgrades Skills from "local instruction folders" into a governed, installable, auditable capability package system.

## Goals

- Support online Skill installation from an official or private Skill Hub.
- Keep Skill enablement explicit and scoped to tenants and agents.
- Preserve progressive disclosure so prompt cost remains low.
- Run executable Skill code in isolated, short-lived cloud sandboxes.
- Provide permission review, audit logs, quotas, and upgrade/rollback paths.
- Reuse the existing `SKILL.md`, `read_skill`, `execute_skill_script`, and sandbox abstractions where possible.

## Non-Goals

- Do not let ordinary users install arbitrary executable Skills without tenant-admin approval.
- Do not introduce long-lived per-user sandboxes as the default runtime model.
- Do not expose all installed Skills to every Agent automatically.
- Do not make Skill Hub a hard dependency for self-hosted deployments.

## Recommended Model

Use a three-layer model:

1. **Tenant-level installation**
   Tenant admins install, update, disable, or remove Skills from a trusted source.

2. **Agent-level enablement**
   Agent creators choose which installed Skills an Agent may use. Existing `none / selected / all` can evolve into `none / selected / recommended`, where `all` is avoided for SaaS governance.

3. **Session/tool-call-level execution**
   When a Skill script runs, WeKnora creates a short-lived sandbox for that specific tool call. Optional session workspaces can persist temporary files across calls with a TTL.

This separates ownership cleanly: tenants govern what is available, agents govern what is exposed to the model, and the runtime governs what code can actually do.

## Skill Package Format

A Skill package should contain:

```text
skill-name/
  skill.json
  SKILL.md
  scripts/
  resources/
  templates/
  tests/
```

`SKILL.md` remains the model-facing instruction file. `skill.json` becomes the platform-facing manifest:

```json
{
  "name": "document-analyzer",
  "version": "1.2.0",
  "description": "Analyze documents and extract structured insights.",
  "author": "weknora",
  "license": "Apache-2.0",
  "entrypoints": {
    "instructions": "SKILL.md"
  },
  "runtime": {
    "type": "python",
    "image": "weknora/skill-python:3.12",
    "timeout_seconds": 60,
    "memory_mb": 512,
    "cpu": 1
  },
  "permissions": {
    "network": [],
    "knowledge": ["read"],
    "files": ["session-temp"],
    "credentials": [],
    "mcp": []
  },
  "compatibility": {
    "weknora": ">=0.5.0"
  }
}
```

The existing frontmatter can remain for backward compatibility, but the registry should treat `skill.json` as the source of truth once a Skill is installed.

## Data Model

Add persistent registry and binding tables:

- `skills`
  Canonical Skill records: name, version, source, digest, manifest, status, created_at.

- `tenant_skill_installs`
  Tenant-scoped installation records: tenant_id, skill_id, enabled, installed_by, installed_at, approved_permissions, upgrade policy.

- `agent_skill_bindings`
  Agent-scoped enablement records: agent_id, tenant_id, skill_id, enabled, config overrides.

- `skill_execution_runs`
  Runtime audit records: tenant_id, user_id, agent_id, session_id, message_id, tool_call_id, skill_id, script path, status, duration, resource usage, error summary.

- `skill_hub_sources`
  Optional source registry for official Hub, private Hub, Git repositories, or local packages.

## API Surface

Recommended API groups:

- `GET /api/v1/skill-hub/skills`
  Browse available Skills from configured Hub sources.

- `POST /api/v1/tenant/skills/install`
  Install a Skill into the current tenant after permission review.

- `GET /api/v1/tenant/skills`
  List installed Skills available to the tenant.

- `PATCH /api/v1/tenant/skills/:id`
  Enable, disable, upgrade policy, or permission changes.

- `GET /api/v1/agents/:id/skills`
  List Skills enabled for an Agent.

- `PUT /api/v1/agents/:id/skills`
  Replace Agent Skill bindings.

- `GET /api/v1/skill-runs`
  Audit Skill executions.

The existing `GET /api/v1/skills` can remain temporarily as a compatibility endpoint backed by tenant-installed Skills instead of only preloaded directories.

## Runtime Architecture

The current `SkillDirs -> Loader -> Manager` flow should evolve into:

```text
Skill Hub / Local Packages
        |
        v
Skill Registry
        |
        v
Tenant Install Resolver
        |
        v
Agent Skill Policy
        |
        v
Prompt Metadata + read_skill
        |
        v
Runtime Executor + Sandbox
```

The Agent should not know where Skill files live. It should only see the Skills allowed by the current tenant, Agent, and session policy.

`read_skill` should load instructions and resources from the installed package store.

`execute_skill_script` should call a runtime executor that:

- validates the Skill is installed and enabled;
- validates the script path belongs to the package;
- evaluates declared permissions;
- creates or reuses a TTL session workspace when needed;
- launches a short-lived sandbox;
- captures stdout, stderr, exit code, duration, and resource usage;
- writes an audit record.

## Sandbox Strategy

For the web service, the default sandbox should be cloud-side and short-lived.

Recommended phases:

1. **MVP**
   Use Docker or Kubernetes Jobs with per-call containers, read-only package mounts, no network by default, CPU/memory/time limits, and temporary writable workspace.

2. **Production SaaS**
   Add stronger isolation with gVisor, Kata Containers, or Firecracker. Add namespace-level quotas, network egress policy, and execution queueing.

3. **Enterprise**
   Support private sandbox pools, tenant-specific base images, private package mirrors, and stricter allowlists.

Avoid default long-lived per-user sandboxes. They are expensive, harder to clean up, and increase cross-session state risk. Use TTL session workspaces only for Skills that truly need continuity.

## Permission and Approval

Skill permissions are declared in the manifest and approved at install time. Runtime should enforce the approved subset, not the package's requested superset.

Permission categories:

- `network`: domain allowlist or disabled.
- `knowledge`: read/write access to selected knowledge bases.
- `files`: none, session temporary files, uploaded files, or generated artifacts.
- `credentials`: named credential scopes, injected only at runtime.
- `mcp`: allowed MCP services and tools.
- `compute`: timeout, memory, CPU, concurrency.

High-risk actions should integrate with the existing tool approval mechanism rather than inventing a separate approval UX.

## Frontend UX

Settings should gain a tenant-level Skill page:

- installed Skills;
- available Hub Skills;
- permission review during install;
- version and upgrade status;
- disable/remove actions;
- execution audit entry points.

Agent editor should show only tenant-installed Skills:

- no Skills;
- selected Skills;
- recommended Skills, if WeKnora later adds recommendation metadata;
- per-Skill permission summary.

The chat stream can continue showing `read_skill` and `execute_skill_script` events, but should include clearer labels, run status, and approval prompts for sensitive actions.

## Migration Plan

Phase 1: Registry-backed preloaded Skills

- Add registry tables.
- Import `skills/preloaded` into registry on startup or migration.
- Back `GET /api/v1/skills` with the registry.
- Preserve current Agent config and sandbox behavior.

Phase 2: Tenant installs and Agent bindings

- Add tenant install APIs.
- Add Agent Skill binding APIs.
- Update Agent editor to select from tenant-installed Skills.
- Convert existing `SkillsSelectionMode` and `SelectedSkills` into bindings.

Phase 3: Package installation

- Add local package upload or Git URL install.
- Add manifest validation, digest calculation, and version records.
- Add permission review.

Phase 4: Cloud runtime executor

- Replace direct filesystem execution with package-store resolution.
- Add short-lived sandbox jobs.
- Add session workspaces, quotas, and audit records.

Phase 5: Skill Hub ecosystem

- Add official Hub browsing.
- Add private Hub sources.
- Add signing, certification, compatibility checks, upgrades, and rollback.

## Risks and Mitigations

- **Prompt injection through Skill instructions**
  Keep tenant/admin trust boundaries clear, require package review, and show source/version in admin UI.

- **Arbitrary code execution**
  Use short-lived cloud sandboxes, no network by default, resource limits, non-root execution, read-only package mounts, and runtime permission checks.

- **Permission drift after upgrades**
  Treat permission expansion as a new approval event. Do not auto-enable expanded permissions.

- **Token bloat**
  Keep progressive disclosure. Inject only metadata for Agent-enabled Skills.

- **Operational cost**
  Use per-call sandbox jobs first, add queueing and quotas, and only enable session workspaces when required.

- **Backward compatibility**
  Keep `SKILL.md`, `read_skill`, `execute_skill_script`, and preloaded Skill import during migration.

## Open Decisions

- Whether official WeKnora Skill Hub should be centralized, self-hostable, or both from day one.
- Whether `all` should remain available for self-hosted deployments while being hidden in hosted SaaS.
- Which sandbox backend should be the first production target: Kubernetes Job, gVisor, Kata, or Firecracker.
- Whether Skill package signing is required for MVP or introduced before public Hub launch.

## Recommendation

Build the platform in this order:

1. Registry-backed preloaded Skills.
2. Tenant installs and Agent bindings.
3. Package installation with permission review.
4. Cloud sandbox runtime.
5. Public/private Skill Hub ecosystem.

This path keeps the current Agent behavior working while creating the governance and isolation layers needed for a real multi-tenant web service.
