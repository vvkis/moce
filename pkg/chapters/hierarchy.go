// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package chapters

import (
	"regexp"
	"strconv"
	"strings"

	"moce/pkg/textclean"
)

// Port of the moon plugin (commit c31efda): chapterTitle + chapter_hierarchy.organize.
// A repeated 卷 number stays in the previous volume unless chapter numbering restarts at 1.

const maxTocEntryBytes = 120

var (
	chineseNumeral = map[rune]bool{}
	reChapterEn    = regexp.MustCompile(`(?i)^chapter\s+[0-9A-Za-z]+[\s:.\-]?.*$`)
	reSectionEn    = regexp.MustCompile(`(?i)^section\s+[0-9A-Za-z]+[\s:.\-]?.*$`)
	rePageEn       = regexp.MustCompile(`(?i)^page\s+[0-9]+[\s:.\-]?.*$`)
	specialTitles  = []string{"序章", "序言", "前言", "楔子", "引子", "尾声", "后记", "番外", "终章", "最终章", "完本感言"}
	invalidBu      = map[rune]bool{'门': true, '队': true, '属': true, '分': true, '件': true, '落': true}
	chapterMarkers = []string{"章", "回", "节", "集", "幕"}
	headingMarkers = []string{"章", "回", "节", "集", "幕", "卷", "部", "篇"}
)

func init() {
	for _, r := range []rune("零〇一二三四五六七八九十百千万两") {
		chineseNumeral[r] = true
	}
}

type hEntry struct {
	title string
	lines []string
	depth int
	toc   bool
}

type volInfo struct {
	id         string
	depth      int
	restarts   bool
	laterStart bool
}

func titleKey(title string) string {
	title = strings.ReplaceAll(title, "\u3000", " ")
	title = strings.ReplaceAll(title, "\u00a0", " ")
	return strings.Join(strings.Fields(title), " ")
}

func isNumberToken(value string) bool {
	value = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\u3000' {
			return -1
		}
		return r
	}, value)
	if value == "" {
		return false
	}
	if isDigits(value) || isRoman(value) {
		return true
	}
	for _, r := range value {
		if !chineseNumeral[r] {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < '０' || r > '９') {
			return false
		}
	}
	return true
}

func isRoman(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case 'I', 'V', 'X', 'L', 'C', 'D', 'M', 'i', 'v', 'x', 'l', 'c', 'd', 'm':
		default:
			return false
		}
	}
	return true
}

func restartsAtOne(n string) bool {
	switch n {
	case "一", "I", "i":
		return true
	}
	var b strings.Builder
	for _, r := range n {
		if r >= '０' && r <= '９' {
			b.WriteByte(byte('0' + r - '０'))
			continue
		}
		b.WriteRune(r)
	}
	v, err := strconv.Atoi(b.String())
	return err == nil && v == 1
}

func splitMarker(line, marker string) (number, suffix string, ok bool) {
	if !strings.HasPrefix(line, "第") {
		return "", "", false
	}
	rest := trimLeftSpace(line[len("第"):])
	i := strings.Index(rest, marker)
	if i <= 0 {
		return "", "", false
	}
	number = strings.TrimSpace(rest[:i])
	if number == "" || strings.Contains(number, "第") {
		return "", "", false
	}
	return number, rest[i+len(marker):], true
}

func trimLeftSpace(s string) string {
	return strings.TrimLeft(s, " \t\u3000")
}

func chapterNumber(title string) (string, bool) {
	title = titleKey(title)
	for _, marker := range chapterMarkers {
		number, _, ok := splitMarker(title, marker)
		if ok && isNumberToken(number) {
			return strings.ReplaceAll(number, " ", ""), true
		}
	}
	return "", false
}

// volumeTitle returns the normalized 第N卷/部 and, for “第一卷-第一章 标题”, the chapter tail.
// 部 uses the same restart rule as 卷 so an existing volume heading still parents the following chapters.
func volumeTitle(title string) (volume, chapter string, ok bool) {
	title = titleKey(title)
	for _, marker := range []string{"卷", "部"} {
		number, suffix, found := splitMarker(title, marker)
		if !found || !isNumberToken(number) {
			continue
		}
		volume = "第" + strings.ReplaceAll(number, " ", "") + marker
		tail := strings.TrimSpace(stripVolumeSep(suffix))
		if _, isCh := chapterNumber(tail); isCh {
			return volume, tail, true
		}
		return volume, "", true
	}
	return "", "", false
}

