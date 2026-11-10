// Package textclean turns raw novel lines into paragraphs.
package textclean

import (
	"strings"
	"unicode/utf8"
)

// Options controls paragraph rebuilding.
type Options struct {
	// MergeBrokenLines joins wrapped lines into one paragraph.
	MergeBrokenLines bool
	// DropEmptyLines omits blank lines from the output. They still separate paragraphs.
	DropEmptyLines bool
}

// Paragraphs converts lines into paragraphs the way moon text2epub does.
// Leading full-width spaces and NBSP are removed; indent is left to CSS.
// Blank lines are boundaries. With MergeBrokenLines, a full-width indent,
// a list or dialogue line, or a line after sentence punctuation starts a
// new paragraph. Wrapped lines are joined, with a space only between ASCII words.
func Paragraphs(lines []string, opt Options) []string {
	if !opt.MergeBrokenLines {
		var out []string
		for _, raw := range lines {
			line, _ := paragraphLine(raw)
			if line == "" {
				if !opt.DropEmptyLines {
					out = append(out, "")
				}
				continue
			}
			out = append(out, line)
		}
		return out
	}

	var groups [][]string
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, current)
		current = nil
	}
	for _, raw := range lines {
		line, indented := paragraphLine(raw)
		if line == "" {
			flush()
			continue
		}
		if isHardLine(line) {
			flush()
			groups = append(groups, []string{line})
			continue
		}
		if indented || (len(current) > 0 && endsSentence(current[len(current)-1])) {
			flush()
		}
		current = append(current, line)
	}
	flush()
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		out = append(out, paragraphText(group))
	}
	return out
}

// paragraphLine matches moon text2epub.paragraphLine: trim ASCII space, then
// peel leading full-width spaces (those mark a new paragraph) and NBSP.
func paragraphLine(raw string) (string, bool) {
	line := trimASCII(raw)
	indented := false
	for {
		switch {
		case strings.HasPrefix(line, "\u3000"):
			line = trimASCIILeft(strings.TrimPrefix(line, "\u3000"))
			indented = true
		case strings.HasPrefix(line, "\u00a0"):
			line = trimASCIILeft(strings.TrimPrefix(line, "\u00a0"))
		default:
			return line, indented
		}
	}
}

func trimASCII(s string) string {
	return strings.Trim(s, " \t\r\n\v\f")
}

func trimASCIILeft(s string) string {
	return strings.TrimLeft(s, " \t\r\n\v\f")
}

func isHardLine(line string) bool {
	s := strings.TrimLeft(line, " \t")
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	switch r {
	case '-', '*', '•', '·', '●', '○', '“', '"', '「', '『':
		return true
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) {
		r, _ := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '.', '、', ')':
			return true
		}
	}
	return false
}

func paragraphText(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(lines[0])
	for i := 1; i < len(lines); i++ {
		prev := lines[i-1]
		next := lines[i]
		if prev != "" && next != "" {
			left, _ := utf8.DecodeLastRuneInString(prev)
			right, _ := utf8.DecodeRuneInString(next)
			asciiWord := isASCIIWord(left) && isASCIIWord(right)
			afterPunct := strings.ContainsRune(",.;:!?%)]}'\"", left) && isASCIIStart(right)
			if asciiWord || afterPunct {
				b.WriteByte(' ')
			}
		}
		b.WriteString(next)
	}
	return b.String()
}

func isASCIIWord(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isASCIIStart(r rune) bool {
	return isASCIIWord(r)
}

func endsSentence(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	switch r {
	case '。', '！', '？', '!', '?', '；', ';':
		return true
	default:
		return false
	}
}
