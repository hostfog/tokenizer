package tokenizer

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// decoder rewrites a token list; the final text is the concatenation.
type decoder interface {
	decode(tokens []string) []string
}

type decoderSeq []decoder

func (seq decoderSeq) decode(tokens []string) []string {
	for _, d := range seq {
		tokens = d.decode(tokens)
	}
	return tokens
}

type byteLevelDecoder struct{}

func (byteLevelDecoder) decode(tokens []string) []string {
	var buf []byte
	for _, t := range tokens {
		n := len(buf)
		for _, r := range t {
			b, ok := runeToByte[r]
			if !ok {
				buf = append(buf[:n], t...)
				break
			}
			buf = append(buf, b)
		}
	}
	return []string{lossy(buf)}
}

// lossy converts b to a string, replacing each maximal invalid subsequence
// with U+FFFD the way Rust's String::from_utf8_lossy does.
func lossy(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || size > 1 {
			sb.Write(b[i : i+size])
			i += size
			continue
		}
		sb.WriteRune(utf8.RuneError)
		i += invalidLen(b[i:])
	}
	return sb.String()
}

func invalidLen(b []byte) int {
	need, lo, hi := 0, byte(0x80), byte(0xBF)
	switch c := b[0]; {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c >= 0xE0 && c <= 0xEF:
		need = 2
		if c == 0xE0 {
			lo = 0xA0
		} else if c == 0xED {
			hi = 0x9F
		}
	case c >= 0xF0 && c <= 0xF4:
		need = 3
		if c == 0xF0 {
			lo = 0x90
		} else if c == 0xF4 {
			hi = 0x8F
		}
	}
	n := 1
	for n <= need && n < len(b) && b[n] >= lo && b[n] <= hi {
		n++
		lo, hi = 0x80, 0xBF
	}
	return n
}

type metaspaceDecoder struct {
	replacement string
	prepend     bool
}

func (m metaspaceDecoder) decode(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		repl := " "
		if i == 0 && m.prepend {
			repl = ""
		}
		out[i] = strings.ReplaceAll(t, m.replacement, repl)
	}
	return out
}

type wordPieceDecoder struct {
	prefix  string
	cleanup bool
}

func (w wordPieceDecoder) decode(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		if i > 0 {
			if strings.HasPrefix(t, w.prefix) {
				t = t[len(w.prefix):]
			} else {
				t = " " + t
			}
		}
		if w.cleanup {
			t = cleanup(t)
		}
		out[i] = t
	}
	return out
}

var cleaner = strings.NewReplacer(
	" .", ".", " ?", "?", " !", "!", " ,", ",", " ' ", "'",
	" n't", "n't", " 'm", "'m", " do not", " don't", " 's", "'s", " 've", "'ve", " 're", "'re",
)

func cleanup(s string) string { return cleaner.Replace(s) }

type bpeDecoder struct{ suffix string }

func (b bpeDecoder) decode(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		repl := " "
		if i == len(tokens)-1 {
			repl = ""
		}
		out[i] = strings.ReplaceAll(t, b.suffix, repl)
	}
	return out
}

type byteFallbackDecoder struct{}

func (byteFallbackDecoder) decode(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	var pending []byte
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if utf8.Valid(pending) {
			out = append(out, string(pending))
		} else {
			for range pending {
				out = append(out, "\uFFFD")
			}
		}
		pending = pending[:0]
	}
	for _, t := range tokens {
		if b, ok := parseByteToken(t); ok {
			pending = append(pending, b)
			continue
		}
		flush()
		out = append(out, t)
	}
	flush()
	return out
}

func parseByteToken(t string) (byte, bool) {
	if len(t) != 6 || !strings.HasPrefix(t, "<0x") || t[5] != '>' {
		return 0, false
	}
	b, err := strconv.ParseUint(t[3:5], 16, 8)
	return byte(b), err == nil
}

type fuseDecoder struct{}

func (fuseDecoder) decode(tokens []string) []string {
	return []string{strings.Join(tokens, "")}
}

type stripDecoder struct {
	content     string
	start, stop int
}

func (s stripDecoder) decode(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		for range s.start {
			if !strings.HasPrefix(t, s.content) {
				break
			}
			t = t[len(s.content):]
		}
		for range s.stop {
			if !strings.HasSuffix(t, s.content) {
				break
			}
			t = t[:len(t)-len(s.content)]
		}
		out[i] = t
	}
	return out
}

type replaceDecoder struct {
	pattern finder
	content string
}

func (r replaceDecoder) decode(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		var sb strings.Builder
		last := 0
		for _, m := range r.pattern(t) {
			sb.WriteString(t[last:m.start])
			sb.WriteString(r.content)
			last = m.end
		}
		sb.WriteString(t[last:])
		out[i] = sb.String()
	}
	return out
}
