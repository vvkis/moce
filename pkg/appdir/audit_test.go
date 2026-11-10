package appdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutStaysNextToExe(t *testing.T) {
	exe := filepath.Join("shelf", "墨册.exe")
	got := LayoutFrom(exe)
	base := filepath.Join("shelf", "cache")
	if got.ExeDir != "shelf" || got.Cache != base {
		t.Fatalf("%+v", got)
	}
	if got.WebView != filepath.Join(base, "webview2") {
		t.Fatalf("webview %s", got.WebView)
	}
	if got.Scrape != filepath.Join(base, "刮搜") {
		t.Fatalf("scrape %s", got.Scrape)
	}
	if got.Covers != filepath.Join(base, "刮搜", "covers") || got.HTTP != filepath.Join(base, "刮搜", "http") {
		t.Fatalf("%+v", got)
	}
	if got.Data != filepath.Join("shelf", "data") || got.Fonts != filepath.Join("shelf", "fonts") {
		t.Fatalf("data/fonts %+v", got)
	}
	for _, p := range []string{got.Data, got.Fonts, got.Cache, got.WebView, got.Scrape, got.Covers, got.HTTP} {
		if filepath.IsAbs(p) {
			t.Fatalf("layout invented an absolute path: %s", p)
		}
		if strings.Contains(strings.ToLower(p), "appdata") || strings.Contains(strings.ToLower(p), "temp") {
			t.Fatalf("layout uses an OS default: %s", p)
		}
	}
}

func TestSettingsStayNextToExe(t *testing.T) {
	root := t.TempDir()
	layout := LayoutFrom(filepath.Join(root, "墨册.exe"))
	got, err := layout.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.OutputDir != "" {
		t.Fatalf("%+v", got)
	}
	path := filepath.Join(root, "data", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := layout.SaveSettings(Settings{OutputDir: filepath.Join(root, "out")}); err != nil {
		t.Fatal(err)
	}
	again, err := layout.LoadSettings()
	if err != nil || again.OutputDir == "" {
		t.Fatalf("%+v %v", again, err)
	}
}

func TestRuntimeSourcesAvoidOSDefaults(t *testing.T) {
	root := moduleRoot(t)
	var hits []string
	skipDir := map[string]bool{
		".cache": true, ".git": true, "node_modules": true, "dist": true, "build": true, "frontend/dist": true,
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, ".cache"+string(filepath.Separator)) {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".ts", ".tsx", ".ps1", ".sh":
		default:
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, needle := range []string{
			"os.TempDir(",
			"os.UserHomeDir(",
			"os.UserCacheDir(",
			"os.UserConfigDir(",
			"os.CreateTemp(",
			"os.MkdirTemp(",
			`C:\`,
			"C:/",
			`D:\`,
			"D:/",
		} {
			if strings.Contains(text, needle) {
				hits = append(hits, rel+": "+needle)
			}
		}
		// Build scripts may point TEMP and APPDATA at the repo cache. They must not
		// name a drive letter, which is checked above. Runtime code must not mention them.
		if ext == ".go" || ext == ".ts" || ext == ".tsx" {
			for _, needle := range []string{"LOCALAPPDATA", "APPDATA", "%TEMP%", "os.TempDir"} {
				if strings.Contains(text, needle) {
					hits = append(hits, rel+": "+needle)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("runtime path leaks:\n%s", strings.Join(hits, "\n"))
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
