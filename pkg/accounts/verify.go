// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package accounts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	hintCookie = "按键盘 F12，点「网络」，刷新网页，复制请求标头里的整段 Cookie"
	hintToken  = "按 F12，点「应用」里的本地存储，复制 ls_qimao_global_userinfo_data 中的 token"
	msgExpired = "登录已失效，请重新登录"
	msgChanged = "账号已变化，请重新登录"
)

// Transport is replaced by tests.
var Transport http.RoundTripper

// Source is one row in the account list.
type Source struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Logged bool   `json:"logged"`
	Method string `json:"method"`
	Hint   string `json:"hint"`
	QR     bool   `json:"qr"`
}

// Sources is the account list in 刮搜 order.
func Sources() []Source {
	rows := []struct {
		id, name, method, hint string
		qr                     bool
	}{
		{"douban", "豆瓣", "cookie", hintCookie, false},
		{"weread", "微信读书", "qr", "", true},
		{"qimao", "七猫小说", "token", hintToken, false},
		{"jjwxc", "晋江文学城", "cookie", hintCookie, false},
		{"qidian", "起点中文网", "cookie", hintCookie, true},
		{"fanqie", "番茄小说", "cookie", hintCookie, true},
		{"changpei", "长佩文学", "cookie", hintCookie, true},
		{"xiaoxiang", "潇湘书院", "cookie", hintCookie, true},
	}
	out := make([]Source, 0, len(rows))
	for _, row := range rows {
		acc, ok, _ := Get(row.id)
		status := "未登录"
		logged := false
		if ok && (acc.Cookie != "" || acc.Token != "") {
			logged = true
			name := acc.Name
			if name == "" {
				name = acc.AccountID
			}
			if name == "" {
				status = "已登录"
			} else {
				status = "已登录 " + name
			}
		}
		out = append(out, Source{ID: row.id, Name: row.name, Status: status, Logged: logged, Method: row.method, Hint: row.hint, QR: row.qr})
	}
	return out
}

type checker func(ctx context.Context, secret string) (Account, error)

var checkers = map[string]checker{
	"douban":    checkDouban,
	"qimao":     checkQimao,
	"jjwxc":     checkJJ,
	"qidian":    checkQidian,
	"changpei":  checkChangpei,
	"xiaoxiang": checkXiaoxiang,
	"weread":    checkWeread,
	"fanqie":    checkFanqie,
}

// ParseCookie turns a browser Cookie header into a normalized header.
// Invalid pairs are dropped. An empty result means nothing usable was copied.
func ParseCookie(input string) string {
	text := strings.TrimSpace(input)
	text = cookiePrefix.ReplaceAllString(text, "")
	jar := map[string]string{}
	for _, pair := range strings.Split(text, ";") {
		name, value, _ := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !validCookie(name, value) {
			continue
		}
		jar[name] = value
	}
	if len(jar) == 0 {
		return ""
	}
	keys := make([]string, 0, len(jar))
	for key := range jar {
		keys = append(keys, key)
	}
	sortStrings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + jar[key]
	}
	return strings.Join(parts, "; ")
}

var cookiePrefix = regexp.MustCompile(`(?i)^\s*cookie\s*:`)
var cookieName = regexp.MustCompile(`^[0-9A-Za-z!#$%&'*+\-.^_` + "`" + `|~]+$`)

