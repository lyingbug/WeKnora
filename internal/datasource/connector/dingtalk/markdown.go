package dingtalk

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type renderResult struct {
	Markdown     string
	UnknownTypes []string
}

type blockEnvelope struct {
	BlockType     string            `json:"blockType"`
	Paragraph     textBlock         `json:"paragraph"`
	Heading       headingBlock      `json:"heading"`
	Blockquote    textBlock         `json:"blockquote"`
	Callout       calloutBlock      `json:"callout"`
	Columns       columnsBlock      `json:"columns"`
	OrderedList   listBlock         `json:"orderedList"`
	UnorderedList listBlock         `json:"unorderedList"`
	Table         tableBlock        `json:"table"`
	Children      []json.RawMessage `json:"children"`
}

type textBlock struct {
	Text string `json:"text"`
}

type headingBlock struct {
	Level headingLevel `json:"level"`
	Text  string       `json:"text"`
}

// headingLevel decodes the documented integer form as well as the string forms
// observed in the wild ("2", "heading-2", "h2"). A strict int would fail the
// whole envelope and discard the heading text along with it.
type headingLevel int

var headingLevelDigits = regexp.MustCompile(`[1-6]`)

func (l *headingLevel) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*l = headingLevel(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		*l = 0
		return nil
	}
	if digit := headingLevelDigits.FindString(text); digit != "" {
		level, _ := strconv.Atoi(digit)
		*l = headingLevel(level)
		return nil
	}
	*l = 0
	return nil
}

type calloutBlock struct {
	Sticker string `json:"sticker"`
}

type columnsBlock struct {
	Size int `json:"size"`
}

type listBlock struct {
	List struct {
		ListID string `json:"listId"`
		Level  int    `json:"level"`
	} `json:"list"`
}

type tableBlock struct {
	Rows  int        `json:"rolSize"`
	Cols  int        `json:"colSize"`
	Cells [][]string `json:"cells"`
}

type inlineEnvelope struct {
	ElementType string            `json:"elementType"`
	Text        string            `json:"text"`
	Bold        bool              `json:"bold"`
	Italic      bool              `json:"italic"`
	Strike      bool              `json:"stike"`
	Fonts       string            `json:"fonts"`
	Properties  inlineProperties  `json:"properties"`
	Children    []json.RawMessage `json:"children"`
}

type inlineProperties struct {
	Code string `json:"code"`
	Src  string `json:"src"`
	Href string `json:"href"`
}

// renderState carries the output buffer plus the list run currently being
// written. DingTalk emits every list item as its own top-level block, so
// consecutive items are only a list because they are adjacent: the state is
// what lets us number an ordered run and close it with a blank line before the
// next block, instead of letting the following table or paragraph be absorbed
// into the list.
type renderState struct {
	builder *strings.Builder
	unknown map[string]struct{}
	list    listRun
}

type listRun struct {
	active bool
	// group is the listId/level pair of the ordered run in progress; ordered
	// numbering restarts whenever it changes, and an unordered item clears it.
	group string
	seq   int
}

// closeList terminates a list run so the next block starts its own paragraph.
func (s *renderState) closeList() {
	if !s.list.active {
		return
	}
	s.builder.WriteByte('\n')
	s.list = listRun{}
}

