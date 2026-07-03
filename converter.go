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

// Internal placeholders for formatting markers. sanitizeControl strips these
// bytes from input, so they can never collide with user text.
const (
	phBold      = "\x01"
	phItalic    = "\x02"
	phUnderline = "\x03"
	phStrike    = "\x04"
	phSpoiler   = "\x05"
	phQuote     = "\x06"
)

// Delimiters for code-block and link placeholders, likewise stripped from
// input by sanitizeControl. Indices sit between open and close bytes, so
// placeholder N is never a substring of placeholder N0.
const (
	codeOpen  = "\x0E"
	codeClose = "\x0F"
	linkOpen  = "\x10"
	linkClose = "\x11"
)

// strippedControl lists every byte Convert reserves for internal use.
const strippedControl = "\x01\x02\x03\x04\x05\x06\x07\x08\x0E\x0F\x10\x11"

var (
	multilineCodeRe = regexp.MustCompile("(?s)```.*?```")
	// Double-backtick spans: content holds no backtick and no placeholder
	// byte, so a fenced-block placeholder is never swallowed.
	doubleBacktickRe = regexp.MustCompile("``([^`" + codeOpen + codeClose + linkOpen + linkClose + "]+?)``")
	// Inline-code content excludes the code-placeholder bytes so an already
	// isolated fenced block is never re-wrapped in a code span.
	specialInlineCodeRe = regexp.MustCompile("`([^`" + codeOpen + codeClose + "]*` +[\\p{L}\\p{N}_]+)`")
	inlineCodeRe        = regexp.MustCompile("`([^`" + codeOpen + codeClose + "]+?)`")
	// RE2's \s is ASCII-only; the class below also covers Unicode
	// whitespace, minus \n so a match can never leave its own line
	// (which would merge adjacent blockquotes or swallow blank lines).
	blockquoteRe = regexp.MustCompile(`(?m)^[\t\x0B\f\r\x1C-\x20\x85\p{Z}]*>[\t\x0B\f\r\x1C-\x20\x85\p{Z}]*(.*)`)
)

func codePlaceholder(i int) string { return codeOpen + strconv.Itoa(i) + codeClose }
func linkPlaceholder(i int) string { return linkOpen + strconv.Itoa(i) + linkClose }

func isSpecialByte(c byte) bool {
	return strings.IndexByte(specialChars, c) >= 0
}

