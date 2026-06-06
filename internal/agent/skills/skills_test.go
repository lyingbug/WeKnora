package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/sandbox"
)

func TestParseSkillFile(t *testing.T) {
	content := `---
name: test-skill
description: A test skill for unit testing purposes.
---
# Test Skill

This is the content of the test skill.

## Usage

Use this skill when testing.
`

	skill, err := ParseSkillFile(content)
	if err != nil {
		t.Fatalf("Failed to parse skill file: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", skill.Name)
	}

	if skill.Description != "A test skill for unit testing purposes." {
		t.Errorf("Expected description 'A test skill for unit testing purposes.', got '%s'", skill.Description)
	}

	if skill.Instructions == "" {
		t.Error("Expected instructions to be non-empty")
	}

	if !skill.Loaded {
		t.Error("Expected Loaded to be true after parsing")
	}

	t.Logf("Parsed skill: name=%s, description=%s, instructions_len=%d",
		skill.Name, skill.Description, len(skill.Instructions))
}

func TestSkillValidation(t *testing.T) {
	tests := []struct {
		name        string
		skillName   string
		description string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid skill",
			skillName:   "my-skill",
			description: "A valid skill",
			wantErr:     false,
		},
		{
			name:        "empty name",
			skillName:   "",
			description: "A skill",
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name:        "invalid characters in name",
			skillName:   "My Skill",
			description: "A skill",
			wantErr:     true,
			errContains: "lowercase letters",
		},
		{
			name:        "reserved word in name",
			skillName:   "my-claude-skill",
			description: "A skill",
			wantErr:     true,
			errContains: "reserved word",
		},
		{
			name:        "empty description",
			skillName:   "my-skill",
			description: "",
			wantErr:     true,
			errContains: "description is required",
		},
		{
			name:        "name too long",
			skillName:   "this-is-a-very-long-skill-name-that-exceeds-the-maximum-allowed-length-of-64-characters",
			description: "A skill",
			wantErr:     true,
			errContains: "exceeds maximum length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &Skill{
				Name:        tt.skillName,
				Description: tt.description,
			}

			err := skill.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errContains)
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoaderDiscoverSkills(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for loader testing.
---
# Test Skill

This is the test skill content.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create loader and discover skills
	loader := NewLoader([]string{tmpDir})
	metadata, err := loader.DiscoverSkills()
	if err != nil {
		t.Fatalf("Failed to discover skills: %v", err)
	}

	if len(metadata) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(metadata))
	}

	if metadata[0].Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", metadata[0].Name)
	}

	t.Logf("Discovered %d skills: %v", len(metadata), metadata[0].Name)
}

func TestLoaderLoadSkillInstructions(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for content loading.
---
# Test Skill

This is the main content.

## Section 1

More content here.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create loader and load skill instructions
	loader := NewLoader([]string{tmpDir})
	skill, err := loader.LoadSkillInstructions("test-skill")
	if err != nil {
		t.Fatalf("Failed to load skill instructions: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", skill.Name)
	}

	if skill.Instructions == "" {
		t.Error("Expected instructions to be non-empty")
	}

	if !skill.Loaded {
		t.Error("Expected Loaded to be true")
	}

	t.Logf("Loaded skill: name=%s, instructions_len=%d", skill.Name, len(skill.Instructions))
}

func TestLoaderLoadSkillFile(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory with additional files
	skillDir := filepath.Join(tmpDir, "test-skill")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill with additional files.
---
# Test Skill

See [GUIDE.md](GUIDE.md) for more info.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Write additional file
	guideContent := "# Guide\n\nThis is the guide content."
	if err := os.WriteFile(filepath.Join(skillDir, "GUIDE.md"), []byte(guideContent), 0644); err != nil {
		t.Fatalf("Failed to write GUIDE.md: %v", err)
	}

	// Write a script
	scriptContent := "#!/usr/bin/env python3\nprint('Hello from script')"
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.py"), []byte(scriptContent), 0644); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Create loader and discover skills first
	loader := NewLoader([]string{tmpDir})
	_, err = loader.DiscoverSkills()
	if err != nil {
		t.Fatalf("Failed to discover skills: %v", err)
	}

	// Load additional file
	file, err := loader.LoadSkillFile("test-skill", "GUIDE.md")
	if err != nil {
		t.Fatalf("Failed to load skill file: %v", err)
	}

	if file.Content != guideContent {
		t.Errorf("Expected guide content, got '%s'", file.Content)
	}

	if file.IsScript {
		t.Error("GUIDE.md should not be marked as script")
	}

	// Load script file
	scriptFile, err := loader.LoadSkillFile("test-skill", "scripts/hello.py")
	if err != nil {
		t.Fatalf("Failed to load script file: %v", err)
	}

	if !scriptFile.IsScript {
		t.Error("hello.py should be marked as script")
	}

	t.Logf("Loaded files: GUIDE.md=%d bytes, hello.py=%d bytes (isScript=%v)",
		len(file.Content), len(scriptFile.Content), scriptFile.IsScript)
}

