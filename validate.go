package tgmarkdown

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// validMarkdownV2 reports whether s would be accepted by Telegram's MarkdownV2
// parser. It is a faithful port of the accept/reject decision of tdlib's
// parse_markdown_v2 (the reference implementation Telegram uses): the same
// escape handling, the same greedy __ grouping, the same reserved-character
// rules, blockquote line boundaries, and link/pre/code termination. Entity
// output and the drop-empty logic are omitted because they never cause a
// rejection.
//
// It is the gate for Convert's safety net, so it must not reject the library's
// own well-formed output nor accept anything Telegram would 400 on.
func validMarkdownV2(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}

	type frame struct {
		typ        string
		byteOffset int
	}
	var stack []frame
	haveBlockquote := false
	canStartBlockquote := true
	resultSize := 0
	var lastResultByte byte
	n := len(s)

	for i := 0; i < n; {
		c := s[i]
		if c == '\\' && i+1 < n && s[i+1] > 0 && s[i+1] <= 126 {
			i++
			nx := s[i]
			resultSize++
			lastResultByte = nx
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
			lastResultByte = c
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
				case "customemoji":
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
			entityByteOffset := i
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
				lastResultByte = '\n'
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
			stack = append(stack, frame{typ, entityByteOffset})
			i++
			continue
		}

		// end of an entity
		top := stack[len(stack)-1]
		typ := top.typ
		if c == '\n' && typ != "blockquote" {
			spoilerOK := typ == "spoiler" && (top.byteOffset == i-2 ||
				(top.byteOffset == i-3 && resultSize != 0 && lastResultByte == '\r'))
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
			var url strings.Builder
			for i < n && s[i] != ')' {
				if s[i] == '\\' && i+1 < n && s[i+1] > 0 && s[i+1] <= 126 {
					url.WriteByte(s[i+1])
					i += 2
					continue
				}
				url.WriteByte(s[i])
				i++
			}
			if i >= n || s[i] != ')' {
				return false
			}
			if !validTelegramCustomEntityURL(url.String()) {
				return false
			}
		case "blockquote":
			haveBlockquote = false
			resultSize++
			lastResultByte = c
			canStartBlockquote = true
		}
		stack = stack[:len(stack)-1]
		i++
	}

	if haveBlockquote && len(stack) > 0 {
		if stack[len(stack)-1].typ == "spoiler" && stack[len(stack)-1].byteOffset == n-2 {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && stack[len(stack)-1].typ == "blockquote" {
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}

func validTelegramCustomEntityURL(rawURL string) bool {
	if query, ok := telegramEntityQuery(rawURL, "emoji"); ok {
		for parameter := range strings.SplitSeq(query, "&") {
			key, value, _ := strings.Cut(parameter, "=")
			if key == "id" {
				id, ok := parseTDLibInteger(value, 64)
				return ok && id != 0
			}
		}
		return false
	}

	query, ok := telegramEntityQuery(rawURL, "time")
	if !ok {
		return false
	}

	var unixTime int64
	format := ""
	for parameter := range strings.SplitSeq(query, "&") {
		key, value, _ := strings.Cut(parameter, "=")
		switch key {
		case "unix":
			parsed, parsedOK := parseTDLibInteger(value, 32)
			if !parsedOK || parsed <= 0 {
				return false
			}
			unixTime = parsed
		case "format":
			format = value
		}
	}
	return unixTime > 0 && validTelegramTimeFormat(format)
}

func telegramEntityQuery(rawURL, host string) (string, bool) {
	if len(rawURL) < len("tg:") || !strings.EqualFold(rawURL[:len("tg:")], "tg:") {
		return "", false
	}
	rest := strings.TrimPrefix(rawURL[len("tg:"):], "//")
	if len(rest) < len(host) || !strings.EqualFold(rest[:len(host)], host) {
		return "", false
	}
	query, ok := strings.CutPrefix(strings.TrimPrefix(rest[len(host):], "/"), "?")
	if !ok {
		return "", false
	}
	query, _, _ = strings.Cut(query, "#")
	return query, true
}

func parseTDLibInteger(value string, bitSize int) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, bitSize)
	if err != nil || strconv.FormatInt(parsed, 10) != value {
		return 0, false
	}
	return parsed, true
}

func validTelegramTimeFormat(format string) bool {
	if format == "r" || format == "R" {
		return true
	}
	for _, c := range format {
		if !strings.ContainsRune("tTdDwW", c) {
			return false
		}
	}
	return true
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
