// Package memory implements long-term memory: a private, wiki-shaped store of
// durable facts per principal, the anchors that connect it to the shared
// knowledge base, and the read and write paths that keep it useful across
// sessions.
//
// The package depends only on interfaces and types, never on the sibling
// service package, so it can be consumed from the chat pipeline, the agent
// runtime and the HTTP layer without an import cycle.
package memory

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Deployment describes the facts about this installation that settings depend
// on but cannot themselves configure.
//
// It is passed in rather than sniffed: the plan's rule is that behaviour keys
// off DB_DRIVER and REDIS_ADDR, not off the product edition, and edition is
// only consulted for the one genuinely product-level capability (shared memory
// spaces). Keeping all three here makes that distinction visible instead of
// scattering os.Getenv calls through the service.
type Deployment struct {
	// Edition is "lite" or "standard".
	Edition string
	// HasRedis reports whether a Redis-backed queue and locks are available.
	HasRedis bool
	// Dialect is the main database driver name.
	Dialect string
}

// IsLite reports whether this is the Lite product edition.
func (d Deployment) IsLite() bool { return strings.EqualFold(d.Edition, "lite") }

// settingsService resolves the layered memory settings.
type settingsService struct {
	deployment  Deployment
	envPatch    types.MemorySettingsPatch
	tenants     interfaces.TenantService
	users       interfaces.UserService
	agents      interfaces.CustomAgentService
	spaces      interfaces.MemorySpaceRepository
	hasEmbedder func(ctx context.Context) bool
}

// NewSettingsService creates the settings resolver.
//
// hasEmbedder may be nil. It is only consulted for capability reporting, and
// exists for a future vector-backed recall path; the current relevance scorer
// needs no embedding model, which is why memory works out of the box.
func NewSettingsService(
	deployment Deployment,
	tenants interfaces.TenantService,
	users interfaces.UserService,
	agents interfaces.CustomAgentService,
	spaces interfaces.MemorySpaceRepository,
	hasEmbedder func(ctx context.Context) bool,
) interfaces.MemorySettingsService {
	return &settingsService{
		deployment:  deployment,
		envPatch:    deploymentPatchFromEnv(),
		tenants:     tenants,
		users:       users,
		agents:      agents,
		spaces:      spaces,
		hasEmbedder: hasEmbedder,
	}
}

// deploymentPatchFromEnv reads the two knobs an operator may need before any
// workspace exists. Everything else is configured in the UI on purpose: a
// feature nobody can find is a feature nobody trusts.
func deploymentPatchFromEnv() types.MemorySettingsPatch {
	patch := types.MemorySettingsPatch{}
	if raw, ok := os.LookupEnv("MEMORY_ENABLE"); ok {
		if enabled, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			patch[types.SettingMemoryEnabled] = enabled
		}
	}
	if raw, ok := os.LookupEnv("MEMORY_RECALL_TIMEOUT_MS"); ok {
		if ms, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && ms > 0 {
			patch[types.SettingMemoryRecallTimeoutMs] = ms
		}
	}
	if len(patch) == 0 {
		return nil
	}
	return patch
}

// Resolve folds every layer that applies to the caller.
//
// A layer that cannot be loaded is skipped with a log line rather than failing:
// this runs on the chat path, and a transient error reading a preference blob
// must never cost the user their answer.
func (s *settingsService) Resolve(
	ctx context.Context, opts types.MemorySettingsResolveOptions,
) (types.MemorySettingsResolution, error) {
	layers := make([]types.MemorySettingsLayer, 0, 5)

	if len(s.envPatch) > 0 {
		layers = append(layers, types.MemorySettingsLayer{
			Layer: types.MemoryLayerDeployment, Patch: s.envPatch,
		})
	}

	if opts.TenantID > 0 && s.tenants != nil {
		if tenant, err := s.tenants.GetTenantByID(ctx, opts.TenantID); err != nil {
			logger.Warnf(ctx, "memory settings: tenant %d unreadable, using defaults: %v", opts.TenantID, err)
		} else if tenant != nil && tenant.MemoryConfig != nil {
			layers = append(layers, types.MemorySettingsLayer{
				Layer: types.MemoryLayerTenant, Patch: *tenant.MemoryConfig,
			})
		}
	}

	if patch := s.agentPatch(ctx, opts); len(patch) > 0 {
		layers = append(layers, types.MemorySettingsLayer{Layer: types.MemoryLayerAgent, Patch: patch})
	}

	if opts.UserID != "" && s.users != nil {
		if user, err := s.users.GetUserByID(ctx, opts.UserID); err != nil {
			logger.Warnf(ctx, "memory settings: user %s unreadable, using defaults: %v", opts.UserID, err)
		} else if user != nil && len(user.Preferences.Memory) > 0 {
			layers = append(layers, types.MemorySettingsLayer{
				Layer: types.MemoryLayerUser, Patch: user.Preferences.Memory,
			})
		}
	}

	if patch := s.spacePatch(ctx, opts); len(patch) > 0 {
		layers = append(layers, types.MemorySettingsLayer{Layer: types.MemoryLayerSpace, Patch: patch})
	}

	return types.ResolveMemorySettings(layers...), nil
}

func (s *settingsService) agentPatch(
	ctx context.Context, opts types.MemorySettingsResolveOptions,
) types.MemorySettingsPatch {
	if len(opts.AgentPatch) > 0 {
		return opts.AgentPatch
	}
	if opts.AgentID == "" || s.agents == nil {
		return nil
	}
	agent, err := s.agents.GetAgentByID(ctx, opts.AgentID)
	if err != nil || agent == nil {
		return nil
	}
	return agent.Config.Memory
}

