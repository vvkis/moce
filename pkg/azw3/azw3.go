// Package azw3 writes standalone Kindle Format 8 (KF8 / .azw3) ebooks in pure Go.
package azw3

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"sort"
	"strings"
	"time"

	"moce/pkg/epub"
)

const (
	nullIndex   = 0xFFFFFFFF
	mobiHdrLen  = 264
	chunkTarget = 8192
)

var flisRecord = []byte{
	'F', 'L', 'I', 'S',
	0, 0, 0, 8,
	0, 0x41, 0, 0,
	0, 0, 0, 0,
	0xFF, 0xFF, 0xFF, 0xFF,
	0, 1, 0, 3,
	0, 0, 0, 3,
	0, 0, 0, 1,
	0xFF, 0xFF, 0xFF, 0xFF,
}

func fcisRecord(textLen uint32) []byte {
	b := make([]byte, 0, 44)
	b = append(b,
		'F', 'C', 'I', 'S',
		0, 0, 0, 0x14,
		0, 0, 0, 0x10,
		0, 0, 0, 2,
		0, 0, 0, 0,
	)
	var u [4]byte
	binary.BigEndian.PutUint32(u[:], textLen)
	b = append(b, u[:]...)
	b = append(b,
		0, 0, 0, 0, 0, 0, 0, 0x28,
		0, 0, 0, 0, 0, 0, 0, 0x28,
		0, 0, 0, 8, 0, 1, 0, 1,
		0, 0, 0, 0,
	)
	return b
}

type pageDoc struct {
	title     string
	bodyAttr  string
	skeleton  []byte
	chunks    [][]byte
	bodyAid   string
	bodyStart int // byte offset of "<body" inside skeleton
	bodyOff   int // byte offset right after "<body ...>" inside skeleton
}

type tocItem struct {
	title    string
	pageIdx  int
	children []tocItem
}

