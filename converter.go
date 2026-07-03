// Package tgmarkdown converts standard Markdown to Telegram-safe MarkdownV2.
//
// Code blocks and links are isolated first, formatting is converted through
// internal placeholders, the remaining special characters are escaped, and
// everything is restored at the end.
package tgmarkdown

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// specialChars are the characters that must be escaped in Telegram
// MarkdownV2. '>' is included and handled separately for blockquotes.
const specialChars = "_*[]()~`>#+-=|{}.!"

// Internal placeholders for formatting markers. Control characters are used
// so they can never collide with printable input text.
const (
	phBold            = "\x01"
	phItalic          = "\x02"
	phUnderline       = "\x03"
	phStrike          = "\x04"
	phSpoiler         = "\x05"
	phQuote           = "\x06"
	phPreservedBold   = "\x07" // ** kept literally (content holds underline formatting)
	phPreservedItalic = "\x08" // * kept literally (content holds strikethrough formatting)
)

// Word characters are matched Unicode-aware: [\p{L}\p{N}_] stands in for
// RE2's ASCII-only \w.
var (
	multilineCodeRe     = regexp.MustCompile("(?s)```.*?```")
	specialInlineCodeRe = regexp.MustCompile("`([^`]*` +[\\p{L}\\p{N}_]+)`")
	inlineCodeRe        = regexp.MustCompile("`([^`]+?)`")
	linkRe              = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	tripleAsteriskRe    = regexp.MustCompile(`\*\*\*([^*]+?)\*\*\*`)
	tripleUnderscoreRe  = regexp.MustCompile(`___([^_]+?)___`)
	doubleUnderscoreRe  = regexp.MustCompile(`__([^_]+?)__`)
	doubleTildeRe       = regexp.MustCompile(`~~([^~]+?)~~`)
	singleTildeRe       = regexp.MustCompile(`~([^~]+?)~`)
	spoilerRe           = regexp.MustCompile(`\|\|([^|]+?)\|\|`)
	// RE2's \s is ASCII-only; the class below also covers Unicode
	// whitespace (\t-\r, \x1c-\x1f, space, NEL, and the Z* separator
	// categories).
	blockquoteRe     = regexp.MustCompile(`(?m)^[\t-\r\x1c-\x20\x85\p{Z}]*>[\t-\r\x1c-\x20\x85\p{Z}]*(.*)`)
	doubleAsteriskRe = regexp.MustCompile(`\*\*([^*]+?)\*\*`)
)

// placeholderRestorer maps the internal placeholders back to MarkdownV2
// syntax. Placeholders are distinct single bytes and no replacement contains
// one, so a single pass is equivalent to sequential replacement.
var placeholderRestorer = strings.NewReplacer(
	phBold, "*",
	phItalic, "_",
	phUnderline, "__",
	phStrike, "~",
	phSpoiler, "||",
	phQuote, ">",
	phPreservedBold, "**",
	phPreservedItalic, "*",
)

func codePlaceholder(i int) string { return "zxzC" + strconv.Itoa(i) + "zxz" }
func linkPlaceholder(i int) string { return "zxzL" + strconv.Itoa(i) + "zxz" }

func isSpecialByte(c byte) bool {
	return strings.IndexByte(specialChars, c) >= 0
}

