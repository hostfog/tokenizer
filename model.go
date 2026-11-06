package tokenizer

import "unicode/utf8"

// token is a model output with offsets relative to the piece it came from.
type token struct {
	id         int
	value      string
	start, end int
}

type model interface {
	tokenize(s string) []token
	tokenToID(s string) (int, bool)
	idToToken(id int) (string, bool)
	vocabSize() int
}

type vocab struct {
	ids    map[string]int
	tokens map[int]string
}

func newVocab(ids map[string]int) vocab {
	tokens := make(map[int]string, len(ids))
	for s, id := range ids {
		tokens[id] = s
	}
	return vocab{ids: ids, tokens: tokens}
}

func (v vocab) tokenToID(s string) (int, bool) {
	id, ok := v.ids[s]
	return id, ok
}

func (v vocab) idToToken(id int) (string, bool) {
	s, ok := v.tokens[id]
	return s, ok
}

func (v vocab) vocabSize() int { return len(v.ids) }

// byteTokens returns the <0xXX> byte fallback tokens for s, if all exist.
func byteTokens(ids map[string]int, s string) ([]int, bool) {
	out := make([]int, len(s))
	for i := 0; i < len(s); i++ {
		id, ok := ids[byteToken(s[i])]
		if !ok {
			return nil, false
		}
		out[i] = id
	}
	return out, true
}

type wordLevel struct {
	vocab
	unk string
}

func (m *wordLevel) tokenize(s string) []token {
	if id, ok := m.ids[s]; ok {
		return []token{{id, s, 0, len(s)}}
	}
	if id, ok := m.ids[m.unk]; ok {
		return []token{{id, m.unk, 0, len(s)}}
	}
	return nil
}

type wordPiece struct {
	vocab
	unk      string
	prefix   string
	maxChars int
}

func (m *wordPiece) tokenize(s string) []token {
	unk := func() []token {
		if id, ok := m.ids[m.unk]; ok {
			return []token{{id, m.unk, 0, len(s)}}
		}
		return nil
	}
	if utf8.RuneCountInString(s) > m.maxChars {
		return unk()
	}
	var out []token
	for start := 0; start < len(s); {
		found := false
		for end := len(s); end > start; {
			sub := s[start:end]
			if start > 0 {
				sub = m.prefix + sub
			}
			if id, ok := m.ids[sub]; ok {
				out = append(out, token{id, sub, start, end})
				start, found = end, true
				break
			}
			_, size := utf8.DecodeLastRuneInString(s[start:end])
			end -= size
		}
		if !found {
			return unk()
		}
	}
	return out
}
