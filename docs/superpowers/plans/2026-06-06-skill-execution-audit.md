# Skill Execution Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist one audit record for each `execute_skill_script` tool call.

**Architecture:** Keep script execution unchanged. `execute_skill_script` already returns structured `skill_name`, `script_path`, `exit_code`, and `duration_ms`; `AgentEngine` owns session/message/tool-call context, so it records a `skill_execution_runs` row after the tool call completes. Recording is best-effort: database failures are logged and never fail the user-facing tool call.

**Tech Stack:** Go, GORM, existing `SkillExecutionRun` table, AgentEngine tool execution pipeline.

---

### Task 1: Repository

**Files:**
- Modify: `internal/types/interfaces/skill.go`
- Modify: `internal/application/repository/skill_execution_run_repository.go`
- Modify: `internal/application/repository/skill_execution_run_repository_test.go`

- [ ] **Step 1: Add interface**

Add `SkillExecutionRunRepository` with:
- `CreateSkillExecutionRun(ctx, run)`
- `ListSkillExecutionRuns(ctx, tenantID, limit)`

- [ ] **Step 2: Implement repository**

Use GORM against `skill_execution_runs`, ordered by `created_at DESC` for listing.

- [ ] **Step 3: Verify repository**

Run:

```bash
go test ./internal/application/repository -run TestSkillExecutionRunRepository -count=1
```

### Task 2: Agent Engine Recorder

**Files:**
- Modify: `internal/agent/engine.go`
- Modify: `internal/agent/act.go`
- Modify: `internal/agent/skill_audit_test.go`

- [ ] **Step 1: Add recorder interface**

Add an agent-level `SkillExecutionRecorder` interface with `RecordSkillExecution(ctx, run) error`.

- [ ] **Step 2: Add setter**

Add `SetSkillExecutionRecorder` on `AgentEngine`.

- [ ] **Step 3: Record after tool completion**

In `runToolCall`, when tool name is `execute_skill_script`, extract:
- `skill_name` and `script_path` from parsed args or result data
- `duration_ms` from result data or measured tool duration
- status `success` or `failed`
- error summary from `ToolResult.Error`

Use tenant/user from context and session/message/tool IDs from the method parameters.

### Task 3: Service Wiring

**Files:**
- Modify: `internal/application/service/agent_service.go`
- Modify: `internal/container/container.go`

- [ ] **Step 1: Add service adapter**

Inject `SkillExecutionRunRepository` into `agentService` and pass it to the engine.

- [ ] **Step 2: Register repository**

Add provider in container.

### Task 4: Final Verification

Run:

```bash
go test ./internal/agent ./internal/application/service ./internal/application/repository ./internal/container -run 'TestSkill|TestAgent|TestSkillExecutionRun' -count=1
go test ./internal/agent/skills ./internal/agent/tools ./internal/application/service ./internal/application/repository ./internal/handler ./internal/container -count=1
git diff --check
```
