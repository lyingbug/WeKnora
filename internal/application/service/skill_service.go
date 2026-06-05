package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DefaultPreloadedSkillsDir is the default directory for preloaded skills
const DefaultPreloadedSkillsDir = "skills/preloaded"

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
	return strings.Trim(cleanID, "-")
}

func skillRegistryDigest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
