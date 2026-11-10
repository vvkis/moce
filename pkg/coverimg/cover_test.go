package coverimg

import (
	"bytes"
	"testing"
)

func TestRenderPNG(t *testing.T) {
	png, err := Render("河灯", "沈小满", nil)
	if err != nil {
		t.Skipf("no cjk font: %v", err)
	}
	if !bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("not a png")
	}
	if len(png) < 1000 {
		t.Fatalf("png too small: %d", len(png))
	}
}