func stripVolumeSep(s string) string {
	s = strings.TrimLeft(s, " \t")
	for len(s) > 0 && (s[0] == '-' || s[0] == ':') {
		s = s[1:]
		s = strings.TrimLeft(s, " \t")
	}
	if strings.HasPrefix(s, "：") {
		s = strings.TrimPrefix(s, "：")
		s = strings.TrimLeft(s, " \t")
	}
	if strings.HasPrefix(s, "—") {
		s = strings.TrimPrefix(s, "—")
		s = strings.TrimLeft(s, " \t")
	}
	return s
}

func stripEquals(line string) string {
	for strings.HasPrefix(line, "=") {
		line = strings.TrimLeft(line, "=")
		line = strings.TrimLeft(line, " \t")
	}
	for {
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, "=") {
			line = strings.TrimRight(trimmed, "=")
			continue
		}
		return strings.TrimSpace(line)
	}
}

func chapterTitle(line string) (string, bool) {
	line = stripEquals(trimOuter(line))
	if line == "" {
		return "", false
	}
	if alt := stripDatePrefix(line); alt != line {
		if t, ok := chapterTitleCore(alt); ok {
			return t, true
		}
	}
	return chapterTitleCore(line)
}

func chapterTitleCore(line string) (string, bool) {
	if len(line) > maxTocEntryBytes {
		return "", false
	}
	if reChapterEn.MatchString(line) || reSectionEn.MatchString(line) || rePageEn.MatchString(line) {
		return line, true
	}
	if strings.HasPrefix(line, "第") {
		for _, marker := range headingMarkers {
			number, suffix, ok := splitMarker(line, marker)
			if !ok || !isNumberToken(number) {
				continue
			}
			if marker == "部" {
				rs := []rune(suffix)
				if len(rs) > 0 && invalidBu[rs[0]] {
					continue
				}
			}
			return line, true
		}
	}
	for _, title := range specialTitles {
		if line == title || specialSuffix(line, title) {
			return line, true
		}
	}
	return "", false
}

func specialSuffix(line, title string) bool {
	if !strings.HasPrefix(line, title) {
		return false
	}
	rest := line[len(title):]
	if rest == "" {
		return false
	}
	if strings.HasPrefix(rest, "：") {
		return len(rest) > len("：")
	}
	r := rest[0]
	if r == ' ' || r == '\t' || r == ':' || r == '-' {
		return len(rest) > 1
	}
	return false
}

func infer(entries []hEntry) []hEntry {
	out := make([]hEntry, 0, len(entries)+4)
	current := ""
	for _, e := range entries {
		key := titleKey(e.title)
		if key != "" && rootSpecial.MatchString(key) {
			current = ""
			e.depth = 1
			e.toc = true
			out = append(out, e)
			continue
		}
		vol, ch, ok := volumeTitle(key)
		if ok && ch == "" {
			current = vol
			e.depth = 1
			e.toc = true
			out = append(out, e)
			continue
		}
		if ok && ch != "" {
			if vol != current {
				out = append(out, hEntry{title: vol, depth: 1, toc: true})
				current = vol
			}
			e.title = ch
		}
		if current != "" {
			e.depth = 2
		} else {
			e.depth = 1
		}
		e.toc = true
		out = append(out, e)
	}
	return out
}

func volumeStarts(entries []hEntry) []*volInfo {
	info := make([]*volInfo, len(entries))
	var next *volInfo
	var first string
	var hasFirst bool
	for i := len(entries) - 1; i >= 0; i-- {
		vol, ch, ok := volumeTitle(titleKey(entries[i].title))
		if ok && ch == "" {
			restarts := hasFirst && restartsAtOne(first)
			later := next != nil && next.id == vol && next.depth == entries[i].depth && next.restarts
			next = &volInfo{id: vol, depth: entries[i].depth, restarts: restarts, laterStart: later}
			info[i] = next
			first = ""
			hasFirst = false
			continue
		}
		src := ch
		if src == "" {
			src = entries[i].title
		}
		if n, ok := chapterNumber(src); ok {
			first = n
			hasFirst = true
		}
	}
	return info
}

type tocNode struct {
	title       string
	depth       int
	index       int
	parent      *tocNode
	hasChildren bool
}

