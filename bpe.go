package tokenizer

import (
	"container/heap"
	"fmt"
	"sync"
	"unicode/utf8"
)

const cacheSize = 10000

type merge struct{ rank, id int }

type bpe struct {
	vocab
	merges       map[[2]int]merge
	unk          string
	prefix       string
	suffix       string
	fuseUnk      bool
	byteFallback bool
	ignoreMerges bool

	mu    sync.RWMutex
	cache map[string][]token
}

func (m *bpe) addMerge(rank int, a, b string) error {
	ia, ok1 := m.ids[a]
	ib, ok2 := m.ids[b]
	if !ok1 || !ok2 {
		return fmt.Errorf("tokenizer: merge %q %q references unknown token", a, b)
	}
	merged := a + b
	if m.prefix != "" && len(b) >= len(m.prefix) && b[:len(m.prefix)] == m.prefix {
		merged = a + b[len(m.prefix):]
	}
	id, ok := m.ids[merged]
	if !ok {
		return fmt.Errorf("tokenizer: merge result %q not in vocabulary", merged)
	}
	m.merges[[2]int{ia, ib}] = merge{rank, id}
	return nil
}

// symbol is one node of the doubly linked list a word is merged on.
type symbol struct {
	id         int
	prev, next int
	start, end int
}

func (m *bpe) tokenize(s string) []token {
	if s == "" {
		return nil
	}
	if m.ignoreMerges {
		if id, ok := m.ids[s]; ok {
			return []token{{id, s, 0, len(s)}}
		}
	}

	m.mu.RLock()
	out, ok := m.cache[s]
	m.mu.RUnlock()
	if ok {
		return out
	}

	syms := m.symbols(s)
	m.mergeAll(syms)

	out = make([]token, 0, len(syms))
	for i := 0; i < len(syms); i = syms[i].next {
		sym := syms[i]
		tok, _ := m.idToToken(sym.id)
		out = append(out, token{sym.id, tok, sym.start, sym.end})
		if sym.next < 0 {
			break
		}
	}

	m.mu.Lock()
	if len(m.cache) < cacheSize {
		m.cache[s] = out
	}
	m.mu.Unlock()
	return out
}

func (m *bpe) symbols(s string) []symbol {
	syms := make([]symbol, 0, len(s))
	unk, hasUnk := m.ids[m.unk]
	push := func(id, start, end int) {
		syms = append(syms, symbol{id: id, prev: len(syms) - 1, next: len(syms) + 1, start: start, end: end})
	}

	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		end := i + size
		ch := s[i:end]
		if i > 0 && m.prefix != "" {
			ch = m.prefix + ch
		}
		if end == len(s) && m.suffix != "" {
			ch += m.suffix
		}

		if id, ok := m.ids[ch]; ok {
			push(id, i, end)
		} else if ids, ok := byteTokens(m.ids, s[i:end]); m.byteFallback && ok {
			for k, id := range ids {
				push(id, i+k, i+k+1)
			}
		} else if hasUnk {
			if n := len(syms); m.fuseUnk && n > 0 && syms[n-1].id == unk {
				syms[n-1].end = end
			} else {
				push(unk, i, end)
			}
		}
		i = end
	}
	if len(syms) > 0 {
		syms[len(syms)-1].next = -1
	}
	return syms
}

func byteToken(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{'<', '0', 'x', hex[b>>4], hex[b&15], '>'})
}

type candidate struct {
	pos, rank, id int
}

type queue []candidate

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].rank != q[j].rank {
		return q[i].rank < q[j].rank
	}
	return q[i].pos < q[j].pos
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(candidate)) }
func (q *queue) Pop() any {
	old := *q
	c := old[len(old)-1]
	*q = old[:len(old)-1]
	return c
}

// mergeAll applies merges in rank order, leftmost first, until none apply.
func (m *bpe) mergeAll(syms []symbol) {
	q := make(queue, 0, len(syms))
	for i := 0; i+1 < len(syms); i++ {
		if mg, ok := m.merges[[2]int{syms[i].id, syms[i+1].id}]; ok {
			q = append(q, candidate{i, mg.rank, mg.id})
		}
	}
	heap.Init(&q)

	for q.Len() > 0 {
		c := heap.Pop(&q).(candidate)
		left := &syms[c.pos]
		if left.end == left.start || left.next < 0 {
			continue
		}
		right := &syms[left.next]
		if mg, ok := m.merges[[2]int{left.id, right.id}]; !ok || mg.id != c.id {
			continue
		}

		left.id = c.id
		left.end = right.end
		left.next = right.next
		right.start, right.end = 0, 0
		if left.next >= 0 {
			syms[left.next].prev = c.pos
		}

		if left.prev >= 0 {
			if mg, ok := m.merges[[2]int{syms[left.prev].id, left.id}]; ok {
				heap.Push(&q, candidate{left.prev, mg.rank, mg.id})
			}
		}
		if left.next >= 0 {
			if mg, ok := m.merges[[2]int{left.id, syms[left.next].id}]; ok {
				heap.Push(&q, candidate{c.pos, mg.rank, mg.id})
			}
		}
	}
}
