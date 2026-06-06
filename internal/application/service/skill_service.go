package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

// DefaultPreloadedSkillsDir is the default directory for preloaded skills
const DefaultPreloadedSkillsDir = "skills/preloaded"

// DefaultSkillPackagesDir is the default directory for local skill packages.
const DefaultSkillPackagesDir = "skills/packages"

const defaultSkillHubMaxBytes = int64(20 << 20)

// skillService implements SkillService interface
type skillService struct {
	loader       *skills.Loader
	repo         interfaces.SkillRepository
	preloadedDir string
	mu           sync.RWMutex
	initialized  bool
}

// NewSkillService creates a new skill service
func NewSkillService(repo interfaces.SkillRepository) interfaces.SkillService {
	return NewSkillServiceWithRepository(repo, getPreloadedSkillsDir())
}

// NewSkillServiceWithRepository creates a skill service with registry persistence.
func NewSkillServiceWithRepository(repo interfaces.SkillRepository, preloadedDir string) interfaces.SkillService {
	return &skillService{
		repo:         repo,
		preloadedDir: preloadedDir,
		initialized:  false,
	}
}

// getPreloadedSkillsDir returns the path to the preloaded skills directory
func getPreloadedSkillsDir() string {
	// Check if SKILLS_DIR environment variable is set
	if dir := os.Getenv("WEKNORA_SKILLS_DIR"); dir != "" {
		return dir
	}

	// Try to find the skills directory relative to the executable
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		skillsDir := filepath.Join(execDir, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	// Try current working directory
	cwd, err := os.Getwd()
	if err == nil {
		skillsDir := filepath.Join(cwd, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	// Default to relative path (will be created if needed)
	return DefaultPreloadedSkillsDir
}

func getSkillPackagesDir() string {
	if dir := os.Getenv("WEKNORA_SKILL_PACKAGES_DIR"); dir != "" {
		return dir
	}
	return DefaultSkillPackagesDir
}

// ensureInitialized initializes the loader if not already done
func (s *skillService) ensureInitialized(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	// Check if preloaded directory exists
	if _, err := os.Stat(s.preloadedDir); os.IsNotExist(err) {
		logger.Warnf(ctx, "Preloaded skills directory does not exist: %s", s.preloadedDir)
		// Create the directory to avoid repeated warnings
		if err := os.MkdirAll(s.preloadedDir, 0755); err != nil {
			logger.Warnf(ctx, "Failed to create preloaded skills directory: %v", err)
		}
	}

	// Create loader with preloaded directory
	s.loader = skills.NewLoader([]string{s.preloadedDir})
	s.initialized = true

	logger.Infof(ctx, "Skill service initialized with preloaded directory: %s", s.preloadedDir)

	return nil
}

// ListPreloadedSkills returns metadata for all preloaded skills
func (s *skillService) ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	if s.repo != nil {
		entries, err := s.repo.ListActiveSkills(ctx)
		if err != nil {
			logger.Warnf(ctx, "Failed to list skills from registry, falling back to filesystem: %v", err)
		} else if len(entries) > 0 {
			logger.Infof(ctx, "Loaded %d preloaded skills from registry", len(entries))
			return skillRegistryEntriesToMetadata(entries), nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, err := s.loader.DiscoverSkills()
	if err != nil {
		logger.Errorf(ctx, "Failed to discover preloaded skills: %v", err)
		return nil, fmt.Errorf("failed to discover skills: %w", err)
	}

	logger.Infof(ctx, "Discovered %d preloaded skills", len(metadata))

	return metadata, nil
}

// ImportPreloadedSkills scans filesystem preloaded skills and upserts metadata into the registry.
func (s *skillService) ImportPreloadedSkills(ctx context.Context) error {
	if err := s.ensureInitialized(ctx); err != nil {
		return fmt.Errorf("failed to initialize skill service: %w", err)
	}
	if s.repo == nil {
		return nil
	}

	s.mu.RLock()
	metadata, err := s.loader.DiscoverSkills()
	s.mu.RUnlock()
	if err != nil {
		logger.Errorf(ctx, "Failed to discover preloaded skills for import: %v", err)
		return fmt.Errorf("failed to discover skills: %w", err)
	}

	for _, meta := range metadata {
		entry := newPreloadedSkillRegistryEntry(s.preloadedDir, meta)
		if err := s.repo.UpsertSkill(ctx, entry); err != nil {
			return fmt.Errorf("failed to upsert preloaded skill %s: %w", meta.Name, err)
		}
	}

	logger.Infof(ctx, "Imported %d preloaded skills into registry", len(metadata))

	return nil
}

func (s *skillService) EnsureTenantPreloadedSkillInstalls(ctx context.Context, tenantID uint64) error {
	if tenantID == 0 || s.repo == nil {
		return nil
	}
	if err := s.ImportPreloadedSkills(ctx); err != nil {
		return err
	}

	entries, err := s.repo.ListActiveSkills(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active skills for tenant install: %w", err)
	}

	for _, entry := range entries {
		if entry.SourceType != types.SkillSourceTypePreloaded {
			continue
		}
		install := &types.TenantSkillInstall{
			ID:                  tenantSkillInstallID(tenantID, entry.ID),
			TenantID:            tenantID,
			SkillID:             entry.ID,
			Enabled:             true,
			ApprovedPermissions: types.JSON("{}"),
		}
		if err := s.repo.UpsertTenantSkillInstall(ctx, install); err != nil {
			return fmt.Errorf("failed to upsert tenant skill install %s: %w", entry.Name, err)
		}
	}

	return nil
}

func (s *skillService) ListTenantSkills(ctx context.Context, tenantID uint64) ([]*skills.SkillMetadata, error) {
	if tenantID == 0 || s.repo == nil {
		return s.ListPreloadedSkills(ctx)
	}
	if err := s.EnsureTenantPreloadedSkillInstalls(ctx, tenantID); err != nil {
		logger.Warnf(ctx, "Failed to ensure tenant skill installs, falling back to preloaded skills: %v", err)
		return s.ListPreloadedSkills(ctx)
	}

	entries, err := s.repo.ListTenantInstalledSkills(ctx, tenantID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list tenant skills, falling back to preloaded skills: %v", err)
		return s.ListPreloadedSkills(ctx)
	}
	return skillRegistryEntriesToMetadata(entries), nil
}

func (s *skillService) ListTenantSkillInstalls(ctx context.Context, tenantID uint64) ([]*types.TenantSkillInstallInfo, error) {
	if tenantID == 0 || s.repo == nil {
		return nil, nil
	}
	if err := s.EnsureTenantPreloadedSkillInstalls(ctx, tenantID); err != nil {
		return nil, err
	}
	return s.repo.ListTenantSkillInstallEntries(ctx, tenantID)
}

func (s *skillService) SetTenantSkillEnabled(ctx context.Context, tenantID uint64, skillID string, enabled bool) error {
	if tenantID == 0 {
		return fmt.Errorf("tenant ID is required")
	}
	if strings.TrimSpace(skillID) == "" {
		return fmt.Errorf("skill ID is required")
	}
	if s.repo == nil {
		return fmt.Errorf("skill repository is required")
	}
	if err := s.repo.SetTenantSkillInstallEnabled(ctx, tenantID, skillID, enabled); err != nil {
		return fmt.Errorf("failed to update tenant skill install: %w", err)
	}
	return nil
}

func (s *skillService) InstallLocalSkillPackage(
	ctx context.Context,
	tenantID uint64,
	packagePath string,
	installedBy string,
) (*types.SkillRegistryEntry, error) {
	return s.InstallLocalSkillPackageWithPermissions(ctx, tenantID, packagePath, installedBy, nil)
}

func (s *skillService) PreviewLocalSkillPackage(
	ctx context.Context,
	packagePath string,
) (*types.LocalSkillPackagePreview, error) {
	packageDir, err := resolveLocalSkillPackageDir(getSkillPackagesDir(), packagePath)
	if err != nil {
		return nil, err
	}
	loaded, err := skills.LoadSkillPackageManifest(packageDir)
	if err != nil {
		return nil, err
	}
	digest, err := skillPackageDigest(packageDir)
	if err != nil {
		return nil, err
	}

	return &types.LocalSkillPackagePreview{
		Name:                 loaded.Manifest.Name,
		Version:              loaded.Manifest.Version,
		Description:          loaded.Manifest.Description,
		SourceType:           types.SkillSourceTypeLocal,
		SourceURI:            packageDir,
		Digest:               digest,
		Manifest:             types.JSON(loaded.RawJSON),
		RequestedPermissions: types.JSON(loaded.PermissionsJSON),
	}, nil
}

func (s *skillService) PreviewSkillHubPackage(
	ctx context.Context,
	sourceURL string,
) (*types.LocalSkillPackagePreview, error) {
	packageDir, cleanup, err := downloadSkillHubPackage(ctx, sourceURL)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	loaded, err := skills.LoadSkillPackageManifest(packageDir)
	if err != nil {
		return nil, err
	}
	digest, err := skillPackageDigest(packageDir)
	if err != nil {
		return nil, err
	}

	return &types.LocalSkillPackagePreview{
		Name:                 loaded.Manifest.Name,
		Version:              loaded.Manifest.Version,
		Description:          loaded.Manifest.Description,
		SourceType:           types.SkillSourceTypeHub,
		SourceURI:            sourceURL,
		Digest:               digest,
		Manifest:             types.JSON(loaded.RawJSON),
		RequestedPermissions: types.JSON(loaded.PermissionsJSON),
	}, nil
}

func (s *skillService) InstallLocalSkillPackageWithPermissions(
	ctx context.Context,
	tenantID uint64,
	packagePath string,
	installedBy string,
	approvedPermissions types.JSON,
) (*types.SkillRegistryEntry, error) {
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if s.repo == nil {
		return nil, fmt.Errorf("skill repository is required")
	}

	packageDir, err := resolveLocalSkillPackageDir(getSkillPackagesDir(), packagePath)
	if err != nil {
		return nil, err
	}
	return s.installSkillPackageDir(ctx, tenantID, packageDir, types.SkillSourceTypeLocal, packageDir, installedBy, approvedPermissions)
}

func (s *skillService) InstallSkillHubPackageWithPermissions(
	ctx context.Context,
	tenantID uint64,
	sourceURL string,
	installedBy string,
	approvedPermissions types.JSON,
) (*types.SkillRegistryEntry, error) {
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant ID is required")
	}
	if s.repo == nil {
		return nil, fmt.Errorf("skill repository is required")
	}
	packageDir, cleanup, err := downloadSkillHubPackage(ctx, sourceURL)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	loaded, err := skills.LoadSkillPackageManifest(packageDir)
	if err != nil {
		return nil, err
	}
	digest, err := skillPackageDigest(packageDir)
	if err != nil {
		return nil, err
	}
	storedDir, err := storeHubSkillPackage(packageDir, loaded.Manifest.Name, loaded.Manifest.Version, digest)
	if err != nil {
		return nil, err
	}
	return s.installSkillPackageDir(ctx, tenantID, storedDir, types.SkillSourceTypeHub, sourceURL, installedBy, approvedPermissions)
}

func (s *skillService) installSkillPackageDir(
	ctx context.Context,
	tenantID uint64,
	packageDir string,
	sourceType string,
	sourceURI string,
	installedBy string,
	approvedPermissions types.JSON,
) (*types.SkillRegistryEntry, error) {
	loaded, err := skills.LoadSkillPackageManifest(packageDir)
	if err != nil {
		return nil, err
	}
	digest, err := skillPackageDigest(packageDir)
	if err != nil {
		return nil, err
	}
	permissions, err := normalizeApprovedSkillPermissions(approvedPermissions, loaded.PermissionsJSON)
	if err != nil {
		return nil, err
	}

	entry := &types.SkillRegistryEntry{
		ID:          skillRegistryID(sourceType, loaded.Manifest.Name, loaded.Manifest.Version),
		Name:        loaded.Manifest.Name,
		Version:     loaded.Manifest.Version,
		Description: loaded.Manifest.Description,
		SourceType:  sourceType,
		SourceURI:   sourceURIForRuntime(sourceType, sourceURI, packageDir),
		Digest:      digest,
		Manifest:    types.JSON(loaded.RawJSON),
		Status:      types.SkillStatusActive,
		IsBuiltin:   false,
	}
	if err := s.repo.UpsertSkill(ctx, entry); err != nil {
		return nil, fmt.Errorf("failed to upsert skill package: %w", err)
	}

	install := &types.TenantSkillInstall{
		ID:                  tenantSkillInstallID(tenantID, entry.ID),
		TenantID:            tenantID,
		SkillID:             entry.ID,
		Enabled:             true,
		InstalledBy:         installedBy,
		ApprovedPermissions: permissions,
	}
	if err := s.repo.UpsertTenantSkillInstall(ctx, install); err != nil {
		return nil, fmt.Errorf("failed to install skill package for tenant: %w", err)
	}

	logger.Infof(ctx, "Installed %s skill package %s@%s for tenant %d", sourceType, entry.Name, entry.Version, tenantID)
	return entry, nil
}

func sourceURIForRuntime(sourceType, sourceURI, packageDir string) string {
	if sourceType == types.SkillSourceTypeHub {
		return packageDir
	}
	return sourceURI
}

func (s *skillService) UpdateTenantSkillCredentials(
	ctx context.Context,
	tenantID uint64,
	skillID string,
	updatedBy string,
	credentials map[string]string,
) error {
	if tenantID == 0 {
		return fmt.Errorf("tenant ID is required")
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return fmt.Errorf("skill ID is required")
	}
	if s.repo == nil {
		return fmt.Errorf("skill repository is required")
	}

	installs, err := s.repo.ListTenantSkillInstallEntries(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to list tenant skill installs: %w", err)
	}
	installed := false
	for _, install := range installs {
		if install.SkillID == skillID {
			installed = true
			break
		}
	}
	if !installed {
		return fmt.Errorf("skill is not installed for tenant: %s", skillID)
	}

	encryptedCredentials, err := normalizeTenantSkillCredentials(credentials)
	if err != nil {
		return err
	}
	rawCredentials, err := json.Marshal(encryptedCredentials)
	if err != nil {
		return fmt.Errorf("failed to encode skill credentials: %w", err)
	}

	return s.repo.UpsertTenantSkillCredential(ctx, &types.TenantSkillCredential{
		ID:          tenantSkillCredentialID(tenantID, skillID),
		TenantID:    tenantID,
		SkillID:     skillID,
		Credentials: types.JSON(rawCredentials),
		UpdatedBy:   updatedBy,
	})
}

func normalizeTenantSkillCredentials(credentials map[string]string) (map[string]string, error) {
	normalized := make(map[string]string, len(credentials))
	key := utils.GetAESKey()
	for name, value := range credentials {
		name = strings.TrimSpace(name)
		if !isSkillCredentialName(name) {
			return nil, fmt.Errorf("credential name %q must be a valid environment variable name", name)
		}
		if value == "" {
			return nil, fmt.Errorf("credential %s must not be empty", name)
		}
		encrypted, err := utils.EncryptAESGCM(value, key)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt credential %s: %w", name, err)
		}
		normalized[name] = encrypted
	}
	return normalized, nil
}

func isSkillCredentialName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (s *skillService) UpdateTenantSkillMCPBindings(
	ctx context.Context,
	tenantID uint64,
	skillID string,
	updatedBy string,
	bindings map[string]string,
) error {
	if tenantID == 0 {
		return fmt.Errorf("tenant ID is required")
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return fmt.Errorf("skill ID is required")
	}
	if s.repo == nil {
		return fmt.Errorf("skill repository is required")
	}

	install, err := s.findTenantSkillInstall(ctx, tenantID, skillID)
	if err != nil {
		return err
	}
	approvedAliases, err := approvedMCPAliases(install.ApprovedPermissions)
	if err != nil {
		return err
	}
	normalized, serviceIDs, err := normalizeTenantSkillMCPBindings(bindings, approvedAliases)
	if err != nil {
		return err
	}
	enabledServices, err := s.repo.ListEnabledTenantMCPServiceIDs(ctx, tenantID, serviceIDs)
	if err != nil {
		return fmt.Errorf("failed to validate MCP services: %w", err)
	}

	rows := make([]*types.TenantSkillMCPBinding, 0, len(normalized))
	for alias, serviceID := range normalized {
		if _, ok := enabledServices[serviceID]; !ok {
			return fmt.Errorf("mcp service %s is not enabled for tenant", serviceID)
		}
		rows = append(rows, &types.TenantSkillMCPBinding{
			ID:        tenantSkillMCPBindingID(tenantID, skillID, alias),
			TenantID:  tenantID,
			SkillID:   skillID,
			MCPName:   alias,
			ServiceID: serviceID,
			UpdatedBy: updatedBy,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].MCPName < rows[j].MCPName
	})
	return s.repo.ReplaceTenantSkillMCPBindings(ctx, tenantID, skillID, rows)
}

func (s *skillService) findTenantSkillInstall(
	ctx context.Context,
	tenantID uint64,
	skillID string,
) (*types.TenantSkillInstallInfo, error) {
	installs, err := s.repo.ListTenantSkillInstallEntries(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenant skill installs: %w", err)
	}
	for _, install := range installs {
		if install.SkillID == skillID {
			return install, nil
		}
	}
	return nil, fmt.Errorf("skill is not installed for tenant: %s", skillID)
}

func approvedMCPAliases(permissions types.JSON) (map[string]struct{}, error) {
	values, err := approvedStringArrayPermission("mcp", permissions)
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]struct{}, len(values))
	for _, value := range values {
		aliases[value] = struct{}{}
	}
	return aliases, nil
}

