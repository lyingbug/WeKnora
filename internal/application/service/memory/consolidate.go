package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// consolidateInterval is the minimum wait between whole-store reviews. This
	// is maintenance, not a feature the user is waiting on, and every run costs
	// a model call, so it is deliberately infrequent.
	consolidateInterval = 24 * time.Hour
	// consolidateMinItems is the store size below which there is nothing worth
	// reviewing: a handful of memories cannot have drifted into contradiction.
	consolidateMinItems = 6
	// consolidateMaxClusters bounds the model calls one review makes.
	consolidateMaxClusters = 3
	// consolidateMinOverlap is how similar two memories must be before they are
	// even considered for merging. Set high on purpose: merging two memories
	// that only looked alike destroys information the user gave us, and a
	// missed merge merely leaves the store slightly redundant.
	consolidateMinOverlap = 0.55
	// staleTaskAge is how long a task can go unmentioned before it stops
	// competing for space. "I'm refactoring payments this week" is worth
	// recalling this week and misleading three months later.
	staleTaskAge = 45 * 24 * time.Hour
)

// consolidateIfDue reviews the whole store for one subject, at most once a day.
//
// Distillation only ever looks at the newest conversation, which is the right
// scope for a single turn and the wrong scope for noticing that five turns
// across three weeks have recorded the same preference five slightly different
// ways, or that a task from last quarter is now just noise. Both Generative
// Agents' reflection step and MemoryOS's segmented store treat this offline
// pass as a separate stage for the same reason: no per-turn call can see it.
//
// It never runs on the request path.
func (s *Service) consolidateIfDue(
	ctx context.Context, scope interfaces.MemoryScope, cfg *types.MemoryConfig,
) {
	subject, err := s.repo.GetSubject(ctx, scope)
	if err != nil || subject == nil {
		return
	}
	if subject.ConsolidatedAt != nil && time.Since(*subject.ConsolidatedAt) < consolidateInterval {
		return
	}

	// Expiry first: an expired task should not be a merge candidate.
	if archived, err := s.repo.ExpireOverdue(ctx, scope); err != nil {
		logger.Warnf(ctx, "memory: expire overdue failed: %v", err)
	} else if archived > 0 {
		logger.Infof(ctx, "memory: archived %d expired memories for %s", archived, scope.SubjectID)
	}

	items, _, err := s.repo.ListItems(ctx, scope, types.MemoryStatusActive, 200, 0)
	if err != nil {
		logger.Warnf(ctx, "memory: consolidation list failed: %v", err)
		return
	}

	demoted := s.demoteStaleTasks(ctx, scope, items)
	merged := 0
	if len(items) >= consolidateMinItems {
		merged = s.mergeRedundant(ctx, scope, cfg, items)
	}

	if err := s.repo.MarkConsolidated(ctx, scope); err != nil {
		logger.Warnf(ctx, "memory: mark consolidated failed: %v", err)
	}
	if merged > 0 || demoted > 0 {
		s.rebuildBlock(ctx, scope)
		logger.Infof(ctx, "memory: consolidation merged %d and demoted %d for %s",
			merged, demoted, scope.SubjectID)
	}
}

// demoteStaleTasks lowers the importance of tasks nobody has mentioned in
// months.
//
// Deleting them would be wrong — the user never said they finished, and we do
// not delete what we were told. Lowering importance is enough: it drops them
// out of the resident block and makes them the first to go when the store hits
// its cap, while leaving them visible and explainable in the memory manager.
func (s *Service) demoteStaleTasks(
	ctx context.Context, scope interfaces.MemoryScope, items []*types.MemoryItem,
) int {
	cutoff := time.Now().Add(-staleTaskAge)
	demoted := 0
	for _, item := range items {
		if item == nil || item.Kind != types.MemoryKindTask || item.Importance <= 1 {
			continue
		}
		last := item.ValidFrom
		if item.LastUsedAt != nil && item.LastUsedAt.After(last) {
			last = *item.LastUsedAt
		}
		if last.After(cutoff) {
			continue
		}
		err := s.repo.UpdateItemContent(ctx, scope, item.ID, item.Content, item.NormalizedKey, 1)
		if err != nil {
			logger.Warnf(ctx, "memory: demote stale task failed: %v", err)
			continue
		}
		demoted++
	}
	return demoted
}

