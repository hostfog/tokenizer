package tokenizer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type preTokenizer interface {
	preTokenize(pieces []normalized) []normalized
}

func eachPiece(pieces []normalized, f func(n normalized) []normalized) []normalized {
	out := make([]normalized, 0, len(pieces))
	for _, p := range pieces {
		out = append(out, f(p)...)
	}
	return out
}

type preTokenizerSeq []preTokenizer

func (seq preTokenizerSeq) preTokenize(pieces []normalized) []normalized {
	for _, p := range seq {
		pieces = p.preTokenize(pieces)
	}
	return pieces
}

type splitter struct {
	find     finder
	behavior behavior
	invert   bool
}

func (s splitter) preTokenize(pieces []normalized) []normalized {
	return eachPiece(pieces, func(n normalized) []normalized {
		return n.split(s.find(n.s), s.behavior, s.invert)
	})
}

// runeFinder matches every rune for which f is true, one match per rune.
func runeFinder(f func(r rune) bool) finder {
	return func(s string) []span {
		var out []span
		for i, r := range s {
			if f(r) {
				out = append(out, span{i, i + utf8.RuneLen(r)})
			}
		}
		return out
	}
}

func whitespaceSplit() preTokenizer {
	return splitter{find: runeFinder(unicode.IsSpace), behavior: removed}
}

func punctuation(b behavior) preTokenizer {
	return splitter{find: runeFinder(isPunct), behavior: b}
}

func digits(individual bool) preTokenizer {
	if individual {
		return splitter{find: runeFinder(unicode.IsNumber), behavior: isolated}
	}
	return splitter{find: runeFinder(unicode.IsNumber), behavior: contiguous}
}

func charDelimiter(c rune) preTokenizer {
	return splitter{find: runeFinder(func(r rune) bool { return r == c }), behavior: removed}
}

func bertPreTokenizer() preTokenizer {
	return preTokenizerSeq{whitespaceSplit(), punctuation(isolated)}
}

// whitespace keeps runs of word characters and runs of other non-space
// characters, like the regex \w+|[^\w\s]+.
type whitespace struct{}

func (whitespace) preTokenize(pieces []normalized) []normalized {
	return eachPiece(pieces, func(n normalized) []normalized {
		var out []normalized
		start, class := 0, 0
		for i, r := range n.s {
			c := 2
			switch {
			case unicode.IsSpace(r):
				c = 0
			case isWord(r):
				c = 1
			}
			if c != class {
				if class != 0 && start < i {
					out = append(out, n.slice(start, i))
				}
				start, class = i, c
			}
		}
		if class != 0 && start < len(n.s) {
			out = append(out, n.slice(start, len(n.s)))
		}
		return out
	})
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.M, r) || unicode.Is(unicode.Nd, r) ||
		unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Pc, r) || r == 0x200C || r == 0x200D
}

func isPunct(r rune) bool {
	return r < utf8.RuneSelf && unicode.IsPrint(r) && !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' ||
		unicode.IsPunct(r)
}

type metaspace struct {
	replacement string
	prepend     string // "always", "first" or "never"
	split       bool
}

func (m metaspace) preTokenize(pieces []normalized) []normalized {
	return eachPiece(pieces, func(n normalized) []normalized {
		n = n.replaceAll(literalFinder(" ")(n.s), m.replacement)
		if n.s != "" && !strings.HasPrefix(n.s, m.replacement) &&
			(m.prepend == "always" || m.prepend == "first" && n.align[0].start == 0) {
			n = n.prepend(m.replacement)
		}
		if !m.split {
			return []normalized{n}
		}
		return n.split(literalFinder(m.replacement)(n.s), mergedWithNext, false)
	})
}

type byteLevel struct {
	addPrefixSpace bool
	useRegex       bool
}

func (b byteLevel) preTokenize(pieces []normalized) []normalized {
	if b.addPrefixSpace {
		pieces = eachPiece(pieces, func(n normalized) []normalized {
			if strings.HasPrefix(n.s, " ") {
				return []normalized{n}
			}
			return []normalized{n.prepend(" ")}
		})
	}
	if b.useRegex {
		pieces = splitter{find: scannerFinder(gpt2Next), behavior: isolated}.preTokenize(pieces)
	}
	return toByteLevel(pieces)
}

// toByteLevel maps every byte to its GPT-2 rune, building all pieces in one
// buffer.
func toByteLevel(pieces []normalized) []normalized {
	size := 0
	for _, n := range pieces {
		size += 2 * len(n.s)
	}
	b := newBuilder(size)
	ends := make([]int, len(pieces))
	for k, n := range pieces {
		for i := 0; i < len(n.s); i++ {
			b.add(byteToRune[n.s[i]], n.align[i])
		}
		ends[k] = len(b.align)
	}
	all := b.normalized()
	start := 0
	for k, end := range ends {
		pieces[k] = all.slice(start, end)
		start = end
	}
	return pieces
}

// byteToRune is the GPT-2 mapping from bytes to printable runes.
var (
	byteToRune [256]string
	runeToByte = make(map[rune]byte, 256)
)

func init() {
	n := 0
	for b := 0; b < 256; b++ {
		r := rune(b)
		if !(b >= '!' && b <= '~' || b >= 0xA1 && b <= 0xAC || b >= 0xAE) {
			r = rune(256 + n)
			n++
		}
		byteToRune[b] = string(r)
		runeToByte[r] = byte(b)
	}
}
