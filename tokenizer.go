// Package tokenizer loads Hugging Face tokenizer.json files and runs them in
// pure Go, without Python, Rust or cgo.
package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Tokenizer encodes text to token IDs and decodes them back. It is safe for
// concurrent use.
type Tokenizer struct {
	model        model
	normalizer   normalizer
	preTokenizer preTokenizer
	post         postProcessor
	decoder      decoder

	added   map[int]*addedToken
	addedID map[string]int
	raw     matcher // added tokens matched against the original text
	norm    matcher // added tokens matched after normalization
}

// Encoding is the result of Encode. Offsets are byte ranges in the input
// text; tokens inserted by the post-processor have offsets [0, 0].
type Encoding struct {
	IDs     []int
	Tokens  []string
	Offsets [][2]int
}

func (e *Encoding) add(id int, tok string, sp span) {
	e.IDs = append(e.IDs, id)
	e.Tokens = append(e.Tokens, tok)
	e.Offsets = append(e.Offsets, [2]int{sp.start, sp.end})
}

// Load reads a tokenizer.json file.
func Load(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromJSON(data)
}

// FromJSON parses the contents of a tokenizer.json file.
func FromJSON(data []byte) (*Tokenizer, error) {
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}

	t := &Tokenizer{
		added:   make(map[int]*addedToken, len(c.AddedTokens)),
		addedID: make(map[string]int, len(c.AddedTokens)),
	}
	var err error
	if t.model, err = buildModel(c.Model); err != nil {
		return nil, err
	}
	if !isNull(c.Normalizer) {
		if t.normalizer, err = buildNormalizer(c.Normalizer); err != nil {
			return nil, err
		}
	}
	if !isNull(c.PreTokenizer) {
		if t.preTokenizer, err = buildPreTokenizer(c.PreTokenizer); err != nil {
			return nil, err
		}
	}
	if !isNull(c.PostProcessor) {
		if t.post, err = buildPostProcessor(c.PostProcessor); err != nil {
			return nil, err
		}
	}
	if !isNull(c.Decoder) {
		if t.decoder, err = buildDecoder(c.Decoder); err != nil {
			return nil, err
		}
	}

	var raw, norm []matchEntry
	for i := range c.AddedTokens {
		tok := &c.AddedTokens[i]
		t.added[tok.ID] = tok
		t.addedID[tok.Content] = tok.ID
		if !tok.Normalized {
			raw = append(raw, matchEntry{tok.Content, tok})
			continue
		}
		content := tok.Content
		if t.normalizer != nil {
			content = t.normalizer.normalize(newNormalized(content, 0)).s
		}
		norm = append(norm, matchEntry{content, tok})
	}
	t.raw, t.norm = newMatcher(raw), newMatcher(norm)
	return t, nil
}

// Encode tokenizes text. When addSpecial is true the post-processor adds the
// model's special tokens, such as a beginning-of-sequence token.
func (t *Tokenizer) Encode(text string, addSpecial bool) Encoding {
	size := len(text)/3 + 4
	enc := Encoding{
		IDs:     make([]int, 0, size),
		Tokens:  make([]string, 0, size),
		Offsets: make([][2]int, 0, size),
	}
	for _, seg := range t.raw.split(text) {
		if seg.tok != nil {
			enc.add(seg.tok.ID, text[seg.start:seg.end], span{seg.start, seg.end})
			continue
		}
		n := newNormalized(text[seg.start:seg.end], seg.start)
		if t.normalizer != nil {
			n = t.normalizer.normalize(n)
		}
		for _, sub := range t.norm.split(n.s) {
			if sub.tok != nil {
				enc.add(sub.tok.ID, n.s[sub.start:sub.end], n.original(sub.start, sub.end))
				continue
			}
			t.encodePiece(&enc, n.slice(sub.start, sub.end))
		}
	}
	if t.post != nil {
		enc = t.post.process(enc, addSpecial)
	}
	return enc
}

func (t *Tokenizer) encodePiece(enc *Encoding, n normalized) {
	pieces := []normalized{n}
	if t.preTokenizer != nil {
		pieces = t.preTokenizer.preTokenize(pieces)
	}
	for _, p := range pieces {
		for _, tok := range t.model.tokenize(p.s) {
			enc.add(tok.id, tok.value, p.original(tok.start, tok.end))
		}
	}
}

// Decode turns token IDs back into text. Unknown IDs are ignored. When
// skipSpecial is true, special tokens are left out.
func (t *Tokenizer) Decode(ids []int, skipSpecial bool) string {
	tokens := make([]string, 0, len(ids))
	for _, id := range ids {
		if tok, ok := t.added[id]; ok && tok.Special && skipSpecial {
			continue
		}
		if s, ok := t.IDToToken(id); ok {
			tokens = append(tokens, s)
		}
	}
	if t.decoder == nil {
		return strings.Join(tokens, " ")
	}
	return strings.Join(t.decoder.decode(tokens), "")
}

// TokenToID returns the ID of a token, including added tokens.
func (t *Tokenizer) TokenToID(token string) (int, bool) {
	if id, ok := t.addedID[token]; ok {
		return id, true
	}
	return t.model.tokenToID(token)
}

// IDToToken returns the token for an ID, including added tokens.
func (t *Tokenizer) IDToToken(id int) (string, bool) {
	if tok, ok := t.added[id]; ok {
		return tok.Content, true
	}
	return t.model.idToToken(id)
}

// VocabSize returns the number of tokens, including added tokens that are
// not part of the model vocabulary.
func (t *Tokenizer) VocabSize() int {
	n := t.model.vocabSize()
	for id := range t.added {
		if _, ok := t.model.idToToken(id); !ok {
			n++
		}
	}
	return n
}
