package tokenizer

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type normalizer interface {
	normalize(n normalized) normalized
}

type normalizerSeq []normalizer

func (seq normalizerSeq) normalize(n normalized) normalized {
	for _, x := range seq {
		n = x.normalize(n)
	}
	return n
}

type unicodeNormalizer struct{ form norm.Form }

func (u unicodeNormalizer) normalize(n normalized) normalized {
	if u.form.IsNormalString(n.s) {
		return n
	}
	b := newBuilder(len(n.s))
	for i := 0; i < len(n.s); {
		k := u.form.NextBoundaryInString(n.s[i:], true)
		if k <= 0 {
			k = len(n.s) - i
		}
		b.addAligned(u.form.String(n.s[i:i+k]), n, i, i+k)
		i += k
	}
	return b.normalized()
}

type lowercase struct{}

func (lowercase) normalize(n normalized) normalized {
	return n.mapRunes(func(r rune) string { return string(unicode.ToLower(r)) })
}

type stripAccents struct{}

func (stripAccents) normalize(n normalized) normalized {
	return n.mapRunes(func(r rune) string {
		if unicode.Is(unicode.Mn, r) {
			return ""
		}
		return string(r)
	})
}

type strip struct{ left, right bool }

func (s strip) normalize(n normalized) normalized {
	i, j := 0, len(n.s)
	if s.left {
		i = len(n.s) - len(strings.TrimLeftFunc(n.s, unicode.IsSpace))
	}
	if s.right {
		j = len(strings.TrimRightFunc(n.s, unicode.IsSpace))
	}
	if i >= j {
		return normalized{}
	}
	return n.slice(i, j)
}

type prepend struct{ s string }

func (p prepend) normalize(n normalized) normalized { return n.prepend(p.s) }

type replace struct {
	pattern finder
	content string
}

func (r replace) normalize(n normalized) normalized {
	return n.replaceAll(r.pattern(n.s), r.content)
}

// finder returns the non-overlapping matches of a pattern in s.
type finder func(s string) []span

func literalFinder(lit string) finder {
	return func(s string) []span {
		if lit == "" {
			return nil
		}
		var out []span
		for i := 0; ; {
			k := strings.Index(s[i:], lit)
			if k < 0 {
				return out
			}
			out = append(out, span{i + k, i + k + len(lit)})
			i += k + len(lit)
		}
	}
}

func regexpFinder(re *regexp.Regexp) finder {
	return func(s string) []span {
		idx := re.FindAllStringIndex(s, -1)
		out := make([]span, len(idx))
		for i, m := range idx {
			out[i] = span{m[0], m[1]}
		}
		return out
	}
}

type bertNormalizer struct {
	cleanText, chineseChars, stripAccents, lowercase bool
}

func (b bertNormalizer) normalize(n normalized) normalized {
	if b.cleanText {
		n = n.mapRunes(func(r rune) string {
			switch {
			case r == 0 || r == unicode.ReplacementChar || isControl(r):
				return ""
			case isBertSpace(r):
				return " "
			}
			return string(r)
		})
	}
	if b.chineseChars {
		n = n.mapRunes(func(r rune) string {
			if isChineseChar(r) {
				return " " + string(r) + " "
			}
			return string(r)
		})
	}
	if b.stripAccents {
		n = stripAccents{}.normalize(unicodeNormalizer{norm.NFD}.normalize(n))
	}
	if b.lowercase {
		n = lowercase{}.normalize(n)
	}
	return n
}

func isBertSpace(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || unicode.IsSpace(r)
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs)
}

func isChineseChar(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF ||
		r >= 0x3400 && r <= 0x4DBF ||
		r >= 0x20000 && r <= 0x2A6DF ||
		r >= 0x2A700 && r <= 0x2B73F ||
		r >= 0x2B740 && r <= 0x2B81F ||
		r >= 0x2B920 && r <= 0x2CEAF ||
		r >= 0xF900 && r <= 0xFAFF ||
		r >= 0x2F800 && r <= 0x2FA1F
}
