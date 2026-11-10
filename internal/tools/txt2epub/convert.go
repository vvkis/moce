package txt2epub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"moce/internal/toolhost"
	"moce/pkg/chapters"
	"moce/pkg/coverimg"
	"moce/pkg/encoding"
	"moce/pkg/epub"
	"moce/pkg/reflow"
	"moce/pkg/scrape"
)

const maxTextBytes = 80 << 20
const maxImageBytes = 20 << 20
const maxFontBytes = 30 << 20

var (
	reAuthorLine   = regexp.MustCompile(`(?m)^[ \t\x{3000}]*作[ \t\x{3000}]*者[ \t\x{3000}]*[:：][ \t\x{3000}]*(\S.*?)\s*$`)
	reBookName     = regexp.MustCompile(`(?m)^[ \t\x{3000}]*书[ \t\x{3000}]*名[ \t\x{3000}]*[:：][ \t\x{3000}]*(\S.*?)\s*$`)
	reBracket      = regexp.MustCompile(`《([^》]+)》`)
	reAuthorInFile = regexp.MustCompile(`作者\s*[:：]\s*(\S+)`)
)

// Tool converts Chinese TXT novels to EPUB and can scrape metadata.
type Tool struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// New returns the TXT to EPUB tool.
func New() *Tool { return &Tool{} }

// Options are the shared conversion settings.
type Options struct {
	ChapterPattern   string  `json:"chapterPattern"`
	VolumePattern    string  `json:"volumePattern"`
	MergeBrokenLines bool    `json:"mergeBrokenLines"`
	DropEmptyLines   bool    `json:"dropEmptyLines"`
	BaseFontSize     int     `json:"baseFontSize"`
	LineHeight       float64 `json:"lineHeight"`
	MarginPercent    float64 `json:"marginPercent"`
	ParagraphEm      float64 `json:"paragraphEm"`
	IndentEm         float64 `json:"indentEm"`
	FallbackRunes    int     `json:"fallbackRunes"`
	RemoveDupCover   bool    `json:"removeDupCover"`
}

// ProbeResult is the chapter preview.
type ProbeResult struct {
	Title        string           `json:"title"`
	Author       string           `json:"author"`
	Encoding     string           `json:"encoding"`
	Confidence   float64          `json:"confidence"`
	BOM          bool             `json:"bom"`
	Fallback     bool             `json:"fallback"`
	Warning      string           `json:"warning,omitempty"`
	ChapterCount int              `json:"chapterCount"`
	VolumeCount  int              `json:"volumeCount"`
	RuneCount    int              `json:"runeCount"`
	Entries      []chapters.Entry `json:"entries"`
	Publisher    string           `json:"publisher,omitempty"`
	Language     string           `json:"language,omitempty"`
	Published    string           `json:"published,omitempty"`
	Identifier   string           `json:"identifier,omitempty"`
	Intro        string           `json:"intro,omitempty"`
	Tags         string           `json:"tags,omitempty"`
	CoverBase64  string           `json:"coverBase64,omitempty"`
	CoverName    string           `json:"coverName,omitempty"`
	Format       string           `json:"format,omitempty"`
}

// ConvertResult is a finished EPUB.
type ConvertResult struct {
	Path         string `json:"path,omitempty"`
	Filename     string `json:"filename"`
	Bytes        int    `json:"bytes"`
	DataBase64   string `json:"dataBase64,omitempty"`
	ChapterCount int    `json:"chapterCount"`
}

type fileIn struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	DataBase64    string  `json:"dataBase64"`
	Title         string  `json:"title"`
	TitleEdited   bool    `json:"titleEdited"`
	Author        string  `json:"author"`
	AuthorEdited  bool    `json:"authorEdited"`
	Series        string  `json:"series"`
	SeriesIndex   string  `json:"seriesIndex"`
	Tags          string  `json:"tags"`
	Publisher     string  `json:"publisher"`
	Language      string  `json:"language"`
	Published     string  `json:"published"`
	Identifier    string  `json:"identifier"`
	Intro         string  `json:"intro"`
	Rating        int     `json:"rating"`
	GenerateCover bool    `json:"generateCover"`
	Encoding      string  `json:"encoding"`
	CoverPath     string  `json:"coverPath"`
	CoverBase64   string  `json:"coverBase64"`
	CoverName     string  `json:"coverName"`
	FontPath      string  `json:"fontPath"`
	FontBase64    string  `json:"fontBase64"`
	FontName      string  `json:"fontName"`
	OutputDir     string  `json:"outputDir"`
	ReturnBytes   bool    `json:"returnBytes"`
	Options       Options `json:"options"`
}

