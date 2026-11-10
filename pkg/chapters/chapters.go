// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

// Package chapters splits Chinese novel text into a volume/chapter tree.
package chapters

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"moce/pkg/textclean"
)

const (
	numeralClass = `[0-9０-９零〇一二三四五六七八九十百千万两壹贰貳叁參肆伍陆陸柒捌玖拾佰仟]+`
	// DefaultChapterPattern matches 第X章/节/回 and standalone headings such as 序章, 楔子, 番外, 尾声, 后记.
	DefaultChapterPattern = `第\s*` + numeralClass + `\s*[章节回].*|序章.*|楔子.*|前言.*|序言.*|引言.*|引子.*|番外.*|尾声.*|后记.*|终章.*|大结局.*|完结感言.*|附录.*`
	// DefaultVolumePattern matches 第X卷 and 第X部.
	DefaultVolumePattern = `第\s*` + numeralClass + `\s*[卷部].*`
	// DefaultFallbackRunes is the chunk size used when no headings are found.
	DefaultFallbackRunes = 8000
	maxHeadingRunes      = 50
	closerSep            = `[\s　·・,，、:：\-—]*`
	closerWord           = `(?:完结|结束|完|终)`
)

// Heading is a source TOC entry. Depth starts at 1.
type Heading struct {
	Title string
	Depth int
}

// Options selects how headings and paragraphs are recognized.
type Options struct {
	ChapterPattern   string
	VolumePattern    string
	MergeBrokenLines bool
	DropEmptyLines   bool
	FallbackRunes    int
	// Headings are titles taken from an existing EPUB nav or NCX.
	// A whole line that matches one is a chapter break. Nested depths are kept.
	Headings []Heading
}

// Node is one TOC entry. A volume has children; a chapter has paragraphs.
type Node struct {
	Title      string
	Paragraphs []string
	Children   []Node
}

// Entry is a flattened TOC row for previews.
type Entry struct {
	Title      string `json:"title"`
	Level      int    `json:"level"`
	Kind       string `json:"kind"`
	Paragraphs int    `json:"paragraphs"`
	Snippet    string `json:"snippet"`
}

// Result is the split book.
type Result struct {
	Nodes        []Node
	Entries      []Entry
	Fallback     bool
	ChapterCount int
	VolumeCount  int
}

var (
	rootSpecial         = regexp.MustCompile(`^(序章|楔子|前言|序言|引言|引子|尾声|后记|终章|大结局|完结感言|附录)`)
	reFanwai            = regexp.MustCompile(`^番外`)
	reVolumeCloser      = regexp.MustCompile(`^第\s*` + numeralClass + `\s*[卷部]` + closerSep + closerWord + `$`)
	reShortVolumeCloser = regexp.MustCompile(`^[卷部]\s*` + numeralClass + closerSep + closerWord + `$`)
	reThisVolumeCloser  = regexp.MustCompile(`^本[卷部]` + closerSep + closerWord + `$`)
	reVolumeStart       = regexp.MustCompile(`^第\s*` + numeralClass + `\s*[卷部]`)
	reDatePrefix        = regexp.MustCompile(`^[（(]?[0-9０-９]{1,4}[.．\-/／][0-9０-９]{1,2}(?:[.．\-/／][0-9０-９]{1,2})?[）)]?\s*`)
)

