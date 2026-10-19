package richtext

import (
	"regexp"
	"strings"
)

// filterStyle reproduces sanitize-html's style handling: postcss parses
// `<tag> {<value>}`, declarations outside allowedStyles are dropped, and the
// rest are re-serialized as `prop:value` joined with ';'. ok is false where
// postcss throws (sanitize-html then drops the attribute). It covers the
// postcss grammar real inline styles use: declarations, comments, strings,
// brackets, !important, nested blocks and at-rules.
func filterStyle(_ string, value string) (string, bool) {
	segments, ok := splitDeclarations(value)
	if !ok {
		return "", false
	}
	var kept []string
	for _, segment := range segments {
		prop, val, important, isDecl, ok := parseDeclaration(segment)
		if !ok {
			return "", false
		}
		if !isDecl || prop != "text-align" || !textAlign.MatchString(val) {
			continue
		}
		decl := prop + ":" + val
		if important {
			decl += " !important"
		}
		kept = append(kept, decl)
	}
	return strings.Join(kept, ";"), true
}

var textAlign = regexp.MustCompile(`^(?:left|center|right|justify)$`)

func isCSSSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// splitDeclarations splits the rule body at top-level ';', skipping nested
// `{...}` blocks (rules whose declarations sanitize-html never keeps). It
// fails on unclosed strings, comments or brackets and on a stray '}'.
func splitDeclarations(body string) ([]string, bool) {
	var segments []string
	start, depth, braces := 0, 0, 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			for ; j < len(body) && body[j] != c; j++ {
				if body[j] == '\\' {
					j++
				}
			}
			if j >= len(body) {
				return nil, false
			}
			i = j
		case c == '/' && i+1 < len(body) && body[i+1] == '*':
			end := strings.Index(body[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			i += end + 3
		case c == '\\':
			i++
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		case depth > 0:
		case c == '{':
			braces++
		case c == '}':
			if braces == 0 {
				return nil, false
			}
			braces--
			if braces == 0 {
				start = i + 1 // the nested rule (with its selector) is not a declaration
			}
		case c == ';' && braces == 0:
			segments = append(segments, body[start:i])
			start = i + 1
		}
	}
	if depth > 0 || braces > 0 {
		return nil, false
	}
	return append(segments, body[start:]), true
}

// stripComments removes comments next to whitespace or the edges, keeping
// those between two non-space tokens as postcss does.
func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		end := strings.Index(s[i+2:], "*/")
		if end < 0 {
			return s
		}
		j := i + 2 + end + 2
		before := i == 0 || isCSSSpace(s[i-1])
		after := j >= len(s) || isCSSSpace(s[j])
		if before || after {
			s = s[:i] + s[j:]
		} else {
			// Kept verbatim; mark it so it is not found again.
			return s[:j] + stripComments(s[j:])
		}
	}
}

var importantSuffix = regexp.MustCompile(`(?i)\s*!\s*important\s*$`)

// parseDeclaration reads one `prop: value` segment. isDecl is false for
// empty segments and at-rules; ok is false where postcss throws.
func parseDeclaration(segment string) (prop, value string, important, isDecl, ok bool) {
	text := strings.TrimFunc(stripComments(segment), func(r rune) bool { return r < 0x80 && isCSSSpace(byte(r)) })
	if text == "" || text[0] == '@' {
		return "", "", false, false, true
	}
	colon := topLevelIndex(text, ':')
	if colon < 0 {
		return "", "", false, false, false // Unknown word
	}
	prop = strings.TrimRight(text[:colon], " \t\n\r\f")
	if strings.ContainsAny(prop, " \t\n\r\f") {
		return "", "", false, false, false // Unknown word between property parts
	}
	if strings.HasPrefix(prop, "_") || strings.HasPrefix(prop, "*") {
		prop = prop[1:]
	}
	value = strings.TrimLeft(text[colon+1:], " \t\n\r\f")
	if loc := importantSuffix.FindStringIndex(value); loc != nil {
		important = true
		value = value[:loc[0]]
	}
	value = strings.TrimRight(value, " \t\n\r\f")
	if topLevelIndex(value, ':') >= 0 {
		return "", "", false, false, false // Missed semicolon
	}
	return prop, value, important, true, true
}

func topLevelIndex(s string, target byte) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			for i++; i < len(s) && s[i] != c; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case c == '\\':
			i++
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case c == target && depth == 0:
			return i
		}
	}
	return -1
}