// Meta describes the tool for the shell.
func (t *Tool) Meta() toolhost.Meta {
	num := func(v float64) *float64 { return &v }
	return toolhost.Meta{
		ID:          "txt2epub",
		Name:        "TXT 转 EPUB",
		Description: "把中文 TXT 小说做成适合 KOReader 的 EPUB",
		Panel:       "txt2epub",
		Schema: []toolhost.Field{
			{Key: "chapterPattern", Label: "章节正则", Type: "textarea", Default: chapters.DefaultChapterPattern, Help: "匹配一行的开头，使用 RE2 语法。默认识别「第1章」「第一章」「序章」「楔子」「番外」「尾声」「后记」。"},
			{Key: "volumePattern", Label: "分卷正则", Type: "textarea", Default: chapters.DefaultVolumePattern, Help: "识别「第一卷」「第一部」。同一本书里还有章节时，会做成二级目录。"},
			{Key: "mergeBrokenLines", Label: "合并断行", Type: "bool", Default: true, Help: "把被折断的句子接回同一段。行首缩进或空行表示新的一段。"},
			{Key: "dropEmptyLines", Label: "去掉空行", Type: "bool", Default: true, Help: "空行不写进正文。"},
			{Key: "baseFontSize", Label: "正文字号（点）", Type: "number", Default: 0, Min: num(0), Max: num(36), Step: num(1), Help: "0 表示跟随阅读器。填了则按 12 点为基准写成 em，不用 px，阅读器仍可调字号。"},
			{Key: "lineHeight", Label: "行距", Type: "number", Default: 1.8, Min: num(0), Max: num(3), Step: num(0.05), Help: "相对字号的倍数，默认 1.8。不加 !important。填 0 则不写。"},
			{Key: "marginPercent", Label: "页边距（%）", Type: "number", Default: 5, Min: num(0), Max: num(30), Step: num(0.5), Help: "正文四边页边距，默认 5%。填 0 则不写。"},
			{Key: "paragraphEm", Label: "段间距（em）", Type: "number", Default: 0.45, Min: num(0), Max: num(3), Step: num(0.05), Help: "段落上下间距，默认 0.45em。填 0 则不写。"},
			{Key: "indentEm", Label: "首行缩进（em）", Type: "number", Default: 2, Min: num(0), Max: num(8), Step: num(0.5), Help: "段落首行缩进，默认 2em。填 0 则不缩进。"},
			{Key: "fallbackRunes", Label: "无章节时分段字数", Type: "number", Default: 8000, Min: num(1000), Max: num(200000), Step: num(500), Help: "一本 TXT 里找不到章节时，按这个长度切开。"},
			{Key: "removeDupCover", Label: "去掉重复封面", Type: "bool", Default: true, Help: "正文开头如果只是书名和作者，就不再单独成章。"},
			{
				Key: "encoding", Label: "编码", Type: "select", Scope: "file", Default: "auto",
				Options: []toolhost.Option{
					{Value: "auto", Label: "自动检测"},
					{Value: "utf-8", Label: "UTF-8"},
					{Value: "gb18030", Label: "GB18030 / GBK"},
					{Value: "big5", Label: "Big5"},
					{Value: "utf-16le", Label: "UTF-16 LE"},
					{Value: "utf-16be", Label: "UTF-16 BE"},
				},
			},
		},
	}
}

// Call handles probe, convert, and 刮搜.
func (t *Tool) Call(action string, payload json.RawMessage) (any, error) {
	switch action {
	case "scrape", "scrapeCover", "scrapeKeep", "scrapeCancel", "scrapeSources":
		return t.scrape(action, payload)
	case "enqueue", "listJobs", "cancelJob", "retryJob", "clearJobs":
		return t.jobs(action, payload)
	case "accountList", "accountCommit", "accountDelete", "accountQRStart", "accountQRPoll":
		return t.accounts(action, payload)
	}
	var in fileIn
	if len(payload) > 0 && string(payload) != "null" {
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, errors.New("请求格式不对")
		}
	}
	switch action {
	case "probe":
		return t.probe(in)
	case "convert":
		return t.convert(in)
	default:
		return nil, fmt.Errorf("未知操作：%s", action)
	}
}

