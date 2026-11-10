package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxBody = 4 << 20

// Transport is replaced by tests. Production uses the default client.
var Transport http.RoundTripper

func do(ctx context.Context, method, rawURL string, headers map[string]string) ([]byte, int, error) {
	if method == http.MethodGet {
		if body, ok := readHTTPCache(rawURL); ok {
			return body, 200, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("请求失败")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: Transport}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, fmt.Errorf("已取消")
		}
		return nil, 0, fmt.Errorf("网络请求失败")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("网络请求失败")
	}
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	if method == http.MethodGet && res.StatusCode == 200 && len(body) > 0 {
		writeHTTPCache(rawURL, body)
	}
	return body, res.StatusCode, nil
}

// FetchCover downloads a cover. headers are sent as-is (豆瓣 needs Referer).
func FetchCover(ctx context.Context, rawURL string, headers map[string]string) ([]byte, string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return nil, "", "", fmt.Errorf("封面地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("封面地址无效")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: Transport}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", "", fmt.Errorf("已取消")
		}
		return nil, "", "", fmt.Errorf("封面下载失败")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, "", "", fmt.Errorf("封面下载失败 (%d)", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if err != nil || len(body) == 0 || len(body) > 8<<20 {
		return nil, "", "", fmt.Errorf("封面下载失败")
	}
	mime := res.Header.Get("Content-Type")
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = mime[:i]
	}
	mime = strings.TrimSpace(mime)
	if !strings.HasPrefix(mime, "image/") {
		mime = sniffImage(body)
	}
	if mime == "" {
		return nil, "", "", fmt.Errorf("封面下载失败")
	}
	path, err := storeCover(body, mime)
	if err != nil {
		return nil, "", "", fmt.Errorf("封面保存失败")
	}
	return body, mime, path, nil
}

func sniffImage(b []byte) string {
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg"
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}
