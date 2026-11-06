package tokenizer

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Go's regexp has no lookahead, so the widely used GPT-2, Llama 3 and Qwen 2
// split patterns are implemented as hand-written scanners that produce the
// same matches.
const (
	gpt2Pattern   = `'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+`
	llama3Pattern = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`
	qwen2Pattern  = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`
)

func patternFinder(pattern string) (finder, error) {
	switch pattern {
	case gpt2Pattern:
		return scannerFinder(gpt2Next), nil
	case llama3Pattern:
		return scannerFinder(cl100kNext(3)), nil
	case qwen2Pattern:
		return scannerFinder(cl100kNext(1)), nil
	}
	re, err := regexp.Compile(translateRegex(pattern))
	if err != nil {
		return nil, fmt.Errorf("tokenizer: unsupported regex %q: %w", pattern, err)
	}
	return regexpFinder(re), nil
}

// translateRegex rewrites the Perl classes \s, \d and \w to their Unicode
// meaning, which is what the Rust regex engine uses.
func translateRegex(p string) string {
	classes := map[byte]string{
		's': `\t\n\v\f\r\x{85}\p{Z}`,
		'd': `\p{Nd}`,
		'w': `\p{L}\p{M}\p{Nd}\p{Nl}\p{Pc}\x{200C}\x{200D}`,
	}
	var sb strings.Builder
	inClass := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\\' && i+1 < len(p):
			if cl, ok := classes[p[i+1]]; ok {
				if inClass {
					sb.WriteString(cl)
				} else {
					sb.WriteString("[" + cl + "]")
				}
				i++
				continue
			}
			sb.WriteString(p[i : i+2])
			i++
			continue
		case c == '[' && !inClass:
			inClass = true
		case c == ']' && inClass:
			inClass = false
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// scannerFinder turns a function returning the length of the match at the
// start of s into a finder. The scanners match every position, so the
// matches tile the input.
func scannerFinder(next func(s string) int) finder {
	return func(s string) []span {
		var out []span
		for i := 0; i < len(s); {
			n := next(s[i:])
			out = append(out, span{i, i + n})
			i += n
		}
		return out
	}
}

const (
	classLetter = iota
	classNumber
	classSpace
	classOther
)

func classify(r rune) int {
	switch {
	case unicode.IsLetter(r):
		return classLetter
	case unicode.IsNumber(r):
		return classNumber
	case unicode.IsSpace(r):
		return classSpace
	}
	return classOther
}

// run returns the length of the longest prefix of s whose runes satisfy f,
// stopping after max runes when max > 0.
func run(s string, max int, f func(r rune) bool) int {
	n, count := 0, 0
	for n < len(s) && (max <= 0 || count < max) {
		r, size := utf8.DecodeRuneInString(s[n:])
		if !f(r) {
			break
		}
		n += size
		count++
	}
	return n
}

func isClass(c int) func(r rune) bool {
	return func(r rune) bool { return classify(r) == c }
}

func contraction(s string, fold bool) int {
	if len(s) < 2 || s[0] != '\'' {
		return 0
	}
	t := s[1:]
	if fold {
		t = strings.ToLower(t[:min(2, len(t))])
	}
	for _, c := range []string{"re", "ve", "ll", "s", "t", "m", "d"} {
		if strings.HasPrefix(t, c) {
			return 1 + len(c)
		}
	}
	return 0
}

// spaceRun handles \s+(?!\S)|\s+ for a whitespace run at the start of s.
func spaceRun(s string) int {
	n := run(s, 0, unicode.IsSpace)
	if n == len(s) {
		return n
	}
	_, last := utf8.DecodeLastRuneInString(s[:n])
	if n-last > 0 {
		return n - last
	}
	return n
}

func gpt2Next(s string) int {
	if n := contraction(s, false); n > 0 {
		return n
	}
	r, size := utf8.DecodeRuneInString(s)
	c := classify(r)
	if r == ' ' && size < len(s) {
		r2, _ := utf8.DecodeRuneInString(s[size:])
		if c2 := classify(r2); c2 != classSpace {
			return size + run(s[size:], 0, isClass(c2))
		}
	}
	if c != classSpace {
		return run(s, 0, isClass(c))
	}
	return spaceRun(s)
}

func cl100kNext(maxDigits int) func(s string) int {
	isNewline := func(r rune) bool { return r == '\r' || r == '\n' }
	return func(s string) int {
		if n := contraction(s, true); n > 0 {
			return n
		}
		r, size := utf8.DecodeRuneInString(s)
		c := classify(r)
		if c == classLetter {
			return run(s, 0, unicode.IsLetter)
		}
		if c != classNumber && !isNewline(r) && size < len(s) {
			if n := run(s[size:], 0, unicode.IsLetter); n > 0 {
				return size + n
			}
		}
		if c == classNumber {
			return run(s, maxDigits, unicode.IsNumber)
		}
		start := 0
		if r == ' ' && size < len(s) {
			r2, _ := utf8.DecodeRuneInString(s[size:])
			if classify(r2) == classOther {
				start = size
				c = classOther
			}
		}
		if c == classOther {
			n := start + run(s[start:], 0, isClass(classOther))
			return n + run(s[n:], 0, isNewline)
		}
		n := run(s, 0, unicode.IsSpace)
		if k := strings.LastIndexAny(s[:n], "\r\n"); k >= 0 {
			return k + 1
		}
		return spaceRun(s)
	}
}
