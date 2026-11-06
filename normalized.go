package tokenizer

import (
	"strings"
	"unicode/utf8"
)

// span is a byte range in the original input.
type span struct{ start, end int }

// normalized is a string together with, for each of its bytes, the span of
// the original input it came from. It lets offsets survive normalization and
// pre-tokenization.
type normalized struct {
	s     string
	align []span
}

func newNormalized(s string, offset int) normalized {
	align := make([]span, len(s))
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		for j := i; j < i+size; j++ {
			align[j] = span{offset + i, offset + i + size}
		}
		i += size
	}
	return normalized{s: s, align: align}
}

// original returns the original span covered by s[i:j].
func (n normalized) original(i, j int) span {
	if len(n.align) == 0 {
		return span{}
	}
	if i >= len(n.align) {
		e := n.align[len(n.align)-1].end
		return span{e, e}
	}
	if j <= i {
		s := n.align[i].start
		return span{s, s}
	}
	return span{n.align[i].start, n.align[j-1].end}
}

func (n normalized) slice(i, j int) normalized {
	return normalized{s: n.s[i:j], align: n.align[i:j]}
}

// builder assembles a new normalized string piece by piece.
type builder struct {
	sb    strings.Builder
	align []span
}

func newBuilder(size int) *builder {
	b := &builder{align: make([]span, 0, size)}
	b.sb.Grow(size)
	return b
}

func (b *builder) add(s string, sp span) {
	b.sb.WriteString(s)
	for range len(s) {
		b.align = append(b.align, sp)
	}
}

// addAligned appends out as the replacement of n.s[i:j]. The k-th rune of out
// is aligned with the k-th rune of the source, or with the last one when out
// is longer.
func (b *builder) addAligned(out string, n normalized, i, j int) {
	for k := 0; k < len(out); {
		_, size := utf8.DecodeRuneInString(out[k:])
		_, src := utf8.DecodeRuneInString(n.s[i:j])
		b.add(out[k:k+size], n.original(i, i+src))
		if i+src < j {
			i += src
		}
		k += size
	}
}

func (b *builder) normalized() normalized {
	return normalized{s: b.sb.String(), align: b.align}
}

// mapRunes rebuilds n by replacing every rune with f(r).
func (n normalized) mapRunes(f func(r rune) string) normalized {
	b := newBuilder(len(n.s))
	for i := 0; i < len(n.s); {
		r, size := utf8.DecodeRuneInString(n.s[i:])
		b.add(f(r), n.original(i, i+size))
		i += size
	}
	return b.normalized()
}

func (n normalized) prepend(s string) normalized {
	if n.s == "" {
		return n
	}
	_, size := utf8.DecodeRuneInString(n.s)
	b := newBuilder(len(s) + len(n.s))
	b.add(s, n.original(0, size))
	b.sb.WriteString(n.s)
	b.align = append(b.align, n.align...)
	return b.normalized()
}

// replaceAll replaces every non-overlapping match with repl.
func (n normalized) replaceAll(matches []span, repl string) normalized {
	if len(matches) == 0 {
		return n
	}
	b := newBuilder(len(n.s))
	last := 0
	for _, m := range matches {
		b.sb.WriteString(n.s[last:m.start])
		b.align = append(b.align, n.align[last:m.start]...)
		b.add(repl, n.original(m.start, m.end))
		last = m.end
	}
	b.sb.WriteString(n.s[last:])
	b.align = append(b.align, n.align[last:]...)
	return b.normalized()
}

type behavior int

const (
	removed behavior = iota
	isolated
	mergedWithPrevious
	mergedWithNext
	contiguous
)

// split cuts n around matches according to the delimiter behavior, the same
// way the Hugging Face NormalizedString.split does. Empty pieces are dropped.
func (n normalized) split(matches []span, b behavior, invert bool) []normalized {
	type part struct {
		span
		match bool
	}
	parts := make([]part, 0, 2*len(matches)+1)
	prev := 0
	for _, m := range matches {
		if prev < m.start {
			parts = append(parts, part{span{prev, m.start}, invert})
		}
		parts = append(parts, part{m, !invert})
		prev = m.end
	}
	if prev < len(n.s) {
		parts = append(parts, part{span{prev, len(n.s)}, invert})
	}

	var out []span
	switch b {
	case removed:
		for _, p := range parts {
			if !p.match {
				out = append(out, p.span)
			}
		}
	case isolated:
		for _, p := range parts {
			out = append(out, p.span)
		}
	case mergedWithPrevious:
		prevMatch := false
		for _, p := range parts {
			if p.match && !prevMatch && len(out) > 0 {
				out[len(out)-1].end = p.end
			} else {
				out = append(out, p.span)
			}
			prevMatch = p.match
		}
	case mergedWithNext:
		prevMatch := false
		for i := len(parts) - 1; i >= 0; i-- {
			p := parts[i]
			if p.match && !prevMatch && len(out) > 0 {
				out[len(out)-1].start = p.start
			} else {
				out = append(out, p.span)
			}
			prevMatch = p.match
		}
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	case contiguous:
		for i, p := range parts {
			if i > 0 && p.match == parts[i-1].match {
				out[len(out)-1].end = p.end
			} else {
				out = append(out, p.span)
			}
		}
	}

	pieces := make([]normalized, 0, len(out))
	for _, sp := range out {
		if sp.start < sp.end {
			pieces = append(pieces, n.slice(sp.start, sp.end))
		}
	}
	return pieces
}
