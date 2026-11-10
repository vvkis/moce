package txt2epub

import (
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func guessTitle(filename, text string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = trim(base)
	if inner := bracketTitle(base); inner != "" {
		return inner
	}
	head := headLines(text, 40)
	if m := reBookName.FindStringSubmatch(head); len(m) == 2 {
		if t := trim(m[1]); t != "" {
			return clip(t, 80)
		}
	}
	if inner := bracketTitle(head); inner != "" {
		return inner
	}
	base = reAuthorInFile.ReplaceAllString(base, "")
	base = strings.Trim(trim(base), "-_—")
	base = trim(base)
	if base == "" {
		return "未命名"
	}
	return clip(base, 80)
}

func guessAuthor(filename, text string) string {
	head := headLines(text, 40)
	if m := reAuthorLine.FindStringSubmatch(head); len(m) == 2 {
		if a := trim(m[1]); a != "" {
			return clip(a, 40)
		}
	}
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if m := reAuthorInFile.FindStringSubmatch(base); len(m) == 2 {
		if a := trim(m[1]); a != "" {
			return clip(a, 40)
		}
	}
	return ""
}

func bracketTitle(s string) string {
	m := reBracket.FindStringSubmatch(s)
	if len(m) != 2 {
		return ""
	}
	return clip(trim(m[1]), 80)
}

func headLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func stripLeadingMeta(text, title, author string) string {
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) && i < 40 {
		t := trim(lines[i])
		if t == "" {
			i++
			continue
		}
		if isMetaLine(t, title, author) {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}

func isMetaLine(line, title, author string) bool {
	if line == "封面" || line == "封面：" || line == "目录" {
		return true
	}
	if title != "" && (line == title || line == "《"+title+"》" || line == "书名："+title || line == "书名:"+title) {
		return true
	}
	if reAuthorLine.MatchString(line) || reBookName.MatchString(line) {
		return true
	}
	if author != "" && (line == author || line == "作者："+author || line == "作者:"+author) {
		return true
	}
	return false
}

func trim(s string) string {
	return strings.Trim(s, " \t\r\n\u3000\u00a0\ufeff")
}

func clip(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func sanitizeFilename(name string) string {
	name = trim(name)
	if name == "" {
		name = "未命名"
	}
	repl := strings.NewReplacer(
		`\`, " ", `/`, " ", `:`, " ", `*`, " ", `?`, " ",
		`"`, " ", `<`, " ", `>`, " ", `|`, " ", "\n", " ", "\r", " ",
	)
	name = trim(repl.Replace(name))
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "未命名"
	}
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "LPT1":
		name += "_"
	}
	rs := []rune(name)
	if len(rs) > 80 {
		name = string(rs[:80])
	}
	return name
}

func runeCount(s string) int {
	return utf8.RuneCountInString(s)
}

func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ',', '，', '、', ';', '；', '\n':
			return true
		default:
			return false
		}
	})
	var out []string
	seen := map[string]bool{}
	for _, part := range parts {
		part = trim(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func normalizeLanguage(s string) (string, error) {
	s = trim(s)
	switch strings.ToLower(s) {
	case "", "中文", "简体中文", "zh", "zh-cn", "zh-hans":
		return "zh", nil
	case "繁体中文", "繁體中文", "zh-tw", "zh-hk", "zh-hant":
		return "zh-Hant", nil
	case "英文", "英语", "en", "eng":
		return "en", nil
	case "日文", "日语", "ja", "jp":
		return "ja", nil
	case "韩文", "韩语", "ko", "kor":
		return "ko", nil
	}
	if languageCode.MatchString(s) {
		return s, nil
	}
	return "", errors.New("语言请填写代码，例如 zh 或 en")
}

var languageCode = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})?$`)

func normalizePublished(s string) (string, error) {
	s = trim(s)
	if s == "" {
		return "", nil
	}
	s = strings.NewReplacer("／", "-", "/", "-", ".", "-", "年", "-", "月", "-", "日", "").Replace(s)
	s = strings.Trim(s, "-")
	parts := strings.Split(s, "-")
	if len(parts) > 3 {
		return "", errors.New("出版日期请写成 2026-11-08")
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 1 || year > 9999 {
		return "", errors.New("出版日期请写成 2026-11-08")
	}
	out := fmtInt(year, 4)
	if len(parts) == 1 {
		return out, nil
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		return "", errors.New("出版日期请写成 2026-11-08")
	}
	out += "-" + fmtInt(month, 2)
	if len(parts) == 2 {
		return out, nil
	}
	day, err := strconv.Atoi(parts[2])
	if err != nil || day < 1 || day > 31 {
		return "", errors.New("出版日期请写成 2026-11-08")
	}
	if _, err := time.Parse("2006-01-02", out+"-"+fmtInt(day, 2)); err != nil {
		return "", errors.New("出版日期请写成 2026-11-08")
	}
	return out + "-" + fmtInt(day, 2), nil
}

func fmtInt(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func normalizeSeriesIndex(raw string, hasSeries bool) (string, error) {
	raw = trim(raw)
	if raw == "" {
		if hasSeries {
			return "1", nil
		}
		return "", nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 || f > 100000 {
		return "", errors.New("丛书序号需要是不小于 0 的数字")
	}
	return strconv.FormatFloat(f, 'f', -1, 64), nil
}

func normalizeRating(n int) (int, error) {
	if n < 0 || n > 5 {
		return 0, errors.New("评分需要在 0 到 5 之间")
	}
	return n, nil
}

func normalizeIdentifiers(s string) []string {
	var out []string
	for _, item := range splitList(s) {
		out = append(out, normalizeOneID(item))
	}
	return out
}

func normalizeOneID(s string) string {
	lower := strings.ToLower(s)
	body := s
	if strings.HasPrefix(lower, "isbn") {
		body = trim(s[len("isbn"):])
		body = strings.TrimLeft(body, ":：")
		body = trim(body)
	}
	compact := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\u3000' {
			return -1
		}
		return r
	}, body)
	if looksISBN(compact) {
		return "urn:isbn:" + strings.ToUpper(compact)
	}
	return s
}

func looksISBN(s string) bool {
	if len(s) == 13 {
		for _, r := range s {
			if !unicode.IsDigit(r) {
				return false
			}
		}
		return true
	}
	if len(s) != 10 {
		return false
	}
	for i, r := range s {
		if unicode.IsDigit(r) {
			continue
		}
		if i == 9 && (r == 'X' || r == 'x') {
			continue
		}
		return false
	}
	return true
}
