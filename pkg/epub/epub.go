// Derived from moon (https://github.com/AnkioTomas/moon, GNU GPLv3).
// Chapter documents follow convert/html2epub.lua wrapXhtml: XHTML 1.1,
// an inline stylesheet, and an h1 title. There is no h2 and no px font size.

// Package epub writes an EPUB 3 file with an EPUB 2 NCX fallback.
package epub

import (
	"crypto/rand"
	"fmt"
	"path"
	"strings"
	"time"
)

const mimetype = "application/epub+zip"

// Chapter is one node in the table of contents.
// A node with paragraphs becomes its own XHTML file. Children are nested in the TOC.
type Chapter struct {
	Title      string
	Paragraphs []string
	Children   []Chapter
}

// Style is the EPUB stylesheet.
// FontSizePt 0 does not set a body font size, so Kindle and KOReader can still change it.
// A positive value is a scale against a 12pt baseline, written as em, never px.
// LineHeight 0, MarginPercent 0, ParagraphEm 0, and IndentEm 0 omit those declarations.
// The shipped default matches moon convert/html2epub.lua.
type Style struct {
	FontSizePt    int
	LineHeight    float64
	MarginPercent float64
	ParagraphEm   float64
	IndentEm      float64
	EmbedFont     bool
	FontExt       string // .ttf or .otf
	FontFormat    string // truetype or opentype
}

// MoonDefaultStyle is the moon plugin stylesheet: 5% margin, 1.8 line height,
// 2em indent, and 0.45em paragraph spacing. No font family and no px font size.
func MoonDefaultStyle() Style {
	return Style{LineHeight: 1.8, MarginPercent: 5, ParagraphEm: 0.45, IndentEm: 2}
}

// Book is the document to pack.
type Book struct {
	Title    string
	Author   string
	Language string
	UUID     string
	Modified time.Time
	Chapters []Chapter
	Style    Style
	// Publisher, Subjects, and the series fields are omitted from the OPF when empty.
	Publisher   string
	Description string
	Subjects    []string
	Identifiers []string // extra ids besides the package uuid, already normalized
	Published   string   // YYYY, YYYY-MM, or YYYY-MM-DD
	Series      string
	SeriesIndex string // decimal text, used only when Series is set
	Rating      int    // 1–5 stars; 0 omits the rating
	// Cover is optional image bytes. When empty, a text cover page and an SVG
	// thumbnail are generated from the title and author.
	Cover      []byte
	CoverName  string // cover.jpg, cover.png, cover.svg
	CoverMIME  string
	Font       []byte
	FontExt    string
	FontMIME   string
	FontFormat string
}

type doc struct {
	id, href, title string
	paragraphs      []string
}

type tocNode struct {
	title    string
	href     string
	children []tocNode
}

