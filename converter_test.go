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

		{"code block", "```\ncode block\n```", "```\ncode block\n\n```"},
		{"code block with inline code", "```\ncode block with `inline code`\n```", "```\ncode block with \\`inline code\\`\n\n```"},
		{"code block with lang", "```python\nprint('Hello')\n```", "```python\nprint('Hello')\n\n```"},

		{"nested bold italic", "**bold and _italic_ text**", "*bold and _italic_ text*"},
		{"nested italic bold asterisk", "*italic **bold** text*", "_italic *bold* text_"},
		{"nested italic bold underscore", "_italic **bold** text_", "_italic *bold* text_"},
		{"triple asterisk", "***bold italic text***", "*_bold italic text_*"},
		{"nested italic underline", "_italic and __underline__ text_", "_italic and __underline__ text_"},
		{
			"deeply nested preserved",
			"**bold _italic bold ~italic bold strikethrough ||italic boldstrikethrough spoiler||~ __underline italic bold___ bold**",
			"**bold _italic bold ~italic bold strikethrough ||italic boldstrikethrough spoiler||~ __underline italic bold___ bold**",
		},
		{"italic with strikethrough inside", "\n*Italic with ~~strikethrough~~ inside*", "\n*Italic with ~strikethrough~ inside*"},

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
		{"code block with bold", "```\n**bold in code block**\n```", "```\n**bold in code block**\n\n```"},
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
		"    visited = {start}\n\n" +
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
	})
}