func (s *settingsService) spacePatch(
	ctx context.Context, opts types.MemorySettingsResolveOptions,
) types.MemorySettingsPatch {
	if len(opts.SpacePatch) > 0 {
		return opts.SpacePatch
	}
	if opts.SpaceID == "" || s.spaces == nil || opts.TenantID == 0 {
		return nil
	}
	space, err := s.spaces.GetByID(ctx, opts.TenantID, opts.SpaceID)
	if err != nil || space == nil {
		return nil
	}
	return space.Config
}

// View renders the settings for a UI editing at a particular layer.
func (s *settingsService) View(
	ctx context.Context, opts types.MemorySettingsResolveOptions, editableLevel string,
) (*types.MemorySettingsView, error) {
	resolution, err := s.Resolve(ctx, opts)
	if err != nil {
		return nil, err
	}
	descriptors := types.MemorySettingDescriptors()
	editable := make(map[string]bool, len(descriptors))
	for _, d := range descriptors {
		editable[d.Key] = resolution.EditableAt(d.Key, editableLevel)
	}
	return &types.MemorySettingsView{
		Values:        resolution.Values,
		Descriptors:   descriptors,
		EditableLevel: editableLevel,
		Editable:      editable,
		Capabilities:  s.Capabilities(ctx, resolution.Settings),
	}, nil
}

// UpdateTenant patches the workspace layer.
func (s *settingsService) UpdateTenant(
	ctx context.Context, tenantID uint64, patch types.MemorySettingsPatch,
) ([]string, error) {
	clean, notes := types.SanitizeMemoryPatch(types.MemoryLayerTenant, patch)
	tenant, err := s.tenants.GetTenantByID(ctx, tenantID)
	if err != nil {
		return notes, err
	}
	merged := mergePatch(tenant.MemoryConfig, clean)
	tenant.MemoryConfig = &merged
	if _, err := s.tenants.UpdateTenant(ctx, tenant); err != nil {
		return notes, err
	}
	return notes, nil
}

// UpdateUser patches the user layer.
func (s *settingsService) UpdateUser(
	ctx context.Context, tenantID uint64, userID string, patch types.MemorySettingsPatch,
) ([]string, error) {
	clean, notes := types.SanitizeMemoryPatch(types.MemoryLayerUser, patch)
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return notes, err
	}
	merged := mergePatch(&user.Preferences.Memory, clean)
	user.Preferences.Memory = merged
	if err := s.users.UpdateUser(ctx, user); err != nil {
		return notes, err
	}
	return notes, nil
}

// UpdateSpace patches the space layer.
func (s *settingsService) UpdateSpace(
	ctx context.Context, tenantID uint64, spaceID string, patch types.MemorySettingsPatch,
) ([]string, error) {
	clean, notes := types.SanitizeMemoryPatch(types.MemoryLayerSpace, patch)
	space, err := s.spaces.GetByID(ctx, tenantID, spaceID)
	if err != nil {
		return notes, err
	}
	space.Config = mergePatch(&space.Config, clean)
	if err := s.spaces.Update(ctx, space); err != nil {
		return notes, err
	}
	return notes, nil
}

// Capabilities reports which features are usable, and why not when they are
// not. Reporting a reason is the difference between a control the user
// understands is unavailable and one that appears broken.
func (s *settingsService) Capabilities(
	ctx context.Context, settings types.MemorySettings,
) map[string]types.MemoryCapability {
	caps := map[string]types.MemoryCapability{}

	// Shared memory spaces build on the shared-workspace product capability,
	// which Lite does not offer. This is the only form-dependent capability.
	switch {
	case s.deployment.IsLite():
		caps[types.MemoryCapabilitySharedSpace] = types.MemoryCapability{
			Available: false, Reason: types.MemoryCapabilityReasonLite,
		}
	case !settings.SharedSpaceEnabled:
		caps[types.MemoryCapabilitySharedSpace] = types.MemoryCapability{
			Available: false, Reason: types.MemoryCapabilityReasonDisabled,
		}
	default:
		caps[types.MemoryCapabilitySharedSpace] = types.MemoryCapability{Available: true}
	}

	// Relevance recall scores memories lexically in Go, so unlike most
	// retrieval features it has no model or vector-store prerequisite: the only
	// way it becomes unavailable is by being switched off.
	relevance := types.MemoryCapability{Available: settings.RelevanceRecall}
	if !settings.RelevanceRecall {
		relevance.Reason = types.MemoryCapabilityReasonDisabled
	}
	caps[types.MemoryCapabilityRelevanceRecall] = relevance

	autoExtract := types.MemoryCapability{Available: settings.AutoExtractEnabled()}
	if !autoExtract.Available {
		autoExtract.Reason = types.MemoryCapabilityReasonDisabled
	}
	caps[types.MemoryCapabilityAutoExtract] = autoExtract

	insights := types.MemoryCapability{Available: settings.InsightsEnabled}
	if !insights.Available {
		insights.Reason = types.MemoryCapabilityReasonDisabled
	}
	caps[types.MemoryCapabilityInsights] = insights

	return caps
}

// mergePatch overlays clean onto base, returning a new patch. A key explicitly
// set to nil is removed, which is how the UI expresses "stop overriding this
// and inherit again".
func mergePatch(base *types.MemorySettingsPatch, clean types.MemorySettingsPatch) types.MemorySettingsPatch {
	out := types.MemorySettingsPatch{}
	if base != nil {
		for k, v := range *base {
			out[k] = v
		}
	}
	for k, v := range clean {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}