// Write packs the book into EPUB bytes.
func Write(book Book) ([]byte, error) {
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
	docs, toc, err := prepare(book.Chapters)
	if err != nil {
		return nil, err
	}
	textCover := false
	if len(book.Cover) == 0 {
		book.Cover = svgCover(book.Title, book.Author)
		book.CoverName = "cover.svg"
		book.CoverMIME = "image/svg+xml"
		textCover = true
	}
	if book.CoverName == "" || book.CoverMIME == "" {
		return nil, fmt.Errorf("封面缺少文件名或类型")
	}
	if len(book.Font) > 0 {
		book.Style.EmbedFont = true
		if book.FontExt == "" {
			book.FontExt = ".ttf"
		}
		if book.FontFormat == "" {
			book.FontFormat = "truetype"
		}
		if book.FontMIME == "" {
			book.FontMIME = "font/ttf"
		}
	}

	uid := "urn:uuid:" + book.UUID
	modified := book.Modified.Format("2006-01-02T15:04:05Z")
	css := stylesheet(book.Style)

	files := []ZipFile{
		{Name: "mimetype", Data: []byte(mimetype), Store: true},
		{Name: "META-INF/container.xml", Data: []byte(containerXML)},
	}
	var oebps []ZipFile
	oebps = append(oebps,
		ZipFile{Name: "OEBPS/style.css", Data: []byte(css)},
		ZipFile{Name: "OEBPS/" + book.CoverName, Data: book.Cover},
		ZipFile{Name: "OEBPS/cover.xhtml", Data: []byte(coverXHTML(book, textCover))},
		ZipFile{Name: "OEBPS/nav.xhtml", Data: []byte(navXHTML(book.Title, toc))},
		ZipFile{Name: "OEBPS/toc.ncx", Data: []byte(ncxXML(book.Title, book.Author, uid, toc))},
	)
	for _, d := range docs {
		oebps = append(oebps, ZipFile{Name: "OEBPS/" + d.href, Data: []byte(chapterXHTML(d.title, d.paragraphs, book.Style))})
	}
	if len(book.Font) > 0 {
		oebps = append(oebps, ZipFile{Name: "OEBPS/fonts/embedded" + book.FontExt, Data: book.Font})
	}
	opf := contentOPF(book, uid, modified, docs, textCover)
	oebps = append(oebps, ZipFile{Name: "OEBPS/content.opf", Data: []byte(opf)})
	files = append(files, oebps...)
	return WriteZip(files, book.Modified)
}

func prepare(chapters []Chapter) ([]doc, []tocNode, error) {
	var docs []doc
	n := 0
	var walk func([]Chapter) []tocNode
	walk = func(chs []Chapter) []tocNode {
		var nodes []tocNode
		for _, c := range chs {
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = "未命名"
			}
			n++
			id := fmt.Sprintf("ch%d", n)
			href := fmt.Sprintf("chapter_%03d.xhtml", n)
			docs = append(docs, doc{id: id, href: href, title: title, paragraphs: c.Paragraphs})
			children := walk(c.Children)
			nodes = append(nodes, tocNode{title: title, href: href, children: children})
		}
		return nodes
	}
	toc := walk(chapters)
	if len(docs) == 0 {
		return nil, nil, fmt.Errorf("没有章节")
	}
	return docs, toc, nil
}

const containerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

