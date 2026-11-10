// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package scrape

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var reTag = regexp.MustCompile(`<[^>]*>`)

func trim(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\u3000", " "))
}

func xmlDecode(s string) string {
	return html.UnescapeString(s)
}

func stripHTML(s string) string {
	s = reTag.ReplaceAllString(s, "")
	return trim(xmlDecode(s))
}

// urlEncode matches Lua socket.url.escape: unreserved bytes stay, everything else is %XX.
func urlEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~'
}

func urlDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			h := s[i+1 : i+3]
			var v byte
			if _, err := fmt.Sscanf(h, "%02X", &v); err == nil {
				b.WriteByte(v)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func formEncode(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, urlEncode(k)+"="+urlEncode(params[k]))
	}
	return strings.Join(parts, "&")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// similarity is moon's byte-wise similar_text, 0 to 1.
func similarity(s1, s2 string) float64 {
	s1 = strings.ToLower(strings.ReplaceAll(s1, " ", ""))
	s2 = strings.ToLower(strings.ReplaceAll(s2, " ", ""))
	if s1 == s2 {
		return 1
	}
	if s1 == "" || s2 == "" {
		return 0
	}
	matches := 0
	i, j := 0, 0
	for i < len(s1) && j < len(s2) {
		if s1[i] == s2[j] {
			matches++
			i++
			j++
			continue
		}
		j++
	}
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	return float64(matches) / float64(maxLen)
}
