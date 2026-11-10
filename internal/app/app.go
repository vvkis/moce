// Package app is the backend the desktop shell and the local preview server share.
package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"moce/internal/toolhost"
	"moce/internal/tools"
)

// App exposes the tool registry.
type App struct {
	tools *toolhost.Registry
}

// New builds the app with the registered tools.
func New() *App {
	return &App{tools: toolhost.New(tools.All()...)}
}

// ListTools returns sidebar metadata.
func (a *App) ListTools() []toolhost.Meta {
	return a.tools.List()
}

// CallTool runs one tool action. payload is a JSON object.
func (a *App) CallTool(id, action, payload string) (string, error) {
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	out, err := a.tools.Call(id, action, json.RawMessage(payload))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CollectTxtPaths expands files and directories into .txt paths.
func (a *App) CollectTxtPaths(paths []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) error {
		if seen[p] {
			return nil
		}
		seen[p] = true
		out = append(out, p)
		if len(out) > 500 {
			return errors.New("一次最多处理 500 个文件")
		}
		return nil
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, errors.New("无法读取：" + p)
		}
		if !info.IsDir() {
			if bookExt(p) {
				if err := add(p); err != nil {
					return nil, err
				}
			}
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() != "." && strings.HasPrefix(d.Name(), ".") && path != p {
					return filepath.SkipDir
				}
				return nil
			}
			if bookExt(path) {
				return add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("没有找到 txt、epub、mobi 或 azw3 文件")
	}
	return out, nil
}

// ReadFileBase64 reads a local file for an in-app preview. Conversion still uses the path.
func (a *App) ReadFileBase64(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("没有选择文件")
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return "", errors.New("无法读取封面")
	}
	const limit = 20 << 20
	if st.Size() > limit {
		return "", errors.New("封面过大")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("无法读取封面")
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func bookExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".epub", ".mobi", ".azw3":
		return true
	default:
		return false
	}
}

// OpenPath opens a file with its associated program.
func (a *App) OpenPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("没有可打开的文件")
	}
	if _, err := os.Stat(path); err != nil {
		return errors.New("找不到文件")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// RevealPath shows the file in the system file manager.
func (a *App) RevealPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("没有可定位的文件")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,"+path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	return cmd.Start()
}
