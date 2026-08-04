package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestDatabaseRegexToolsRejectNonPortableSyntax(t *testing.T) {
	tests := []struct {
		name    string
		execute func() (*types.ToolResult, error)
	}{
		{
			name: "grep_chunks",
			execute: func() (*types.ToolResult, error) {
				return NewGrepChunksTool(nil, nil).Execute(
					context.Background(),
					json.RawMessage(`{"query":"\\brag\\b"}`),
				)
			},
		},
		{
			name: "wiki_search",
			execute: func() (*types.ToolResult, error) {
				return NewWikiSearchTool(nil, nil, nil, nil).Execute(
					context.Background(),
					json.RawMessage(`{"queries":["\\brag\\b"]}`),
				)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := testCase.execute()
			if err == nil {
				t.Fatal("expected non-portable regex to return an error")
			}
			if result == nil || result.Success {
				t.Fatalf("expected failed tool result, got %#v", result)
			}
			if !strings.Contains(result.Error, "portable") {
				t.Fatalf("expected actionable portable-regex error, got %q", result.Error)
			}
		})
	}
}

// The character-class shorthands mean the same thing to PostgreSQL ARE, MySQL
// ICU and Go RE2, so rejecting them only cost the agent expressiveness without
// buying any cross-database consistency.
func TestPortableRegexAcceptsAgreedCharacterClasses(t *testing.T) {
	for _, pattern := range []string{
		`\d+`,
		`^chapter\s+\d+`,
		`\w+`,
		`[A-Z]\S*`,
		`error\D`,
		`a\tb`,
		`C\+\+`,
	} {
		if _, err := compilePortableCaseInsensitiveRegex(pattern); err != nil {
			t.Errorf("pattern %q should be portable, got error: %v", pattern, err)
		}
	}
}

// \b is the motivating case for keeping a portable subset at all: MySQL and
// RE2 read it as a word boundary while PostgreSQL ARE reads it as a literal
// backspace, so the same pattern matches different text per deployment.
func TestPortableRegexRejectsEscapesThatDivergeAcrossEngines(t *testing.T) {
	for _, pattern := range []string{
		`\brag\b`,
		`\Bfoo`,
		`\mword`,
		`\yword`,
		`\Astart`,
		`\Zend`,
		`(foo)\1`,
		`(?i)foo`,
		`(?=foo)`,
		`trailing\`,
	} {
		if _, err := compilePortableCaseInsensitiveRegex(pattern); err == nil {
			t.Errorf("pattern %q should be rejected as non-portable", pattern)
		}
	}
}
