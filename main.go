package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"moce/internal/app"
	"moce/internal/toolhost"
	"moce/internal/tools/txt2epub"
	"moce/pkg/accounts"
	"moce/pkg/appdir"
	"moce/pkg/coverimg"
	"moce/pkg/scrape"
)

var portable appdir.Layout

func main() {
	layout, err := appdir.Resolve()
	if err != nil {
		println("无法定位程序目录:", err.Error())
		return
	}
	if err := layout.Ensure(); err != nil {
		println("无法创建缓存目录:", err.Error())
		return
	}
	scrape.Use(layout)
	scrape.Sweep()
	coverimg.SetFontDir(layout.Fonts)
	txt2epub.SetDataDir(layout.Data)
	accounts.Use(layout.Data, func() string {
		s, err := layout.LoadSettings()
		if err != nil || s.AccountMode != "dpapi" {
			return "plain"
		}
		return "dpapi"
	})
	portable = layout
	if _, err := layout.LoadSettings(); err != nil {
		println("无法写入设置:", err.Error())
		return
	}
	application := NewApp()
	err = wails.Run(&options.App{
		Title:            "墨册",
		Width:            1180,
		Height:           820,
		MinWidth:         920,
		MinHeight:        640,
		BackgroundColour: options.NewRGB(246, 240, 230),
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.startup,
		OnShutdown: func(ctx context.Context) { scrape.Sweep() },
		Windows: &windows.Options{
			WebviewUserDataPath: layout.WebView,
		},
		Bind: []interface{}{
			application,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// App is the Wails binding. Methods here are callable from the page.
type App struct {
	ctx   context.Context
	inner *app.App
}

// NewApp constructs the shell.
func NewApp() *App {
	return &App{inner: app.New()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// LoadSettings reads data/settings.json beside the exe, creating it if needed.
func (a *App) LoadSettings() (appdir.Settings, error) {
	return portable.LoadSettings()
}

// SaveSettings writes data/settings.json beside the exe.
func (a *App) SaveSettings(s appdir.Settings) error {
	return portable.SaveSettings(s)
}

// OpenPath opens a file with the associated program. It does not copy the file.
func (a *App) OpenPath(path string) error {
	return a.inner.OpenPath(path)
}

// ListTools returns the registered tools.
func (a *App) ListTools() []toolhost.Meta {
	return a.inner.ListTools()
}

// CallTool runs a tool action. payload is a JSON object string.
func (a *App) CallTool(id, action, payload string) (string, error) {
	return a.inner.CallTool(id, action, payload)
}

// CollectTxtPaths expands dropped files and folders.
func (a *App) CollectTxtPaths(paths []string) ([]string, error) {
	return a.inner.CollectTxtPaths(paths)
}

// RevealPath shows a file in Explorer.
func (a *App) RevealPath(path string) error {
	return a.inner.RevealPath(path)
}

// PickTxtFiles opens a multi-file dialog.
func (a *App) PickTxtFiles() ([]string, error) {
	return a.pickFiles("选择书籍", []wruntime.FileFilter{{DisplayName: "TXT / EPUB / MOBI / AZW3", Pattern: "*.txt;*.epub;*.mobi;*.azw3"}})
}

// ReadFileBase64 loads a picked image so the page can preview it.
func (a *App) ReadFileBase64(path string) (string, error) {
	return a.inner.ReadFileBase64(path)
}

// PickCover opens an image dialog.
func (a *App) PickCover() (string, error) {
	return a.pickFile("选择封面", []wruntime.FileFilter{{DisplayName: "图片 (*.jpg, *.png)", Pattern: "*.jpg;*.jpeg;*.png"}})
}

// PickFont opens a font dialog.
func (a *App) PickFont() (string, error) {
	return a.pickFile("选择字体", []wruntime.FileFilter{{DisplayName: "字体 (*.ttf, *.otf)", Pattern: "*.ttf;*.otf"}})
}

// PickDirectory opens a folder dialog.
func (a *App) PickDirectory() (string, error) {
	if a.ctx == nil {
		return "", errors.New("窗口还没准备好")
	}
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "选择输出目录"})
}

func (a *App) pickFiles(title string, filters []wruntime.FileFilter) ([]string, error) {
	if a.ctx == nil {
		return nil, errors.New("窗口还没准备好")
	}
	return wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{Title: title, Filters: filters})
}

func (a *App) pickFile(title string, filters []wruntime.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", errors.New("窗口还没准备好")
	}
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{Title: title, Filters: filters})
}
