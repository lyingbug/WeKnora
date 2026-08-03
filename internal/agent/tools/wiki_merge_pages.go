package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type wikiMergePagesTool struct {
	BaseTool
	wikiPageService interfaces.WikiPageService
	kbIDs           []string
	routes          *WikiRouteResolver
}

// NewWikiMergePagesTool creates the wiki_merge_pages tool.
//
// It is what makes a "these two pages are the same subject" finding repairable at
// all. Doing it with the existing tools — write one page, delete the other — would
// drop the absorbed page's aliases, source documents and citations, so the next
// ingest of those documents would recreate the duplicate and the finding would
// come straight back.
func NewWikiMergePagesTool(
	wikiPageService interfaces.WikiPageService,
	kbIDs []string,
	routes ...*WikiRouteResolver,
) types.Tool {
	return &wikiMergePagesTool{
		BaseTool: NewBaseTool(
			ToolWikiMergePages,
			"Merge two Wiki pages that describe the same subject into one. The target page "+
				"survives with the content you supply and absorbs the other page's aliases, "+
				"source documents and citations; the other page is deleted and every link to "+
				"it is repointed at the target. Read both pages first and compose merged "+
				"content that keeps every fact worth keeping — the absorbed page cannot be "+
				"recovered.",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_slug": {
						"type": "string",
						"description": "Slug of the page that survives the merge"
					},
					"source_slug": {
						"type": "string",
						"description": "Slug of the page that is absorbed and then deleted"
					},
					"content": {
						"type": "string",
						"description": "Full markdown body for the surviving page, combining both pages. Required."
					},
					"summary": {
						"type": "string",
						"description": "Optional one-line summary for the surviving page. Omit to keep its existing summary."
					}
				},
				"required": ["target_slug", "source_slug", "content"]
			}`),
		),
		wikiPageService: wikiPageService,
		kbIDs:           kbIDs,
		routes:          firstWikiRoute(routes),
	}
}

func (t *wikiMergePagesTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	// Attribute every page write performed by this tool to the agent so revision
	// history distinguishes agent edits from pipeline/user ones.
	ctx = types.WithWikiEditSource(ctx, types.WikiEditSourceAgent)
	var params struct {
		TargetSlug string `json:"target_slug"`
		SourceSlug string `json:"source_slug"`
		Content    string `json:"content"`
		Summary    string `json:"summary"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to parse arguments: " + err.Error()}, nil
	}
	if len(t.kbIDs) == 0 {
		return &types.ToolResult{Success: false, Error: "No knowledge bases available for editing"}, nil
	}
	if strings.TrimSpace(params.Content) == "" {
		return &types.ToolResult{
			Success: false,
			Error: "content is required: the merged page must be composed deliberately, " +
				"since the absorbed page cannot be recovered",
		}, nil
	}
	targetSlug, err := normalizeAndValidateWikiSlug(params.TargetSlug)
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}
	sourceSlug, err := normalizeAndValidateWikiSlug(params.SourceSlug)
	if err != nil {
		return &types.ToolResult{Success: false, Error: err.Error()}, nil
	}
	if targetSlug == sourceSlug {
		return &types.ToolResult{Success: false, Error: "target_slug and source_slug must differ"}, nil
	}

	// Both pages are resolved before anything is written, so a typo in either
	// slug fails the call rather than half-applying a merge.
	_, kbID, err := resolveUniqueWikiPage(ctx, t.wikiPageService, targetSlug, t.kbIDs, t.routes)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to resolve target page: " + err.Error()}, nil
	}
	source, sourceKBID, err := resolveUniqueWikiPage(ctx, t.wikiPageService, sourceSlug, t.kbIDs, t.routes)
	if err != nil {
		return &types.ToolResult{Success: false, Error: "Failed to resolve page to merge: " + err.Error()}, nil
	}
	if sourceKBID != kbID {
		return &types.ToolResult{
			Success: false,
			Error:   "Both pages must live in the same knowledge base to be merged",
		}, nil
	}

	inLinks := make([]string, len(source.InLinks))
	copy(inLinks, source.InLinks)

	// Repoint inbound links before the merge, because this is the only part that
	// can be rolled back. If the merge itself then fails, the wiki is left intact
	// apart from links that now point at the page the reader wanted anyway.
	changes, updatedSlugs, rewriteErr := applyIncomingWikiContentRewrite(
		ctx, t.wikiPageService, kbID, inLinks,
		func(content string) (string, bool) {
			updated := strings.ReplaceAll(content, "[["+sourceSlug+"]]", "[["+targetSlug+"]]")
			updated = strings.ReplaceAll(updated, "[["+sourceSlug+"|", "[["+targetSlug+"|")
			return updated, updated != content
		},
	)
	if rewriteErr != nil {
		rollbackErr := rollbackWikiContentChanges(ctx, t.wikiPageService, changes)
		return &types.ToolResult{
			Success: false,
			Error: "Merge aborted while repointing incoming links: " +
				joinWikiMutationErrors(rewriteErr, rollbackErr),
		}, nil
	}

	merged, mergeErr := t.wikiPageService.MergePages(ctx, types.WikiPageMergeRequest{
		KnowledgeBaseID: kbID,
		TargetSlug:      targetSlug,
		SourceSlug:      sourceSlug,
		Content:         params.Content,
		Summary:         params.Summary,
	})
	if mergeErr != nil {
		rollbackErr := rollbackWikiContentChanges(ctx, t.wikiPageService, changes)
		return &types.ToolResult{
			Success: false,
			Error:   "Merge failed: " + joinWikiMutationErrors(mergeErr, rollbackErr),
		}, nil
	}
	t.routes.forget(sourceSlug, kbID)
	t.routes.remember(targetSlug, kbID)

	out, _ := json.Marshal(map[string]interface{}{
		"merged_into":          merged.Slug,
		"absorbed":             sourceSlug,
		"version":              merged.Version,
		"aliases":              merged.Aliases,
		"source_refs":          merged.SourceRefs,
		"repointed_link_pages": updatedSlugs,
	})
	return &types.ToolResult{
		Success: true,
		Output: fmt.Sprintf(
			"Merged [[%s]] into [[%s]] (now v%d). %d page(s) had their links repointed. "+
				"The absorbed page's title is now an alias of the survivor.\n%s",
			sourceSlug, targetSlug, merged.Version, len(updatedSlugs), string(out),
		),
	}, nil
}
