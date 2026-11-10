// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package scrape

import (
	"context"
	"fmt"
	"strings"
)

// Search runs one source. An empty result list is returned as 无搜索结果 by the caller
// when the source itself did not fail.
func Search(ctx context.Context, query, sourceID string) ([]Row, error) {
	query = trim(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键词为空")
	}
	var (
		rows []Row
		err  error
	)
	switch sourceID {
	case "douban":
		rows, err = searchDouban(ctx, query)
	case "weread":
		rows, err = searchWeread(ctx, query)
	case "qimao":
		rows, err = searchQimao(ctx, query)
	case "jjwxc":
		rows, err = searchJJWXC(ctx, query)
	case "qidian":
		rows, err = searchQidian(ctx, query)
	case "fanqie":
		rows, err = searchFanqie(ctx, query)
	case "changpei":
		rows, err = searchChangpei(ctx, query)
	case "xiaoxiang":
		rows, err = searchXiaoxiang(ctx, query)
	default:
		return nil, fmt.Errorf("未知数据源")
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("无搜索结果")
	}
	for i := range rows {
		if rows[i].Tags == nil {
			rows[i].Tags = []string{}
		}
		if rows[i].Source == "" {
			rows[i].Source = sourceID
		}
	}
	return rows, nil
}

// Name returns the Chinese label for a source id.
func Name(id string) string {
	name := sourceName(id)
	if name == id && !known(id) {
		return id
	}
	return name
}

func known(id string) bool {
	for _, src := range Sources() {
		if src.ID == id {
			return true
		}
	}
	return false
}

// Stars maps a 10-point scrape rating onto the 0–5 star field.
func Stars(rating string) int {
	rating = strings.TrimSpace(rating)
	if rating == "" {
		return 0
	}
	var n float64
	if _, err := fmt.Sscanf(rating, "%f", &n); err != nil || n <= 0 {
		return 0
	}
	stars := int(n + 0.5)
	if n > 5 {
		stars = int(n/2 + 0.5)
	}
	if stars < 1 {
		return 1
	}
	if stars > 5 {
		return 5
	}
	return stars
}
