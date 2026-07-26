package tgmarkdown

import "testing"

func TestReplaceAsteriskEmphasis(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"italic", "*text*", phItalic + "text" + phItalic},
		{"bold", "**text**", phBold + "text" + phBold},
		{
			"flush opening bold in italic",
			"***bold** and italic*",
			phItalic + phBold + "bold" + phBold + " and italic" + phItalic,
		},
		{
			"flush opening italic in bold",
			"***italic* and bold**",
			phBold + phItalic + "italic" + phItalic + " and bold" + phBold,
		},
		{
			"flush closing italic in bold",
			"**bold *italic***",
			phBold + "bold " + phItalic + "italic" + phItalic + phBold,
		},
		{
			"flush closing bold in italic",
			"*italic **bold***",
			phItalic + "italic " + phBold + "bold" + phBold + phItalic,
		},
		{
			"escaped literal inside italic",
			`*foo \* bar*`,
			phItalic + `foo \* bar` + phItalic,
		},
		{
			"nested bold is flattened",
			"****foo****",
			phBold + "foo" + phBold,
		},
		{
			"nested bold sharing an opener is flattened",
			"****x** y**",
			phBold + "x y" + phBold,
		},
		{"list marker", "* item", "* item"},
		{"spaced operator", "5 * 3 * 2", "5 * 3 * 2"},
		{"intraword run", "a**b**c", "a**b**c"},
		{"escaped delimiters", `\*text\*`, `\*text\*`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceAsteriskEmphasis(tt.in); got != tt.want {
				t.Fatalf("replaceAsteriskEmphasis(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}
