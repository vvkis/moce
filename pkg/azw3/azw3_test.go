package azw3

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"moce/pkg/epub"
	"moce/pkg/reflow"
)

func sampleBook(t *testing.T) epub.Book {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{0x9D, 0x2C, 0x2C, 0xFF})
	var coverBuf bytes.Buffer
	if err := png.Encode(&coverBuf, img); err != nil {
		t.Fatal(err)
	}
	return epub.Book{
		Title:       `河灯 & 夜`,
		Author:      `沈小满 & 林纸`,
		Language:    "zh",
		Publisher:   "试印社",
		Description: "一条河与一盏灯。",
		Subjects:    []string{"玄幻", "完结"},
		Identifiers: []string{"urn:isbn:9790000000001"},
		Published:   "2026-11-08",
		UUID:        "12345678-1234-4234-8234-123456789abc",
		Modified:    time.Date(2026, 11, 8, 1, 2, 3, 0, time.UTC),
		Style:       epub.MoonDefaultStyle(),
		Cover:       coverBuf.Bytes(),
		CoverName:   "cover.png",
		CoverMIME:   "image/png",
		Chapters: []epub.Chapter{
			{
				Title: "第一卷 上游",
				Children: []epub.Chapter{
					{Title: "第一章 纸灯", Paragraphs: []string{"他把灯放进河里。", `A & B <C> "D"`, strings.Repeat("长段落测试文字。", 300)}},
					{Title: "第二章 码头", Paragraphs: []string{"码头上空无一人。"}},
				},
			},
			{Title: "后记", Paragraphs: []string{"写完了。"}},
		},
	}
}

func TestPalmDOCRoundTrip(t *testing.T) {
	samples := [][]byte{
		[]byte("Hello world!   Hello world!  "),
		[]byte("第一章 纸灯\n他把灯放进河里。他把灯放进河里。他把灯放进河里。"),
		bytes.Repeat([]byte("中文重复段落测试，包含 ASCII 12345 和空格 word word。\n"), 50),
	}
	for _, src := range samples {
		comp := compressPalmDOC(src)
		got := decompressPalmDOC(comp)
		if !bytes.Equal(got, src) {
			t.Fatalf("roundtrip mismatch: got len %d want %d", len(got), len(src))
		}
	}
}

func decompressPalmDOC(in []byte) []byte {
	var out []byte
	for i := 0; i < len(in); {
		c := in[i]
		i++
		switch {
		case c == 0 || (c >= 0x09 && c <= 0x7F):
			out = append(out, c)
		case c >= 0x01 && c <= 0x08:
			n := int(c)
			out = append(out, in[i:i+n]...)
			i += n
		case c >= 0x80 && c <= 0xBF:
			c2 := in[i]
			i++
			comb := int(c)<<8 | int(c2)
			dist := (comb >> 3) & 0x7FF
			ln := (comb & 7) + 3
			for k := 0; k < ln; k++ {
				out = append(out, out[len(out)-dist])
			}
		default:
			out = append(out, ' ', c^0x80)
		}
	}
	return out
}

