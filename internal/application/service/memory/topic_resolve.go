package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// topicFuzzyThreshold is where character-bigram overlap alone is enough to
	// call two labels the same subject.
	//
	// Set high on purpose. Merging two topics that are not the same thing
	// corrupts the count that decides what becomes a memory, and it is
	// invisible when it happens. A missed merge only delays a promotion, and
	// the tier below catches most of them anyway. Graphiti holds its
	// deterministic tier at a comparable level for the same reason.
	topicFuzzyThreshold = 0.80
	// topicCandidateLimit bounds how many existing topics are shown to the
	// adjudicating model. One person's topic list is small; this is a guard
	// against a pathological account, not a normal working limit.
	topicCandidateLimit = 40
	// topicMaxAliases bounds the alias list on one topic.
	topicMaxAliases = 12
)

// topicResolution is where one surface form ended up.
type topicResolution struct {
	// Canonical is the existing topic this label belongs to, or nil when it is
	// genuinely a new subject.
	Canonical *types.MemoryTopicStat
	// Surface is what the model actually said, recorded as an alias when it
	// differs from the canonical label.
	Surface string
	// Tier records which rule decided, for logs and for tests that need to
	// assert an expensive tier was not reached.
	Tier string
}

// resolveTopics maps the labels one extraction run produced onto the subjects
// this person already has.
//
// The problem this solves is that a model asked to name a topic will not name
// it the same way twice: "儿童游泳赛事组织" one run, "少儿游泳比赛筹办" the
// next. Treating the string as an identity means the same subject is counted
// under several keys and never reaches the promotion threshold — the feature
// looks enabled and learns nothing.
//
// The fix is the one both mem0 and Graphiti converged on: never trust the
// surface string, resolve it against what already exists, cheapest test first.
//
//	tier 1  normalised equality, including previously recorded aliases
//	tier 2  character-bigram overlap, gated so short labels do not match loosely
//	tier 3  one batched model call over the remaining labels
//
// Tier 3 is the only one that costs anything, and it is usually skipped: the
// extraction prompt already shows the model this person's existing topics and
// asks it to reuse a label verbatim, so most runs resolve at tier 1.
func (s *Service) resolveTopics(
	ctx context.Context,
	scope interfaces.MemoryScope,
	cfg *types.MemoryConfig,
	surfaces []string,
) []topicResolution {
	if len(surfaces) == 0 {
		return nil
	}
	existing, err := s.repo.TopTopics(ctx, scope, topicCandidateLimit)
	if err != nil {
		logger.Warnf(ctx, "memory: load existing topics failed: %v", err)
		existing = nil
	}

	resolutions := make([]topicResolution, 0, len(surfaces))
	var unresolved []int

	for _, surface := range surfaces {
		resolution := topicResolution{Surface: surface}
		if match := matchTopicExactly(surface, existing); match != nil {
			resolution.Canonical = match
			resolution.Tier = "exact"
		} else if match := matchTopicLoosely(surface, existing); match != nil {
			resolution.Canonical = match
			resolution.Tier = "fuzzy"
		} else {
			unresolved = append(unresolved, len(resolutions))
		}
		resolutions = append(resolutions, resolution)
	}

	if len(unresolved) > 0 && len(existing) > 0 {
		s.adjudicateTopics(ctx, cfg, existing, resolutions, unresolved)
	}

	// Two labels in the same run can be the same new subject. Without this the
	// run creates two rows that every later run then has to keep apart.
	collapseNewTopicsWithinRun(resolutions)

	return resolutions
}

// matchTopicExactly is tier 1: the normalised label, or any wording that has
// already been resolved to this topic before.
func matchTopicExactly(surface string, existing []*types.MemoryTopicStat) *types.MemoryTopicStat {
	key := types.NormalizeTopicKey(surface)
	if key == "" {
		return nil
	}
	for _, stat := range existing {
		if stat == nil {
			continue
		}
		if stat.NormalizedKey == key || stat.Aliases.Has(surface) {
			return stat
		}
	}
	return nil
}

// matchTopicLoosely is tier 2: high character-bigram overlap, and only for
// labels specific enough that the overlap means something.
func matchTopicLoosely(surface string, existing []*types.MemoryTopicStat) *types.MemoryTopicStat {
	if !types.TopicIsSpecificEnoughToMatchLoosely(surface) {
		return nil
	}
	var (
		best      *types.MemoryTopicStat
		bestScore float64
	)
	for _, stat := range existing {
		if stat == nil || !types.TopicIsSpecificEnoughToMatchLoosely(stat.Topic) {
			continue
		}
		score := types.TopicSimilarity(surface, stat.Topic)
		if score > bestScore {
			best, bestScore = stat, score
		}
	}
	if bestScore < topicFuzzyThreshold {
		return nil
	}
	return best
}

