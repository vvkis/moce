package txt2epub

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func fullOptions() Options {
	return Options{
		MergeBrokenLines: true,
		DropEmptyLines:   true,
		LineHeight:       1.8,
		MarginPercent:    5,
		ParagraphEm:      0.45,
		IndentEm:         2,
		FallbackRunes:    8000,
		RemoveDupCover:   true,
	}
}

func TestProbeAndConvertGBK(t *testing.T) {
	text := "《河灯》\n作者：沈小满\n\n第一章 纸灯\n　　他把灯放进河里。\n灯走远了。\n\n第二章 码头\n　　码头上没有人。\n"
	enc := simplifiedchinese.GBK.NewEncoder()
	raw, _, err := transform.Bytes(enc, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	tool := New()
	probed, err := tool.Call("probe", mustJSON(fileIn{
		Name:       "随便.txt",
		DataBase64: base64.StdEncoding.EncodeToString(raw),
		Options:    fullOptions(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	preview := probed.(ProbeResult)
	if preview.Encoding != "gb18030" || preview.Title != "河灯" || preview.Author != "沈小满" {
		t.Fatalf("%+v", preview)
	}
	if preview.Fallback || preview.ChapterCount != 2 {
		t.Fatalf("chapters %+v", preview)
	}
	if preview.Entries[0].Title != "第一章 纸灯" {
		t.Fatalf("leading meta was not stripped: %#v", preview.Entries)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "河灯.txt")
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	converted, err := tool.Call("convert", mustJSON(fileIn{
		Name:     "河灯.txt",
		Path:     src,
		Title:    "河灯",
		Author:   "沈小满",
		Encoding: "gbk",
		Options:  fullOptions(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	result := converted.(ConvertResult)
	if result.Path == "" || result.ChapterCount != 2 {
		t.Fatalf("%+v", result)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, body)
	var joined strings.Builder
	for name, b := range files {
		if strings.HasSuffix(name, ".xhtml") {
			joined.Write(b)
		}
	}
	if !strings.Contains(joined.String(), "<p>他把灯放进河里。</p>") || !strings.Contains(joined.String(), "<p>灯走远了。</p>") {
		t.Fatalf("paragraphs missing:\n%s", joined.String())
	}
	if strings.Contains(joined.String(), "作者：沈小满") {
		t.Fatal("duplicate author line kept in the body")
	}
	if _, ok := files["OEBPS/content.opf"]; !ok {
		t.Fatal("missing opf")
	}
	if !strings.Contains(string(files["OEBPS/content.opf"]), "<dc:language>zh</dc:language>") {
		t.Fatal("language")
	}
}

func TestKeepLeadingWhenRequested(t *testing.T) {
	text := "《河灯》\n作者：沈小满\n\n第一章 纸灯\n　　正文。\n"
	opt := fullOptions()
	opt.RemoveDupCover = false
	tool := New()
	out, err := tool.Call("probe", mustJSON(fileIn{
		Name:       "河灯.txt",
		DataBase64: base64.StdEncoding.EncodeToString([]byte(text)),
		Encoding:   "utf-8",
		Options:    opt,
	}))
	if err != nil {
		t.Fatal(err)
	}
	preview := out.(ProbeResult)
	if preview.Entries[0].Title != "前言" {
		t.Fatalf("%#v", preview.Entries)
	}
}

func TestMetadataInOPF(t *testing.T) {
	text := "第一章 纸灯\n　　正文。\n"
	tool := New()
	out, err := tool.Call("convert", mustJSON(fileIn{
		Name:         "河灯.txt",
		DataBase64:   base64.StdEncoding.EncodeToString([]byte(text)),
		Title:        "河灯",
		Author:       "沈小满 & 林纸",
		AuthorEdited: true,
		Series:       "河岸",
		SeriesIndex:  "2.5",
		Tags:         "玄幻、完结，热血",
		Publisher:    "试印社",
		Language:     "中文",
		Published:    "2026年11月8日",
		Identifier:   "ISBN 979-0-0000-0000-1",
		Rating:       5,
		ReturnBytes:  true,
		Options:      fullOptions(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(out.(ConvertResult).DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, raw)
	opf := string(files["OEBPS/content.opf"])
	for _, needle := range []string{
		"<dc:language>zh</dc:language>",
		"<dc:creator>沈小满</dc:creator>",
		"<dc:creator>林纸</dc:creator>",
		"<dc:publisher>试印社</dc:publisher>",
		"<dc:subject>玄幻</dc:subject>",
		"<dc:subject>完结</dc:subject>",
		"<dc:subject>热血</dc:subject>",
		"<dc:date>2026-11-08</dc:date>",
		"<meta property=\"dcterms:issued\">2026-11-08</meta>",
		"urn:isbn:9790000000001",
		"<meta name=\"calibre:series\" content=\"河岸\"/>",
		"<meta name=\"calibre:series_index\" content=\"2.5\"/>",
		"<meta name=\"calibre:rating\" content=\"10\"/>",
		"<meta property=\"belongs-to-collection\" id=\"series\">河岸</meta>",
	} {
		if !strings.Contains(opf, needle) {
			t.Fatalf("missing %s\n%s", needle, opf)
		}
	}
}

func TestClearedAuthorStaysEmpty(t *testing.T) {
	text := "作者：沈小满\n\n第一章 甲\n　　正文。\n"
	tool := New()
	out, err := tool.Call("convert", mustJSON(fileIn{
		Name:         "甲.txt",
		DataBase64:   base64.StdEncoding.EncodeToString([]byte(text)),
		Title:        "甲",
		Author:       "",
		AuthorEdited: true,
		ReturnBytes:  true,
		Options:      fullOptions(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(out.(ConvertResult).DataBase64)
	opf := string(unzip(t, raw)["OEBPS/content.opf"])
	if !strings.Contains(opf, "<dc:creator>佚名</dc:creator>") {
		t.Fatal(opf)
	}
	if strings.Contains(opf, "沈小满") {
		t.Fatal("cleared author was parsed again")
	}
}

func TestNestedTOCAndDefaultCSS(t *testing.T) {
	text := "第五卷 风起\n第一章 甲\n　　甲的正文。\n第五卷 完\n第六卷 云\n第二章 乙\n　　乙的正文。\n终章\n　　到这里。\n"
	out, err := New().Call("convert", mustJSON(fileIn{
		Name:         "书.txt",
		DataBase64:   base64.StdEncoding.EncodeToString([]byte(text)),
		Title:        "书",
		Author:       "甲",
		AuthorEdited: true,
		ReturnBytes:  true,
		Options:      fullOptions(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(out.(ConvertResult).DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, raw)
	nav := string(files["OEBPS/nav.xhtml"])
	ncx := string(files["OEBPS/toc.ncx"])
	css := string(files["OEBPS/style.css"])
	if strings.Contains(nav, "第五卷 完") || strings.Contains(ncx, "第五卷 完") {
		t.Fatalf("closer in toc\n%s\n%s", nav, ncx)
	}
	if strings.Index(nav, "第五卷 风起") < 0 || strings.Index(nav, "第五卷 风起") > strings.Index(nav, "第一章 甲") {
		t.Fatal(nav)
	}
	if strings.Count(nav, "<ol>") < 2 {
		t.Fatalf("nav not nested:\n%s", nav)
	}
	if strings.Index(ncx, "第五卷 风起") < 0 || strings.Index(ncx, "第五卷 风起") > strings.Index(ncx, "第一章 甲") {
		t.Fatal(ncx)
	}
	if strings.Count(ncx, "<navPoint") < 4 {
		t.Fatalf("ncx:\n%s", ncx)
	}
	var body strings.Builder
	for name, b := range files {
		if strings.HasSuffix(name, ".xhtml") {
			body.Write(b)
		}
	}
	if !strings.Contains(body.String(), "第五卷 完") {
		t.Fatal("closer missing from chapter body")
	}
	if regexp.MustCompile(`font-size:\s*\d+px`).MatchString(css) || strings.Contains(css, "!important") || strings.Contains(css, "font-family") {
		t.Fatalf("default css locks font size:\n%s", css)
	}
	for _, needle := range []string{
		"body{margin:5%;line-height:1.8;text-align:justify;}",
		"h1{font-size:1.5em;text-align:center;margin:2em 0 1.5em;page-break-after:avoid;}",
		"p{text-indent:2em;margin:0.45em 0;orphans:2;widows:2;}",
	} {
		if !strings.Contains(css, needle) {
			t.Fatalf("missing %s\n%s", needle, css)
		}
	}
}

func TestFilenameAuthor(t *testing.T) {
	if got := guessTitle("《大河》作者：林深.txt", ""); got != "大河" {
		t.Fatal(got)
	}
	if got := guessAuthor("《大河》作者：林深.txt", ""); got != "林深" {
		t.Fatal(got)
	}
	if got := guessTitle("笔记.txt", "书名：河灯\n你好\n"); got != "河灯" {
		t.Fatal(got)
	}
	if got := sanitizeFilename(`a/b:c*?.txt`); strings.ContainsAny(got, `/\:*?`) {
		t.Fatal(got)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
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
