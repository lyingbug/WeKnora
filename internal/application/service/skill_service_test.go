package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.SkillRegistryEntry{},
		&types.TenantSkillInstall{},
		&types.AgentSkillBinding{},
		&types.TenantSkillCredential{},
		&types.TenantSkillMCPBinding{},
		&types.MCPService{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_version ON skills(name, version)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_installs_tenant_skill ON tenant_skill_installs(tenant_id, skill_id)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_skill_bindings_tenant_agent_skill ON agent_skill_bindings(tenant_id, agent_id, skill_id)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_credentials_tenant_skill ON tenant_skill_credentials(tenant_id, skill_id)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_mcp_bindings_tenant_skill_name ON tenant_skill_mcp_bindings(tenant_id, skill_id, mcp_name)").Error)

	return db
}

func writeTestSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()

	skillDir := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))
}

func writeTestSkillPackage(t *testing.T, root, dir, name, version, description string, permissions map[string]any) string {
	t.Helper()

	skillDir := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))

	manifest := map[string]any{
		"name":        name,
		"version":     version,
		"description": description,
		"entrypoints": map[string]any{
			"instructions": "SKILL.md",
		},
		"permissions": permissions,
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "skill.json"), raw, 0644))

	return skillDir
}

func buildTestSkillPackageZip(t *testing.T, name, version string, permissions map[string]any) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	manifest := map[string]any{
		"name":        name,
		"version":     version,
		"description": "Sample skill",
		"entrypoints": map[string]any{
			"instructions": "SKILL.md",
		},
		"permissions": permissions,
	}
	rawManifest, err := json.Marshal(manifest)
	require.NoError(t, err)

	files := map[string]string{
		filepath.ToSlash(filepath.Join(name, "skill.json")): string(rawManifest),
		filepath.ToSlash(filepath.Join(name, "SKILL.md")):   "---\nname: " + name + "\ndescription: Sample skill\n---\n\n# " + name + "\n",
	}
	for path, content := range files {
		file, err := writer.Create(path)
		require.NoError(t, err)
		_, err = file.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

func signTestSkillHubArchive(t *testing.T, archive []byte) (string, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	digest := sha256.Sum256(archive)
	signature := ed25519.Sign(priv, digest[:])
	raw, err := json.Marshal(map[string]string{
		"publisher": "test-publisher",
		"algorithm": "ed25519-sha256",
		"signature": base64.StdEncoding.EncodeToString(signature),
	})
	require.NoError(t, err)
	return "test-publisher:" + base64.StdEncoding.EncodeToString(pub), raw
}

func TestSkillService_ImportPreloadedSkills_ImportsIntoRegistryAndListsRegistryEntries(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.ImportPreloadedSkills(ctx))

	entry, err := repo.GetActiveSkillByNameVersion(ctx, "alpha", types.DefaultSkillVersion)
	require.NoError(t, err)
	assert.Equal(t, "preloaded-alpha-0-0-0", entry.ID)
	assert.Equal(t, types.SkillSourceTypePreloaded, entry.SourceType)
	assert.Equal(t, types.SkillStatusActive, entry.Status)
	assert.True(t, entry.IsBuiltin)
	assert.Equal(t, filepath.Join(tempDir, "alpha-dir"), entry.SourceURI)
	assert.NotEmpty(t, entry.Digest)
	assert.JSONEq(t, `{}`, entry.Manifest.ToString())

	require.NoError(t, os.RemoveAll(filepath.Join(tempDir, "alpha-dir")))

	got, err := svc.ListPreloadedSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "Alpha skill", got[0].Description)
}

func TestSkillService_ListPreloadedSkills_FallsBackToFilesystemWhenRegistryIsEmpty(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	got, err := svc.ListPreloadedSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta", got[0].Name)
	assert.Equal(t, "Beta skill", got[0].Description)
}

