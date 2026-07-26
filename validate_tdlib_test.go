package tgmarkdown

import "testing"

// These accept/reject cases are pinned to TDLib commit
// 022d60202e446ad1287b9fb68e687c8a0760788b. They are derived from
// parse_markdown_v2 in td/telegram/MessageEntity.cpp, the tg://emoji and
// tg://time parsing in td/telegram/LinkManager.cpp, and the date-format parser
// in td/telegram/FormattedDate.cpp at that commit.
func TestValidMarkdownV2TDLibCorpus(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty", text: "", want: true},
		{name: "unicode text", text: "café 👍", want: true},
		{name: "escaped reserved character", text: `a\*b`, want: true},
		{name: "nested entities", text: `*bold _italic __underlined__ text_*`, want: true},
		{name: "code", text: "`a_b`", want: true},
		{name: "pre", text: "```go\nx := 1\n```", want: true},
		{name: "explicit text URL", text: `[text](https://example.com/a\)b)`, want: true},
		{name: "implicit text URL", text: `[https://example\.com]`, want: true},
		{name: "blockquote at EOF", text: ">quote", want: true},
		{name: "multiline blockquote at EOF", text: ">first\n>second", want: true},
		{name: "blockquote ends at newline", text: ">quote\nplain", want: true},
		{name: "expandable blockquote at EOF", text: ">quote||", want: true},
		{name: "expandable blockquote at newline", text: ">quote||\nplain", want: true},
		{name: "expandable blockquote at CRLF", text: ">quote||\r\nplain", want: true},
		{name: "custom emoji", text: "![👍](tg://emoji?id=5368324170671202286)", want: true},
		{name: "custom emoji host variants", text: "![x](TG:EMOJI/?unused=1&id=-1#fragment)", want: true},
		{name: "empty custom emoji text is parseable", text: "![](tg:emoji?id=1)", want: true},
		{name: "formatted date", text: "![today](tg://time?unix=1647531900&format=wDT)", want: true},
		{name: "relative formatted date", text: "![now](TG://TIME/?unix=2147483647&format=R#fragment)", want: true},
		{name: "default formatted date", text: "![now](tg:time?format=&unix=1)", want: true},

		{name: "invalid UTF-8 byte", text: string([]byte{0xff}), want: false},
		{name: "truncated UTF-8 sequence", text: string([]byte{0xe2, 0x82}), want: false},
		{name: "unescaped reserved character", text: "plain.text", want: false},
		{name: "unclosed bold", text: "*bold", want: false},
		{name: "crossing entities", text: "_~_~", want: false},
		{name: "single pipe", text: "a|b", want: false},
		{name: "stray close parenthesis", text: "a)b", want: false},
		{name: "unclosed URL", text: "[text](https://example.com", want: false},
		{name: "expandability mark outside quote", text: "||", want: false},
		{name: "expandability mark before bare CR", text: ">quote||\r", want: false},
		{name: "expandability mark inside open bold", text: ">*quote||", want: false},
		{name: "custom entity requires URL", text: "![x]", want: false},
		{name: "custom entity requires tg URL", text: "![x](https://example.com)", want: false},
		{name: "custom emoji requires ID", text: "![x](tg://emoji?unused=1)", want: false},
		{name: "custom emoji rejects zero ID", text: "![x](tg://emoji?id=0)", want: false},
		{name: "custom emoji rejects noncanonical ID", text: "![x](tg://emoji?id=01)", want: false},
		{name: "custom emoji rejects plus sign", text: "![x](tg://emoji?id=+1)", want: false},
		{name: "custom emoji rejects overflow", text: "![x](tg://emoji?id=9223372036854775808)", want: false},
		{name: "custom emoji rejects extra path separator", text: "![x](tg://emoji//?id=1)", want: false},
		{name: "formatted date requires Unix time", text: "![x](tg://time?format=t)", want: false},
		{name: "formatted date rejects zero", text: "![x](tg://time?unix=0)", want: false},
		{name: "formatted date rejects negative time", text: "![x](tg://time?unix=-1)", want: false},
		{name: "formatted date rejects noncanonical time", text: "![x](tg://time?unix=01)", want: false},
		{name: "formatted date rejects int32 overflow", text: "![x](tg://time?unix=2147483648)", want: false},
		{name: "formatted date rejects unknown format", text: "![x](tg://time?unix=1&format=x)", want: false},
		{name: "formatted date rejects mixed relative format", text: "![x](tg://time?unix=1&format=rt)", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validMarkdownV2(test.text); got != test.want {
				t.Fatalf("validMarkdownV2(%q) = %t, want %t", test.text, got, test.want)
			}
		})
	}
}