func approvedStringArrayPermission(key string, permissions types.JSON) ([]string, error) {
	permissionsMap, err := permissions.Map()
	if err != nil {
		return nil, fmt.Errorf("approved permissions are invalid JSON: %w", err)
	}
	raw, ok := permissionsMap[key]
	if !ok || raw == nil {
		return nil, nil
	}
	return stringArrayPermission(raw, "approved "+key)
}

func normalizeTenantSkillMCPBindings(
	bindings map[string]string,
	approvedAliases map[string]struct{},
) (map[string]string, []string, error) {
	normalized := make(map[string]string, len(bindings))
	serviceIDs := make([]string, 0, len(bindings))
	for alias, serviceID := range bindings {
		alias = strings.TrimSpace(alias)
		serviceID = strings.TrimSpace(serviceID)
		if alias == "" {
			return nil, nil, fmt.Errorf("mcp binding alias is required")
		}
		if _, ok := approvedAliases[alias]; !ok {
			return nil, nil, fmt.Errorf("mcp binding alias %s was not approved for skill", alias)
		}
		if serviceID == "" {
			return nil, nil, fmt.Errorf("mcp service id is required for alias %s", alias)
		}
		normalized[alias] = serviceID
		serviceIDs = append(serviceIDs, serviceID)
	}
	sort.Strings(serviceIDs)
	return normalized, serviceIDs, nil
}