func contentOPF(book Book, uid, modified string, docs []doc, textCover bool) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookId" version="3.0" xml:lang="%s">`+"\n", xmlEscape(book.Language))
	b.WriteString(`  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	writeMetadata(&b, book, uid, modified)
	b.WriteString(`  </metadata>` + "\n")
	b.WriteString(`  <manifest>` + "\n")
	b.WriteString(`    <item id="css" href="style.css" media-type="text/css"/>` + "\n")
	fmt.Fprintf(&b, `    <item id="cover-image" href="%s" media-type="%s" properties="cover-image"/>`+"\n", xmlEscape(book.CoverName), xmlEscape(book.CoverMIME))
	b.WriteString(`    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/>` + "\n")
	b.WriteString(`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	b.WriteString(`    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>` + "\n")
	for _, d := range docs {
		fmt.Fprintf(&b, `    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n", d.id, xmlEscape(d.href))
	}
	if book.Style.EmbedFont {
		fmt.Fprintf(&b, `    <item id="font1" href="fonts/embedded%s" media-type="%s"/>`+"\n", xmlEscape(book.FontExt), xmlEscape(book.FontMIME))
	}
	b.WriteString(`  </manifest>` + "\n")
	b.WriteString(`  <spine toc="ncx">` + "\n")
	b.WriteString(`    <itemref idref="cover"/>` + "\n")
	for _, d := range docs {
		fmt.Fprintf(&b, `    <itemref idref="%s"/>`+"\n", d.id)
	}
	b.WriteString(`  </spine>` + "\n")
	b.WriteString(`  <guide>` + "\n")
	b.WriteString(`    <reference type="cover" title="封面" href="cover.xhtml"/>` + "\n")
	b.WriteString(`    <reference type="text" title="正文" href="` + xmlEscape(docs[0].href) + `"/>` + "\n")
	b.WriteString(`  </guide>` + "\n")
	b.WriteString(`</package>` + "\n")
	_ = textCover
	return b.String()
}

func writeMetadata(b *strings.Builder, book Book, uid, modified string) {
	fmt.Fprintf(b, `    <dc:identifier id="BookId">%s</dc:identifier>`+"\n", xmlEscape(uid))
	for i, id := range book.Identifiers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		fmt.Fprintf(b, `    <dc:identifier id="id-%d">%s</dc:identifier>`+"\n", i+1, xmlEscape(id))
	}
	fmt.Fprintf(b, `    <dc:title>%s</dc:title>`+"\n", xmlEscape(book.Title))
	if desc := strings.TrimSpace(book.Description); desc != "" {
		fmt.Fprintf(b, `    <dc:description>%s</dc:description>`+"\n", xmlEscape(desc))
	}
	fmt.Fprintf(b, `    <dc:language>%s</dc:language>`+"\n", xmlEscape(book.Language))
	for _, author := range authorNames(book.Author) {
		fmt.Fprintf(b, `    <dc:creator>%s</dc:creator>`+"\n", xmlEscape(author))
	}
	if publisher := strings.TrimSpace(book.Publisher); publisher != "" {
		fmt.Fprintf(b, `    <dc:publisher>%s</dc:publisher>`+"\n", xmlEscape(publisher))
	}
	for _, tag := range book.Subjects {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		fmt.Fprintf(b, `    <dc:subject>%s</dc:subject>`+"\n", xmlEscape(tag))
	}
	if published := strings.TrimSpace(book.Published); published != "" {
		fmt.Fprintf(b, `    <dc:date>%s</dc:date>`+"\n", xmlEscape(published))
		fmt.Fprintf(b, `    <meta property="dcterms:issued">%s</meta>`+"\n", xmlEscape(published))
	}
	fmt.Fprintf(b, `    <meta property="dcterms:modified">%s</meta>`+"\n", modified)
	if series := strings.TrimSpace(book.Series); series != "" {
		index := strings.TrimSpace(book.SeriesIndex)
		if index == "" {
			index = "1"
		}
		fmt.Fprintf(b, `    <meta property="belongs-to-collection" id="series">%s</meta>`+"\n", xmlEscape(series))
		b.WriteString(`    <meta refines="#series" property="collection-type">series</meta>` + "\n")
		fmt.Fprintf(b, `    <meta refines="#series" property="group-position">%s</meta>`+"\n", xmlEscape(index))
		fmt.Fprintf(b, `    <meta name="calibre:series" content="%s"/>`+"\n", xmlEscape(series))
		fmt.Fprintf(b, `    <meta name="calibre:series_index" content="%s"/>`+"\n", xmlEscape(index))
	}
	if book.Rating >= 1 && book.Rating <= 5 {
		// Calibre stores stars × 2, so five stars is 10.
		fmt.Fprintf(b, `    <meta name="calibre:rating" content="%d"/>`+"\n", book.Rating*2)
	}
	b.WriteString(`    <meta name="cover" content="cover-image"/>` + "\n")
}

func authorNames(author string) []string {
	author = strings.TrimSpace(author)
	if author == "" {
		return []string{"佚名"}
	}
	if !strings.Contains(author, " & ") {
		return []string{author}
	}
	var out []string
	for _, part := range strings.Split(author, " & ") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"佚名"}
	}
	return out
}

func stylesheet(style Style) string {
	var b strings.Builder
	var extra []string
	if style.EmbedFont {
		ext := style.FontExt
		if ext == "" {
			ext = ".ttf"
		}
		format := style.FontFormat
		if format == "" {
			format = "truetype"
		}
		fmt.Fprintf(&b, `@font-face{font-family:"EmbeddedFont";src:url("fonts/embedded%s") format("%s");font-weight:normal;font-style:normal;}`, ext, format)
		extra = append(extra, `font-family:"EmbeddedFont",serif`)
	}
	if style.FontSizePt > 0 {
		// Relative to a 12pt baseline. em scales with the reader's font size; px does not.
		extra = append(extra, fmt.Sprintf("font-size:%gem", float64(style.FontSizePt)/12))
	}
	// Property order matches the moon stylesheet when the extras are absent.
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
	return b.String()
}

func coverXHTML(book Book, textCover bool) string {
	var body strings.Builder
	body.WriteString(`<body class="coverpage">`)
	if textCover {
		body.WriteString(`<div class="cover-frame">`)
		body.WriteString(`<p class="cover-mark">墨册</p>`)
		fmt.Fprintf(&body, `<h1 class="cover-title">%s</h1>`, xmlEscape(book.Title))
		fmt.Fprintf(&body, `<p class="cover-author">%s</p>`, xmlEscape(book.Author))
		body.WriteString(`</div>`)
	} else {
		fmt.Fprintf(&body, `<p class="cover-img"><img src="%s" alt="%s"/></p>`, xmlEscape(path.Base(book.CoverName)), xmlEscape(book.Title))
	}
	body.WriteString(`</body>`)
	return xhtmlShell(book.Title, body.String())
}

func chapterXHTML(title string, paragraphs []string, style Style) string {
	var body strings.Builder
	fmt.Fprintf(&body, "<h1>%s</h1>", xmlEscape(title))
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		fmt.Fprintf(&body, "\n<p>%s</p>", xmlEscape(p))
	}
	return moonChapter(title, body.String(), style)
}

// moonChapter is convert/html2epub.lua wrapXhtml. The stylesheet is inline so
// KOReader's epub.css (p margin 0, indent 1.2em, heading margin 0.7em) does
// not replace it. Titles are h1 for both volumes and chapters.
func moonChapter(title, fragment string, style Style) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE html>` + "\n" +
		`<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="zh">` + "\n" +
		`<head>` + "\n" +
		`<title>` + xmlEscape(title) + `</title>` + "\n" +
		`<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>` + "\n" +
		`<style type="text/css">` + "\n" +
		moonRules(style) + "\n" +
		`</style>` + "\n" +
		`</head>` + "\n" +
		`<body>` + "\n" +
		fragment + "\n" +
		`</body>` + "\n" +
		`</html>` + "\n"
}

func moonRules(style Style) string {
	css := stylesheet(style)
	if i := strings.Index(css, "body.coverpage"); i >= 0 {
		css = css[:i]
	}
	css = strings.Replace(css, "}h1{", "}\nh1{", 1)
	css = strings.Replace(css, "}p{", "}\np{", 1)
	return css
}

func xhtmlShell(title, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE html>` + "\n" +
		`<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="zh" lang="zh">` +
		`<head><meta charset="UTF-8"/><title>` + xmlEscape(title) + `</title>` +
		`<link rel="stylesheet" type="text/css" href="style.css"/></head>` +
		body + `</html>` + "\n"
}

func navXHTML(title string, toc []tocNode) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE html>` + "\n")
	b.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="zh" lang="zh">`)
	b.WriteString(`<head><meta charset="UTF-8"/><title>目录</title><link rel="stylesheet" type="text/css" href="style.css"/></head>`)
	b.WriteString(`<body><nav epub:type="toc" id="toc"><h1>目录</h1><ol>`)
	b.WriteString(`<li><a href="cover.xhtml">封面</a></li>`)
	writeNavList(&b, toc)
	b.WriteString(`</ol></nav></body></html>` + "\n")
	_ = title
	return b.String()
}

func writeNavList(b *strings.Builder, nodes []tocNode) {
	for _, n := range nodes {
		fmt.Fprintf(b, `<li><a href="%s">%s</a>`, xmlEscape(n.href), xmlEscape(n.title))
		if len(n.children) > 0 {
			b.WriteString(`<ol>`)
			writeNavList(b, n.children)
			b.WriteString(`</ol>`)
		}
		b.WriteString(`</li>`)
	}
}

func ncxXML(title, author, uid string, toc []tocNode) string {
	depth := 1
	var measure func([]tocNode, int)
	measure = func(nodes []tocNode, d int) {
		if d > depth {
			depth = d
		}
		for _, n := range nodes {
			measure(n.children, d+1)
		}
	}
	measure(toc, 1)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">` + "\n")
	b.WriteString(`  <head>` + "\n")
	fmt.Fprintf(&b, `    <meta name="dtb:uid" content="%s"/>`+"\n", xmlEscape(uid))
	fmt.Fprintf(&b, `    <meta name="dtb:depth" content="%d"/>`+"\n", depth)
	b.WriteString(`    <meta name="dtb:totalPageCount" content="0"/>` + "\n")
	b.WriteString(`    <meta name="dtb:maxPageNumber" content="0"/>` + "\n")
	b.WriteString(`  </head>` + "\n")
	fmt.Fprintf(&b, `  <docTitle><text>%s</text></docTitle>`+"\n", xmlEscape(title))
	fmt.Fprintf(&b, `  <docAuthor><text>%s</text></docAuthor>`+"\n", xmlEscape(author))
	b.WriteString(`  <navMap>` + "\n")
	play := 0
	writeNavPoints(&b, append([]tocNode{{title: "封面", href: "cover.xhtml"}}, toc...), &play, "    ")
	b.WriteString(`  </navMap>` + "\n")
	b.WriteString(`</ncx>` + "\n")
	return b.String()
}