// mergeRedundant folds groups of near-duplicate memories into one statement.
func (s *Service) mergeRedundant(
	ctx context.Context, scope interfaces.MemoryScope, cfg *types.MemoryConfig, items []*types.MemoryItem,
) int {
	clusters := clusterSimilar(items)
	if len(clusters) == 0 {
		return 0
	}
	if len(clusters) > consolidateMaxClusters {
		clusters = clusters[:consolidateMaxClusters]
	}

	merged := 0
	for _, cluster := range clusters {
		statement, ok := s.callConsolidationModel(ctx, cfg, cluster)
		if !ok {
			continue
		}
		statement = types.SanitizeMemoryContent(statement)
		if statement == "" {
			continue
		}
		primary := cluster[0]
		replacement, err := s.write(ctx, scope, cfg, types.MemoryItem{
			Kind:            primary.Kind,
			Topic:           primary.Topic,
			Content:         statement,
			Importance:      primary.Importance,
			Origin:          primary.Origin,
			SourceSessionID: primary.SourceSessionID,
			SourceMessageID: primary.SourceMessageID,
		})
		if err != nil || replacement == nil {
			if err != nil {
				logger.Warnf(ctx, "memory: consolidation write failed: %v", err)
			}
			continue
		}
		// Supersede rather than delete: the old wording keeps its dates, so the
		// memory manager can still explain what this statement used to be and
		// when it changed.
		for _, item := range cluster {
			if item.ID == replacement.ID {
				continue
			}
			if err := s.repo.SupersedeItem(ctx, scope, item.ID, replacement.ID); err != nil {
				logger.Warnf(ctx, "memory: supersede during consolidation failed: %v", err)
			}
		}
		merged++
	}
	return merged
}

// clusterSimilar groups memories of the same kind that say nearly the same
// thing. Groups of one are not returned.
func clusterSimilar(items []*types.MemoryItem) [][]*types.MemoryItem {
	var clusters [][]*types.MemoryItem
	taken := make(map[string]bool, len(items))

	for i, item := range items {
		if item == nil || taken[item.ID] {
			continue
		}
		base := tokenize(item.Topic + " " + item.Content)
		if len(base) == 0 {
			continue
		}
		group := []*types.MemoryItem{item}
		for _, other := range items[i+1:] {
			if other == nil || taken[other.ID] || other.Kind != item.Kind {
				continue
			}
			if jaccard(base, tokenize(other.Topic+" "+other.Content)) < consolidateMinOverlap {
				continue
			}
			group = append(group, other)
			taken[other.ID] = true
		}
		if len(group) < 2 {
			continue
		}
		taken[item.ID] = true
		clusters = append(clusters, group)
	}
	return clusters
}

// jaccard is the overlap between two token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, token := range a {
		set[token] = struct{}{}
	}
	shared := 0
	seen := make(map[string]struct{}, len(b))
	for _, token := range b {
		if _, dup := seen[token]; dup {
			continue
		}
		seen[token] = struct{}{}
		if _, ok := set[token]; ok {
			shared++
		}
	}
	union := len(set) + len(seen) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

const consolidationSystemPrompt = `你在整理一个人的长期记忆。下面几条记录说的是同一件事，请合并成一条。

规则：
- 只用这些记录里已有的信息，不要补充、不要推测。
- 如果它们互相矛盾，以日期最新的一条为准。
- 保留最具体的细节（具体的名称、数字、版本），丢掉重复的说法。
- 用记录本身的语言，一句话，不超过 60 字。
- 只输出 JSON：{"statement":"合并后的一句话"}
- 如果这些记录其实不是同一件事，输出 {"statement":""}。`

var consolidationSchema = json.RawMessage(`{
  "type": "object",
  "properties": {"statement": {"type": "string"}},
  "required": ["statement"]
}`)

// callConsolidationModel asks the model to merge one cluster.
func (s *Service) callConsolidationModel(
	ctx context.Context, cfg *types.MemoryConfig, cluster []*types.MemoryItem,
) (string, bool) {
	modelID := cfg.ExtractModelID
	if modelID == "" {
		return "", false
	}
	chatModel, err := s.modelService.GetChatModel(ctx, modelID)
	if err != nil || chatModel == nil {
		logger.Warnf(ctx, "memory: consolidation model unavailable: %v", err)
		return "", false
	}

	var b strings.Builder
	for _, item := range cluster {
		b.WriteString(fmt.Sprintf("- (%s) %s\n",
			item.ValidFrom.Format("2006-01-02"), types.SanitizeMemoryContent(item.Content)))
	}

	// Thinking off, for the reason given on completeExtraction: a reasoning
	// model spends this whole budget on its own deliberation and returns
	// nothing, which here would silently skip every merge.
	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: consolidationSystemPrompt},
		{Role: "user", Content: b.String()},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: 600,
		Thinking:            &thinking,
		Format:              consolidationSchema,
	})
	if err != nil || response == nil {
		logger.Warnf(ctx, "memory: consolidation call failed: %v", err)
		return "", false
	}

	content := strings.TrimSpace(response.Content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", false
	}
	var parsed struct {
		Statement string `json:"statement"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &parsed); err != nil {
		return "", false
	}
	statement := strings.TrimSpace(parsed.Statement)
	return statement, statement != ""
}
