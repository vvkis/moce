// Package appdir places every runtime folder next to the executable.
// Nothing here consults the user profile, TEMP, or a drive letter.
package appdir

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Layout is the on-disk tree for one running executable.
type Layout struct {
	ExeDir  string
	Data    string
	Fonts   string
	Cache   string
	WebView string
	Scrape  string
	Covers  string
	HTTP    string
}

// Settings is data/settings.json. OutputDir is empty until the user picks a folder.
// AccountMode is "plain" (default, portable) or "dpapi" (this Windows user only).
type Settings struct {
	OutputDir   string `json:"outputDir"`
	AccountMode string `json:"accountMode,omitempty"`
}

// LayoutFrom builds paths from an executable path. The cache moves with the exe.
func LayoutFrom(exePath string) Layout {
	dir := filepath.Dir(exePath)
	cache := filepath.Join(dir, "cache")
	scrape := filepath.Join(cache, "刮搜")
	return Layout{
		ExeDir:  dir,
		Data:    filepath.Join(dir, "data"),
		Fonts:   filepath.Join(dir, "fonts"),
		Cache:   cache,
		WebView: filepath.Join(cache, "webview2"),
		Scrape:  scrape,
		Covers:  filepath.Join(scrape, "covers"),
		HTTP:    filepath.Join(scrape, "http"),
	}
}

// Resolve uses the running executable. Symlinks are resolved so the cache
// sits next to the real file.
func Resolve() (Layout, error) {
	exe, err := os.Executable()
	if err != nil {
		return Layout{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return LayoutFrom(exe), nil
}

// Ensure creates the runtime folders.
func (l Layout) Ensure() error {
	for _, dir := range []string{l.Data, l.Fonts, l.WebView, l.Covers, l.HTTP} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (l Layout) settingsPath() string {
	return filepath.Join(l.Data, "settings.json")
}

// LoadSettings reads data/settings.json, creating it on the first run.
func (l Layout) LoadSettings() (Settings, error) {
	if err := l.Ensure(); err != nil {
		return Settings{}, err
	}
	body, err := os.ReadFile(l.settingsPath())
	if os.IsNotExist(err) {
		fresh := Settings{}
		if err := l.SaveSettings(fresh); err != nil {
			return Settings{}, err
		}
		return fresh, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(body, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings writes data/settings.json next to the executable.
func (l Layout) SaveSettings(s Settings) error {
	if err := l.Ensure(); err != nil {
		return err
	}
	if s.AccountMode == "" {
		if prev, err := os.ReadFile(l.settingsPath()); err == nil {
			var old Settings
			if json.Unmarshal(prev, &old) == nil {
				s.AccountMode = old.AccountMode
			}
		}
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(l.settingsPath(), body, 0o644)
}