func TestSkillRegistryID_FitsDatabaseColumn(t *testing.T) {
	id := skillRegistryID(
		types.SkillSourceTypePreloaded,
		"very-long-skill-name-that-can-still-be-valid-but-would-overflow-the-id-column",
		types.DefaultSkillVersion,
	)

	require.LessOrEqual(t, len(id), 64)
	assert.Contains(t, id, "-")
}

func TestSkillRegistryID_UsesStableHashFallbackForNonASCIIName(t *testing.T) {
	first := skillRegistryID(types.SkillSourceTypePreloaded, "数据处理器", types.DefaultSkillVersion)
	second := skillRegistryID(types.SkillSourceTypePreloaded, "文档分析器", types.DefaultSkillVersion)

	require.NotEqual(t, first, second)
	assert.Regexp(t, `^preloaded-[a-f0-9]{12}-0-0-0$`, first)
	assert.Regexp(t, `^preloaded-[a-f0-9]{12}-0-0-0$`, second)
}

func TestSkillService_ImportPreloadedSkills_ImportsNonASCIINamesWithoutIDCollision(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "data-processor", "数据处理器", "Data processing skill")
	writeTestSkill(t, tempDir, "document-analyzer", "文档分析器", "Document analysis skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.ImportPreloadedSkills(ctx))

	first, err := repo.GetActiveSkillByNameVersion(ctx, "数据处理器", types.DefaultSkillVersion)
	require.NoError(t, err)
	second, err := repo.GetActiveSkillByNameVersion(ctx, "文档分析器", types.DefaultSkillVersion)
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
	assert.Regexp(t, `^preloaded-[a-f0-9]{12}-0-0-0$`, first.ID)
	assert.Regexp(t, `^preloaded-[a-f0-9]{12}-0-0-0$`, second.ID)
}

func TestSkillService_EnsureTenantPreloadedSkillInstalls_ListsTenantSkills(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	got, err := svc.ListTenantSkills(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "beta", got[1].Name)
}

func TestSkillService_EnsureTenantPreloadedSkillInstalls_DoesNotReenableDisabledInstall(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))
	require.NoError(t, db.Model(&types.TenantSkillInstall{}).
		Where("tenant_id = ? AND skill_id = ?", 10, "preloaded-alpha-0-0-0").
		Update("enabled", false).Error)
	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	got, err := svc.ListTenantSkills(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, got)

	installed, err := repo.ListTenantInstalledSkills(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, installed)
}

func TestSkillService_TenantSkillLifecycle_DisableHidesFromCompatibilityAndRuntime(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	installs, err := svc.ListTenantSkillInstalls(ctx, 10)
	require.NoError(t, err)
	require.Len(t, installs, 2)
	assert.Equal(t, "alpha", installs[0].Name)
	assert.True(t, installs[0].Enabled)
	assert.Equal(t, "beta", installs[1].Name)
	assert.True(t, installs[1].Enabled)

	require.NoError(t, svc.SetTenantSkillEnabled(ctx, 10, "preloaded-alpha-0-0-0", false))

	installs, err = svc.ListTenantSkillInstalls(ctx, 10)
	require.NoError(t, err)
	require.Len(t, installs, 2)
	assert.False(t, installs[0].Enabled)
	assert.True(t, installs[1].Enabled)

	metadata, err := svc.ListTenantSkills(ctx, 10)
	require.NoError(t, err)
	require.Len(t, metadata, 1)
	assert.Equal(t, "beta", metadata[0].Name)

	names, dirs, err := svc.ResolveAgentSkillAccess(ctx, 10, "agent-a", "all", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, names)
	assert.Equal(t, []string{tempDir}, dirs)
}

