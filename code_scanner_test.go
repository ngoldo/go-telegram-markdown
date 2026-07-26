package tgmarkdown

import (
	"fmt"
	"reflect"
	"testing"
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
			got := isolateCode(tt.input, func(rendered string) string {
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
	got := isolateCode(input, func(rendered string) string {
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
