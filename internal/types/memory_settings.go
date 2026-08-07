package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Memory settings are the control surface for the whole subsystem. Rather than
// one master switch, every behaviour a user might reasonably want to refuse —
// automatic extraction, relevance recall, personalised ranking, illumination,
// aggregate insights — has its own key.
//
// The design has three parts, and each exists for a reason:
//
//  1. A single descriptor table (memorySettingDescriptors) declares every key
//     once: its type, default, bounds, which layers may set it, and how layers
//     combine. Nothing else in the codebase gets to invent a memory setting,
//     and the settings API can render a complete UI straight from this table.
//
//  2. Layers are sparse patches, not full structs, so "unset" is distinct from
//     "set to the default". Without that distinction a tenant could never tell
//     whether a user had deliberately opted out.
//
//  3. Merge behaviour is per-key, not global. Enable flags AND together so any
//     layer can veto; budgets take the stricter value so a narrow layer cannot
//     inflate cost; safety keys can only tighten. A single "nearest wins" rule
//     would let a user re-enable something an administrator disabled.
//
// Layer order, widest to narrowest: deployment, tenant, agent, user, space.

// Setting layers.
const (
	MemoryLayerDeployment = "deployment"
	MemoryLayerTenant     = "tenant"
	MemoryLayerAgent      = "agent"
	MemoryLayerUser       = "user"
	MemoryLayerSpace      = "space"
	// memoryLayerDefault is the synthetic source reported when no layer set a
	// value and the built-in default applies.
	memoryLayerDefault = "default"
)

// memoryLayerOrder lists layers from widest to narrowest.
var memoryLayerOrder = []string{
	MemoryLayerDeployment,
	MemoryLayerTenant,
	MemoryLayerAgent,
	MemoryLayerUser,
	MemoryLayerSpace,
}

func memoryLayerRank(layer string) int {
	for i, l := range memoryLayerOrder {
		if l == layer {
			return i
		}
	}
	return -1
}

// Setting value kinds.
type memorySettingKind string

const (
	memoryKindBool       memorySettingKind = "bool"
	memoryKindInt        memorySettingKind = "int"
	memoryKindFloat      memorySettingKind = "float"
	memoryKindString     memorySettingKind = "string"
	memoryKindEnum       memorySettingKind = "enum"
	memoryKindStringList memorySettingKind = "string_list"
	memoryKindFloatMap   memorySettingKind = "float_map"
	memoryKindIntMap     memorySettingKind = "int_map"
)

// Merge strategies.
type memoryMergeKind string

const (
	// mergeAnd: booleans where false is the restrictive value.
	mergeAnd memoryMergeKind = "and"
	// mergeOr: booleans where true is the restrictive value (safety switches).
	mergeOr memoryMergeKind = "or"
	// mergeMin: numeric budgets — the smallest value wins.
	mergeMin memoryMergeKind = "min"
	// mergeMax: numeric thresholds where higher is stricter.
	mergeMax memoryMergeKind = "max"
	// mergeOverride: nearest layer wins outright.
	mergeOverride memoryMergeKind = "override"
	// mergeIntersect: string lists narrow as layers stack.
	mergeIntersect memoryMergeKind = "intersect"
	// mergeUnion: string lists accumulate (deny lists).
	mergeUnion memoryMergeKind = "union"
	// mergeStrictestEnum: ordered enum, the most restrictive value wins.
	mergeStrictestEnum memoryMergeKind = "strictest_enum"
)

// Setting groups, used to lay out the settings UI.
const (
	MemoryGroupGeneral   = "general"
	MemoryGroupWrite     = "write"
	MemoryGroupRecall    = "recall"
	MemoryGroupBoost     = "boost"
	MemoryGroupAnchor    = "anchor"
	MemoryGroupLifecycle = "lifecycle"
	MemoryGroupPrivacy   = "privacy"
	MemoryGroupInsights  = "insights"
)

