package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// Tool name constant for execute_skill_script

var executeSkillScriptTool = BaseTool{
	name: ToolExecuteSkillScript,
	description: `Execute a script from a skill in a sandboxed environment.

## Usage
- Use this tool to run utility scripts bundled with a skill
- Scripts are executed in an isolated sandbox for security
- Only scripts from loaded skills can be executed

## When to Use
- When a skill's instructions reference a utility script (e.g., "Run scripts/analyze_form.py")
- When automation or data processing is needed as part of skill workflow
- For deterministic operations where script execution is more reliable than generating code

## Security
- Scripts run in a sandboxed environment with limited permissions
- Network access is disabled by default
- File access is restricted to the skill directory

## Returns
- Script stdout and stderr output
- Exit code indicating success (0) or failure (non-zero)`,
	schema: utils.GenerateSchema[ExecuteSkillScriptInput](),
}

// ExecuteSkillScriptInput defines the input parameters for the execute_skill_script tool
type ExecuteSkillScriptInput struct {
	SkillName  string   `json:"skill_name" jsonschema:"Name of the skill containing the script"`
	ScriptPath string   `json:"script_path" jsonschema:"Relative path to the script within the skill directory (e.g. scripts/analyze.py)"`
	Args       []string `json:"args,omitempty" jsonschema:"Optional command-line arguments to pass to the script. Note: if using --file flag, you must provide an actual file path that exists in the skill directory. If you have data in memory (not a file), use the 'input' parameter instead."`
	Input      string   `json:"input,omitempty" jsonschema:"Optional input data to pass to the script via stdin. Use this when you have data in memory (e.g. JSON string) that the script should process. This is equivalent to piping data: echo 'data' | python script.py"`
}

// ExecuteSkillScriptTool allows the agent to execute skill scripts in a sandbox
type ExecuteSkillScriptTool struct {
	BaseTool
	skillManager      *skills.Manager
	permissionChecker SkillPermissionChecker
}

type SkillPermissionChecker interface {
	ApprovedPermissions(ctx context.Context, tenantID uint64, skillName string) (types.JSON, error)
}

// NewExecuteSkillScriptTool creates a new execute_skill_script tool instance
func NewExecuteSkillScriptTool(skillManager *skills.Manager) *ExecuteSkillScriptTool {
	return &ExecuteSkillScriptTool{
		BaseTool:     executeSkillScriptTool,
		skillManager: skillManager,
	}
}

func (t *ExecuteSkillScriptTool) SetPermissionChecker(checker SkillPermissionChecker) {
	t.permissionChecker = checker
}