func TestAZW3BinaryStructureAndReflowRoundTrip(t *testing.T) {
	book := sampleBook(t)
	book.VolumePageStyle = "divider"
	book.InlineTOC = true
	raw, err := Write(book)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Validate PalmDB header
	if string(raw[60:68]) != "BOOKMOBI" {
		t.Fatalf("bad magic: %q", string(raw[60:68]))
	}
	recs := parsePalmRecords(t, raw)
	rec0 := recs[0]
	if string(rec0[16:20]) != "MOBI" {
		t.Fatal("missing MOBI header")
	}
	if got := binary.BigEndian.Uint32(rec0[20:24]); got != 264 {
		t.Fatalf("headerLen = %d", got)
	}
	if got := binary.BigEndian.Uint32(rec0[36:40]); got != 8 {
		t.Fatalf("fileVersion = %d, want 8 (KF8)", got)
	}
	if got := binary.BigEndian.Uint32(rec0[28:32]); got != 65001 {
		t.Fatalf("encoding = %d", got)
	}

	// Verify FDST, SKEL, CHUNK, NCX, GUIDE, FLIS, FCIS, EOF records exist with proper magic.
	fdstIdx := binary.BigEndian.Uint32(rec0[192:196])
	fcisIdx := binary.BigEndian.Uint32(rec0[200:204])
	flisIdx := binary.BigEndian.Uint32(rec0[208:212])
	ncxIdx := binary.BigEndian.Uint32(rec0[244:248])
	chunkIdx := binary.BigEndian.Uint32(rec0[248:252])
	skelIdx := binary.BigEndian.Uint32(rec0[252:256])
	guideIdx := binary.BigEndian.Uint32(rec0[260:264])

	for name, idx := range map[string]uint32{
		"FDST": fdstIdx, "FCIS": fcisIdx, "FLIS": flisIdx,
		"NCX": ncxIdx, "CHUNK": chunkIdx, "SKEL": skelIdx, "GUIDE": guideIdx,
	} {
		if int(idx) >= len(recs) {
			t.Fatalf("%s index out of range: %d >= %d", name, idx, len(recs))
		}
	}
	if string(recs[fdstIdx][:4]) != "FDST" {
		t.Fatalf("bad FDST magic: %q", recs[fdstIdx][:4])
	}
	if string(recs[flisIdx][:4]) != "FLIS" || string(recs[fcisIdx][:4]) != "FCIS" {
		t.Fatal("bad FLIS/FCIS")
	}
	if string(recs[ncxIdx][:4]) != "INDX" || string(recs[skelIdx][:4]) != "INDX" || string(recs[chunkIdx][:4]) != "INDX" {
		t.Fatal("bad INDX headers")
	}
	if !bytes.Equal(recs[len(recs)-1], []byte{0xE9, 0x8E, '\r', '\n'}) {
		t.Fatal("missing EOF record")
	}

	// Verify Chunk CNCX uses KF8 parent selector "P-//*[@aid=...]".
	chunkCount := int(binary.BigEndian.Uint32(recs[chunkIdx][24:28]))
	chunkCNCX := string(recs[int(chunkIdx)+1+chunkCount])
	if !strings.Contains(chunkCNCX, "P-//*[@aid=") || strings.Contains(chunkCNCX, "0-//*[@aid=") {
		t.Fatalf("invalid chunk selector in CNCX: %q", chunkCNCX[:min(len(chunkCNCX), 80)])
	}
	if got := binary.BigEndian.Uint32(rec0[240:244]); got != 1 {
		t.Fatalf("extra_data_flags = %d, want 1", got)
	}
	// Verify cover resource was normalized to JPEG for Kindle native thumbnailing.
	firstRes := int(binary.BigEndian.Uint32(rec0[108:112]))
	if len(recs[firstRes]) < 3 || recs[firstRes][0] != 0xFF || recs[firstRes][1] != 0xD8 {
		t.Fatalf("expected JPEG cover resource, got %x", recs[firstRes][:8])
	}

	// Verify NCX CNCX contains Chinese titles.
	ncxCount := int(binary.BigEndian.Uint32(recs[ncxIdx][24:28]))
	cncxCount := int(binary.BigEndian.Uint32(recs[ncxIdx][52:56]))
	if cncxCount < 1 {
		t.Fatal("expected CNCX record for NCX")
	}
	cncxRaw := string(recs[int(ncxIdx)+1+ncxCount])
	for _, want := range []string{"封面", "第一卷 上游", "第一章 纸灯", "第二章 码头", "后记"} {
		if !strings.Contains(cncxRaw, want) {
			t.Fatalf("NCX CNCX missing %q", want)
		}
	}
	// BFS order: top-level items ("封面", "第一卷 上游", "后记") appear before depth-1 children ("第一章 纸灯").
	if strings.Index(cncxRaw, "后记") > strings.Index(cncxRaw, "第一章 纸灯") {
		t.Fatalf("NCX should be breadth-first (all depth-0 before depth-1), got CNCX order: %q", cncxRaw)
	}

	// 2. Round-trip through moce's own reflow.Open("河灯.azw3", raw)
	reopened, err := reflow.Open("河灯.azw3", raw)
	if err != nil {
		t.Fatalf("reflow.Open failed: %v", err)
	}
	if reopened.Title != "河灯 & 夜" {
		t.Fatalf("title = %q", reopened.Title)
	}
	if reopened.Publisher != "试印社" || reopened.Identifier != "9790000000001" {
		t.Fatalf("meta mismatch: %+v", reopened)
	}
	if len(reopened.Cover) == 0 {
		t.Fatal("cover image was not extracted back from AZW3")
	}
	if !strings.Contains(reopened.Text, "他把灯放进河里。") || !strings.Contains(reopened.Text, "写完了。") {
		t.Fatalf("body text missing:\n%s", reopened.Text)
	}
	if strings.Contains(reopened.Text, "text-indent:") {
		t.Fatalf("CSS flow leaked into reopened text:\n%s", reopened.Text)
	}
}

func TestAZW3SkipVolumeAndEmbeddedFont(t *testing.T) {
	book := sampleBook(t)
	book.VolumePageStyle = "skip"
	book.HideCoverInTOC = true
	book.Font = []byte("fake-ttf-font-data")
	raw, err := Write(book)
	if err != nil {
		t.Fatal(err)
	}
	recs := parsePalmRecords(t, raw)
	rec0 := recs[0]
	exthFlags := binary.BigEndian.Uint32(rec0[128:132])
	if exthFlags&0x1000 == 0 {
		t.Fatalf("expected embedded font bit in exth_flags: 0x%x", exthFlags)
	}
	firstRes := int(binary.BigEndian.Uint32(rec0[108:112]))
	// Resource 0 is PNG cover, resource 1 is FONT record
	if string(recs[firstRes+1][:4]) != "FONT" {
		t.Fatalf("expected FONT record at %d, got %q", firstRes+1, recs[firstRes+1][:4])
	}
}

func parsePalmRecords(t *testing.T, data []byte) [][]byte {
	t.Helper()
	n := int(binary.BigEndian.Uint16(data[76:78]))
	offs := make([]int, n)
	for i := 0; i < n; i++ {
		offs[i] = int(binary.BigEndian.Uint32(data[78+i*8:]))
	}
	recs := make([][]byte, n)
	for i := 0; i < n; i++ {
		end := len(data)
		if i+1 < n {
			end = offs[i+1]
		}
		recs[i] = data[offs[i]:end]
	}
	return recs
}
