// Package tgmarkdown converts a focused Markdown dialect to Telegram-safe
// MarkdownV2.
//
// Links, unsupported images, and code are structurally isolated first,
// formatting is converted through internal placeholders, the remaining
// special characters are escaped, and everything is restored at the end.
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
	phClose     = "\x12" // followed by the emphasis marker being closed
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
const strippedControl = "\x01\x02\x03\x04\x05\x06\x07\x08\x0E\x0F\x10\x11\x12"

var (
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

// isWordRune reports whether r continues a word when only that rune is
// available. Marks and join controls are included so right-hand delimiter
// boundaries stay conservative.
func isWordRune(r rune) bool {
	return r == '_' || r == '\u200C' || r == '\u200D' ||
		unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
}

// isWordBefore classifies the grapheme immediately before pos. Trailing marks,
// variation selectors, and join controls inherit their base character, so NFD
// letters and ideographic variation sequences remain word-bound. U+20E3 forms
// a keycap emoji and therefore remains a symbol boundary even though Unicode
// categorises it as a mark.
func isWordBefore(text string, pos int) bool {
	for pos > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:pos])
		if r == '\u20E3' {
			return false
		}
		if unicode.IsMark(r) || r == '\u200C' || r == '\u200D' {
			pos -= size
			continue
		}
		return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
	}
	return false
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
		if c < ' ' && strings.IndexByte(strippedControl, c) >= 0 {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// normaliseLineEndings applies CommonMark's line preprocessing before any
// structure is scanned. Telegram later discards bare carriage returns, so
// retaining source CR bytes would collapse code and blockquote lines.
func normaliseLineEndings(text string) string {
	if !strings.ContainsRune(text, '\r') {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
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
	return escapeSpecialChars(strings.ToValidUTF8(text, "\uFFFD"))
}

// escapeSpecialChars is the internal fast path. Convert normalises UTF-8 once
// at its public boundary, so recursive conversion does not need to rescan it.
func escapeSpecialChars(text string) string {
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
// Escaped delimiters and delimiters inside a different already-matched
// emphasis span cannot close an opener. Index the unescaped positions within
// each enclosing span so that looking for a closer does not rescan the text.
func replaceDelimited(text, delim string, repl func(content string) string) string {
	d := delim[0]
	firstDelimiter := strings.IndexByte(text, d)
	if firstDelimiter < 0 || !strings.Contains(text[firstDelimiter:], delim) {
		return text
	}
	width := len(delim)
	runs := indexDelimiterRuns(text, delim)
	var b strings.Builder
	last := 0
	for pos, opener := range runs {
		i := opener.start
		if i < last || i != opener.closer || opener.length >= 2*width {
			continue
		}
		if isWordBefore(text, i) {
			continue
		}
		closer := opener.next
		if closer < 0 {
			// Retain a crossing candidate for the renderer to reject
			// rather than silently formatting only one of the spans.
			closer = pos + 1
			if closer == len(runs) {
				continue
			}
		}
		j := runs[closer].closer
		content := text[i+width : j]
		first, _ := utf8.DecodeRuneInString(content)
		previous, _ := utf8.DecodeLastRuneInString(content)
		if unicode.IsSpace(first) || unicode.IsSpace(previous) {
			continue
		}
		after := j + width
		if after < len(text) {
			next, _ := utf8.DecodeRuneInString(text[after:])
			if isWordRune(next) || next == rune(d) && i == firstDelimiter {
				continue
			}
			// An opening bracket or quote directly before a run favours
			// a nested opener if it has content on the right and another
			// delimiter follows. Match that inner pair on this pass.
			if isOpeningPunctuation(previous) && !unicode.IsSpace(next) &&
				canOpenNestedSpan(text, runs, closer, width) {
				continue
			}
		}
		b.WriteString(text[last:i])
		b.WriteString(repl(content))
		last = after
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func canOpenNestedSpan(text string, runs []delimiterRun, opener, width int) bool {
	run := runs[opener]
	if run.start != run.closer || run.length >= 2*width || run.next < 0 {
		return false
	}
	closer := runs[run.next]
	if closer.next < 0 {
		return false // the enclosing span needs a closer of its own
	}
	return hasClosingBoundary(text, closer.closer, width) &&
		hasClosingBoundary(text, runs[closer.next].closer, width)
}

func hasClosingBoundary(text string, start, width int) bool {
	last, _ := utf8.DecodeLastRuneInString(text[:start])
	next, _ := utf8.DecodeRuneInString(text[start+width:])
	return !unicode.IsSpace(last) && !isWordRune(next)
}

func isOpeningPunctuation(r rune) bool {
	return unicode.Is(unicode.Ps, r) || unicode.Is(unicode.Pi, r)
}

type delimiterRun struct {
	start  int
	length int
	closer int
	next   int
}

// Index each unescaped run once, with a separate chain for every enclosing
// span. A run can close at its second byte if only its first byte is escaped.
func indexDelimiterRuns(text, delim string) []delimiterRun {
	var runs []delimiterRun
	var scope emphasisScope
	previous := make(map[int]int)
	for i := 0; i < len(text); {
		if width := scope.consume(text[i], i); width != 0 {
			i += width
			continue
		}
		if text[i] != delim[0] {
			i++
			continue
		}
		start := i
		for i < len(text) && text[i] == delim[0] {
			i++
		}
		closer := start
		if oddBackslashesBefore(text, start) {
			closer++
		}
		if i-closer < len(delim) {
			continue
		}
		enclosing := scope.current()
		if prior, ok := previous[enclosing]; ok {
			runs[prior].next = len(runs)
		}
		previous[enclosing] = len(runs)
		runs = append(runs, delimiterRun{start: start, length: i - start, closer: closer, next: -1})
	}
	return runs
}

// An opening marker's offset identifies its scope, including when several
// distinct spans use the same style. The outermost text has scope zero.
type emphasisScope []int

func (s *emphasisScope) consume(marker byte, offset int) int {
	switch marker {
	case phBold[0], phItalic[0], phUnderline[0], phStrike[0], phSpoiler[0]:
		*s = append(*s, offset+1)
		return 1
	case phClose[0]:
		if len(*s) > 0 {
			*s = (*s)[:len(*s)-1]
		}
		return 2 // the following byte names the closing style
	default:
		return 0
	}
}

func (s emphasisScope) current() int {
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

// restorePlaceholders renders balanced emphasis after the link labels have
// been expanded. A nested span of the same style needs no extra delimiters:
// Telegram would interpret them as closing and reopening the outer span.
func restorePlaceholders(text string) (string, bool) {
	if !strings.ContainsAny(text, phBold+phItalic+phUnderline+phStrike+phSpoiler+phQuote+phClose) {
		return text, true
	}
	var b strings.Builder
	b.Grow(len(text) + 8)
	var stack []byte
	var depth [6]int // emphasis markers occupy bytes 1 through 5
	var lastByte byte
	for i := 0; i < len(text); i++ {
		closing := text[i] == phClose[0]
		if closing {
			i++
			if i == len(text) {
				return "", false
			}
		}
		marker := text[i]
		var out string
		switch marker {
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
			if closing {
				return "", false
			}
			out = ">"
		default:
			if closing {
				return "", false
			}
			b.WriteByte(marker)
			lastByte = marker
			continue
		}

		if closing {
			if len(stack) == 0 || stack[len(stack)-1] != marker {
				return "", false
			}
			stack = stack[:len(stack)-1]
			depth[marker]--
			if depth[marker] > 0 {
				continue
			}
		} else if marker != phQuote[0] {
			stack = append(stack, marker)
			depth[marker]++
			if depth[marker] > 1 {
				continue
			}
		}

		// Telegram groups __ greedily. Separate adjacent underscore markers
		// with the MarkdownV2 carriage-return disambiguator.
		if out[0] == '_' && lastByte == '_' {
			b.WriteByte('\r')
		}
		b.WriteString(out)
		lastByte = out[len(out)-1]
	}
	if len(stack) != 0 {
		return "", false
	}
	return b.String(), true
}

// Convert converts a Markdown string to a Telegram-safe MarkdownV2 string.
//
// It works in five passes:
//  1. Bytes Convert reserves internally are stripped, then links, unsupported
//     images, and code are isolated behind placeholders. Destinations are
//     consumed before their literal backticks can pair with surrounding prose.
//     Code content keeps only the escaping MarkdownV2 requires; link URLs may
//     contain balanced parentheses.
//  2. Markdown formatting is rewritten to internal placeholders. Emphasis
//     needs word boundaries and non-whitespace flanking, so list markers
//     ("* item") and spaced operators ("5 * 3") stay literal text.
//  3. All remaining special characters are escaped.
//  4. Links and code blocks are restored; link text is converted recursively,
//     and ')' and '\' inside URLs are escaped as the spec requires.
//  5. Balanced formatting placeholders are rendered as MarkdownV2 syntax.
//     Duplicate nested styles are flattened and adjacent underscore markers
//     are separated by the MarkdownV2 carriage-return disambiguator.
//
// Well-formed Markdown always converts to valid MarkdownV2. Deeply malformed
// input (for example crossing emphasis like "_~_~") can defeat the multi-pass
// conversion; rather than emit output Telegram would reject, Convert falls
// back to the input escaped as plain text, which is always safe to send.
func Convert(text string) string {
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = normaliseLineEndings(text)
	text = sanitizeControl(text)
	if out, balanced := restorePlaceholders(convert(text, false)); balanced && validMarkdownV2(out) {
		return out
	}
	return escapeSpecialChars(text)
}

// convert leaves the formatting markers in place so that the caller can
// render emphasis spanning both the surrounding text and link labels.
// Only the outer conversion recognises blockquotes.
func convert(text string, inline bool) string {
	if text == "" {
		return text
	}

	var codeBlocks []string
	type link struct {
		text, url string
		literal   string
	}
	var links []link

	// --- Pass 1: isolate links, images, and code behind placeholders ---

	// Link discovery uses a code-span skip index without replacing code yet.
	// Confirmed destinations are therefore consumed atomically: their literal
	// backticks cannot pair with prose outside the URL, while brackets inside
	// real code remain opaque.
	text = isolateInlineLinks(text, func(linkText, url string) string {
		links = append(links, link{text: linkText, url: url})
		return linkPlaceholder(len(links) - 1)
	})
	text = isolateImages(text, func(source string) string {
		links = append(links, link{literal: source})
		return linkPlaceholder(len(links) - 1)
	})
	text = isolateCode(text, func(_, rendered string) string {
		codeBlocks = append(codeBlocks, rendered)
		return codePlaceholder(len(codeBlocks) - 1)
	})

	if inline {
		// Only ordinary label text interprets Markdown backslash escapes.
		// Code and unsupported images are opaque by this point.
		text = normaliseCommonMarkLinkTextEscapes(text)
	}

	// --- Pass 2: rewrite Markdown formatting to placeholders ---
	// Match the non-asterisk styles again after inner spans become opaque.
	// Each successful match removes source delimiters. Bound the number of
	// passes so deeply nested input cannot cause unbounded rescanning.
	const maxEmphasisPasses = 32
	for range maxEmphasisPasses {
		before := text
		text = replaceDelimited(text, "___", func(c string) string {
			return phUnderline + phItalic + c + phClose + phItalic + phClose + phUnderline
		})
		text = replaceDelimited(text, "__", func(c string) string {
			return phUnderline + c + phClose + phUnderline
		})
		text = replaceDelimited(text, "_", func(c string) string {
			return phItalic + c + phClose + phItalic
		})
		// Resolve enclosing underscores before the asterisk pass tests word
		// boundaries. Raw underscores would hide valid asterisk runs.
		text = replaceAsteriskEmphasis(text, true)
		text = replaceDelimited(text, "~~", func(c string) string {
			return phStrike + c + phClose + phStrike
		})
		text = replaceDelimited(text, "~", func(c string) string {
			return phStrike + c + phClose + phStrike
		})
		text = replaceDelimited(text, "||", func(c string) string {
			return phSpoiler + c + phClose + phSpoiler
		})
		if text == before {
			break
		}
	}
	// Once every within-scope asterisk pair is resolved, try the remaining
	// pairs across scope boundaries. The renderer rejects crossing spans
	// so malformed input keeps the documented plain-text fallback.
	if strings.Contains(text, phClose) {
		text = replaceAsteriskEmphasis(text, false)
	}
	if !inline {
		text = blockquoteRe.ReplaceAllString(text, phQuote+"${1}")
	}

	// --- Pass 3: escape all remaining special characters ---
	text = escapeSpecialChars(text)

	// --- Pass 4: restore links, then code blocks ---
	// Each placeholder kind is expanded in one indexed pass; repeatedly calling
	// strings.ReplaceAll here made span-heavy input quadratic.
	renderedLinks := make([]string, len(links))
	for i := range links {
		if links[i].literal != "" {
			renderedLinks[i] = escapeSpecialChars(links[i].literal)
			continue
		}

		url := links[i].url
		url = strings.ReplaceAll(url, `\`, `\\`)
		url = strings.ReplaceAll(url, ")", `\)`)
		renderedLinks[i] = "[" + convert(links[i].text, true) + "](" + url + ")"
	}
	text = expandIndexedPlaceholders(text, linkOpen[0], linkClose[0], renderedLinks)
	text = expandIndexedPlaceholders(text, codeOpen[0], codeClose[0], codeBlocks)

	return text
}