func TestSkillService_SyncAndResolveAgentSelectedSkills(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)
	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	require.NoError(t, svc.SyncAgentSkillBindings(ctx, 10, "agent-a", "selected", []string{"beta", "missing"}))
	selected, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "selected", []string{"beta", "missing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, selected)

	all, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "all", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, all)

	require.NoError(t, svc.SyncAgentSkillBindings(ctx, 10, "agent-a", "none", []string{"beta"}))
	selected, err = svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "selected", []string{"beta"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, selected)

	none, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "none", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestSkillService_InstallLocalSkillPackage(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	packageDir := writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": []string{"api.example.com"},
	})

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	entry, err := svc.InstallLocalSkillPackage(ctx, 10, "sample-skill", "user-a")
	require.NoError(t, err)

	assert.Equal(t, "local-sample-skill-1-2-3", entry.ID)
	assert.Equal(t, "sample-skill", entry.Name)
	assert.Equal(t, "1.2.3", entry.Version)
	assert.Equal(t, types.SkillSourceTypeLocal, entry.SourceType)
	assert.Equal(t, packageDir, entry.SourceURI)
	assert.Equal(t, types.SkillStatusActive, entry.Status)
	assert.False(t, entry.IsBuiltin)
	assert.NotEmpty(t, entry.Digest)
	assert.JSONEq(t, `{"description":"Sample skill","entrypoints":{"instructions":"SKILL.md"},"name":"sample-skill","permissions":{"network":["api.example.com"]},"version":"1.2.3"}`, entry.Manifest.ToString())

	var install types.TenantSkillInstall
	require.NoError(t, db.Where("tenant_id = ? AND skill_id = ?", 10, entry.ID).First(&install).Error)
	assert.True(t, install.Enabled)
	assert.Equal(t, "user-a", install.InstalledBy)
	assert.JSONEq(t, `{"network":["api.example.com"]}`, install.ApprovedPermissions.ToString())

	outside := filepath.Join(t.TempDir(), "sample-skill")
	_, err = svc.InstallLocalSkillPackage(ctx, 10, outside, "user-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "within skill packages directory")
}

