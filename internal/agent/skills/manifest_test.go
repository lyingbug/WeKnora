package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePackageSkill(t *testing.T, dir, name, description string) {
	t.Helper()

	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nUse this skill carefully.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, SkillFileName), []byte(content), 0644))
}

func writePackageManifest(t *testing.T, dir, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, SkillManifestFileName), []byte(content), 0644))
}

func TestLoadSkillPackageManifest_LoadsValidPackage(t *testing.T) {
	dir := t.TempDir()
	writePackageSkill(t, dir, "sample-skill", "Sample skill")
	writePackageManifest(t, dir, `{
		"name": "sample-skill",
		"version": "1.2.3",
		"description": "Sample skill",
		"entrypoints": {
			"instructions": "SKILL.md"
		},
		"permissions": {
			"network": ["api.example.com"]
		}
	}`)

	got, err := LoadSkillPackageManifest(dir)

	require.NoError(t, err)
	assert.Equal(t, "sample-skill", got.Manifest.Name)
	assert.Equal(t, "1.2.3", got.Manifest.Version)
	assert.Equal(t, filepath.Join(dir, SkillFileName), got.InstructionsPath)
	assert.JSONEq(t, `{"network":["api.example.com"]}`, string(got.PermissionsJSON))
	assert.Contains(t, string(got.RawJSON), `"sample-skill"`)
	assert.Equal(t, "sample-skill", got.Skill.Name)
}

func TestLoadSkillPackageManifest_RejectsNameMismatch(t *testing.T) {
	dir := t.TempDir()
	writePackageSkill(t, dir, "frontmatter-name", "Sample skill")
	writePackageManifest(t, dir, `{
		"name": "manifest-name",
		"version": "1.0.0",
		"description": "Sample skill"
	}`)

	_, err := LoadSkillPackageManifest(dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestLoadSkillPackageManifest_RejectsEscapingEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writePackageSkill(t, dir, "sample-skill", "Sample skill")
	writePackageManifest(t, dir, `{
		"name": "sample-skill",
		"version": "1.0.0",
		"description": "Sample skill",
		"entrypoints": {
			"instructions": "../SKILL.md"
		}
	}`)

	_, err := LoadSkillPackageManifest(dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "within package")
}
