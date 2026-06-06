package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/sandbox"
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
	mcpBroker         *SkillMCPBroker
}

type SkillPermissionChecker interface {
	ApprovedPermissions(ctx context.Context, tenantID uint64, skillName string) (types.JSON, error)
	ApprovedCredentials(ctx context.Context, tenantID uint64, skillName string) (types.JSON, error)
	ApprovedMCPBindings(ctx context.Context, tenantID uint64, skillName string) (types.JSON, error)
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

func (t *ExecuteSkillScriptTool) SetMCPBroker(broker *SkillMCPBroker) {
	t.mcpBroker = broker
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
	if policy.Cleanup != nil {
		defer policy.Cleanup()
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
			AllowNetwork:          policy.AllowNetwork,
			AllowedNetworkDomains: policy.AllowedNetworkDomains,
			MemoryLimit:           policy.MemoryLimit,
			CPULimit:              policy.CPULimit,
			Mounts:                policy.Mounts,
			Env:                   policy.Env,
			Metadata:              skillExecutionMetadata(ctx, input.SkillName),
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

func skillExecutionMetadata(ctx context.Context, skillName string) map[string]string {
	metadata := map[string]string{
		"skill_name": skillName,
	}
	if tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64); ok && tenantID > 0 {
		metadata["tenant_id"] = fmt.Sprintf("%d", tenantID)
	}
	if sessionTenantID, ok := ctx.Value(types.SessionTenantIDContextKey).(uint64); ok && sessionTenantID > 0 {
		metadata["session_tenant_id"] = fmt.Sprintf("%d", sessionTenantID)
	}
	if userID, ok := ctx.Value(types.UserIDContextKey).(string); ok && userID != "" {
		metadata["user_id"] = userID
	}
	if sessionID, ok := ctx.Value(types.SessionIDContextKey).(string); ok && sessionID != "" {
		metadata["session_id"] = sessionID
	}
	if requestID, ok := ctx.Value(types.RequestIDContextKey).(string); ok && requestID != "" {
		metadata["request_id"] = requestID
	}
	if execMeta, ok := ToolExecFromContext(ctx); ok {
		if execMeta.SessionID != "" {
			metadata["session_id"] = execMeta.SessionID
		}
		if execMeta.UserID != "" {
			metadata["user_id"] = execMeta.UserID
		}
		if execMeta.RequestID != "" {
			metadata["request_id"] = execMeta.RequestID
		}
		if execMeta.AssistantMessageID != "" {
			metadata["assistant_message_id"] = execMeta.AssistantMessageID
		}
		if execMeta.ToolCallID != "" {
			metadata["tool_call_id"] = execMeta.ToolCallID
		}
	}
	return metadata
}

type skillExecutionPolicy struct {
	Timeout               time.Duration
	AllowNetwork          bool
	AllowedNetworkDomains []string
	MemoryLimit           int64
	CPULimit              float64
	Mounts                []sandbox.Mount
	Env                   map[string]string
	Cleanup               func()
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
	networkDomains, err := approvedNetworkDomains(permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	if err := rejectUnsupportedRuntimePermissions(permissions); err != nil {
		return skillExecutionPolicy{}, err
	}
	mounts, env, err := approvedFileMounts(ctx, permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	networkEnv, networkCleanup, err := approvedNetworkProxyEnv(ctx, networkDomains)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	env = mergeEnv(env, networkEnv)
	credentialEnv, err := t.approvedCredentialEnv(ctx, inputTenantID(ctx), skillName, permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	env = mergeEnv(env, credentialEnv)
	mcpEnv, cleanup, err := t.approvedMCPBindingEnv(ctx, inputTenantID(ctx), skillName, permissions)
	if err != nil {
		return skillExecutionPolicy{}, err
	}
	env = mergeEnv(env, mcpEnv)
	return skillExecutionPolicy{
		Timeout:               timeout,
		AllowNetwork:          len(networkDomains) > 0 || len(mcpEnv) > 0,
		AllowedNetworkDomains: networkDomains,
		MemoryLimit:           memoryLimit,
		CPULimit:              cpuLimit,
		Mounts:                mounts,
		Env:                   env,
		Cleanup:               mergeCleanup(networkCleanup, cleanup),
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
	domains, err := approvedNetworkDomains(permissions)
	if err != nil {
		return false, err
	}
	return len(domains) > 0, nil
}

func approvedNetworkProxyEnv(ctx context.Context, domains []string) (map[string]string, func(), error) {
	if len(domains) == 0 {
		return nil, nil, nil
	}
	proxy := sandbox.NewEgressProxy(domains)
	if err := proxy.Start(ctx); err != nil {
		return nil, nil, err
	}
	proxyURL := dockerHostBrokerURL(proxy.URL())
	return map[string]string{
			"HTTP_PROXY":  proxyURL,
			"HTTPS_PROXY": proxyURL,
			"ALL_PROXY":   proxyURL,
			"NO_PROXY":    "localhost,127.0.0.1,host.docker.internal",
		}, func() {
			_ = proxy.Shutdown(context.Background())
		}, nil
}

func mergeCleanup(cleanups ...func()) func() {
	active := make([]func(), 0, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup != nil {
			active = append(active, cleanup)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func() {
		for i := len(active) - 1; i >= 0; i-- {
			active[i]()
		}
	}
}

func approvedNetworkDomains(permissions types.JSON) ([]string, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return nil, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	networkRaw, ok := permissionsMap["network"]
	if !ok || networkRaw == nil {
		return nil, nil
	}
	network, ok := networkRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("approved permissions network must be an array")
	}
	domains := make([]string, 0, len(network))
	for _, rawDomain := range network {
		domain, ok := rawDomain.(string)
		if !ok {
			return nil, fmt.Errorf("approved permissions network entries must be strings")
		}
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains, nil
}

func rejectUnsupportedRuntimePermissions(permissions types.JSON) error {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	_ = permissionsMap
	return nil
}

func rejectUnsupportedRuntimePermission(permissionsMap map[string]interface{}, key string) error {
	raw, ok := permissionsMap[key]
	if !ok || raw == nil {
		return nil
	}
	if permissionValueIsEmpty(raw) {
		return nil
	}
	return fmt.Errorf("%s permissions are not supported at runtime until sandbox/runtime binding is implemented", key)
}

func permissionValueIsEmpty(raw interface{}) bool {
	switch value := raw.(type) {
	case []interface{}:
		for _, item := range value {
			if !permissionValueIsEmpty(item) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		for _, item := range value {
			if !permissionValueIsEmpty(item) {
				return false
			}
		}
		return true
	case string:
		return strings.TrimSpace(value) == ""
	default:
		return false
	}
}

func (t *ExecuteSkillScriptTool) approvedCredentialEnv(
	ctx context.Context,
	tenantID uint64,
	skillName string,
	permissions types.JSON,
) (map[string]string, error) {
	names, err := approvedCredentialNames(permissions)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	if t.permissionChecker == nil || tenantID == 0 {
		return nil, fmt.Errorf("approved credentials require tenant context")
	}
	credentials, err := t.permissionChecker.ApprovedCredentials(ctx, tenantID, skillName)
	if err != nil {
		return nil, fmt.Errorf("approved credentials are not configured for skill: %s", skillName)
	}
	credentialMap, err := credentials.Map()
	if err != nil {
		return nil, fmt.Errorf("approved credentials are invalid JSON: %w", err)
	}
	env := make(map[string]string, len(names))
	for _, name := range names {
		if !isSafeEnvName(name) {
			return nil, fmt.Errorf("approved credential name is not a safe environment variable: %s", name)
		}
		rawValue, ok := credentialMap[name]
		if !ok || rawValue == nil {
			return nil, fmt.Errorf("approved credential %s is not configured", name)
		}
		value, ok := rawValue.(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("approved credential %s must be a non-empty string", name)
		}
		env[name] = value
	}
	return env, nil
}

func approvedCredentialNames(permissions types.JSON) ([]string, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return nil, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	raw, ok := permissionsMap["credentials"]
	if !ok || raw == nil || permissionValueIsEmpty(raw) {
		return nil, nil
	}
	values, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("approved permissions credentials must be an array")
	}
	names := make([]string, 0, len(values))
	for _, rawName := range values {
		name, ok := rawName.(string)
		if !ok {
			return nil, fmt.Errorf("approved permissions credentials entries must be strings")
		}
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func inputTenantID(ctx context.Context) uint64 {
	tenantID, _ := types.TenantIDFromContext(ctx)
	return tenantID
}

func mergeEnv(base map[string]string, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(extra))
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func isSafeEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !((r >= 'A' && r <= 'Z') || r == '_') {
			return false
		}
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func (t *ExecuteSkillScriptTool) approvedMCPBindingEnv(
	ctx context.Context,
	tenantID uint64,
	skillName string,
	permissions types.JSON,
) (map[string]string, func(), error) {
	names, err := approvedMCPNames(permissions)
	if err != nil || len(names) == 0 {
		return nil, nil, err
	}
	if t.permissionChecker == nil || tenantID == 0 {
		return nil, nil, fmt.Errorf("approved mcp bindings require tenant context")
	}
	bindings, err := t.permissionChecker.ApprovedMCPBindings(ctx, tenantID, skillName)
	if err != nil {
		return nil, nil, fmt.Errorf("approved mcp bindings are not configured for skill: %s", skillName)
	}
	bindingMap, err := bindings.Map()
	if err != nil {
		return nil, nil, fmt.Errorf("approved mcp bindings are invalid JSON: %w", err)
	}
	approved := make(map[string]string, len(names))
	for _, name := range names {
		rawServiceID, ok := bindingMap[name]
		if !ok || rawServiceID == nil {
			return nil, nil, fmt.Errorf("approved mcp binding %s is not configured", name)
		}
		serviceID, ok := rawServiceID.(string)
		if !ok || strings.TrimSpace(serviceID) == "" {
			return nil, nil, fmt.Errorf("approved mcp binding %s must be a non-empty service id", name)
		}
		approved[name] = serviceID
	}
	raw, err := json.Marshal(approved)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode approved mcp bindings: %w", err)
	}
	env := map[string]string{
		"WEKNORA_SKILL_MCP_BINDINGS": string(raw),
	}
	if t.mcpBroker == nil {
		return env, nil, nil
	}
	registration, err := t.mcpBroker.Register(ctx, SkillMCPBrokerRegistration{
		TenantID: tenantID,
		Bindings: approved,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to register skill mcp broker session: %w", err)
	}
	env["WEKNORA_SKILL_MCP_BROKER_URL"] = dockerHostBrokerURL(registration.URL)
	if publicURL := strings.TrimSpace(os.Getenv("WEKNORA_SKILL_MCP_BROKER_PUBLIC_URL")); publicURL != "" {
		env["WEKNORA_SKILL_MCP_BROKER_URL"] = publicURL
	}
	env["WEKNORA_SKILL_MCP_TOKEN"] = registration.Token
	return env, registration.Cleanup, nil
}

func dockerHostBrokerURL(rawURL string) string {
	return strings.Replace(rawURL, "http://127.0.0.1:", "http://host.docker.internal:", 1)
}

func approvedMCPNames(permissions types.JSON) ([]string, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return nil, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	raw, ok := permissionsMap["mcp"]
	if !ok || raw == nil || permissionValueIsEmpty(raw) {
		return nil, nil
	}
	values, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("approved permissions mcp must be an array")
	}
	names := make([]string, 0, len(values))
	for _, rawName := range values {
		name, ok := rawName.(string)
		if !ok {
			return nil, fmt.Errorf("approved permissions mcp entries must be strings")
		}
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

const skillSessionMountPath = "/mnt/weknora/session"

func approvedFileMounts(ctx context.Context, permissions types.JSON) ([]sandbox.Mount, map[string]string, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return nil, nil, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	filesRaw, ok := permissionsMap["files"]
	if !ok || filesRaw == nil || permissionValueIsEmpty(filesRaw) {
		return nil, nil, nil
	}
	files, ok := filesRaw.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("approved permissions files must be an array")
	}
	for _, rawScope := range files {
		scope, ok := rawScope.(string)
		if !ok {
			return nil, nil, fmt.Errorf("approved permissions files entries must be strings")
		}
		if strings.TrimSpace(scope) == "" {
			continue
		}
		if scope != "session-temp" {
			return nil, nil, fmt.Errorf("unsupported files permission scope: %s", scope)
		}
		hostPath, err := skillSessionTempDir(ctx)
		if err != nil {
			return nil, nil, err
		}
		return []sandbox.Mount{
				{
					HostPath:      hostPath,
					ContainerPath: skillSessionMountPath,
					ReadOnly:      false,
				},
			}, map[string]string{
				"WEKNORA_SKILL_SESSION_DIR": skillSessionMountPath,
			}, nil
	}
	return nil, nil, nil
}

func skillSessionTempDir(ctx context.Context) (string, error) {
	tenantID, _ := types.TenantIDFromContext(ctx)
	userID, ok := types.UserIDFromContext(ctx)
	if !ok {
		userID = "anonymous"
	}
	sessionID, ok := types.SessionIDFromContext(ctx)
	if !ok {
		if requestID, requestOK := types.RequestIDFromContext(ctx); requestOK {
			sessionID = requestID
		} else {
			sessionID = "default"
		}
	}
	root := strings.TrimSpace(os.Getenv("WEKNORA_SKILL_SESSION_DIR"))
	if root == "" {
		root = filepath.Join(os.TempDir(), "weknora", "skill-sessions")
	}
	dir := filepath.Join(
		root,
		fmt.Sprintf("tenant-%d", tenantID),
		safePathComponent(userID),
		safePathComponent(sessionID),
	)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create skill session temp dir: %w", err)
	}
	return dir, nil
}

func safePathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

// Cleanup releases any resources
func (t *ExecuteSkillScriptTool) Cleanup(ctx context.Context) error {
	return nil
}