func normalizeApprovedSkillPermissions(approved types.JSON, fallback []byte) (types.JSON, error) {
	raw := []byte(approved)
	explicitApproval := len(raw) > 0
	if len(raw) == 0 {
		raw = fallback
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("approved permissions must be a JSON object: %w", err)
	}
	if obj == nil {
		return types.JSON("{}"), nil
	}
	if explicitApproval {
		requested, err := parseSkillPermissionObject(fallback, "requested permissions")
		if err != nil {
			return nil, err
		}
		for key := range obj {
			if _, ok := requested[key]; !ok {
				return nil, fmt.Errorf("approved permission %s was not requested by skill manifest", key)
			}
		}
		if err := validateApprovedStringArraySubset("network", obj, requested); err != nil {
			return nil, err
		}
		if err := validateApprovedStringArraySubset("files", obj, requested); err != nil {
			return nil, err
		}
		if err := validateApprovedStringArraySubset("credentials", obj, requested); err != nil {
			return nil, err
		}
		if err := validateApprovedStringArraySubset("mcp", obj, requested); err != nil {
			return nil, err
		}
		if err := validateApprovedComputeSubset(obj, requested); err != nil {
			return nil, err
		}
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize approved permissions: %w", err)
	}
	return types.JSON(normalized), nil
}

