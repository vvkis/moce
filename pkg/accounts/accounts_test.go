package accounts

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripPlain(t *testing.T) {
	root := t.TempDir()
	Use(filepath.Join(root, "data"), func() string { return "plain" })
	t.Cleanup(func() { Use("", nil) })
	acc := Account{Cookie: "a=b", AccountID: "7", Name: "读者"}
	if err := Put("douban", acc); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Get("douban")
	if err != nil || !ok || got.Cookie != "a=b" || got.Name != "读者" {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	body, err := os.ReadFile(filepath.Join(root, "data", "accounts", "douban.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"cookie": "a=b"`) || strings.Contains(string(body), ".tmp") {
		t.Fatalf("%s", body)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "data", "accounts", "*.tmp"))
	if len(matches) != 0 {
		t.Fatal(matches)
	}
	if err := Delete("douban"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Get("douban"); ok {
		t.Fatal("still present")
	}
}

func TestVerifyBeforeSave(t *testing.T) {
	root := t.TempDir()
	Use(filepath.Join(root, "data"), nil)
	t.Cleanup(func() { Use("", nil); Transport = nil })
	Transport = round(func(r *http.Request) (*http.Response, error) {
		return text(200, "请登录"), nil
	})
	err := Commit(context.Background(), "douban", "dbcl2=1")
	if err == nil {
		t.Fatal("saved without verify")
	}
	if _, ok, _ := Get("douban"); ok {
		t.Fatal("file written")
	}
	Transport = round(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "dbcl2=1" {
			t.Fatal("cookie missing")
		}
		return text(200, "我的豆瓣"), nil
	})
	if err := Commit(context.Background(), "douban", "dbcl2=1"); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := Get("douban")
	if !ok || got.Cookie != "dbcl2=1" {
		t.Fatalf("%+v", got)
	}
}

func TestExpiryAndAccountChange(t *testing.T) {
	root := t.TempDir()
	Use(filepath.Join(root, "data"), nil)
	t.Cleanup(func() { Use("", nil); Transport = nil })
	if err := Put("weread", Account{Cookie: "wr_vid=1; wr_skey=old", Token: "old", AccountID: "1", Name: "甲"}); err != nil {
		t.Fatal(err)
	}
	Transport = round(func(r *http.Request) (*http.Response, error) {
		return text(401, "no"), nil
	})
	err := Commit(context.Background(), "weread", "wr_vid=1; wr_skey=old")
	if err == nil || err.Error() != "登录已失效，请重新登录" {
		t.Fatal(err)
	}
	got, _, _ := Get("weread")
	if got.Token != "old" {
		t.Fatal("expired login overwrote the file")
	}
	Transport = round(func(r *http.Request) (*http.Response, error) {
		_ = Put("weread", Account{Cookie: "wr_vid=9; wr_skey=other", Token: "other", AccountID: "9", Name: "乙"})
		return text(200, "ok"), nil
	})
	err = Commit(context.Background(), "weread", "wr_vid=1; wr_skey=old")
	if err == nil || err.Error() != "账号已变化，请重新登录" {
		t.Fatal(err)
	}
	got, _, _ = Get("weread")
	if got.AccountID != "9" {
		t.Fatal("concurrent login was overwritten")
	}
}

func TestSwitchAccountReplacesFile(t *testing.T) {
	root := t.TempDir()
	Use(filepath.Join(root, "data"), nil)
	t.Cleanup(func() { Use("", nil); Transport = nil })
	if err := Put("weread", Account{Cookie: "wr_vid=1; wr_skey=old", Token: "old", AccountID: "1", Name: "甲"}); err != nil {
		t.Fatal(err)
	}
	Transport = round(func(r *http.Request) (*http.Response, error) {
		return text(200, "ok"), nil
	})
	if err := Commit(context.Background(), "weread", "wr_vid=2; wr_skey=new"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Get("weread")
	if got.AccountID != "2" || got.Token != "new" {
		t.Fatalf("%+v", got)
	}
}

func TestParseCookie(t *testing.T) {
	got := ParseCookie("Cookie: b=2; a=1; bad; c=")
	if got != "a=1; b=2" {
		t.Fatal(got)
	}
}

type round func(*http.Request) (*http.Response, error)

func (f round) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func text(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: ioNop(body), Header: http.Header{}}
}

type nop struct{ *strings.Reader }

func (nop) Close() error { return nil }

func ioNop(s string) nop { return nop{strings.NewReader(s)} }
