package ffdb

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// glob is a parsed shell-style wildcard pattern (issue #109). It matches the
// whole file name, ignoring case:
//
//   - any characters, including none
//     ?        exactly one character
//     [abc]    one character from the set; ranges such as [a-z] work, and
//     [!x] or [^x] negate the set
//     \x       x literally, for example \* or \[
//
// A "[" with no closing "]" is literal.
type glob struct {
	// re is the exact matcher. Each literal run of the pattern is a capture
	// group, which HighlightSpans uses.
	re *regexp.Regexp
	// like is a LIKE pattern, with no ESCAPE clause, that matches every name
	// re matches and possibly more: "?", each class, and a literal "%" or "_"
	// all become "_". Rows it selects are filtered with re.
	like string
	// longestLiteral is the longest run of characters in like that are not
	// LIKE wildcards. The FTS5 trigram index can answer a LIKE only when that
	// run has 3 or more characters.
	longestLiteral int
}

// globToken is one element of a pattern: a literal run, "*", "?" or a class.
type globToken struct {
	kind byte   // 'l' literal, '*', '?', '[' class
	text string // the literal text, or the RE2 class for '['
}

// parseGlob parses term as a glob. It returns ok=false when the term has no
// unescaped "*" or "?", which leaves it a literal search exactly as before:
// "Movie [1080p].mkv" and "a\*b" are searched literally.
func parseGlob(term string) (g *glob, ok bool) {
	tokens, wild := tokenizeGlob(term)
	if !wild {
		return nil, false
	}

	var re, like strings.Builder
	re.WriteString(`(?is)^`)
	for _, tok := range tokens {
		switch tok.kind {
		case 'l':
			re.WriteString("(" + regexp.QuoteMeta(tok.text) + ")")
			for _, r := range tok.text {
				if r == '%' || r == '_' {
					like.WriteByte('_')
				} else {
					like.WriteRune(r)
				}
			}
		case '*':
			re.WriteString(".*")
			if !strings.HasSuffix(like.String(), "%") {
				like.WriteByte('%')
			}
		case '?':
			re.WriteString(".")
			like.WriteByte('_')
		case '[':
			re.WriteString(tok.text)
			like.WriteByte('_')
		}
	}
	re.WriteString(`$`)

	compiled, err := regexp.Compile(re.String())
	if err != nil {
		// Every part is quoted or built from escaped runes, so this should
		// not happen; search literally rather than fail.
		return nil, false
	}
	g = &glob{re: compiled, like: like.String()}
	run := 0
	for _, r := range g.like {
		if r == '%' || r == '_' {
			run = 0
			continue
		}
		run++
		if run > g.longestLiteral {
			g.longestLiteral = run
		}
	}
	return g, true
}

// tokenizeGlob splits term into literal runs, wildcards and classes. wild
// reports whether it found an unescaped "*" or "?".
func tokenizeGlob(term string) (tokens []globToken, wild bool) {
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			tokens = append(tokens, globToken{kind: 'l', text: lit.String()})
			lit.Reset()
		}
	}

	runes := []rune(term)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '\\':
			if i+1 < len(runes) {
				i++
				lit.WriteRune(runes[i])
			} else {
				lit.WriteRune(r) // a trailing backslash is literal
			}
		case '*':
			flush()
			if len(tokens) == 0 || tokens[len(tokens)-1].kind != '*' {
				tokens = append(tokens, globToken{kind: '*'})
			}
			wild = true
		case '?':
			flush()
			tokens = append(tokens, globToken{kind: '?'})
			wild = true
		case '[':
			class, next, ok := parseClass(runes, i)
			if !ok {
				lit.WriteRune(r) // no closing "]": literal
				continue
			}
			flush()
			tokens = append(tokens, globToken{kind: '[', text: class})
			i = next
		default:
			lit.WriteRune(r)
		}
	}
	flush()
	return tokens, wild
}

// parseClass parses the bracket expression that starts at runes[start] ("[")
// and returns it as an RE2 class, with the index of its closing "]". A "]"
// right after "[" (or "[!") is a member, as in the shell.
func parseClass(runes []rune, start int) (class string, end int, ok bool) {
	i := start + 1
	negate := false
	if i < len(runes) && (runes[i] == '!' || runes[i] == '^') {
		negate = true
		i++
	}

	type span struct{ lo, hi rune }
	var spans []span
	first := true
	for ; i < len(runes); i++ {
		r := runes[i]
		if r == ']' && !first {
			break
		}
		first = false
		if r == '\\' && i+1 < len(runes) {
			i++
			r = runes[i]
		}
		lo, hi := r, r
		if i+2 < len(runes) && runes[i+1] == '-' && runes[i+2] != ']' {
			hi = runes[i+2]
			i += 2
			if hi == '\\' && i+1 < len(runes) {
				i++
				hi = runes[i]
			}
		}
		if lo <= hi { // a reversed range such as [z-a] matches nothing
			spans = append(spans, span{lo, hi})
		}
	}
	if i >= len(runes) {
		return "", 0, false
	}

	if len(spans) == 0 {
		// Only reversed ranges: the class matches no character, or, when
		// negated, any character.
		if negate {
			return `[\x{0}-\x{10FFFF}]`, i, true
		}
		return `[^\x{0}-\x{10FFFF}]`, i, true
	}
	var b strings.Builder
	b.WriteByte('[')
	if negate {
		b.WriteByte('^')
	}
	for _, s := range spans {
		b.WriteString(classRune(s.lo))
		if s.hi != s.lo {
			b.WriteByte('-')
			b.WriteString(classRune(s.hi))
		}
	}
	b.WriteByte(']')
	return b.String(), i, true
}

// classRune writes r for use inside an RE2 character class.
func classRune(r rune) string {
	switch r {
	case '\\', ']', '[', '^', '-':
		return `\` + string(r)
	}
	if r < 0x20 || r == utf8.RuneError {
		return `\x{` + strconv.FormatInt(int64(r), 16) + `}`
	}
	return string(r)
}

// HighlightSpans returns the byte ranges of name to highlight for a search
// for term. For a glob, those are the parts of name that the pattern's
// literal runs matched; "*", "?" and classes are not highlighted. For a glob
// the result is never nil, though it is empty when name does not match or the
// pattern has no literal parts. It is nil only for a literal term, and callers
// then fall back to shared.SplitFileName.
func HighlightSpans(name, term string) [][2]int {
	g, ok := parseGlob(SearchName(term))
	if !ok {
		return nil
	}
	spans := [][2]int{}
	m := g.re.FindStringSubmatchIndex(name)
	for i := 2; i+1 < len(m); i += 2 {
		if m[i] >= 0 && m[i+1] > m[i] {
			spans = append(spans, [2]int{m[i], m[i+1]})
		}
	}
	return spans
}

// SplitBySpans cuts s into alternating plain and highlighted parts: the
// result starts with a plain part, and every odd index is a highlighted span.
// spans must be ordered and must not overlap.
func SplitBySpans(s string, spans [][2]int) []string {
	parts := make([]string, 0, 2*len(spans)+1)
	pos := 0
	for _, sp := range spans {
		parts = append(parts, s[pos:sp[0]], s[sp[0]:sp[1]])
		pos = sp[1]
	}
	return append(parts, s[pos:])
}