// Write packs an epub.Book into a standalone KF8 (.azw3) file.
func Write(book epub.Book) ([]byte, error) {
	if strings.TrimSpace(book.Title) == "" {
		book.Title = "未命名"
	}
	if strings.TrimSpace(book.Author) == "" {
		book.Author = "佚名"
	}
	if book.Language == "" {
		book.Language = "zh"
	}
	if book.UUID == "" {
		book.UUID = newUUID()
	}
	if book.Modified.IsZero() {
		book.Modified = time.Now().UTC()
	} else {
		book.Modified = book.Modified.UTC()
	}

	// Prepare resources: Kindle-compliant JFIF JPEG cover + dedicated <=300px JPEG thumbnail + optional font.
	// Amazon's native MOBI/KF8 renderer rejects JPEGs without JFIF metadata and drops covers with EXIF segments.
	var resources [][]byte
	coverResIdx := -1 // 0-based index into resources
	thumbResIdx := -1 // 0-based index into resources
	coverRef := ""
	if len(book.Cover) > 0 && isRasterImage(book.Cover, book.CoverMIME) {
		covBytes, thumbBytes, covMIME := prepareKindleCoverAndThumb(book.Cover, book.CoverMIME)
		coverResIdx = len(resources)
		resources = append(resources, covBytes)
		coverRef = fmt.Sprintf("kindle:embed:%s?mime=%s", toBase32(coverResIdx+1, 4), covMIME)
		if len(thumbBytes) > 0 {
			thumbResIdx = len(resources)
			resources = append(resources, thumbBytes)
		} else {
			thumbResIdx = coverResIdx
		}
	}

	fontRef := ""
	hasFonts := false
	if len(book.Font) > 0 {
		fontResIdx := len(resources)
		resources = append(resources, wrapFontRecord(book.Font))
		fontRef = fmt.Sprintf("kindle:embed:%s", toBase32(fontResIdx+1, 4))
		hasFonts = true
	}

	// Build Flow 1 CSS.
	cssFlow := []byte(buildStylesheet(book.Style, fontRef))

	// Build pages: cover page, optional inline TOC placeholder, and chapter/volume pages.
	var pages []pageDoc
	aidSeq := 1
	nextAid := func() string {
		s := toBase32(aidSeq, 1)
		aidSeq++
		return s
	}

	// Page 0: cover page.
	coverPageIdx := 0
	var coverInner string
	if coverRef != "" {
		coverInner = fmt.Sprintf(`<p class="cover-img" aid="%s"><img src="%s" alt="%s"/></p>`, nextAid(), xmlEscape(coverRef), xmlEscape(book.Title))
	} else {
		coverInner = fmt.Sprintf(
			`<div class="cover-frame" aid="%s"><p class="cover-mark" aid="%s">墨册</p><h1 class="cover-title" aid="%s">%s</h1><p class="cover-author" aid="%s">%s</p></div>`,
			nextAid(), nextAid(), nextAid(), xmlEscape(book.Title), nextAid(), xmlEscape(book.Author),
		)
	}
	pages = append(pages, makePage(book.Title, `class="coverpage"`, []string{coverInner}, nextAid()))

	// Optional Page 1: inline TOC (filled after chapter FIDs are known).
	inlineTOCIdx := -1
	if book.InlineTOC {
		inlineTOCIdx = len(pages)
		pages = append(pages, pageDoc{}) // placeholder
	}

	// Build chapter pages & TOC tree.
	tocTree, err := buildChapterPages(book.Chapters, book.VolumePageStyle, &pages, nextAid)
	if err != nil {
		return nil, err
	}
	firstTextPageIdx := 1
	if inlineTOCIdx >= 0 {
		firstTextPageIdx = 2
	}
	if len(pages) <= firstTextPageIdx {
		return nil, fmt.Errorf("没有章节")
	}

	// Compute chunk counts per page before generating inline TOC links if needed.
	// Inline TOC sits before chapters, so its own chunk count shifts chapter FIDs.
	if inlineTOCIdx >= 0 {
		tocAidStart := aidSeq
		tocChunkCount := 1
		for pass := 0; pass < 2; pass++ {
			aidSeq = tocAidStart
			pageFID := computePageFIDs(pages, inlineTOCIdx, tocChunkCount)
			tocChunks := renderInlineTOC(tocTree, !book.HideCoverInTOC, coverPageIdx, pageFID, nextAid)
			if len(tocChunks) == tocChunkCount {
				pages[inlineTOCIdx] = makePage("目录", "", tocChunks, nextAid())
				break
			}
			tocChunkCount = len(tocChunks)
			pages[inlineTOCIdx] = makePage("目录", "", tocChunks, nextAid())
		}
	}

	// Assemble Flow 0 (raw HTML stream) + SKEL table + CHUNK table.
	var flow0 bytes.Buffer
	skels := make([]skelEntry, 0, len(pages))
	chunks := make([]chunkEntry, 0, len(pages))
	pageBodyOffset := make([]int, len(pages))
	pageTotalLen := make([]int, len(pages))
	pageFirstFID := make([]int, len(pages))

	globalFID := 0
	for i, pg := range pages {
		startPos := flow0.Len()
		pageBodyOffset[i] = startPos + pg.bodyStart
		pageFirstFID[i] = globalFID

		flow0.Write(pg.skeleton)
		cp := 0
		for _, ch := range pg.chunks {
			flow0.Write(ch)
			insertPos := startPos + pg.bodyOff + cp
			chunks = append(chunks, chunkEntry{
				insertPos:      insertPos,
				selector:       fmt.Sprintf("P-//*[@aid='%s']", pg.bodyAid),
				fileNumber:     i,
				sequenceNumber: globalFID,
				startPos:       cp,
				length:         len(ch),
			})
			cp += len(ch)
			globalFID++
		}
		pageTotalLen[i] = len(pg.skeleton) + cp
		skels = append(skels, skelEntry{
			fileNumber: i,
			name:       fmt.Sprintf("SKEL%010d", i),
			chunkCount: len(pg.chunks),
			startPos:   startPos,
			length:     len(pg.skeleton),
		})
	}

	// Combine Flow 0 + Flow 1 into the single text stream described by FDST.
	flow0Bytes := flow0.Bytes()
	combined := make([]byte, 0, len(flow0Bytes)+len(cssFlow))
	combined = append(combined, flow0Bytes...)
	combined = append(combined, cssFlow...)

	fdstRec := buildFDST([][2]uint32{
		{0, uint32(len(flow0Bytes))},
		{uint32(len(flow0Bytes)), uint32(len(combined))},
	})

	// Split into strict 4096-byte text records with UTF-8 multibyte overlap (extra_data_flags bit 0).
	rawTextChunks := splitTextRecords(combined)
	records := make([][]byte, 0, len(rawTextChunks)+32)
	records = append(records, nil) // placeholder for Record 0
	uncompressedLengths := make([]int, len(rawTextChunks))
	for i, chunk := range rawTextChunks {
		uncompressedLengths[i] = len(chunk.data)
		comp := compressPalmDOC(chunk.data)
		if len(chunk.overlap) > 0 {
			comp = append(comp, chunk.overlap...)
		}
		comp = append(comp, byte(len(chunk.overlap)))
		records = append(records, comp)
	}
	lastTextRec := len(rawTextChunks)

	// Build NCX entries and apply TBS (Trailing Byte Sequences, extra_data_flags bit 1)
	// directly onto text records 1..lastTextRec, matching Calibre / KindleGen KF8 output.
	ncxEntries := flattenNCX(tocTree, !book.HideCoverInTOC, coverPageIdx, pageBodyOffset, pageTotalLen, pageFirstFID, len(flow0Bytes))
	hasTBS := applyTrailingByteSequences(ncxEntries, records, uncompressedLengths)
	extraDataFlags := uint32(1)
	if hasTBS {
		extraDataFlags |= 2
	}

	// Pad after the last text record so the first INDX record aligns cleanly to 4 bytes.
	textBytesTotal := 0
	for i := 1; i <= lastTextRec; i++ {
		textBytesTotal += len(records[i])
	}
	firstNonTextRec := lastTextRec + 1
	if rem := textBytesTotal % 4; rem != 0 {
		records = append(records, make([]byte, 4-rem))
		firstNonTextRec++
	}

	// Build INDX records: Chunk (FRAG), Skel, Guide, NCX.
	chunkRecs := buildChunkIndex(chunks)
	skelRecs := buildSkelIndex(skels)

	guides := []guideEntry{
		{typ: "text", title: "正文", posFID: [2]int{pageFirstFID[firstTextPageIdx], 0}},
	}
	if inlineTOCIdx >= 0 {
		guides = append(guides, guideEntry{typ: "toc", title: "目录", posFID: [2]int{pageFirstFID[inlineTOCIdx], 0}})
	}
	sort.Slice(guides, func(i, j int) bool { return guides[i].typ < guides[j].typ })
	guideRecs := buildGuideIndex(guides)
	ncxRecs := buildNCXIndex(ncxEntries)

	chunkIdx := uint32(len(records))
	records = append(records, chunkRecs...)
	skelIdx := uint32(len(records))
	records = append(records, skelRecs...)

	guideIdx := uint32(nullIndex)
	if len(guideRecs) > 0 {
		guideIdx = uint32(len(records))
		records = append(records, guideRecs...)
	}
	ncxIdx := uint32(nullIndex)
	if len(ncxRecs) > 0 {
		ncxIdx = uint32(len(records))
		records = append(records, ncxRecs...)
	}

	// Append resources (cover + thumbnail + optional font).
	firstResIdx := uint32(nullIndex)
	if len(resources) > 0 {
		firstResIdx = uint32(len(records))
		records = append(records, resources...)
	}

	// Append FDST, FLIS, FCIS, EOF.
	fdstIdx := uint32(len(records))
	records = append(records, fdstRec)

	flisIdx := uint32(len(records))
	records = append(records, flisRecord)

	fcisIdx := uint32(len(records))
	records = append(records, fcisRecord(uint32(len(combined))))

	records = append(records, []byte{0xE9, 0x8E, '\r', '\n'}) // EOF

	// Build EXTH and Record 0.
	exth := buildEXTH(book, coverResIdx, thumbResIdx, len(resources), pageBodyOffset[firstTextPageIdx])
	records[0] = buildRecord0(record0Params{
		textLen:         uint32(len(combined)),
		lastTextRec:     uint16(lastTextRec),
		firstNonTextRec: uint32(firstNonTextRec),
		firstResRec:     firstResIdx,
		fdstRec:         fdstIdx,
		fdstCount:       2,
		flisRec:         flisIdx,
		fcisRec:         fcisIdx,
		ncxIdx:          ncxIdx,
		chunkIdx:        chunkIdx,
		skelIdx:         skelIdx,
		guideIdx:        guideIdx,
		langCode:        mobiLangCode(book.Language),
		hasFonts:        hasFonts,
		extraDataFlags:  extraDataFlags,
		exth:            exth,
		fullTitle:       []byte(book.Title),
		uid:             randomUID(),
	})

	return buildPalmDB(book.Title, book.Modified, records), nil
}

