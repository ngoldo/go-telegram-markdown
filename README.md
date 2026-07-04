# go-telegram-markdown

A Go library for converting standard Markdown to Telegram's MarkdownV2 format, with proper escaping of special characters.

## Features

- ✅ Convert standard Markdown to Telegram MarkdownV2 format
- ✅ Proper escaping of special characters, without double-escaping
- ✅ Preserve code blocks and inline code (escaping only what the MarkdownV2 spec requires inside them)
- ✅ Handle nested and mixed emphasis to any depth — e.g. an italic span inside a bold one, `**bold *italic***` → `*bold _italic_*` (the delimiter passes repeat to a fixpoint, so an outer span resolves once its inner span has)
- ✅ Support for links, bold, italic, strikethrough, underline, and spoiler text
- ✅ Word-boundary and flanking rules: list markers (`* item`), spaced operators (`5 * 3`), and intraword delimiters (`snake_case`, `file~1`) stay literal
- ✅ Link URLs with balanced parentheses (Wikipedia-style) survive intact
- ✅ Recursive processing of markdown inside link text
- ✅ Always sendable: the result is guaranteed to be valid MarkdownV2 (see below)
- ✅ Zero dependencies

## Safety guarantee

`Convert` never returns output that Telegram would reject with a "can't parse entities" error. Well-formed Markdown converts to the equivalent MarkdownV2. Deeply malformed input that can't be expressed as valid MarkdownV2 (for example crossing emphasis like `_~_~`) falls back to the text escaped as plain — still delivered, just without formatting. This is verified by fuzzing: every output parses as balanced MarkdownV2.

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

| Standard Markdown                     | Telegram MarkdownV2        | Description          |
| ------------------------------------- | -------------------------- | -------------------- |
| `**bold**`                            | `*bold*`                   | Bold text            |
| `***bold italic***`                   | `*_bold italic_*`          | Bold and italic text |
| `**bold *italic***`                   | `*bold _italic_*`          | Nested emphasis, any depth |
| `*italic*` / `_italic_`               | `_italic_`                 | Italic text          |
| `~~strikethrough~~` / `~strikethrough~` | `~strikethrough~`        | Strikethrough text   |
| `__underline__`                       | `__underline__`            | Underlined text      |
| `\|\|spoiler\|\|`                     | `\|\|spoiler\|\|`          | Spoiler text         |
| `> blockquote`                        | `>blockquote`              | Blockquotes          |
| `[link](url)`                         | `[link](url)`              | Hyperlinks           |
| `` `inline code` ``                   | `` `inline code` ``        | Inline code          |
| ```` ```code block``` ````            | ```` ```code block``` ```` | Code blocks          |

## API Reference

### `Convert(text string) string`

Converts standard Markdown text to Telegram MarkdownV2 format. The result is safe to send with `parse_mode: "MarkdownV2"`.

### `EscapeSpecialChars(text string) string`

Escapes all MarkdownV2 special characters (`_*[]()~`&#96;`>#+-=|{}.!`) in the text without double-escaping already-escaped sequences. `Convert` calls this internally; use it directly when you need plain text escaped without any markdown conversion.

## Development

```bash
go test ./...            # run the test suite
go test -fuzz=FuzzConvert -fuzztime=30s   # fuzz for robustness
```

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file in this repository.

## Acknowledgments

- Built for use with the Telegram Bot API
- Follows Telegram's [MarkdownV2 specification](https://core.telegram.org/bots/api#markdownv2-style)