func TestSkillService_InstallLocalSkillPackageWithPermissions_StoresApprovedSubset(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": []string{"api.example.com"},
		"files":   []string{"session-temp"},
	})

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	entry, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"network":[]}`),
	)
	require.NoError(t, err)

	var install types.TenantSkillInstall
	require.NoError(t, db.Where("tenant_id = ? AND skill_id = ?", 10, entry.ID).First(&install).Error)
	assert.JSONEq(t, `{"network":[]}`, install.ApprovedPermissions.ToString())

	_, err = svc.InstallLocalSkillPackageWithPermissions(ctx, 10, "sample-skill", "user-a", types.JSON(`[]`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved permissions must be a JSON object")
}

func TestSkillService_InstallLocalSkillPackageWithPermissions_RejectsUnrequestedPermissionKeys(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": []string{"api.example.com"},
	})

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	_, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"network":[],"credentials":["OPENAI_API_KEY"]}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved permission credentials was not requested by skill manifest")
}

func TestSkillService_InstallLocalSkillPackageWithPermissions_RejectsUnrequestedNetworkScope(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": []string{"api.example.com"},
	})

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	_, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"network":["evil.example.com"]}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved network scope evil.example.com was not requested by skill manifest")
}

func TestSkillService_InstallLocalSkillPackageWithPermissions_RejectsExpandedComputeLimits(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"compute": map[string]any{
			"timeout_seconds": 30,
			"memory_mb":       256,
			"cpu":             1,
		},
	})

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	_, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"compute":{"timeout_seconds":60,"memory_mb":256,"cpu":1}}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved compute.timeout_seconds exceeds requested value")

	_, err = svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"compute":{"timeout_seconds":10,"memory_mb":512,"cpu":1}}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved compute.memory_mb exceeds requested value")

	_, err = svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"compute":{"timeout_seconds":10,"memory_mb":128,"cpu":2}}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved compute.cpu exceeds requested value")
}

func TestSkillService_UpdateTenantSkillCredentials_StoresCredentials(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"credentials": []string{"API_KEY"},
	})

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())
	entry, err := svc.InstallLocalSkillPackage(ctx, 10, "sample-skill", "user-a")
	require.NoError(t, err)

	err = svc.UpdateTenantSkillCredentials(ctx, 10, entry.ID, "user-a", map[string]string{
		"API_KEY": "secret",
	})
	require.NoError(t, err)

	got, err := repo.GetTenantSkillCredentialByName(ctx, 10, "sample-skill")
	require.NoError(t, err)
	assert.JSONEq(t, `{"API_KEY":"secret"}`, got.Credentials.ToString())
	assert.Equal(t, "user-a", got.UpdatedBy)
}

func TestSkillService_InstallSkillHubPackageWithPermissions_DownloadsAndInstalls(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	archive := buildTestSkillPackageZip(t, "hub-skill", "1.2.3", map[string]any{
		"network": []string{"api.example.com"},
	})
	publisher, sig := signTestSkillHubArchive(t, archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hub-skill.zip.sig" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(sig)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("WEKNORA_SKILL_HUB_ALLOWED_HOSTS", parsed.Hostname())
	t.Setenv("WEKNORA_SKILL_HUB_TRUSTED_PUBLISHERS", publisher)

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())
	entry, err := svc.InstallSkillHubPackageWithPermissions(
		ctx,
		10,
		server.URL+"/hub-skill.zip",
		"user-a",
		types.JSON(`{"network":["api.example.com"]}`),
	)
	require.NoError(t, err)
	assert.Equal(t, types.SkillSourceTypeHub, entry.SourceType)
	assert.Contains(t, entry.SourceURI, filepath.Join(packagesRoot, "hub"))
	require.DirExists(t, entry.SourceURI)

	installs, err := repo.ListTenantSkillInstallEntries(ctx, 10)
	require.NoError(t, err)
	require.Len(t, installs, 1)
	assert.Equal(t, entry.ID, installs[0].SkillID)
	assert.JSONEq(t, `{"network":["api.example.com"]}`, installs[0].ApprovedPermissions.ToString())
}

func TestSkillService_PreviewSkillHubPackage_RejectsUnallowedHost(t *testing.T) {
	svc := NewSkillServiceWithRepository(repository.NewSkillRepository(setupSkillServiceTestDB(t)), t.TempDir())

	_, err := svc.PreviewSkillHubPackage(context.Background(), "https://example.com/hub-skill.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill hub host example.com is not allowed")
}

func TestSkillService_PreviewSkillHubPackage_RejectsUnsignedArchive(t *testing.T) {
	archive := buildTestSkillPackageZip(t, "hub-skill", "1.2.3", nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hub-skill.zip.sig" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("WEKNORA_SKILL_HUB_ALLOWED_HOSTS", parsed.Hostname())
	publisher, _ := signTestSkillHubArchive(t, archive)
	t.Setenv("WEKNORA_SKILL_HUB_TRUSTED_PUBLISHERS", publisher)

	svc := NewSkillServiceWithRepository(repository.NewSkillRepository(setupSkillServiceTestDB(t)), t.TempDir())
	_, err = svc.PreviewSkillHubPackage(context.Background(), server.URL+"/hub-skill.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download skill hub signature")
}

func TestExtractZipArchiveRejectsUnsafePath(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("../skill.json")
	require.NoError(t, err)
	_, err = file.Write([]byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	archivePath := filepath.Join(t.TempDir(), "unsafe.zip")
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0644))

	err = extractZipArchive(archivePath, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe path")
}

func TestSkillService_UpdateTenantSkillMCPBindings_StoresApprovedBindings(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"mcp": []string{"github"},
	})

	db := setupSkillServiceTestDB(t)
	require.NoError(t, db.Create(&types.MCPService{
		ID:            "mcp-service-1",
		TenantID:      10,
		Name:          "github-service",
		Enabled:       true,
		TransportType: types.MCPTransportSSE,
	}).Error)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())
	entry, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"mcp":["github"]}`),
	)
	require.NoError(t, err)

	err = svc.UpdateTenantSkillMCPBindings(ctx, 10, entry.ID, "user-a", map[string]string{
		"github": "mcp-service-1",
	})
	require.NoError(t, err)

	got, err := repo.ListTenantSkillMCPBindingsByName(ctx, 10, "sample-skill")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "github", got[0].MCPName)
	assert.Equal(t, "mcp-service-1", got[0].ServiceID)
	assert.Equal(t, "user-a", got[0].UpdatedBy)
}