func makePage(title, bodyAttr string, rawChunks []string, bodyAid string) pageDoc {
	var skel bytes.Buffer
	skel.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	skel.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>`)
	skel.WriteString(xmlEscape(title))
	skel.WriteString(`</title><meta http-equiv="Content-Type" content="text/html; charset=utf-8"/><link rel="stylesheet" type="text/css" href="kindle:flow:0001?mime=text/css"/></head>`)
	bodyStart := skel.Len()
	skel.WriteString(`<body`)
	if bodyAttr != "" {
		skel.WriteByte(' ')
		skel.WriteString(bodyAttr)
	}
	skel.WriteString(` aid="`)
	skel.WriteString(bodyAid)
	skel.WriteString(`">`)
	bodyOff := skel.Len()
	skel.WriteString(`</body></html>`)

	chunkBytes := make([][]byte, 0, len(rawChunks))
	for _, s := range rawChunks {
		chunkBytes = append(chunkBytes, []byte(s))
	}
	if len(chunkBytes) == 0 {
		chunkBytes = append(chunkBytes, []byte{})
	}
	return pageDoc{
		title:     title,
		bodyAttr:  bodyAttr,
		skeleton:  skel.Bytes(),
		chunks:    chunkBytes,
		bodyAid:   bodyAid,
		bodyStart: bodyStart,
		bodyOff:   bodyOff,
	}
}

func countLeafChapters(chs []epub.Chapter) int {
	n := 0
	for _, c := range chs {
		if len(c.Paragraphs) > 0 || len(c.Children) == 0 {
			n++
		}
		n += countLeafChapters(c.Children)
	}
	return n
}

