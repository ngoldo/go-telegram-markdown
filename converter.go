// Package tgmarkdown converts a focused Markdown dialect to Telegram-safe
// MarkdownV2.
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

// isWordRune reports whether r continues a word. Marks and join controls are
// included so canonically equivalent text has the same delimiter boundaries:
// cafe\u0301_test_ must behave like café_test_.
func isWordRune(r rune) bool {
	return r == '_' || r == '\u200C' || r == '\u200D' ||
		unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
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
// Escaped delimiter bytes and shorter runs may remain literal content, so the
// closer search advances past them to the next complete unescaped candidate.
// Failed opener candidates advance by one position, like a regex engine's
// scan.
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
		search := i + len(delim)
		j := -1
		for search < len(text) {
			rel := strings.IndexByte(text[search:], d)
			if rel < 0 {
				break
			}
			candidate := search + rel
			if strings.HasPrefix(text[candidate:], delim) &&
				!oddBackslashesBefore(text, candidate) {
				j = candidate
				break
			}
			// An escaped delimiter is literal content, not proof that the
			// opener has no later closer: *foo \* bar* must keep looking.
			search = candidate + 1
		}
		if j < 0 {
			break // no delimiter char remains anywhere
		}
		content := text[i+len(delim) : j]
		if content == "" {
			i++
			continue
		}
		if after := j + len(delim); after < len(text) {
			r, _ := utf8.DecodeRuneInString(text[after:])
			switch {
			case isWordRune(r):
				// Flanking: a closer flush against a word character does not
				// close, so "file~1" and "a**b**c" stay literal.
				i++
				continue
			case r == rune(d) && strings.IndexByte(text[:i], d) < 0:
				// The closer is followed by a run of its own delimiter char and
				// no same-delimiter opener precedes this span: the closer is part
				// of a longer run ("a~~b~~c"), not a nested closer, so it cannot
				// match. When a same-delimiter opener *does* precede, the trailing
				// run is an enclosing span's flush closer — "**a *b***" is
				// bold(a, italic(b)), whose inner *b* closes flush against the
				// outer ** — so allow the match and let the fixpoint resolve the
				// outer span. A wrong guess still fails validMarkdownV2 and falls
				// back to escaped plain text.
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
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = sanitizeControl(text)
	if out := convert(text, false); validMarkdownV2(out) {
		return out
	}
	return escapeSpecialChars(text)
}

// convert runs the five passes. When inline is true (converting link text)
// blockquotes are not recognised — a link's text is an inline context, and a
// '>' there is a reserved character that must stay escaped, not a quote.
func convert(text string, inline bool) string {
	if text == "" {
		return text
	}

	var codeBlocks []string
	type link struct{ text, url string }
	var links []link

	// --- Pass 1: isolate code blocks and links behind placeholders ---

	text = isolateCode(text, func(rendered string) string {
		codeBlocks = append(codeBlocks, rendered)
		return codePlaceholder(len(codeBlocks) - 1)
	})

	text = isolateInlineLinks(text, func(linkText, url string) string {
		links = append(links, link{text: linkText, url: url})
		return linkPlaceholder(len(links) - 1)
	})

	// --- Pass 2: rewrite markdown formatting to placeholders ---
	// Asterisk delimiter runs are resolved together by a stack pass, so
	// flush-opening, flush-closing, and duplicate nested emphasis are handled
	// symmetrically. The other delimiters retain the fixpoint passes used for
	// same-character nesting. Telegram nests all of them freely; underscore
	// disambiguation happens at restore time.
	//
	// Repeat the passes to a fixpoint. Same-character nesting — a *italic*
	// span inside a **bold** span, or vice versa — can't resolve in one
	// sweep: replaceDelimited abandons a span whose content still holds its
	// own delimiter char, so the outer span only matches after its inner
	// span becomes a placeholder. Each conversion replaces delimiters with
	// non-delimiter placeholder bytes, so the delimiter count strictly
	// drops and this converges in a couple of iterations. The loop always
	// exits early via `text == before`; maxEmphasisPasses is a
	// belt-and-braces bound for adversarial input. Anything unresolved at
	// that point remains literal or triggers the valid plain-text fallback.
	const maxEmphasisPasses = 32
	for range maxEmphasisPasses {
		before := text
		text = replaceAsteriskEmphasis(text)
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
		if text == before {
			break
		}
	}
	if !inline {
		text = blockquoteRe.ReplaceAllString(text, phQuote+"${1}")
	}

	// --- Pass 3: escape all remaining special characters ---
	text = escapeSpecialChars(text)

	// --- Pass 4: restore formatting placeholders ---
	text = restorePlaceholders(text)

	// --- Pass 5: restore links, then code blocks ---
	// Links go first because their text and URLs may contain code placeholders.
	// Each placeholder kind is expanded in one indexed pass; repeatedly calling
	// strings.ReplaceAll here made span-heavy input quadratic.
	renderedLinks := make([]string, len(links))
	for i := range links {
		url := links[i].url
		// A code span isolated inside the URL would reinsert raw backticks
		// and parens after escaping; expand it here so its bytes are
		// escaped as part of the URL instead.
		if strings.Contains(url, codeOpen) {
			url = expandIndexedPlaceholders(url, codeOpen[0], codeClose[0], codeBlocks)
		}
		url = strings.ReplaceAll(url, `\`, `\\`)
		url = strings.ReplaceAll(url, ")", `\)`)
		renderedLinks[i] = "[" + convert(links[i].text, true) + "](" + url + ")"
	}
	text = expandIndexedPlaceholders(text, linkOpen[0], linkClose[0], renderedLinks)
	text = expandIndexedPlaceholders(text, codeOpen[0], codeClose[0], codeBlocks)

	return text
}