func TestSkillService_UpdateTenantSkillMCPBindings_RejectsUnapprovedAlias(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"mcp": []string{"github"},
	})

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())
	entry, err := svc.InstallLocalSkillPackageWithPermissions(
		ctx,
		10,
		"sample-skill",
		"user-a",
		types.JSON(`{"mcp":["github"]}`),
	)
	require.NoError(t, err)

	err = svc.UpdateTenantSkillMCPBindings(ctx, 10, entry.ID, "user-a", map[string]string{
		"jira": "mcp-service-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp binding alias jira was not approved for skill")
}

func TestSkillService_PreviewLocalSkillPackage_ValidatesWithoutInstalling(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	packageDir := writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": []string{"api.example.com"},
	})

	db := setupSkillServiceTestDB(t)
	repo := repository.NewSkillRepository(db)
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	got, err := svc.PreviewLocalSkillPackage(ctx, "sample-skill")
	require.NoError(t, err)
	assert.Equal(t, "sample-skill", got.Name)
	assert.Equal(t, "1.2.3", got.Version)
	assert.Equal(t, "Sample skill", got.Description)
	assert.Equal(t, types.SkillSourceTypeLocal, got.SourceType)
	assert.Equal(t, packageDir, got.SourceURI)
	assert.NotEmpty(t, got.Digest)
	assert.JSONEq(t, `{"network":["api.example.com"]}`, got.RequestedPermissions.ToString())

	count, err := repo.CountSkills(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestSkillService_PreviewLocalSkillPackage_RejectsInvalidPermissionShape(t *testing.T) {
	ctx := context.Background()
	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	writeTestSkillPackage(t, packagesRoot, "sample-skill", "sample-skill", "1.2.3", "Sample skill", map[string]any{
		"network": "api.example.com",
	})

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, t.TempDir())

	_, err := svc.PreviewLocalSkillPackage(ctx, "sample-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permissions.network must be an array")
}

func TestSkillService_ResolveAgentSkillAccess(t *testing.T) {
	ctx := context.Background()
	preloadedRoot := t.TempDir()
	writeTestSkill(t, preloadedRoot, "alpha-dir", "alpha", "Alpha skill")

	packagesRoot := t.TempDir()
	t.Setenv("WEKNORA_SKILL_PACKAGES_DIR", packagesRoot)
	localDir := writeTestSkillPackage(t, packagesRoot, "local-one", "local-one", "1.0.0", "Local one", map[string]any{})
	writeTestSkillPackage(t, packagesRoot, "local-two", "local-two", "1.0.0", "Local two", map[string]any{})

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, preloadedRoot)
	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))
	_, err := svc.InstallLocalSkillPackage(ctx, 10, "local-one", "user-a")
	require.NoError(t, err)
	_, err = svc.InstallLocalSkillPackage(ctx, 10, "local-two", "user-a")
	require.NoError(t, err)

	names, dirs, err := svc.ResolveAgentSkillAccess(ctx, 10, "agent-a", "selected", []string{"local-one", "missing", "alpha"})
	require.NoError(t, err)
	assert.Equal(t, []string{"local-one", "alpha"}, names)
	assert.ElementsMatch(t, []string{preloadedRoot, packagesRoot}, dirs)

	names, dirs, err = svc.ResolveAgentSkillAccess(ctx, 10, "agent-a", "all", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "local-one", "local-two"}, names)
	assert.ElementsMatch(t, []string{preloadedRoot, filepath.Dir(localDir)}, dirs)

	require.NoError(t, svc.SyncAgentSkillBindings(ctx, 10, "agent-a", "selected", []string{"local-two"}))
	names, dirs, err = svc.ResolveAgentSkillAccess(ctx, 10, "agent-a", "selected", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-two"}, names)
	assert.Equal(t, []string{packagesRoot}, dirs)
}
