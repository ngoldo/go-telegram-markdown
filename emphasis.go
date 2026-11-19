package tgmarkdown

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type asteriskReplacement struct {
	offset int
	width  int
	marker string
}

type asteriskRun struct {
	start      int
	length     int
	scope      int
	next       int
	leftUsed   int
	rightUsed  int
	canOpen    bool
	canClose   bool
	preferOpen bool
	repls      []asteriskReplacement
}

func (r *asteriskRun) remaining() int {
	return r.length - r.leftUsed - r.rightUsed
}

// replaceAsteriskEmphasis resolves the one- and two-character asterisk
// delimiters in a single delimiter-stack pass. Unlike replaceDelimited, a run
// can close an inner span and then close an outer one (or vice versa), so both
// flush-opening and flush-closing mixed emphasis are handled symmetrically.
//
// The library deliberately keeps its conservative word-boundary behaviour:
// delimiters embedded directly in a word stay literal. Whitespace flanking and
// the CommonMark multiple-of-three rule prevent list markers and ambiguous
// delimiter runs from becoming emphasis.
func replaceAsteriskEmphasis(text string, respectScope bool) string {
	if !strings.Contains(text, "*") {
		return text
	}

	var runs []asteriskRun
	var scope emphasisScope
	for i := 0; i < len(text); {
		if width := scope.consume(text[i], i); width != 0 {
			i += width
			continue
		}
		if text[i] != '*' {
			i++
			continue
		}
		start := i
		for i < len(text) && text[i] == '*' {
			i++
		}
		end := i

		// The final asterisk in "*\uFE0F\u20E3" (or "*\u20E3") is the
		// base of a keycap emoji, not an emphasis delimiter. Exclude just
		// that byte so any preceding asterisks in the same run can still
		// participate in matching.
		if asteriskStartsKeycap(text, end-1) {
			end--
		}

		// A backslash escapes only the first asterisk in a run. Keep that
		// byte as literal input, then let any remaining asterisks participate
		// in delimiter matching.
		if oddBackslashesBefore(text, start) {
			start++
		}
		if start >= end {
			continue
		}

		var before, after rune
		haveBefore := start > 0
		haveAfter := end < len(text)
		if haveBefore {
			before, _ = utf8.DecodeLastRuneInString(text[:start])
		}
		if haveAfter {
			after, _ = utf8.DecodeRuneInString(text[end:])
		}

		// A run directly after an opening bracket and before an
		// underscore span can introduce that span, not end a previous one.
		beforeUnderscoreOpen := haveAfter && isUnderscoreMarker(text[end])
		canOpen := haveAfter && !unicode.IsSpace(after) &&
			(!haveBefore || !isWordBefore(text, start))
		canClose := haveBefore && !unicode.IsSpace(before) &&
			(!haveAfter || !isWordRune(after))

		enclosing := 0
		if respectScope {
			enclosing = scope.current()
		}
		runs = append(runs, asteriskRun{
			start:      start,
			length:     end - start,
			scope:      enclosing,
			next:       -1,
			canOpen:    canOpen,
			canClose:   canClose,
			preferOpen: respectScope && beforeUnderscoreOpen && afterOpeningPunctuation(text, start),
		})
	}

	if respectScope {
		nextByScope := make(map[int]int)
		for i := len(runs) - 1; i >= 0; i-- {
			run := &runs[i]
			if next, ok := nextByScope[run.scope]; ok {
				run.next = next
			}
			nextByScope[run.scope] = i
		}
	}

	matched := false
	byScope := make(map[int]*[6][]int)
	for closerIndex := range runs {
		closer := &runs[closerIndex]
		openers := byScope[closer.scope]
		if closer.canClose && openers != nil && !preferNestedAsteriskOpener(text, runs, closer) {
			for closer.remaining() > 0 {
				openerIndex := -1
				openerGroup := -1
				for group := range openers {
					stack := openers[group]
					for len(stack) > 0 && runs[stack[len(stack)-1]].remaining() == 0 {
						stack = stack[:len(stack)-1]
					}
					openers[group] = stack
					if len(stack) == 0 {
						continue
					}
					candidate := stack[len(stack)-1]
					if candidate <= openerIndex ||
						violatesAsteriskRuleOfThree(&runs[candidate], closer) {
						continue
					}
					openerIndex = candidate
					openerGroup = group
				}
				if openerIndex < 0 {
					break
				}

				opener := &runs[openerIndex]
				width := 1
				marker := phItalic
				if opener.remaining() >= 2 && closer.remaining() >= 2 {
					width = 2
					marker = phBold
					// A symmetric triple run represents bold containing
					// italic. Consume the inner italic delimiter first so the
					// remaining two bytes become the outer bold delimiter.
					if opener.remaining() == 3 && closer.remaining() == 3 {
						width = 1
						marker = phItalic
					}
				}

				openOffset := opener.length - opener.rightUsed - width
				opener.rightUsed += width

				closeOffset := closer.leftUsed
				closer.leftUsed += width
				opener.repls = append(opener.repls, asteriskReplacement{
					offset: openOffset,
					width:  width,
					marker: marker,
				})
				closer.repls = append(closer.repls, asteriskReplacement{
					offset: closeOffset,
					width:  width,
					marker: phClose + marker,
				})
				matched = true

				if opener.remaining() == 0 {
					stack := openers[openerGroup]
					openers[openerGroup] = stack[:len(stack)-1]
				}
			}
		}

		if closer.canOpen && closer.remaining() > 0 {
			if openers == nil {
				openers = new([6][]int)
				byScope[closer.scope] = openers
			}
			group := closer.length % 3 * 2
			if closer.canClose {
				group++
			}
			openers[group] = append(openers[group], closerIndex)
		}
	}

	if !matched {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for i := range runs {
		run := &runs[i]
		if len(run.repls) == 0 {
			continue
		}
		b.WriteString(text[last:run.start])
		sort.Slice(run.repls, func(i, j int) bool {
			return run.repls[i].offset < run.repls[j].offset
		})

		runPos := 0
		for _, repl := range run.repls {
			if repl.offset > runPos {
				b.WriteString(text[run.start+runPos : run.start+repl.offset])
			}
			b.WriteString(repl.marker)
			runPos = repl.offset + repl.width
		}
		if runPos < run.length {
			b.WriteString(text[run.start+runPos : run.start+run.length])
		}
		last = run.start + run.length
	}
	b.WriteString(text[last:])
	return b.String()
}

// Defer a valid closer only if the next runs can close the proposed inner
// span and still leave a closer for its parent. An independent opener
// following opening punctuation cannot supply either closing delimiter.
func preferNestedAsteriskOpener(text string, runs []asteriskRun, run *asteriskRun) bool {
	if !run.preferOpen || !run.canOpen || run.next < 0 {
		return false
	}
	innerClose := &runs[run.next]
	if innerClose.length < run.length || !canEndAsteriskSpan(text, innerClose) ||
		violatesAsteriskRuleOfThree(run, innerClose) {
		return false
	}
	if innerClose.length-run.length >= run.length {
		return true // one run can close both spans
	}
	return innerClose.next >= 0 && canEndAsteriskSpan(text, &runs[innerClose.next])
}

func canEndAsteriskSpan(text string, run *asteriskRun) bool {
	return run.canClose && (!run.canOpen || !afterOpeningPunctuation(text, run.start))
}

func isUnderscoreMarker(marker byte) bool {
	return marker == phItalic[0] || marker == phUnderline[0]
}

func asteriskStartsKeycap(text string, pos int) bool {
	if pos < 0 || pos >= len(text) || text[pos] != '*' {
		return false
	}
	pos++
	if pos >= len(text) {
		return false
	}
	r, size := utf8.DecodeRuneInString(text[pos:])
	if r == '\uFE0F' {
		pos += size
		if pos >= len(text) {
			return false
		}
		r, _ = utf8.DecodeRuneInString(text[pos:])
	}
	return r == '\u20E3'
}

func violatesAsteriskRuleOfThree(opener, closer *asteriskRun) bool {
	if !opener.canClose && !closer.canOpen {
		return false
	}
	if (opener.length+closer.length)%3 != 0 {
		return false
	}
	return opener.length%3 != 0 || closer.length%3 != 0
}
