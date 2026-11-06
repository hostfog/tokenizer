package tokenizer

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type addedToken struct {
	ID         int    `json:"id"`
	Content    string `json:"content"`
	Special    bool   `json:"special"`
	SingleWord bool   `json:"single_word"`
	LStrip     bool   `json:"lstrip"`
	RStrip     bool   `json:"rstrip"`
	Normalized bool   `json:"normalized"`
}

// matcher finds added tokens in a string, leftmost-longest.
type matcher struct {
	byFirst *[256][]matchEntry
}

type matchEntry struct {
	content string
	tok     *addedToken
}

func newMatcher(entries []matchEntry) matcher {
	if len(entries) == 0 {
		return matcher{}
	}
	m := matcher{byFirst: new([256][]matchEntry)}
	for _, e := range entries {
		if e.content != "" {
			m.byFirst[e.content[0]] = append(m.byFirst[e.content[0]], e)
		}
	}
	for _, list := range m.byFirst {
		sort.SliceStable(list, func(i, j int) bool { return len(list[i].content) > len(list[j].content) })
	}
	return m
}

type segment struct {
	tok        *addedToken
	start, end int
}

// split cuts s into plain text segments and added token segments.
func (m matcher) split(s string) []segment {
	if m.byFirst == nil {
		return []segment{{nil, 0, len(s)}}
	}
	var out []segment
	last := 0
	for i := 0; i < len(s); {
		e, ok := m.match(s[i:])
		if !ok {
			i++
			continue
		}
		start, end := i, i+len(e.content)
		if e.tok.SingleWord && (endsWithWord(s[:start]) || startsWithWord(s[end:])) {
			i = end
			continue
		}
		if e.tok.LStrip {
			start = max(len(strings.TrimRightFunc(s[:start], unicode.IsSpace)), last)
		}
		if e.tok.RStrip {
			end = len(s) - len(strings.TrimLeftFunc(s[end:], unicode.IsSpace))
		}
		if last < start {
			out = append(out, segment{nil, last, start})
		}
		out = append(out, segment{e.tok, start, end})
		last, i = end, end
	}
	if last < len(s) {
		out = append(out, segment{nil, last, len(s)})
	}
	return out
}

func (m matcher) match(s string) (matchEntry, bool) {
	for _, e := range m.byFirst[s[0]] {
		if strings.HasPrefix(s, e.content) {
			return e, true
		}
	}
	return matchEntry{}, false
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

func endsWithWord(s string) bool {
	r, size := utf8.DecodeLastRuneInString(s)
	return size > 0 && isWordChar(r)
}

func startsWithWord(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	return size > 0 && isWordChar(r)
}
