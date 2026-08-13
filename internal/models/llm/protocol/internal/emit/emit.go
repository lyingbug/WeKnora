// Package emit holds the stream-emission bookkeeping every protocol driver
// shares: the reasoning-to-answer hand-off, the inline <think> convention, and
// error termination.
//
// Centralizing it is what keeps three protocols behaving identically from the
// UI's point of view, even though each learns about reasoning differently —
// Chat Completions through a delta field, Responses through typed events, and
// Anthropic through content blocks.
package emit

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// Thinking owns the reasoning-to-answer hand-off: reasoning chunks flow as
// they arrive, and exactly one done marker is emitted before the first answer
// token, or when the stream ends without one.
//
// Consumers rely on that single marker to close the thinking panel, so
// emitting it twice or never both misrender.
type Thinking struct {
	active bool
}

// Emit forwards a reasoning chunk and records that a done marker is owed.
func (t *Thinking) Emit(out chan<- types.StreamResponse, content string) {
	if content == "" {
		return
	}
	t.active = true
	out <- types.StreamResponse{
		ResponseType: types.ResponseTypeThinking,
		Content:      content,
	}
}

// Finish emits the done marker if one is owed. It is safe to call repeatedly.
func (t *Thinking) Finish(out chan<- types.StreamResponse) {
	if !t.active {
		return
	}
	t.active = false
	out <- types.StreamResponse{
		ResponseType: types.ResponseTypeThinking,
		Done:         true,
	}
}

// Active reports whether a done marker is still owed.
func (t *Thinking) Active() bool { return t.active }

// Error terminates a stream with an error chunk.
func Error(out chan<- types.StreamResponse, message string) {
	out <- types.StreamResponse{
		ResponseType: types.ResponseTypeError,
		Content:      message,
		Done:         true,
	}
}

// InlineSplitter separates reasoning that a model inlines in <think> tags from
// the answer that follows, across chunk boundaries.
//
// Some open-weight models have no reasoning field and emit the tags in the
// content instead. Handling that in the splitter rather than at the end lets
// the reasoning stream live, and the state machine is needed because a tag can
// straddle two chunks: "<thi" and "nk>" arrive separately often enough that a
// per-chunk check silently leaks tags into the answer.
type InlineSplitter struct {
	inThinking bool
	started    bool
	pending    string
}

// Feed consumes one content chunk and reports the answer text and the inline
// reasoning text it contained.
func (s *InlineSplitter) Feed(chunk string) (answer, thinking string) {
	s.pending += chunk

	var answerParts, thinkingParts []string
	for s.pending != "" {
		if s.inThinking {
			idx := strings.Index(s.pending, thinkClose)
			if idx < 0 {
				// Hold back a possible partial closing tag; release the rest.
				safe, held := splitPartial(s.pending, thinkClose)
				thinkingParts = append(thinkingParts, safe)
				s.pending = held
				break
			}
			thinkingParts = append(thinkingParts, s.pending[:idx])
			s.pending = s.pending[idx+len(thinkClose):]
			s.inThinking = false
			continue
		}

		// The convention only applies when the tag opens the message; a
		// mid-answer "<think>" is ordinary text a model may legitimately write.
		if !s.started {
			trimmed := strings.TrimLeft(s.pending, " \t\r\n")
			if strings.HasPrefix(trimmed, thinkOpen) {
				s.started = true
				s.inThinking = true
				s.pending = trimmed[len(thinkOpen):]
				continue
			}
			if isPrefixOf(trimmed, thinkOpen) {
				// Could still become an opening tag once more arrives.
				break
			}
			s.started = true
		}

		answerParts = append(answerParts, s.pending)
		s.pending = ""
	}

	return strings.Join(answerParts, ""), strings.Join(thinkingParts, "")
}

// splitPartial returns the portion of s that cannot be the start of tag, plus
// the trailing portion that might be.
func splitPartial(s, tag string) (safe, held string) {
	maxHold := len(tag) - 1
	if maxHold > len(s) {
		maxHold = len(s)
	}
	for hold := maxHold; hold > 0; hold-- {
		if isPrefixOf(s[len(s)-hold:], tag) {
			return s[:len(s)-hold], s[len(s)-hold:]
		}
	}
	return s, ""
}

// isPrefixOf reports whether s could be the beginning of tag.
func isPrefixOf(s, tag string) bool {
	if s == "" || len(s) >= len(tag) {
		return false
	}
	return strings.HasPrefix(tag, s)
}

// SplitInlineThinking separates inlined reasoning from the answer in a
// complete, non-streamed message.
func SplitInlineThinking(content string) (answer, thinking string) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, thinkOpen) {
		return content, ""
	}
	// The last closing tag wins, so a model that nests or repeats the tag does
	// not leak its reasoning into the answer.
	end := strings.LastIndex(trimmed, thinkClose)
	if end < 0 {
		// Reasoning was truncated before it closed; there is no answer yet.
		return "", strings.TrimSpace(trimmed[len(thinkOpen):])
	}
	thinking = strings.TrimSpace(trimmed[len(thinkOpen):end])
	answer = strings.TrimSpace(trimmed[end+len(thinkClose):])
	return answer, thinking
}
