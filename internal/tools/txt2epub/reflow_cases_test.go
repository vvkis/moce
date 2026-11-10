package txt2epub

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const hedeng = `第五卷 上游夜航
第50章 甲
　　这一章正文足够长，不该被卷末规则收走。
　　第二行也是正文。
8.11第3章 灯还在吗？
　　日期前缀不该留在目录里。
　　另一行。
第1章 纸灯入水！
　　他把灯放进河里。
　　第二行还是正文。
第1章 又是开头
　　重复的章号各自成章。
　　不要并掉。
第六卷：下游未至，敬请期待……
第六卷 下游芦苇
第一章 重启
　　新的一卷从第一章开始，正文足够长。
　　继续写。
第七卷 回水湾
第53章 灯芯还在
　　正常正文一行。
　　正常正文二行。
第七卷：回水湾
纸灯顺水而下，字迹还在（完）
第八卷 旧码头
第1章 开始
　　正文。
　　还有。
第八卷：旧码头
番外
第一章 早安
　　番外正文。
　　第二段。
`

func TestHedengTextAndReflow(t *testing.T) {
	tool := New()
	check := func(name string, data []byte) {
		t.Helper()
		out, err := tool.Call("convert", mustJSON(fileIn{
			Name: name, DataBase64: base64.StdEncoding.EncodeToString(data),
			Title: "河灯", TitleEdited: true, ReturnBytes: true, Options: fullOptions(),
		}))
		if err != nil {
			t.Fatal(name, err)
		}
		raw, _ := base64.StdEncoding.DecodeString(out.(ConvertResult).DataBase64)
		files := unzip(t, raw)
		nav := string(files["OEBPS/nav.xhtml"])
		ncx := string(files["OEBPS/toc.ncx"])
		css := string(files["OEBPS/style.css"])
		for _, needle := range []string{"第3章 灯还在吗？", "第1章 纸灯入水！", "第1章 又是开头", "第六卷 下游芦苇", "第七卷 回水湾", "第53章 灯芯还在", "第八卷 旧码头", "番外", "第一章 早安"} {
			if !strings.Contains(nav, needle) || !strings.Contains(ncx, needle) {
				t.Fatalf("%s missing %s\nnav:\n%s", name, needle, nav)
			}
		}
		if strings.Contains(nav, "敬请期待") || strings.Contains(nav, "第七卷：") || strings.Contains(nav, "第八卷：") || strings.Contains(nav, "8.11") {
			t.Fatalf("%s bad toc\n%s", name, nav)
		}
		if strings.Index(nav, "番外") > strings.Index(nav, "第一章 早安") {
			t.Fatalf("%s fanwai not parent\n%s", name, nav)
		}
		for _, needle := range []string{
			"body{margin:5%;line-height:1.8;text-align:justify;}",
			"h1{font-size:1.5em;text-align:center;margin:2em 0 1.5em;page-break-after:avoid;}",
			"p{text-indent:2em;margin:0.45em 0;orphans:2;widows:2;}",
		} {
			if !strings.Contains(css, needle) {
				t.Fatalf("%s css %s\n%s", name, needle, css)
			}
		}
		if pxFont.MatchString(css) {
			t.Fatal(css)
		}
		if dir := os.Getenv("MOCE_VERIFY_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name+".epub"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	check("河灯.txt", []byte(hedeng))
	check("河灯.epub", hedengEPUB())
	check("河灯.mobi", hedengMOBI())
	check("河灯.azw3", hedengMOBI())
}

var pxFont = regexp.MustCompile(`font-size:\s*\d+px`)

func hedengEPUB() []byte {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	put := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	put("mimetype", "application/epub+zip")
	put("META-INF/container.xml", `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	put("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>河灯</dc:title><dc:creator>沈小满</dc:creator><dc:language>zh</dc:language></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`)
	put("OEBPS/c1.xhtml", "<html><body><p>"+strings.ReplaceAll(hedeng, "\n", "</p><p>")+"</p></body></html>")
	_ = zw.Close()
	return buf.Bytes()
}

func hedengMOBI() []byte {
	text := []byte("<p>" + strings.ReplaceAll(hedeng, "\n", "</p><p>") + "</p>")
	rec0 := make([]byte, 16)
	binary.BigEndian.PutUint16(rec0[0:], 1)
	binary.BigEndian.PutUint32(rec0[4:], uint32(len(text)))
	binary.BigEndian.PutUint16(rec0[8:], 1)
	binary.BigEndian.PutUint16(rec0[10:], 4096)
	header := make([]byte, 78)
	copy(header, "河灯")
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
