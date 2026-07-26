package tgmarkdown

import (
	"strings"
	"testing"
	"time"
)

func TestIsolateInlineLinks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantURL  string
	}{
		{
			name:     "escaped closing bracket in label",
			input:    `[a \] b](url)`,
			wantText: `a \] b`,
			wantURL:  `url`,
		},
		{
			name:     "escaped parenthesis in destination",
			input:    `[x](https://example.com/a\)b)`,
			wantText: `x`,
			wantURL:  `https://example.com/a)b`,
		},
		{
			name:     "nested brackets in label",
			input:    `[outer [inner] label](url)`,
			wantText: `outer [inner] label`,
			wantURL:  `url`,
		},
		{
			name:     "balanced raw parentheses",
			input:    `[Go](https://en.wikipedia.org/wiki/Go_(language))`,
			wantText: `Go`,
			wantURL:  `https://en.wikipedia.org/wiki/Go_(language)`,
		},
		{
			name:     "angle destination is unwrapped",
			input:    `[x](<https://example.com/a b>)`,
			wantText: `x`,
			wantURL:  `https://example.com/a b`,
		},
		{
			name:     "ASCII punctuation escapes are decoded",
			input:    `[x \/](https://example.com/a\_b)`,
			wantText: `x /`,
			wantURL:  `https://example.com/a_b`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			got := isolateInlineLinks(test.input, func(linkText, url string) string {
				calls++
				if linkText != test.wantText {
					t.Errorf("link text = %q, want %q", linkText, test.wantText)
				}
				if url != test.wantURL {
					t.Errorf("URL = %q, want %q", url, test.wantURL)
				}
				return "<stored>"
			})
			if calls != 1 {
				t.Fatalf("store calls = %d, want 1", calls)
			}
			if got != "<stored>" {
				t.Errorf("result = %q, want %q", got, "<stored>")
			}
		})
	}
}

func TestIsolateInlineLinksPreservesSurroundingTextAndPlaceholders(t *testing.T) {
	textPlaceholder := codePlaceholder(2)
	urlPlaceholder := codePlaceholder(3)
	input := "before [x " + textPlaceholder + "](u" + urlPlaceholder + ") after"

	got := isolateInlineLinks(input, func(linkText, url string) string {
		if want := "x " + textPlaceholder; linkText != want {
			t.Errorf("link text = %q, want %q", linkText, want)
		}
		if want := "u" + urlPlaceholder; url != want {
			t.Errorf("URL = %q, want %q", url, want)
		}
		return linkPlaceholder(4)
	})

	if want := "before " + linkPlaceholder(4) + " after"; got != want {
		t.Errorf("result = %q, want %q", got, want)
	}
}

func TestIsolateInlineLinksLeavesMalformedAndEscapedSyntaxAlone(t *testing.T) {
	tests := []string{
		`\[not a link](url)`,
		`[not a link]\(url)`,
		`[missing close](url`,
		`[bad angle](<url)`,
		"[line break](<a\nb>)",
		`[](url)`,
		`[empty destination]()`,
		`[empty angle](<>)`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := isolateInlineLinks(input, func(_, _ string) string {
				t.Fatal("store called for malformed link")
				return ""
			})
			if got != input {
				t.Errorf("result = %q, want unchanged %q", got, input)
			}
		})
	}
}

func TestIsolateInlineLinksMultiple(t *testing.T) {
	input := `one [a](u) two [b \]](<v w>) three`
	var stored []string
	got := isolateInlineLinks(input, func(text, url string) string {
		stored = append(stored, text+"|"+url)
		return "{" + text + "}"
	})

	if want := `one {a} two {b \]} three`; got != want {
		t.Errorf("result = %q, want %q", got, want)
	}
	if want := []string{"a|u", `b \]|v w`}; len(stored) != len(want) {
		t.Fatalf("stored = %q, want %q", stored, want)
	} else {
		for i := range want {
			if stored[i] != want[i] {
				t.Errorf("stored[%d] = %q, want %q", i, stored[i], want[i])
			}
		}
	}
}

func TestConvertInlineLinkCommonMarkEscapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`[a \] b](url)`, `[a \] b](url)`},
		{`[\*literal stars\*](url)`, `[\*literal stars\*](url)`},
		{`[\_literal underscores\_](url)`, `[\_literal underscores\_](url)`},
		{`[slash \/ and quote \"](url)`, `[slash / and quote "](url)`},
		{`[x](https://example.com/a\)b)`, `[x](https://example.com/a\)b)`},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			if got := Convert(test.input); got != test.want {
				t.Errorf("Convert(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestIsolateInlineLinksAdversarialManyOpenBrackets(t *testing.T) {
	if testing.Short() {
		t.Skip("performance sanity check")
	}

	input := strings.Repeat("[", 250_000) + "x](unterminated"
	start := time.Now()
	got := isolateInlineLinks(input, func(_, _ string) string {
		t.Fatal("store called for malformed link")
		return ""
	})
	elapsed := time.Since(start)

	if got != input {
		t.Fatal("malformed input changed")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("adversarial scan took %s; expected near-linear behaviour", elapsed)
	}
}