func validateApprovedComputeSubset(
	approved map[string]interface{},
	requested map[string]interface{},
) error {
	approvedRaw, ok := approved["compute"]
	if !ok || approvedRaw == nil {
		return nil
	}
	approvedCompute, ok := approvedRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("approved compute permission must be an object")
	}
	requestedCompute, ok := requested["compute"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("requested compute permission must be an object")
	}
	for _, key := range []string{"timeout_seconds", "memory_mb", "cpu"} {
		approvedValue, ok := approvedCompute[key]
		if !ok || approvedValue == nil {
			continue
		}
		approvedNumber, err := numericPermissionValue(approvedValue, "approved compute."+key)
		if err != nil {
			return err
		}
		requestedValue, ok := requestedCompute[key]
		if !ok || requestedValue == nil {
			return fmt.Errorf("approved compute.%s was not requested by skill manifest", key)
		}
		requestedNumber, err := numericPermissionValue(requestedValue, "requested compute."+key)
		if err != nil {
			return err
		}
		if approvedNumber > requestedNumber {
			return fmt.Errorf("approved compute.%s exceeds requested value", key)
		}
	}
	return nil
}

func numericPermissionValue(raw interface{}, label string) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", label)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", label)
	}
}

func validateApprovedStringArraySubset(
	key string,
	approved map[string]interface{},
	requested map[string]interface{},
) error {
	approvedRaw, ok := approved[key]
	if !ok || approvedRaw == nil {
		return nil
	}
	approvedValues, err := stringArrayPermission(approvedRaw, "approved "+key)
	if err != nil {
		return err
	}
	requestedValues, err := stringArrayPermission(requested[key], "requested "+key)
	if err != nil {
		return err
	}
	requestedSet := make(map[string]struct{}, len(requestedValues))
	for _, value := range requestedValues {
		requestedSet[value] = struct{}{}
	}
	for _, value := range approvedValues {
		if _, ok := requestedSet[value]; !ok {
			return fmt.Errorf("approved %s scope %s was not requested by skill manifest", key, value)
		}
	}
	return nil
}