func writeNavPoints(b *strings.Builder, nodes []tocNode, play *int, indent string) {
	for _, n := range nodes {
		*play++
		id := *play
		fmt.Fprintf(b, `%s<navPoint id="navPoint-%d" playOrder="%d">`+"\n", indent, id, id)
		fmt.Fprintf(b, `%s  <navLabel><text>%s</text></navLabel>`+"\n", indent, xmlEscape(n.title))
		fmt.Fprintf(b, `%s  <content src="%s"/>`+"\n", indent, xmlEscape(n.href))
		if len(n.children) > 0 {
			writeNavPoints(b, n.children, play, indent+"  ")
		}
		fmt.Fprintf(b, `%s</navPoint>`+"\n", indent)
	}
}

func svgCover(title, author string) []byte {
	lines := wrapRunes(title, 10)
	if len(lines) == 0 {
		lines = []string{title}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" viewBox="0 0 900 1200">`)
	b.WriteString(`<rect width="900" height="1200" fill="#f4ecdf"/>`)
	b.WriteString(`<rect x="46" y="46" width="808" height="1108" fill="none" stroke="#9d2c2c" stroke-width="3"/>`)
	b.WriteString(`<rect x="62" y="62" width="776" height="1076" fill="none" stroke="#9d2c2c" stroke-width="1"/>`)
	b.WriteString(`<text x="450" y="280" text-anchor="middle" font-size="28" fill="#9d2c2c" font-family="serif">墨册</text>`)
	base := 560 - (len(lines)-1)*40
	for i, line := range lines {
		fmt.Fprintf(&b, `<text x="450" y="%d" text-anchor="middle" font-size="64" fill="#1c1612" font-family="serif">%s</text>`, base+i*84, xmlEscape(line))
	}
	fmt.Fprintf(&b, `<text x="450" y="%d" text-anchor="middle" font-size="32" fill="#5c4636" font-family="serif">%s</text>`, base+len(lines)*84+40, xmlEscape(author))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

func wrapRunes(s string, n int) []string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) == 0 {
		return nil
	}
	var out []string
	for len(rs) > 0 {
		if len(rs) <= n {
			out = append(out, string(rs))
			break
		}
		out = append(out, string(rs[:n]))
		rs = rs[n:]
	}
	return out
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
