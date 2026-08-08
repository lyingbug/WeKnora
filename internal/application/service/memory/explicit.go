package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
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
	"keep in mind that", "keep in mind", "don't forget that",
	"don't forget", "do not forget",
}

// ClassifyExplicitMemory turns the common forms of "remember ..." into a
// useful saved-memory slot without paying for another model call. It is
// intentionally conservative: uncertain statements remain episodes, but are
// still durable because all explicit memories are saved.
func ClassifyExplicitMemory(statement string) (string, string, types.MemoryPreference) {
	lower := strings.ToLower(strings.TrimSpace(statement))
	pref := types.MemoryPreference{}

	if containsAny(lower, "简洁", "简短", "精简", "concise", "brief answers", "short answers") {
		pref.Verbosity = types.MemoryVerbosityConcise
	} else if containsAny(lower, "详细", "详尽", "detailed", "in depth", "thorough") {
		pref.Verbosity = types.MemoryVerbosityDetailed
	}
	if containsAny(lower, "中文回答", "用中文", "简体中文", "chinese answers", "answer in chinese") {
		pref.Language = "zh"
	} else if containsAny(lower, "英文回答", "用英文", "english answers", "answer in english") {
		pref.Language = "en"
	}
	if containsAny(lower, "专业语气", "正式语气", "professional tone") {
		pref.Tone = types.MemoryToneProfessional
	} else if containsAny(lower, "友好语气", "亲切", "friendly tone") {
		pref.Tone = types.MemoryToneFriendly
	}
	if containsAny(lower, "项目符号", "列表回答", "bullet points", "bulleted") {
		pref.Format = types.MemoryFormatBullets
	} else if containsAny(lower, "markdown") {
		pref.Format = types.MemoryFormatMarkdown
	}

	if !pref.IsZero() || containsAny(lower, "我偏好", "我喜欢", "我希望回答", "i prefer", "my preference") {
		key := "preference/general"
		switch {
		case pref.Language != "":
			key = "preference/language"
		case pref.Verbosity != "":
			key = "preference/verbosity"
		case pref.Tone != "":
			key = "preference/tone"
		case pref.Format != "":
			key = "preference/format"
		}
		return types.MemoryTypePreference, key, pref.Sanitize()
	}

	if containsAny(lower, "我是", "我的名字", "我的职业", "我的角色", "i am ", "i'm ", "my name", "my role") {
		return types.MemoryTypeProfile, "profile/identity", pref
	}
	if containsAny(lower, "我在做", "我正在做", "我的项目", "我负责", "i work on", "my project", "i'm building") {
		return types.MemoryTypeProject, "project/current", pref
	}
	return types.MemoryTypeEpisode, "episode/" + normalizeSlugSegment(DeriveMemoryTitle(statement)), pref
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
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
		// Every occurrence has to be considered, not just the first: the marker
		// may appear mid-sentence as an ordinary verb before appearing as an
		// imperative later.
		for offset := 0; ; {
			at := strings.Index(lower[offset:], marker)
			if at < 0 {
				break
			}
			at += offset
			offset = at + 1
			if !startsClause(lower, at) {
				continue
			}
			// The earliest marker wins, and among markers at the same offset the
			// longest one does, so "帮我记住" is not read as "记住".
			if idx < 0 || at < idx || (at == idx && len(marker) > markerLen) {
				idx, markerLen = at, len(marker)
			}
			break
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

// startsClause reports whether the marker at idx sits where an imperative can:
// at the beginning of the message, after a clause boundary, or after a word of
// politeness leading the request.
//
// Position is what separates a request from a mention. "记住我用 Go" is an
// instruction; "我不记得上次是谁改的" and "I can never remember which flag" merely
// contain the verb, and matching anywhere stored the tail of those sentences as
// though it were a fact about the user.
func startsClause(lower string, idx int) bool {
	prefix := strings.TrimRight(lower[:idx], " \t")
	if prefix == "" {
		return true
	}
	// Decoded as a rune, because the Chinese clause marks are multi-byte and
	// stepping back one byte lands inside one.
	last, size := utf8.DecodeLastRuneInString(prefix)
	if strings.ContainsRune(clauseBoundaries, last) {
		return true
	}
	_ = size
	for _, word := range politenessPrefixes {
		if strings.HasSuffix(prefix, word) {
			return startsClause(lower, len(prefix)-len(word))
		}
	}
	return false
}

// clauseBoundaries are the characters a clause can end on.
const clauseBoundaries = ",.;:!?\n，。；：！？、"

// politenessPrefixes may lead an imperative without changing that it is one.
// Compared against a right-trimmed prefix, so no trailing spaces here.
var politenessPrefixes = []string{"please", "kindly", "请", "麻烦", "帮我"}

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
