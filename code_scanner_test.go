package tgmarkdown

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIsolateCode(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantText   string
		wantStored []string
	}{
		{
			name:     "four-backtick fence preserves inner triple run",
			input:    "````md\nalpha\n```\nomega\n````",
			wantText: "<0>",
			wantStored: []string{
				"```md\nalpha\n\\`\\`\\`\nomega\n```",
			},
		},
		{
			name:     "triple run after code is content not a closing fence",
			input:    "```\nalpha ```\nomega\n```",
			wantText: "<0>",
			wantStored: []string{
				"```\nalpha \\`\\`\\`\nomega\n```",
			},
		},
		{
			name:       "escaped backticks stay literal",
			input:      "before \\`not code\\` after",
			wantText:   "before \\`not code\\` after",
			wantStored: nil,
		},
		{
			name:     "escaped first backtick leaves code opener",
			input:    "before \\``code` after",
			wantText: "before \\`<0> after",
			wantStored: []string{
				"`code`",
			},
		},
		{
			name:     "double-backtick span contains single backtick",
			input:    "before ``code ` tick`` after",
			wantText: "before <0> after",
			wantStored: []string{
				"`code \\` tick`",
			},
		},
		{
			name:     "quoted fenced block drops continuation quote markers",
			input:    "> ```go\n> fmt.Println(\"x\")\n> ```\nafter",
			wantText: "> <0>\nafter",
			wantStored: []string{
				"```go\nfmt.Println(\"x\")\n```",
			},
		},
		{
			name:     "unclosed fence is closed at end of input",
			input:    "```go\nfmt.Println(\"x\")",
			wantText: "<0>",
			wantStored: []string{
				"```go\nfmt.Println(\"x\")\n```",
			},
		},
		{
			name:     "fence info keeps only Telegram language token",
			input:    "``` go title=example\nx\n```",
			wantText: "<0>",
			wantStored: []string{
				"```go\nx\n```",
			},
		},
		{
			name:     "fence language decodes CommonMark punctuation escape",
			input:    "``` foo\\+bar title=example\nx\n```",
			wantText: "<0>",
			wantStored: []string{
				"```foo+bar\nx\n```",
			},
		},
		{
			name:     "carriage-return fenced block",
			input:    "```go\rx\r```\rafter",
			wantText: "<0>\rafter",
			wantStored: []string{
				"```go\nx\r\n```",
			},
		},
		{
			name:     "CRLF fenced block",
			input:    "```go\r\nx\r\n```\r\nafter",
			wantText: "<0>\r\nafter",
			wantStored: []string{
				"```go\nx\r\n```",
			},
		},
		{
			name:     "unclosed quoted fence stops with blockquote",
			input:    "> ```go\n> quoted\noutside",
			wantText: "> <0>\noutside",
			wantStored: []string{
				"```go\nquoted\n```",
			},
		},
		{
			name:       "escaped unmatched run is not an opener",
			input:      "\\`literal",
			wantText:   "\\`literal",
			wantStored: nil,
		},
		{
			name:     "legacy same-line triple fence",
			input:    "before ```echo `date` now``` after",
			wantText: "before <0> after",
			wantStored: []string{
				"```\necho \\`date\\` now\n```",
			},
		},
		{
			name:     "four-backtick inline span",
			input:    "before ````code ``` tick```` after",
			wantText: "before <0> after",
			wantStored: []string{
				"`code \\`\\`\\` tick`",
			},
		},
		{
			name:     "code span padding",
			input:    "`` padded `` and ``  ``",
			wantText: "<0> and <1>",
			wantStored: []string{
				"`padded`",
				"`  `",
			},
		},
		{
			name:     "fence takes precedence over enclosing inline-looking run",
			input:    "``before ```inside``` after``",
			wantText: "``before <0> after``",
			wantStored: []string{
				"```\ninside\n```",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored []string
			got := isolateCode(tt.input, func(_, rendered string) string {
				stored = append(stored, rendered)
				return fmt.Sprintf("<%d>", len(stored)-1)
			})
			if got != tt.wantText {
				t.Errorf("isolateCode(%q)\n got: %q\nwant: %q", tt.input, got, tt.wantText)
			}
			if !reflect.DeepEqual(stored, tt.wantStored) {
				t.Errorf("stored code\n got: %#v\nwant: %#v", stored, tt.wantStored)
			}
		})
	}
}

func TestIsolateCodeFenceLength(t *testing.T) {
	input := "````\nfirst\n```\nstill code\n`````\nafter"
	var stored []string
	got := isolateCode(input, func(_, rendered string) string {
		stored = append(stored, rendered)
		return "<code>"
	})

	if got != "<code>\nafter" {
		t.Fatalf("isolateCode(%q) = %q, want %q", input, got, "<code>\nafter")
	}
	want := []string{"```\nfirst\n\\`\\`\\`\nstill code\n```"}
	if !reflect.DeepEqual(stored, want) {
		t.Fatalf("stored code = %#v, want %#v", stored, want)
	}
}

func TestIsolateCodeReportsExactSource(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "escaped first backtick",
			in:   "before \\``code` after",
			want: []string{"`code`"},
		},
		{
			name: "lone CR fence",
			in:   "```go\rx\r```\rafter",
			want: []string{"```go\rx\r```"},
		},
		{
			name: "CRLF fence",
			in:   "```go\r\nx\r\n```\r\nafter",
			want: []string{"```go\r\nx\r\n```"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sources []string
			isolateCode(test.in, func(source, _ string) string {
				sources = append(sources, source)
				return strings.Repeat("x", len(source))
			})
			if !reflect.DeepEqual(sources, test.want) {
				t.Errorf("sources = %#v, want %#v", sources, test.want)
			}
		})
	}
}

func TestCodeSpanEndsPreservesIndexes(t *testing.T) {
	tests := []string{
		"`[x](url)`",
		"before \\``code` after",
		"```go\r\nx\r\n```\r\nafter",
		"``a`` and `b`",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			ends := codeSpanEnds(input)
			if len(ends) != len(input) {
				t.Fatalf("len(codeSpanEnds) = %d, want %d", len(ends), len(input))
			}
			for start, end := range ends {
				if end != 0 && (end <= start || end > len(input)) {
					t.Fatalf("invalid span %d:%d for input length %d", start, end, len(input))
				}
			}
		})
	}
}

func TestIsolateCodeEscapedRunsStayLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("performance sanity check")
	}

	input := strings.Repeat("\\``a", 100_000) + "\n```\nx\n```\n`"
	start := time.Now()
	got := isolateCode(input, func(_, _ string) string {
		return "<code>"
	})
	elapsed := time.Since(start)

	if got == "" {
		t.Fatal("isolateCode returned an empty result")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("escaped-run scan took %s; expected near-linear behaviour", elapsed)
	}
}
