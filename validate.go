package tgmarkdown

import "strings"

// validMarkdownV2 reports whether s would be accepted by Telegram's MarkdownV2
// parser. It is a faithful port of the accept/reject decision of tdlib's
// parse_markdown_v2 (the reference implementation Telegram uses): the same
// escape handling, the same greedy __ grouping, the same reserved-character
// rules, blockquotes that end at a newline and force-close open inline
// entities, and link/pre/code termination. Entity output and the drop-empty
// logic are omitted because they never cause a rejection.
//
// It is the gate for Convert's safety net, so it must not reject the library's
// own well-formed output nor accept anything Telegram would 400 on.
func validMarkdownV2(s string) bool {
	type frame struct {
		typ        string
		byteOffset int
	}
	var stack []frame
	haveBlockquote := false
	canStartBlockquote := true
	resultSize := 0
	n := len(s)

	for i := 0; i < n; {
		c := s[i]
		if c == '\\' && i+1 < n && s[i+1] > 0 && s[i+1] <= 126 {
			i++
			nx := s[i]
			resultSize++
			if nx != '\r' {
				canStartBlockquote = nx == '\n'
			}
			i++
			continue
		}

		reserved := "_*[]()~`>#+-=|{}.!\n"
		if len(stack) > 0 {
			switch stack[len(stack)-1].typ {
			case "code", "pre", "precode":
				reserved = "`"
			}
		}
		if strings.IndexByte(reserved, c) < 0 {
			if c&0xC0 != 0x80 { // first byte of a UTF-8 code point
				if c != '\r' {
					canStartBlockquote = false
				}
			}
			resultSize++
			i++
			continue
		}

		isEnd := false
		if len(stack) > 0 {
			if haveBlockquote && c == '\n' && (i+1 == n || s[i+1] != '>') {
				isEnd = true
			} else {
				switch stack[len(stack)-1].typ {
				case "bold":
					isEnd = c == '*'
				case "italic":
					isEnd = c == '_' && (i+1 >= n || s[i+1] != '_')
				case "code":
					isEnd = c == '`'
				case "pre", "precode":
					isEnd = c == '`' && i+2 < n && s[i+1] == '`' && s[i+2] == '`'
				case "texturl":
					isEnd = c == ']'
				case "underline":
					isEnd = c == '_' && i+1 < n && s[i+1] == '_'
				case "strike":
					isEnd = c == '~'
				case "spoiler":
					isEnd = c == '|' && i+1 < n && s[i+1] == '|'
				case "blockquote":
					isEnd = false
				}
			}
		}

		if !isEnd {
			var typ string
			switch c {
			case '_':
				if i+1 < n && s[i+1] == '_' {
					typ = "underline"
					i++
				} else {
					typ = "italic"
				}
			case '*':
				typ = "bold"
			case '~':
				typ = "strike"
			case '|':
				if i+1 < n && s[i+1] == '|' {
					i++
					typ = "spoiler"
				} else {
					return false
				}
			case '[':
				typ = "texturl"
			case '`':
				if i+2 < n && s[i+1] == '`' && s[i+2] == '`' {
					i += 3
					typ = "pre"
					le := i
					for le < n && !isASCIISpace(s[le]) && s[le] != '`' {
						le++
					}
					if i != le && le < n && s[le] != '`' {
						typ = "precode"
						i = le
					}
					if i < n && (s[i] == '\n' || s[i] == '\r') {
						if i+1 < n && (s[i+1] == '\n' || s[i+1] == '\r') && s[i] != s[i+1] {
							i += 2
						} else {
							i++
						}
					}
					i--
				} else {
					typ = "code"
				}
			case '!':
				if i+1 < n && s[i+1] == '[' {
					i++
					typ = "customemoji"
				} else {
					return false
				}
			case '\n':
				resultSize++
				canStartBlockquote = true
				i++
				continue
			case '>':
				if canStartBlockquote {
					if haveBlockquote {
						i++
						continue
					}
					typ = "blockquote"
					haveBlockquote = true
				} else {
					return false
				}
			default:
				return false
			}
			stack = append(stack, frame{typ, i})
			i++
			continue
		}

		// end of an entity
		top := stack[len(stack)-1]
		typ := top.typ
		if c == '\n' && typ != "blockquote" {
			spoilerOK := typ == "spoiler" && (top.byteOffset == i-2 ||
				(top.byteOffset == i-3 && resultSize != 0 && s[resultSize-1] == '\r'))
			if !spoilerOK {
				return false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 || stack[len(stack)-1].typ != "blockquote" {
				return false
			}
			typ = "blockquote"
		}
		switch typ {
		case "underline", "spoiler":
			i++
		case "pre", "precode":
			i += 2
		case "texturl":
			if i+1 < n && s[i+1] == '(' {
				i += 2
				for i < n && s[i] != ')' {
					if s[i] == '\\' && i+1 < n && s[i+1] > 0 && s[i+1] <= 126 {
						i += 2
						continue
					}
					i++
				}
				if i >= n || s[i] != ')' {
					return false
				}
			}
		case "customemoji":
			if i+1 >= n || s[i+1] != '(' {
				return false
			}
			i += 2
			for i < n && s[i] != ')' {
				if s[i] == '\\' && i+1 < n && s[i+1] > 0 && s[i+1] <= 126 {
					i += 2
					continue
				}
				i++
			}
			if i >= n || s[i] != ')' {
				return false
			}
		case "blockquote":
			haveBlockquote = false
			resultSize++
			canStartBlockquote = true
		}
		stack = stack[:len(stack)-1]
		i++
	}

	if haveBlockquote && len(stack) > 0 && stack[len(stack)-1].typ == "blockquote" {
		stack = stack[:len(stack)-1]
	}
	return len(stack) == 0
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
