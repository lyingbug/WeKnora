package memory

import (
	"strings"
	"unicode"
)

// rememberMarkers are the phrases that turn a message into a direct request to
// remember something. They are deliberately imperative: a bare statement like
// "I am a backend engineer" is durable context for the automatic extractor to
// judge, but only "remember that ..." is an instruction to store it, and only
// the latter is honoured while the write mode is explicit-only.
var rememberMarkers = []string{
	"请记住", "帮我记住", "帮我记一下", "帮我记下", "麻烦记住",
	"记住", "记一下", "记下来", "记录一下", "记note",
	"别忘了", "别忘记", "不要忘了", "不要忘记", "以后记得",
	"please remember", "remember that", "remember this", "remember:", "remember ",
	"note that", "keep in mind that", "keep in mind", "don't forget that",
	"don't forget", "do not forget",
}

// statementTrimCutset removes the punctuation and particles that sit between the
// imperative and the fact itself, as in "记住：我用 Go" or "remember - I use Go".
const statementTrimCutset = " \t\n：:，,。.、;；-—~!！\"'“”‘’()（）"

// questionEndings mark a message that asks about remembering rather than asking
// for something to be remembered, e.g. "你能记住我说的话吗".
var questionEndings = []string{"吗", "呢", "?", "？", "吧"}

// DetectRememberRequest reports whether the user directly asked for something to
// be remembered, and returns the statement to store.
//
// The statement may sit on either side of the imperative — both "记住，我用 Go"
// and "我用 Go，记住" are natural — so the text after the marker is preferred and
// the text before it is the fallback.
func DetectRememberRequest(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)

	idx, markerLen := -1, 0
	for _, marker := range rememberMarkers {
		at := strings.Index(lower, marker)
		if at < 0 {
			continue
		}
		// The earliest marker wins, and among markers starting at the same
		// offset the longest one does, so "帮我记住" is not read as "记住".
		if idx < 0 || at < idx || (at == idx && len(marker) > markerLen) {
			idx, markerLen = at, len(marker)
		}
	}
	if idx < 0 {
		return "", false
	}

	after := strings.Trim(trimmed[idx+markerLen:], statementTrimCutset)
	before := strings.Trim(trimmed[:idx], statementTrimCutset)

	statement := after
	if !isStorableStatement(statement) {
		statement = before
	}
	if !isStorableStatement(statement) {
		return "", false
	}
	// "你能记住我说的话吗" reaches here with a plausible-looking statement, so
	// interrogatives are rejected last rather than by the marker scan.
	for _, ending := range questionEndings {
		if strings.HasSuffix(statement, ending) {
			return "", false
		}
	}
	if LooksLikeInstruction(statement) {
		return "", false
	}
	return statement, true
}

// isStorableStatement rejects fragments too small to mean anything on their own.
func isStorableStatement(statement string) bool {
	count := 0
	for _, r := range statement {
		if !unicode.IsSpace(r) {
			count++
		}
		if count >= 3 {
			return true
		}
	}
	return false
}
