package reflow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Open reads an EPUB, MOBI, or AZW3 into plain text plus the metadata worth keeping.
func Open(name string, data []byte) (Book, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".epub":
		return openEPUB(data)
	case ".mobi":
		return openMOBI(data, "MOBI")
	case ".azw3":
		return openMOBI(data, "AZW3")
	default:
		return Book{}, fmt.Errorf("无法读取这本书")
	}
}

// BookExt reports whether the name is a book this app can reflow.
func BookExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".epub", ".mobi", ".azw3":
		return true
	default:
		return false
	}
}
