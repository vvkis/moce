package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func main() {
	pngBytes := drawIcon(512)
	root := filepath.Join("build")
	_ = os.MkdirAll(filepath.Join(root, "windows"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "appicon.png"), pngBytes, 0o644); err != nil {
		panic(err)
	}
	small := drawIcon(256)
	if err := writeICO(filepath.Join(root, "windows", "icon.ico"), small); err != nil {
		panic(err)
	}
}

func drawIcon(size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{0x9D, 0x2C, 0x2C, 0xFF}
	ink := color.RGBA{0xF6, 0xF0, 0xE6, 0xFF}
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	m := size / 16
	stroke(img, m*2, m*2, size-m*4, 2, ink)
	stroke(img, m*3, m*3, size-m*6, 1, ink)
	if face, err := loadFace(float64(size) * 0.46); err == nil {
		d := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face}
		w := d.MeasureString("册").Round()
		ascent := face.Metrics().Ascent.Round()
		d.Dot = fixed.P((size-w)/2, (size+ascent)/2-size/18)
		d.DrawString("册")
		if c, ok := face.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func stroke(img *image.RGBA, x, y, span, thick int, c color.RGBA) {
	for t := 0; t < thick; t++ {
		for i := 0; i < span; i++ {
			img.SetRGBA(x+t+i, y+t, c)
			img.SetRGBA(x+t+i, y+span-1-t, c)
			img.SetRGBA(x+t, y+t+i, c)
			img.SetRGBA(x+span-1-t, y+t+i, c)
		}
	}
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

func loadFace(size float64) (font.Face, error) {
	paths := []string{
		os.Getenv("MOCE_COVER_FONT"),
		"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		windowsFont("msyh.ttc"),
		windowsFont("simhei.ttf"),
	}
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
		f, err := opentype.Parse(b)
		if err != nil {
			col, cerr := opentype.ParseCollection(b)
			if cerr != nil {
				last = err
				continue
			}
			f, err = col.Font(0)
			if err != nil {
				last = err
				continue
			}
		}
		return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return nil, last
}

func writeICO(path string, pngBytes []byte) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	b.WriteByte(0)
	b.WriteByte(0)
	b.WriteByte(0)
	b.WriteByte(0)
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(32))
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pngBytes)))
	_ = binary.Write(&b, binary.LittleEndian, uint32(22))
	b.Write(pngBytes)
	return os.WriteFile(path, b.Bytes(), 0o644)
}
