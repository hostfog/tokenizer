package tokenizer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type config struct {
	AddedTokens   []addedToken    `json:"added_tokens"`
	Normalizer    json.RawMessage `json:"normalizer"`
	PreTokenizer  json.RawMessage `json:"pre_tokenizer"`
	Model         modelConfig     `json:"model"`
	PostProcessor json.RawMessage `json:"post_processor"`
	Decoder       json.RawMessage `json:"decoder"`
}

type modelConfig struct {
	Type                    string          `json:"type"`
	Vocab                   json.RawMessage `json:"vocab"`
	Merges                  []any           `json:"merges"`
	UnkToken                string          `json:"unk_token"`
	UnkID                   *int            `json:"unk_id"`
	ContinuingSubwordPrefix string          `json:"continuing_subword_prefix"`
	EndOfWordSuffix         string          `json:"end_of_word_suffix"`
	FuseUnk                 bool            `json:"fuse_unk"`
	ByteFallback            bool            `json:"byte_fallback"`
	IgnoreMerges            bool            `json:"ignore_merges"`
	MaxInputCharsPerWord    int             `json:"max_input_chars_per_word"`
}

// component holds the union of the fields used by normalizers,
// pre-tokenizers, post-processors and decoders.
type component struct {
	Type    string       `json:"type"`
	Pattern *patternSpec `json:"pattern"`
	Content string       `json:"content"`

	Normalizers   []json.RawMessage `json:"normalizers"`
	PreTokenizers []json.RawMessage `json:"pretokenizers"`
	Processors    []json.RawMessage `json:"processors"`
	Decoders      []json.RawMessage `json:"decoders"`

	Prepend             string `json:"prepend"`
	StripLeft           bool   `json:"strip_left"`
	StripRight          bool   `json:"strip_right"`
	CleanText           bool   `json:"clean_text"`
	HandleChineseChars  bool   `json:"handle_chinese_chars"`
	StripAccents        *bool  `json:"strip_accents"`
	Lowercase           bool   `json:"lowercase"`
	PrecompiledCharsmap string `json:"precompiled_charsmap"`

	Behavior         string `json:"behavior"`
	Invert           bool   `json:"invert"`
	AddPrefixSpace   *bool  `json:"add_prefix_space"`
	UseRegex         *bool  `json:"use_regex"`
	TrimOffsets      bool   `json:"trim_offsets"`
	Replacement      string `json:"replacement"`
	PrependScheme    string `json:"prepend_scheme"`
	Split            *bool  `json:"split"`
	IndividualDigits bool   `json:"individual_digits"`
	Delimiter        string `json:"delimiter"`

	Single        []map[string]templateRef `json:"single"`
	SpecialTokens map[string]struct {
		IDs    []int    `json:"ids"`
		Tokens []string `json:"tokens"`
	} `json:"special_tokens"`
	Sep []any `json:"sep"`
	Cls []any `json:"cls"`

	Prefix  string `json:"prefix"`
	Cleanup bool   `json:"cleanup"`
	Suffix  string `json:"suffix"`
	Start   int    `json:"start"`
	Stop    int    `json:"stop"`
}

type patternSpec struct {
	String *string `json:"String"`
	Regex  *string `json:"Regex"`
}

type templateRef struct {
	ID string `json:"id"`
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

func parseComponent(raw json.RawMessage) (component, error) {
	var c component
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("tokenizer: %w", err)
	}
	return c, nil
}

func (p *patternSpec) finder() (finder, error) {
	switch {
	case p == nil:
		return nil, fmt.Errorf("tokenizer: missing pattern")
	case p.String != nil:
		return literalFinder(*p.String), nil
	case p.Regex != nil:
		return patternFinder(*p.Regex)
	}
	return nil, fmt.Errorf("tokenizer: empty pattern")
}

func buildNormalizer(raw json.RawMessage) (normalizer, error) {
	c, err := parseComponent(raw)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "Sequence":
		seq := make(normalizerSeq, 0, len(c.Normalizers))
		for _, r := range c.Normalizers {
			n, err := buildNormalizer(r)
			if err != nil {
				return nil, err
			}
			seq = append(seq, n)
		}
		return seq, nil
	case "NFC":
		return unicodeNormalizer{norm.NFC}, nil
	case "NFKC":
		return unicodeNormalizer{norm.NFKC}, nil
	case "NFD":
		return unicodeNormalizer{norm.NFD}, nil
	case "NFKD":
		return unicodeNormalizer{norm.NFKD}, nil
	case "Lowercase":
		return lowercase{}, nil
	case "StripAccents":
		return stripAccents{}, nil
	case "Strip":
		return strip{c.StripLeft, c.StripRight}, nil
	case "Prepend":
		return prepend{c.Prepend}, nil
	case "Replace":
		f, err := c.Pattern.finder()
		if err != nil {
			return nil, err
		}
		return replace{f, c.Content}, nil
	case "BertNormalizer":
		accents := c.Lowercase
		if c.StripAccents != nil {
			accents = *c.StripAccents
		}
		return bertNormalizer{c.CleanText, c.HandleChineseChars, accents, c.Lowercase}, nil
	case "Precompiled":
		data, err := base64.StdEncoding.DecodeString(c.PrecompiledCharsmap)
		if err != nil {
			return nil, fmt.Errorf("tokenizer: precompiled charsmap: %w", err)
		}
		if len(data) == 0 {
			return normalizerSeq{}, nil
		}
		return newPrecompiled(data)
	}
	return nil, fmt.Errorf("tokenizer: unsupported normalizer %q", c.Type)
}

