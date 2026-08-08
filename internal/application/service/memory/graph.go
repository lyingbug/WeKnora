package memory

import (
	"sort"

	"github.com/Tencent/WeKnora/internal/types"
)

// BuildMemoryGraph turns a space's pages (and, in bridged mode, its anchors)
// into the node/edge payload the graph canvas renders.
//
// A pure function over already-loaded slices, mirroring the wiki graph's
// computeGraphSubset: it makes the traversal and truncation rules testable
// without a database, and keeps the arithmetic identical on both engines.
func BuildMemoryGraph(
	pages []*types.MemoryPage, anchors []*types.MemoryAnchor, req *types.MemoryGraphRequest,
) *types.MemoryGraphData {
	req.Normalize()

	bySlug := make(map[string]*types.MemoryPage, len(pages))
	for _, page := range pages {
		if page.Status == types.MemoryPageStatusArchived {
			// Archived memories are still visible in the list, where the user
			// can restore them, but they no longer shape the graph.
			continue
		}
		bySlug[page.Slug] = page
	}

	typeAllowed := make(map[string]bool, len(req.Types))
	for _, t := range req.Types {
		typeAllowed[t] = true
	}
	hasTypeFilter := len(typeAllowed) > 0

	candidates := make([]*types.MemoryPage, 0, len(bySlug))
	for _, page := range bySlug {
		if hasTypeFilter && !typeAllowed[page.PageType] {
			continue
		}
		candidates = append(candidates, page)
	}

	total := len(candidates)
	selected := selectGraphSlice(bySlug, candidates, req, typeAllowed)

	data := &types.MemoryGraphData{
		Nodes: make([]types.MemoryGraphNode, 0, len(selected)),
		Edges: make([]types.MemoryGraphEdge, 0, len(selected)),
		Meta: types.MemoryGraphMeta{
			Mode: req.Mode, Total: total, Center: req.Center,
		},
	}
	if req.Center != "" {
		data.Meta.Depth = req.Depth
	}

	for slug := range selected {
		page := bySlug[slug]
		data.Nodes = append(data.Nodes, types.MemoryGraphNode{
			ID:        memoryNodeID(page.Slug),
			Kind:      types.MemoryGraphNodeMemory,
			Slug:      page.Slug,
			Title:     page.Title,
			Type:      page.PageType,
			LinkCount: len(page.InLinks) + len(page.OutLinks),
			Strength:  page.Strength,
		})
	}

	for slug := range selected {
		page := bySlug[slug]
		for _, target := range page.OutLinks {
			if _, ok := selected[target]; !ok {
				continue
			}
			data.Edges = append(data.Edges, types.MemoryGraphEdge{
				Source: memoryNodeID(page.Slug),
				Target: memoryNodeID(target),
				Kind:   types.MemoryGraphEdgeLink,
			})
		}
	}

	if req.Mode == types.MemoryGraphModeBridged {
		appendAnchorSatellites(data, bySlug, selected, anchors)
	}

	sortGraph(data)
	// Returned counts the memories shown, not the nodes drawn: Total counts
	// memory candidates, so including the wiki satellites made bridged mode
	// report things like "showing 40 of 25".
	data.Meta.Returned = len(selected)
	data.Meta.Truncated = total > len(selected)
	return data
}

// selectGraphSlice picks the visible node set: a breadth-first neighbourhood
// when the caller named a centre, otherwise the most connected pages.
func selectGraphSlice(
	bySlug map[string]*types.MemoryPage,
	candidates []*types.MemoryPage,
	req *types.MemoryGraphRequest,
	typeAllowed map[string]bool,
) map[string]struct{} {
	if req.Center != "" {
		if _, ok := bySlug[req.Center]; ok {
			return bfsNeighbourhood(bySlug, req.Center, req.Depth, req.Limit, typeAllowed)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		li := len(candidates[i].InLinks) + len(candidates[i].OutLinks)
		lj := len(candidates[j].InLinks) + len(candidates[j].OutLinks)
		if li != lj {
			return li > lj
		}
		if candidates[i].Strength != candidates[j].Strength {
			return candidates[i].Strength > candidates[j].Strength
		}
		// Deterministic tiebreak keeps the API stable between identical calls.
		return candidates[i].Slug < candidates[j].Slug
	})
	if req.Limit > 0 && len(candidates) > req.Limit {
		candidates = candidates[:req.Limit]
	}

	selected := make(map[string]struct{}, len(candidates))
	for _, page := range candidates {
		selected[page.Slug] = struct{}{}
	}
	return selected
}

