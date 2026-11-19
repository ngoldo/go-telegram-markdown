package tgmarkdown

import (
	"strings"
	"testing"
)

func TestReplaceAsteriskEmphasis(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"italic", "*text*", "_text_"},
		{"bold", "**text**", "*text*"},
		{
			"flush opening bold in italic",
			"***bold** and italic*",
			"_*bold* and italic_",
		},
		{
			"flush opening italic in bold",
			"***italic* and bold**",
			"*_italic_ and bold*",
		},
		{
			"flush closing italic in bold",
			"**bold *italic***",
			"*bold _italic_*",
		},
		{
			"flush closing bold in italic",
			"*italic **bold***",
			"_italic *bold*_",
		},
		{
			"escaped literal inside italic",
			`*foo \* bar*`,
			`_foo \* bar_`,
		},
		{
			"escaped asterisk before italic opener",
			`\**foo*`,
			`\*_foo_`,
		},
		{
			"escaped asterisk before bold opener",
			`\***foo**`,
			`\**foo*`,
		},
		{
			"escaped asterisk before italic closer",
			`*foo \**`,
			`_foo \*_`,
		},
		{
			"escaped asterisk before bold closer",
			`**foo \***`,
			`*foo \**`,
		},
		{
			"nested bold is flattened",
			"****foo****",
			"*foo*",
		},
		{
			"nested bold sharing an opener is flattened",
			"****x** y**",
			"*x y*",
		},
		{"list marker", "* item", "* item"},
		{"spaced operator", "5 * 3 * 2", "5 * 3 * 2"},
		{"intraword run", "a**b**c", "a**b**c"},
		{"escaped delimiters", `\*text\*`, `\*text\*`},
		{"asterisk keycap is literal", "*️⃣word*", "*️⃣word*"},
		{"asterisk keycap without variation selector is literal", "*\u20E3word*", "*\u20E3word*"},
		{
			"delimiter before asterisk keycap remains active",
			"**️⃣word*",
			"_*️⃣word_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, balanced := restorePlaceholders(replaceAsteriskEmphasis(tt.in, true)); !balanced || got != tt.want {
				t.Fatalf("replaceAsteriskEmphasis(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReplaceDelimitedLongPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("performance sanity check")
	}

	const spans = 65_536
	prefix := strings.Repeat("x", 8<<20)
	input := prefix + strings.Repeat(" ~q~~", spans)
	got := replaceDelimited(input, "~", func(content string) string {
		return "[" + content + "]"
	})

	// The first span still treats the longer closing run as literal.
	want := prefix + " ~q~~" + strings.Repeat(" [q]~", spans-1)
	if got != want {
		t.Fatal("delimiter conversion changed around the long prefix")
	}
}

func BenchmarkReplaceDelimitedLongPrefix(b *testing.B) {
	input := strings.Repeat("x", 8<<20) + strings.Repeat(" ~q~~", 65_536)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for b.Loop() {
		replaceDelimited(input, "~", func(content string) string {
			return "[" + content + "]"
		})
	}
}

func TestIndexDelimiterRunsKeepsLiteralRunsCompact(t *testing.T) {
	// A long unmatched run must occupy one index entry, rather than one
	// entry for each position where the delimiter could begin.
	runs := indexDelimiterRuns(strings.Repeat("_", 1<<20), "__")
	if len(runs) != 1 || runs[0].length != 1<<20 {
		t.Fatalf("indexed %d runs, want one full-width run", len(runs))
	}
}
