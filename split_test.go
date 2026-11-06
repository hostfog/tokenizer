package tokenizer

import (
	"reflect"
	"testing"
)

func pieces(ns []normalized) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.s
	}
	return out
}

func TestScanners(t *testing.T) {
	gpt2, _ := patternFinder(gpt2Pattern)
	llama3, _ := patternFinder(llama3Pattern)
	qwen2, _ := patternFinder(qwen2Pattern)

	tests := []struct {
		name string
		find finder
		in   string
		want []string
	}{
		{"gpt2", gpt2, "Hello world", []string{"Hello", " world"}},
		{"gpt2", gpt2, "I'm   fine", []string{"I", "'m", "  ", " fine"}},
		{"gpt2", gpt2, "I'M OK", []string{"I", "'", "M", " OK"}},
		{"gpt2", gpt2, "x  \n\n  y", []string{"x", "  \n\n ", " y"}},
		{"gpt2", gpt2, "12345 abc", []string{"12345", " abc"}},
		{"gpt2", gpt2, "  ...!!\n", []string{" ", " ...!!", "\n"}},
		{"gpt2", gpt2, "a\tb", []string{"a", "\t", "b"}},
		{"llama3", llama3, "I'M OK", []string{"I", "'M", " OK"}},
		{"llama3", llama3, "x  \n\n  y", []string{"x", "  \n\n", " ", " y"}},
		{"llama3", llama3, "12345 abc", []string{"123", "45", " abc"}},
		{"llama3", llama3, "  ...!!\n", []string{" ", " ...!!\n"}},
		{"llama3", llama3, "'xyz", []string{"'xyz"}},
		{"llama3", llama3, "a\tb", []string{"a", "\tb"}},
		{"qwen2", qwen2, "12345", []string{"1", "2", "3", "4", "5"}},
	}
	for _, tt := range tests {
		n := newNormalized(tt.in, 0)
		got := pieces(n.split(tt.find(n.s), isolated, false))
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestSplitBehavior(t *testing.T) {
	tests := []struct {
		b    behavior
		want []string
	}{
		{removed, []string{"a", "b", "c"}},
		{isolated, []string{"a", "-", "b", "-", "-", "c", "-"}},
		{mergedWithPrevious, []string{"a-", "b-", "-", "c-"}},
		{mergedWithNext, []string{"a", "-b", "-", "-c", "-"}},
		{contiguous, []string{"a", "-", "b", "--", "c", "-"}},
	}
	n := newNormalized("a-b--c-", 0)
	for _, tt := range tests {
		got := pieces(n.split(literalFinder("-")(n.s), tt.b, false))
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("behavior %d = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestPreTokenizers(t *testing.T) {
	tests := []struct {
		name string
		p    preTokenizer
		in   string
		want []string
	}{
		{"whitespace", whitespace{}, "Hey, friend_1!!  héllo", []string{"Hey", ",", "friend_1", "!!", "héllo"}},
		{"digits", digits(false), "a123b4", []string{"a", "123", "b", "4"}},
		{"digits individual", digits(true), "a12", []string{"a", "1", "2"}},
		{"bert", bertPreTokenizer(), "Hi, there!", []string{"Hi", ",", "there", "!"}},
		{"char delimiter", charDelimiter('|'), "a|b||c", []string{"a", "b", "c"}},
		{"metaspace", metaspace{"▁", "always", true}, "a b", []string{"▁a", "▁b"}},
		{"byte level", byteLevel{addPrefixSpace: true, useRegex: true}, "hi there", []string{"Ġhi", "Ġthere"}},
	}
	for _, tt := range tests {
		got := pieces(tt.p.preTokenize([]normalized{newNormalized(tt.in, 0)}))
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestTranslateRegex(t *testing.T) {
	tests := []struct{ in, want string }{
		{`\s+`, `[\t\n\v\f\r\x{85}\p{Z}]+`},
		{`[^\s\d]`, `[^\t\n\v\f\r\x{85}\p{Z}\p{Nd}]`},
		{`\.\w`, `\.[\p{L}\p{M}\p{Nd}\p{Nl}\p{Pc}\x{200C}\x{200D}]`},
	}
	for _, tt := range tests {
		if got := translateRegex(tt.in); got != tt.want {
			t.Errorf("translateRegex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if _, err := patternFinder(`a(?!b)`); err == nil {
		t.Error("lookahead pattern compiled, want error")
	}
}

func TestLossy(t *testing.T) {
	tests := []struct {
		in   []byte
		want string
	}{
		{[]byte("ok"), "ok"},
		{[]byte{0xE2, 0x82, 'x'}, "\uFFFDx"},
		{[]byte{0xFF, 0xFE}, "\uFFFD\uFFFD"},
		{[]byte{0xF0, 0x9F, 0x98}, "\uFFFD"},
	}
	for _, tt := range tests {
		if got := lossy(tt.in); got != tt.want {
			t.Errorf("lossy(%x) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