func renderDocument(title string, blocks []json.RawMessage) renderResult {
	var builder strings.Builder
	title = strings.TrimSpace(title)
	if title != "" {
		builder.WriteString("# ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
	}

	unknown := make(map[string]struct{})
	state := &renderState{builder: &builder, unknown: unknown}
	for _, raw := range blocks {
		renderBlock(state, raw, 0)
	}
	state.closeList()
	content := strings.TrimSpace(builder.String())
	if content != "" {
		content += "\n"
	}
	unknownTypes := make([]string, 0, len(unknown))
	for blockType := range unknown {
		unknownTypes = append(unknownTypes, blockType)
	}
	sort.Strings(unknownTypes)
	return renderResult{Markdown: content, UnknownTypes: unknownTypes}
}

func renderBlock(state *renderState, raw json.RawMessage, depth int) {
	builder := state.builder
	unknown := state.unknown
	if depth > maxResourceDepth {
		state.closeList()
		unknown["max_depth"] = struct{}{}
		return
	}
	var block blockEnvelope
	if err := json.Unmarshal(raw, &block); err != nil {
		state.closeList()
		renderUndecodableBlock(state, raw, depth)
		return
	}
	blockType := strings.ToLower(strings.TrimSpace(block.BlockType))
	if blockType != "orderedlist" && blockType != "unorderedlist" {
		state.closeList()
	}
	switch blockType {
	case "paragraph":
		text := renderInlineChildren(block.Children, depth+1, unknown)
		if text == "" {
			text = block.Paragraph.Text
		}
		writeParagraph(builder, text)
	case "heading":
		text := renderInlineChildren(block.Children, depth+1, unknown)
		if text == "" {
			text = block.Heading.Text
		}
		if text != "" {
			level := int(block.Heading.Level)
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			builder.WriteString(strings.Repeat("#", level))
			builder.WriteByte(' ')
			builder.WriteString(text)
			builder.WriteString("\n\n")
		}
	case "blockquote":
		text := renderInlineChildren(block.Children, depth+1, unknown)
		if text == "" {
			text = block.Blockquote.Text
		}
		if text != "" {
			for _, line := range strings.Split(text, "\n") {
				builder.WriteString("> ")
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
			builder.WriteByte('\n')
		}
	case "orderedlist", "unorderedlist":
		text := renderInlineChildren(block.Children, depth+1, unknown)
		if text == "" {
			return
		}
		level := 0
		marker := "- "
		if blockType == "orderedlist" {
			level = block.OrderedList.List.Level
			marker = state.orderedMarker(block.OrderedList.List.ListID, level)
		} else {
			level = block.UnorderedList.List.Level
			state.list.group = ""
		}
		if level < 0 {
			level = 0
		}
		if level > maxResourceDepth {
			level = maxResourceDepth
		}
		state.list.active = true
		builder.WriteString(strings.Repeat("  ", level))
		builder.WriteString(marker)
		builder.WriteString(text)
		builder.WriteByte('\n')
	case "callout", "columns":
		for _, child := range block.Children {
			renderBlock(state, child, depth+1)
		}
		state.closeList()
	case "table":
		renderTable(builder, block.Table.Cells)
	case "":
		unknown["missing_block_type"] = struct{}{}
	default:
		unknown[blockType] = struct{}{}
	}
}

// orderedMarker returns the next number of the ordered run identified by
// listId+level, restarting at 1 whenever the run changes. Emitting real numbers
// keeps the ordinal meaningful in the plain text that reaches retrieval, where
// a repeated "1." would otherwise flatten every item to the same rank.
func (s *renderState) orderedMarker(listID string, level int) string {
	group := listID + "\x00" + strconv.Itoa(level)
	if !s.list.active || s.list.group != group {
		s.list.group = group
		s.list.seq = 0
	}
	s.list.seq++
	return strconv.Itoa(s.list.seq) + ". "
}

// renderUndecodableBlock salvages a block whose type-specific payload does not
// match the documented schema. Only the envelope is re-read, so an unexpected
// field type costs the block's formatting instead of its text.
func renderUndecodableBlock(state *renderState, raw json.RawMessage, depth int) {
	var envelope struct {
		BlockType string            `json:"blockType"`
		Children  []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		state.unknown["invalid_json"] = struct{}{}
		return
	}
	state.unknown["partial_decode"] = struct{}{}
	text := renderInlineChildren(envelope.Children, depth+1, state.unknown)
	writeParagraph(state.builder, text)
}

func renderInlineChildren(
	children []json.RawMessage,
	depth int,
	unknown map[string]struct{},
) string {
	if depth > maxResourceDepth {
		unknown["inline_max_depth"] = struct{}{}
		return ""
	}
	var builder strings.Builder
	for _, raw := range children {
		var inline inlineEnvelope
		if err := json.Unmarshal(raw, &inline); err != nil {
			unknown["invalid_inline_json"] = struct{}{}
			continue
		}
		switch strings.ToLower(strings.TrimSpace(inline.ElementType)) {
		case "", "text":
			builder.WriteString(styleInlineText(inline.Text, inline))
		case "sticker":
			builder.WriteString(strings.TrimSpace(inline.Properties.Code))
		case "image":
			if src := strings.TrimSpace(inline.Properties.Src); src != "" {
				fmt.Fprintf(&builder, "![image](%s)", src)
			}
		case "link":
			label := renderInlineChildren(inline.Children, depth+1, unknown)
			href := strings.TrimSpace(inline.Properties.Href)
			switch {
			case href == "":
				builder.WriteString(label)
			case label == "":
				fmt.Fprintf(&builder, "[%s](%s)", escapeLabel(href), href)
			default:
				fmt.Fprintf(&builder, "[%s](%s)", escapeLabel(label), href)
			}
		default:
			unknown["inline_"+strings.ToLower(inline.ElementType)] = struct{}{}
			builder.WriteString(inline.Text)
		}
	}
	return builder.String()
}

func styleInlineText(text string, inline inlineEnvelope) string {
	if text == "" {
		return ""
	}
	if strings.EqualFold(inline.Fonts, "monospace") {
		text = "`" + strings.ReplaceAll(text, "`", "\\`") + "`"
	}
	if inline.Bold {
		text = "**" + text + "**"
	}
	if inline.Italic {
		text = "*" + text + "*"
	}
	if inline.Strike {
		text = "~~" + text + "~~"
	}
	return text
}

func writeParagraph(builder *strings.Builder, text string) {
	if text == "" {
		return
	}
	builder.WriteString(text)
	builder.WriteString("\n\n")
}

func renderTable(builder *strings.Builder, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return
	}
	writeTableRow(builder, normalizedRow(rows[0], columnCount))
	separator := make([]string, columnCount)
	for i := range separator {
		separator[i] = "---"
	}
	writeTableRow(builder, separator)
	for _, row := range rows[1:] {
		writeTableRow(builder, normalizedRow(row, columnCount))
	}
	builder.WriteByte('\n')
}

func normalizedRow(row []string, columns int) []string {
	out := make([]string, columns)
	for i := 0; i < len(row) && i < columns; i++ {
		out[i] = escapeTableCell(row[i])
	}
	return out
}

func writeTableRow(builder *strings.Builder, row []string) {
	builder.WriteString("| ")
	builder.WriteString(strings.Join(row, " | "))
	builder.WriteString(" |\n")
}

func escapeLabel(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "[", "\\[")
	return strings.ReplaceAll(text, "]", "\\]")
}

func escapeTableCell(text string) string {
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "\r\n", "<br>")
	return strings.ReplaceAll(text, "\n", "<br>")
}
