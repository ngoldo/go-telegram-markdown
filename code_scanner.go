package tgmarkdown

import (
	"strconv"
	"strings"
)

// These bytes are reserved by sanitizeControl, so temporary fenced-code
// placeholders cannot collide with user input. Fenced blocks are protected
// during the inline-code pass: an inline span must never swallow a block that
// has already been isolated.
const (
	scannerFenceOpen  = "\x07"
	scannerFenceClose = "\x08"
)

// isolateCode replaces fenced blocks and inline code spans with values
// returned by store. The callback receives code already normalised for
// Telegram MarkdownV2: fenced blocks use triple-backtick delimiters, inline
// spans use single-backtick delimiters, and their content is escaped with
// escapeCode.
//
// The caller must invoke sanitizeControl first, and store must return an
// opaque, backtick-free placeholder. Fenced blocks are collected before
// inline spans so an inline delimiter pair cannot consume a fenced block.
func isolateCode(text string, store func(rendered string) string) string {
	if text == "" {
		return text
	}

	var fencedPlaceholders []string
	text = isolateFencedCode(text, func(rendered string) string {
		fencedPlaceholders = append(fencedPlaceholders, store(rendered))
		return scannerFencePlaceholder(len(fencedPlaceholders) - 1)
	})
	text = isolateInlineCode(text, store)
	if len(fencedPlaceholders) == 0 {
		return text
	}
	return restoreScannerFencePlaceholders(text, fencedPlaceholders)
}

type fencedCodeMatch struct {
	end      int
	rendered string
}