// isWordRune reports whether r is a word character (a Unicode letter or
// number, or an underscore) — the Unicode-aware equivalent of \w.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// oddBackslashesBefore reports whether the byte at pos is preceded by an odd
// number of backslashes, i.e. whether it is escaped.
func oddBackslashesBefore(text string, pos int) bool {
	n := 0
	for pos-1-n >= 0 && text[pos-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}

// sanitizeControl removes the control bytes Convert reserves as internal
// placeholders. They have no legitimate rendering in a Telegram message, and
// stray occurrences would otherwise be rewritten into formatting markers.
func sanitizeControl(text string) string {
	if !strings.ContainsAny(text, strippedControl) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= 0x01 && c <= 0x08 || c >= 0x0E && c <= 0x11 {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// escapeCode escapes the characters MarkdownV2 requires escaped inside code
// entities: backslash and backtick.
func escapeCode(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "`", "\\`")
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

// replaceDelimited rewrites emphasis spans delimited by delim — a run of one
// repeated ASCII character d — emulating the pattern
//
//	(?<!\w)(?<!\\)D([^d]+?)D(?!\w)
//
// (RE2 has no lookbehind/lookahead) with these rules:
//
//   - Word boundary: a delimiter adjacent to a word character does not open or
//     close a span, so "snake_case" stays literal.
//   - Escapes: a delimiter preceded by an odd number of backslashes is escaped
//     and matches nothing, so "\_x\_" stays literal.
//   - Flanking: the content must not start or end with whitespace, so list
//     markers ("* item") and spaced operators ("5 * 3 * 2") stay literal.
//   - Runs: a delimiter adjacent to its own character is part of a longer run
//     (e.g. the stray tildes in "a~~b~~c" or the pipes in "a|||b|||c") and does
//     not match, so half of a run never opens a lone span.
//
// Content may not contain d, so the first d after the opener must begin a
// valid closing delimiter or the opener is abandoned. Failed candidates
// advance by one position, like a regex engine's scan.
func replaceDelimited(text, delim string, repl func(content string) string) string {
	d := delim[0]
	if !strings.Contains(text, delim) {
		return text
	}
	blocked := func(r rune) bool { return isWordRune(r) || r == rune(d) }

	var b strings.Builder
	last, i := 0, 0
	for i < len(text) {
		if !strings.HasPrefix(text[i:], delim) {
			i++
			continue
		}
		if i > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:i])
			if blocked(r) || oddBackslashesBefore(text, i) {
				i++
				continue
			}
		}
		rel := strings.IndexByte(text[i+len(delim):], d)
		if rel < 0 {
			break // no delimiter char remains anywhere
		}
		j := rel + i + len(delim)
		if !strings.HasPrefix(text[j:], delim) || oddBackslashesBefore(text, j) {
			// The only candidate closer is escaped or malformed, and
			// content may not contain d, so this opener cannot match.
			i++
			continue
		}
		content := text[i+len(delim) : j]
		if content == "" {
			i++
			continue
		}
		if after := j + len(delim); after < len(text) {
			r, _ := utf8.DecodeRuneInString(text[after:])
			if blocked(r) {
				i++
				continue
			}
		}
		first, _ := utf8.DecodeRuneInString(content)
		lastRune, _ := utf8.DecodeLastRuneInString(content)
		if unicode.IsSpace(first) || unicode.IsSpace(lastRune) {
			i++
			continue
		}
		b.WriteString(text[last:i])
		b.WriteString(repl(content))
		i = j + len(delim)
		last = i
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// isolateLinks replaces every [text](url) with the string returned by store.
// The text part may not contain ']'; the url part may contain balanced
// parentheses and ends at the ')' matching the opening one, so Wikipedia-style
// URLs survive intact. A '[' escaped with a backslash is left literal.
func isolateLinks(text string, store func(linkText, url string) string) string {
	if !strings.Contains(text, "](") {
		return text
	}
	var b strings.Builder
	last, i := 0, 0
	for i < len(text) {
		if text[i] != '[' || oddBackslashesBefore(text, i) {
			i++
			continue
		}
		closeBracket := strings.IndexByte(text[i+1:], ']')
		if closeBracket < 0 {
			break
		}
		closeBracket += i + 1
		if closeBracket == i+1 || closeBracket+1 >= len(text) || text[closeBracket+1] != '(' {
			i++
			continue
		}
		depth := 1
		j := closeBracket + 2
		for j < len(text) && depth > 0 {
			switch text[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth != 0 || j-1 == closeBracket+2 {
			i++
			continue
		}
		b.WriteString(text[last:i])
		b.WriteString(store(text[i+1:closeBracket], text[closeBracket+2:j-1]))
		i = j
		last = i
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// restorePlaceholders turns the formatting-marker bytes back into MarkdownV2
// syntax. Italic (_) and underline (__) share the underscore character and
// Telegram groups __ greedily left to right, so whenever two underscore
// delimiters would end up adjacent (at a nesting boundary such as ___x___) a
// carriage return is inserted between them — the spec's disambiguator — which
// keeps every underscore run unambiguous and every entity balanced.
func restorePlaceholders(text string) string {
	if !strings.ContainsAny(text, phBold+phItalic+phUnderline+phStrike+phSpoiler+phQuote) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + 8)
	var lastByte byte
	for i := 0; i < len(text); i++ {
		var out string
		switch text[i] {
		case phBold[0]:
			out = "*"
		case phItalic[0]:
			out = "_"
		case phUnderline[0]:
			out = "__"
		case phStrike[0]:
			out = "~"
		case phSpoiler[0]:
			out = "||"
		case phQuote[0]:
			out = ">"
		default:
			b.WriteByte(text[i])
			lastByte = text[i]
			continue
		}
		if out[0] == '_' && lastByte == '_' {
			b.WriteByte('\r')
		}
		b.WriteString(out)
		lastByte = out[len(out)-1]
	}
	return b.String()
}

// Convert converts a Markdown string to a Telegram-safe MarkdownV2 string.
//
// It works in five passes:
//  1. Bytes Convert reserves internally are stripped, then code blocks and
//     links are isolated behind placeholders. Code content keeps only the
//     escaping the MarkdownV2 spec requires (backslashes and backticks);
//     link URLs may contain balanced parentheses.
//  2. Markdown formatting is rewritten to internal placeholders. Emphasis
//     needs word boundaries and non-whitespace flanking, so list markers
//     ("* item") and spaced operators ("5 * 3") stay literal text.
//  3. All remaining special characters are escaped.
//  4. Formatting placeholders are restored as MarkdownV2 syntax, inserting the
//     spec's carriage-return disambiguator between adjacent underscore runs.
//  5. Links and code blocks are restored; link text is converted recursively,
//     and ')' and '\' inside URLs are escaped as the spec requires.
//
// Well-formed Markdown always converts to valid MarkdownV2. Deeply malformed
// input (for example crossing emphasis like "_~_~") can defeat the multi-pass
// conversion; rather than emit output Telegram would reject, Convert falls
// back to the input escaped as plain text, which is always safe to send.
func Convert(text string) string {
	text = sanitizeControl(text)
	if out := convert(text, false); validMarkdownV2(out) {
		return out
	}
	return EscapeSpecialChars(text)
}

// convert runs the five passes. When inline is true (converting link text)
// blockquotes are not recognized — a link's text is an inline context, and a
// '>' there is a reserved character that must stay escaped, not a quote.
func convert(text string, inline bool) string {
	if text == "" {
		return text
	}

	var codeBlocks []string
	type link struct{ text, url string }
	var links []link

	// --- Pass 1: isolate code blocks and links behind placeholders ---

	text = replaceAllSubmatchFunc(multilineCodeRe, text, func(groups []string) string {
		content := groups[0]
		var reconstructed string
		if firstNL := strings.IndexByte(content, '\n'); firstNL >= 0 {
			// ```lang on the opening line, code content after it. The
			// match always ends with the closing fence. Normalize the
			// opening fence to exactly ``` plus a backtick-free info
			// string; surplus backticks would open a nested code entity
			// inside the pre block and leave it unterminated.
			info := strings.ReplaceAll(strings.TrimLeft(content[:firstNL], "`"), "`", "")
			rest := content[firstNL+1:]
			escaped := escapeCode(rest[:len(rest)-3])
			if escaped != "" && !strings.HasSuffix(escaped, "\n") {
				escaped += "\n"
			}
			reconstructed = "```" + info + "\n" + escaped + "```"
		} else {
			// Single-line block: split the fences onto their own lines.
			reconstructed = "```\n" + escapeCode(content[3:len(content)-3]) + "\n```"
		}
		codeBlocks = append(codeBlocks, reconstructed)
		return codePlaceholder(len(codeBlocks) - 1)
	})

	// Double-backtick code spans: ``x`` means a code span containing x.
	text = replaceAllSubmatchFunc(doubleBacktickRe, text, func(groups []string) string {
		content := groups[1]
		// A single leading and trailing space is padding, not content —
		// but only when something other than spaces remains.
		if len(content) >= 2 && content[0] == ' ' && content[len(content)-1] == ' ' &&
			strings.TrimRight(content, " ") != "" {
			content = content[1 : len(content)-1]
		}
		codeBlocks = append(codeBlocks, "`"+escapeCode(content)+"`")
		return codePlaceholder(len(codeBlocks) - 1)
	})

	// Inline code whose content itself contains a backtick, e.g.
	// `code with \ and ` backticks`.
	text = replaceAllSubmatchFunc(specialInlineCodeRe, text, func(groups []string) string {
		codeBlocks = append(codeBlocks, "`"+escapeCode(groups[1])+"`")
		return codePlaceholder(len(codeBlocks) - 1)
	})

	text = replaceAllSubmatchFunc(inlineCodeRe, text, func(groups []string) string {
		// Content cannot contain backticks, so only backslashes need care.
		escaped := strings.ReplaceAll(groups[1], `\`, `\\`)
		codeBlocks = append(codeBlocks, "`"+escaped+"`")
		return codePlaceholder(len(codeBlocks) - 1)
	})

	text = isolateLinks(text, func(linkText, url string) string {
		links = append(links, link{text: linkText, url: url})
		return linkPlaceholder(len(links) - 1)
	})

	// --- Pass 2: rewrite markdown formatting to placeholders ---
	// Longest delimiters first so ***/___/~~ are not split by the shorter
	// passes. Telegram nests these freely, so a single conversion is always
	// valid; the carriage-return disambiguation happens at restore time.

	text = replaceDelimited(text, "***", func(c string) string {
		return phBold + phItalic + c + phItalic + phBold
	})
	text = replaceDelimited(text, "___", func(c string) string {
		return phUnderline + phItalic + c + phItalic + phUnderline
	})
	text = replaceDelimited(text, "__", func(c string) string {
		return phUnderline + c + phUnderline
	})
	text = replaceDelimited(text, "_", func(c string) string {
		return phItalic + c + phItalic
	})
	text = replaceDelimited(text, "~~", func(c string) string {
		return phStrike + c + phStrike
	})
	text = replaceDelimited(text, "~", func(c string) string {
		return phStrike + c + phStrike
	})
	text = replaceDelimited(text, "||", func(c string) string {
		return phSpoiler + c + phSpoiler
	})
	text = replaceDelimited(text, "**", func(c string) string {
		return phBold + c + phBold
	})
	text = replaceDelimited(text, "*", func(c string) string {
		return phItalic + c + phItalic
	})
	if !inline {
		text = blockquoteRe.ReplaceAllString(text, phQuote+"${1}")
	}

	// --- Pass 3: escape all remaining special characters ---
	text = EscapeSpecialChars(text)

	// --- Pass 4: restore formatting placeholders ---
	text = restorePlaceholders(text)

	// --- Pass 5: restore links, then code blocks ---
	// Links go first: their text may contain code placeholders.
	for i := len(links) - 1; i >= 0; i-- {
		url := links[i].url
		// A code span isolated inside the URL would reinsert raw backticks
		// and parens after escaping; expand it here so its bytes are
		// escaped as part of the URL instead.
		if strings.Contains(url, codeOpen) {
			for k := len(codeBlocks) - 1; k >= 0; k-- {
				url = strings.ReplaceAll(url, codePlaceholder(k), codeBlocks[k])
			}
		}
		url = strings.ReplaceAll(url, `\`, `\\`)
		url = strings.ReplaceAll(url, ")", `\)`)
		text = strings.ReplaceAll(text, linkPlaceholder(i), "["+convert(links[i].text, true)+"]("+url+")")
	}
	for i := len(codeBlocks) - 1; i >= 0; i-- {
		text = strings.ReplaceAll(text, codePlaceholder(i), codeBlocks[i])
	}

	return text
}
