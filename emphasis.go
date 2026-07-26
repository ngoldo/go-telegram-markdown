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
	start     int
	length    int
	leftUsed  int
	rightUsed int
	canOpen   bool
	canClose  bool
	repls     []asteriskReplacement
}

type asteriskMatch struct {
	openerIndex int
	closerIndex int
	openOffset  int
	closeOffset int
	width       int
	marker      string
	emit        bool
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
func replaceAsteriskEmphasis(text string) string {
	if !strings.Contains(text, "*") {
		return text
	}

	var runs []asteriskRun
	for i := 0; i < len(text); {
		if text[i] != '*' {
			i++
			continue
		}
		start := i
		for i < len(text) && text[i] == '*' {
			i++
		}

		var before, after rune
		haveBefore := start > 0
		haveAfter := i < len(text)
		if haveBefore {
			before, _ = utf8.DecodeLastRuneInString(text[:start])
		}
		if haveAfter {
			after, _ = utf8.DecodeRuneInString(text[i:])
		}

		escaped := oddBackslashesBefore(text, start)
		canOpen := !escaped && haveAfter && !unicode.IsSpace(after) &&
			(!haveBefore || !isWordRune(before))
		canClose := !escaped && haveBefore && !unicode.IsSpace(before) &&
			(!haveAfter || !isWordRune(after))

		runs = append(runs, asteriskRun{
			start:    start,
			length:   i - start,
			canOpen:  canOpen,
			canClose: canClose,
		})
	}

	matched := false
	var matches []asteriskMatch
	var openers [6][]int
	for closerIndex := range runs {
		closer := &runs[closerIndex]
		if closer.canClose {
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
				matches = append(matches, asteriskMatch{
					openerIndex: openerIndex,
					closerIndex: closerIndex,
					openOffset:  openOffset,
					closeOffset: closeOffset,
					width:       width,
					marker:      marker,
					emit:        true,
				})
				matched = true

				if opener.remaining() == 0 {
					stack := openers[openerGroup]
					openers[openerGroup] = stack[:len(stack)-1]
				}
			}
		}

		if closer.canOpen && closer.remaining() > 0 {
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

	// Telegram cannot represent an entity nested inside another entity of the
	// same type. Such nesting is idempotent, so flatten it to the outer span
	// whilst still consuming every source delimiter. Without this step,
	// ****text**** would become **text**, which TDLib interprets as empty bold
	// entities rather than bold text.
	suppressNestedAsteriskMatches(matches, runs)
	for _, match := range matches {
		marker := ""
		if match.emit {
			marker = match.marker
		}
		opener := &runs[match.openerIndex]
		opener.repls = append(opener.repls, asteriskReplacement{
			offset: match.openOffset,
			width:  match.width,
			marker: marker,
		})
		closer := &runs[match.closerIndex]
		closer.repls = append(closer.repls, asteriskReplacement{
			offset: match.closeOffset,
			width:  match.width,
			marker: marker,
		})
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

func suppressNestedAsteriskMatches(matches []asteriskMatch, runs []asteriskRun) {
	for _, marker := range []string{phBold, phItalic} {
		var ordered []int
		for i := range matches {
			if matches[i].marker == marker {
				ordered = append(ordered, i)
			}
		}
		sort.Slice(ordered, func(i, j int) bool {
			left := matches[ordered[i]]
			right := matches[ordered[j]]
			leftStart := runs[left.openerIndex].start + left.openOffset + left.width
			rightStart := runs[right.openerIndex].start + right.openOffset + right.width
			if leftStart != rightStart {
				return leftStart < rightStart
			}
			leftEnd := runs[left.closerIndex].start + left.closeOffset
			rightEnd := runs[right.closerIndex].start + right.closeOffset
			return leftEnd > rightEnd
		})

		outerEnd := -1
		for _, matchIndex := range ordered {
			match := &matches[matchIndex]
			start := runs[match.openerIndex].start + match.openOffset + match.width
			end := runs[match.closerIndex].start + match.closeOffset
			if start < outerEnd && end <= outerEnd {
				match.emit = false
				continue
			}
			outerEnd = end
		}
	}
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