func buildChapterPages(chs []epub.Chapter, volStyle string, pages *[]pageDoc, nextAid func() string) ([]tocItem, error) {
	var walk func([]epub.Chapter) []tocItem
	walk = func(list []epub.Chapter) []tocItem {
		var items []tocItem
		for _, c := range list {
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = "未命名"
			}
			emptyParent := len(c.Paragraphs) == 0 && len(c.Children) > 0
			if emptyParent && volStyle == "skip" {
				children := walk(c.Children)
				if len(children) > 0 {
					items = append(items, tocItem{
						title:    title,
						pageIdx:  children[0].pageIdx,
						children: children,
					})
					continue
				}
			}

			pageIdx := len(*pages)
			var chunks []string
			if emptyParent && volStyle == "divider" {
				var b strings.Builder
				fmt.Fprintf(&b, `<div class="vol-divider" aid="%s"><h1 aid="%s">%s</h1>`, nextAid(), nextAid(), xmlEscape(title))
				if cnt := countLeafChapters(c.Children); cnt > 0 {
					fmt.Fprintf(&b, `<p class="vol-count" aid="%s">共 %d 章</p>`, nextAid(), cnt)
				}
				b.WriteString(`</div>`)
				chunks = []string{b.String()}
			} else {
				chunks = splitChapterChunks(title, c.Paragraphs, nextAid)
			}
			*pages = append(*pages, makePage(title, "", chunks, nextAid()))
			children := walk(c.Children)
			items = append(items, tocItem{
				title:    title,
				pageIdx:  pageIdx,
				children: children,
			})
		}
		return items
	}
	return walk(chs), nil
}

