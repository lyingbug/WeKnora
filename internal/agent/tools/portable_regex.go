package tools

import (
	"fmt"
	"regexp"
	"unicode"
)

// compilePortableCaseInsensitiveRegex limits database-backed regex tools to
// syntax shared by PostgreSQL ARE, MySQL ICU, and Go RE2. Engine-specific
// shorthand and inline constructs otherwise make validation and DB execution
// disagree across supported deployments.
func compilePortableCaseInsensitiveRegex(pattern string) (*regexp.Regexp, error) {
	if err := validatePortableRegex(pattern); err != nil {
		return nil, err
	}

	compiled, err := regexp.Compile("(?i:" + pattern + ")")
	if err != nil {
		return nil, fmt.Errorf("invalid portable regular expression %q: %w", pattern, err)
	}
	return compiled, nil
}

// portableEscapes are the backslash escapes that PostgreSQL ARE, MySQL ICU,
// and Go RE2 all agree on, so a pattern using them matches the same text
// whichever backend runs it.
//
// Notably absent is \b: MySQL and RE2 read it as a word boundary, while
// PostgreSQL ARE reads it as a literal backspace, so the same pattern silently
// matches different text per deployment. \A, \Z, \z, \m, \M, \y and numeric
// backreferences diverge the same way and stay rejected.
var portableEscapes = map[rune]struct{}{
	'd': {}, 'D': {},
	's': {}, 'S': {},
	'w': {}, 'W': {},
	'n': {}, 'r': {}, 't': {}, 'f': {},
}

func validatePortableRegex(pattern string) error {
	runes := []rune(pattern)
	inCharacterClass := false

	for index := 0; index < len(runes); index++ {
		current := runes[index]
		if current == '\\' {
			if index+1 >= len(runes) {
				return fmt.Errorf("invalid portable regular expression %q: trailing backslash", pattern)
			}
			escaped := runes[index+1]
			_, portable := portableEscapes[escaped]
			if !portable && (unicode.IsLetter(escaped) || unicode.IsDigit(escaped)) {
				return fmt.Errorf(
					"non-portable regex escape \\%c in %q; portable escapes are "+
						"\\d \\D \\s \\S \\w \\W \\n \\r \\t \\f, plus literals, character "+
						"ranges, grouping, alternation, anchors, and quantifiers",
					escaped,
					pattern,
				)
			}
			index++
			continue
		}

		switch current {
		case '[':
			inCharacterClass = true
		case ']':
			inCharacterClass = false
		case '(':
			if !inCharacterClass && index+1 < len(runes) && runes[index+1] == '?' {
				return fmt.Errorf(
					"non-portable regex construct \"(?\" in %q; inline flags, lookarounds, "+
						"and named groups are not supported",
					pattern,
				)
			}
		}
	}

	return nil
}