// isWordRune reports whether r is a word character (a Unicode letter or
// number, or an underscore) — the Unicode-aware equivalent of \w.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// EscapeSpecialChars escapes MarkdownV2 special characters in text without
// double-escaping sequences that are already escaped.
func EscapeSpecialChars(text string) string {
	if !strings.ContainsAny(text, specialChars+`\`) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + 16)
	last := 0
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '\\':
			if i+1 < len(text) && (isSpecialByte(text[i+1]) || text[i+1] == '\\') {
				// Already-escaped sequence: keep it as is.
				b.WriteString(text[last:i])
				b.WriteString(text[i : i+2])
				i += 2
			} else {
				// Bare backslash: escape the backslash itself.
				b.WriteString(text[last:i])
				b.WriteString(`\\`)
				i++
			}
			last = i
		case isSpecialByte(c):
			b.WriteString(text[last:i])
			b.WriteByte('\\')
			b.WriteByte(c)
			i++
			last = i
		default:
			i++
		}
	}
	b.WriteString(text[last:])
	return b.String()
}

// replaceAllSubmatchFunc is regexp.ReplaceAllStringFunc with access to
// capture groups: repl receives the full match followed by each group.
func replaceAllSubmatchFunc(re *regexp.Regexp, s string, repl func(groups []string) string) string {
	matches := re.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		groups := make([]string, len(m)/2)
		for g := range groups {
			if m[2*g] >= 0 {
				groups[g] = s[m[2*g]:m[2*g+1]]
			}
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(repl(groups))
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// replaceSingleDelimited emulates the pattern
//
//	(?<!\w)(?<!\\)X([^X]+?)X(?!\w)
//
// for a single-byte delimiter X, since RE2 has no lookbehind/lookahead.
// A match opens at a delimiter not preceded by a word character (nor by a
// backslash, when forbidBackslashBefore is set), spans non-empty content free
// of the delimiter, and closes at a delimiter not followed by a word
// character. Failed candidates advance by one position, like a regex
// engine's scan.
func replaceSingleDelimited(text string, delim byte, forbidBackslashBefore bool, repl func(content string) string) string {
	if strings.IndexByte(text, delim) < 0 {
		return text
	}
	var b strings.Builder
	last, i := 0, 0
	for i < len(text) {
		if text[i] != delim {
			i++
			continue
		}
		if i > 0 {
			if r, _ := utf8.DecodeLastRuneInString(text[:i]); isWordRune(r) {
				i++
				continue
			}
			if forbidBackslashBefore && text[i-1] == '\\' {
				i++
				continue
			}
		}
		j := strings.IndexByte(text[i+1:], delim)
		if j < 0 {
			break // no closing delimiter left anywhere
		}
		j += i + 1
		if j == i+1 {
			i++ // empty content cannot match [^X]+?
			continue
		}
		if j+1 < len(text) {
			if r, _ := utf8.DecodeRuneInString(text[j+1:]); isWordRune(r) {
				i++
				continue
			}
		}
		b.WriteString(text[last:i])
		b.WriteString(repl(text[i+1 : j]))
		i = j + 1
		last = i
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// Convert converts a Markdown string to a Telegram-safe MarkdownV2 string.
//
// It works in five passes:
//  1. Code blocks and links are isolated behind safe placeholders. Fenced
//     blocks spanning lines get backslashes and backticks in their content
//     escaped; a single-line fenced block is kept verbatim with the fence
//     split onto its own line; inline code gets backslashes escaped (and
//     backticks too when the content itself contains one).
//  2. Markdown formatting is rewritten to internal placeholders.
//  3. All remaining special characters are escaped.
//  4. Formatting placeholders are restored as MarkdownV2 syntax.
//  5. Links and code blocks are restored; link text is converted recursively
//     to handle nested formatting.
func Convert(text string) string {
	if text == "" {
		return text
	}

	var codeBlocks []string
	type link struct{ text, url string }
	var links []link

	// --- Pass 1: isolate code blocks and links behind placeholders ---

	text = replaceAllSubmatchFunc(multilineCodeRe, text, func(groups []string) string {
		content := groups[0]
		reconstructed := content
		if firstNL := strings.IndexByte(content, '\n'); firstNL >= 0 {
			// ```lang on the opening line, code content after it.
			opening := content[:firstNL]
			rest := content[firstNL+1:]
			if strings.HasSuffix(rest, "```") {
				escaped := strings.ReplaceAll(rest[:len(rest)-3], `\`, `\\`)
				escaped = strings.ReplaceAll(escaped, "`", "\\`")
				reconstructed = opening + "\n" + escaped + "\n```"
			}
		} else {
			// Single-line block: split the fence onto its own line.
			reconstructed = content[:3] + "\n" + content[3:]
		}
		codeBlocks = append(codeBlocks, reconstructed)
		return codePlaceholder(len(codeBlocks) - 1)
	})

	// Inline code whose content itself contains a backtick, e.g.
	// `code with \ and ` backticks`.
	text = replaceAllSubmatchFunc(specialInlineCodeRe, text, func(groups []string) string {
		escaped := strings.ReplaceAll(groups[1], `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, "`", "\\`")
		codeBlocks = append(codeBlocks, "`"+escaped+"`")
		return codePlaceholder(len(codeBlocks) - 1)
	})

	text = replaceAllSubmatchFunc(inlineCodeRe, text, func(groups []string) string {
		escaped := strings.ReplaceAll(groups[1], `\`, `\\`)
		codeBlocks = append(codeBlocks, "`"+escaped+"`")
		return codePlaceholder(len(codeBlocks) - 1)
	})

	text = replaceAllSubmatchFunc(linkRe, text, func(groups []string) string {
		links = append(links, link{text: groups[1], url: groups[2]})
		return linkPlaceholder(len(links) - 1)
	})

	// --- Pass 2: rewrite markdown formatting to placeholders ---

	text = tripleAsteriskRe.ReplaceAllString(text, phBold+phItalic+"${1}"+phItalic+phBold)
	text = tripleUnderscoreRe.ReplaceAllString(text, phUnderline+phItalic+"${1}"+phItalic+phUnderline)
	text = doubleUnderscoreRe.ReplaceAllString(text, phUnderline+"${1}"+phUnderline)
	text = replaceSingleDelimited(text, '_', false, func(content string) string {
		return phItalic + content + phItalic
	})
	text = doubleTildeRe.ReplaceAllString(text, phStrike+"${1}"+phStrike)
	text = singleTildeRe.ReplaceAllString(text, phStrike+"${1}"+phStrike)
	text = spoilerRe.ReplaceAllString(text, phSpoiler+"${1}"+phSpoiler)
	text = blockquoteRe.ReplaceAllString(text, phQuote+"${1}")

	// **bold** whose content holds underline formatting must keep its literal
	// asterisks: Telegram cannot nest *…* around __…__.
	text = replaceAllSubmatchFunc(doubleAsteriskRe, text, func(groups []string) string {
		content := groups[1]
		if strings.Contains(content, "___") || strings.Contains(content, phUnderline) {
			return phPreservedBold + content + phPreservedBold
		}
		return phBold + content + phBold
	})

	// *italic*, keeping the literal asterisk when the content holds
	// strikethrough formatting.
	text = replaceSingleDelimited(text, '*', true, func(content string) string {
		if strings.Contains(content, phStrike) {
			return phPreservedItalic + content + phPreservedItalic
		}
		return phItalic + content + phItalic
	})

	// --- Pass 3: escape all remaining special characters ---
	text = EscapeSpecialChars(text)

	// --- Pass 4: restore formatting placeholders ---
	text = placeholderRestorer.Replace(text)

	// --- Pass 5: restore links, then code blocks ---
	// Links go first: their text may contain code placeholders.
	for i := len(links) - 1; i >= 0; i-- {
		text = strings.ReplaceAll(text, linkPlaceholder(i), "["+Convert(links[i].text)+"]("+links[i].url+")")
	}
	for i := len(codeBlocks) - 1; i >= 0; i-- {
		text = strings.ReplaceAll(text, codePlaceholder(i), codeBlocks[i])
	}

	return text
}