func splitChapterChunks(title string, paragraphs []string, nextAid func() string) []string {
	var chunks []string
	var cur strings.Builder
	fmt.Fprintf(&cur, `<h1 aid="%s">%s</h1>`, nextAid(), xmlEscape(title))
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		frag := fmt.Sprintf("\n<p aid=\"%s\">%s</p>", nextAid(), xmlEscape(p))
		if cur.Len() > 0 && cur.Len()+len(frag) > chunkTarget {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		cur.WriteString(frag)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

func computePageFIDs(pages []pageDoc, placeholderIdx, placeholderChunks int) []int {
	fids := make([]int, len(pages))
	fid := 0
	for i, pg := range pages {
		fids[i] = fid
		if i == placeholderIdx {
			fid += placeholderChunks
		} else {
			fid += len(pg.chunks)
		}
	}
	return fids
}

func renderInlineTOC(tree []tocItem, includeCover bool, coverPageIdx int, pageFID []int, nextAid func() string) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf(`<h1 aid="%s">目录</h1>`, nextAid()))
	if includeCover {
		lines = append(lines, fmt.Sprintf(`<p class="toc-item toc-l1" aid="%s"><a href="kindle:pos:fid:%s:off:%s" aid="%s">封面</a></p>`,
			nextAid(), toBase32(pageFID[coverPageIdx], 4), toBase32(0, 10), nextAid()))
	}
	var walk func([]tocItem, int)
	walk = func(items []tocItem, depth int) {
		cls := depth
		if cls > 4 {
			cls = 4
		}
		for _, it := range items {
			href := fmt.Sprintf("kindle:pos:fid:%s:off:%s", toBase32(pageFID[it.pageIdx], 4), toBase32(0, 10))
			lines = append(lines, fmt.Sprintf(`<p class="toc-item toc-l%d" aid="%s"><a href="%s" aid="%s">%s</a></p>`, cls, nextAid(), href, nextAid(), xmlEscape(it.title)))
			walk(it.children, depth+1)
		}
	}
	walk(tree, 1)

	var chunks []string
	var cur strings.Builder
	for _, ln := range lines {
		if cur.Len() > 0 && cur.Len()+len(ln)+1 > chunkTarget {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
		}
		cur.WriteString(ln)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// flattenNCX builds the KF8 NCX index in breadth-first (level-order) sequence so
// every parent's direct children occupy a contiguous [first_child, last_child] slice,
// matching Calibre & KindleGen's KF8 NCX layout.
func flattenNCX(tree []tocItem, includeCover bool, coverPageIdx int, pageBodyOff, pageLen, pageFID []int, totalFlow0 int) []ncxEntry {
	roots := make([]tocItem, 0, len(tree)+1)
	if includeCover {
		roots = append(roots, tocItem{title: "封面", pageIdx: coverPageIdx})
	}
	roots = append(roots, tree...)
	if len(roots) == 0 {
		return nil
	}

	type queueItem struct {
		item      tocItem
		depth     int
		parentIdx int
	}

	var queue []queueItem
	for _, it := range roots {
		queue = append(queue, queueItem{item: it, depth: 0, parentIdx: -1})
	}

	flat := make([]ncxEntry, 0, len(roots))
	for head := 0; head < len(queue); head++ {
		q := queue[head]
		idx := len(flat)
		entry := ncxEntry{
			index:     idx,
			offset:    pageBodyOff[q.item.pageIdx],
			label:     q.item.title,
			depth:     q.depth,
			hasParent: q.parentIdx >= 0,
			parent:    q.parentIdx,
			posFID:    [2]int{pageFID[q.item.pageIdx], 0},
		}
		flat = append(flat, entry)
		if q.parentIdx >= 0 {
			p := &flat[q.parentIdx]
			if !p.hasChild {
				p.hasChild = true
				p.firstChild = idx
			}
			p.lastChild = idx
		}
		for _, ch := range q.item.children {
			queue = append(queue, queueItem{item: ch, depth: q.depth + 1, parentIdx: idx})
		}
	}

	// Match Calibre's get_next_start: smallest strictly greater offset at depth <= entry.depth,
	// or totalFlow0 if none exists.
	for i := range flat {
		endOff := totalFlow0
		for j := range flat {
			if flat[j].depth <= flat[i].depth && flat[j].offset > flat[i].offset && flat[j].offset < endOff {
				endOff = flat[j].offset
			}
		}
		length := endOff - flat[i].offset
		if length <= 0 {
			length = 1
		}
		flat[i].length = length
	}
	return flat
}

func buildStylesheet(style epub.Style, fontRef string) string {
	var b strings.Builder
	var extra []string
	if fontRef != "" {
		fmt.Fprintf(&b, `@font-face{font-family:"EmbeddedFont";src:url("%s");font-weight:normal;font-style:normal;}`, fontRef)
		extra = append(extra, `font-family:"EmbeddedFont",serif`)
	}
	if style.FontSizePt > 0 {
		extra = append(extra, fmt.Sprintf("font-size:%gem", float64(style.FontSizePt)/12))
	}
	var body []string
	if style.MarginPercent > 0 {
		body = append(body, fmt.Sprintf("margin:%g%%", style.MarginPercent))
	}
	if style.LineHeight > 0 {
		body = append(body, fmt.Sprintf("line-height:%g", style.LineHeight))
	}
	body = append(body, "text-align:justify")
	body = append(extra, body...)
	fmt.Fprintf(&b, "body{%s;}", strings.Join(body, ";"))
	b.WriteString(`h1{font-size:1.5em;text-align:center;margin:2em 0 1.5em;page-break-after:avoid;}`)
	var para []string
	if style.IndentEm > 0 {
		para = append(para, fmt.Sprintf("text-indent:%gem", style.IndentEm))
	}
	if style.ParagraphEm > 0 {
		para = append(para, fmt.Sprintf("margin:%gem 0", style.ParagraphEm))
	}
	para = append(para, "orphans:2", "widows:2")
	fmt.Fprintf(&b, "p{%s;}", strings.Join(para, ";"))
	b.WriteString(`body.coverpage{margin:0;padding:0;text-align:center;}`)
	b.WriteString(`body.coverpage p,body.coverpage h1{text-indent:0;text-align:center;}`)
	b.WriteString(`.cover-frame{margin:10% 8%;border:2px solid #7a2a24;padding:16% 8%;}`)
	b.WriteString(`.cover-mark{letter-spacing:0.45em;font-size:0.85em;margin:0 0 1.2em;}`)
	b.WriteString(`.cover-title{font-size:1.85em;line-height:1.35;margin:0.4em 0;}`)
	b.WriteString(`.cover-author{font-size:1.05em;margin-top:1.4em;}`)
	b.WriteString(`.cover-img{margin:0;text-indent:0;}`)
	b.WriteString(`.cover-img img{max-width:100%;}`)
	b.WriteString(`.vol-divider{margin:24% 8% 0;padding:1.8em 0;border-top:0.08em solid #8c7b6b;border-bottom:0.08em solid #8c7b6b;text-align:center;}`)
	b.WriteString(`.vol-divider h1{margin:0;font-size:1.7em;line-height:1.45;text-align:center;}`)
	b.WriteString(`.vol-count{margin:1em 0 0;text-indent:0;text-align:center;font-size:0.85em;letter-spacing:0.15em;color:#666;}`)
	b.WriteString(`.toc-item{text-indent:0;margin:0.35em 0;line-height:1.6;}`)
	b.WriteString(`.toc-item a{text-decoration:none;color:inherit;}`)
	b.WriteString(`.toc-l2{padding-left:1.3em;}`)
	b.WriteString(`.toc-l3{padding-left:2.6em;}`)
	b.WriteString(`.toc-l4{padding-left:3.9em;}`)
	return b.String()
}

func buildFDST(flows [][2]uint32) []byte {
	var buf bytes.Buffer
	buf.WriteString("FDST")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], 12)
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(flows)))
	buf.Write(hdr[:])
	for _, f := range flows {
		binary.BigEndian.PutUint32(hdr[0:4], f[0])
		binary.BigEndian.PutUint32(hdr[4:8], f[1])
		buf.Write(hdr[:])
	}
	return buf.Bytes()
}

