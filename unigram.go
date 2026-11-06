package tokenizer

import (
	"math"
	"unicode/utf8"
)

const unkPenalty = 10.0

type unigram struct {
	vocab
	scores       []float64
	unk          int
	minScore     float64
	maxLen       int
	byteFallback bool
}

func newUnigram(pieces []string, scores []float64, unk int, byteFallback bool) *unigram {
	ids := make(map[string]int, len(pieces))
	m := &unigram{scores: scores, unk: unk, minScore: math.Inf(1), byteFallback: byteFallback}
	for i, p := range pieces {
		if _, dup := ids[p]; !dup {
			ids[p] = i
		}
		m.minScore = min(m.minScore, scores[i])
		m.maxLen = max(m.maxLen, len(p))
	}
	m.vocab = newVocab(ids)
	return m
}

type lattice struct {
	id    int
	start int
	score float64
}

// tokenize finds the most likely segmentation of s with the Viterbi algorithm.
func (m *unigram) tokenize(s string) []token {
	if s == "" {
		return nil
	}
	best := make([]lattice, len(s)+1)
	for i := range best {
		best[i].start = -1
	}
	best[0].start = 0
	unkScore := m.minScore - unkPenalty

	for i := 0; i < len(s); {
		base := best[i].score
		_, size := utf8.DecodeRuneInString(s[i:])
		single := false
		for end := i + size; end <= len(s) && end-i <= m.maxLen; {
			if id, ok := m.ids[s[i:end]]; ok {
				score := base + m.scores[id]
				if best[end].start < 0 || score > best[end].score {
					best[end] = lattice{id, i, score}
				}
				if end == i+size {
					single = true
				}
			}
			if end == len(s) {
				break
			}
			_, n := utf8.DecodeRuneInString(s[end:])
			end += n
		}
		if !single {
			score := base + unkScore
			if best[i+size].start < 0 || score > best[i+size].score {
				best[i+size] = lattice{m.unk, i, score}
			}
		}
		i += size
	}

	var rev []token
	for end := len(s); end > 0; {
		node := best[end]
		if node.id == m.unk && len(rev) > 0 && rev[len(rev)-1].id == m.unk {
			rev[len(rev)-1].start = node.start
		} else {
			rev = append(rev, token{id: node.id, start: node.start, end: end})
		}
		end = node.start
	}

	out := make([]token, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		t := rev[i]
		if t.id != m.unk {
			t.value, _ = m.idToToken(t.id)
			out = append(out, t)
			continue
		}
		if ids, ok := byteTokens(m.ids, s[t.start:t.end]); m.byteFallback && ok {
			for _, id := range ids {
				tok, _ := m.idToToken(id)
				out = append(out, token{id, tok, t.start, t.end})
			}
			continue
		}
		if m.unk >= 0 {
			t.value = s[t.start:t.end]
			out = append(out, t)
		}
	}
	return out
}
