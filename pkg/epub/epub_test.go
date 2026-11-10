package epub

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func sampleBook() Book {
	return Book{
		Title:       `河灯 & 夜`,
		Author:      `沈小满 <著>`,
		Language:    "zh",
		Publisher:   "试印社",
		Subjects:    []string{"玄幻", "完结"},
		Identifiers: []string{"urn:isbn:9790000000001"},
		Published:   "2026-11-08",
		Series:      "河岸",
		SeriesIndex: "1.5",
		Rating:      4,
		UUID:        "12345678-1234-4234-8234-123456789abc",
		Modified:    time.Date(2026, 11, 8, 1, 2, 3, 0, time.UTC),
		Style:       MoonDefaultStyle(),
		Chapters: []Chapter{
			{
				Title: "第一卷 上游",
				Children: []Chapter{
					{Title: "第一章 纸灯", Paragraphs: []string{"他把灯放进河里。", `A & B <C> "D"`}},
					{Title: "第二章 码头", Paragraphs: []string{"码头上空无一人。"}},
				},
			},
			{Title: "后记", Paragraphs: []string{"写完了。\x01"}},
		},
	}
}

func TestMimetypeIsFirstAndStored(t *testing.T) {
	data, err := Write(sampleBook())
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(data[0:4]) != 0x04034b50 {
		t.Fatal("missing local header")
	}
	if binary.LittleEndian.Uint16(data[8:10]) != 0 {
		t.Fatal("mimetype is compressed")
	}
	nameLen := binary.LittleEndian.Uint16(data[26:28])
	extraLen := binary.LittleEndian.Uint16(data[28:30])
	if extraLen != 0 {
		t.Fatalf("extra field %d", extraLen)
	}
	name := string(data[30 : 30+nameLen])
	if name != "mimetype" {
		t.Fatalf("first name %s", name)
	}
	compSize := binary.LittleEndian.Uint32(data[18:22])
	start := int(nameLen) + 30
	payload := string(data[start : start+int(compSize)])
	if payload != "application/epub+zip" {
		t.Fatalf("payload %q", payload)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store || len(zr.File[0].Extra) != 0 {
		t.Fatalf("central %+v extra %d", zr.File[0].FileHeader, len(zr.File[0].Extra))
	}
}

func TestStructureEscapeAndTOC(t *testing.T) {
	data, err := Write(sampleBook())
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	for _, name := range []string{
		"mimetype",
		"META-INF/container.xml",
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/toc.ncx",
		"OEBPS/style.css",
		"OEBPS/cover.xhtml",
		"OEBPS/cover.svg",
		"OEBPS/chapter_001.xhtml",
		"OEBPS/chapter_002.xhtml",
		"OEBPS/chapter_003.xhtml",
		"OEBPS/chapter_004.xhtml",
	} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if !strings.Contains(string(files["META-INF/container.xml"]), `full-path="OEBPS/content.opf"`) {
		t.Fatal("container rootfile")
	}
	opf := string(files["OEBPS/content.opf"])
	for _, needle := range []string{
		`unique-identifier="BookId"`,
		`<dc:identifier id="BookId">urn:uuid:12345678-1234-4234-8234-123456789abc</dc:identifier>`,
		`<dc:language>zh</dc:language>`,
		`<dc:title>河灯 &amp; 夜</dc:title>`,
		`<dc:creator>沈小满 &lt;著&gt;</dc:creator>`,
		`<dc:publisher>试印社</dc:publisher>`,
		`<dc:subject>玄幻</dc:subject>`,
		`<dc:subject>完结</dc:subject>`,
		`<dc:date>2026-11-08</dc:date>`,
		`<meta property="dcterms:issued">2026-11-08</meta>`,
		`urn:isbn:9790000000001`,
		`<meta property="belongs-to-collection" id="series">河岸</meta>`,
		`<meta name="calibre:series" content="河岸"/>`,
		`<meta name="calibre:series_index" content="1.5"/>`,
		`<meta name="calibre:rating" content="8"/>`,
		`<meta property="dcterms:modified">2026-11-08T01:02:03Z</meta>`,
		`properties="nav"`,
		`spine toc="ncx"`,
		`id="cover-image"`,
		`properties="cover-image"`,
		`idref="ch1"`,
		`idref="ch4"`,
	} {
		if !strings.Contains(opf, needle) {
			t.Fatalf("opf missing %s", needle)
		}
	}
	css := string(files["OEBPS/style.css"])
	assertReaderControlledCSS(t, css)
	ch := string(files["OEBPS/chapter_002.xhtml"])
	if !strings.Contains(ch, "A &amp; B &lt;C&gt; &quot;D&quot;") {
		t.Fatalf("escape failed: %s", ch)
	}
	if strings.Contains(ch, "\x01") {
		t.Fatal("control char leaked")
	}
	nav := string(files["OEBPS/nav.xhtml"])
	if !strings.Contains(nav, `epub:type="toc"`) || !strings.Contains(nav, "<ol>") {
		t.Fatal("nav")
	}
	vol := strings.Index(nav, "第一卷 上游")
	ch1 := strings.Index(nav, "第一章 纸灯")
	if vol < 0 || ch1 < vol {
		t.Fatalf("nav order vol %d ch %d", vol, ch1)
	}
	ncx := string(files["OEBPS/toc.ncx"])
	if !strings.Contains(ncx, `content="urn:uuid:12345678-1234-4234-8234-123456789abc"`) {
		t.Fatal("ncx uid")
	}
	if strings.Index(ncx, "第一卷 上游") > strings.Index(ncx, "第一章 纸灯") {
		t.Fatal("ncx nesting order")
	}
	orders := []string{`playOrder="1"`, `playOrder="2"`, `playOrder="3"`, `playOrder="4"`, `playOrder="5"`}
	last := -1
	for _, o := range orders {
		i := strings.Index(ncx, o)
		if i < 0 || i < last {
			t.Fatalf("playOrder %s at %d", o, i)
		}
		last = i
	}
	for name, body := range files {
		if strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".ncx") || strings.HasSuffix(name, ".opf") || strings.HasSuffix(name, ".svg") {
			if err := wellFormed(body); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	cover := string(files["OEBPS/cover.xhtml"])
	if !strings.Contains(cover, "河灯 &amp; 夜") || !strings.Contains(cover, "沈小满") {
		t.Fatalf("text cover missing title: %s", cover)
	}
}

func TestEmbeddedFontAndPNGCover(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	book := sampleBook()
	book.Cover = buf.Bytes()
	book.CoverName = "cover.png"
	book.CoverMIME = "image/png"
	book.Font = []byte("fake-font-bytes")
	book.FontExt = ".ttf"
	book.FontMIME = "font/ttf"
	book.FontFormat = "truetype"
	data, err := Write(book)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	if !bytes.Equal(files["OEBPS/cover.png"], buf.Bytes()) {
		t.Fatal("cover bytes")
	}
	if !bytes.Equal(files["OEBPS/fonts/embedded.ttf"], []byte("fake-font-bytes")) {
		t.Fatal("font bytes")
	}
	css := string(files["OEBPS/style.css"])
	if !strings.Contains(css, `@font-face`) || !strings.Contains(css, `fonts/embedded.ttf`) || !strings.Contains(css, `font-family:"EmbeddedFont",serif`) {
		t.Fatal(css)
	}
	if regexp.MustCompile(`font-size:\s*\d+px`).MatchString(css) || strings.Contains(css, "!important") {
		t.Fatalf("embedded font must not lock size:\n%s", css)
	}
	if !strings.Contains(css, "line-height:1.8") || !strings.Contains(css, "margin:5%") {
		t.Fatalf("missing moon defaults:\n%s", css)
	}
	opf := string(files["OEBPS/content.opf"])
	if !strings.Contains(opf, `media-type="font/ttf"`) || !strings.Contains(opf, `image/png`) {
		t.Fatal(opf)
	}
	if !strings.Contains(string(files["OEBPS/cover.xhtml"]), `src="cover.png"`) {
		t.Fatal("cover page should use the image")
	}
}

func TestDefaultStylesheetLeavesReaderInControl(t *testing.T) {
	data, err := Write(sampleBook())
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	css := string(files["OEBPS/style.css"])
	assertReaderControlledCSS(t, css)
	var joined string
	for name, body := range files {
		if strings.HasSuffix(name, ".xhtml") {
			joined += string(body)
		}
	}
	if !strings.Contains(joined, "<h1>第一卷 上游</h1>") || !strings.Contains(joined, "<h1>第一章 纸灯</h1>") || !strings.Contains(joined, "<h1>后记</h1>") {
		t.Fatalf("heading levels:\n%s", joined)
	}
	if strings.Contains(joined, "<h2>") {
		t.Fatal("moon chapters use h1 only")
	}
}

func TestExplicitStyleOverrides(t *testing.T) {
	book := sampleBook()
	book.Style = Style{FontSizePt: 18, LineHeight: 2, MarginPercent: 8, ParagraphEm: 0.8, IndentEm: 3}
	data, err := Write(book)
	if err != nil {
		t.Fatal(err)
	}
	css := string(unzip(t, data)["OEBPS/style.css"])
	for _, needle := range []string{"font-size:1.5em", "line-height:2", "margin:8%", "text-indent:3em", "margin:0.8em 0"} {
		if !strings.Contains(css, needle) {
			t.Fatalf("missing override %s\n%s", needle, css)
		}
	}
	if strings.Contains(css, "@page") || strings.Contains(css, "margin:8pt") {
		t.Fatalf("override still uses points:\n%s", css)
	}
	if regexp.MustCompile(`font-size:\s*\d+px`).MatchString(css) || strings.Contains(css, "!important") {
		t.Fatal(css)
	}
}

func assertReaderControlledCSS(t *testing.T, css string) {
	t.Helper()
	for _, needle := range []string{
		"body{margin:5%;line-height:1.8;text-align:justify;}",
		"h1{font-size:1.5em;text-align:center;margin:2em 0 1.5em;page-break-after:avoid;}",
		"p{text-indent:2em;margin:0.45em 0;orphans:2;widows:2;}",
	} {
		if !strings.Contains(css, needle) {
			t.Fatalf("missing %s\n%s", needle, css)
		}
	}
	if regexp.MustCompile(`font-size:\s*\d+px`).MatchString(css) || strings.Contains(css, "!important") || strings.Contains(css, "font-family") {
		t.Fatalf("default stylesheet locks the reader:\n%s", css)
	}
	if regexp.MustCompile(`body\{[^}]*font-size`).MatchString(css) || regexp.MustCompile(`(^|})p\{[^}]*font-size`).MatchString(css) {
		t.Fatalf("body or p font-size is locked:\n%s", css)
	}
}

func TestEpubCheck(t *testing.T) {
	jar := findEpubcheck()
	if jar == "" {
		t.Skip("epubcheck jar not found")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found")
	}
	data, err := Write(sampleBook())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "book.epub")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("java", "-jar", jar, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("epubcheck: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("0 errors")) && !bytes.Contains(out, []byte("No errors")) {
		t.Logf("epubcheck output:\n%s", out)
	}
}

func unzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}

func wellFormed(b []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func findEpubcheck() string {
	if v := os.Getenv("EPUBCHECK_JAR"); v != "" {
		if st, err := os.Stat(v); err == nil && !st.IsDir() {
			return v
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		root := filepath.Join(dir, ".cache", "epubcheck")
		matches, _ := filepath.Glob(filepath.Join(root, "epubcheck.jar"))
		nested, _ := filepath.Glob(filepath.Join(root, "*", "epubcheck.jar"))
		matches = append(matches, nested...)
		if len(matches) > 0 {
			return matches[0]
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