// Split detects headings and builds a two-level TOC.
// When nothing matches, the text is cut into sized parts.
func Split(text string, opt Options) (Result, error) {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.TrimSpace(strings.ReplaceAll(text, "\u3000", "")) == "" {
		return Result{}, fmt.Errorf("没有可转换的正文")
	}
	chapterPat := strings.TrimSpace(opt.ChapterPattern)
	if chapterPat == "" {
		chapterPat = DefaultChapterPattern
	}
	volumePat := strings.TrimSpace(opt.VolumePattern)
	if volumePat == "" {
		volumePat = DefaultVolumePattern
	}
	chapterRe, err := regexp.Compile(chapterPat)
	if err != nil {
		return Result{}, fmt.Errorf("章节正则无效：%v", err)
	}
	volumeRe, err := regexp.Compile(volumePat)
	if err != nil {
		return Result{}, fmt.Errorf("分卷正则无效：%v", err)
	}
	if opt.FallbackRunes <= 0 {
		opt.FallbackRunes = DefaultFallbackRunes
	}
	useMoon := chapterPat == DefaultChapterPattern && volumePat == DefaultVolumePattern
	known := map[string]bool{}
	for _, h := range opt.Headings {
		if key := titleKey(trimOuter(h.Title)); key != "" {
			known[key] = true
		}
	}

	lines := strings.Split(text, "\n")
	type section struct {
		title   string
		kind    int // 0 preface, 1 chapter, 2 volume
		special bool
		lines   []string
	}
	var sections []section
	cur := -1
	anyHeading := false
	for _, line := range lines {
		trimmed := trimOuter(line)
		display := stripDatePrefix(trimmed)
		// 「第五卷 完」「本卷完」 close the previous chapter. They are not TOC nodes.
		if isVolumeCloser(display) {
			if cur < 0 {
				sections = append(sections, section{kind: 0})
				cur = 0
			}
			sections[cur].lines = append(sections[cur].lines, line)
			continue
		}
		var ok bool
		var title string
		var kind int
		var special bool
		if useMoon {
			title, ok = chapterTitle(trimmed)
			if !ok && known[titleKey(trimmed)] {
				title, ok = cleanTitle(trimmed), true
			}
		} else {
			kind, special, title, ok = matchHeading(trimmed, chapterRe, volumeRe)
		}
		if ok {
			anyHeading = true
			sections = append(sections, section{
				title:   title,
				kind:    kind,
				special: special,
			})
			cur = len(sections) - 1
			continue
		}
		if cur < 0 {
			sections = append(sections, section{kind: 0})
			cur = 0
		}
		sections[cur].lines = append(sections[cur].lines, line)
	}
	if useMoon {
		if !anyHeading {
			nodes, err := fallback(lines, opt)
			if err != nil {
				return Result{}, err
			}
			return finish(nodes, true), nil
		}
		entries := make([]hEntry, len(sections))
		for i, s := range sections {
			entries[i] = hEntry{title: s.title, lines: s.lines, toc: true}
		}
		entries = dropTocEchoes(entries)
		entries = organize(entries, opt.Headings)
		entries = mergeHidden(entries)
		entries = groupFanwai(entries)
		clean := textclean.Options{MergeBrokenLines: opt.MergeBrokenLines, DropEmptyLines: opt.DropEmptyLines}
		roots := collapseLonelyVolumes(treeFrom(entries, clean))
		if len(roots) == 0 {
			return Result{}, fmt.Errorf("没有可转换的正文")
		}
		return finish(roots, false), nil
	}

	if !anyHeading {
		nodes, err := fallback(lines, opt)
		if err != nil {
			return Result{}, err
		}
		return finish(nodes, true), nil
	}

	hasChapter := false
	for _, s := range sections {
		if s.kind == 1 {
			hasChapter = true
			break
		}
	}
	if !hasChapter {
		for i := range sections {
			if sections[i].kind == 2 {
				sections[i].kind = 1
			}
		}
	}

	clean := textclean.Options{MergeBrokenLines: opt.MergeBrokenLines, DropEmptyLines: opt.DropEmptyLines}
	var roots []Node
	volIndex := -1
	for _, s := range sections {
		paras := textclean.Paragraphs(s.lines, clean)
		if s.kind == 0 {
			if len(paras) == 0 {
				continue
			}
			roots = append(roots, Node{Title: "前言", Paragraphs: paras})
			continue
		}
		node := Node{Title: s.title, Paragraphs: paras}
		if s.kind == 2 {
			roots = append(roots, node)
			volIndex = len(roots) - 1
			continue
		}
		if volIndex >= 0 && !s.special && hasChapter {
			roots[volIndex].Children = append(roots[volIndex].Children, node)
			continue
		}
		roots = append(roots, node)
		if s.special {
			volIndex = -1
		}
	}
	roots = collapseLonelyVolumes(roots)
	if len(roots) == 0 {
		return Result{}, fmt.Errorf("没有可转换的正文")
	}
	return finish(roots, false), nil
}

// isVolumeCloser reports lines that only mark the end of a volume, such as
// 「第五卷 完」「第五卷完」「第5卷终」「卷一完」「本卷完」「第X部 完」.
// 「第五卷 完结篇」 and 「第五卷 风起」 stay real titles: the closer word must
// be the whole remainder, and the next heading is not required to decide that.
// A volume heading with that shape and almost no body is still a closer when
// the following heading is another volume or a chapter, because it was not
// given its own text.
func isVolumeCloser(title string) bool {
	s := stripEdgeMarks(cleanTitle(title))
	if s == "" || utf8.RuneCountInString(s) > maxHeadingRunes {
		return false
	}
	return reVolumeCloser.MatchString(s) || reShortVolumeCloser.MatchString(s) || reThisVolumeCloser.MatchString(s)
}

func stripEdgeMarks(s string) string {
	return strings.Trim(s, " \t　。！？!?,，、：:；;\"'“”‘’（）()【】[]《》<>·…—～~、")
}

func collapseLonelyVolumes(nodes []Node) []Node {
	var out []Node
	var pending []string
	for _, n := range nodes {
		if lonelyVolume(n) {
			bits := n.Paragraphs
			if strings.TrimSpace(n.Title) != "" {
				bits = append([]string{n.Title}, bits...)
			}
			if len(out) > 0 {
				out[len(out)-1].Paragraphs = append(out[len(out)-1].Paragraphs, bits...)
			} else {
				pending = append(pending, bits...)
			}
			continue
		}
		if len(pending) > 0 {
			n.Paragraphs = append(append([]string{}, pending...), n.Paragraphs...)
			pending = nil
		}
		out = append(out, n)
	}
	if len(pending) > 0 && len(out) == 0 {
		out = append(out, Node{Title: "正文", Paragraphs: pending})
	}
	return out
}

