// Derived from moon (https://github.com/AnkioTomas/moon), GNU GPLv3.

package scrape

import (
	"fmt"
	"regexp"
	"strings"
)

// Row is one scrape hit. Field names follow moon's Novel.fromBooks.
type Row struct {
	Title        string            `json:"title"`
	Author       string            `json:"author"`
	Intro        string            `json:"intro"`
	Tags         []string          `json:"tags"`
	CoverURL     string            `json:"coverUrl"`
	CoverHeaders map[string]string `json:"coverHeaders,omitempty"`
	Publisher    string            `json:"publisher"`
	Year         string            `json:"year"`
	ISBN         string            `json:"isbn"`
	Rating       string            `json:"rating"`
	Series       string            `json:"series"`
	Source       string            `json:"source"`
	URL          string            `json:"url,omitempty"`
	DoubanID     string            `json:"doubanId,omitempty"`
	Similarity   float64           `json:"similarity,omitempty"`
}

// Source is one entry in Search.SOURCES, in that order.
type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Sources is the picker order from moon Search.SOURCES.
func Sources() []Source {
	return []Source{
		{ID: "douban", Name: "豆瓣"},
		{ID: "weread", Name: "微信读书"},
		{ID: "qimao", Name: "七猫小说"},
		{ID: "jjwxc", Name: "晋江文学城"},
		{ID: "qidian", Name: "起点中文网"},
		{ID: "fanqie", Name: "番茄小说"},
		{ID: "changpei", Name: "长佩文学"},
		{ID: "xiaoxiang", Name: "潇湘书院"},
	}
}

func sourceName(id string) string {
	for _, src := range Sources() {
		if src.ID == id {
			return src.Name
		}
	}
	return id
}

// Book is the slice of a moon Book that fromBooks reads.
type Book struct {
	Title    string
	Authors  string
	Intro    string
	Category string
	Cover    string
	Series   string
	Meta     BookMeta
}

// BookMeta is book.meta.
type BookMeta struct {
	Publisher string
	Published string
	ISBN      string
	Rating    float64
}

var yearRE = regexp.MustCompile(`\d{4}`)

// FromBooks maps moon Book rows into scrape rows. label is the source id.
func FromBooks(label string, books []Book) []Row {
	out := make([]Row, 0, len(books))
	for _, book := range books {
		score := book.Meta.Rating
		rating := ""
		if score > 0 {
			rating = fmt.Sprintf("%.1f", score)
		}
		tags := []string{}
		if strings.TrimSpace(book.Category) != "" {
			tags = []string{strings.TrimSpace(book.Category)}
		}
		year := ""
		if m := yearRE.FindString(book.Meta.Published); m != "" {
			year = m
		}
		out = append(out, Row{
			Title:     trim(book.Title),
			Author:    trim(book.Authors),
			Intro:     book.Intro,
			Tags:      tags,
			CoverURL:  book.Cover,
			Publisher: trim(book.Meta.Publisher),
			Year:      year,
			ISBN:      trim(book.Meta.ISBN),
			Rating:    rating,
			Series:    trim(book.Series),
			Source:    label,
		})
	}
	return out
}
