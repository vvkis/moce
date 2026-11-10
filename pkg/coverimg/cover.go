// Package coverimg draws a simple title page as a PNG, using a CJK font
// already on the machine. The font is not bundled.
package coverimg

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	coverW = 900
	coverH = 1200
)

var (
	sysOnce sync.Once
	sysFont *opentype.Font
	sysErr  error
)

// Render draws title and author. preferred is an optional TTF/OTF/TTC to try
// before the system CJK font.
func Render(title, author string, preferred []byte) ([]byte, error) {
	if title == "" {
		title = "未命名"
	}
	if len(preferred) > 0 {
		if f, err := parseFont(preferred); err == nil {
			if png, err := renderWith(title, author, f); err == nil {
				return png, nil
			}
		}
	}
	f, err := systemFont()
	if err != nil {
		return nil, err
	}
	return renderWith(title, author, f)
}

func systemFont() (*opentype.Font, error) {
	sysOnce.Do(func() {
		b, err := loadSystemFont()
		if err != nil {
			sysErr = err
			return
		}
		sysFont, sysErr = parseFont(b)
	})
	return sysFont, sysErr
}

var extraFontDir string

// SetFontDir points cover rendering at the portable fonts folder beside the exe.
func SetFontDir(dir string) {
	extraFontDir = dir
}

func windowsFont(name string) string {
	root := os.Getenv("WINDIR")
	if root == "" {
		root = os.Getenv("SystemRoot")
	}
	if root == "" {
		return ""
	}
	return filepath.Join(root, "Fonts", name)
}

func userFonts(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".ttf", ".otf", ".ttc":
			out = append(out, filepath.Join(dir, entry.Name()))
		}
	}
	return out
}

func loadSystemFont() ([]byte, error) {
	paths := append(userFonts(extraFontDir),
		os.Getenv("MOCE_COVER_FONT"),
		windowsFont("msyh.ttc"),
		windowsFont("msyh.ttf"),
		windowsFont("simhei.ttf"),
		windowsFont("simsun.ttc"),
		windowsFont("simkai.ttf"),
		windowsFont("Deng.ttf"),
		"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSerifCJK-Regular.ttc",
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/Library/Fonts/Arial Unicode.ttf",
	)
	var last error
	for _, p := range paths {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			last = err
			continue
		}
		if len(b) > 0 {
			return b, nil
		}
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return nil, last
}

func parseFont(b []byte) (*opentype.Font, error) {
	if f, err := opentype.Parse(b); err == nil {
		return f, nil
	}
	col, err := opentype.ParseCollection(b)
	if err != nil {
		return nil, err
	}
	return col.Font(0)
}

func renderWith(title, author string, f *opentype.Font) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, coverW, coverH))
	bg := color.RGBA{0xF4, 0xEC, 0xDF, 0xFF}
	ink := color.RGBA{0x1C, 0x16, 0x12, 0xFF}
	seal := color.RGBA{0x9D, 0x2C, 0x2C, 0xFF}
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	strokeRect(img, 46, 46, 808, 1108, 3, seal)
	strokeRect(img, 62, 62, 776, 1076, 1, seal)

	mark, err := newFace(f, 28)
	if err != nil {
		return nil, err
	}
	defer closeFace(mark)
	drawCentered(img, mark, "墨册", 250, seal)

	titleFace, err := newFace(f, 64)
	if err != nil {
		return nil, err
	}
	defer closeFace(titleFace)
	lines := wrap(titleFace, title, 680)
	if len(lines) > 6 {
		titleFace2, err := newFace(f, 46)
		if err == nil {
			closeFace(titleFace)
			titleFace = titleFace2
			lines = wrap(titleFace, title, 700)
		}
	}
	lh := lineHeight(titleFace, 78)
	block := lh * len(lines)
	top := 560 - block/2
	for i, line := range lines {
		drawCentered(img, titleFace, line, top+i*lh, ink)
	}
	if author != "" {
		af, err := newFace(f, 32)
		if err != nil {
			return nil, err
		}
		defer closeFace(af)
		drawCentered(img, af, author, top+block+70, color.RGBA{0x5C, 0x46, 0x36, 0xFF})
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newFace(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
}

func closeFace(face font.Face) {
	if c, ok := face.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}

func lineHeight(face font.Face, min int) int {
	h := face.Metrics().Height.Ceil()
	if h < min {
		return min
	}
	return h
}

func wrap(face font.Face, text string, maxWidth int) []string {
	rs := []rune(text)
	if len(rs) == 0 {
		return []string{""}
	}
	var lines []string
	var cur []rune
	for _, r := range rs {
		trial := append(append([]rune{}, cur...), r)
		if measure(face, string(trial)) > maxWidth && len(cur) > 0 {
			lines = append(lines, string(cur))
			cur = []rune{r}
			continue
		}
		cur = trial
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return lines
}

func measure(face font.Face, s string) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(s).Ceil()
}

func drawCentered(img *image.RGBA, face font.Face, text string, baseline int, col color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
	}
	w := d.MeasureString(text).Ceil()
	x := (coverW - w) / 2
	if x < 40 {
		x = 40
	}
	d.Dot = fixed.P(x, baseline)
	d.DrawString(text)
}

func strokeRect(img *image.RGBA, x, y, w, h, thickness int, c color.RGBA) {
	for t := 0; t < thickness; t++ {
		top, bot := y+t, y+h-1-t
		left, right := x+t, x+w-1-t
		for xx := left; xx <= right; xx++ {
			img.SetRGBA(xx, top, c)
			img.SetRGBA(xx, bot, c)
		}
		for yy := top; yy <= bot; yy++ {
			img.SetRGBA(left, yy, c)
			img.SetRGBA(right, yy, c)
		}
	}
}
