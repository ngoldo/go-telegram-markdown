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
			"escaped asterisk before italic opener",
			`\**foo*`,
			`\*` + phItalic + "foo" + phItalic,
		},
		{
			"escaped asterisk before bold opener",
			`\***foo**`,
			`\*` + phBold + "foo" + phBold,
		},
		{
			"escaped asterisk before italic closer",
			`*foo \**`,
			phItalic + `foo \*` + phItalic,
		},
		{
			"escaped asterisk before bold closer",
			`**foo \***`,
			phBold + `foo \*` + phBold,
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
		{"asterisk keycap is literal", "*️⃣word*", "*️⃣word*"},
		{"asterisk keycap without variation selector is literal", "*\u20E3word*", "*\u20E3word*"},
		{
			"delimiter before asterisk keycap remains active",
			"**️⃣word*",
			phItalic + "*️⃣word" + phItalic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceAsteriskEmphasis(tt.in); got != tt.want {
				t.Fatalf("replaceAsteriskEmphasis(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}