func validCookie(name, value string) bool {
	return cookieName.MatchString(name) && value != "" && !strings.ContainsAny(value, ";\r\n")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Commit verifies a cookie or token and only then writes the account file.
// If the saved login changes while the check is in flight, nothing is written
// and the error is 「账号已变化，请重新登录」. A different account id replaces
// the previous file. A failed re-check of a saved login is 「登录已失效」.
func Commit(ctx context.Context, id, secret string) error {
	id = cleanID(id)
	if id == "" {
		return fmt.Errorf("未知数据源")
	}
	if checkers[id] != nil && id != "qimao" && id != "weread" {
		secret = ParseCookie(secret)
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("没有读到 Cookie")
	}
	prev, had, _ := Get(id)
	check := checkers[id]
	if check == nil {
		return fmt.Errorf("未知数据源")
	}
	acc, err := check(ctx, secret)
	now, _, _ := Get(id)
	if had && (now.Cookie != prev.Cookie || now.Token != prev.Token || now.AccountID != prev.AccountID) {
		return fmt.Errorf("%s", msgChanged)
	}
	if err != nil {
		if had {
			return fmt.Errorf("%s", msgExpired)
		}
		return err
	}
	if acc.Cookie == "" && acc.Token == "" {
		if strings.Contains(secret, "=") {
			acc.Cookie = secret
		} else {
			acc.Token = secret
		}
	}
	return Put(id, acc)
}

func checkDouban(ctx context.Context, cookie string) (Account, error) {
	body, code, err := get(ctx, "https://www.douban.com/", cookie, nil)
	if err != nil || code != 200 || !strings.Contains(body, "dbcl2") && !strings.Contains(body, "我的豆瓣") {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, Name: "豆瓣"}, nil
}

func checkQimao(ctx context.Context, token string) (Account, error) {
	token = strings.TrimPrefix(strings.TrimPrefix(token, "Bearer "), "bearer ")
	body, code, err := get(ctx, "https://api-ks.wtzw.com/api/pc/v1/user/get-user-info", "", map[string]string{"Authorization": token})
	if err != nil || code != 200 || !strings.Contains(body, "nickname") && !strings.Contains(body, "uid") {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Token: token, Name: "七猫"}, nil
}

func checkJJ(ctx context.Context, cookie string) (Account, error) {
	body, code, err := get(ctx, "https://m.jjwxc.com/my/fav", cookie, nil)
	if err != nil || code != 200 || strings.Contains(body, "login") && !strings.Contains(body, "fav") {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, Name: "晋江"}, nil
}

func checkQidian(ctx context.Context, cookie string) (Account, error) {
	if !strings.Contains(cookie, "_csrfToken=") {
		return Account{}, fmt.Errorf("起点 Cookie 缺少登录凭证")
	}
	body, code, err := get(ctx, "https://m.qidian.com/bookshelf/my/", cookie, nil)
	if err != nil || code != 200 || !strings.Contains(body, "isLogin") {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, AccountID: "qidian", Name: "起点"}, nil
}

func checkChangpei(ctx context.Context, cookie string) (Account, error) {
	body, code, err := get(ctx, "https://www.gongzicp.com/webapi/user/getUserInfo", cookie, map[string]string{"Client": "pc"})
	if err != nil || code != 200 || !strings.Contains(body, "show_uid") {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, Name: "长佩"}, nil
}

func checkXiaoxiang(ctx context.Context, cookie string) (Account, error) {
	body, code, err := get(ctx, "https://www.xxsypro.com/api/user/auth/personal?_csrfToken=moon", cookie, nil)
	if err != nil || code != 200 || !strings.Contains(body, `"isLogin":1`) && !strings.Contains(body, `"isLogin": 1`) {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, Name: "潇湘"}, nil
}

func checkWeread(ctx context.Context, secret string) (Account, error) {
	vid, token := splitWeread(secret)
	if vid == "" || token == "" {
		return Account{}, fmt.Errorf("登录失败")
	}
	body, code, err := get(ctx, "https://i.weread.qq.com/user/profile?vid="+vid, "", map[string]string{"vid": vid, "accessToken": token})
	if err != nil || code != 200 {
		return Account{}, fmt.Errorf("登录失败")
	}
	_ = body
	return Account{Cookie: secret, Token: token, AccountID: vid, Name: vid}, nil
}

func checkFanqie(ctx context.Context, cookie string) (Account, error) {
	if !strings.Contains(cookie, "sessionid") {
		return Account{}, fmt.Errorf("登录失败")
	}
	_, code, err := get(ctx, "https://fanqienovel.com/", cookie, nil)
	if err != nil || code != 200 {
		return Account{}, fmt.Errorf("登录失败")
	}
	return Account{Cookie: cookie, Name: "番茄"}, nil
}

func splitWeread(secret string) (string, string) {
	vid, token := "", ""
	for _, part := range strings.Split(secret, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "wr_vid=") {
			vid = strings.TrimPrefix(part, "wr_vid=")
		}
		if strings.HasPrefix(part, "wr_skey=") {
			token = strings.TrimPrefix(part, "wr_skey=")
		}
	}
	if vid == "" && token == "" && strings.Contains(secret, "|") {
		bits := strings.SplitN(secret, "|", 2)
		return bits[0], bits[1]
	}
	return vid, token
}

func get(ctx context.Context, rawURL, cookie string, headers map[string]string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: Transport}
	res, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return string(body), res.StatusCode, nil
}
