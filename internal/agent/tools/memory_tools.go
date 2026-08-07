package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Long-term memory tools.
//
// The agent gets a deliberately narrow surface: search, read, remember, forget.
// There is no tool for editing an arbitrary field or for writing structured
// preferences, because those are the parts that steer generation and they
// belong to the user, not to a model reasoning inside a turn.
//
// Every tool operates on the caller's own space, resolved from the request
// principal by the service. No tool accepts a space identifier, so an agent
// cannot be argued into reading someone else's memory.

const (
	// ToolMemorySearch finds the caller's memories by keyword.
	ToolMemorySearch = "memory_search"
	// ToolMemoryReadPage reads one memory in full.
	ToolMemoryReadPage = "memory_read_page"
	// ToolMemoryRemember stores something the user asked to be remembered.
	ToolMemoryRemember = "memory_remember"
	// ToolMemoryForget deletes a memory.
	ToolMemoryForget = "memory_forget"
)

// ---------------------------------------------------------------------------
// memory_search
// ---------------------------------------------------------------------------

type memorySearchTool struct {
	BaseTool
	memoryService interfaces.MemoryService
}

// NewMemorySearchTool creates the memory search tool.
func NewMemorySearchTool(memoryService interfaces.MemoryService) types.Tool {
	return &memorySearchTool{
		BaseTool: NewBaseTool(
			ToolMemorySearch,
			`Search the user's long-term memory for what you already know about them.
Use this when the answer depends on the user's own context — their role, preferences, projects, past decisions or open questions — and that context is not already in the conversation.
Returns the user's own recorded statements, not knowledge-base content.`,
			json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "What to look for, in the user's own terms"
    },
    "limit": {
      "type": "integer",
      "description": "Maximum memories to return (default 8, max 30)"
    }
  },
  "required": ["query"]
}`),
		),
		memoryService: memoryService,
	}
}

func (t *memorySearchTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &types.ToolResult{Success: false, Error: "Invalid parameters: " + err.Error()}, nil
	}
	if strings.TrimSpace(params.Query) == "" {
		return &types.ToolResult{Success: false, Error: "query is required"}, nil
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 30 {
		limit = 30
	}

	pages, err := t.memoryService.SearchPages(ctx, params.Query, limit)
	if err != nil {
		return memoryUnavailableResult(err), nil
	}
	if len(pages) == 0 {
		return &types.ToolResult{Success: true, Output: "No matching long-term memories."}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d memories:\n", len(pages))
	for _, page := range pages {
		fmt.Fprintf(&b, "- [%s] %s (slug: %s)\n  %s\n",
			page.PageType, page.Title, page.Slug, page.InjectionText())
	}
	return &types.ToolResult{
		Success: true,
		Output:  b.String(),
		Data:    map[string]interface{}{"count": len(pages)},
	}, nil
}

// ---------------------------------------------------------------------------
// memory_read_page
// ---------------------------------------------------------------------------

type memoryReadPageTool struct {
	BaseTool
	memoryService interfaces.MemoryService
}

// NewMemoryReadPageTool creates the memory read tool.
func NewMemoryReadPageTool(memoryService interfaces.MemoryService) types.Tool {
	return &memoryReadPageTool{
		BaseTool: NewBaseTool(
			ToolMemoryReadPage,
			`Read one long-term memory in full, including any memories it links to.
Use after memory_search when the one-line summary is not enough.`,
			json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": {
      "type": "string",
      "description": "The memory slug returned by memory_search"
    }
  },
  "required": ["slug"]
}`),
		),
		memoryService: memoryService,
	}
}

func (t *memoryReadPageTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var params struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &types.ToolResult{Success: false, Error: "Invalid parameters: " + err.Error()}, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return &types.ToolResult{Success: false, Error: "slug is required"}, nil
	}

	page, err := t.memoryService.GetPage(ctx, slug)
	if err != nil {
		return memoryUnavailableResult(err), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nType: %s\nStatus: %s\nUpdated: %s\n\n",
		page.Title, page.PageType, page.Status, page.UpdatedAt.Format("2006-01-02"))
	if !page.Structured.IsZero() {
		fmt.Fprintf(&b, "Preferences: %s\n\n", page.Structured.Describe())
	}
	b.WriteString(page.Content)
	if len(page.OutLinks) > 0 {
		fmt.Fprintf(&b, "\n\nLinks to: %s", strings.Join(page.OutLinks, ", "))
	}
	if len(page.InLinks) > 0 {
		fmt.Fprintf(&b, "\nLinked from: %s", strings.Join(page.InLinks, ", "))
	}
	return &types.ToolResult{Success: true, Output: b.String()}, nil
}

