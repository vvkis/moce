package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileBase64(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jpg")
	payload := []byte{0xff, 0xd8, 0xff, 0xd9}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := New().ReadFileBase64(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/9j/2Q==" {
		t.Fatalf("got %s", got)
	}
	if _, err := New().ReadFileBase64(filepath.Join(dir, "missing.jpg")); err == nil {
		t.Fatal("expected missing file error")
	}
}
