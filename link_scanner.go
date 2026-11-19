package tgmarkdown

import "strings"

// isolateInlineLinks replaces each inline Markdown link with the value returned
// by store. It recognises nested brackets in link text, balanced parentheses in
// destinations, and angle-bracket destinations. URL backslash escapes are
// decoded before the values are handed to store. The label is handed to the
// recursive inline converter with its original escapes.
//
// The structural indexes are built in one pass. In particular, a malformed
// destination is never rescanned for each preceding '[', keeping the scanner
// linear for adversarial input as well as ordinary prose.
func isolateInlineLinks(text string, store func(linkText, url string) string) string {
	if text == "" || !strings.Contains(text, "](") {
		return text
	}

	codeEnds := codeSpanEnds(text)
	index := newInlineLinkIndex(text)
	imageEnds := findImageEnds(text, codeEnds, index)
	openBrackets := make([]int, 0, 8)

	var out strings.Builder
	last := 0
	changed := false

	for i := 0; i < len(text); i++ {
		if end := codeEnds[i]; end > i {
			i = end - 1
			continue
		}
		if end := imageEnds[i]; end > i {
			// A complete unsupported image is opaque whilst ordinary links
			// are discovered. If it is inside a real link label, leaving the
			// outer bracket stack untouched lets that supported link close.
			i = end - 1
			continue
		}
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
			if i+1 >= len(text) ||
				text[i+1] != '(' ||
				index.escaped[i+1] {
				continue
			}

			urlStart, urlEnd, linkEnd, ok := index.destination(text, i+1)
			if !ok {
				continue
			}
			if i == openBracket+1 || openBracket < last {
				continue
			}

			out.WriteString(text[last:openBracket])
			linkText := text[openBracket+1 : i]
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

// findImageEnds returns each confirmed image's end keyed by the byte offset of
// its '!'. Code constructs are skipped only when their opener is actually
// visited, and every confirmed link or image destination is consumed
// atomically. A backtick inside a destination therefore cannot hide later
// brackets, while brackets inside real code remain opaque.
func findImageEnds(text string, codeEnds []int, index inlineLinkIndex) []int {
	ends := make([]int, len(text))
	if text == "" || !strings.Contains(text, "![") || !strings.Contains(text, "](") {
		return ends
	}

	openBrackets := make([]int, 0, 8)
	for i := 0; i < len(text); i++ {
		if end := codeEnds[i]; end > i {
			i = end - 1
			continue
		}
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
			if i+1 >= len(text) ||
				text[i+1] != '(' ||
				index.escaped[i+1] {
				continue
			}

			_, _, constructEnd, ok := index.destination(text, i+1)
			if !ok {
				continue
			}
			isImage := openBracket > 0 &&
				text[openBracket-1] == '!' &&
				!index.escaped[openBracket-1]
			if isImage {
				ends[openBracket-1] = constructEnd
			}
			if isImage || i != openBracket+1 {
				i = constructEnd - 1
			}
		}
	}
	return ends
}

// isolateImages protects complete images after ordinary links have been
// isolated. Images are outside the supported dialect, so their exact source is
// restored later as escaped plain text.
func isolateImages(text string, store func(source string) string) string {
	if text == "" || !strings.Contains(text, "![") || !strings.Contains(text, "](") {
		return text
	}
	index := newInlineLinkIndex(text)
	imageEnds := findImageEnds(text, codeSpanEnds(text), index)

	var out strings.Builder
	last := 0
	changed := false
	for i := 0; i < len(text); i++ {
		imageEnd := imageEnds[i]
		if imageEnd <= i {
			continue
		}
		out.WriteString(text[last:i])
		out.WriteString(store(text[i:imageEnd]))
		last = imageEnd
		changed = true
		i = imageEnd - 1
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
