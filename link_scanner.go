package tgmarkdown

import "strings"

// isolateInlineLinks replaces each inline Markdown link with the value returned
// by store. It recognises nested brackets in link text, balanced parentheses in
// destinations, and angle-bracket destinations. URL backslash escapes are
// decoded before the values are handed to store. Link-text escapes which carry
// Markdown meaning remain escaped for the caller's recursive inline conversion;
// escapes of other ASCII punctuation are decoded immediately.
//
// The structural indexes are built in one pass. In particular, a malformed
// destination is never rescanned for each preceding '[', keeping the scanner
// linear for adversarial input as well as ordinary prose.
func isolateInlineLinks(text string, store func(linkText, url string) string) string {
	if text == "" || !strings.Contains(text, "](") {
		return text
	}

	index := newInlineLinkIndex(text)
	openBrackets := make([]int, 0, 8)

	var out strings.Builder
	last := 0
	changed := false

	for i := 0; i < len(text); i++ {
		if index.escaped[i] {
			continue
		}

		switch text[i] {
		case '[':
			openBrackets = append(openBrackets, i)

		case ']':
			if len(openBrackets) == 0 {
				continue
			}

			openBracket := openBrackets[len(openBrackets)-1]
			openBrackets = openBrackets[:len(openBrackets)-1]
			if i == openBracket+1 ||
				i+1 >= len(text) ||
				text[i+1] != '(' ||
				index.escaped[i+1] {
				continue
			}

			urlStart, urlEnd, linkEnd, ok := index.destination(text, i+1)
			if !ok || openBracket < last {
				continue
			}

			out.WriteString(text[last:openBracket])
			linkText := normaliseCommonMarkLinkTextEscapes(text[openBracket+1 : i])
			url := decodeCommonMarkEscapes(text[urlStart:urlEnd])
			out.WriteString(store(linkText, url))

			last = linkEnd
			changed = true
			i = linkEnd - 1
		}
	}

	if !changed {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

type inlineLinkIndex struct {
	escaped       []bool
	parenClose    []int
	nextAngleEnd  []int
	nextAngleOpen []int
	nextLineEnd   []int
}

func newInlineLinkIndex(text string) inlineLinkIndex {
	escaped := inlineLinkEscapedBytes(text)
	parenClose := make([]int, len(text))
	for i := range parenClose {
		parenClose[i] = -1
	}

	openParens := make([]int, 0, 8)
	for i := 0; i < len(text); i++ {
		if escaped[i] {
			continue
		}
		switch text[i] {
		case '(':
			openParens = append(openParens, i)
		case ')':
			if len(openParens) == 0 {
				continue
			}
			open := openParens[len(openParens)-1]
			openParens = openParens[:len(openParens)-1]
			parenClose[open] = i
		}
	}

	nextAngleEnd := make([]int, len(text)+1)
	nextAngleOpen := make([]int, len(text)+1)
	nextLineEnd := make([]int, len(text)+1)
	nextEnd, nextOpen, nextLine := -1, -1, -1
	nextAngleEnd[len(text)] = -1
	nextAngleOpen[len(text)] = -1
	nextLineEnd[len(text)] = -1
	for i := len(text) - 1; i >= 0; i-- {
		if !escaped[i] {
			switch text[i] {
			case '>':
				nextEnd = i
			case '<':
				nextOpen = i
			case '\n', '\r':
				nextLine = i
			}
		}
		nextAngleEnd[i] = nextEnd
		nextAngleOpen[i] = nextOpen
		nextLineEnd[i] = nextLine
	}

	return inlineLinkIndex{
		escaped:       escaped,
		parenClose:    parenClose,
		nextAngleEnd:  nextAngleEnd,
		nextAngleOpen: nextAngleOpen,
		nextLineEnd:   nextLineEnd,
	}
}

// destination returns the unwrapped destination bounds and the position just
// after the link. openParen is the '(' immediately following the link label.
func (index inlineLinkIndex) destination(text string, openParen int) (
	urlStart, urlEnd, linkEnd int, ok bool,
) {
	start := openParen + 1
	if start >= len(text) {
		return 0, 0, 0, false
	}

	if text[start] != '<' || index.escaped[start] {
		closeParen := index.parenClose[openParen]
		if closeParen <= start {
			return 0, 0, 0, false
		}
		return start, closeParen, closeParen + 1, true
	}

	// An angle destination ends at the first unescaped '>'. A further
	// unescaped '<' or a line ending before it makes the wrapper malformed.
	closeAngle := index.nextAngleEnd[start+1]
	if closeAngle <= start+1 ||
		indexBefore(index.nextAngleOpen[start+1], closeAngle) ||
		indexBefore(index.nextLineEnd[start+1], closeAngle) {
		return 0, 0, 0, false
	}
	closeParen := closeAngle + 1
	if closeParen >= len(text) || text[closeParen] != ')' || index.escaped[closeParen] {
		return 0, 0, 0, false
	}
	return start + 1, closeAngle, closeParen + 1, true
}

func indexBefore(candidate, limit int) bool {
	return candidate >= 0 && candidate < limit
}

// inlineLinkEscapedBytes marks ASCII punctuation escaped by an odd-length
// backslash run. Only ASCII punctuation can be escaped in CommonMark.
func inlineLinkEscapedBytes(text string) []bool {
	escaped := make([]bool, len(text))
	for i := 0; i < len(text); {
		if text[i] != '\\' {
			i++
			continue
		}
		start := i
		for i < len(text) && text[i] == '\\' {
			i++
		}
		if i < len(text) && (i-start)%2 == 1 && isASCIIPunctuation(text[i]) {
			escaped[i] = true
		}
	}
	return escaped
}

// decodeCommonMarkEscapes removes a backslash only when it precedes ASCII
// punctuation. Control-byte placeholders pass through byte-for-byte.
func decodeCommonMarkEscapes(text string) string {
	if !strings.Contains(text, `\`) {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && isASCIIPunctuation(text[i+1]) {
			i++
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

// normaliseCommonMarkLinkTextEscapes prepares a label for recursive Markdown
// conversion. Syntax-significant punctuation must retain its source escape so
// that, for example, \*literal\* cannot turn into emphasis. Other escaped ASCII
// punctuation is decoded now because it needs no Telegram escape.
func normaliseCommonMarkLinkTextEscapes(text string) string {
	if !strings.Contains(text, `\`) {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' &&
			i+1 < len(text) &&
			isASCIIPunctuation(text[i+1]) &&
			text[i+1] != '\\' &&
			!isSpecialByte(text[i+1]) {
			i++
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

func isASCIIPunctuation(c byte) bool {
	return c >= '!' && c <= '/' ||
		c >= ':' && c <= '@' ||
		c >= '[' && c <= '`' ||
		c >= '{' && c <= '~'
}