func buildEXTH(book epub.Book, coverResIdx, thumbResIdx, numResources, startOffset int) []byte {
	var body bytes.Buffer
	count := 0
	putBytes := func(code uint32, val []byte) {
		var h [8]byte
		binary.BigEndian.PutUint32(h[0:4], code)
		binary.BigEndian.PutUint32(h[4:8], uint32(8+len(val)))
		body.Write(h[:])
		body.Write(val)
		count++
	}
	putStr := func(code uint32, s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		putBytes(code, []byte(s))
	}
	putU32 := func(code, v uint32) {
		var u [4]byte
		binary.BigEndian.PutUint32(u[:], v)
		putBytes(code, u[:])
	}

	// Author(s)
	for _, a := range splitAuthors(book.Author) {
		putStr(100, a)
	}
	putStr(101, book.Publisher)
	putStr(103, book.Description)
	for _, id := range book.Identifiers {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(strings.ToLower(id), "urn:isbn:") {
			putStr(104, id[len("urn:isbn:"):])
			break
		}
	}
	for _, subj := range book.Subjects {
		putStr(105, subj)
	}
	pubDate := strings.TrimSpace(book.Published)
	if pubDate == "" {
		pubDate = book.Modified.Format(time.RFC3339)
	}
	putStr(106, pubDate)
	putStr(108, "moce")
	putStr(112, "moce:"+book.UUID)
	putStr(113, book.UUID) // ASIN / UUID
	putU32(116, uint32(startOffset))
	putU32(125, uint32(numResources))
	if thumbResIdx >= 0 {
		// Calibre / KindleGen encodes thumbnail_offset (0-based) into EXTH 129.
		// When cover=0 and thumbnail=1, toBase32(1, 4) == "0001".
		uriIdx := thumbResIdx
		if uriIdx == 0 {
			uriIdx = 1
		}
		putStr(129, fmt.Sprintf("kindle:embed:%s", toBase32(uriIdx, 4)))
	}
	if numResources > 0 {
		putU32(131, 0)
	}
	if coverResIdx >= 0 {
		putU32(201, uint32(coverResIdx))
		if thumbResIdx >= 0 {
			putU32(202, uint32(thumbResIdx))
		} else {
			putU32(202, uint32(coverResIdx))
		}
		putU32(203, 0)
	}
	// KindleGen 2.9 / Calibre KF8 compatibility creator markers
	putU32(204, 200)
	putU32(205, 2)
	putU32(206, 9)
	putU32(207, 0)
	putStr(501, "EBOK")
	putStr(503, book.Title)
	putStr(524, book.Language)
	putStr(528, "true")
	putStr(535, "0730-890adc2")

	// Match Calibre writer8/exth.py: EXTH header length excludes trailing alignment pad,
	// and 1..4 zero padding bytes are always appended after the EXTH records before full_title.
	exthDataLen := body.Len()
	pad := 4 - (exthDataLen % 4)
	var out bytes.Buffer
	out.WriteString("EXTH")
	var h [8]byte
	binary.BigEndian.PutUint32(h[0:4], uint32(exthDataLen+12))
	binary.BigEndian.PutUint32(h[4:8], uint32(count))
	out.Write(h[:])
	out.Write(body.Bytes())
	out.Write(make([]byte, pad))
	return out.Bytes()
}

type record0Params struct {
	textLen         uint32
	lastTextRec     uint16
	firstNonTextRec uint32
	firstResRec     uint32
	fdstRec         uint32
	fdstCount       uint32
	flisRec         uint32
	fcisRec         uint32
	ncxIdx          uint32
	chunkIdx        uint32
	skelIdx         uint32
	guideIdx        uint32
	langCode        uint32
	hasFonts        bool
	extraDataFlags  uint32
	exth            []byte
	fullTitle       []byte
	uid             uint32
}

func buildRecord0(p record0Params) []byte {
	// Record 0 fixed portion = 16 (PalmDOC) + 264 (MOBI v8) = 280 bytes.
	hdr := make([]byte, 16+mobiHdrLen)

	// PalmDOC header (0..15)
	binary.BigEndian.PutUint16(hdr[0:2], 2) // PalmDOC LZ77 compression
	binary.BigEndian.PutUint32(hdr[4:8], p.textLen)
	binary.BigEndian.PutUint16(hdr[8:10], p.lastTextRec)
	binary.BigEndian.PutUint16(hdr[10:12], maxRecordSize)

	// MOBI header (16..279)
	copy(hdr[16:20], "MOBI")
	binary.BigEndian.PutUint32(hdr[20:24], mobiHdrLen)
	binary.BigEndian.PutUint32(hdr[24:28], 2)     // Book
	binary.BigEndian.PutUint32(hdr[28:32], 65001) // UTF-8
	binary.BigEndian.PutUint32(hdr[32:36], p.uid)
	binary.BigEndian.PutUint32(hdr[36:40], 8) // KF8 file version

	// 40..79: 10 x NULL index fields
	for off := 40; off < 80; off += 4 {
		binary.BigEndian.PutUint32(hdr[off:off+4], nullIndex)
	}

	titleOffset := uint32(len(hdr) + len(p.exth))
	binary.BigEndian.PutUint32(hdr[80:84], p.firstNonTextRec)
	binary.BigEndian.PutUint32(hdr[84:88], titleOffset)
	binary.BigEndian.PutUint32(hdr[88:92], uint32(len(p.fullTitle)))
	binary.BigEndian.PutUint32(hdr[92:96], p.langCode)
	binary.BigEndian.PutUint32(hdr[104:108], 8) // min version = KF8
	binary.BigEndian.PutUint32(hdr[108:112], p.firstResRec)

	exthFlags := uint32(0x50)
	if p.hasFonts {
		exthFlags |= 0x1000
	}
	binary.BigEndian.PutUint32(hdr[128:132], exthFlags)
	binary.BigEndian.PutUint32(hdr[164:168], nullIndex)
	binary.BigEndian.PutUint32(hdr[168:172], nullIndex) // DRM offset

	binary.BigEndian.PutUint32(hdr[192:196], p.fdstRec)
	binary.BigEndian.PutUint32(hdr[196:200], p.fdstCount)
	binary.BigEndian.PutUint32(hdr[200:204], p.fcisRec)
	binary.BigEndian.PutUint32(hdr[204:208], 1)
	binary.BigEndian.PutUint32(hdr[208:212], p.flisRec)
	binary.BigEndian.PutUint32(hdr[212:216], 1)

	binary.BigEndian.PutUint32(hdr[224:228], nullIndex) // SRCS
	binary.BigEndian.PutUint32(hdr[232:236], nullIndex)
	binary.BigEndian.PutUint32(hdr[236:240], nullIndex)
	binary.BigEndian.PutUint32(hdr[240:244], p.extraDataFlags)

	// KF8 indices
	binary.BigEndian.PutUint32(hdr[244:248], p.ncxIdx)
	binary.BigEndian.PutUint32(hdr[248:252], p.chunkIdx)
	binary.BigEndian.PutUint32(hdr[252:256], p.skelIdx)
	binary.BigEndian.PutUint32(hdr[256:260], nullIndex) // DATP
	binary.BigEndian.PutUint32(hdr[260:264], p.guideIdx)

	binary.BigEndian.PutUint32(hdr[264:268], nullIndex)
	binary.BigEndian.PutUint32(hdr[272:276], nullIndex)

	var out bytes.Buffer
	out.Write(hdr)
	out.Write(p.exth)
	out.Write(p.fullTitle)
	// Standard 8192-byte DTP padding matching Calibre / KindleGen.
	out.Write(make([]byte, 8192))
	return out.Bytes()
}

