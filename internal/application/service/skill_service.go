package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DefaultPreloadedSkillsDir is the default directory for preloaded skills
const DefaultPreloadedSkillsDir = "skills/preloaded"

// DefaultSkillPackagesDir is the default directory for local skill packages.
const DefaultSkillPackagesDir = "skills/packages"

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
		ID:          skillRegistryID(types.SkillSourceTypeLocal, loaded.Manifest.Name, loaded.Manifest.Version),
		Name:        loaded.Manifest.Name,
		Version:     loaded.Manifest.Version,
		Description: loaded.Manifest.Description,
		SourceType:  types.SkillSourceTypeLocal,
		SourceURI:   packageDir,
		Digest:      digest,
		Manifest:    types.JSON(loaded.RawJSON),
		Status:      types.SkillStatusActive,
		IsBuiltin:   false,
	}
	if err := s.repo.UpsertSkill(ctx, entry); err != nil {
		return nil, fmt.Errorf("failed to upsert local skill package: %w", err)
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
		return nil, fmt.Errorf("failed to install local skill package for tenant: %w", err)
	}

	logger.Infof(ctx, "Installed local skill package %s@%s for tenant %d", entry.Name, entry.Version, tenantID)

	return entry, nil
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
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize approved permissions: %w", err)
	}
	return types.JSON(normalized), nil
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