// Setting keys. Referenced by code, the API and the UI.
const (
	SettingMemoryEnabled            = "memory.enabled"
	SettingMemoryChannels           = "memory.channels"
	SettingMemoryEmbedVisitorSpace  = "memory.embed_visitor_space"
	SettingMemorySharedSpaceEnabled = "memory.shared_space_enabled"

	SettingMemoryWriteMode          = "memory.write.mode"
	SettingMemoryWriteRequireReview = "memory.write.require_review"
	SettingMemoryWriteAllowedTypes  = "memory.write.allowed_types"
	SettingMemoryWriteMinConfidence = "memory.write.min_confidence"
	SettingMemoryWriteGateKeywords  = "memory.write.gate_keywords"
	SettingMemoryWriteDebounce      = "memory.write.debounce_seconds"
	SettingMemoryWriteTurnInterval  = "memory.write.turn_interval"
	SettingMemoryWriteMaxNotes      = "memory.write.max_notes_per_window"
	SettingMemoryWriteExtractModel  = "memory.write.extraction_model_id"

	SettingMemoryRecallEnabled       = "memory.recall.enabled"
	SettingMemoryRecallRelevance     = "memory.recall.relevance_enabled"
	SettingMemoryRecallResidentTypes = "memory.recall.always_include_types"
	SettingMemoryRecallMaxItems      = "memory.recall.max_items"
	SettingMemoryRecallTokenBudget   = "memory.recall.injection_token_budget"
	SettingMemoryRecallTimeoutMs     = "memory.recall.timeout_ms"
	SettingMemoryRecallShowUsed      = "memory.recall.show_used_memories"
	SettingMemoryRecallCite          = "memory.recall.cite_memories"

	SettingMemoryBoostEnabled = "memory.boost.enabled"
	SettingMemoryBoostFactor  = "memory.boost.factor"

	SettingMemoryAnchorRuntime      = "memory.anchor.runtime_enabled"
	SettingMemoryAnchorResolve      = "memory.anchor.resolve_enabled"
	SettingMemoryOverlayEnabled     = "memory.overlay.enabled"
	SettingMemoryOverlayFamiliar    = "memory.overlay.familiar_threshold"
	SettingMemoryOverlayMastered    = "memory.overlay.mastered_threshold"
	SettingMemoryOverlayWeights     = "memory.overlay.relation_weights"
	SettingMemoryOverlayHalfLife    = "memory.overlay.anchor_half_life_days"
	SettingMemoryOverlayDecayExempt = "memory.overlay.decay_exempt_relations"

	SettingMemoryDecayEnabled     = "memory.decay.enabled"
	SettingMemoryDecayHalfLives   = "memory.decay.half_life_days"
	SettingMemoryDecayArchiveAt   = "memory.decay.archive_threshold"
	SettingMemoryMaxPagesPerSpace = "memory.capacity.max_pages_per_space"
	SettingMemoryRetentionDays    = "memory.retention.days"
	SettingMemoryPurgeArchivedAft = "memory.retention.purge_archived_after_days"

	SettingMemoryPIIRedaction    = "memory.privacy.pii_redaction"
	SettingMemoryBlockedPatterns = "memory.privacy.blocked_patterns"
	SettingMemoryAdminMetaOnly   = "memory.privacy.admin_metadata_only"
	SettingMemoryInjectSanitize  = "memory.security.injection_sanitize"
	SettingMemoryExtractSources  = "memory.security.extract_sources"
	SettingMemoryExportEnabled   = "memory.privacy.export_enabled"
	SettingMemoryForgetEnabled   = "memory.privacy.forget_enabled"

	SettingMemoryInsightsEnabled  = "memory.insights.enabled"
	SettingMemoryInsightsKAnon    = "memory.insights.k_anonymity"
	SettingMemoryInsightsCoverage = "memory.insights.member_coverage_visible"
	SettingMemoryInsightsAutoFile = "memory.insights.auto_file_wiki_issues"
)

// Write modes, ordered from most to least restrictive.
const (
	// MemoryWriteModeOff records nothing.
	MemoryWriteModeOff = "off"
	// MemoryWriteModeExplicit records only what the user or an agent asks to
	// be remembered. No model is invoked, so the feature costs nothing.
	MemoryWriteModeExplicit = "explicit_only"
	// MemoryWriteModeGatedAuto extracts only after a rule-based gate fires.
	MemoryWriteModeGatedAuto = "gated_auto"
	// MemoryWriteModeAlwaysAuto extracts once per debounce window.
	MemoryWriteModeAlwaysAuto = "always_auto"
)

// memoryWriteModeOrder ranks write modes; a lower index is more restrictive.
var memoryWriteModeOrder = []string{
	MemoryWriteModeOff,
	MemoryWriteModeExplicit,
	MemoryWriteModeGatedAuto,
	MemoryWriteModeAlwaysAuto,
}

// Embed visitor space policies.
const (
	MemoryEmbedSpaceOff         = "off"
	MemoryEmbedSpaceSessionOnly = "session_only"
	MemoryEmbedSpacePersistent  = "persistent"
)

var memoryEmbedSpaceOrder = []string{
	MemoryEmbedSpaceOff,
	MemoryEmbedSpaceSessionOnly,
	MemoryEmbedSpacePersistent,
}

// PII redaction policies, ordered from least to most restrictive.
const (
	MemoryPIIOff    = "off"
	MemoryPIIRedact = "redact"
	MemoryPIIBlock  = "block"
)

var memoryPIIOrder = []string{MemoryPIIOff, MemoryPIIRedact, MemoryPIIBlock}

// Extraction sources. Only user messages are permitted, and the descriptor
// marks the key as hard-locked so no layer can widen it. Allowing documents or
// tool output here would turn a poisoned document into a permanent instruction.
const (
	MemoryExtractSourceUserMessage = "user_message"
)

// Channels that may own a persistent memory space.
const (
	MemoryChannelWeb   = "web"
	MemoryChannelAPI   = "api"
	MemoryChannelIM    = "im"
	MemoryChannelEmbed = "embed"
)

// MemorySettingDescriptor is the single declaration of one setting.
type MemorySettingDescriptor struct {
	Key   string            `json:"key"`
	Group string            `json:"group"`
	Kind  memorySettingKind `json:"kind"`
	// Default applies when no layer sets the key.
	Default any `json:"default"`
	// Merge decides how layer values combine.
	Merge memoryMergeKind `json:"merge"`
	// Levels lists the layers allowed to set this key.
	Levels []string `json:"levels"`
	// Allowed enumerates valid values for enum / string_list keys.
	Allowed []string `json:"allowed,omitempty"`
	// Min / Max bound numeric keys. Values outside are clamped, not rejected,
	// so a bad config degrades to a safe value instead of breaking chat.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// HardLocked keys cannot be changed by any layer through the API. They
	// exist as settings so the UI can show them and explain why.
	HardLocked bool `json:"hard_locked,omitempty"`
}

func f64(v float64) *float64 { return &v }

