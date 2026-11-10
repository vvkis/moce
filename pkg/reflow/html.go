// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package reflow

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// HTMLToText follows moon Text.stripHtml: block tags become newlines, entities
// are decoded once, each line is trimmed, and empty lines are dropped.
func HTMLToText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = dropTags(s)
	s = xmlDecode(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = dropTags(s)
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.Join(strings.Fields(line), " "))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

var (
	reCDATA   = regexp.MustCompile(`(?s)<!\[CDATA\[(.*?)\]\]>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reScript  = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	reStyle   = regexp.MustCompile(`(?is)<style\b.*?</style\s*>`)
	reTag     = regexp.MustCompile(`(?s)<\s*/?\s*([A-Za-z][A-Za-z0-9]*)[^>]*>`)
)

var blockTags = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "tr": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

func dropTags(s string) string {
	s = reCDATA.ReplaceAllString(s, "$1")
	s = reComment.ReplaceAllString(s, " ")
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	return reTag.ReplaceAllStringFunc(s, func(tag string) string {
		m := reTag.FindStringSubmatch(tag)
		if m == nil {
			return " "
		}
		if blockTags[strings.ToLower(m[1])] {
			return "\n"
		}
		return " "
	})
}

func xmlDecode(s string) string {
	return entityRE.ReplaceAllStringFunc(s, func(ent string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(ent, "&"), ";")
		if strings.HasPrefix(name, "#x") || strings.HasPrefix(name, "#X") {
			n, err := strconv.ParseInt(name[2:], 16, 32)
			if err != nil {
				return ent
			}
			return utf8Char(int(n))
		}
		if strings.HasPrefix(name, "#") {
			n, err := strconv.Atoi(name[1:])
			if err != nil {
				return ent
			}
			return utf8Char(n)
		}
		if v, ok := namedEntities[name]; ok {
			return v
		}
		return ent
	})
}

var entityRE = regexp.MustCompile(`&(#?[0-9A-Za-z]+);`)

var namedEntities = map[string]string{
	"lt": "<", "gt": ">", "quot": `"`, "apos": "'", "amp": "&",
	"nbsp": "\u00a0", "mdash": "\u2014", "ndash": "\u2013", "hellip": "\u2026",
	"lsquo": "\u2018", "rsquo": "\u2019", "ldquo": "\u201c", "rdquo": "\u201d",
}

func utf8Char(cp int) string {
	if cp == 0 || (cp >= 0xD800 && cp < 0xE000) || cp > 0x10FFFF {
		return ""
	}
	var b [4]byte
	n := utf8.EncodeRune(b[:], rune(cp))
	return string(b[:n])
}