func (t *Tool) scrape(action string, payload json.RawMessage) (any, error) {
	switch action {
	case "scrapeSources":
		return scrape.Sources(), nil
	case "scrapeCancel":
		t.stopScrape()
		return map[string]bool{"ok": true}, nil
	case "scrapeKeep":
		var in struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, errors.New("请求格式不对")
		}
		if err := scrape.KeepCover(in.Path); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, nil
	case "scrapeCover":
		var in struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, errors.New("请求格式不对")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		body, mime, path, err := scrape.FetchCover(ctx, in.URL, in.Headers)
		if err != nil {
			return nil, err
		}
		ext := ".img"
		switch mime {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		}
		return map[string]string{
			"dataBase64": base64.StdEncoding.EncodeToString(body),
			"mime":       mime,
			"name":       "cover" + ext,
			"path":       path,
		}, nil
	case "scrape":
		var in struct {
			Query  string `json:"query"`
			Source string `json:"source"`
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &in); err != nil {
				return nil, errors.New("请求格式不对")
			}
		}
		ctx := t.beginScrape()
		rows, err := scrape.Search(ctx, in.Query, in.Source)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"source":     in.Source,
			"sourceName": scrape.Name(in.Source),
			"results":    rows,
		}, nil
	default:
		return nil, fmt.Errorf("未知操作：%s", action)
	}
}

func (t *Tool) beginScrape() context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	t.cancel = cancel
	return ctx
}

func (t *Tool) stopScrape() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
}

func (t *Tool) probe(in fileIn) (ProbeResult, error) {
	opt, err := normalize(in.Options)
	if err != nil {
		return ProbeResult{}, err
	}
	doc, err := loadDocument(in)
	if err != nil {
		return ProbeResult{}, err
	}
	text, res := doc.text, doc.enc
	title := guessTitle(in.Name, text)
	author := guessAuthor(in.Name, text)
	if doc.book != nil {
		if doc.book.Title != "" {
			title = doc.book.Title
		}
		if doc.book.Author != "" {
			author = doc.book.Author
		}
	}
	body := text
	if opt.RemoveDupCover {
		body = stripLeadingMeta(text, title, author)
	}
	split, err := chapters.Split(body, splitOptions(opt, doc))
	if err != nil {
		return ProbeResult{}, err
	}
	warning := ""
	if split.Fallback {
		warning = fmt.Sprintf("未识别到章节标题，已按约 %d 字分段。可以改章节正则后再识别。", opt.FallbackRunes)
	}
	entries := split.Entries
	if entries == nil {
		entries = []chapters.Entry{}
	}
	out := ProbeResult{
		Title:        title,
		Author:       author,
		Encoding:     res.Name,
		Confidence:   res.Confidence,
		BOM:          res.BOM,
		Fallback:     split.Fallback,
		Warning:      warning,
		ChapterCount: split.ChapterCount,
		VolumeCount:  split.VolumeCount,
		RuneCount:    runeCount(text),
		Entries:      entries,
		Format:       formatOf(in.Name),
	}
	if doc.book != nil {
		out.Publisher = doc.book.Publisher
		out.Language = doc.book.Language
		out.Published = doc.book.Published
		out.Identifier = doc.book.Identifier
		out.Intro = doc.book.Intro
		out.Tags = strings.Join(doc.book.Tags, "、")
		if len(doc.book.Cover) > 0 {
			out.CoverBase64 = base64.StdEncoding.EncodeToString(doc.book.Cover)
			out.CoverName = doc.book.CoverName
		}
		if out.Format == "" {
			out.Format = doc.book.Format
		}
	}
	return out, nil
}

