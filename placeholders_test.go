package tgmarkdown

import (
	"strings"
	"testing"
)

func TestExpandIndexedPlaceholders(t *testing.T) {
	text := "a" + codePlaceholder(1) + "b" + codePlaceholder(0) + "c"
	got := expandIndexedPlaceholders(text, codeOpen[0], codeClose[0], []string{"zero", "one"})
	if want := "aonebzeroc"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandIndexedPlaceholdersDoesNotRecurse(t *testing.T) {
	nested := codePlaceholder(0)
	got := expandIndexedPlaceholders(nested, codeOpen[0], codeClose[0], []string{nested})
	if got != nested {
		t.Fatalf("got %q, want unchanged nested placeholder %q", got, nested)
	}
}

func BenchmarkExpandIndexedPlaceholders(b *testing.B) {
	const count = 16_384
	var text strings.Builder
	values := make([]string, count)
	for i := range count {
		text.WriteString(codePlaceholder(i))
		values[i] = "`x`"
	}
	input := text.String()

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		_ = expandIndexedPlaceholders(input, codeOpen[0], codeClose[0], values)
	}
}

func BenchmarkConvertManyProtectedSpans(b *testing.B) {
	const count = 8_192
	input := strings.Repeat("`x` [label](https://example.com/a) ", count)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		_ = Convert(input)
	}
}