// memorySettingDescriptors is the authoritative catalogue.
var memorySettingDescriptors = []MemorySettingDescriptor{
	// -- General ------------------------------------------------------------
	{
		Key: SettingMemoryEnabled, Group: MemoryGroupGeneral, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerDeployment, MemoryLayerTenant, MemoryLayerUser, MemoryLayerSpace},
	},
	{
		Key: SettingMemoryChannels, Group: MemoryGroupGeneral, Kind: memoryKindStringList,
		Default: []string{MemoryChannelWeb}, Merge: mergeOverride,
		Levels:  []string{MemoryLayerTenant},
		Allowed: []string{MemoryChannelWeb, MemoryChannelAPI, MemoryChannelIM, MemoryChannelEmbed},
	},
	{
		Key: SettingMemoryEmbedVisitorSpace, Group: MemoryGroupGeneral, Kind: memoryKindEnum,
		Default: MemoryEmbedSpaceSessionOnly, Merge: mergeStrictestEnum,
		Levels:  []string{MemoryLayerTenant},
		Allowed: memoryEmbedSpaceOrder,
	},
	{
		Key: SettingMemorySharedSpaceEnabled, Group: MemoryGroupGeneral, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},

	// -- Write / extraction -------------------------------------------------
	{
		Key: SettingMemoryWriteMode, Group: MemoryGroupWrite, Kind: memoryKindEnum,
		Default: MemoryWriteModeExplicit, Merge: mergeStrictestEnum,
		Levels:  []string{MemoryLayerTenant, MemoryLayerAgent, MemoryLayerUser, MemoryLayerSpace},
		Allowed: memoryWriteModeOrder,
	},
	{
		Key: SettingMemoryWriteRequireReview, Group: MemoryGroupWrite, Kind: memoryKindBool,
		Default: true, Merge: mergeOr,
		Levels: []string{MemoryLayerTenant, MemoryLayerUser},
	},
	{
		Key: SettingMemoryWriteAllowedTypes, Group: MemoryGroupWrite, Kind: memoryKindStringList,
		Default: AllMemoryTypes(), Merge: mergeIntersect,
		Levels:  []string{MemoryLayerTenant, MemoryLayerUser},
		Allowed: AllMemoryTypes(),
	},
	{
		Key: SettingMemoryWriteMinConfidence, Group: MemoryGroupWrite, Kind: memoryKindFloat,
		Default: 0.6, Merge: mergeMax,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(1),
	},
	{
		Key: SettingMemoryWriteGateKeywords, Group: MemoryGroupWrite, Kind: memoryKindStringList,
		Default: DefaultMemoryGateKeywords(), Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryWriteDebounce, Group: MemoryGroupWrite, Kind: memoryKindInt,
		Default: 60, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(5), Max: f64(3600),
	},
	{
		Key: SettingMemoryWriteTurnInterval, Group: MemoryGroupWrite, Kind: memoryKindInt,
		Default: 3, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(1), Max: f64(50),
	},
	{
		Key: SettingMemoryWriteMaxNotes, Group: MemoryGroupWrite, Kind: memoryKindInt,
		Default: 10, Merge: mergeMin,
		Levels: []string{MemoryLayerTenant}, Min: f64(1), Max: f64(50),
	},
	{
		Key: SettingMemoryWriteExtractModel, Group: MemoryGroupWrite, Kind: memoryKindString,
		Default: "", Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent},
	},

	// -- Recall / injection -------------------------------------------------
	{
		Key: SettingMemoryRecallEnabled, Group: MemoryGroupRecall, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent, MemoryLayerUser, MemoryLayerSpace},
	},
	{
		Key: SettingMemoryRecallRelevance, Group: MemoryGroupRecall, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent},
	},
	{
		Key: SettingMemoryRecallResidentTypes, Group: MemoryGroupRecall, Kind: memoryKindStringList,
		Default: []string{MemoryTypeProfile, MemoryTypePreference}, Merge: mergeOverride,
		Levels:  []string{MemoryLayerTenant, MemoryLayerUser},
		Allowed: AllMemoryTypes(),
	},
	{
		Key: SettingMemoryRecallMaxItems, Group: MemoryGroupRecall, Kind: memoryKindInt,
		Default: 8, Merge: mergeMin,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent}, Min: f64(1), Max: f64(30),
	},
	{
		Key: SettingMemoryRecallTokenBudget, Group: MemoryGroupRecall, Kind: memoryKindInt,
		Default: 600, Merge: mergeMin,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent}, Min: f64(50), Max: f64(1200),
	},
	{
		Key: SettingMemoryRecallTimeoutMs, Group: MemoryGroupRecall, Kind: memoryKindInt,
		Default: 300, Merge: mergeMin,
		Levels: []string{MemoryLayerDeployment, MemoryLayerTenant}, Min: f64(50), Max: f64(5000),
	},
	{
		Key: SettingMemoryRecallShowUsed, Group: MemoryGroupRecall, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerUser},
	},
	{
		Key: SettingMemoryRecallCite, Group: MemoryGroupRecall, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerUser},
	},

	// -- Personalised ranking ----------------------------------------------
	{
		Key: SettingMemoryBoostEnabled, Group: MemoryGroupBoost, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerAgent},
	},
	{
		Key: SettingMemoryBoostFactor, Group: MemoryGroupBoost, Kind: memoryKindFloat,
		Default: 1.25, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(1), Max: f64(2),
	},

	// -- Anchors / illumination --------------------------------------------
	{
		Key: SettingMemoryAnchorRuntime, Group: MemoryGroupAnchor, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryAnchorResolve, Group: MemoryGroupAnchor, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryOverlayEnabled, Group: MemoryGroupAnchor, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant, MemoryLayerUser},
	},
	{
		Key: SettingMemoryOverlayFamiliar, Group: MemoryGroupAnchor, Kind: memoryKindFloat,
		Default: 0.25, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(1),
	},
	{
		Key: SettingMemoryOverlayMastered, Group: MemoryGroupAnchor, Kind: memoryKindFloat,
		Default: 0.6, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(1),
	},
	{
		Key: SettingMemoryOverlayWeights, Group: MemoryGroupAnchor, Kind: memoryKindFloatMap,
		Default: DefaultMemoryRelationWeights(), Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Allowed: AllMemoryRelations(),
	},
	{
		Key: SettingMemoryOverlayHalfLife, Group: MemoryGroupAnchor, Kind: memoryKindInt,
		Default: 120, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(1), Max: f64(3650),
	},
	{
		Key: SettingMemoryOverlayDecayExempt, Group: MemoryGroupAnchor, Kind: memoryKindStringList,
		// owns and bookmarked express a standing relationship rather than a
		// recent interaction. Letting them decay produced the absurd result
		// that "I own this topic" dimmed after a quiet six months.
		Default: []string{MemoryRelationOwns, MemoryRelationBookmarked}, Merge: mergeOverride,
		Levels:  []string{MemoryLayerTenant},
		Allowed: AllMemoryRelations(),
	},

	// -- Decay / capacity / retention --------------------------------------
	{
		Key: SettingMemoryDecayEnabled, Group: MemoryGroupLifecycle, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryDecayHalfLives, Group: MemoryGroupLifecycle, Kind: memoryKindIntMap,
		Default: DefaultMemoryHalfLives(), Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Allowed: AllMemoryTypes(),
	},
	{
		Key: SettingMemoryDecayArchiveAt, Group: MemoryGroupLifecycle, Kind: memoryKindFloat,
		Default: 0.15, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(1),
	},
	{
		Key: SettingMemoryMaxPagesPerSpace, Group: MemoryGroupLifecycle, Kind: memoryKindInt,
		Default: 2000, Merge: mergeMin,
		Levels: []string{MemoryLayerTenant}, Min: f64(10), Max: f64(100000),
	},
	{
		Key: SettingMemoryRetentionDays, Group: MemoryGroupLifecycle, Kind: memoryKindInt,
		Default: 0, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(36500),
	},
	{
		Key: SettingMemoryPurgeArchivedAft, Group: MemoryGroupLifecycle, Kind: memoryKindInt,
		Default: 0, Merge: mergeOverride,
		Levels: []string{MemoryLayerTenant}, Min: f64(0), Max: f64(36500),
	},

	// -- Privacy / security -------------------------------------------------
	{
		Key: SettingMemoryPIIRedaction, Group: MemoryGroupPrivacy, Kind: memoryKindEnum,
		Default: MemoryPIIRedact, Merge: mergeStrictestEnum,
		Levels:  []string{MemoryLayerTenant},
		Allowed: memoryPIIOrder,
	},
	{
		Key: SettingMemoryBlockedPatterns, Group: MemoryGroupPrivacy, Kind: memoryKindStringList,
		Default: DefaultMemoryBlockedPatterns(), Merge: mergeUnion,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryAdminMetaOnly, Group: MemoryGroupPrivacy, Kind: memoryKindBool,
		Default: true, Merge: mergeOr, HardLocked: true,
		Levels: []string{},
	},
	{
		Key: SettingMemoryInjectSanitize, Group: MemoryGroupPrivacy, Kind: memoryKindBool,
		Default: true, Merge: mergeOr, HardLocked: true,
		Levels: []string{},
	},
	{
		Key: SettingMemoryExtractSources, Group: MemoryGroupPrivacy, Kind: memoryKindStringList,
		Default: []string{MemoryExtractSourceUserMessage}, Merge: mergeIntersect, HardLocked: true,
		Levels:  []string{},
		Allowed: []string{MemoryExtractSourceUserMessage},
	},
	{
		Key: SettingMemoryExportEnabled, Group: MemoryGroupPrivacy, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryForgetEnabled, Group: MemoryGroupPrivacy, Kind: memoryKindBool,
		Default: true, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},

	// -- Insights -----------------------------------------------------------
	{
		Key: SettingMemoryInsightsEnabled, Group: MemoryGroupInsights, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
	{
		Key: SettingMemoryInsightsKAnon, Group: MemoryGroupInsights, Kind: memoryKindInt,
		Default: 5, Merge: mergeMax,
		Levels: []string{MemoryLayerTenant}, Min: f64(3), Max: f64(1000),
	},
	{
		Key: SettingMemoryInsightsCoverage, Group: MemoryGroupInsights, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerUser},
	},
	{
		Key: SettingMemoryInsightsAutoFile, Group: MemoryGroupInsights, Kind: memoryKindBool,
		Default: false, Merge: mergeAnd,
		Levels: []string{MemoryLayerTenant},
	},
}

