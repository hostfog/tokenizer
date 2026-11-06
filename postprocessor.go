package tokenizer

import (
	"unicode"
	"unicode/utf8"
)

type postProcessor interface {
	process(enc Encoding, addSpecial bool) Encoding
}

type postProcessorSeq []postProcessor

func (seq postProcessorSeq) process(enc Encoding, addSpecial bool) Encoding {
	for _, p := range seq {
		enc = p.process(enc, addSpecial)
	}
	return enc
}

// templatePiece is either a special token (ids set) or the input sequence.
type templatePiece struct {
	ids    []int
	tokens []string
}

type template []templatePiece

func (t template) process(enc Encoding, addSpecial bool) Encoding {
	if !addSpecial {
		return enc
	}
	var out Encoding
	for _, p := range t {
		if p.ids == nil {
			out.IDs = append(out.IDs, enc.IDs...)
			out.Tokens = append(out.Tokens, enc.Tokens...)
			out.Offsets = append(out.Offsets, enc.Offsets...)
			continue
		}
		for i, id := range p.ids {
			out.IDs = append(out.IDs, id)
			out.Tokens = append(out.Tokens, p.tokens[i])
			out.Offsets = append(out.Offsets, [2]int{})
		}
	}
	return out
}

func wrap(cls, sep templatePiece) template {
	return template{cls, {}, sep}
}

// trimOffsets removes leading and trailing spaces from byte-level token
// offsets.
type trimOffsets struct {
	addPrefixSpace bool
}

func (t trimOffsets) process(enc Encoding, _ bool) Encoding {
	isSpace := func(r rune) bool { return r == 'Ġ' || unicode.IsSpace(r) }
	for i, tok := range enc.Tokens {
		off := &enc.Offsets[i]
		lead := 0
		for _, r := range tok {
			if !isSpace(r) {
				break
			}
			lead++
		}
		trail := 0
		for s := tok; s != ""; {
			r, size := utf8.DecodeLastRuneInString(s)
			if !isSpace(r) {
				break
			}
			trail++
			s = s[:len(s)-size]
		}
		if lead > 0 {
			if (i == 0 || off[0] == 0) && t.addPrefixSpace && lead == 1 {
				lead = 0
			}
			off[0] = min(off[0]+lead, off[1])
		}
		if trail > 0 && off[1] >= trail {
			off[1] = max(off[1]-trail, off[0])
		}
	}
	return enc
}