func stringArrayPermission(raw interface{}, label string) ([]string, error) {
	values, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s permission must be an array", label)
	}
	result := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("%s permission entries must be strings", label)
		}
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

func parseSkillPermissionObject(raw []byte, label string) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", label, err)
	}
	if obj == nil {
		return map[string]interface{}{}, nil
	}
	return obj, nil
}

func (s *skillService) SyncAgentSkillBindings(
	ctx context.Context,
	tenantID uint64,
	agentID string,
	mode string,
	selectedSkillNames []string,
) error {
	if tenantID == 0 || agentID == "" || s.repo == nil {
		return nil
	}
	if mode != "selected" {
		return s.repo.ReplaceAgentSkillBindings(ctx, tenantID, agentID, nil)
	}
	if err := s.EnsureTenantPreloadedSkillInstalls(ctx, tenantID); err != nil {
		return err
	}

	installed, err := s.repo.ListTenantInstalledSkillNames(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to list tenant installed skills: %w", err)
	}

	skillIDs := make([]string, 0, len(selectedSkillNames))
	seen := make(map[string]struct{}, len(selectedSkillNames))
	for _, name := range selectedSkillNames {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if entry, ok := installed[name]; ok {
			skillIDs = append(skillIDs, entry.ID)
		}
	}

	return s.repo.ReplaceAgentSkillBindings(ctx, tenantID, agentID, skillIDs)
}

func (s *skillService) ResolveAgentSelectedSkills(
	ctx context.Context,
	tenantID uint64,
	agentID string,
	mode string,
	selectedSkillNames []string,
) ([]string, error) {
	names, _, err := s.ResolveAgentSkillAccess(ctx, tenantID, agentID, mode, selectedSkillNames)
	return names, err
}