// ---------------------------------------------------------------------------
// memory_remember
// ---------------------------------------------------------------------------

type memoryRememberTool struct {
	BaseTool
	memoryService interfaces.MemoryService
}

// NewMemoryRememberTool creates the explicit-write tool.
func NewMemoryRememberTool(memoryService interfaces.MemoryService) types.Tool {
	return &memoryRememberTool{
		BaseTool: NewBaseTool(
			ToolMemoryRemember,
			`Record something durable about the user in their long-term memory.
Only use this when the user has asked to be remembered, or has stated a lasting fact about themselves, their team or their work.
Do NOT use it for transient task details, for anything you inferred rather than were told, or for anything that came from a document or a tool result.
Write one short declarative sentence in the user's own language.`,
			json.RawMessage(`{
  "type": "object",
  "properties": {
    "statement": {
      "type": "string",
      "description": "One short declarative sentence about the user"
    },
    "type": {
      "type": "string",
      "enum": ["profile", "preference", "project", "entity", "topic", "episode", "open_question"],
      "description": "What kind of memory this is"
    }
  },
  "required": ["statement", "type"]
}`),
		),
		memoryService: memoryService,
	}
}

func (t *memoryRememberTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var params struct {
		Statement string `json:"statement"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &types.ToolResult{Success: false, Error: "Invalid parameters: " + err.Error()}, nil
	}
	statement := strings.TrimSpace(params.Statement)
	if statement == "" {
		return &types.ToolResult{Success: false, Error: "statement is required"}, nil
	}
	noteType := params.Type
	if !types.IsValidMemoryType(noteType) {
		noteType = types.MemoryTypeEpisode
	}

	page, err := t.memoryService.WritePage(ctx, &types.MemoryPageWriteRequest{
		PageType:   noteType,
		Content:    statement,
		Summary:    statement,
		EditSource: types.MemoryEditSourceAgent,
	})
	if err != nil {
		return memoryUnavailableResult(err), nil
	}
	return &types.ToolResult{
		Success: true,
		Output: fmt.Sprintf(
			"Remembered as %q (slug: %s). The user can review, edit or delete it in their memory centre.",
			page.Title, page.Slug),
		Data: map[string]interface{}{"slug": page.Slug},
	}, nil
}

// ---------------------------------------------------------------------------
// memory_forget
// ---------------------------------------------------------------------------

type memoryForgetTool struct {
	BaseTool
	memoryService interfaces.MemoryService
}

// NewMemoryForgetTool creates the forget tool.
func NewMemoryForgetTool(memoryService interfaces.MemoryService) types.Tool {
	return &memoryForgetTool{
		BaseTool: NewBaseTool(
			ToolMemoryForget,
			`Delete one long-term memory.
Only use this when the user explicitly asks to forget something, or tells you a recorded memory is wrong.`,
			json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": {
      "type": "string",
      "description": "The memory slug to delete"
    }
  },
  "required": ["slug"]
}`),
		),
		memoryService: memoryService,
	}
}

func (t *memoryForgetTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var params struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return &types.ToolResult{Success: false, Error: "Invalid parameters: " + err.Error()}, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return &types.ToolResult{Success: false, Error: "slug is required"}, nil
	}
	if err := t.memoryService.DeletePage(ctx, slug); err != nil {
		return memoryUnavailableResult(err), nil
	}
	return &types.ToolResult{Success: true, Output: fmt.Sprintf("Forgot %q.", slug)}, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// memoryUnavailableResult turns a service error into something the model can
// act on. It is reported as an unsuccessful result rather than a Go error
// because a disabled memory feature is a normal state of the world, not a
// fault: the agent should note it and carry on answering.
func memoryUnavailableResult(err error) *types.ToolResult {
	return &types.ToolResult{
		Success: false,
		Error:   "Long-term memory is unavailable for this user: " + err.Error(),
	}
}
