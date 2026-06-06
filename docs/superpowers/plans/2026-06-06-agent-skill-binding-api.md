# Agent Skill Binding API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Agent-level Skill binding APIs that keep `CustomAgentConfig` and `agent_skill_bindings` synchronized.

**Architecture:** Expose `GET /api/v1/agents/:id/skills` and `PUT /api/v1/agents/:id/skills` from `CustomAgentHandler`. The service updates the Agent's `skills_selection_mode` and `selected_skills`, persists the Agent, then reuses existing Skill binding sync logic. Runtime continues to resolve from Agent config plus tenant installed/enabled Skills, so there is one source of behavioral truth.

**Tech Stack:** Go, Gin, existing `CustomAgentService`, `SkillService`, and `agent_skill_bindings`.

---

### Task 1: Custom Agent Service Methods

**Files:**
- Modify: `internal/types/interfaces/custom_agent.go`
- Modify: `internal/application/service/custom_agent.go`

- [ ] **Step 1: Add interface methods**

Add:
- `GetAgentSkillConfig(ctx, agentID) (*types.AgentSkillConfig, error)`
- `UpdateAgentSkillConfig(ctx, agentID, mode string, selected []string) (*types.AgentSkillConfig, error)`

- [ ] **Step 2: Add type**

Add `types.AgentSkillConfig` with:
- `AgentID`
- `Mode`
- `SelectedSkills`

- [ ] **Step 3: Implement service**

Implementation must:
- load the Agent using existing tenant context resolution
- reject unsupported modes except `none`, `selected`, `all`, and empty
- normalize empty mode to `none`
- update `CustomAgent.Config.SkillsSelectionMode`
- set `SelectedSkills` only for `selected`, and clear it for other modes
- persist via repository
- call existing `syncAgentSkillBindings`

### Task 2: Handler and Routes

**Files:**
- Modify: `internal/handler/custom_agent.go`
- Modify: `internal/router/router.go`
- Modify: `internal/handler/custom_agent_skill_test.go`

- [ ] **Step 1: Add handler tests**

Add tests for:
- `GET /agents/:id/skills` returns current mode and selected skills
- `PUT /agents/:id/skills` sends mode and selected skills to service
- invalid mode returns `400`

- [ ] **Step 2: Implement handlers**

Add:
- `GetAgentSkills`
- `UpdateAgentSkills`

- [ ] **Step 3: Register routes**

Add:
- `GET /agents/:id/skills` as Viewer+
- `PUT /agents/:id/skills` as OwnedAgentOrAdmin+

### Task 3: Docs and Verification

**Files:**
- Modify: `docs/agent-skills.md`

- [ ] **Step 1: Update docs**

Document Agent binding endpoints and clarify that tenant install enabled state still gates runtime availability.

- [ ] **Step 2: Run focused tests**

```bash
go test ./internal/handler ./internal/application/service -run 'TestCustomAgent.*Skill|TestAgentSkill|TestSkill' -count=1
```

- [ ] **Step 3: Run broader tests**

```bash
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -count=1
```