// Execute executes the execute_skill_script tool
func (t *ExecuteSkillScriptTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][ExecuteSkillScript] Execute started")

	// Parse input
	var input ExecuteSkillScriptInput
	if err := json.Unmarshal(args, &input); err != nil {
		logger.Errorf(ctx, "[Tool][ExecuteSkillScript] Failed to parse args: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse args: %v", err),
		}, nil
	}

	// Validate required fields
	if input.SkillName == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "skill_name is required",
		}, nil
	}

	if input.ScriptPath == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "script_path is required",
		}, nil
	}

	// Check if skill manager is available
	if t.skillManager == nil || !t.skillManager.IsEnabled() {
		return &types.ToolResult{
			Success: false,
			Error:   "Skills are not enabled",
		}, nil
	}

	policy, err := t.approvedExecutionPolicy(ctx, input.SkillName)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	execCtx := ctx
	cancel := func() {}
	if policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}

	// Execute the script in sandbox
	logger.Infof(ctx, "[Tool][ExecuteSkillScript] Executing script: %s/%s with args: %v, input length: %d",
		input.SkillName, input.ScriptPath, input.Args, len(input.Input))

	result, err := t.skillManager.ExecuteScriptWithOptions(
		execCtx,
		input.SkillName,
		input.ScriptPath,
		input.Args,
		input.Input,
		skills.ExecuteScriptOptions{
			AllowNetwork: policy.AllowNetwork,
			MemoryLimit:  policy.MemoryLimit,
			CPULimit:     policy.CPULimit,
		},
	)
	if err != nil {
		logger.Errorf(ctx, "[Tool][ExecuteSkillScript] Script execution failed: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Script execution failed: %v", err),
		}, nil
	}

	// Build output
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("=== Script Execution: %s/%s ===\n\n", input.SkillName, input.ScriptPath))

	if len(input.Args) > 0 {
		builder.WriteString(fmt.Sprintf("**Arguments**: %v\n", input.Args))
	}

	builder.WriteString(fmt.Sprintf("**Exit Code**: %d\n", result.ExitCode))
	builder.WriteString(fmt.Sprintf("**Duration**: %v\n\n", result.Duration))

	if result.Killed {
		builder.WriteString("**Warning**: Script was terminated (timeout or killed)\n\n")
	}

	if result.Stdout != "" {
		builder.WriteString("## Standard Output\n\n")
		builder.WriteString("```\n")
		builder.WriteString(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("```\n\n")
	}

	if result.Stderr != "" {
		builder.WriteString("## Standard Error\n\n")
		builder.WriteString("```\n")
		builder.WriteString(result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString("```\n\n")
	}

	if result.Error != "" {
		builder.WriteString("## Error\n\n")
		builder.WriteString(result.Error)
		builder.WriteString("\n")
	}

	// Determine success based on exit code
	success := result.IsSuccess()

	resultData := map[string]interface{}{
		"skill_name":  input.SkillName,
		"script_path": input.ScriptPath,
		"args":        input.Args,
		"exit_code":   result.ExitCode,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"duration_ms": result.Duration.Milliseconds(),
		"killed":      result.Killed,
	}

	logger.Infof(ctx, "[Tool][ExecuteSkillScript] Script completed with exit code: %d", result.ExitCode)

	return &types.ToolResult{
		Success: success,
		Output:  builder.String(),
		Data:    resultData,
		Error: func() string {
			if !success {
				if result.Error != "" {
					return result.Error
				}
				return fmt.Sprintf("Script exited with code %d", result.ExitCode)
			}
			return ""
		}(),
	}, nil
}

type skillExecutionPolicy struct {
	Timeout      time.Duration
	AllowNetwork bool
	MemoryLimit  int64
	CPULimit     float64
}

func (t *ExecuteSkillScriptTool) approvedExecutionPolicy(ctx context.Context, skillName string) (skillExecutionPolicy, error) {
	if t.permissionChecker == nil {
		return skillExecutionPolicy{}, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return skillExecutionPolicy{}, nil
	}
	permissions, err := t.permissionChecker.ApprovedPermissions(ctx, tenantID, skillName)
	if err != nil {
		return skillExecutionPolicy{}, fmt.Errorf("skill is not installed or enabled for this tenant: %s", skillName)
	}

	timeout, err := approvedComputeTimeout(permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	memoryLimit, err := approvedComputeMemoryLimit(permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	cpuLimit, err := approvedComputeCPULimit(permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	allowNetwork, err := approvedNetworkAllowed(permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	if err := rejectUnsupportedFilePermissions(permissions); err != nil {
		return skillExecutionPolicy{}, err
	}
	return skillExecutionPolicy{
		Timeout:      timeout,
		AllowNetwork: allowNetwork,
		MemoryLimit:  memoryLimit,
		CPULimit:     cpuLimit,
	}, nil
}

func approvedComputeTimeout(permissions types.JSON) (time.Duration, error) {
	seconds, ok, err := approvedComputeNumber(permissions, "timeout_seconds")
	if err != nil || !ok {
		return 0, err
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func approvedComputeMemoryLimit(permissions types.JSON) (int64, error) {
	megabytes, ok, err := approvedComputeNumber(permissions, "memory_mb")
	if err != nil || !ok {
		return 0, err
	}
	return int64(megabytes * 1024 * 1024), nil
}

func approvedComputeCPULimit(permissions types.JSON) (float64, error) {
	cpu, ok, err := approvedComputeNumber(permissions, "cpu")
	if err != nil || !ok {
		return 0, err
	}
	return cpu, nil
}

func approvedComputeNumber(permissions types.JSON, key string) (float64, bool, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return 0, false, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	computeRaw, ok := permissionsMap["compute"]
	if !ok {
		return 0, false, nil
	}
	compute, ok := computeRaw.(map[string]interface{})
	if !ok {
		return 0, false, fmt.Errorf("approved permissions compute must be an object")
	}
	rawValue, ok := compute[key]
	if !ok {
		return 0, false, nil
	}

	var number float64
	switch value := rawValue.(type) {
	case float64:
		number = value
	case int:
		number = float64(value)
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, false, fmt.Errorf("compute.%s must be a number", key)
		}
		number = parsed
	default:
		return 0, false, fmt.Errorf("compute.%s must be a number", key)
	}
	if number <= 0 {
		return 0, false, fmt.Errorf("compute.%s must be greater than zero", key)
	}
	return number, true, nil
}

func approvedNetworkAllowed(permissions types.JSON) (bool, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return false, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	networkRaw, ok := permissionsMap["network"]
	if !ok || networkRaw == nil {
		return false, nil
	}
	network, ok := networkRaw.([]interface{})
	if !ok {
		return false, fmt.Errorf("approved permissions network must be an array")
	}
	for _, rawDomain := range network {
		domain, ok := rawDomain.(string)
		if !ok {
			return false, fmt.Errorf("approved permissions network entries must be strings")
		}
		if strings.TrimSpace(domain) != "" {
			return true, nil
		}
	}
	return false, nil
}

func rejectUnsupportedFilePermissions(permissions types.JSON) error {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	filesRaw, ok := permissionsMap["files"]
	if !ok || filesRaw == nil {
		return nil
	}
	files, ok := filesRaw.([]interface{})
	if !ok {
		return fmt.Errorf("approved permissions files must be an array")
	}
	for _, rawScope := range files {
		scope, ok := rawScope.(string)
		if !ok {
			return fmt.Errorf("approved permissions files entries must be strings")
		}
		if strings.TrimSpace(scope) != "" {
			return fmt.Errorf("files permissions are not supported at runtime until sandbox file mounts are implemented")
		}
	}
	return nil
}

// Cleanup releases any resources
func (t *ExecuteSkillScriptTool) Cleanup(ctx context.Context) error {
	return nil
}