func (t *Tool) convert(in fileIn) (ConvertResult, error) {
	report("extracting", 0.18)
	if err := aborted(); err != nil {
		return ConvertResult{}, err
	}
	opt, err := normalize(in.Options)
	if err != nil {
		return ConvertResult{}, err
	}
	doc, err := loadDocument(in)
	if err != nil {
		return ConvertResult{}, err
	}
	text := doc.text
	title := trim(in.Title)
	if title == "" && !in.TitleEdited {
		title = guessTitle(in.Name, text)
		if doc.book != nil && doc.book.Title != "" {
			title = doc.book.Title
		}
	}
	author := trim(in.Author)
	if author == "" && !in.AuthorEdited {
		author = guessAuthor(in.Name, text)
		if doc.book != nil && doc.book.Author != "" {
			author = doc.book.Author
		}
	}
	language, err := normalizeLanguage(in.Language)
	if err != nil {
		return ConvertResult{}, err
	}
	published, err := normalizePublished(in.Published)
	if err != nil {
		return ConvertResult{}, err
	}
	series := trim(in.Series)
	seriesIndex, err := normalizeSeriesIndex(in.SeriesIndex, series != "")
	if err != nil {
		return ConvertResult{}, err
	}
	rating, err := normalizeRating(in.Rating)
	if err != nil {
		return ConvertResult{}, err
	}
	report("detecting", 0.52)
	if err := aborted(); err != nil {
		return ConvertResult{}, err
	}
	body := text
	if opt.RemoveDupCover {
		body = stripLeadingMeta(text, title, author)
	}
	split, err := chapters.Split(body, splitOptions(opt, doc))
	if err != nil {
		return ConvertResult{}, err
	}
	var cover []byte
	var coverName, coverMIME string
	if !in.GenerateCover {
		cover, coverName, coverMIME, err = loadCover(in)
		if err != nil {
			return ConvertResult{}, err
		}
	}
	font, fontExt, fontMIME, fontFormat, err := loadFont(in)
	if err != nil {
		return ConvertResult{}, err
	}
	if len(cover) == 0 && doc.book != nil && len(doc.book.Cover) > 0 && !in.GenerateCover {
		cover = doc.book.Cover
		coverName = doc.book.CoverName
		if sniffed, mime, err := sniffImage(cover); err == nil {
			coverName, coverMIME = sniffed, mime
		}
	}
	if len(cover) == 0 {
		if png, err := coverimg.Render(title, author, font); err == nil {
			cover, coverName, coverMIME = png, "cover.png", "image/png"
		}
	}
	report("packaging", 0.82)
	if err := aborted(); err != nil {
		return ConvertResult{}, err
	}
	book := epub.Book{
		Title:       title,
		Author:      author,
		Language:    language,
		Publisher:   trim(in.Publisher),
		Description: trim(in.Intro),
		Subjects:    splitList(in.Tags),
		Identifiers: normalizeIdentifiers(in.Identifier),
		Published:   published,
		Series:      series,
		SeriesIndex: seriesIndex,
		Rating:      rating,
		Chapters:    toChapters(split.Nodes),
		Style: epub.Style{
			FontSizePt:    opt.BaseFontSize,
			LineHeight:    opt.LineHeight,
			MarginPercent: opt.MarginPercent,
			ParagraphEm:   opt.ParagraphEm,
			IndentEm:      opt.IndentEm,
		},
		Cover:      cover,
		CoverName:  coverName,
		CoverMIME:  coverMIME,
		Font:       font,
		FontExt:    fontExt,
		FontMIME:   fontMIME,
		FontFormat: fontFormat,
	}
	data, err := epub.Write(book)
	if err != nil {
		return ConvertResult{}, err
	}
	filename := sanitizeFilename(title) + ".epub"
	out := ConvertResult{Filename: filename, Bytes: len(data), ChapterCount: split.ChapterCount}
	outDir := trim(in.OutputDir)
	if outDir == "" && in.Path != "" {
		outDir = filepath.Dir(in.Path)
	}
	if outDir == "" && !in.ReturnBytes {
		return ConvertResult{}, errors.New("请选择输出目录")
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return ConvertResult{}, fmt.Errorf("无法创建输出目录")
		}
		outPath := filepath.Join(outDir, filename)
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return ConvertResult{}, fmt.Errorf("无法写入 EPUB")
		}
		out.Path = outPath
	}
	if in.ReturnBytes || out.Path == "" {
		out.DataBase64 = base64.StdEncoding.EncodeToString(data)
	}
	return out, nil
}

func normalize(o Options) (Options, error) {
	if strings.TrimSpace(o.ChapterPattern) == "" {
		o.ChapterPattern = chapters.DefaultChapterPattern
	}
	if strings.TrimSpace(o.VolumePattern) == "" {
		o.VolumePattern = chapters.DefaultVolumePattern
	}
	if o.BaseFontSize < 0 || o.BaseFontSize > 36 {
		return o, errors.New("正文字号填 0（跟随阅读器），或 1 到 36 点")
	}
	if o.LineHeight < 0 || o.LineHeight > 3 {
		return o, errors.New("行距填 0 表示不写，或填 0 到 3 的倍数")
	}
	if o.MarginPercent < 0 || o.MarginPercent > 30 {
		return o, errors.New("页边距填 0 表示不写，或填 0 到 30%")
	}
	if o.ParagraphEm < 0 || o.ParagraphEm > 3 {
		return o, errors.New("段间距填 0 表示不写，或填 0 到 3em")
	}
	if o.IndentEm < 0 || o.IndentEm > 8 {
		return o, errors.New("首行缩进填 0 到 8em")
	}
	if o.FallbackRunes == 0 {
		o.FallbackRunes = chapters.DefaultFallbackRunes
	}
	if o.FallbackRunes < 1000 || o.FallbackRunes > 200000 {
		return o, errors.New("分段字数需在 1000 到 200000 之间")
	}
	if _, err := regexp.Compile(o.ChapterPattern); err != nil {
		return o, fmt.Errorf("章节正则无效：%v", err)
	}
	if _, err := regexp.Compile(o.VolumePattern); err != nil {
		return o, fmt.Errorf("分卷正则无效：%v", err)
	}
	return o, nil
}