// preserve keeps a nested source TOC. A flat TOC returns nil so infer can run.
func preserve(entries []hEntry, headings []Heading) []hEntry {
	if len(headings) == 0 {
		return nil
	}
	var stack []*tocNode
	by := map[string][]*tocNode{}
	nested := false
	for i, h := range headings {
		key := titleKey(strings.TrimSpace(h.Title))
		if key == "" {
			continue
		}
		depth := h.Depth
		if depth < 1 {
			depth = 1
		}
		if depth > len(stack)+1 {
			depth = len(stack) + 1
		}
		for len(stack) >= depth {
			stack = stack[:len(stack)-1]
		}
		var parent *tocNode
		if depth > 1 && len(stack) >= depth-1 {
			parent = stack[depth-2]
		}
		node := &tocNode{title: strings.TrimSpace(h.Title), depth: depth, index: i + 1, parent: parent}
		if parent != nil {
			parent.hasChildren = true
		}
		stack = append(stack, node)
		by[key] = append(by[key], node)
		if depth > 1 {
			nested = true
		}
	}
	if !nested {
		return nil
	}
	next := map[string]int{}
	matches := make([]*tocNode, len(entries))
	cursor := 0
	matchedChild := false
	for i, entry := range entries {
		key := titleKey(entry.title)
		list := by[key]
		j := next[key]
		for j < len(list) && list[j].index <= cursor {
			j++
		}
		if j < len(list) {
			matches[i] = list[j]
			cursor = list[j].index
			next[key] = j + 1
			if list[j].depth > 1 {
				matchedChild = true
			}
			continue
		}
		next[key] = j
	}
	if !matchedChild {
		return nil
	}
	emitted := map[*tocNode]bool{}
	var out []hEntry
	var addParent func(*tocNode)
	addParent = func(n *tocNode) {
		if n == nil || emitted[n] {
			return
		}
		addParent(n.parent)
		out = append(out, hEntry{title: n.title, depth: n.depth, toc: true})
		emitted[n] = true
	}
	var previous *tocNode
	for i, entry := range entries {
		if m := matches[i]; m != nil {
			addParent(m.parent)
			entry.depth = m.depth
			entry.toc = true
			emitted[m] = true
			previous = m
		} else if previous != nil {
			extra := 0
			if previous.hasChildren {
				extra = 1
			}
			entry.depth = previous.depth + extra
			entry.toc = true
		} else {
			entry.depth = 1
			entry.toc = true
		}
		out = append(out, entry)
	}
	return out
}

func organize(entries []hEntry, headings []Heading) []hEntry {
	if kept := preserve(entries, headings); kept != nil {
		entries = kept
	} else {
		entries = infer(entries)
	}
	vols := volumeStarts(entries)
	parents := map[int]int{0: -1}
	depthOf := map[int]int{-1: 0}
	lastVolume := map[int]int{}
	hasLast := map[int]bool{}
	enclosing := map[int]string{}

	for i := range entries {
		depth := entries[i].depth
		parent, ok := parents[depth-1]
		if !ok {
			parent = -1
		}
		vol := vols[i]
		prev, hasPrev := lastVolume[parent]
		if !hasLast[parent] {
			hasPrev = false
		}
		inside := vol != nil && enclosing[parent] == vol.id
		repeated := inside || (vol != nil && hasPrev && vols[prev] != nil && vols[prev].id == vol.id)
		if vol != nil && !vol.restarts && (repeated || vol.laterStart) {
			entries[i].toc = false
			if !inside && hasPrev {
				parents[depth] = prev
			} else {
				parents[depth] = parent
			}
			continue
		}
		entries[i].depth = depthOf[parent] + 1
		parents[depth] = i
		depthOf[i] = entries[i].depth
		if vol != nil {
			lastVolume[parent] = i
			hasLast[parent] = true
			enclosing[i] = vol.id
		} else {
			hasLast[parent] = false
			if id, ok := enclosing[parent]; ok {
				enclosing[i] = id
			}
		}
	}
	return entries
}

func blankLines(lines []string) bool {
	for _, line := range lines {
		if trimOuter(line) != "" {
			return false
		}
	}
	return true
}

// dropTocEchoes folds a printed TOC page: three or more consecutive empty headings
// become paragraphs of the previous section. A single empty volume title stays.
func dropTocEchoes(entries []hEntry) []hEntry {
	remaining := map[string]int{}
	for _, e := range entries {
		remaining[e.title]++
	}
	var kept, run []hEntry
	flush := func() {
		if len(run) >= 3 {
			if len(kept) == 0 {
				kept = append(kept, hEntry{title: "前言", toc: true})
			}
			lines := kept[len(kept)-1].lines
			for _, e := range run {
				lines = append(lines, "", e.title)
			}
			kept[len(kept)-1].lines = lines
		} else {
			kept = append(kept, run...)
		}
		run = nil
	}
	for _, e := range entries {
		remaining[e.title]--
		if !blankLines(e.lines) {
			flush()
			kept = append(kept, e)
			continue
		}
		if remaining[e.title] == 0 {
			run = append(run, e)
		}
	}
	flush()
	return kept
}

