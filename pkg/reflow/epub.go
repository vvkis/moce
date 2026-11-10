package reflow

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"strings"

	"moce/pkg/chapters"
)

// Book is the text and metadata pulled out of an existing ebook.
type Book struct {
	Text       string
	Title      string
	Author     string
	Publisher  string
	Language   string
	Published  string
	Identifier string
	Intro      string
	Tags       []string
	Cover      []byte
	CoverName  string
	Headings   []chapters.Heading
	Format     string
}

func openEPUB(data []byte) (Book, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Book{}, fmt.Errorf("无法读取 EPUB")
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	root, err := readZip(files, "META-INF/container.xml")
	if err != nil {
		return Book{}, fmt.Errorf("无法读取 EPUB")
	}
	opfPath := containerRoot(root)
	if opfPath == "" {
		return Book{}, fmt.Errorf("无法读取 EPUB")
	}
	opf, err := readZip(files, opfPath)
	if err != nil {
		return Book{}, fmt.Errorf("无法读取 EPUB")
	}
	book, err := parseOPF(opf, path.Dir(opfPath), files)
	if err != nil {
		return Book{}, err
	}
	book.Format = "EPUB"
	return book, nil
}

func readZip(files map[string]*zip.File, name string) ([]byte, error) {
	f := files[name]
	if f == nil {
		for k, v := range files {
			if strings.EqualFold(k, name) {
				f = v
				break
			}
		}
	}
	if f == nil {
		return nil, fmt.Errorf("missing")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	buf := make([]byte, 0, f.UncompressedSize64)
	tmp := make([]byte, 32<<10)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
		if len(buf) > 40<<20 {
			break
		}
	}
	return buf, nil
}

func containerRoot(data []byte) string {
	var doc struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if xml.Unmarshal(data, &doc) != nil || len(doc.Rootfiles) == 0 {
		return ""
	}
	return doc.Rootfiles[0].FullPath
}

func parseOPF(data []byte, dir string, files map[string]*zip.File) (Book, error) {
	var opf struct {
		Metadata struct {
			Titles      []string `xml:"title"`
			Creators    []string `xml:"creator"`
			Publisher   []string `xml:"publisher"`
			Language    []string `xml:"language"`
			Date        []string `xml:"date"`
			Identifiers []string `xml:"identifier"`
			Description []string `xml:"description"`
			Subjects    []string `xml:"subject"`
			Meta        []struct {
				Name     string `xml:"name,attr"`
				Content  string `xml:"content,attr"`
				Property string `xml:"property,attr"`
			} `xml:"meta"`
		} `xml:"metadata"`
		Manifest struct {
			Items []struct {
				ID         string `xml:"id,attr"`
				Href       string `xml:"href,attr"`
				MediaType  string `xml:"media-type,attr"`
				Properties string `xml:"properties,attr"`
			} `xml:"item"`
		} `xml:"manifest"`
		Spine struct {
			Refs []struct {
				IDRef string `xml:"idref,attr"`
			} `xml:"itemref"`
		} `xml:"spine"`
	}
	if err := xml.Unmarshal(data, &opf); err != nil {
		return Book{}, fmt.Errorf("无法读取 EPUB")
	}
	byID := map[string]string{}
	var navHref, ncxHref, coverHref, coverName string
	for _, item := range opf.Manifest.Items {
		href := path.Join(dir, item.Href)
		byID[item.ID] = href
		if strings.Contains(item.Properties, "nav") || strings.HasSuffix(item.Href, "nav.xhtml") {
			navHref = href
		}
		if strings.Contains(item.MediaType, "dtbncx") || strings.HasSuffix(item.Href, ".ncx") {
			ncxHref = href
		}
		if strings.Contains(item.Properties, "cover-image") {
			coverHref, coverName = href, path.Base(item.Href)
		}
	}
	for _, m := range opf.Metadata.Meta {
		if m.Name == "cover" && coverHref == "" {
			if href := byID[m.Content]; href != "" {
				coverHref, coverName = href, path.Base(href)
			}
		}
	}
	var text strings.Builder
	for _, ref := range opf.Spine.Refs {
		href := byID[ref.IDRef]
		if href == "" {
			continue
		}
		body, err := readZip(files, href)
		if err != nil {
			continue
		}
		chunk := HTMLToText(string(body))
		if chunk == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(chunk)
	}
	if strings.TrimSpace(text.String()) == "" {
		return Book{}, fmt.Errorf("这本书里没有可排版的正文")
	}
	var headings []chapters.Heading
	if navHref != "" {
		if raw, err := readZip(files, navHref); err == nil {
			headings = navHeadings(raw)
		}
	}
	if len(headings) == 0 && ncxHref != "" {
		if raw, err := readZip(files, ncxHref); err == nil {
			headings = ncxHeadings(raw)
		}
	}
	out := Book{
		Text:       text.String(),
		Title:      first(opf.Metadata.Titles),
		Author:     strings.Join(cleanList(opf.Metadata.Creators), " & "),
		Publisher:  first(opf.Metadata.Publisher),
		Language:   first(opf.Metadata.Language),
		Published:  first(opf.Metadata.Date),
		Identifier: isbnOf(opf.Metadata.Identifiers),
		Intro:      first(opf.Metadata.Description),
		Tags:       cleanList(opf.Metadata.Subjects),
		Headings:   headings,
	}
	if coverHref != "" {
		if raw, err := readZip(files, coverHref); err == nil && len(raw) > 0 {
			out.Cover = raw
			out.CoverName = coverName
		}
	}
	return out, nil
}