func parseBehavior(s string) (behavior, error) {
	switch s {
	case "Removed":
		return removed, nil
	case "Isolated", "":
		return isolated, nil
	case "MergedWithPrevious":
		return mergedWithPrevious, nil
	case "MergedWithNext":
		return mergedWithNext, nil
	case "Contiguous":
		return contiguous, nil
	}
	return 0, fmt.Errorf("tokenizer: unknown split behavior %q", s)
}

func orDefault(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func buildPreTokenizer(raw json.RawMessage) (preTokenizer, error) {
	c, err := parseComponent(raw)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "Sequence":
		seq := make(preTokenizerSeq, 0, len(c.PreTokenizers))
		for _, r := range c.PreTokenizers {
			p, err := buildPreTokenizer(r)
			if err != nil {
				return nil, err
			}
			seq = append(seq, p)
		}
		return seq, nil
	case "ByteLevel":
		return byteLevel{orDefault(c.AddPrefixSpace, true), orDefault(c.UseRegex, true)}, nil
	case "Split":
		f, err := c.Pattern.finder()
		if err != nil {
			return nil, err
		}
		b, err := parseBehavior(c.Behavior)
		if err != nil {
			return nil, err
		}
		return splitter{f, b, c.Invert}, nil
	case "Whitespace":
		return whitespace{}, nil
	case "WhitespaceSplit":
		return whitespaceSplit(), nil
	case "Metaspace":
		return metaspace{
			replacement: c.Replacement,
			prepend:     prependScheme(c),
			split:       orDefault(c.Split, true),
		}, nil
	case "Punctuation":
		b, err := parseBehavior(c.Behavior)
		if err != nil {
			return nil, err
		}
		return punctuation(b), nil
	case "Digits":
		return digits(c.IndividualDigits), nil
	case "BertPreTokenizer":
		return bertPreTokenizer(), nil
	case "CharDelimiterSplit":
		r := []rune(c.Delimiter)
		if len(r) != 1 {
			return nil, fmt.Errorf("tokenizer: invalid delimiter %q", c.Delimiter)
		}
		return charDelimiter(r[0]), nil
	}
	return nil, fmt.Errorf("tokenizer: unsupported pre_tokenizer %q", c.Type)
}

func prependScheme(c component) string {
	if c.PrependScheme != "" {
		return c.PrependScheme
	}
	if orDefault(c.AddPrefixSpace, true) {
		return "always"
	}
	return "never"
}

func buildModel(c modelConfig) (model, error) {
	typ := c.Type
	if typ == "" {
		switch {
		case c.Merges != nil:
			typ = "BPE"
		case c.MaxInputCharsPerWord > 0:
			typ = "WordPiece"
		case strings.HasPrefix(strings.TrimSpace(string(c.Vocab)), "["):
			typ = "Unigram"
		default:
			typ = "WordLevel"
		}
	}

	if typ == "Unigram" {
		return buildUnigram(c)
	}

	var ids map[string]int
	if err := json.Unmarshal(c.Vocab, &ids); err != nil {
		return nil, fmt.Errorf("tokenizer: %s vocab: %w", typ, err)
	}
	v := newVocab(ids)

	switch typ {
	case "BPE":
		m := &bpe{
			vocab:        v,
			merges:       make(map[[2]int]merge, len(c.Merges)),
			unk:          c.UnkToken,
			prefix:       c.ContinuingSubwordPrefix,
			suffix:       c.EndOfWordSuffix,
			fuseUnk:      c.FuseUnk,
			byteFallback: c.ByteFallback,
			ignoreMerges: c.IgnoreMerges,
			cache:        make(map[string][]token),
		}
		for rank, raw := range c.Merges {
			a, b, err := parseMerge(raw)
			if err != nil {
				return nil, err
			}
			if err := m.addMerge(rank, a, b); err != nil {
				return nil, err
			}
		}
		return m, nil
	case "WordPiece":
		m := &wordPiece{vocab: v, unk: c.UnkToken, prefix: c.ContinuingSubwordPrefix, maxChars: c.MaxInputCharsPerWord}
		if m.maxChars == 0 {
			m.maxChars = 100
		}
		return m, nil
	case "WordLevel":
		return &wordLevel{vocab: v, unk: c.UnkToken}, nil
	}
	return nil, fmt.Errorf("tokenizer: unsupported model %q", typ)
}