// isolateFencedCode recognises CommonMark-style backtick fences at the start
// of a line (after up to three spaces or a blockquote prefix). A closing fence
// must be on its own line and contain at least as many backticks as the
// opening fence. For backwards compatibility, an unescaped pair of exactly
// three backtick runs on one line is also treated as a fenced block.
func isolateFencedCode(text string, store func(rendered string) string) string {
	var b strings.Builder
	last := 0
	lineStart := 0
	changed := false

	for i := 0; i < len(text); {
		if text[i] == '\n' {
			i++
			lineStart = i
			continue
		}
		if text[i] != '`' {
			i++
			continue
		}

		runEnd := backtickRunEnd(text, i)
		runLen := runEnd - i
		if oddBackslashesBefore(text, i) {
			i = runEnd
			continue
		}

		var match fencedCodeMatch
		var ok bool
		if runLen >= 3 {
			match, ok = matchMultilineFence(text, lineStart, i, runEnd)
			if !ok && runLen == 3 {
				match, ok = matchLegacySingleLineFence(text, i, runEnd)
			}
		}
		if !ok {
			i = runEnd
			continue
		}

		b.WriteString(text[last:i])
		b.WriteString(store(match.rendered))
		last = match.end
		i = match.end
		changed = true
	}

	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

// matchMultilineFence matches a line-aware fenced block beginning at
// tickStart. The structural '>' prefix is retained on the opening line for the
// later blockquote pass, but removed from every code-content and closing line.
func matchMultilineFence(text string, lineStart, tickStart, runEnd int) (fencedCodeMatch, bool) {
	quoted, ok := fenceOpeningPrefix(text, lineStart, tickStart)
	if !ok {
		return fencedCodeMatch{}, false
	}

	hasOpeningNewline := true
	openLineEndRel := strings.IndexByte(text[runEnd:], '\n')
	if openLineEndRel < 0 {
		hasOpeningNewline = false
		openLineEndRel = len(text) - runEnd
	}
	openLineEnd := runEnd + openLineEndRel
	infoEnd := openLineEnd
	if infoEnd > runEnd && text[infoEnd-1] == '\r' {
		infoEnd--
	}
	info := text[runEnd:infoEnd]
	// Backticks are forbidden in the info string. Without this guard a later
	// run on the opening line could be mistaken for code content.
	if strings.ContainsRune(info, '`') {
		return fencedCodeMatch{}, false
	}
	info = fencedCodeLanguage(info)
	if !hasOpeningNewline {
		return fencedCodeMatch{
			end:      len(text),
			rendered: renderFencedCode(info, ""),
		}, true
	}

	openRunLen := runEnd - tickStart
	var content strings.Builder
	cursor := openLineEnd + 1
	for cursor < len(text) {
		lineEnd := len(text)
		nextLine := len(text)
		if rel := strings.IndexByte(text[cursor:], '\n'); rel >= 0 {
			lineEnd = cursor + rel
			nextLine = lineEnd + 1
		}
		syntaxEnd := lineEnd
		if syntaxEnd > cursor && text[syntaxEnd-1] == '\r' {
			syntaxEnd--
		}

		bodyStart := cursor
		hadQuote := false
		if quoted {
			bodyStart, hadQuote = stripStructuralQuotePrefix(text, cursor, syntaxEnd)
		}
		if quoted && !hadQuote {
			// A fenced block inside a blockquote ends when the quote does. A
			// missing source closer is harmless because the rendered Telegram
			// entity always receives an explicit closing fence.
			end := cursor
			if end > 0 && text[end-1] == '\n' {
				end--
				if end > 0 && text[end-1] == '\r' {
					end--
				}
			}
			return fencedCodeMatch{
				end:      end,
				rendered: renderFencedCode(info, content.String()),
			}, true
		}
		if !quoted || hadQuote {
			if closeEnd, isClose := closingFenceEnd(text, bodyStart, syntaxEnd, openRunLen); isClose {
				return fencedCodeMatch{
					end:      closeEnd,
					rendered: renderFencedCode(info, content.String()),
				}, true
			}
		}

		if quoted && hadQuote {
			content.WriteString(text[bodyStart:nextLine])
		} else {
			content.WriteString(text[cursor:nextLine])
		}
		if nextLine == len(text) {
			break
		}
		cursor = nextLine
	}

	// CommonMark permits a fenced block to continue to the end of its
	// container without a source closing fence. Telegram does not, so close
	// the normalised entity explicitly.
	return fencedCodeMatch{
		end:      len(text),
		rendered: renderFencedCode(info, content.String()),
	}, true
}

func renderFencedCode(info, content string) string {
	escaped := escapeCode(content)
	if escaped != "" && !strings.HasSuffix(escaped, "\n") {
		escaped += "\n"
	}
	return "```" + info + "\n" + escaped + "```"
}

// fencedCodeLanguage maps CommonMark's optional info string to Telegram's
// single language token. Emitting trailing attributes after the token would
// make TDLib treat them as the first bytes of the code content.
func fencedCodeLanguage(info string) string {
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// fenceOpeningPrefix reports whether a backtick run is in a valid opening
// position. A blockquote marker may be preceded by up to three spaces and
// followed by one optional horizontal whitespace byte plus up to three spaces.
func fenceOpeningPrefix(text string, lineStart, tickStart int) (quoted bool, ok bool) {
	i := lineStart
	for spaces := 0; i < tickStart && spaces < 3 && text[i] == ' '; spaces++ {
		i++
	}
	if i == tickStart {
		return false, true
	}
	if text[i] != '>' {
		return false, false
	}
	i++
	if i < tickStart && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	for spaces := 0; i < tickStart && spaces < 3 && text[i] == ' '; spaces++ {
		i++
	}
	return true, i == tickStart
}

// stripStructuralQuotePrefix removes one blockquote marker and its optional
// following horizontal whitespace byte from a source line.
func stripStructuralQuotePrefix(text string, lineStart, lineEnd int) (int, bool) {
	i := lineStart
	for i < lineEnd && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if i == lineEnd || text[i] != '>' {
		return lineStart, false
	}
	i++
	if i < lineEnd && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i, true
}

// closingFenceEnd returns the end of a closing run, excluding any trailing
// whitespace. Retaining that whitespace keeps the scanner lossless outside
// the code entity.
func closingFenceEnd(text string, lineStart, lineEnd, minimumRun int) (int, bool) {
	i := lineStart
	for spaces := 0; i < lineEnd && spaces < 3 && text[i] == ' '; spaces++ {
		i++
	}
	if i == lineEnd || text[i] != '`' {
		return 0, false
	}
	runEnd := backtickRunEnd(text, i)
	if runEnd-i < minimumRun {
		return 0, false
	}
	for j := runEnd; j < lineEnd; j++ {
		if text[j] != ' ' && text[j] != '\t' {
			return 0, false
		}
	}
	return runEnd, true
}

func matchLegacySingleLineFence(text string, tickStart, runEnd int) (fencedCodeMatch, bool) {
	lineEnd := len(text)
	if rel := strings.IndexByte(text[runEnd:], '\n'); rel >= 0 {
		lineEnd = runEnd + rel
	}
	for i := runEnd; i < lineEnd; {
		if text[i] != '`' {
			i++
			continue
		}
		candidateEnd := backtickRunEnd(text, i)
		if candidateEnd-i == 3 {
			return fencedCodeMatch{
				end:      candidateEnd,
				rendered: "```\n" + escapeCode(text[runEnd:i]) + "\n```",
			}, true
		}
		i = candidateEnd
	}
	return fencedCodeMatch{}, false
}

// isolateInlineCode pairs unescaped opening backtick runs with the next run of
// exactly the same length. Runs of other lengths remain content, so a
// two-backtick span can safely contain a single backtick.
func isolateInlineCode(text string, store func(rendered string) string) string {
	type run struct {
		start int
		end   int
	}

	var runs []run
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		end := backtickRunEnd(text, i)
		runs = append(runs, run{start: i, end: end})
		i = end
	}
	if len(runs) == 0 {
		return text
	}

	// The next run of a given length is the only possible closer. Precomputing
	// it prevents a long series of unmatched, differently sized runs from
	// rescanning the remainder of the input for every candidate opener.
	nextSameLength := make([]int, len(runs))
	nextByLength := make(map[int]int)
	for i := len(runs) - 1; i >= 0; i-- {
		length := runs[i].end - runs[i].start
		next, ok := nextByLength[length]
		if !ok {
			next = -1
		}
		nextSameLength[i] = next
		nextByLength[length] = i
	}

	var b strings.Builder
	last := 0
	changed := false

	for runIndex := 0; runIndex < len(runs); {
		opener := runs[runIndex]
		if oddBackslashesBefore(text, opener.start) {
			runIndex++
			continue
		}

		closeIndex := nextSameLength[runIndex]
		if closeIndex < 0 {
			runIndex++
			continue
		}
		closer := runs[closeIndex]
		content := text[opener.end:closer.start]
		if strings.ContainsAny(content, scannerFenceOpen+scannerFenceClose) {
			// A fenced block has precedence over an enclosing inline-looking
			// pair. Leave the opener literal and continue after it.
			runIndex++
			continue
		}
		content = trimCodeSpanPadding(content)

		b.WriteString(text[last:opener.start])
		b.WriteString(store("`" + escapeCode(content) + "`"))
		last = closer.end
		runIndex = closeIndex + 1
		changed = true
	}

	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func backtickRunEnd(text string, start int) int {
	end := start
	for end < len(text) && text[end] == '`' {
		end++
	}
	return end
}

func trimCodeSpanPadding(content string) string {
	if len(content) >= 2 &&
		content[0] == ' ' &&
		content[len(content)-1] == ' ' &&
		strings.Trim(content, " ") != "" {
		return content[1 : len(content)-1]
	}
	return content
}

func scannerFencePlaceholder(i int) string {
	return scannerFenceOpen + strconv.Itoa(i) + scannerFenceClose
}

// restoreScannerFencePlaceholders expands only scanner-generated tokens in a
// single pass. The callback's placeholders are inserted after inline scanning,
// so they cannot accidentally become inline-code content.
func restoreScannerFencePlaceholders(text string, replacements []string) string {
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		if text[i] != scannerFenceOpen[0] {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i + 1
		index := 0
		haveDigit := false
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			haveDigit = true
			index = index*10 + int(text[j]-'0')
			j++
		}
		if !haveDigit || j >= len(text) || text[j] != scannerFenceClose[0] || index >= len(replacements) {
			b.WriteByte(text[i])
			i++
			continue
		}
		b.WriteString(replacements[index])
		i = j + 1
	}
	return b.String()
}