func splitOptions(opt Options, doc loadedDoc) chapters.Options {
	var headings []chapters.Heading
	if doc.book != nil {
		headings = doc.book.Headings
	}
	return chapters.Options{
		ChapterPattern:   opt.ChapterPattern,
		VolumePattern:    opt.VolumePattern,
		MergeBrokenLines: opt.MergeBrokenLines,
		DropEmptyLines:   opt.DropEmptyLines,
		FallbackRunes:    opt.FallbackRunes,
		Headings:         headings,
	}
}

type loadedDoc struct {
	text string
	enc  encoding.Result
	book *reflow.Book
}

func loadDocument(in fileIn) (loadedDoc, error) {
	name := in.Name
	if name == "" {
		name = in.Path
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".epub" || ext == ".mobi" || ext == ".azw3" {
		b, err := readPayload(in.Path, in.DataBase64, maxTextBytes, "文件")
		if err != nil {
			return loadedDoc{}, err
		}
		book, err := reflow.Open(name, b)
		if err != nil {
			return loadedDoc{}, err
		}
		return loadedDoc{text: book.Text, enc: encoding.Result{Name: book.Format, Confidence: 1}, book: &book}, nil
	}
	text, res, err := loadText(in)
	if err != nil {
		return loadedDoc{}, err
	}
	return loadedDoc{text: text, enc: res}, nil
}

func formatOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".epub":
		return "EPUB"
	case ".mobi":
		return "MOBI"
	case ".azw3":
		return "AZW3"
	default:
		return "TXT"
	}
}

func loadText(in fileIn) (string, encoding.Result, error) {
	b, err := readPayload(in.Path, in.DataBase64, maxTextBytes, "文件")
	if err != nil {
		return "", encoding.Result{}, err
	}
	return encoding.Decode(b, in.Encoding)
}

func loadCover(in fileIn) ([]byte, string, string, error) {
	if in.CoverPath == "" && in.CoverBase64 == "" {
		return nil, "", "", nil
	}
	b, err := readPayload(in.CoverPath, in.CoverBase64, maxImageBytes, "封面")
	if err != nil {
		return nil, "", "", err
	}
	name, mime, err := sniffImage(b)
	if err != nil {
		return nil, "", "", err
	}
	return b, name, mime, nil
}

func loadFont(in fileIn) ([]byte, string, string, string, error) {
	if in.FontPath == "" && in.FontBase64 == "" {
		return nil, "", "", "", nil
	}
	b, err := readPayload(in.FontPath, in.FontBase64, maxFontBytes, "字体")
	if err != nil {
		return nil, "", "", "", err
	}
	ext := strings.ToLower(filepath.Ext(in.FontName))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(in.FontPath))
	}
	switch ext {
	case ".ttf":
		return b, ".ttf", "font/ttf", "truetype", nil
	case ".otf":
		return b, ".otf", "font/otf", "opentype", nil
	default:
		return nil, "", "", "", errors.New("字体只支持 TTF 或 OTF")
	}
}

func readPayload(path, b64 string, limit int, label string) ([]byte, error) {
	if path != "" {
		st, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("无法读取%s", label)
		}
		if st.Size() > int64(limit) {
			return nil, fmt.Errorf("%s过大", label)
		}
		return os.ReadFile(path)
	}
	if b64 == "" {
		return nil, fmt.Errorf("没有%s", label)
	}
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s内容无法解码", label)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s过大", label)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s是空的", label)
	}
	return data, nil
}

func sniffImage(b []byte) (string, string, error) {
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "cover.jpg", "image/jpeg", nil
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(b) >= len(png) && string(b[:len(png)]) == string(png) {
		return "cover.png", "image/png", nil
	}
	return "", "", errors.New("封面只支持 JPG 或 PNG")
}

func toChapters(nodes []chapters.Node) []epub.Chapter {
	out := make([]epub.Chapter, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, epub.Chapter{
			Title:      n.Title,
			Paragraphs: n.Paragraphs,
			Children:   toChapters(n.Children),
		})
	}
	return out
}
