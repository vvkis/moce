// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package scrape

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
)

const doubanSearchURL = "https://www.douban.com/search"

var doubanCoverHeaders = map[string]string{"Referer": "https://book.douban.com/"}

var (
	reDoubanTitle1 = regexp.MustCompile(`(?s)<h3>.*?<a\s+href="([^"]+)"[^>]*>([^<]+)</a>`)
	reDoubanTitle2 = regexp.MustCompile(`(?s)<h3>.*?<a\s+href="([^"]+)"[^>]*>\s*<span[^>]*>([^<]+)</span>`)
	reDoubanCast   = regexp.MustCompile(`class="subject-cast"[^>]*>([^<]+)<`)
	reDoubanCover  = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"`)
	reDoubanRating = regexp.MustCompile(`<span class="rating_nums">([^<]+)</span>`)
	reDoubanIntro  = regexp.MustCompile(`<p>([^<]+)</p>`)
	reSubjectPath  = regexp.MustCompile(`subject/(\d+)`)
	reSid          = regexp.MustCompile(`sid:\s*(\d+)`)
	reYear4        = regexp.MustCompile(`^(\d{4})$`)
	reYearDate     = regexp.MustCompile(`^(\d{4})-`)
)

// ParseDoubanSearch parses a douban search HTML page the way moon's scrape/douban.lua does.
func ParseDoubanSearch(html, query string) []Row {
	var starts []int
	pos := 0
	for {
		i := strings.Index(html[pos:], `<div class="result">`)
		if i < 0 {
			break
		}
		starts = append(starts, pos+i)
		pos = pos + i + 1
	}
	var results []Row
	for i, from := range starts {
		to := len(html)
		if i+1 < len(starts) {
			to = starts[i+1]
		}
		item, ok := parseDoubanBlock(html[from:to], query)
		if ok {
			results = append(results, item)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

func parseDoubanBlock(block, query string) (Row, bool) {
	url, title, ok := firstTwo(reDoubanTitle1, block)
	if !ok {
		url, title, ok = firstTwo(reDoubanTitle2, block)
	}
	if !ok {
		return Row{}, false
	}
	title = trim(xmlDecode(title))
	id := extractSubjectID(block, url)
	if id == "" {
		return Row{}, false
	}
	sim := similarity(query, title)
	if sim < 0.6 {
		return Row{}, false
	}
	cast := ""
	if m := reDoubanCast.FindStringSubmatch(block); m != nil {
		cast = xmlDecode(m[1])
	}
	author, publisher, year := parseCast(cast)
	cover := ""
	if m := reDoubanCover.FindStringSubmatch(block); m != nil {
		cover = xmlDecode(m[1])
	}
	rating := ""
	if m := reDoubanRating.FindStringSubmatch(block); m != nil {
		rating = trim(xmlDecode(m[1]))
	}
	intro := ""
	if m := reDoubanIntro.FindStringSubmatch(block); m != nil {
		intro = stripHTML(m[1])
	}
	return Row{
		Title:        title,
		Author:       author,
		Publisher:    publisher,
		Year:         year,
		CoverURL:     cover,
		CoverHeaders: doubanCoverHeaders,
		Rating:       rating,
		Intro:        intro,
		URL:          "https://book.douban.com/subject/" + id + "/",
		DoubanID:     id,
		Similarity:   sim,
		Source:       "douban",
		Tags:         []string{},
	}, true
}

func firstTwo(re *regexp.Regexp, s string) (string, string, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func extractSubjectID(block, rawURL string) string {
	decoded := urlDecode(xmlDecode(rawURL))
	if m := reSubjectPath.FindStringSubmatch(decoded); m != nil {
		return m[1]
	}
	if m := reSid.FindStringSubmatch(block); m != nil {
		return m[1]
	}
	return ""
}

func parseCast(cast string) (author, publisher, year string) {
	if strings.TrimSpace(cast) == "" {
		return "", "", ""
	}
	var parts []string
	for _, p := range strings.Split(cast, "/") {
		p = trim(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		author = parts[0]
	}
	yearIdx := -1
	for i, part := range parts {
		if m := reYear4.FindStringSubmatch(part); m != nil {
			year = m[1]
			yearIdx = i
			break
		}
		if m := reYearDate.FindStringSubmatch(part); m != nil {
			year = m[1]
			yearIdx = i
			break
		}
	}
	if yearIdx > 1 {
		publisher = parts[yearIdx-1]
	} else if yearIdx < 0 && len(parts) > 2 {
		publisher = parts[len(parts)-2]
	}
	return author, publisher, year
}

func searchDouban(ctx context.Context, query string) ([]Row, error) {
	query = trim(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键词为空")
	}
	rawURL := doubanSearchURL + "?cat=1001&q=" + urlEncode(query)
	body, code, err := do(ctx, "GET", rawURL, map[string]string{
		"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"X-Forwarded-For":           fmt.Sprintf("203.0.113.%d", rand.Intn(200)+1),
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8",
		"Accept-Encoding":           "identity",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
		"Cache-Control":             "max-age=0",
	})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("网络请求失败 (%d)", code)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("响应为空")
	}
	return ParseDoubanSearch(string(body), query), nil
}
