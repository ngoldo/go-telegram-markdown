package tgmarkdown

import (
	"strconv"
	"strings"
)

// expandIndexedPlaceholders restores all placeholders of one kind in a single
// pass. Replacement values are not scanned recursively; callers deliberately
// restore links first and code second because rendered link text and URLs may
// contain code placeholders.
func expandIndexedPlaceholders(text string, open, close byte, values []string) string {
	if len(values) == 0 || strings.IndexByte(text, open) < 0 {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))
	last := 0
	replaced := false

	for i := 0; i < len(text); {
		if text[i] != open {
			i++
			continue
		}
		endRel := strings.IndexByte(text[i+1:], close)
		if endRel < 0 {
			break
		}
		end := i + 1 + endRel
		if end == i+1 {
			i++
			continue
		}
		index, err := strconv.Atoi(text[i+1 : end])
		if err != nil || index < 0 || index >= len(values) {
			i++
			continue
		}

		b.WriteString(text[last:i])
		b.WriteString(values[index])
		i = end + 1
		last = i
		replaced = true
	}

	if !replaced {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}
