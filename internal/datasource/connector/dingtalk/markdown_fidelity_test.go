package dingtalk

import (
	"encoding/json"
	"strings"
	"testing"
)

// officialDocumentBlocks mirrors the payload shapes in DingTalk's "块元素数据结构"
// reference: an integer heading level, plain text inline runs that carry no
// elementType, link/image runs that do, and one list item per block.
func officialDocumentBlocks() []json.RawMessage {
	return []json.RawMessage{
		json.RawMessage(`{"blockType":"heading","heading":{"level":1,"text":"一级标题"}}`),
		json.RawMessage(`{"blockType":"paragraph","paragraph":{"text":"段落文字内容"}}`),
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"l1","level":0}},` +
			`"children":[{"text":"有序项一"}]}`),
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"l1","level":0}},` +
			`"children":[{"text":"有序项二"}]}`),
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"l1","level":0}},` +
			`"children":[{"text":"有序项三"}]}`),
		json.RawMessage(`{"blockType":"table","table":{"rolSize":2,"colSize":2,` +
			`"cells":[["列A","列B"],["值1","值2"]]}}`),
	}
}

func TestRenderDocumentNumbersOrderedRunsSequentially(t *testing.T) {
	result := renderDocument("", officialDocumentBlocks())
	for _, want := range []string{"1. 有序项一", "2. 有序项二", "3. 有序项三"} {
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("Markdown missing %q:\n%s", want, result.Markdown)
		}
	}
}

func TestRenderDocumentRestartsNumberingPerListGroup(t *testing.T) {
	blocks := []json.RawMessage{
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"a","level":0}},` +
			`"children":[{"text":"first"}]}`),
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"a","level":0}},` +
			`"children":[{"text":"second"}]}`),
		json.RawMessage(`{"blockType":"paragraph","paragraph":{"text":"between"}}`),
		json.RawMessage(`{"blockType":"orderedList","orderedList":{"list":{"listId":"b","level":0}},` +
			`"children":[{"text":"restarted"}]}`),
	}
	result := renderDocument("", blocks)
	for _, want := range []string{"1. first", "2. second", "1. restarted"} {
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("Markdown missing %q:\n%s", want, result.Markdown)
		}
	}
}

func TestRenderDocumentSeparatesListRunFromNextBlock(t *testing.T) {
	result := renderDocument("", officialDocumentBlocks())
	// Without a blank line the table header is parsed as a lazy continuation of
	// the preceding list item rather than as a table.
	if !strings.Contains(result.Markdown, "有序项三\n\n| 列A | 列B |") {
		t.Fatalf("list run not terminated before the table:\n%s", result.Markdown)
	}
}

func TestRenderDocumentAcceptsNonIntegerHeadingLevels(t *testing.T) {
	cases := map[string]string{
		`{"blockType":"heading","heading":{"level":"2","text":"标题"}}`:         "## 标题",
		`{"blockType":"heading","heading":{"level":"heading-3","text":"标题"}}`: "### 标题",
		`{"blockType":"heading","heading":{"level":"h4","text":"标题"}}`:        "#### 标题",
		`{"blockType":"heading","heading":{"level":"","text":"标题"}}`:          "# 标题",
	}
	for payload, want := range cases {
		result := renderDocument("", []json.RawMessage{json.RawMessage(payload)})
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("payload %s rendered %q, want %q", payload, result.Markdown, want)
		}
		if len(result.UnknownTypes) != 0 {
			t.Errorf("payload %s reported unknown types %v", payload, result.UnknownTypes)
		}
	}
}

func TestRenderDocumentSalvagesTextFromUndecodableBlocks(t *testing.T) {
	// A type-specific payload that does not match the documented schema must
	// cost the block's formatting, not its text.
	result := renderDocument("", []json.RawMessage{
		json.RawMessage(`{"blockType":"paragraph","paragraph":"unexpected string",` +
			`"children":[{"text":"salvaged text"}]}`),
	})
	if !strings.Contains(result.Markdown, "salvaged text") {
		t.Fatalf("text lost for undecodable block: %q", result.Markdown)
	}
	if strings.Join(result.UnknownTypes, ",") != "partial_decode" {
		t.Fatalf("unknown types = %v, want [partial_decode]", result.UnknownTypes)
	}
}