func buildPalmDB(title string, mod time.Time, records [][]byte) []byte {
	name := asciiPalmName(title)
	now := uint32(mod.Unix())
	n := len(records)

	var buf bytes.Buffer
	buf.Write(name) // 32 bytes
	var meta [28]byte
	binary.BigEndian.PutUint32(meta[4:8], now)
	binary.BigEndian.PutUint32(meta[8:12], now)
	buf.Write(meta[:])
	buf.WriteString("BOOKMOBI")
	var tail [10]byte
	binary.BigEndian.PutUint32(tail[0:4], uint32(2*n-1))
	binary.BigEndian.PutUint16(tail[8:10], uint16(n))
	buf.Write(tail[:])

	offset := uint32(buf.Len() + 8*n + 2)
	var recInfo [8]byte
	for i, rec := range records {
		binary.BigEndian.PutUint32(recInfo[0:4], offset)
		uid := uint32(2 * i)
		recInfo[4] = 0
		recInfo[5] = byte(uid >> 16)
		recInfo[6] = byte(uid >> 8)
		recInfo[7] = byte(uid)
		buf.Write(recInfo[:])
		offset += uint32(len(rec))
	}
	buf.Write([]byte{0, 0})
	for _, rec := range records {
		buf.Write(rec)
	}
	return buf.Bytes()
}

func asciiPalmName(title string) []byte {
	var b strings.Builder
	for _, r := range title {
		if r >= 0x20 && r <= 0x7E {
			if r == ' ' {
				b.WriteByte('_')
			} else {
				b.WriteRune(r)
			}
		}
		if b.Len() >= 31 {
			break
		}
	}
	s := b.String()
	if s == "" {
		s = "moce_book"
	}
	out := make([]byte, 32)
	copy(out, s)
	return out
}

func wrapFontRecord(fontBytes []byte) []byte {
	// Zlib-compressed MOBI FONT record matching Calibre write_font_record.
	var zbuf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&zbuf, zlib.BestCompression)
	payload := fontBytes
	flags := uint32(0)
	if err == nil {
		if _, werr := zw.Write(fontBytes); werr == nil {
			if cerr := zw.Close(); cerr == nil {
				payload = zbuf.Bytes()
				flags = 1 // zlib compressed
			}
		}
	}
	hdr := make([]byte, 24)
	copy(hdr[0:4], "FONT")
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(fontBytes)))
	binary.BigEndian.PutUint32(hdr[8:12], flags)
	binary.BigEndian.PutUint32(hdr[12:16], 24) // data offset
	binary.BigEndian.PutUint32(hdr[16:20], 0)  // xor key len
	binary.BigEndian.PutUint32(hdr[20:24], 24) // xor key start
	out := make([]byte, 0, 24+len(payload))
	out = append(out, hdr...)
	out = append(out, payload...)
	return out
}

func isRasterImage(data []byte, mime string) bool {
	if strings.HasPrefix(mime, "image/jpeg") || strings.HasPrefix(mime, "image/png") || strings.HasPrefix(mime, "image/gif") {
		return true
	}
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return true
	}
	return false
}

// jfifAPP0 is a standard 18-byte JFIF 1.01 APP0 segment required by Amazon's Kindle
// native JPEG renderer (Go's stdlib image/jpeg omits JFIF headers by default).
var jfifAPP0 = []byte{
	0xFF, 0xE0, 0x00, 0x10,
	'J', 'F', 'I', 'F', 0x00,
	0x01, 0x01,
	0x00,
	0x00, 0x01,
	0x00, 0x01,
	0x00, 0x00,
}

