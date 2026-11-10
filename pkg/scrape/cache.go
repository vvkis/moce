package scrape

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"moce/pkg/appdir"
)

const (
	cacheTTL   = 6 * time.Hour
	cacheCap   = 64 << 20
	partSuffix = ".part"
)

var (
	dirs appdir.Layout
	mu   sync.Mutex
)

// Use points downloads and HTTP bodies at the exe-relative layout.
func Use(layout appdir.Layout) {
	mu.Lock()
	dirs = layout
	mu.Unlock()
}

func cacheDirs() appdir.Layout {
	mu.Lock()
	defer mu.Unlock()
	return dirs
}

func readHTTPCache(rawURL string) ([]byte, bool) {
	path := httpPath(rawURL)
	if path == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > cacheTTL {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return nil, false
	}
	return body, true
}

func writeHTTPCache(rawURL string, body []byte) {
	path := httpPath(rawURL)
	if path == "" || len(body) == 0 {
		return
	}
	_ = writeAtomic(path, body)
}

func httpPath(rawURL string) string {
	dir := cacheDirs().HTTP
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(rawURL))
	return filepath.Join(dir, hex.EncodeToString(sum[:]))
}

// storeCover writes a downloaded image and returns its path. It is unkept until KeepCover.
func storeCover(body []byte, mime string) (string, error) {
	dir := cacheDirs().Covers
	if dir == "" {
		return "", nil
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
	sum := sha256.Sum256(body)
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+ext)
	if err := writeAtomic(path, body); err != nil {
		return "", err
	}
	return path, nil
}

// KeepCover marks a downloaded cover as chosen so exit cleanup leaves it in place.
func KeepCover(path string) error {
	dir := cacheDirs().Covers
	if dir == "" || path == "" {
		return nil
	}
	if filepath.Dir(path) != dir {
		return nil
	}
	name := filepath.Base(path)
	mu.Lock()
	defer mu.Unlock()
	kept := readKept(dir)
	kept[name] = time.Now().Unix()
	return writeKept(dir, kept)
}

// Sweep drops covers that were only previews, expired HTTP bodies, and oldest files past the size cap.
func Sweep() {
	layout := cacheDirs()
	if layout.Scrape == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	kept := readKept(layout.Covers)
	entries, _ := os.ReadDir(layout.Covers)
	for _, entry := range entries {
		name := entry.Name()
		if name == "kept.json" || strings.HasSuffix(name, partSuffix) {
			if strings.HasSuffix(name, partSuffix) {
				_ = os.Remove(filepath.Join(layout.Covers, name))
			}
			continue
		}
		if _, ok := kept[name]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(layout.Covers, name))
	}
	httpEntries, _ := os.ReadDir(layout.HTTP)
	for _, entry := range httpEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > cacheTTL || strings.HasSuffix(entry.Name(), partSuffix) {
			_ = os.Remove(filepath.Join(layout.HTTP, entry.Name()))
		}
	}
	trimSize(layout.Scrape, layout.Covers)
}

type fileAge struct {
	path string
	size int64
	mod  time.Time
	kept bool
}

func trimSize(root, covers string) {
	var files []fileAge
	var total int64
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "kept.json" {
			return nil
		}
		total += info.Size()
		kept := false
		if filepath.Dir(path) == covers {
			_, kept = readKept(covers)[info.Name()]
		}
		files = append(files, fileAge{path: path, size: info.Size(), mod: info.ModTime(), kept: kept})
		return nil
	})
	if total <= cacheCap {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].kept != files[j].kept {
			return !files[i].kept
		}
		return files[i].mod.Before(files[j].mod)
	})
	kept := readKept(covers)
	for _, file := range files {
		if total <= cacheCap {
			break
		}
		if err := os.Remove(file.path); err == nil {
			total -= file.size
			delete(kept, filepath.Base(file.path))
		}
	}
	_ = writeKept(covers, kept)
}

func readKept(dir string) map[string]int64 {
	out := map[string]int64{}
	body, err := os.ReadFile(filepath.Join(dir, "kept.json"))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(body, &out)
	return out
}

func writeKept(dir string, kept map[string]int64) error {
	if dir == "" {
		return nil
	}
	body, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "kept.json"), body)
}

func writeAtomic(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	part := path + partSuffix
	if err := os.WriteFile(part, body, 0o644); err != nil {
		return err
	}
	return os.Rename(part, path)
}