func bfsNeighbourhood(
	bySlug map[string]*types.MemoryPage,
	center string, depth, limit int, typeAllowed map[string]bool,
) map[string]struct{} {
	selected := map[string]struct{}{center: {}}
	frontier := []string{center}

	for level := 0; level < depth && len(frontier) > 0; level++ {
		next := make([]string, 0, len(frontier))
		for _, slug := range frontier {
			page, ok := bySlug[slug]
			if !ok {
				continue
			}
			neighbours := append(append([]string{}, page.OutLinks...), page.InLinks...)
			for _, neighbour := range neighbours {
				if _, seen := selected[neighbour]; seen {
					continue
				}
				target, ok := bySlug[neighbour]
				if !ok {
					continue
				}
				if len(typeAllowed) > 0 && !typeAllowed[target.PageType] {
					continue
				}
				if limit > 0 && len(selected) >= limit {
					return selected
				}
				selected[neighbour] = struct{}{}
				next = append(next, neighbour)
			}
		}
		frontier = next
	}
	return selected
}

// appendAnchorSatellites adds the knowledge-base pages the selected memories
// point at, as a second family of nodes.
//
// This is the visual form of the whole idea: on one canvas the user sees what
// they know and where it attaches to what the organisation knows.
func appendAnchorSatellites(
	data *types.MemoryGraphData,
	bySlug map[string]*types.MemoryPage,
	selected map[string]struct{},
	anchors []*types.MemoryAnchor,
) {
	pageIDToSlug := make(map[string]string, len(bySlug))
	for slug := range selected {
		if page, ok := bySlug[slug]; ok {
			pageIDToSlug[page.ID] = slug
		}
	}

	satellites := map[string]*types.MemoryGraphNode{}
	for _, anchor := range anchors {
		if anchor.TargetKind != types.MemoryAnchorTargetWikiPage {
			continue
		}
		memorySlug, ok := pageIDToSlug[anchor.MemoryPageID]
		if !ok {
			continue
		}
		nodeID := wikiNodeID(anchor.KnowledgeBaseID, anchor.TargetRef)
		if _, exists := satellites[nodeID]; !exists {
			satellites[nodeID] = &types.MemoryGraphNode{
				ID:              nodeID,
				Kind:            types.MemoryGraphNodeWiki,
				Slug:            anchor.TargetRef,
				Title:           anchor.TargetRef,
				KnowledgeBaseID: anchor.KnowledgeBaseID,
			}
		}
		satellites[nodeID].LinkCount++
		data.Edges = append(data.Edges, types.MemoryGraphEdge{
			Source:   memoryNodeID(memorySlug),
			Target:   nodeID,
			Kind:     types.MemoryGraphEdgeAnchor,
			Relation: anchor.Relation,
		})
	}

	for _, node := range satellites {
		data.Nodes = append(data.Nodes, *node)
	}
}

func sortGraph(data *types.MemoryGraphData) {
	sort.Slice(data.Nodes, func(i, j int) bool {
		if data.Nodes[i].Kind != data.Nodes[j].Kind {
			// Memory nodes first so the canvas lays the user's own graph out
			// before hanging satellites off it.
			return data.Nodes[i].Kind == types.MemoryGraphNodeMemory
		}
		if data.Nodes[i].LinkCount != data.Nodes[j].LinkCount {
			return data.Nodes[i].LinkCount > data.Nodes[j].LinkCount
		}
		return data.Nodes[i].ID < data.Nodes[j].ID
	})
	sort.Slice(data.Edges, func(i, j int) bool {
		if data.Edges[i].Source != data.Edges[j].Source {
			return data.Edges[i].Source < data.Edges[j].Source
		}
		return data.Edges[i].Target < data.Edges[j].Target
	})
}

func memoryNodeID(slug string) string { return types.MemoryGraphNodeMemory + ":" + slug }

func wikiNodeID(kbID, slug string) string {
	return types.MemoryGraphNodeWiki + ":" + kbID + ":" + slug
}