// normalizeJPEGForKindle ensures a JPEG stream begins with a JFIF APP0 segment
// and strips any APP1 (EXIF/XMP) segments that break Kindle cover display.
func normalizeJPEGForKindle(raw []byte) []byte {
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return raw
	}
	var out bytes.Buffer
	out.Grow(len(raw) + len(jfifAPP0))
	out.Write([]byte{0xFF, 0xD8})
	out.Write(jfifAPP0)

	i := 2
	for i < len(raw) {
		if raw[i] != 0xFF {
			out.Write(raw[i:])
			return out.Bytes()
		}
		for i < len(raw) && raw[i] == 0xFF {
			i++
		}
		if i >= len(raw) {
			break
		}
		marker := raw[i]
		i++
		if marker == 0xDA || marker == 0xD9 {
			out.WriteByte(0xFF)
			out.WriteByte(marker)
			out.Write(raw[i:])
			return out.Bytes()
		}
		if (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			out.WriteByte(0xFF)
			out.WriteByte(marker)
			continue
		}
		if i+2 > len(raw) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(raw[i : i+2]))
		if segLen < 2 || i+segLen > len(raw) {
			out.WriteByte(0xFF)
			out.WriteByte(marker)
			out.Write(raw[i:])
			return out.Bytes()
		}
		// Drop existing APP0 (replaced by canonical JFIF above) and APP1 (EXIF/XMP).
		if marker != 0xE0 && marker != 0xE1 {
			out.WriteByte(0xFF)
			out.WriteByte(marker)
			out.Write(raw[i : i+segLen])
		}
		i += segLen
	}
	return out.Bytes()
}

func flattenImageToOpaque(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

func scaleImageFit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || (sw <= maxW && sh <= maxH) {
		return src
	}
	dw, dh := sw, sh
	if dw > maxW {
		dh = dh * maxW / dw
		dw = maxW
	}
	if dh > maxH {
		dw = dw * maxH / dh
		dh = maxH
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy0 := b.Min.Y + y*sh/dh
		sy1 := b.Min.Y + (y+1)*sh/dh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > b.Max.Y {
			sy1 = b.Max.Y
		}
		for x := 0; x < dw; x++ {
			sx0 := b.Min.X + x*sw/dw
			sx1 := b.Min.X + (x+1)*sw/dw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > b.Max.X {
				sx1 = b.Max.X
			}
			var sr, sg, sb, cnt uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bl, _ := src.At(sx, sy).RGBA()
					sr += uint64(r >> 8)
					sg += uint64(g >> 8)
					sb += uint64(bl >> 8)
					cnt++
				}
			}
			if cnt == 0 {
				cnt = 1
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(sr / cnt),
				G: uint8(sg / cnt),
				B: uint8(sb / cnt),
				A: 0xFF,
			})
		}
	}
	return dst
}

func encodeKindleJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return normalizeJPEGForKindle(buf.Bytes()), nil
}

// prepareKindleCoverAndThumb converts the cover into a JFIF-tagged, EXIF-free JPEG
// and builds a separate <=300x300 (<=32KB) JPEG thumbnail matching Calibre's Resources.
func prepareKindleCoverAndThumb(data []byte, mime string) (coverBytes, thumbBytes []byte, outMIME string) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			norm := normalizeJPEGForKindle(data)
			return norm, nil, "image/jpeg"
		}
		if mime == "" {
			mime = sniffMIME(data)
		}
		return data, nil, mime
	}

	opaque := flattenImageToOpaque(img)
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		coverBytes = normalizeJPEGForKindle(data)
	} else {
		if enc, encErr := encodeKindleJPEG(opaque, 90); encErr == nil {
			coverBytes = enc
		} else {
			return data, nil, sniffMIME(data)
		}
	}

	thumbImg := scaleImageFit(opaque, 300, 300)
	for q := 85; q >= 45; q -= 10 {
		enc, encErr := encodeKindleJPEG(thumbImg, q)
		if encErr != nil {
			break
		}
		thumbBytes = enc
		if len(thumbBytes) <= 32*1024 {
			break
		}
	}
	return coverBytes, thumbBytes, "image/jpeg"
}

func sniffMIME(data []byte) string {
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	return "image/jpeg"
}

func splitAuthors(author string) []string {
	author = strings.TrimSpace(author)
	if author == "" {
		return []string{"佚名"}
	}
	if !strings.Contains(author, " & ") {
		return []string{author}
	}
	var out []string
	for _, p := range strings.Split(author, " & ") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"佚名"}
	}
	return out
}

func mobiLangCode(lang string) uint32 {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "zh-hans":
		return 0x0004
	case "zh-tw", "zh-hk", "zh-hant":
		return 0x0404
	case "en", "en-us":
		return 0x0409
	case "ja", "jp":
		return 0x0411
	case "ko":
		return 0x0412
	default:
		return 0x0004
	}
}

func xmlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func newUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

func randomUID() uint32 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return binary.BigEndian.Uint32(b[:])
}