func (s *skillService) ResolveAgentSkillAccess(
	ctx context.Context,
	tenantID uint64,
	agentID string,
	mode string,
	selectedSkillNames []string,
) ([]string, []string, error) {
	if tenantID == 0 || s.repo == nil {
		if mode == "selected" {
			return selectedSkillNames, []string{s.preloadedDir}, nil
		}
		if mode == "all" {
			metadata, err := s.ListPreloadedSkills(ctx)
			if err != nil {
				return nil, nil, err
			}
			names := skillMetadataNames(metadata)
			if len(names) == 0 {
				return names, nil, nil
			}
			return names, []string{s.preloadedDir}, nil
		}
		return nil, nil, nil
	}

	if err := s.EnsureTenantPreloadedSkillInstalls(ctx, tenantID); err != nil {
		return nil, nil, err
	}

	var entries []*types.SkillRegistryEntry
	var err error
	switch mode {
	case "all":
		entries, err = s.repo.ListTenantInstalledSkills(ctx, tenantID)
		if err != nil {
			return nil, nil, err
		}
	case "selected":
		if len(selectedSkillNames) == 0 && agentID != "" {
			entries, err = s.repo.ListAgentSkillBindings(ctx, tenantID, agentID)
			if err != nil {
				return nil, nil, err
			}
			break
		}
		installed, err := s.repo.ListTenantInstalledSkillNames(ctx, tenantID)
		if err != nil {
			return nil, nil, err
		}
		entries = make([]*types.SkillRegistryEntry, 0, len(selectedSkillNames))
		seen := make(map[string]struct{}, len(selectedSkillNames))
		for _, name := range selectedSkillNames {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if entry, ok := installed[name]; ok {
				entries = append(entries, entry)
			}
		}
	default:
		return nil, nil, nil
	}

	return skillEntryNamesInOrder(entries), skillEntryLoaderDirs(entries), nil
}

// GetSkillByName retrieves a skill by its name
func (s *skillService) GetSkillByName(ctx context.Context, name string) (*skills.Skill, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.loader.LoadSkillInstructions(name)
	if err != nil {
		logger.Errorf(ctx, "Failed to load skill %s: %v", name, err)
		return nil, fmt.Errorf("failed to load skill: %w", err)
	}

	return skill, nil
}

// GetPreloadedDir returns the configured preloaded skills directory
func (s *skillService) GetPreloadedDir() string {
	return s.preloadedDir
}

func skillRegistryEntriesToMetadata(entries []*types.SkillRegistryEntry) []*skills.SkillMetadata {
	metadata := make([]*skills.SkillMetadata, 0, len(entries))
	for _, entry := range entries {
		metadata = append(metadata, &skills.SkillMetadata{
			Name:        entry.Name,
			Description: entry.Description,
			BasePath:    entry.SourceURI,
		})
	}
	return metadata
}

func skillEntryNames(entries []*types.SkillRegistryEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}

func skillEntryNamesInOrder(entries []*types.SkillRegistryEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

func skillEntryLoaderDirs(entries []*types.SkillRegistryEntry) []string {
	dirs := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		dir := skillEntryLoaderDir(entry)
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs
}

func skillEntryLoaderDir(entry *types.SkillRegistryEntry) string {
	if entry == nil || entry.SourceURI == "" {
		return ""
	}
	return filepath.Dir(entry.SourceURI)
}

func skillMetadataNames(metadata []*skills.SkillMetadata) []string {
	names := make([]string, 0, len(metadata))
	for _, meta := range metadata {
		names = append(names, meta.Name)
	}
	sort.Strings(names)
	return names
}

func newPreloadedSkillRegistryEntry(preloadedDir string, meta *skills.SkillMetadata) *types.SkillRegistryEntry {
	version := types.DefaultSkillVersion
	sourceURI := meta.BasePath
	if sourceURI == "" {
		sourceURI = filepath.Join(preloadedDir, meta.Name)
	}

	return &types.SkillRegistryEntry{
		ID:          skillRegistryID(types.SkillSourceTypePreloaded, meta.Name, version),
		Name:        meta.Name,
		Version:     version,
		Description: meta.Description,
		SourceType:  types.SkillSourceTypePreloaded,
		SourceURI:   sourceURI,
		Digest:      skillRegistryDigest(meta.Name, version, meta.Description, sourceURI),
		Manifest:    types.JSON("{}"),
		Status:      types.SkillStatusActive,
		IsBuiltin:   true,
	}
}

func skillRegistryID(sourceType, name, version string) string {
	rawID := sourceType + "-" + name + "-" + version
	cleanID := regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(rawID, "-")
	cleanID = strings.Trim(cleanID, "-")
	if len(cleanID) <= 64 {
		return cleanID
	}

	sum := sha256.Sum256([]byte(rawID))
	suffix := hex.EncodeToString(sum[:])[:12]
	prefix := strings.Trim(cleanID[:64-len(suffix)-1], "-")
	return prefix + "-" + suffix
}

func tenantSkillInstallID(tenantID uint64, skillID string) string {
	return skillRegistryID("tenant-install", fmt.Sprintf("%d", tenantID), skillID)
}

func tenantSkillCredentialID(tenantID uint64, skillID string) string {
	return skillRegistryID("tenant-credential", fmt.Sprintf("%d", tenantID), skillID)
}

func tenantSkillMCPBindingID(tenantID uint64, skillID, alias string) string {
	return skillRegistryID("tenant-mcp-binding", fmt.Sprintf("%d-%s", tenantID, alias), skillID)
}

func skillRegistryDigest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func resolveLocalSkillPackageDir(packagesRoot, packagePath string) (string, error) {
	if strings.TrimSpace(packagePath) == "" {
		return "", fmt.Errorf("skill package path is required")
	}
	root, err := filepath.Abs(packagesRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve skill packages directory: %w", err)
	}

	var candidate string
	if filepath.IsAbs(packagePath) {
		candidate = filepath.Clean(packagePath)
	} else {
		candidate = filepath.Join(root, packagePath)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to resolve skill package path: %w", err)
	}
	if candidate != root && !strings.HasPrefix(candidate, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("skill package path must be within skill packages directory %s", root)
	}

	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to stat skill package path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skill package path must be a directory")
	}
	return candidate, nil
}