func TestManagerIntegration(t *testing.T) {
	// Create a temporary skills directory
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test skill directory
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for manager integration testing.
---
# Test Skill

Integration test content.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create manager with config
	config := &ManagerConfig{
		SkillDirs:     []string{tmpDir},
		AllowedSkills: []string{}, // Allow all
		Enabled:       true,
	}

	manager := NewManager(config, nil) // No sandbox for this test

	// Initialize
	ctx := context.Background()
	if err := manager.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize manager: %v", err)
	}

	// Get all metadata
	metadata := manager.GetAllMetadata()
	if len(metadata) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(metadata))
	}

	// Load skill
	skill, err := manager.LoadSkill(ctx, "test-skill")
	if err != nil {
		t.Fatalf("Failed to load skill: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", skill.Name)
	}

	t.Logf("Manager integration test passed: %d skills discovered", len(metadata))
}

func TestManagerExecuteScriptWithOptionsPassesAllowNetwork(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, "web-fetcher")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}
	skillContent := `---
name: web-fetcher
description: A test skill for network execution.
---
# Web Fetcher
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "fetch.py"), []byte("print('ok')"), 0644); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	fakeSandbox := &capturingSandboxManager{sandboxType: sandbox.SandboxTypeDocker}
	manager := NewManager(&ManagerConfig{
		SkillDirs: []string{tmpDir},
		Enabled:   true,
	}, fakeSandbox)

	_, err = manager.ExecuteScriptWithOptions(
		context.Background(),
		"web-fetcher",
		"scripts/fetch.py",
		nil,
		"",
		ExecuteScriptOptions{
			AllowNetwork:          true,
			AllowedNetworkDomains: []string{"api.example.com"},
			MemoryLimit:           128 * 1024 * 1024,
			CPULimit:              0.75,
			Mounts: []sandbox.Mount{
				{
					HostPath:      "/tmp/weknora/session",
					ContainerPath: "/mnt/weknora/session",
					ReadOnly:      false,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("ExecuteScriptWithOptions failed: %v", err)
	}
	if fakeSandbox.lastConfig == nil {
		t.Fatal("Expected sandbox to receive execute config")
	}
	if !fakeSandbox.lastConfig.AllowNetwork {
		t.Fatal("Expected AllowNetwork to be passed to sandbox")
	}
	if len(fakeSandbox.lastConfig.AllowedNetworkDomains) != 1 || fakeSandbox.lastConfig.AllowedNetworkDomains[0] != "api.example.com" {
		t.Fatalf("Expected AllowedNetworkDomains to be passed to sandbox, got %v", fakeSandbox.lastConfig.AllowedNetworkDomains)
	}
	if fakeSandbox.lastConfig.MemoryLimit != 128*1024*1024 {
		t.Fatalf("Expected MemoryLimit to be passed to sandbox, got %d", fakeSandbox.lastConfig.MemoryLimit)
	}
	if fakeSandbox.lastConfig.CPULimit != 0.75 {
		t.Fatalf("Expected CPULimit to be passed to sandbox, got %f", fakeSandbox.lastConfig.CPULimit)
	}
	if len(fakeSandbox.lastConfig.Mounts) != 1 {
		t.Fatalf("Expected one mount to be passed to sandbox, got %d", len(fakeSandbox.lastConfig.Mounts))
	}
}

func TestManagerExecuteScriptWithOptionsRejectsMountsWithoutDocker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, "file-skill")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}
	skillContent := `---
name: file-skill
description: A test skill for file mounts.
---
# File Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "run.py"), []byte("print('ok')"), 0644); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	manager := NewManager(&ManagerConfig{
		SkillDirs: []string{tmpDir},
		Enabled:   true,
	}, &capturingSandboxManager{sandboxType: sandbox.SandboxTypeLocal})

	_, err = manager.ExecuteScriptWithOptions(
		context.Background(),
		"file-skill",
		"scripts/run.py",
		nil,
		"",
		ExecuteScriptOptions{
			Mounts: []sandbox.Mount{{HostPath: "/tmp/session", ContainerPath: "/mnt/weknora/session"}},
		},
	)
	if err == nil {
		t.Fatal("Expected mounts to require docker sandbox")
	}
	if !containsString(err.Error(), "require docker sandbox") {
		t.Fatalf("Expected docker sandbox error, got %v", err)
	}
}

type capturingSandboxManager struct {
	lastConfig  *sandbox.ExecuteConfig
	sandboxType sandbox.SandboxType
}

func (m *capturingSandboxManager) Execute(ctx context.Context, config *sandbox.ExecuteConfig) (*sandbox.ExecuteResult, error) {
	m.lastConfig = config
	return &sandbox.ExecuteResult{}, nil
}

func (m *capturingSandboxManager) Cleanup(context.Context) error {
	return nil
}

func (m *capturingSandboxManager) GetSandbox() sandbox.Sandbox {
	return nil
}

func (m *capturingSandboxManager) GetType() sandbox.SandboxType {
	if m.sandboxType == "" {
		return sandbox.SandboxTypeLocal
	}
	return m.sandboxType
}

func TestIsScript(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"script.py", true},
		{"script.sh", true},
		{"script.bash", true},
		{"script.js", true},
		{"script.ts", true},
		{"script.rb", true},
		{"README.md", false},
		{"data.json", false},
		{"config.yaml", false},
	}

	for _, tt := range tests {
		result := IsScript(tt.path)
		if result != tt.expected {
			t.Errorf("IsScript(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}