func mergeHidden(entries []hEntry) []hEntry {
	kept := make([]hEntry, 0, len(entries))
	for _, e := range entries {
		if e.toc || len(kept) == 0 {
			kept = append(kept, e)
			continue
		}
		prev := &kept[len(kept)-1]
		prev.lines = append(prev.lines, "", e.title, "")
		prev.lines = append(prev.lines, e.lines...)
	}
	return kept
}

// groupFanwai makes 番外 a parent of the following 章 until the next 卷 or terminal heading.
func groupFanwai(entries []hEntry) []hEntry {
	in := false
	for i := range entries {
		key := titleKey(entries[i].title)
		if reFanwai.MatchString(key) {
			entries[i].depth = 1
			in = true
			continue
		}
		if !in {
			continue
		}
		if rootSpecial.MatchString(key) {
			in = false
			continue
		}
		if vol, ch, ok := volumeTitle(key); ok && ch == "" && vol != "" {
			in = false
			continue
		}
		entries[i].depth = 2
	}
	return entries
}

// nestByRank turns a flat heading list into 部 > 卷 > 章 > 节.
// Rank comes from the marker, not from how the title is worded after it.
// 番外 sits with 卷, so the following 章 belong to it until the next 卷 or 部.
// 后记 and the other root specials stay at the top and do not adopt later headings.
func nestByRank(entries []hEntry) []hEntry {
	type open struct {
		rank  int
		depth int
	}
	var stack []open
	for i := range entries {
		// 番外 is its own top-level parent. The following chapters belong to it
		// until the next volume or part, matching the existing fanwai grouping.
		if reFanwai.MatchString(titleKey(entries[i].title)) {
			entries[i].depth = 1
			stack = []open{{rank: 2, depth: 1}}
			continue
		}
		rank, parentable := markerRank(entries[i].title)
		if !parentable {
			entries[i].depth = 1
			stack = nil
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].rank >= rank {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			entries[i].depth = 1
		} else {
			entries[i].depth = stack[len(stack)-1].depth + 1
		}
		stack = append(stack, open{rank: rank, depth: entries[i].depth})
	}
	return entries
}

func markerRank(title string) (rank int, parentable bool) {
	key := titleKey(title)
	if key == "" || rootSpecial.MatchString(key) {
		return 0, false
	}
	if reFanwai.MatchString(key) {
		return 2, true
	}
	if strings.HasPrefix(key, "第") {
		for _, item := range []struct {
			marker string
			rank   int
		}{
			{"篇", 1},
			{"部", 1},
			{"卷", 2},
			{"节", 4},
			{"章", 3},
			{"回", 3},
			{"集", 3},
			{"幕", 3},
		} {
			number, _, ok := splitMarker(key, item.marker)
			if ok && isNumberToken(number) {
				return item.rank, true
			}
		}
	}
	return 3, true
}

func treeFrom(entries []hEntry, clean textclean.Options) []Node {
	var roots []Node
	type frame struct {
		depth int
		path  []int
	}
	var stack []frame
	for _, e := range entries {
		paras := textclean.Paragraphs(e.lines, clean)
		title := e.title
		if title == "" {
			if len(paras) == 0 {
				continue
			}
			title = "前言"
		}
		depth := e.depth
		if depth < 1 {
			depth = 1
		}
		node := Node{Title: title, Paragraphs: paras}
		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}
		var path []int
		if len(stack) == 0 {
			roots = append(roots, node)
			path = []int{len(roots) - 1}
		} else {
			parent := nodeAt(&roots, stack[len(stack)-1].path)
			parent.Children = append(parent.Children, node)
			path = append(append([]int{}, stack[len(stack)-1].path...), len(parent.Children)-1)
		}
		stack = append(stack, frame{depth: depth, path: path})
	}
	return roots
}

func nodeAt(roots *[]Node, path []int) *Node {
	n := &(*roots)[path[0]]
	for _, i := range path[1:] {
		n = &n.Children[i]
	}
	return n
}
