package encoding

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func encode(t *testing.T, name, s string) []byte {
	t.Helper()
	var enc transform.Transformer
	switch name {
	case "gb18030":
		enc = simplifiedchinese.GB18030.NewEncoder()
	case "gbk":
		enc = simplifiedchinese.GBK.NewEncoder()
	case "big5":
		enc = traditionalchinese.Big5.NewEncoder()
	case "utf-16le":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewEncoder()
	case "utf-16be":
		enc = unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewEncoder()
	case "utf-16le-nobom":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	default:
		t.Fatalf("unknown %s", name)
	}
	out, _, err := transform.Bytes(enc, []byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDetectAndDecode(t *testing.T) {
	simp := "这是一段简体中文小说，作者写了国门、书和为了什么而出发。第一章开始，他说了很多话。"
	trad := "這是一段繁體中文小說，作者寫了國門、書和為了什麼而出發。第一章開始，他說了很多話。"
	rare := simp + string(rune(0x20000))

	cases := []struct {
		name string
		raw  []byte
		want string
		text string
	}{
		{name: "utf-8", raw: []byte(simp), want: "utf-8", text: simp},
		{name: "utf-8 bom", raw: append([]byte{0xEF, 0xBB, 0xBF}, []byte(simp)...), want: "utf-8", text: simp},
		{name: "gbk", raw: encode(t, "gbk", simp), want: "gb18030", text: simp},
		{name: "gb18030", raw: encode(t, "gb18030", rare), want: "gb18030", text: rare},
		{name: "big5", raw: encode(t, "big5", trad), want: "big5", text: trad},
		{name: "utf-16le", raw: encode(t, "utf-16le", simp), want: "utf-16le", text: simp},
		{name: "utf-16be", raw: encode(t, "utf-16be", simp), want: "utf-16be", text: simp},
		{name: "utf-16le no bom", raw: encode(t, "utf-16le-nobom", simp), want: "utf-16le", text: simp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Detect(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if res.Name != tc.want {
				t.Fatalf("detect got %s want %s", res.Name, tc.want)
			}
			got, _, err := Decode(tc.raw, "auto")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.text {
				t.Fatalf("decode mismatch\n got %q\nwant %q", got, tc.text)
			}
		})
	}
}

func TestBOMFlags(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte("正文")...)
	res, err := Detect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.BOM || res.Name != "utf-8" {
		t.Fatalf("%+v", res)
	}
	le := encode(t, "utf-16le", "正文")
	res, err = Detect(le)
	if err != nil || !res.BOM || res.Name != "utf-16le" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestManualOverride(t *testing.T) {
	simp := "这是简体中文，用来确认手动指定 GBK 也能正确解码。"
	raw := encode(t, "gbk", simp)
	got, res, err := Decode(raw, "gbk")
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "gb18030" || got != simp {
		t.Fatalf("name %s text %q", res.Name, got)
	}
	if _, _, err := Decode(raw, "utf-8"); err == nil {
		t.Fatal("expected utf-8 override to fail on gbk bytes")
	}
	if _, _, err := Decode(raw, "shift-jis"); err == nil {
		t.Fatal("expected unsupported encoding")
	}
}

func TestNewlinesAndEmpty(t *testing.T) {
	got, _, err := Decode([]byte("甲\r\n乙\r丙"), "utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "甲\n乙\n丙" {
		t.Fatalf("%q", got)
	}
	_, _, err = Decode(nil, "auto")
	if err == nil || !strings.Contains(err.Error(), "空") {
		t.Fatal(err)
	}
}

func TestCanonical(t *testing.T) {
	for _, alias := range []string{"UTF-8", "utf8", "GBK", "gb2312", "cp936", "Big5", "utf-16le"} {
		got, err := Canonical(alias)
		if err != nil || got == "" {
			t.Fatalf("%s -> %s %v", alias, got, err)
		}
	}
}

func TestGB18030NotValidUTF8(t *testing.T) {
	raw := encode(t, "gbk", "这是简体中文小说的一句话。")
	if utf8.Valid(raw) {
		t.Fatal("gbk fixture unexpectedly valid utf-8; detection order would be wrong")
	}
}
