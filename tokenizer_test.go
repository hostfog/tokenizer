package tokenizer

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

func load(t testing.TB, name string) *Tokenizer {
	t.Helper()
	tok, err := Load("testdata/" + name)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("%s not present", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestEncode(t *testing.T) {
	tests := []struct {
		file    string
		in      string
		ids     []int
		tokens  []string
		offsets [][2]int
		decoded string
	}{
		{
			"wordpiece.json", "Hello, world!",
			[]int{2, 5, 8, 6, 7, 3},
			[]string{"[CLS]", "hello", ",", "world", "!", "[SEP]"},
			[][2]int{{0, 0}, {0, 5}, {5, 6}, {7, 12}, {12, 13}, {0, 0}},
			"hello, world!",
		},
		{
			"wordpiece.json", "unaffable café",
			[]int{2, 9, 10, 11, 12, 3},
			[]string{"[CLS]", "un", "##aff", "##able", "cafe", "[SEP]"},
			[][2]int{{0, 0}, {0, 2}, {2, 5}, {5, 9}, {10, 15}, {0, 0}},
			"unaffable cafe",
		},
		{
			"wordpiece.json", "  hello   [MASK]  world",
			[]int{2, 5, 4, 6, 3},
			[]string{"[CLS]", "hello", "   [MASK]", "world", "[SEP]"},
			[][2]int{{0, 0}, {2, 7}, {7, 16}, {18, 23}, {0, 0}},
			"hello world",
		},
		{
			"wordpiece.json", "中文 tokenizer",
			[]int{2, 13, 14, 18, 19, 3},
			[]string{"[CLS]", "中", "文", "token", "##izer", "[SEP]"},
			[][2]int{{0, 0}, {0, 3}, {3, 6}, {7, 12}, {12, 16}, {0, 0}},
			"中 文 tokenizer",
		},
		{
			"wordpiece.json", "xyz hello",
			[]int{2, 1, 5, 3},
			[]string{"[CLS]", "[UNK]", "hello", "[SEP]"},
			[][2]int{{0, 0}, {0, 3}, {4, 9}, {0, 0}},
			"hello",
		},
		{
			"bpe.json", "lower low",
			[]int{1, 15, 12, 20},
			[]string{"<bos>", "lower</w>", "low</w>", "<eos>"},
			[][2]int{{0, 0}, {0, 5}, {6, 9}, {0, 0}},
			"lower low",
		},
		{
			"bpe.json", "LOW lol lower",
			[]int{1, 12, 21, 15, 20},
			[]string{"<bos>", "low</w>", "lol", "lower</w>", "<eos>"},
			[][2]int{{0, 0}, {0, 3}, {4, 7}, {8, 13}, {0, 0}},
			"low lollower",
		},
		{
			"bpe.json", "ＬＯＷ  12!",
			[]int{1, 12, 16, 17, 18, 20},
			[]string{"<bos>", "low</w>", "1</w>", "2</w>", "!</w>", "<eos>"},
			[][2]int{{0, 0}, {0, 9}, {11, 12}, {12, 13}, {13, 14}, {0, 0}},
			"low 1 2 !",
		},
		{
			"bpe.json", "lolo lol",
			[]int{1, 11, 2, 8, 21, 20},
			[]string{"<bos>", "lo", "l", "o</w>", "lol", "<eos>"},
			[][2]int{{0, 0}, {0, 2}, {2, 3}, {3, 4}, {5, 8}, {0, 0}},
			"lolo lol",
		},
		{
			"bpe.json", "lowx",
			[]int{1, 14, 0, 20},
			[]string{"<bos>", "low", "<unk>", "<eos>"},
			[][2]int{{0, 0}, {0, 3}, {3, 4}, {0, 0}},
			"low",
		},
		{
			"metaspace.json", "hi there",
			[]int{1, 17, 10, 18, 19, 14},
			[]string{"<s>", "▁hi", "▁", "th", "er", "e"},
			[][2]int{{0, 0}, {0, 2}, {2, 3}, {3, 5}, {5, 7}, {7, 8}},
			"hi there",
		},
		{
			"metaspace.json", "€ 😀",
			[]int{1, 10, 3, 4, 5, 10, 6, 7, 8, 9},
			[]string{"<s>", "▁", "<0xE2>", "<0x82>", "<0xAC>", "▁", "<0xF0>", "<0x9F>", "<0x98>", "<0x80>"},
			[][2]int{{0, 0}, {0, 3}, {0, 3}, {0, 3}, {0, 3}, {3, 4}, {4, 8}, {4, 8}, {4, 8}, {4, 8}},
			"€ 😀",
		},
		{
			"metaspace.json", "hi<s>there",
			[]int{1, 17, 1, 18, 19, 14},
			[]string{"<s>", "▁hi", "<s>", "th", "er", "e"},
			[][2]int{{0, 0}, {0, 2}, {2, 5}, {5, 7}, {7, 9}, {9, 10}},
			"hithere",
		},
		{
			"unigram.json", "the hugging face",
			[]int{4, 6, 10, 1},
			[]string{"▁the", "▁hugging", "▁face", "</s>"},
			[][2]int{{0, 3}, {4, 11}, {12, 16}, {0, 0}},
			"the hugging face",
		},
		{
			"unigram.json", "  hugs   faces ",
			[]int{5, 13, 10, 13, 1},
			[]string{"▁hug", "s", "▁face", "s", "</s>"},
			[][2]int{{2, 5}, {5, 6}, {9, 13}, {13, 14}, {0, 0}},
			"hugs faces",
		},
		{
			"unigram.json", "hugz",
			[]int{5, 2, 1},
			[]string{"▁hug", "z", "</s>"},
			[][2]int{{0, 3}, {3, 4}, {0, 0}},
			"hug",
		},
	}
	for _, tt := range tests {
		tok := load(t, tt.file)
		enc := tok.Encode(tt.in, true)
		if !reflect.DeepEqual(enc.IDs, tt.ids) {
			t.Errorf("%s: Encode(%q) ids = %v, want %v", tt.file, tt.in, enc.IDs, tt.ids)
		}
		if !reflect.DeepEqual(enc.Offsets, tt.offsets) {
			t.Errorf("%s: Encode(%q) offsets = %v, want %v", tt.file, tt.in, enc.Offsets, tt.offsets)
		}
		if !reflect.DeepEqual(enc.Tokens, tt.tokens) {
			t.Errorf("%s: Encode(%q) tokens = %q, want %q", tt.file, tt.in, enc.Tokens, tt.tokens)
		}
		if got := tok.Decode(enc.IDs, true); got != tt.decoded {
			t.Errorf("%s: Decode(%v) = %q, want %q", tt.file, enc.IDs, got, tt.decoded)
		}
	}
}

func TestGPT2(t *testing.T) {
	tok := load(t, "gpt2.json")
	tests := []struct {
		in  string
		ids []int
	}{
		{"Hello world", []int{15496, 995}},
		{"Hello, world!", []int{15496, 11, 995, 0}},
		{" leading space", []int{3756, 2272}},
		{"I'm   here", []int{40, 1101, 220, 220, 994}},
		{"<|endoftext|>", []int{50256}},
		{"日本", []int{33768, 98, 17312, 105}},
	}
	for _, tt := range tests {
		enc := tok.Encode(tt.in, true)
		if !reflect.DeepEqual(enc.IDs, tt.ids) {
			t.Errorf("Encode(%q) = %v, want %v", tt.in, enc.IDs, tt.ids)
		}
		if got := tok.Decode(enc.IDs, false); got != tt.in {
			t.Errorf("Decode(%v) = %q, want %q", enc.IDs, got, tt.in)
		}
	}
	if n := tok.VocabSize(); n != 50257 {
		t.Errorf("VocabSize() = %d, want 50257", n)
	}
}

func TestSpecialTokens(t *testing.T) {
	tok := load(t, "metaspace.json")
	ids := tok.Encode("hi", true).IDs
	if got := tok.Encode("hi", false).IDs; !reflect.DeepEqual(got, ids[1:]) {
		t.Errorf("Encode without special = %v, want %v", got, ids[1:])
	}
	if got := tok.Decode(ids, false); got != "<s> hi" {
		t.Errorf("Decode keeping special = %q", got)
	}
}

func TestVocab(t *testing.T) {
	tok := load(t, "wordpiece.json")
	if id, ok := tok.TokenToID("hello"); !ok || id != 5 {
		t.Errorf("TokenToID(hello) = %d, %v", id, ok)
	}
	if id, ok := tok.TokenToID("[MASK]"); !ok || id != 4 {
		t.Errorf("TokenToID([MASK]) = %d, %v", id, ok)
	}
	if _, ok := tok.TokenToID("missing"); ok {
		t.Error("TokenToID(missing) found")
	}
	if s, ok := tok.IDToToken(19); !ok || s != "##izer" {
		t.Errorf("IDToToken(19) = %q, %v", s, ok)
	}
	if _, ok := tok.IDToToken(1000); ok {
		t.Error("IDToToken(1000) found")
	}
	if n := tok.VocabSize(); n != 20 {
		t.Errorf("VocabSize() = %d, want 20", n)
	}
}

func TestFromJSONErrors(t *testing.T) {
	tests := []string{
		`not json`,
		`{"model": {"type": "Nope", "vocab": {}}}`,
		`{"model": {"type": "WordLevel", "vocab": {}}, "normalizer": {"type": "Nope"}}`,
		`{"model": {"type": "WordLevel", "vocab": {}}, "pre_tokenizer": {"type": "Split", "pattern": {"Regex": "a(?=b)"}, "behavior": "Isolated"}}`,
		`{"model": {"type": "BPE", "vocab": {"a": 0}, "merges": ["a b"]}}`,
		`{"model": {"type": "BPE", "vocab": {"a": 0}, "merges": [1]}}`,
		`{"model": {"type": "Unigram", "unk_id": 5, "vocab": [["a", -1.0]]}}`,
		`{"model": {"type": "WordLevel", "vocab": {}}, "post_processor": {"type": "TemplateProcessing", "single": [{"SpecialToken": {"id": "<s>"}}], "special_tokens": {}}}`,
		`{"model": {"type": "WordLevel", "vocab": {}}, "decoder": {"type": "CTC"}}`,
	}
	for _, in := range tests {
		if _, err := FromJSON([]byte(in)); err == nil {
			t.Errorf("FromJSON(%s) succeeded, want error", in)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	tok := load(b, "gpt2.json")
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. It's 2026, and Go tokenizes text! ", 50)
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for range b.N {
		tok.Encode(text, true)
	}
}