func parseMerge(raw any) (string, string, error) {
	switch m := raw.(type) {
	case string:
		if a, b, ok := strings.Cut(m, " "); ok {
			return a, b, nil
		}
	case []any:
		if len(m) == 2 {
			a, ok1 := m[0].(string)
			b, ok2 := m[1].(string)
			if ok1 && ok2 {
				return a, b, nil
			}
		}
	}
	return "", "", fmt.Errorf("tokenizer: invalid merge %v", raw)
}

func buildUnigram(c modelConfig) (model, error) {
	var vocab [][2]any
	if err := json.Unmarshal(c.Vocab, &vocab); err != nil {
		return nil, fmt.Errorf("tokenizer: Unigram vocab: %w", err)
	}
	pieces := make([]string, len(vocab))
	scores := make([]float64, len(vocab))
	for i, e := range vocab {
		p, ok1 := e[0].(string)
		s, ok2 := e[1].(float64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("tokenizer: invalid Unigram entry %v", e)
		}
		pieces[i], scores[i] = p, s
	}
	unk := -1
	if c.UnkID != nil {
		unk = *c.UnkID
		if unk < 0 || unk >= len(pieces) {
			return nil, fmt.Errorf("tokenizer: unk_id %d out of range", unk)
		}
	}
	return newUnigram(pieces, scores, unk, c.ByteFallback), nil
}

func buildPostProcessor(raw json.RawMessage) (postProcessor, error) {
	c, err := parseComponent(raw)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "Sequence":
		seq := make(postProcessorSeq, 0, len(c.Processors))
		for _, r := range c.Processors {
			p, err := buildPostProcessor(r)
			if err != nil {
				return nil, err
			}
			seq = append(seq, p)
		}
		return seq, nil
	case "ByteLevel":
		if !c.TrimOffsets {
			return postProcessorSeq{}, nil
		}
		return trimOffsets{orDefault(c.AddPrefixSpace, true)}, nil
	case "TemplateProcessing":
		var t template
		for _, item := range c.Single {
			if _, ok := item["Sequence"]; ok {
				t = append(t, templatePiece{})
				continue
			}
			ref, ok := item["SpecialToken"]
			if !ok {
				return nil, fmt.Errorf("tokenizer: invalid template item %v", item)
			}
			sp, ok := c.SpecialTokens[ref.ID]
			if !ok || len(sp.IDs) == 0 || len(sp.IDs) != len(sp.Tokens) {
				return nil, fmt.Errorf("tokenizer: template special token %q missing", ref.ID)
			}
			t = append(t, templatePiece{ids: sp.IDs, tokens: sp.Tokens})
		}
		return t, nil
	case "BertProcessing", "RobertaProcessing":
		cls, err := specialPair(c.Cls)
		if err != nil {
			return nil, err
		}
		sep, err := specialPair(c.Sep)
		if err != nil {
			return nil, err
		}
		if c.Type == "RobertaProcessing" && c.TrimOffsets {
			return postProcessorSeq{trimOffsets{orDefault(c.AddPrefixSpace, true)}, wrap(cls, sep)}, nil
		}
		return wrap(cls, sep), nil
	}
	return nil, fmt.Errorf("tokenizer: unsupported post_processor %q", c.Type)
}

func specialPair(v []any) (templatePiece, error) {
	if len(v) == 2 {
		tok, ok1 := v[0].(string)
		id, ok2 := v[1].(float64)
		if ok1 && ok2 {
			return templatePiece{ids: []int{int(id)}, tokens: []string{tok}}, nil
		}
	}
	return templatePiece{}, fmt.Errorf("tokenizer: invalid special token %v", v)
}

func buildDecoder(raw json.RawMessage) (decoder, error) {
	c, err := parseComponent(raw)
	if err != nil {
		return nil, err
	}
	switch c.Type {
	case "Sequence":
		seq := make(decoderSeq, 0, len(c.Decoders))
		for _, r := range c.Decoders {
			d, err := buildDecoder(r)
			if err != nil {
				return nil, err
			}
			seq = append(seq, d)
		}
		return seq, nil
	case "ByteLevel":
		return byteLevelDecoder{}, nil
	case "Metaspace":
		return metaspaceDecoder{c.Replacement, prependScheme(c) != "never"}, nil
	case "WordPiece":
		return wordPieceDecoder{c.Prefix, c.Cleanup}, nil
	case "BPEDecoder":
		return bpeDecoder{c.Suffix}, nil
	case "ByteFallback":
		return byteFallbackDecoder{}, nil
	case "Fuse":
		return fuseDecoder{}, nil
	case "Strip":
		return stripDecoder{c.Content, c.Start, c.Stop}, nil
	case "Replace":
		f, err := c.Pattern.finder()
		if err != nil {
			return nil, err
		}
		return replaceDecoder{f, c.Content}, nil
	}
	return nil, fmt.Errorf("tokenizer: unsupported decoder %q", c.Type)
}