var memorySettingIndex = func() map[string]MemorySettingDescriptor {
	m := make(map[string]MemorySettingDescriptor, len(memorySettingDescriptors))
	for _, d := range memorySettingDescriptors {
		m[d.Key] = d
	}
	return m
}()

// MemorySettingDescriptors returns the catalogue, sorted by group then key, so
// the settings UI can be rendered directly from the API response.
func MemorySettingDescriptors() []MemorySettingDescriptor {
	out := make([]MemorySettingDescriptor, len(memorySettingDescriptors))
	copy(out, memorySettingDescriptors)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// LookupMemorySetting returns the descriptor for key.
func LookupMemorySetting(key string) (MemorySettingDescriptor, bool) {
	d, ok := memorySettingIndex[key]
	return d, ok
}

// DefaultMemoryRelationWeights returns the built-in anchor relation weights.
func DefaultMemoryRelationWeights() map[string]float64 {
	return map[string]float64{
		MemoryRelationMentioned:  0.5,
		MemoryRelationAskedAbout: 1.0,
		MemoryRelationBookmarked: 1.5,
		MemoryRelationDisagreed:  1.5,
		MemoryRelationLearned:    2.0,
		MemoryRelationCorrected:  2.5,
		MemoryRelationOwns:       3.0,
	}
}

// DefaultMemoryHalfLives returns the built-in per-type decay half-lives in days.
func DefaultMemoryHalfLives() map[string]int {
	return map[string]int{
		MemoryTypeProfile:      365,
		MemoryTypePreference:   365,
		MemoryTypeProject:      180,
		MemoryTypeEntity:       120,
		MemoryTypeTopic:        90,
		MemoryTypeEpisode:      90,
		MemoryTypeOpenQuestion: 30,
	}
}

// DefaultMemoryGateKeywords returns the multilingual phrases that signal the
// user is stating something worth keeping. The gate is deliberately cheap: it
// runs before any model call, so most turns cost nothing at all.
func DefaultMemoryGateKeywords() []string {
	return []string{
		// Chinese
		"记住", "记一下", "以后都", "下次", "我是", "我叫", "我们团队", "我负责",
		"不要再", "别再", "默认用", "我的习惯", "我偏好", "我喜欢", "我不喜欢",
		// English
		"remember", "from now on", "i am", "i'm", "my name is", "my team",
		"i prefer", "i like", "i don't like", "always use", "never use", "stop using",
	}
}

// DefaultMemoryBlockedPatterns returns regexes for content that must never be
// persisted as a memory.
func DefaultMemoryBlockedPatterns() []string {
	return []string{
		`(?i)\b(api[_-]?key|secret|password|passwd|token|credential)\b\s*[:=]\s*\S+`,
		`(?i)\bsk-[A-Za-z0-9]{16,}\b`,
		`-----BEGIN [A-Z ]*PRIVATE KEY-----`,
	}
}

// ---------------------------------------------------------------------------
// Patches
// ---------------------------------------------------------------------------

// MemorySettingsPatch is one layer's sparse set of overrides. Unknown keys are
// dropped on write, so a stale client cannot persist junk.
type MemorySettingsPatch map[string]any

// Value implements driver.Valuer.
func (p MemorySettingsPatch) Value() (driver.Value, error) {
	if p == nil {
		return "{}", nil
	}
	b, err := json.Marshal(map[string]any(p))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (p *MemorySettingsPatch) Scan(src any) error {
	raw, err := jsonScanBytes(src, "MemorySettingsPatch")
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		*p = nil
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*p = out
	return nil
}

// MemorySettingsLayer is a patch tagged with the layer that owns it.
type MemorySettingsLayer struct {
	Layer string
	Patch MemorySettingsPatch
}

// SanitizeMemoryPatch validates a patch for a given layer, returning the
// cleaned patch plus a list of human-readable notes about what was adjusted.
// Out-of-range numbers are clamped rather than rejected: a bad value should
// degrade to a safe one, not break the chat path.
func SanitizeMemoryPatch(layer string, patch MemorySettingsPatch) (MemorySettingsPatch, []string) {
	out := MemorySettingsPatch{}
	notes := make([]string, 0)
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		desc, ok := LookupMemorySetting(key)
		if !ok {
			notes = append(notes, fmt.Sprintf("%s: unknown setting, ignored", key))
			continue
		}
		// An explicit null means "stop overriding this and inherit again",
		// which is a different intent from "never set it" and has to survive
		// sanitisation so the caller can remove the key.
		if patch[key] == nil {
			out[key] = nil
			continue
		}
		if desc.HardLocked {
			notes = append(notes, fmt.Sprintf("%s: locked by the platform, ignored", key))
			continue
		}
		if !containsString(desc.Levels, layer) {
			notes = append(notes, fmt.Sprintf("%s: not settable at the %s level, ignored", key, layer))
			continue
		}
		value, note := coerceSettingValue(desc, patch[key])
		if note != "" {
			notes = append(notes, fmt.Sprintf("%s: %s", key, note))
		}
		if value == nil {
			continue
		}
		out[key] = value
	}
	return out, notes
}

func coerceSettingValue(desc MemorySettingDescriptor, raw any) (any, string) {
	switch desc.Kind {
	case memoryKindBool:
		b, ok := raw.(bool)
		if !ok {
			return nil, "expected a boolean, ignored"
		}
		return b, ""
	case memoryKindInt:
		n, ok := toFloat(raw)
		if !ok {
			return nil, "expected a number, ignored"
		}
		clamped, note := clamp(n, desc.Min, desc.Max)
		return int(clamped), note
	case memoryKindFloat:
		n, ok := toFloat(raw)
		if !ok {
			return nil, "expected a number, ignored"
		}
		clamped, note := clamp(n, desc.Min, desc.Max)
		return clamped, note
	case memoryKindString:
		s, ok := raw.(string)
		if !ok {
			return nil, "expected a string, ignored"
		}
		return strings.TrimSpace(s), ""
	case memoryKindEnum:
		s, ok := raw.(string)
		if !ok {
			return nil, "expected a string, ignored"
		}
		s = strings.TrimSpace(s)
		if !containsString(desc.Allowed, s) {
			return nil, "not an allowed value, ignored"
		}
		return s, ""
	case memoryKindStringList:
		list, ok := toStringList(raw)
		if !ok {
			return nil, "expected a list of strings, ignored"
		}
		if len(desc.Allowed) > 0 {
			filtered := make([]string, 0, len(list))
			dropped := false
			for _, v := range list {
				if containsString(desc.Allowed, v) {
					filtered = append(filtered, v)
				} else {
					dropped = true
				}
			}
			if dropped {
				return filtered, "unsupported entries were dropped"
			}
			return filtered, ""
		}
		return list, ""
	case memoryKindFloatMap:
		m, ok := toFloatMap(raw)
		if !ok {
			return nil, "expected an object of numbers, ignored"
		}
		return filterMapKeys(m, desc.Allowed), ""
	case memoryKindIntMap:
		m, ok := toFloatMap(raw)
		if !ok {
			return nil, "expected an object of numbers, ignored"
		}
		ints := make(map[string]int, len(m))
		for k, v := range filterMapKeys(m, desc.Allowed) {
			ints[k] = int(v)
		}
		return ints, ""
	default:
		return nil, "unsupported setting kind, ignored"
	}
}

func clamp(v float64, min, max *float64) (float64, string) {
	if min != nil && v < *min {
		return *min, fmt.Sprintf("raised to the minimum %g", *min)
	}
	if max != nil && v > *max {
		return *max, fmt.Sprintf("lowered to the maximum %g", *max)
	}
	return v, ""
}

// ---------------------------------------------------------------------------
// Resolution
// ---------------------------------------------------------------------------

// MemorySettingValue is one resolved setting as reported to the API.
type MemorySettingValue struct {
	Value any `json:"value"`
	// Source is the layer that produced the effective value, or "default".
	Source string `json:"source"`
	// LockedBy names the widest layer whose value constrains the outcome, so
	// the UI can say "your workspace disabled this" instead of showing a
	// switch that silently does nothing. Empty when nothing constrains it.
	LockedBy string `json:"locked_by,omitempty"`
}

// MemorySettingsResolution is the full resolved view.
type MemorySettingsResolution struct {
	Values   map[string]MemorySettingValue `json:"values"`
	Settings MemorySettings                `json:"-"`
}

// EditableAt reports whether the given layer can still meaningfully change key.
func (r MemorySettingsResolution) EditableAt(key, layer string) bool {
	desc, ok := LookupMemorySetting(key)
	if !ok || desc.HardLocked || !containsString(desc.Levels, layer) {
		return false
	}
	v, ok := r.Values[key]
	if !ok || v.LockedBy == "" {
		return true
	}
	// The key is already at its most restrictive value. A layer at least as
	// narrow as the one that pinned it can still relax its own contribution;
	// anything narrower than that is looking at a control that would do
	// nothing, and should be told so rather than left to discover it.
	return memoryLayerRank(layer) <= memoryLayerRank(v.LockedBy)
}

// ResolveMemorySettings folds the layers, widest first, into effective values.
func ResolveMemorySettings(layers ...MemorySettingsLayer) MemorySettingsResolution {
	values := make(map[string]MemorySettingValue, len(memorySettingDescriptors))

	for _, desc := range memorySettingDescriptors {
		current := MemorySettingValue{Value: desc.Default, Source: memoryLayerDefault}
		if desc.HardLocked {
			current.LockedBy = MemoryLayerDeployment
			values[desc.Key] = current
			continue
		}
		// The built-in default is not a layer. Combining it as if it were would
		// make defaults unoverridable in the restrictive direction: a key that
		// defaults to false and merges with AND could never be switched on, and
		// one that defaults to true and merges with OR could never be switched
		// off. So the first layer to set a key takes it outright, and only
		// subsequent layers combine.
		//
		// Deny lists are the exception: they accumulate onto the built-ins,
		// because dropping the shipped credential patterns the moment a
		// workspace adds one of its own would be a silent downgrade.
		explicitlySet := desc.Merge == mergeUnion

		for _, layer := range layers {
			raw, present := layer.Patch[desc.Key]
			if !present {
				continue
			}
			// Persisted patches are re-validated on read as well as on write:
			// descriptors evolve, and a value that was legal when stored may
			// no longer be. Dropping it silently falls back to the wider
			// layer, which is the safe direction.
			candidate, _ := coerceSettingValue(desc, raw)
			if candidate == nil {
				continue
			}
			if !explicitlySet {
				current = MemorySettingValue{Value: candidate, Source: layer.Layer}
				explicitlySet = true
				continue
			}
			current = mergeSettingValue(desc, current, candidate, layer.Layer)
		}
		current.LockedBy = lockedBy(desc, current)
		values[desc.Key] = current
	}

	return MemorySettingsResolution{Values: values, Settings: buildMemorySettings(values)}
}

// mergeSettingValue folds one layer's candidate into the running value.
//
// Only reached once a wider layer has already set the key, so every branch here
// is answering the same question: may this narrower layer change what the wider
// one decided, and in which direction.
func mergeSettingValue(
	desc MemorySettingDescriptor, current MemorySettingValue, candidate any, layer string,
) MemorySettingValue {
	switch desc.Merge {
	case mergeAnd:
		cur, _ := current.Value.(bool)
		cand, _ := candidate.(bool)
		if cur && !cand {
			// Turning something off is always allowed.
			return MemorySettingValue{Value: false, Source: layer}
		}
		return current

	case mergeOr:
		cur, _ := current.Value.(bool)
		cand, _ := candidate.(bool)
		if !cur && cand {
			// Turning a safety switch on is always allowed.
			return MemorySettingValue{Value: true, Source: layer}
		}
		return current

	case mergeMin:
		curF, _ := toFloat(current.Value)
		candF, _ := toFloat(candidate)
		if candF < curF {
			return MemorySettingValue{Value: candidate, Source: layer}
		}
		return current

	case mergeMax:
		curF, _ := toFloat(current.Value)
		candF, _ := toFloat(candidate)
		if candF > curF {
			return MemorySettingValue{Value: candidate, Source: layer}
		}
		return current

	case mergeStrictestEnum:
		curIdx := indexOf(desc.Allowed, toStringValue(current.Value))
		candIdx := indexOf(desc.Allowed, toStringValue(candidate))
		if candIdx < curIdx {
			return MemorySettingValue{Value: candidate, Source: layer}
		}
		return current

	case mergeIntersect:
		cur, _ := toStringList(current.Value)
		cand, _ := toStringList(candidate)
		return MemorySettingValue{Value: intersectStrings(cur, cand), Source: layer}

	case mergeUnion:
		cur, _ := toStringList(current.Value)
		cand, _ := toStringList(candidate)
		return MemorySettingValue{Value: unionStrings(cur, cand), Source: layer}

	default: // mergeOverride
		return MemorySettingValue{Value: candidate, Source: layer}
	}
}

// lockedBy names the layer that has pinned a key, or "" when narrower layers
// can still change it.
//
// A key is only reported as locked when it has already reached its most
// restrictive value, because that is the one case where a narrower layer's
// edit would do nothing at all. A budget capped by a workspace is not locked:
// an agent can still lower it further, and greying that control out would be a
// lie. Getting this distinction right is what makes the settings UI honest
// instead of merely defensive.
func lockedBy(desc MemorySettingDescriptor, value MemorySettingValue) string {
	if desc.HardLocked {
		return MemoryLayerDeployment
	}
	if value.Source == memoryLayerDefault {
		return ""
	}
	switch desc.Merge {
	case mergeAnd:
		if enabled, _ := value.Value.(bool); !enabled {
			return value.Source
		}
	case mergeOr:
		if enabled, _ := value.Value.(bool); enabled {
			return value.Source
		}
	case mergeStrictestEnum:
		if len(desc.Allowed) > 0 && toStringValue(value.Value) == desc.Allowed[0] {
			return value.Source
		}
	case mergeIntersect:
		if list, _ := toStringList(value.Value); len(list) == 0 {
			return value.Source
		}
	}
	return ""
}

// MemorySettings is the typed view the rest of the code reads. Building it once
// per request keeps hot paths free of map lookups and string keys.
type MemorySettings struct {
	Enabled            bool
	Channels           []string
	EmbedVisitorSpace  string
	SharedSpaceEnabled bool

	WriteMode         string
	RequireReview     bool
	AllowedTypes      []string
	MinConfidence     float64
	GateKeywords      []string
	DebounceSeconds   int
	TurnInterval      int
	MaxNotesPerWindow int
	ExtractionModelID string

	RecallEnabled        bool
	RelevanceRecall      bool
	ResidentTypes        []string
	RecallMaxItems       int
	InjectionTokenBudget int
	RecallTimeoutMs      int
	ShowUsedMemories     bool
	CiteMemories         bool

	BoostEnabled bool
	BoostFactor  float64

	AnchorRuntimeEnabled bool
	AnchorResolveEnabled bool
	OverlayEnabled       bool
	FamiliarThreshold    float64
	MasteredThreshold    float64
	RelationWeights      map[string]float64
	AnchorHalfLifeDays   int
	DecayExemptRelations []string

	DecayEnabled           bool
	HalfLifeDays           map[string]int
	ArchiveThreshold       float64
	MaxPagesPerSpace       int
	RetentionDays          int
	PurgeArchivedAfterDays int

	PIIRedaction      string
	BlockedPatterns   []string
	AdminMetadataOnly bool
	InjectionSanitize bool
	ExtractSources    []string
	ExportEnabled     bool
	ForgetEnabled     bool

	InsightsEnabled       bool
	InsightsKAnonymity    int
	MemberCoverageVisible bool
	AutoFileWikiIssues    bool
}

// DefaultMemorySettings returns the settings with no layer applied.
func DefaultMemorySettings() MemorySettings {
	return ResolveMemorySettings().Settings
}

// AutoExtractEnabled reports whether the write mode invokes the extractor.
func (s MemorySettings) AutoExtractEnabled() bool {
	return s.WriteMode == MemoryWriteModeGatedAuto || s.WriteMode == MemoryWriteModeAlwaysAuto
}

// WritesAllowed reports whether any memory may be written at all.
func (s MemorySettings) WritesAllowed() bool {
	return s.Enabled && s.WriteMode != MemoryWriteModeOff
}

// TypeAllowed reports whether memories of the given type may be stored.
func (s MemorySettings) TypeAllowed(t string) bool {
	return containsString(s.AllowedTypes, t)
}

// RelationWeight returns the configured weight for an anchor relation.
func (s MemorySettings) RelationWeight(relation string) float64 {
	if w, ok := s.RelationWeights[relation]; ok {
		return w
	}
	if w, ok := DefaultMemoryRelationWeights()[relation]; ok {
		return w
	}
	return 0
}

// RelationDecays reports whether an anchor relation ages out.
func (s MemorySettings) RelationDecays(relation string) bool {
	return !containsString(s.DecayExemptRelations, relation)
}

// HalfLifeFor returns the decay half-life in days for a memory type.
func (s MemorySettings) HalfLifeFor(pageType string) int {
	if d, ok := s.HalfLifeDays[pageType]; ok && d > 0 {
		return d
	}
	if d, ok := DefaultMemoryHalfLives()[pageType]; ok {
		return d
	}
	return 90
}

// ChannelAllowed reports whether a channel may own a persistent memory space.
func (s MemorySettings) ChannelAllowed(channel string) bool {
	return containsString(s.Channels, channel)
}

func buildMemorySettings(values map[string]MemorySettingValue) MemorySettings {
	return MemorySettings{
		Enabled:            boolAt(values, SettingMemoryEnabled),
		Channels:           listAt(values, SettingMemoryChannels),
		EmbedVisitorSpace:  stringAt(values, SettingMemoryEmbedVisitorSpace),
		SharedSpaceEnabled: boolAt(values, SettingMemorySharedSpaceEnabled),

		WriteMode:         stringAt(values, SettingMemoryWriteMode),
		RequireReview:     boolAt(values, SettingMemoryWriteRequireReview),
		AllowedTypes:      listAt(values, SettingMemoryWriteAllowedTypes),
		MinConfidence:     floatAt(values, SettingMemoryWriteMinConfidence),
		GateKeywords:      listAt(values, SettingMemoryWriteGateKeywords),
		DebounceSeconds:   intAt(values, SettingMemoryWriteDebounce),
		TurnInterval:      intAt(values, SettingMemoryWriteTurnInterval),
		MaxNotesPerWindow: intAt(values, SettingMemoryWriteMaxNotes),
		ExtractionModelID: stringAt(values, SettingMemoryWriteExtractModel),

		RecallEnabled:        boolAt(values, SettingMemoryRecallEnabled),
		RelevanceRecall:      boolAt(values, SettingMemoryRecallRelevance),
		ResidentTypes:        listAt(values, SettingMemoryRecallResidentTypes),
		RecallMaxItems:       intAt(values, SettingMemoryRecallMaxItems),
		InjectionTokenBudget: intAt(values, SettingMemoryRecallTokenBudget),
		RecallTimeoutMs:      intAt(values, SettingMemoryRecallTimeoutMs),
		ShowUsedMemories:     boolAt(values, SettingMemoryRecallShowUsed),
		CiteMemories:         boolAt(values, SettingMemoryRecallCite),

		BoostEnabled: boolAt(values, SettingMemoryBoostEnabled),
		BoostFactor:  floatAt(values, SettingMemoryBoostFactor),

		AnchorRuntimeEnabled: boolAt(values, SettingMemoryAnchorRuntime),
		AnchorResolveEnabled: boolAt(values, SettingMemoryAnchorResolve),
		OverlayEnabled:       boolAt(values, SettingMemoryOverlayEnabled),
		FamiliarThreshold:    floatAt(values, SettingMemoryOverlayFamiliar),
		MasteredThreshold:    floatAt(values, SettingMemoryOverlayMastered),
		RelationWeights:      floatMapAt(values, SettingMemoryOverlayWeights),
		AnchorHalfLifeDays:   intAt(values, SettingMemoryOverlayHalfLife),
		DecayExemptRelations: listAt(values, SettingMemoryOverlayDecayExempt),

		DecayEnabled:           boolAt(values, SettingMemoryDecayEnabled),
		HalfLifeDays:           intMapAt(values, SettingMemoryDecayHalfLives),
		ArchiveThreshold:       floatAt(values, SettingMemoryDecayArchiveAt),
		MaxPagesPerSpace:       intAt(values, SettingMemoryMaxPagesPerSpace),
		RetentionDays:          intAt(values, SettingMemoryRetentionDays),
		PurgeArchivedAfterDays: intAt(values, SettingMemoryPurgeArchivedAft),

		PIIRedaction:      stringAt(values, SettingMemoryPIIRedaction),
		BlockedPatterns:   listAt(values, SettingMemoryBlockedPatterns),
		AdminMetadataOnly: boolAt(values, SettingMemoryAdminMetaOnly),
		InjectionSanitize: boolAt(values, SettingMemoryInjectSanitize),
		ExtractSources:    listAt(values, SettingMemoryExtractSources),
		ExportEnabled:     boolAt(values, SettingMemoryExportEnabled),
		ForgetEnabled:     boolAt(values, SettingMemoryForgetEnabled),

		InsightsEnabled:       boolAt(values, SettingMemoryInsightsEnabled),
		InsightsKAnonymity:    intAt(values, SettingMemoryInsightsKAnon),
		MemberCoverageVisible: boolAt(values, SettingMemoryInsightsCoverage),
		AutoFileWikiIssues:    boolAt(values, SettingMemoryInsightsAutoFile),
	}
}

// ---------------------------------------------------------------------------
// Small typed helpers
// ---------------------------------------------------------------------------

func boolAt(values map[string]MemorySettingValue, key string) bool {
	v, _ := values[key].Value.(bool)
	return v
}

func intAt(values map[string]MemorySettingValue, key string) int {
	f, _ := toFloat(values[key].Value)
	return int(f)
}

func floatAt(values map[string]MemorySettingValue, key string) float64 {
	f, _ := toFloat(values[key].Value)
	return f
}

func stringAt(values map[string]MemorySettingValue, key string) string {
	return toStringValue(values[key].Value)
}

func listAt(values map[string]MemorySettingValue, key string) []string {
	l, _ := toStringList(values[key].Value)
	return l
}

func floatMapAt(values map[string]MemorySettingValue, key string) map[string]float64 {
	m, _ := toFloatMap(values[key].Value)
	return m
}

func intMapAt(values map[string]MemorySettingValue, key string) map[string]int {
	if m, ok := values[key].Value.(map[string]int); ok {
		return m
	}
	f, _ := toFloatMap(values[key].Value)
	out := make(map[string]int, len(f))
	for k, v := range f {
		out[k] = int(v)
	}
	return out
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func toStringValue(v any) string {
	s, _ := v.(string)
	return s
}

func toStringList(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}

func toFloatMap(v any) (map[string]float64, bool) {
	switch m := v.(type) {
	case map[string]float64:
		return m, true
	case map[string]int:
		out := make(map[string]float64, len(m))
		for k, val := range m {
			out[k] = float64(val)
		}
		return out, true
	case map[string]any:
		out := make(map[string]float64, len(m))
		for k, val := range m {
			f, ok := toFloat(val)
			if !ok {
				return nil, false
			}
			out[k] = f
		}
		return out, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}

func filterMapKeys(m map[string]float64, allowed []string) map[string]float64 {
	if len(allowed) == 0 {
		return m
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if containsString(allowed, k) {
			out[k] = v
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func indexOf(list []string, v string) int {
	for i, item := range list {
		if item == v {
			return i
		}
	}
	return len(list)
}

func intersectStrings(a, b []string) []string {
	out := make([]string, 0, len(b))
	for _, v := range b {
		if containsString(a, v) {
			out = append(out, v)
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, v := range b {
		if !containsString(out, v) {
			out = append(out, v)
		}
	}
	return out
}
