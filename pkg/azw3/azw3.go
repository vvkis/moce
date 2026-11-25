// Package azw3 writes standalone Kindle Format 8 (KF8 / .azw3) ebooks in pure Go.
package azw3

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
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
	title    string
	bodyAttr string
	skeleton []byte
	chunks   [][]byte
	bodyAid  string
	bodyOff  int // byte offset right after <body ...> inside skeleton
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

	// Prepare resources (cover image + optional embedded font).
	var resources [][]byte
	coverResIdx := -1 // 0-based index into resources
	coverRef := ""
	coverMIME := ""
	if len(book.Cover) > 0 && isRasterImage(book.Cover, book.CoverMIME) {
		coverResIdx = len(resources)
		resources = append(resources, book.Cover)
		coverMIME = book.CoverMIME
		if coverMIME == "" {
			coverMIME = sniffMIME(book.Cover)
		}
		coverRef = fmt.Sprintf("kindle:embed:%s?mime=%s", toBase32(coverResIdx+1, 4), coverMIME)
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
		s := toBase32(aidSeq, 4)
		aidSeq++
		return s
	}

	// Page 0: cover page.
	coverPageIdx := 0
	var coverInner string
	if coverRef != "" {
		coverInner = fmt.Sprintf(`<p class="cover-img"><img src="%s" alt="%s"/></p>`, xmlEscape(coverRef), xmlEscape(book.Title))
	} else {
		coverInner = fmt.Sprintf(
			`<div class="cover-frame"><p class="cover-mark">墨册</p><h1 class="cover-title">%s</h1><p class="cover-author">%s</p></div>`,
			xmlEscape(book.Title), xmlEscape(book.Author),
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
		// First pass: estimate FIDs assuming inline TOC has 1 chunk; if inline TOC
		// splits into K chunks, regenerate links with exact K offset.
		tocChunkCount := 1
		for pass := 0; pass < 2; pass++ {
			pageFID := computePageFIDs(pages, inlineTOCIdx, tocChunkCount)
			tocChunks := renderInlineTOC(tocTree, !book.HideCoverInTOC, coverPageIdx, pageFID)
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
	pageStartOffset := make([]int, len(pages))
	pageTotalLen := make([]int, len(pages))
	pageFirstFID := make([]int, len(pages))

	globalFID := 0
	for i, pg := range pages {
		startPos := flow0.Len()
		pageStartOffset[i] = startPos
		pageFirstFID[i] = globalFID

		flow0.Write(pg.skeleton)
		cp := 0
		for _, ch := range pg.chunks {
			flow0.Write(ch)
			insertPos := startPos + pg.bodyOff + cp
			chunks = append(chunks, chunkEntry{
				insertPos:      insertPos,
				selector:       fmt.Sprintf("0-//*[@aid='%s']", pg.bodyAid),
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

	// Split and PalmDOC-compress text records.
	rawTextChunks := splitTextRecords(combined)
	records := make([][]byte, 0, len(rawTextChunks)+32)
	records = append(records, nil) // placeholder for Record 0
	textBytesTotal := 0
	for _, chunk := range rawTextChunks {
		comp := compressPalmDOC(chunk)
		records = append(records, comp)
		textBytesTotal += len(comp)
	}
	lastTextRec := len(rawTextChunks)
	firstNonTextRec := lastTextRec + 1
	if rem := textBytesTotal % 4; rem != 0 {
		records = append(records, make([]byte, 4-rem))
		firstNonTextRec++
	}

	// Build INDX records: Chunk (FRAG), Skel, Guide, NCX.
	chunkRecs := buildChunkIndex(chunks)
	skelRecs := buildSkelIndex(skels)

	guides := []guideEntry{
		{typ: "cover", title: "封面", posFID: [2]int{pageFirstFID[coverPageIdx], 0}},
		{typ: "text", title: "正文", posFID: [2]int{pageFirstFID[firstTextPageIdx], 0}},
	}
	if inlineTOCIdx >= 0 {
		guides = append(guides, guideEntry{typ: "toc", title: "目录", posFID: [2]int{pageFirstFID[inlineTOCIdx], 0}})
	}
	guideRecs := buildGuideIndex(guides)

	ncxEntries := flattenNCX(tocTree, !book.HideCoverInTOC, coverPageIdx, pageStartOffset, pageTotalLen, pageFirstFID, len(flow0Bytes))
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

	// Append resources (cover + font).
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
	exth := buildEXTH(book, coverResIdx, len(resources), pageStartOffset[firstTextPageIdx])
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
		exth:            exth,
		fullTitle:       []byte(book.Title),
		uid:             randomUID(),
	})

	return buildPalmDB(book.Title, book.Modified, records), nil
}

func makePage(title, bodyAttr string, rawChunks []string, bodyAid string) pageDoc {
	var skel bytes.Buffer
	skel.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	skel.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="zh"><head><title>`)
	skel.WriteString(xmlEscape(title))
	skel.WriteString(`</title><link rel="stylesheet" type="text/css" href="kindle:flow:0001?mime=text/css"/></head><body`)
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
		title:    title,
		bodyAttr: bodyAttr,
		skeleton: skel.Bytes(),
		chunks:   chunkBytes,
		bodyAid:  bodyAid,
		bodyOff:  bodyOff,
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
				b.WriteString(`<div class="vol-divider"><h1>`)
				b.WriteString(xmlEscape(title))
				b.WriteString(`</h1>`)
				if cnt := countLeafChapters(c.Children); cnt > 0 {
					fmt.Fprintf(&b, `<p class="vol-count">共 %d 章</p>`, cnt)
				}
				b.WriteString(`</div>`)
				chunks = []string{b.String()}
			} else {
				chunks = splitChapterChunks(title, c.Paragraphs)
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

func splitChapterChunks(title string, paragraphs []string) []string {
	var chunks []string
	var cur strings.Builder
	fmt.Fprintf(&cur, "<h1>%s</h1>", xmlEscape(title))
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		frag := "\n<p>" + xmlEscape(p) + "</p>"
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

func renderInlineTOC(tree []tocItem, includeCover bool, coverPageIdx int, pageFID []int) []string {
	var lines []string
	lines = append(lines, `<h1>目录</h1>`)
	if includeCover {
		lines = append(lines, fmt.Sprintf(`<p class="toc-item toc-l1"><a href="kindle:pos:fid:%s:off:%s">封面</a></p>`,
			toBase32(pageFID[coverPageIdx], 4), toBase32(0, 10)))
	}
	var walk func([]tocItem, int)
	walk = func(items []tocItem, depth int) {
		cls := depth
		if cls > 4 {
			cls = 4
		}
		for _, it := range items {
			href := fmt.Sprintf("kindle:pos:fid:%s:off:%s", toBase32(pageFID[it.pageIdx], 4), toBase32(0, 10))
			lines = append(lines, fmt.Sprintf(`<p class="toc-item toc-l%d"><a href="%s">%s</a></p>`, cls, href, xmlEscape(it.title)))
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

func flattenNCX(tree []tocItem, includeCover bool, coverPageIdx int, pageOff, pageLen, pageFID []int, totalFlow0 int) []ncxEntry {
	var flat []ncxEntry
	if includeCover {
		flat = append(flat, ncxEntry{
			index:  0,
			offset: pageOff[coverPageIdx],
			length: pageLen[coverPageIdx],
			label:  "封面",
			depth:  0,
			posFID: [2]int{pageFID[coverPageIdx], 0},
		})
	}
	var walk func([]tocItem, int, int)
	walk = func(items []tocItem, depth int, parentIdx int) {
		firstIdx := -1
		lastIdx := -1
		for _, it := range items {
			idx := len(flat)
			if firstIdx < 0 {
				firstIdx = idx
			}
			lastIdx = idx
			entry := ncxEntry{
				index:     idx,
				offset:    pageOff[it.pageIdx],
				length:    pageLen[it.pageIdx],
				label:     it.title,
				depth:     depth,
				hasParent: parentIdx >= 0,
				parent:    parentIdx,
				posFID:    [2]int{pageFID[it.pageIdx], 0},
			}
			flat = append(flat, entry)
			if len(it.children) > 0 {
				walk(it.children, depth+1, idx)
			}
		}
		if parentIdx >= 0 && firstIdx >= 0 {
			flat[parentIdx].hasChild = true
			flat[parentIdx].firstChild = firstIdx
			flat[parentIdx].lastChild = lastIdx
		}
	}
	walk(tree, 0, -1)

	// Adjust section lengths to span until the next entry offset (or end of flow0).
	for i := range flat {
		nextOff := totalFlow0
		for j := i + 1; j < len(flat); j++ {
			if flat[j].offset > flat[i].offset {
				nextOff = flat[j].offset
				break
			}
		}
		if span := nextOff - flat[i].offset; span > 0 {
			flat[i].length = span
		}
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

func buildEXTH(book epub.Book, coverResIdx, numResources, startOffset int) []byte {
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
	putStr(106, book.Published)
	putStr(113, book.UUID) // ASIN / UUID
	putU32(116, uint32(startOffset))
	putU32(125, uint32(numResources))
	if coverResIdx >= 0 {
		putStr(129, fmt.Sprintf("kindle:embed:%s", toBase32(coverResIdx+1, 4)))
	}
	if numResources > 0 {
		putU32(131, 0)
	}
	if coverResIdx >= 0 {
		putU32(201, uint32(coverResIdx))
		putU32(202, uint32(coverResIdx))
		putU32(203, 0)
	}
	// KindleGen2 compatibility creator markers
	putU32(204, 2)
	putU32(205, 2)
	putU32(206, 9)
	putU32(207, 0)
	putStr(501, "EBOK")
	putStr(503, book.Title)
	putStr(524, book.Language)

	rawLen := 12 + body.Len()
	pad := (4 - (rawLen % 4)) % 4
	var out bytes.Buffer
	out.WriteString("EXTH")
	var h [8]byte
	binary.BigEndian.PutUint32(h[0:4], uint32(rawLen+pad))
	binary.BigEndian.PutUint32(h[4:8], uint32(count))
	out.Write(h[:])
	out.Write(body.Bytes())
	if pad > 0 {
		out.Write(make([]byte, pad))
	}
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
	binary.BigEndian.PutUint32(hdr[240:244], 0) // extra_data_flags = 0 (clean UTF-8 split)

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
	// Align and append modest padding (2048 bytes like standard MOBI/KF8).
	out.Write(make([]byte, 2048))
	return align4(out.Bytes())
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
	// Uncompressed MOBI FONT record (24-byte header + raw TTF/OTF bytes).
	hdr := make([]byte, 24)
	copy(hdr[0:4], "FONT")
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(fontBytes)))
	binary.BigEndian.PutUint32(hdr[8:12], 0)  // flags: uncompressed, unencrypted
	binary.BigEndian.PutUint32(hdr[12:16], 24) // data offset
	out := make([]byte, 0, 24+len(fontBytes))
	out = append(out, hdr...)
	out = append(out, fontBytes...)
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
		return 0x0804
	case "zh-tw", "zh-hk", "zh-hant":
		return 0x0404
	case "en", "en-us":
		return 0x0409
	case "ja", "jp":
		return 0x0411
	case "ko":
		return 0x0412
	default:
		return 0x0804
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