func downloadSkillHubPackage(ctx context.Context, sourceURL string) (string, func(), error) {
	parsed, err := validateSkillHubURL(sourceURL)
	if err != nil {
		return "", nil, err
	}
	tmpDir, err := os.MkdirTemp("", "weknora-skill-hub-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create skill hub temp directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	archivePath := filepath.Join(tmpDir, "package.archive")
	if err := downloadSkillHubArchive(ctx, parsed.String(), archivePath); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := verifySkillHubArchiveSignature(ctx, parsed.String(), archivePath); err != nil {
		cleanup()
		return "", nil, err
	}
	extractRoot := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractRoot, 0755); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to create skill hub extraction directory: %w", err)
	}
	if err := extractSkillHubArchive(archivePath, parsed.Path, extractRoot); err != nil {
		cleanup()
		return "", nil, err
	}
	packageDir, err := locateExtractedSkillPackage(extractRoot)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return packageDir, cleanup, nil
}

func validateSkillHubURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("skill hub source_url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid skill hub source_url: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("skill hub source_url must use http or https")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("skill hub source_url must not include user info")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("skill hub source_url host is required")
	}
	if !skillHubHostAllowed(parsed.Hostname()) {
		return nil, fmt.Errorf("skill hub host %s is not allowed", parsed.Hostname())
	}
	return parsed, nil
}

func skillHubHostAllowed(host string) bool {
	for _, value := range strings.Split(os.Getenv("WEKNORA_SKILL_HUB_ALLOWED_HOSTS"), ",") {
		value = strings.TrimSpace(value)
		if value != "" && strings.EqualFold(host, value) {
			return true
		}
	}
	return false
}

func downloadSkillHubArchive(ctx context.Context, sourceURL, archivePath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create skill hub request: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download skill hub package: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download skill hub package: status %d", resp.StatusCode)
	}

	maxBytes := skillHubMaxBytes()
	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create skill hub archive: %w", err)
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("failed to save skill hub archive: %w", err)
	}
	if written > maxBytes {
		return fmt.Errorf("skill hub package exceeds max size of %d bytes", maxBytes)
	}
	return nil
}

type skillHubSignature struct {
	Publisher string `json:"publisher"`
	Algorithm string `json:"algorithm"`
	Signature string `json:"signature"`
}

func verifySkillHubArchiveSignature(ctx context.Context, sourceURL, archivePath string) error {
	trustedPublishers, err := trustedSkillHubPublishers()
	if err != nil {
		return err
	}
	signatureURL := sourceURL + ".sig"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signatureURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create skill hub signature request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("failed to download skill hub signature: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download skill hub signature: status %d", resp.StatusCode)
	}
	var signature skillHubSignature
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&signature); err != nil {
		return fmt.Errorf("failed to parse skill hub signature: %w", err)
	}
	if signature.Algorithm != "ed25519-sha256" {
		return fmt.Errorf("unsupported skill hub signature algorithm: %s", signature.Algorithm)
	}
	publicKey, ok := trustedPublishers[signature.Publisher]
	if !ok {
		return fmt.Errorf("skill hub publisher %s is not trusted", signature.Publisher)
	}
	rawSignature, err := base64.StdEncoding.DecodeString(signature.Signature)
	if err != nil {
		return fmt.Errorf("invalid skill hub signature encoding: %w", err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("failed to read skill hub archive for signature verification: %w", err)
	}
	digest := sha256.Sum256(archive)
	if !ed25519.Verify(publicKey, digest[:], rawSignature) {
		return fmt.Errorf("skill hub signature verification failed")
	}
	return nil
}

