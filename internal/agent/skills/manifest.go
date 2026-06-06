package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const SkillManifestFileName = "skill.json"

var skillVersionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type SkillPackageManifest struct {
	Name          string                  `json:"name"`
	Version       string                  `json:"version"`
	Description   string                  `json:"description"`
	Author        string                  `json:"author,omitempty"`
	License       string                  `json:"license,omitempty"`
	Entrypoints   SkillPackageEntrypoints `json:"entrypoints,omitempty"`
	Runtime       SkillPackageRuntime     `json:"runtime,omitempty"`
	Permissions   map[string]any          `json:"permissions,omitempty"`
	Compatibility map[string]string       `json:"compatibility,omitempty"`
}

type SkillPackageEntrypoints struct {
	Instructions string `json:"instructions,omitempty"`
}

type SkillPackageRuntime struct {
	Type           string  `json:"type,omitempty"`
	Image          string  `json:"image,omitempty"`
	TimeoutSeconds int     `json:"timeout_seconds,omitempty"`
	MemoryMB       int     `json:"memory_mb,omitempty"`
	CPU            float64 `json:"cpu,omitempty"`
}

type LoadedSkillPackageManifest struct {
	Manifest         SkillPackageManifest
	RawJSON          []byte
	PermissionsJSON  []byte
	PackageDir       string
	InstructionsPath string
	Skill            *Skill
}

func LoadSkillPackageManifest(packageDir string) (*LoadedSkillPackageManifest, error) {
	absDir, err := filepath.Abs(packageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve skill package directory: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(absDir, SkillManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", SkillManifestFileName, err)
	}

	var manifest SkillPackageManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", SkillManifestFileName, err)
	}
	if err := validateSkillPackageManifest(&manifest); err != nil {
		return nil, err
	}

	instructions := manifest.Entrypoints.Instructions
	if instructions == "" {
		instructions = SkillFileName
	}
	instructionsPath, err := containedPath(absDir, instructions)
	if err != nil {
		return nil, fmt.Errorf("instructions entrypoint must be within package: %w", err)
	}

	content, err := os.ReadFile(instructionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read instructions entrypoint: %w", err)
	}
	skill, err := ParseSkillFile(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse instructions entrypoint: %w", err)
	}
	if skill.Name != manifest.Name {
		return nil, fmt.Errorf("manifest name %q does not match instructions name %q", manifest.Name, skill.Name)
	}
	skill.BasePath = absDir
	skill.FilePath = instructionsPath

	permissionsJSON := []byte("{}")
	if len(manifest.Permissions) > 0 {
		permissionsJSON, err = json.Marshal(manifest.Permissions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal permissions: %w", err)
		}
	}

	return &LoadedSkillPackageManifest{
		Manifest:         manifest,
		RawJSON:          raw,
		PermissionsJSON:  permissionsJSON,
		PackageDir:       absDir,
		InstructionsPath: instructionsPath,
		Skill:            skill,
	}, nil
}

func validateSkillPackageManifest(manifest *SkillPackageManifest) error {
	if manifest.Name == "" {
		return errors.New("skill manifest name is required")
	}
	if manifest.Version == "" {
		return errors.New("skill manifest version is required")
	}
	if len(manifest.Version) > MaxNameLength {
		return fmt.Errorf("skill manifest version exceeds maximum length of %d characters", MaxNameLength)
	}
	if !skillVersionPattern.MatchString(manifest.Version) {
		return errors.New("skill manifest version must contain only letters, numbers, dots, underscores, and hyphens")
	}
	if manifest.Description == "" {
		return errors.New("skill manifest description is required")
	}

	probe := &Skill{Name: manifest.Name, Description: manifest.Description}
	if err := probe.Validate(); err != nil {
		return fmt.Errorf("skill manifest validation failed: %w", err)
	}
	return nil
}

func containedPath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute paths are not allowed")
	}
	cleanRoot := filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(cleanRoot, rel))
	if candidate == cleanRoot || strings.HasPrefix(candidate, cleanRoot+string(os.PathSeparator)) {
		return candidate, nil
	}
	return "", fmt.Errorf("%s escapes %s", rel, root)
}
