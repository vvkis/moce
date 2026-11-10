// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package scrape

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"moce/pkg/accounts"
)

func searchWeread(ctx context.Context, query string) ([]Row, error) {
	acc, ok, _ := accounts.Get("weread")
	if !ok || acc.AccountID == "" || acc.Token == "" {
		return nil, fmt.Errorf("需要登录")
	}
	before := acc.Token
	q := url.Values{}
	q.Set("keyword", query)
	q.Set("count", "20")
	q.Set("maxIdx", "0")
	q.Set("type", "0")
	q.Set("v", "2")
	body, code, err := do(ctx, "GET", "https://i.weread.qq.com/store/search?"+q.Encode(), map[string]string{
		"vid": acc.AccountID, "accessToken": acc.Token, "User-Agent": "WeRead/10.2.2",
	})
	if later, _, _ := accounts.Get("weread"); later.Token != before {
		return nil, fmt.Errorf("账号已变化，请重新登录")
	}
	if err != nil {
		return nil, err
	}
	if code == 401 || code == 403 {
		return nil, fmt.Errorf("登录已失效，请重新登录")
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Books []map[string]any `json:"books"`
		Data  []map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &wire) != nil {
		return nil, fmt.Errorf("返回非 JSON")
	}
	rows := wire.Books
	if len(rows) == 0 {
		rows = wire.Data
	}
	var books []Book
	for _, entry := range rows {
		info, _ := entry["bookInfo"].(map[string]any)
		if info == nil {
			info = entry
		}
		id := firstString(info, "bookId", "id")
		if id == "" {
			continue
		}
		books = append(books, Book{
			Title: firstString(info, "title"), Authors: firstString(info, "author"),
			Intro: firstString(info, "intro"), Cover: firstString(info, "cover"),
			Meta: BookMeta{Publisher: firstString(info, "publisher"), Published: firstString(info, "publishTime"), ISBN: firstString(info, "isbn")},
		})
	}
	return FromBooks("weread", books), nil
}

const qimaoSignKey = "d3dGiJc651gSQ8w1"

func qimaoSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	b.WriteString(qimaoSignKey)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func searchQimao(ctx context.Context, query string) ([]Row, error) {
	versions := []string{"73720", "73700", "73620", "73600", "73500", "73420", "73400", "73328", "73325", "73320"}
	headers := map[string]string{
		"AUTHORIZATION":  "",
		"app-version":    versions[0],
		"application-id": "com.****.reader",
		"channel":        "unknown",
		"net-env":        "1",
		"platform":       "android",
		"qm-params":      "",
		"reg":            "0",
	}
	signHeaders := map[string]string{}
	for k, v := range headers {
		signHeaders[k] = v
	}
	headers["sign"] = qimaoSign(signHeaders)
	headers["Accept"] = "application/json"
	queryParams := map[string]string{
		"extend": "", "tab": "0", "gender": "0", "refresh_state": "8",
		"page": "1", "wd": query, "is_short_story_user": "0",
	}
	queryParams["sign"] = qimaoSign(queryParams)
	rawURL := "https://api-bc.wtzw.com/search/v1/words?" + formEncode(queryParams)
	body, code, err := do(ctx, "GET", rawURL, headers)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Data struct {
			Books []map[string]any `json:"books"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("响应无效")
	}
	var books []Book
	for _, row := range wire.Data.Books {
		id := firstString(row, "id", "book_id")
		if id == "" {
			continue
		}
		books = append(books, Book{
			Title:    firstString(row, "original_title", "title", "book_title"),
			Authors:  firstString(row, "original_author", "author"),
			Category: firstString(row, "category2_name", "ptags"),
			Intro:    stripHTML(firstString(row, "intro")),
			Cover:    qimaoCover(firstString(row, "image_link")),
		})
	}
	return FromBooks("qimao", books), nil
}

func qimaoCover(link string) string {
	if strings.HasPrefix(link, "/") {
		return "https://cdn.wtzw.com" + link
	}
	return link
}

const jjwxcUA = "Mozilla/5.0 (Linux; Android 5.1; Lenovo) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/39.0.0.0 Mobile Safari/537.36/JINJIANG-Android/206(Lenovo;android 5.1;Scale/2.0)"

func searchJJWXC(ctx context.Context, query string) ([]Row, error) {
	params := map[string]string{
		"keyword":     urlEncode(query),
		"type":        "1",
		"page":        "1",
		"versionCode": "206",
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	rawURL := "https://app-cdn.jjwxc.net/androidapi/search?" + strings.Join(parts, "&")
	body, code, err := do(ctx, "GET", rawURL, map[string]string{
		"User-Agent": jjwxcUA,
		"Referer":    "http://android.jjwxc.net?v=206",
		"Accept":     "application/json",
	})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("响应无效")
	}
	var books []Book
	for _, item := range wire.Items {
		id := firstString(item, "novelid")
		if id == "" {
			continue
		}
		books = append(books, Book{
			Title:   firstString(item, "novelname"),
			Authors: firstString(item, "authorname"),
			Intro:   stripHTML(firstString(item, "novelintroshort")),
			Cover:   jjwxcCover(firstString(item, "ebookurl")),
		})
	}
	return FromBooks("jjwxc", books), nil
}

func jjwxcCover(url string) string {
	if url == "" {
		return ""
	}
	if i := strings.Index(url, "&ver="); i >= 0 {
		end := strings.Index(url[i+1:], "&")
		if end < 0 {
			return url[:i]
		}
		return url[:i] + url[i+1+end:]
	}
	return url
}

const qidianUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1"

var qidianCSRF = func() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}()

func searchQidian(ctx context.Context, query string) ([]Row, error) {
	cookie := "_csrfToken=" + qidianCSRF
	params := map[string]string{"kw": query, "pageNum": "1", "_csrfToken": qidianCSRF}
	rawURL := "https://m.qidian.com/webcommon/search/list?" + formEncode(params)
	body, code, err := do(ctx, "GET", rawURL, map[string]string{
		"Cookie":     cookie,
		"User-Agent": qidianUA,
		"Referer":    "https://m.qidian.com/bookshelf/my/",
		"Origin":     "https://m.qidian.com",
		"Accept":     "application/json",
	})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Code int `json:"code"`
		Data struct {
			BookInfo struct {
				Records []map[string]any `json:"records"`
			} `json:"bookInfo"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &wire); err != nil || wire.Code != 0 {
		if wire.Msg != "" {
			return nil, fmt.Errorf("%s", wire.Msg)
		}
		return nil, fmt.Errorf("起点搜索响应无效")
	}
	var books []Book
	for _, row := range wire.Data.BookInfo.Records {
		id := firstString(row, "bid")
		if id == "" {
			continue
		}
		author := firstString(row, "bAuth")
		if strings.HasSuffix(author, "...") {
			author = ""
		}
		books = append(books, Book{
			Title:    firstString(row, "bName"),
			Authors:  author,
			Category: firstString(row, "cat"),
			Intro:    stripHTML(firstString(row, "desc")),
			Cover:    "https://bookcover.yuewen.com/qdbimg/349573/" + id + "/180",
		})
	}
	return FromBooks("qidian", books), nil
}