func trustedSkillHubPublishers() (map[string]ed25519.PublicKey, error) {
	raw := strings.TrimSpace(os.Getenv("WEKNORA_SKILL_HUB_TRUSTED_PUBLISHERS"))
	if raw == "" {
		return nil, fmt.Errorf("WEKNORA_SKILL_HUB_TRUSTED_PUBLISHERS is required")
	}
	result := make(map[string]ed25519.PublicKey)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid trusted skill hub publisher entry")
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid trusted skill hub publisher key: %w", err)
		}
		if len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted skill hub publisher key has invalid length")
		}
		result[strings.TrimSpace(parts[0])] = ed25519.PublicKey(key)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("WEKNORA_SKILL_HUB_TRUSTED_PUBLISHERS is required")
	}
	return result, nil
}

func skillHubMaxBytes() int64 {
	if raw := strings.TrimSpace(os.Getenv("WEKNORA_SKILL_HUB_MAX_BYTES")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && value > 0 {
			return value
		}
	}
	return defaultSkillHubMaxBytes
}

func extractSkillHubArchive(archivePath, sourcePath, destination string) error {
	lowerPath := strings.ToLower(sourcePath)
	switch {
	case strings.HasSuffix(lowerPath, ".zip"):
		return extractZipArchive(archivePath, destination)
	case strings.HasSuffix(lowerPath, ".tar.gz"), strings.HasSuffix(lowerPath, ".tgz"):
		return extractTarGzArchive(archivePath, destination)
	default:
		return fmt.Errorf("skill hub package must be a .zip, .tar.gz, or .tgz archive")
	}
}

func extractZipArchive(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open skill hub zip archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		target, err := safeArchiveTarget(destination, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create archive directory: %w", err)
			}
			continue
		}
		if !file.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("skill hub archive contains unsupported entry: %s", file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create archive parent directory: %w", err)
		}
		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open archive file: %w", err)
		}
		if err := writeArchiveFile(target, src, file.FileInfo().Mode()); err != nil {
			_ = src.Close()
			return err
		}
		if err := src.Close(); err != nil {
			return fmt.Errorf("failed to close archive file: %w", err)
		}
	}
	return nil
}

func extractTarGzArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open skill hub tar archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to open skill hub gzip archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read tar archive: %w", err)
		}
		target, err := safeArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create archive directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create archive parent directory: %w", err)
			}
			if err := writeArchiveFile(target, reader, os.FileMode(header.Mode)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("skill hub archive contains unsupported entry: %s", header.Name)
		}
	}
}

func safeArchiveTarget(destination, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if cleanName == "." || cleanName == ".." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("skill hub archive contains unsafe path: %s", name)
	}
	target := filepath.Join(destination, cleanName)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("failed to resolve archive target: %w", err)
	}
	destinationAbs, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("failed to resolve archive destination: %w", err)
	}
	if targetAbs != destinationAbs && !strings.HasPrefix(targetAbs, destinationAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("skill hub archive contains unsafe path: %s", name)
	}
	return targetAbs, nil
}

func writeArchiveFile(path string, reader io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, reader); err != nil {
		return fmt.Errorf("failed to write archive file: %w", err)
	}
	return nil
}

func locateExtractedSkillPackage(root string) (string, error) {
	var matches []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "skill.json" {
			matches = append(matches, filepath.Dir(path))
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to inspect extracted skill package: %w", err)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("skill hub package must contain exactly one skill.json")
	}
	return matches[0], nil
}

func storeHubSkillPackage(sourceDir, name, version, digest string) (string, error) {
	root, err := filepath.Abs(filepath.Join(getSkillPackagesDir(), "hub"))
	if err != nil {
		return "", fmt.Errorf("failed to resolve skill hub package directory: %w", err)
	}
	dirName := skillRegistryID(types.SkillSourceTypeHub, name, version+"-"+digest[:12])
	target := filepath.Join(root, dirName)
	if err := os.RemoveAll(target); err != nil {
		return "", fmt.Errorf("failed to replace existing skill hub package: %w", err)
	}
	if err := copySkillPackageDir(sourceDir, target); err != nil {
		return "", err
	}
	return target, nil
}

func copySkillPackageDir(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to copy skill package: %w", err)
		}
		target := filepath.Join(targetDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to read skill package file info: %w", err)
		}
		src, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open skill package file: %w", err)
		}
		defer src.Close()
		return writeArchiveFile(target, src, info.Mode())
	})
}

func skillPackageDigest(packageDir string) (string, error) {
	var files []string
	if err := filepath.WalkDir(packageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to walk skill package: %w", err)
	}
	sort.Strings(files)

	hash := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(packageDir, path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})

		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(hash, file); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
