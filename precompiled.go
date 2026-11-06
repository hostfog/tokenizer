package tokenizer

import (
	"encoding/binary"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// precompiled is the SentencePiece normalizer: a double-array trie that maps
// byte sequences to replacement strings.
type precompiled struct {
	trie       []uint32
	normalized []byte
}

func newPrecompiled(data []byte) (*precompiled, error) {
	if len(data) < 4 {
		return nil, errors.New("tokenizer: precompiled charsmap too short")
	}
	size := int(binary.LittleEndian.Uint32(data))
	if size%4 != 0 || 4+size > len(data) {
		return nil, errors.New("tokenizer: invalid precompiled charsmap")
	}
	trie := make([]uint32, size/4)
	for i := range trie {
		trie[i] = binary.LittleEndian.Uint32(data[4+4*i:])
	}
	return &precompiled{trie: trie, normalized: data[4+size:]}, nil
}

// lookup returns the replacement for the shortest prefix of key in the trie.
func (p *precompiled) lookup(key string) (string, bool) {
	if len(p.trie) == 0 {
		return "", false
	}
	pos := offset(p.trie[0])
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == 0 {
			break
		}
		pos ^= int(c)
		if pos >= len(p.trie) {
			return "", false
		}
		unit := p.trie[pos]
		if unit&(1<<31|0xFF) != uint32(c) {
			return "", false
		}
		pos ^= offset(unit)
		if unit>>8&1 == 1 {
			if pos >= len(p.trie) {
				return "", false
			}
			start := int(p.trie[pos] & (1<<31 - 1))
			if start >= len(p.normalized) {
				return "", false
			}
			end := start
			for end < len(p.normalized) && p.normalized[end] != 0 {
				end++
			}
			return string(p.normalized[start:end]), true
		}
	}
	return "", false
}

func offset(unit uint32) int {
	return int((unit >> 10) << ((unit & (1 << 9)) >> 6))
}

func (p *precompiled) normalize(n normalized) normalized {
	b := newBuilder(len(n.s))
	for i := 0; i < len(n.s); {
		g := graphemeLen(n.s[i:])
		if g < 6 {
			if out, ok := p.lookup(n.s[i : i+g]); ok {
				b.addAligned(out, n, i, i+g)
				i += g
				continue
			}
		}
		for end := i + g; i < end; {
			_, size := utf8.DecodeRuneInString(n.s[i:])
			out, ok := p.lookup(n.s[i : i+size])
			if !ok {
				out = n.s[i : i+size]
			}
			b.add(out, n.original(i, i+size))
			i += size
		}
	}
	return b.normalized()
}

// graphemeLen approximates the length of the first extended grapheme cluster
// in s: a base rune followed by extending marks, with CRLF and emoji ZWJ
// sequences kept together.
func graphemeLen(s string) int {
	if strings.HasPrefix(s, "\r\n") {
		return 2
	}
	base, n := utf8.DecodeRuneInString(s)
	if unicode.IsControl(base) || base == 0x2028 || base == 0x2029 {
		return n
	}
	for n < len(s) {
		r, size := utf8.DecodeRuneInString(s[n:])
		if r == 0x200D && n+size < len(s) {
			next, nsize := utf8.DecodeRuneInString(s[n+size:])
			if isPictographic(base) && isPictographic(next) {
				n += size + nsize
				continue
			}
		}
		if !isExtend(r) {
			break
		}
		n += size
	}
	return n
}

func isExtend(r rune) bool {
	return unicode.Is(unicode.M, r) || r == 0x200C || r == 0x200D ||
		r >= 0xFE00 && r <= 0xFE0F || r >= 0x1F3FB && r <= 0x1F3FF || r >= 0xE0020 && r <= 0xE007F
}

func isPictographic(r rune) bool {
	return r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF ||
		r >= 0x2300 && r <= 0x23FF || r >= 0x2B00 && r <= 0x2BFF
}