const fanqieAppUA = "com.dragon.read/6.5.3.32 (Linux; U; Android 13; zh_CN; VDE-AL10; Build/TP1A.220624.014;tt-ok/3.10.0.2)"

func searchFanqie(ctx context.Context, query string) ([]Row, error) {
	rawURL := "https://novel.snssdk.com/api/novel/channel/homepage/search/search/v1/?aid=1967&offset=0&q=" + urlEncode(query)
	body, code, err := do(ctx, "GET", rawURL, map[string]string{
		"User-Agent": fanqieAppUA,
		"Accept":     "application/json, text/plain, */*",
	})
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Code int `json:"code"`
		Data struct {
			RetData []map[string]any `json:"ret_data"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("响应无效")
	}
	if wire.Code != 0 {
		if wire.Message != "" {
			return nil, fmt.Errorf("%s", wire.Message)
		}
		return nil, fmt.Errorf("番茄搜索失败")
	}
	var books []Book
	for _, row := range wire.Data.RetData {
		id := firstString(row, "book_id")
		if id == "" {
			continue
		}
		cover := fanqieCover(firstString(row, "thumb_url"))
		if cover == "" {
			cover = fanqieCover(firstString(row, "audio_thumb_uri"))
		}
		books = append(books, Book{
			Title:    firstString(row, "title"),
			Authors:  firstString(row, "author"),
			Category: firstString(row, "category"),
			Intro:    firstString(row, "abstract"),
			Cover:    cover,
		})
	}
	return FromBooks("fanqie", books), nil
}

func fanqieCover(thumb string) string {
	if thumb == "" {
		return ""
	}
	path := ""
	if m := regexp.MustCompile(`(novel-pic/[^~?#]+)`).FindStringSubmatch(thumb); m != nil {
		path = m[1]
	} else if m := regexp.MustCompile(`(novel-static/[^~?#]+)`).FindStringSubmatch(thumb); m != nil {
		path = m[1]
	}
	if path == "" {
		if strings.HasPrefix(thumb, "http://") || strings.HasPrefix(thumb, "https://") {
			return thumb
		}
		path = thumb
	}
	return "https://p6-novel.byteimg.com/" + path + "~tplv-shrink:320:0.image"
}

const changpeiUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func searchChangpei(ctx context.Context, query string) ([]Row, error) {
	rawURL := "https://www.gongzicp.com/webapi/search/novels?page=1&k=" + urlEncode(query)
	body, code, err := do(ctx, "GET", rawURL, map[string]string{
		"Client":     "pc",
		"Referer":    "https://www.gongzicp.com/",
		"User-Agent": changpeiUA,
		"Accept":     "application/json",
	})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	var wire struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil || wire.Code != 200 || wire.Data.List == nil {
		if wire.Msg != "" && wire.Code != 200 {
			return nil, fmt.Errorf("%s", wire.Msg)
		}
		return nil, fmt.Errorf("响应无效")
	}
	var books []Book
	for _, row := range wire.Data.List {
		id := firstString(row, "novel_id")
		if id == "" {
			continue
		}
		category := firstString(row, "novel_type")
		intro := firstString(row, "novel_info")
		if intro == "" {
			intro = firstString(row, "novel_desc")
		}
		books = append(books, Book{
			Title:    firstString(row, "novel_name"),
			Authors:  firstString(row, "novel_author"),
			Category: category,
			Intro:    trim(intro),
			Cover:    changpeiCover(firstString(row, "novel_cover")),
		})
	}
	return FromBooks("changpei", books), nil
}

func changpeiCover(url string) string {
	if url == "" || strings.Contains(url, "nocover") {
		return ""
	}
	if i := strings.Index(url, "?"); i >= 0 {
		return url[:i]
	}
	return url
}

const xiaoxiangUA = qidianUA

var reLongInt = regexp.MustCompile(`("[\w]+"\s*:\s*)(\d{16,})([,}\]\s])`)

func searchXiaoxiang(ctx context.Context, query string) ([]Row, error) {
	var all []Book
	for index := 1; index <= 3; index++ {
		rawURL := "https://www.xxsypro.com/api/xxsy/bookstore/web/search?key=" + urlEncode(query) +
			"&start=" + strconv.Itoa((index-1)*20) + "&n=20&_csrfToken=" + urlEncode("moon")
		body, code, err := do(ctx, "GET", rawURL, map[string]string{
			"Cookie":     "csrfToken=moon",
			"Referer":    "https://www.xxsypro.com/",
			"User-Agent": xiaoxiangUA,
			"Accept":     "application/json",
		})
		if err != nil {
			if len(all) > 0 {
				return FromBooks("xiaoxiang", all), nil
			}
			return nil, err
		}
		if code != 200 {
			if len(all) > 0 {
				return FromBooks("xiaoxiang", all), nil
			}
			return nil, fmt.Errorf("网络请求失败 (%d)", code)
		}
		quoted := reLongInt.ReplaceAllString(string(body), `${1}"${2}"${3}`)
		var wire struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				SearchData struct {
					Total    float64          `json:"total"`
					CardList []map[string]any `json:"cardList"`
				} `json:"searchData"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(quoted), &wire); err != nil || wire.Code != 0 {
			if len(all) > 0 {
				return FromBooks("xiaoxiang", all), nil
			}
			if wire.Msg != "" {
				return nil, fmt.Errorf("%s", wire.Msg)
			}
			return nil, fmt.Errorf("响应无效")
		}
		books, err := xiaoxiangBooks(wire.Data.SearchData.CardList)
		if err != nil {
			return nil, err
		}
		all = append(all, books...)
		if float64(index*20) >= wire.Data.SearchData.Total || len(all) >= 60 {
			break
		}
	}
	return FromBooks("xiaoxiang", all), nil
}

func xiaoxiangBooks(rows []map[string]any) ([]Book, error) {
	var books []Book
	for _, row := range rows {
		id := firstString(row, "cbId", "cbid")
		title := firstString(row, "bookName", "title")
		if id == "" || !regexp.MustCompile(`^\d+$`).MatchString(id) || title == "" {
			return nil, fmt.Errorf("响应无效")
		}
		books = append(books, Book{
			Title:    trim(title),
			Authors:  trim(firstString(row, "authorName", "author")),
			Category: firstString(row, "cate2Name", "catel2name", "categoryShortName"),
			Intro:    trim(firstString(row, "intro")),
			Cover:    "https://bookcover.yuewen.com/qdbimg/349573/c_" + id + "/180",
		})
	}
	return books, nil
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := row[key]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case string:
			if strings.TrimSpace(n) != "" {
				return n
			}
		case float64:
			if n == float64(int64(n)) {
				return strconv.FormatInt(int64(n), 10)
			}
			return strconv.FormatFloat(n, 'f', -1, 64)
		case json.Number:
			return n.String()
		}
	}
	return ""
}