func first(v []string) string {
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func cleanList(v []string) []string {
	var out []string
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func isbnOf(ids []string) string {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.Contains(strings.ToLower(id), "isbn") || len(digits(id)) >= 10 {
			return id
		}
	}
	return first(ids)
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func navHeadings(data []byte) []chapters.Heading {
	return nestHeadings(string(data), "ol", "li", "a")
}

func ncxHeadings(data []byte) []chapters.Heading {
	return nestHeadings(string(data), "navMap", "navPoint", "text")
}

func nestHeadings(s, list, item, label string) []chapters.Heading {
	var out []chapters.Heading
	var walk func(chunk string, depth int)
	walk = func(chunk string, depth int) {
		for _, block := range splitBlocks(chunk, item) {
			title := innerText(block, label)
			if title != "" {
				out = append(out, chapters.Heading{Title: title, Depth: depth})
			}
			if nested := nestedBlock(block, list); nested != "" {
				walk(nested, depth+1)
			}
		}
	}
	body := nestedBlock(s, list)
	if body == "" {
		body = s
	}
	walk(body, 1)
	return out
}

func splitBlocks(s, tag string) []string {
	low := strings.ToLower(s)
	open := "<" + tag
	close := "</" + tag + ">"
	var out []string
	i := 0
	for {
		start := strings.Index(low[i:], open)
		if start < 0 {
			break
		}
		start += i
		gt := strings.Index(low[start:], ">")
		if gt < 0 {
			break
		}
		contentStart := start + gt + 1
		depth := 1
		pos := contentStart
		end := -1
		for pos < len(low) && depth > 0 {
			nextOpen := strings.Index(low[pos:], open)
			nextClose := strings.Index(low[pos:], close)
			if nextClose < 0 {
				break
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				depth++
				pos += nextOpen + len(open)
				continue
			}
			depth--
			if depth == 0 {
				end = pos + nextClose
				break
			}
			pos += nextClose + len(close)
		}
		if end < 0 {
			break
		}
		out = append(out, s[contentStart:end])
		i = end + len(close)
	}
	return out
}

func nestedBlock(s, tag string) string {
	low := strings.ToLower(s)
	open := "<" + tag
	idx := strings.Index(low, open)
	if idx < 0 {
		return ""
	}
	gt := strings.Index(low[idx:], ">")
	if gt < 0 {
		return ""
	}
	return s[idx+gt+1:]
}

func innerText(block, tag string) string {
	low := strings.ToLower(block)
	open := "<" + tag
	idx := strings.Index(low, open)
	if idx < 0 {
		return ""
	}
	gt := strings.Index(low[idx:], ">")
	if gt < 0 {
		return ""
	}
	rest := block[idx+gt+1:]
	end := strings.Index(strings.ToLower(rest), "</"+tag)
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(HTMLToText(rest[:end]))
}
