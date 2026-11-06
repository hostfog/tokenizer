# tokenizer

Pure-Go runner for Hugging Face `tokenizer.json` files. Tokenize for GPT-2, Llama, Qwen, Mistral, BERT, T5 and similar models without Python, Rust or cgo.

```sh
go get github.com/hostfog/tokenizer
```

## Usage

```go
tok, err := tokenizer.Load("tokenizer.json")
if err != nil {
	log.Fatal(err)
}

enc := tok.Encode("Hello world", true)
fmt.Println(enc.IDs)     // [15496 995]
fmt.Println(enc.Tokens)  // [Hello Ġworld]
fmt.Println(enc.Offsets) // [[0 5] [5 11]]  byte ranges in the input

text := tok.Decode(enc.IDs, true) // "Hello world"

id, ok := tok.TokenToID("<|endoftext|>")
```

A `Tokenizer` is safe for concurrent use. Unsupported components make `Load` return an error rather than tokenize differently.

## Supported components

| Stage          | Types |
|----------------|-------|
| Model          | `BPE` (byte fallback, `ignore_merges`, subword prefix/suffix, string or array merges), `WordPiece`, `Unigram`, `WordLevel` |
| Normalizer     | `NFC`, `NFD`, `NFKC`, `NFKD`, `Lowercase`, `StripAccents`, `Strip`, `Replace`, `Prepend`, `BertNormalizer`, `Precompiled`, `Sequence` |
| Pre-tokenizer  | `ByteLevel`, `Split`, `Whitespace`, `WhitespaceSplit`, `Metaspace`, `Punctuation`, `Digits`, `BertPreTokenizer`, `CharDelimiterSplit`, `Sequence` |
| Post-processor | `TemplateProcessing`, `ByteLevel`, `BertProcessing`, `RobertaProcessing`, `Sequence` |
| Decoder        | `ByteLevel`, `Metaspace`, `WordPiece`, `BPEDecoder`, `ByteFallback`, `Fuse`, `Strip`, `Replace`, `Sequence` |

Added tokens (special and not) are matched before pre-tokenization and respect `lstrip`, `rstrip`, `single_word` and `normalized`.

Output was checked token-for-token, including offsets and decoding, against the Python `tokenizers` library for GPT-2, RoBERTa, BERT, Llama 2, Llama 3, Mistral, Qwen 2.5 and T5.

## Limitations

- Go's `regexp` has no lookaround. The GPT-2, Llama 3 and Qwen 2 split patterns run on hand-written scanners that give identical results; other patterns use `regexp` with `\s`, `\d` and `\w` widened to Unicode, and patterns that need lookahead (such as GPT-4o's) fail to load.
- Single sequences only: no pair encoding, truncation, padding, type IDs or attention masks.
- `Precompiled` uses an approximation of Unicode grapheme clusters, so rare scripts may normalize differently from SentencePiece.
- BPE dropout is ignored, and `UnicodeScripts`, `CTC` and a few other rarely used components are not implemented.

## License

MIT. `testdata/gpt2.json` is OpenAI's GPT-2 tokenizer, used only as a test fixture under its own licence.
