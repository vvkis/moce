// Package encoding detects and decodes the encodings commonly used by Chinese TXT novels.
package encoding

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	utf16 "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Result is the outcome of an encoding detection.
type Result struct {
	Name       string  // utf-8, utf-16le, utf-16be, gb18030, big5
	Confidence float64 // 0..1
	BOM        bool
}

const sniffLimit = 256 * 1024

var (
	// ErrEmpty means the input had no bytes.
	ErrEmpty = errors.New("文件是空的")
	// ErrUnknown means the bytes did not look like a supported encoding.
	ErrUnknown = errors.New("无法识别编码，请手动指定")
)

// Detect guesses the encoding of b.
func Detect(b []byte) (Result, error) {
	if len(b) == 0 {
		return Result{}, ErrEmpty
	}
	if name, ok := sniffBOM(b); ok {
		return Result{Name: name, Confidence: 1, BOM: true}, nil
	}
	if utf8.Valid(b) {
		return Result{Name: "utf-8", Confidence: 0.99}, nil
	}
	sample := b
	if len(sample) > sniffLimit {
		sample = sample[:sniffLimit]
	}
	if len(sample)%2 == 1 {
		sample = sample[:len(sample)-1]
	}
	type cand struct {
		name  string
		score int
	}
	var best cand
	found := false
	consider := func(name string, score int) {
		if !found || score > best.score {
			best = cand{name: name, score: score}
			found = true
		}
	}
	if len(sample) >= 2 && len(sample)%2 == 0 {
		if s, err := decodeAs("utf-16le", sample); err == nil {
			consider("utf-16le", scoreText(s))
		}
		if s, err := decodeAs("utf-16be", sample); err == nil {
			consider("utf-16be", scoreText(s))
		}
	}
	if s, err := decodeAs("gb18030", sample); err == nil {
		consider("gb18030", scoreText(s))
	}
	if s, err := decodeAs("big5", sample); err == nil {
		consider("big5", scoreText(s))
	}
	if !found || best.score < 0 {
		return Result{}, ErrUnknown
	}
	return Result{Name: best.name, Confidence: 0.8}, nil
}

// Decode decodes b. override is empty or "auto" to detect, otherwise a
// supported encoding name (utf-8, gbk, gb18030, big5, utf-16le, utf-16be).
func Decode(b []byte, override string) (string, Result, error) {
	if len(b) == 0 {
		return "", Result{}, ErrEmpty
	}
	name := strings.ToLower(strings.TrimSpace(override))
	var res Result
	var err error
	if name == "" || name == "auto" {
		res, err = Detect(b)
		if err != nil {
			return "", Result{}, err
		}
		name = res.Name
	} else {
		name, err = Canonical(name)
		if err != nil {
			return "", Result{}, err
		}
		res = Result{Name: name, Confidence: 1, BOM: hasBOM(b, name)}
	}
	text, err := decodeAs(name, b)
	if err != nil {
		return "", res, err
	}
	text = strings.TrimPrefix(text, "\uFEFF")
	text = normalizeNewlines(text)
	return text, res, nil
}

// Canonical maps aliases onto the names this package decodes.
func Canonical(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "utf-8", "utf8":
		return "utf-8", nil
	case "utf-16le", "utf16le", "utf-16-le":
		return "utf-16le", nil
	case "utf-16be", "utf16be", "utf-16-be":
		return "utf-16be", nil
	case "gb18030", "gbk", "gb2312", "gb-2312", "cp936":
		return "gb18030", nil
	case "big5", "big-5", "cp950":
		return "big5", nil
	case "auto", "":
		return "auto", nil
	default:
		return "", fmt.Errorf("不支持的编码：%s", name)
	}
}

func sniffBOM(b []byte) (string, bool) {
	if len(b) >= 4 && b[0] == 0xFF && b[1] == 0xFE && b[2] == 0x00 && b[3] == 0x00 {
		return "", false
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return "utf-16le", true
	}
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return "utf-16be", true
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return "utf-8", true
	}
	return "", false
}

func hasBOM(b []byte, name string) bool {
	got, ok := sniffBOM(b)
	return ok && got == name
}

func decodeAs(name string, b []byte) (string, error) {
	switch name {
	case "utf-8":
		raw := bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
		if !utf8.Valid(raw) {
			return "", errors.New("不是有效的 UTF-8")
		}
		return string(raw), nil
	case "utf-16le":
		return decodeTransform(utf16.UTF16(utf16.LittleEndian, utf16.IgnoreBOM).NewDecoder(), b)
	case "utf-16be":
		return decodeTransform(utf16.UTF16(utf16.BigEndian, utf16.IgnoreBOM).NewDecoder(), b)
	case "gb18030":
		return decodeTransform(simplifiedchinese.GB18030.NewDecoder(), b)
	case "big5":
		return decodeTransform(traditionalchinese.Big5.NewDecoder(), b)
	default:
		return "", fmt.Errorf("不支持的编码：%s", name)
	}
}

func decodeTransform(dec transform.Transformer, b []byte) (string, error) {
	out, _, err := transform.Bytes(dec, b)
	if err != nil {
		return "", errors.New("解码失败")
	}
	return string(out), nil
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

var simplifiedHint = []rune("这说们时为发对国书门吗后个来过还爱里会")
var traditionalHint = []rune("這說們時為發對國書門嗎後個來過還愛裡會")

func scoreText(s string) int {
	if s == "" {
		return -1_000_000
	}
	simp := map[rune]struct{}{}
	trad := map[rune]struct{}{}
	for _, r := range simplifiedHint {
		simp[r] = struct{}{}
	}
	for _, r := range traditionalHint {
		trad[r] = struct{}{}
	}
	han, total, repl, bad, simpN, tradN := 0, 0, 0, 0, 0, 0
	for _, r := range s {
		total++
		switch {
		case r == '\uFFFD':
			repl++
		case r == '\n' || r == '\r' || r == '\t' || r == ' ':
		case r < 0x20:
			bad++
		case unicode.Is(unicode.Han, r):
			han++
			if _, ok := simp[r]; ok {
				simpN++
			}
			if _, ok := trad[r]; ok {
				tradN++
			}
		case r <= 0x7F || unicode.IsPunct(r) || unicode.IsSpace(r):
		default:
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
				bad++
			}
		}
	}
	if total == 0 {
		return -1_000_000
	}
	score := han*20 - repl*80 - bad*40
	if simpN > tradN {
		score += (simpN - tradN) * 30
	} else if tradN > simpN {
		score += (tradN - simpN) * 30
	}
	// A page of Chinese should be mostly Han. Latin noise loses.
	if han*5 < total {
		score -= total
	}
	return score
}