func lonelyVolume(n Node) bool {
	if len(n.Children) > 0 || !volumeLikeTitle(n.Title) {
		return false
	}
	for _, p := range n.Paragraphs {
		p = strings.TrimSpace(p)
		if p == "" || isVolumeCloser(p) || p == "完" || p == "终" || p == "完结" || p == "结束" {
			continue
		}
		return false
	}
	return true
}

func volumeLikeTitle(title string) bool {
	s := cleanTitle(title)
	if s == "" || rootSpecial.MatchString(s) {
		return false
	}
	return reVolumeStart.MatchString(s)
}

func finish(nodes []Node, fallback bool) Result {
	return Result{
		Nodes:        nodes,
		Entries:      Flatten(nodes),
		Fallback:     fallback,
		ChapterCount: countBodies(nodes),
		VolumeCount:  countVolumes(nodes),
	}
}

func fallback(lines []string, opt Options) ([]Node, error) {
	paras := textclean.Paragraphs(lines, textclean.Options{
		MergeBrokenLines: opt.MergeBrokenLines,
		DropEmptyLines:   opt.DropEmptyLines,
	})
	var kept []string
	for _, p := range paras {
		if p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("没有可转换的正文")
	}
	var nodes []Node
	var cur []string
	count := 0
	part := 1
	flush := func() {
		if len(cur) == 0 {
			return
		}
		title := fmt.Sprintf("第%d部分", part)
		nodes = append(nodes, Node{Title: title, Paragraphs: append([]string(nil), cur...)})
		part++
		cur = nil
		count = 0
	}
	for _, p := range kept {
		n := utf8.RuneCountInString(p)
		if count > 0 && count+n > opt.FallbackRunes {
			flush()
		}
		cur = append(cur, p)
		count += n
	}
	flush()
	if len(nodes) == 1 {
		nodes[0].Title = "正文"
	}
	return nodes, nil
}

func matchHeading(trimmed string, chapterRe, volumeRe *regexp.Regexp) (kind int, special bool, title string, ok bool) {
	display := stripDatePrefix(trimmed)
	if display == "" {
		return 0, false, "", false
	}
	n := utf8.RuneCountInString(display)
	if n > maxHeadingRunes {
		return 0, false, "", false
	}
	// Sentence punctuation is fine on a short line that still starts as 第X章/卷.
	// A long line, or one that only mentions 第X章 mid-sentence, is body text.
	if strings.ContainsAny(display, "。！？!?") && n > 40 {
		return 0, false, "", false
	}
	if reFanwai.MatchString(display) {
		return 2, false, cleanTitle(display), true
	}
	if volumeRe != nil && prefixMatch(volumeRe, display) {
		return 2, false, cleanTitle(display), true
	}
	if chapterRe != nil && prefixMatch(chapterRe, display) {
		return 1, rootSpecial.MatchString(display), cleanTitle(display), true
	}
	return 0, false, "", false
}

func stripDatePrefix(s string) string {
	s = trimOuter(s)
	if loc := reDatePrefix.FindStringIndex(s); loc != nil && loc[0] == 0 && loc[1] < len(s) {
		rest := trimOuter(s[loc[1]:])
		if strings.HasPrefix(rest, "第") || reFanwai.MatchString(rest) {
			return rest
		}
	}
	return s
}

func prefixMatch(re *regexp.Regexp, s string) bool {
	loc := re.FindStringIndex(s)
	return loc != nil && loc[0] == 0 && loc[1] > loc[0]
}

func trimOuter(s string) string {
	return strings.Trim(s, " \t\r\n\u3000\u00a0\ufeff")
}

func cleanTitle(s string) string {
	s = trimOuter(s)
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\u3000'
	})
	return strings.Join(parts, " ")
}

// Flatten walks the tree in reading order.
func Flatten(nodes []Node) []Entry {
	var out []Entry
	var walk func([]Node, int)
	walk = func(ns []Node, level int) {
		for _, n := range ns {
			kind := "chapter"
			if len(n.Children) > 0 {
				kind = "volume"
			}
			snip := ""
			switch {
			case len(n.Paragraphs) > 0:
				snip = snippet(n.Paragraphs[0])
			case len(n.Children) > 0:
				for _, ch := range n.Children {
					if len(ch.Paragraphs) > 0 {
						snip = snippet(ch.Paragraphs[0])
						break
					}
				}
			}
			out = append(out, Entry{
				Title:      n.Title,
				Level:      level,
				Kind:       kind,
				Paragraphs: len(n.Paragraphs),
				Snippet:    snip,
			})
			walk(n.Children, level+1)
		}
	}
	walk(nodes, 1)
	return out
}

func snippet(s string) string {
	rs := []rune(s)
	if len(rs) > 36 {
		return string(rs[:36]) + "…"
	}
	return s
}

func countBodies(nodes []Node) int {
	n := 0
	var walk func([]Node)
	walk = func(ns []Node) {
		for _, node := range ns {
			if len(node.Paragraphs) > 0 || len(node.Children) == 0 {
				n++
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return n
}

func countVolumes(nodes []Node) int {
	n := 0
	for _, node := range nodes {
		if len(node.Children) > 0 {
			n++
		}
	}
	return n
}
