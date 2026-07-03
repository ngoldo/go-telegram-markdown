package tgmarkdown

import (
	"testing"
	"unicode/utf8"
)

func TestConvert(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no markdown", "Hello world.", `Hello world\.`},

		{"simple bold", "**bold text**", "*bold text*"},

		{"simple italic underscore", "_italic text_", "_italic text_"},
		{"simple italic asterisk", "*italic text*", "_italic text_"},

		{"simple strikethrough", "~strikethrough text~", "~strikethrough text~"},

		{"double tilde strikethrough", "~~strikethrough text~~", "~strikethrough text~"},
		{"double tilde in middle", "Text with ~~strikethrough~~ in middle", "Text with ~strikethrough~ in middle"},
		{"double tilde multiple", "~~Multiple~~ ~~strikethrough~~ sections", "~Multiple~ ~strikethrough~ sections"},

		{"simple underline", "__underline text__", "__underline text__"},

		{"simple spoiler", "||spoiler text||", "||spoiler text||"},

		{"blockquote with space", "> blockquote", ">blockquote"},
		{"blockquote without space", ">blockquote", ">blockquote"},
		{"blockquote multiline", "> blockquote\n> another line", ">blockquote\n>another line"},

		{"inline code", "This is `inline code`.", "This is `inline code`\\."},

		{"code block", "```\ncode block\n```", "```\ncode block\n```"},
		{"code block with inline code", "```\ncode block with `inline code`\n```", "```\ncode block with \\`inline code\\`\n```"},
		{"code block with lang", "```python\nprint('Hello')\n```", "```python\nprint('Hello')\n```"},

		{"nested bold italic", "**bold and _italic_ text**", "*bold and _italic_ text*"},
		{"nested italic bold asterisk", "*italic **bold** text*", "_italic *bold* text_"},
		{"nested italic bold underscore", "_italic **bold** text_", "_italic *bold* text_"},
		{"triple asterisk", "***bold italic text***", "*_bold italic text_*"},
		{"nested italic underline", "_italic and __underline__ text_", "_italic and __underline__ text_"},
		{
			// Bold converts to a single '*'; inner delimiters that cannot
			// form valid nested spans stay escaped. Balanced MarkdownV2.
			"deeply nested",
			"**bold _italic bold ~italic bold strikethrough ||italic boldstrikethrough spoiler||~ __underline italic bold___ bold**",
			"*bold \\_italic bold ~italic bold strikethrough ||italic boldstrikethrough spoiler||~ \\_\\_underline italic bold\\_\\_\\_ bold*",
		},
		// Strikethrough nests inside italic, so single '*' becomes italic '_'.
		{"italic with strikethrough inside", "\n*Italic with ~~strikethrough~~ inside*", "\n_Italic with ~strikethrough~ inside_"},

		{"link", "[link 2](https://google.com)", "[link 2](https://google.com)"},
		{"link with markdown", "[**bold link**](https://google.com)", "[*bold link*](https://google.com)"},

		{
			"special characters",
			"Characters: _*[]()~`>#+-=|{}.!",
			`Characters: \_\*\[\]\(\)\~` + "\\`" + `\>\#\+\-\=\|\{\}\.\!`,
		},

		{"backslash path", `Path: C:\Users\user\Documents`, `Path: C:\\Users\\user\\Documents`},
		{"backslash trailing", `C:\test\`, `C:\\test\\`},
		{"forward slashes", "C:/test/", "C:/test/"},
		{"backslash with dot", `C:\test\file.txt`, `C:\\test\\file\.txt`},
		{"backslash in code block", "```python\nprint('Hello')\nC:\\test\\```", "```python\nprint('Hello')\nC:\\\\test\\\\\n```"},

		{"code with markdown chars", "`code with * and _`", "`code with * and _`"},
		{"code block with bold", "```\n**bold in code block**\n```", "```\n**bold in code block**\n```"},
		{"code with backslash and backtick", "`code with \\ and ` backticks`", "`code with \\\\ and \\` backticks`"},
		{"code with special chars", "`code with ( ) special [.] characters!`", "`code with ( ) special [.] characters!`"},

		{"empty string", "", ""},

		{"already escaped", `This is \*already escaped\*`, `This is \*already escaped\*`},

		{
			"path inline code",
			"`C:\\Users\\user\\Documents\\Project\\ver_2.3\\`",
			"`C:\\\\Users\\\\user\\\\Documents\\\\Project\\\\ver_2.3\\\\`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// A realistic LLM answer mixing code blocks, inline code, lists, links,
// and formatting.
func TestConvertComplexTextWithCode(t *testing.T) {
	in := "Of course! Here’s the Python maze shortest path example in English:" +
		"\n\n---\n\n### 🧭 Example: Shortest Path in a " +
		"Maze using BFS (Python)\n\n```python\nfrom collections import deque\n\n" +
		"def shortest_path(maze, start, end):\n" +
		"    rows, cols = len(maze), len(maze[0])\n" +
		"    queue = deque([(start, [start])])\n" +
		"    directions = [(-1,0), (1,0), (0,-1), (0,1)]\n" +
		"    visited = {start}\n" +
		"```\n\n---\n\n" +
		"**What does this do?**\n\n- **Input:**\n" +
		"  - A maze (`sample_maze`) as a 2D list: `0` is open, `1` is a wall.\n" +
		"  - `start` and `end` points as (row, col) tuples.\n\n" +
		"- **How:**\n" +
		"  - Uses Breadth-First Search to find from `start` to `end`.\n" +
		"  - Moves only up/down/left/right (no diagonals).\n" +
		"  - Tracks visited cells to avoid loops.\n\n" +
		"- **Output:**\n" +
		"  - Prints the list of coordinates, or `None` if not found.\n\n" +
		"---\n\n" +
		"If you want another code example" +
		"(e.g., **bold**, _italic_, ~strikethrough~" +
		"\n" +
		">blockquotes" +
		"\n" +
		"[links](http://example.com)), just let me know—happy to help!"

	want := "Of course\\! Here’s the Python maze shortest path example in English:\n" +
		"\n" +
		"\\-\\-\\-\n" +
		"\n" +
		"\\#\\#\\# 🧭 Example: Shortest Path in a Maze using BFS \\(Python\\)\n" +
		"\n" +
		"```python\n" +
		"from collections import deque\n" +
		"\n" +
		"def shortest_path(maze, start, end):\n" +
		"    rows, cols = len(maze), len(maze[0])\n" +
		"    queue = deque([(start, [start])])\n" +
		"    directions = [(-1,0), (1,0), (0,-1), (0,1)]\n" +
		"    visited = {start}\n" +
		"```\n" +
		"\n" +
		"\\-\\-\\-\n" +
		"\n" +
		"*What does this do?*\n" +
		"\n" +
		"\\- *Input:*\n" +
		"  \\- A maze \\(`sample_maze`\\) as a 2D list: `0` is open, `1` is a wall\\.\n" +
		"  \\- `start` and `end` points as \\(row, col\\) tuples\\.\n" +
		"\n" +
		"\\- *How:*\n" +
		"  \\- Uses Breadth\\-First Search to find from `start` to `end`\\.\n" +
		"  \\- Moves only up/down/left/right \\(no diagonals\\)\\.\n" +
		"  \\- Tracks visited cells to avoid loops\\.\n" +
		"\n" +
		"\\- *Output:*\n" +
		"  \\- Prints the list of coordinates, or `None` if not found\\.\n" +
		"\n" +
		"\\-\\-\\-\n" +
		"\n" +
		"If you want another code example" +
		"\\(e\\.g\\., *bold*, _italic_, ~strikethrough~" +
		"\n" +
		">blockquotes" +
		"\n" +
		"[links](http://example.com)\\), just let me know—happy to help\\!"

	if got := Convert(in); got != want {
		t.Errorf("Convert() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestConvertWordBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Delimiters inside words must not toggle formatting.
		{"snake case", "snake_case_name", `snake\_case\_name`},
		{"asterisk inside word", "2*3*4", `2\*3\*4`},
		// Word matching is Unicode-aware: é blocks the word boundary.
		{"unicode word boundary", "café_test_х", `café\_test\_х`},
		{"unicode italic", "*привет мир*", "_привет мир_"},
		// Whitespace matching is Unicode-aware too: exotic whitespace
		// around '>' still forms a blockquote.
		{"blockquote after nbsp", " > quote", ">quote"},
		{"blockquote nbsp content", "> after", ">after"},
		{"blockquote after vertical tab", "\v> q", ">q"},
		{"blockquote after ideographic space", "　> q", ">q"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// One case per finding of the 2026-07-03 review; comments name the defect.
func TestConvertReviewFixes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Parenthesized URLs survive; ')' inside URLs is escaped.
		{"wikipedia link", "[Go](https://en.wikipedia.org/wiki/Go_(programming_language))",
			`[Go](https://en.wikipedia.org/wiki/Go_(programming_language\))`},
		{"url with backslash", `[t](http://a.com/a\b)`, `[t](http://a.com/a\\b)`},

		// Blockquote matching stays on its own line.
		{"blockquotes with blank line", "> q1\n\n> q2", ">q1\n\n>q2"},
		{"blockquote after paragraph", "para\n\n> quote", "para\n\n>quote"},
		{"empty quote then plain line", ">\nnext line", ">\nnext line"},

		// Flanking: list markers and spaced operators are not emphasis.
		{"asterisk bullet list", "* first\n* second\n* third", "\\* first\n\\* second\n\\* third"},
		{"spaced multiplication", "5 * 3 * 2 = 30", `5 \* 3 \* 2 \= 30`},
		{"spaced exponentiation", "2 ** 3 ** 4", `2 \*\* 3 \*\* 4`},

		// ___x___ nests underline over italic; \r separates each adjacent
		// underscore run so greedy __ parsing stays balanced.
		{"bold italic underline", "___both___", "__\r_both_\r__"},

		// Word boundaries for the previously unguarded delimiters.
		{"intraword tilde", "file~1~2", `file\~1\~2`},
		{"intraword tilde range", "between 5~6 and 7~8 units", `between 5\~6 and 7\~8 units`},
		{"intraword double underscore", "snake__case__here", `snake\_\_case\_\_here`},
		{"logical or in prose", "if a || b || c then", `if a \|\| b \|\| c then`},

		// Escaped underscores stay literal, like escaped asterisks.
		{"escaped underscores", `\_not italic\_`, `\_not italic\_`},

		// Double-backtick code spans.
		{"double backtick span", "``x``", "`x`"},
		{"double backtick padding", "`` x ``", "`x`"},

		// Single-line fenced blocks get spec escaping like multiline ones.
		{"single line fence backslash", "```C:\\path\\file```", "```\nC:\\\\path\\\\file\n```"},
		{"single line fence backticks", "```echo `date` now```", "```\necho \\`date\\` now\n```"},

		// Reserved control bytes are stripped, not turned into formatting.
		{"control bytes stripped", "a\x01b\x03c\x0Ed", "abcd"},

		// Placeholder-like literal text is not corrupted.
		{"placeholder-like literal", "zxzC0zxz and `code`", "zxzC0zxz and `code`"},
		{"placeholder-like with links", "zxzL0zxz [a](https://e.com) [b](https://e.com)",
			"zxzL0zxz [a](https://e.com) [b](https://e.com)"},

		// **…** containing literal underscores is bold, not preserved-literal.
		{"bold with literal underscores", "**a ___ b**", `*a \_\_\_ b*`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Convert(tt.in); got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// Cases from the second-pass adversarial review: nesting and run edge cases
// whose output must be balanced MarkdownV2.
func TestConvertNesting(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Adjacent underscore runs are separated by \r at every boundary,
		// so nothing collapses into an ambiguous ___ that Telegram misreads.
		{"italic wrapping triple underscore", "*___both___*", "_\r__\r_both_\r__\r_"},
		{"bold wrapping triple underscore", "**bold ___both___ bold**", "*bold __\r_both_\r__ bold*"},
		{"italic flush with underline", "*__x__*", "_\r__x__\r_"},
		{"bold italic flush with underline", "***__x__***", "*_\r__x__\r_*"},
		{"stray underscore runs escape", "____four____", `\_\_\_\_four\_\_\_\_`},
		{"intraword triple underscore", "snake___case___here", `snake\_\_\_case\_\_\_here`},

		// Word-flanked ** with underline content stays literal (balanced).
		{"word-flanked bold underline", "a**__b__**c", `a\*\*__b__\*\*c`},

		// Delimiter runs never open a lone span.
		{"intraword double tilde", "a~~b~~c", `a\~\~b\~\~c`},
		{"double tilde around word", "word~~strike~~word", `word\~\~strike\~\~word`},
		{"triple pipe run", "a|||b|||c", `a\|\|\|b\|\|\|c`},

		// Escaped delimiters stay literal on both the opening and closing side.
		{"escaped italic closer", `_text\_`, `\_text\_`},
		{"escaped bold closer", `*text\*`, `\*text\*`},

		// A code span (or fenced block) is never re-wrapped or left raw.
		{"fence inside double backtick", "``a ```x``` b``", "\\`\\`a ```\nx\n``` b\\`\\`"},
		{"code span in link url", "[x](a`)`b)", "[x](a`\\)`b)"},
		{"all-space double backtick", "``  ``", "`  `"},
		// A link whose text is a code span containing ']' is valid tdlib.
		{"code span with bracket in link text", "[`]`](b)", "[`]`](b)"},

		// Escaped brackets do not form a link.
		{"escaped brackets", `\[not a link\](url)`, `\[not a link\]\(url\)`},

		// Multiline fence with a 4+ backtick opening normalizes to ```.
		{"four backtick fence", "````\ncode\n```", "```\ncode\n```"},

		// A '>' in link text is escaped, not turned into a blockquote.
		{"gt in link text", "[>x](u)", `[\>x](u)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Convert(tt.in)
			if got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
			if !validMarkdownV2(got) {
				t.Errorf("Convert(%q) = %q is not valid MarkdownV2", tt.in, got)
			}
		})
	}
}

// Deeply malformed input that the multi-pass conversion cannot render as valid
// MarkdownV2 falls back to fully escaped plain text — always safe to send.
func TestConvertSafetyNet(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"crossing italic strike", "_~_~", `\_\~\_\~`},
		{"crossing bold strike", "~**~**", `\~\*\*\~\*\*`},
		{"crossing underline spoiler", "__||__||", `\_\_\|\|\_\_\|\|`},
		// Emphasis opened in a blockquote and closed on an unquoted line —
		// Telegram force-closes the entity at the blockquote-ending newline.
		{"emphasis crosses blockquote end", ">*a\nb*", "\\>\\*a\nb\\*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Convert(tt.in)
			if got != tt.want {
				t.Errorf("Convert(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
			if !validMarkdownV2(got) {
				t.Errorf("fallback for %q is still invalid: %q", tt.in, got)
			}
		})
	}
}

func TestValidMarkdownV2(t *testing.T) {
	valid := []string{
		"", "plain text", "*bold*", "_italic_", "__underline__", "~strike~",
		"||spoiler||", "*_bi_*", "__\r_both_\r__", "`code`", "```\npre\n```",
		"[text](http://a.com)", "a\\*b", ">quote", "[x](a`\\)`b)",
		">a\n>b", "[`]`](b)", // multi-line quote; link text = code span with ]
	}
	invalid := []string{
		"*unclosed", "_~_~", "`unterminated", "[text](url", "a!b", "plain)text",
		"__underline_", ">*a\nb*", // emphasis crosses blockquote end
	}
	for _, s := range valid {
		if !validMarkdownV2(s) {
			t.Errorf("validMarkdownV2(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if validMarkdownV2(s) {
			t.Errorf("validMarkdownV2(%q) = true, want false", s)
		}
	}
}

func TestEscapeSpecialChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"specials", "a.b!c", `a\.b\!c`},
		{"already escaped", `\.`, `\.`},
		{"escaped backslash", `\\`, `\\`},
		{"bare backslash", `a\b`, `a\\b`},
		{"trailing backslash", `a\`, `a\\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeSpecialChars(tt.in); got != tt.want {
				t.Errorf("EscapeSpecialChars(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func BenchmarkConvert(b *testing.B) {
	in := "Here's **bold**, _italic_ and a [link](https://example.com):\n\n" +
		"```go\nfmt.Println(\"hello\")\n```\n\n" +
		"- `inline code` with special chars: 1 + 2 = 3!\n" +
		"> a quote to finish."
	b.ReportAllocs()
	for b.Loop() {
		Convert(in)
	}
}

func FuzzConvert(f *testing.F) {
	seeds := []string{
		"",
		"Hello world.",
		"**bold** _italic_ ~strike~ ||spoiler|| `code`",
		"```go\nfmt.Println(\"hi\")\n```",
		"[link](https://example.com) > quote",
		`\*escaped\* C:\path\`,
		"***a*** ___b___ __c__ ~~d~~",
		"*ставка **на** слово*",
		"`a ` b` ``` \\",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Convert(in) // must not panic or hang
		if utf8.ValidString(in) && !utf8.ValidString(out) {
			t.Errorf("Convert(%q) produced invalid UTF-8: %q", in, out)
		}
		// Every output must be sendable MarkdownV2 — for well-formed input
		// directly, and for malformed input via the plain-text fallback.
		if !validMarkdownV2(out) {
			t.Errorf("Convert(%q) = %q is not valid MarkdownV2", in, out)
		}
	})
}
