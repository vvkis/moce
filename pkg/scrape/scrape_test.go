package scrape

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"moce/pkg/appdir"
)

func TestFromBooks(t *testing.T) {
	rows := FromBooks("qimao", []Book{{Title: " 书 ", Authors: "乙", Category: "玄幻", Cover: "u"}})
	if rows[0].Title != "书" || rows[0].Author != "乙" || rows[0].Tags[0] != "玄幻" || rows[0].Source != "qimao" {
		t.Fatalf("%+v", rows[0])
	}
	rows = FromBooks("weread", []Book{{
		Title: "书", Authors: "乙", Series: "系",
		Meta: BookMeta{Publisher: "社", Published: "2022-01-01 00:00:00", ISBN: "9", Rating: 9.55},
	}})
	if rows[0].Publisher != "社" || rows[0].Year != "2022" || rows[0].ISBN != "9" || rows[0].Rating != "9.6" || rows[0].Series != "系" {
		t.Fatalf("%+v", rows[0])
	}
}

func TestDoubanFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "douban_search.html"))
	if err != nil {
		t.Fatal(err)
	}
	results := ParseDoubanSearch(string(body), "河灯")
	if len(results) != 4 {
		t.Fatalf("len %d", len(results))
	}
	if results[0].Title != "河灯" || results[0].Similarity != 1 {
		t.Fatalf("first %+v", results[0])
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Similarity < results[i].Similarity {
			t.Fatalf("order %v", results)
		}
	}
	byID := map[string]Row{}
	for _, row := range results {
		byID[row.DoubanID] = row
	}
	exact := byID["9100001"]
	if exact.URL != "https://book.douban.com/subject/9100001/" || exact.Rating != "8.6" ||
		exact.CoverURL != "https://img.example/hedeng.jpg" ||
		exact.CoverHeaders["Referer"] != "https://book.douban.com/" ||
		exact.Intro != "镇上的人把愿望写在纸灯上，放进河里。" ||
		exact.Source != "douban" || exact.Author != "沈小满" || exact.Publisher != "试印社" || exact.Year != "2024" {
		t.Fatalf("exact %+v", exact)
	}
	direct := byID["9100002"]
	if direct.Title != "河灯记" || direct.Author != "林纸" || direct.Publisher != "纸灯社" || direct.Year != "2020" || direct.Rating != "8.1" {
		t.Fatalf("direct %+v", direct)
	}
	if byID["9100003"].Title != "河灯呀" {
		t.Fatalf("encoded %+v", byID["9100003"])
	}
	if byID["9100004"].Title != "河灯传" || byID["9100004"].Author != "李四" {
		t.Fatalf("sid %+v", byID["9100004"])
	}
	if _, ok := byID["1000001"]; ok {
		t.Fatal("low similarity kept")
	}
}

func TestDoubanEntitiesAndLimit(t *testing.T) {
	html := `<div class="result"><div class="content"><h3>
<a href="https://book.douban.com/subject/1234567/">河灯&amp;夜</a>
</h3><span class="subject-cast">沈&amp;纸 / 试印社 / 2024</span>
<img src="https://img.example/cover.jpg?a=1&amp;b=2">
<p>纸灯&amp;河水</p></div></div>`
	rows := ParseDoubanSearch(html, "河灯&夜")
	if len(rows) != 1 || rows[0].Title != "河灯&夜" || rows[0].Author != "沈&纸" ||
		rows[0].CoverURL != "https://img.example/cover.jpg?a=1&b=2" || rows[0].Intro != "纸灯&河水" {
		t.Fatalf("%+v", rows)
	}
	var blocks []string
	for i := 1; i <= 12; i++ {
		blocks = append(blocks, `<div class="result"><div class="content"><h3><a href="https://book.douban.com/subject/`+
			strconv.Itoa(3000+i)+`/" class="nbg">河灯</a></h3><span class="subject-cast">沈小满 / 试印社 / 2024</span></div></div>`)
	}
	rows = ParseDoubanSearch(strings.Join(blocks, "\n"), "河灯")
	if len(rows) != 10 || rows[0].Similarity != 1 || rows[0].Source != "douban" {
		t.Fatalf("%d %+v", len(rows), rows[0])
	}
}

func TestDoubanSearchRequest(t *testing.T) {
	if _, err := Search(context.Background(), "   ", "douban"); err == nil || err.Error() != "搜索关键词为空" {
		t.Fatalf("%v", err)
	}
	var got string
	Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		got = r.URL.String()
		return textResponse(418, "teapot"), nil
	})
	t.Cleanup(func() { Transport = nil })
	_, err := Search(context.Background(), "河灯", "douban")
	if err == nil || err.Error() != "网络请求失败 (418)" {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(got, "cat=1001&q=") || !strings.Contains(got, "q=%E6%B2%B3%E7%81%AF") {
		t.Fatalf("url %s", got)
	}
	Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return textResponse(200, ""), nil
	})
	if _, err := Search(context.Background(), "河灯", "douban"); err == nil || err.Error() != "响应为空" {
		t.Fatalf("%v", err)
	}
}

func TestWereadNeedsLogin(t *testing.T) {
	called := false
	Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		return textResponse(200, "{}"), nil
	})
	t.Cleanup(func() { Transport = nil })
	_, err := Search(context.Background(), "河灯", "weread")
	if err == nil || err.Error() != "需要登录" || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestQimaoSignAndFanqieCover(t *testing.T) {
	sum := md5.Sum([]byte("a=1b=2" + qimaoSignKey))
	if qimaoSign(map[string]string{"b": "2", "a": "1"}) != hex.EncodeToString(sum[:]) {
		t.Fatal(qimaoSign(map[string]string{"a": "1", "b": "2"}))
	}
	if !strings.Contains(fanqieCover("novel-pic/abc.jpg"), "https://p6-novel.byteimg.com/novel-pic/abc.jpg") {
		t.Fatal(fanqieCover("novel-pic/abc.jpg"))
	}
	if jjwxcCover("https://img.example/a.jpg&ver=1&x=2") != "https://img.example/a.jpg&x=2" {
		t.Fatal(jjwxcCover("https://img.example/a.jpg&ver=1&x=2"))
	}
}

func TestSweepDropsUnkeptCovers(t *testing.T) {
	root := t.TempDir()
	layout := appdir.LayoutFrom(filepath.Join(root, "墨册.exe"))
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	Use(layout)
	t.Cleanup(func() { Use(appdir.Layout{}) })
	kept := filepath.Join(layout.Covers, "kept.jpg")
	other := filepath.Join(layout.Covers, "preview.jpg")
	if err := os.WriteFile(kept, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := KeepCover(kept); err != nil {
		t.Fatal(err)
	}
	Sweep()
	if _, err := os.Stat(kept); err != nil {
		t.Fatal("chosen cover removed", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatal("preview cover still there")
	}
	if _, err := os.Stat(filepath.Join(layout.Covers, "kept.json")); err != nil {
		t.Fatal(err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func textResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       ioNop(body),
		Header:     http.Header{},
	}
}

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

func ioNop(s string) nopCloser { return nopCloser{strings.NewReader(s)} }
