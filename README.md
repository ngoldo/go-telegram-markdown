# go-telegram-markdown

A zero-dependency Go library for converting a focused Markdown dialect to
Telegram's MarkdownV2 format. It requires Go 1.27 or later.

## Features

- Converts bold, italic, underline, strikethrough, spoilers, inline links,
  inline code, fenced code blocks, and line blockquotes.
- Supports properly nested emphasis and formatting inside link labels.
- Keeps list markers, spaced operators, and intraword delimiter characters as
  literal text.
- Escapes Telegram MarkdownV2's reserved characters, including plain-text
  fallback for malformed or crossing input.
- Produces valid UTF-8, gates structured output through a TDLib-derived
  MarkdownV2 acceptance checker, and uses escaped text as its safety fallback.
- Has no third-party dependencies.

## Safety guarantee

`Convert` returns syntactically valid MarkdownV2. If its structured conversion
does not pass the local TDLib-derived checker, it returns the whole input as
escaped plain text instead. For example, crossing emphasis such as `_~_~`
loses formatting but remains sendable.

Both public functions normalise invalid UTF-8 byte runs to U+FFFD, the Unicode
replacement character. `Convert` also removes the C0 control bytes it reserves
for internal placeholders.

This guarantee covers UTF-8 and MarkdownV2 entity syntax. It does not bypass
Telegram limits or policy checks such as message length, an inaccessible link,
or permission to use a particular custom emoji.

## Installation

```bash
go get github.com/ngoldo/go-telegram-markdown
```

## Quick Start

```go
package main

import (
	"fmt"

	tgmarkdown "github.com/ngoldo/go-telegram-markdown"
)

func main() {
	// Basic usage
	text := "This is **bold** and *italic* text with a [link](https://example.com)"
	fmt.Println(tgmarkdown.Convert(text))
	// Output: This is *bold* and _italic_ text with a [link](https://example.com)

	// Special characters are escaped
	fmt.Println(tgmarkdown.Convert("Special chars: . ! - = + will be escaped"))
	// Output: Special chars: \. \! \- \= \+ will be escaped

	// Code blocks and inline code are preserved
	fmt.Println(tgmarkdown.Convert("Here's some `inline code`"))
	// Output: Here's some `inline code`
}
```

## Supported Markdown Elements

| Input | Telegram MarkdownV2 output | Notes |
| --- | --- | --- |
| `**bold**` | `*bold*` | Bold |
| `*italic*` or `_italic_` | `_italic_` | Italic |
| `***bold italic***` | `*_bold italic_*` | Combined bold and italic |
| `__underline__` | `__underline__` | Underline |
| `~~strike~~` or `~strike~` | `~strike~` | Strikethrough |
| `||spoiler||` | `||spoiler||` | Spoiler |
| `[label](https://example.com)` | `[label](https://example.com)` | Inline link |
| `` `code` `` | `` `code` `` | Inline code |
| <code>```go<br>code<br>```</code> | Telegram fenced code | Optional language |
| `> quote` | `>quote` | Line blockquote |

Delimiter recognition uses flanking and word-boundary rules rather than a full
CommonMark parser. Properly nested combinations are supported; malformed or
crossing combinations may trigger the plain-text fallback. Link destinations
may contain balanced parentheses and Markdown backslash escapes.

This library does not interpret headings, lists, tables, images, raw HTML,
reference links, or other CommonMark/GFM extensions as structures. Their
punctuation is escaped and sent as text.

## API Reference

### `Convert(text string) string`

Converts the supported input dialect to Telegram MarkdownV2. The result is
valid UTF-8 and safe to use with `parse_mode: "MarkdownV2"`, subject to
Telegram's non-syntax limits and permissions.

### `EscapeSpecialChars(text string) string`

Escapes all MarkdownV2 special characters (`_*[]()~`&#96;`>#+-=|{}.!`) in
plain text without double-escaping existing MarkdownV2 escape sequences. Use it
when no formatting conversion is wanted.

## Development

```bash
go test ./...
go test -fuzz=FuzzConvert -fuzztime=30s
```

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file in this repository.

## Acknowledgements

- Built for use with the Telegram Bot API
- Follows Telegram's [MarkdownV2 specification](https://core.telegram.org/bots/api#markdownv2-style)
- The local acceptance checker and its pinned corpus follow TDLib's
  [`parse_markdown_v2`](https://github.com/tdlib/td/blob/022d60202e446ad1287b9fb68e687c8a0760788b/td/telegram/MessageEntity.cpp)
