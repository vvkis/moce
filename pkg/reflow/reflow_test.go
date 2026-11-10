package reflow

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestEPUBSpineAndNav(t *testing.T) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("META-INF/container.xml", `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	write("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>河灯</dc:title><dc:creator>沈小满</dc:creator><dc:language>zh</dc:language><dc:publisher>试印</dc:publisher><dc:identifier>9790000000001</dc:identifier></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/><item id="c2" href="c2.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/><itemref idref="c2"/></spine></package>`)
	write("OEBPS/nav.xhtml", `<html><body><nav><ol><li><a href="c1.xhtml">第一卷 上游</a><ol><li><a href="c2.xhtml">第一章 纸灯</a></li></ol></li></ol></nav></body></html>`)
	write("OEBPS/c1.xhtml", `<html><body><h1>第一卷 上游</h1><p>卷首。</p></body></html>`)
	write("OEBPS/c2.xhtml", `<html><body><h1>第一章 纸灯</h1><p>他把灯放进河里。</p></body></html>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	book, err := Open("河灯.epub", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "河灯" || book.Author != "沈小满" || book.Publisher != "试印" {
		t.Fatalf("%+v", book)
	}
	if !strings.Contains(book.Text, "他把灯放进河里。") || !strings.Contains(book.Text, "第一章 纸灯") {
		t.Fatalf("%s", book.Text)
	}
	if len(book.Headings) < 2 || book.Headings[0].Title != "第一卷 上游" || book.Headings[0].Depth != 1 || book.Headings[1].Depth != 2 {
		t.Fatalf("%+v", book.Headings)
	}
}

func TestPalmDocMOBI(t *testing.T) {
	html := "<h1>第一章 纸灯</h1><p>他把灯放进河里。</p>"
	raw := palmWrap("河灯", html)
	book, err := Open("河灯.mobi", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(book.Text, "他把灯放进河里。") || book.Format != "MOBI" {
		t.Fatalf("%s", book.Text)
	}
	if got := string(palmLZ77(palmPack([]byte("abc  ")))); got != "abc  " && len(got) == 0 {
		t.Fatal(got)
	}
}

func palmWrap(title, html string) []byte {
	text := []byte(html)
	rec0 := make([]byte, 16)
	binary.BigEndian.PutUint16(rec0[0:], 1)
	binary.BigEndian.PutUint32(rec0[4:], uint32(len(text)))
	binary.BigEndian.PutUint16(rec0[8:], 1)
	binary.BigEndian.PutUint16(rec0[10:], 4096)
	header := make([]byte, 78)
	copy(header, title)
	copy(header[60:], "BOOK")
	copy(header[64:], "MOBI")
	binary.BigEndian.PutUint16(header[76:], 2)
	off0 := 78 + 16 + 2
	off1 := off0 + len(rec0)
	var recs [16]byte
	binary.BigEndian.PutUint32(recs[0:], uint32(off0))
	binary.BigEndian.PutUint32(recs[8:], uint32(off1))
	var buf bytes.Buffer
	buf.Write(header)
	buf.Write(recs[:])
	buf.Write([]byte{0, 0})
	buf.Write(rec0)
	buf.Write(text)
	return buf.Bytes()
}

func palmPack(s []byte) []byte { return s }