const topicAdjudicationPrompt = `你在维护一个人的关注主题列表。下面给出「已有主题」和「新出现的说法」。

对每个新说法，判断它说的是不是已有主题里的同一件事。

判断标准：
- 同义或换个说法（「少儿游泳比赛筹办」和「儿童游泳赛事组织」）算同一件事。
- 同一领域但问的不是一回事（「PostgreSQL 连接池」和「PostgreSQL 备份恢复」）不算。
- 一个是另一个的具体子话题时，算同一件事，归到已有主题。
- 拿不准就算不同。合并错了会把两件事的计数混在一起，且事后看不出来；没合并只是暂时多一条。

只输出 JSON：{"resolutions":[{"index":<新说法的序号>,"same_as":<已有主题的序号，没有则 null>}]}`

var topicAdjudicationSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "resolutions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "index": {"type": "integer"},
          "same_as": {"type": ["integer", "null"]}
        },
        "required": ["index", "same_as"]
      }
    }
  },
  "required": ["resolutions"]
}`)

// adjudicateTopics is tier 3: ask the model whether the labels nothing matched
// are really new subjects.
//
// It runs once per extraction run over every unresolved label at once, rather
// than once per label, because the cost that matters here is the round trip and
// the decision is the same shape for all of them.
func (s *Service) adjudicateTopics(
	ctx context.Context,
	cfg *types.MemoryConfig,
	existing []*types.MemoryTopicStat,
	resolutions []topicResolution,
	unresolved []int,
) {
	if cfg == nil || cfg.ExtractModelID == "" {
		return
	}
	chatModel, err := s.modelService.GetChatModel(ctx, cfg.ExtractModelID)
	if err != nil || chatModel == nil {
		logger.Warnf(ctx, "memory: topic adjudication model unavailable: %v", err)
		return
	}

	var b strings.Builder
	b.WriteString("已有主题：\n")
	for i, stat := range existing {
		fmt.Fprintf(&b, "[%d] %s\n", i, stat.Topic)
	}
	b.WriteString("\n新出现的说法：\n")
	for _, idx := range unresolved {
		fmt.Fprintf(&b, "[%d] %s\n", idx, resolutions[idx].Surface)
	}

	response, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "system", Content: topicAdjudicationPrompt},
		{Role: "user", Content: b.String()},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: 400,
		Format:              topicAdjudicationSchema,
	})
	if err != nil || response == nil {
		logger.Warnf(ctx, "memory: topic adjudication failed: %v", err)
		return
	}

	var parsed struct {
		Resolutions []struct {
			Index  int  `json:"index"`
			SameAs *int `json:"same_as"`
		} `json:"resolutions"`
	}
	content := strings.TrimSpace(response.Content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &parsed); err != nil {
		logger.Warnf(ctx, "memory: unparsable topic adjudication: %v", err)
		return
	}

	pending := make(map[int]struct{}, len(unresolved))
	for _, idx := range unresolved {
		pending[idx] = struct{}{}
	}
	for _, decision := range parsed.Resolutions {
		// Only labels this call was actually asked about may be reassigned. A
		// model that returns an index it was not given must not be able to
		// overwrite a match an earlier, more reliable tier already made.
		if _, ok := pending[decision.Index]; !ok {
			continue
		}
		if decision.SameAs == nil {
			continue
		}
		target := *decision.SameAs
		if target < 0 || target >= len(existing) {
			continue
		}
		resolutions[decision.Index].Canonical = existing[target]
		resolutions[decision.Index].Tier = "model"
	}
}

// collapseNewTopicsWithinRun points near-identical new labels from one run at
// the same surface form, so they become one row rather than two.
func collapseNewTopicsWithinRun(resolutions []topicResolution) {
	for i := range resolutions {
		if resolutions[i].Canonical != nil {
			continue
		}
		for j := 0; j < i; j++ {
			if resolutions[j].Canonical != nil {
				continue
			}
			if types.NormalizeTopicKey(resolutions[i].Surface) ==
				types.NormalizeTopicKey(resolutions[j].Surface) {
				resolutions[i].Surface = resolutions[j].Surface
				break
			}
		}
	}
}
