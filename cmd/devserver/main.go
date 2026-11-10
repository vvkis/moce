// Dev server for checking the interface in a browser. The Windows exe does not include this.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"moce/internal/app"
	"moce/internal/tools/txt2epub"
	"moce/pkg/accounts"
	"moce/pkg/appdir"
	"moce/pkg/coverimg"
	"moce/pkg/scrape"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:47321", "listen address")
	dist := flag.String("dist", "frontend/dist", "built frontend directory")
	flag.Parse()

	if layout, err := appdir.Resolve(); err == nil && layout.Ensure() == nil {
		scrape.Use(layout)
		scrape.Sweep()
		coverimg.SetFontDir(layout.Fonts)
		txt2epub.SetDataDir(layout.Data)
		accounts.Use(layout.Data, func() string { return "plain" })
	}
	application := app.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, application.ListTools())
	})
	mux.HandleFunc("/api/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST"})
			return
		}
		var req struct {
			ID      string          `json:"id"`
			Action  string          `json:"action"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不对"})
			return
		}
		out, err := application.CallTool(req.ID, req.Action, string(req.Payload))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"result":` + out + `}`))
	})

	root := *dist
	if !filepath.IsAbs(root) {
		if wd, err := os.Getwd(); err == nil {
			root = filepath.Join(wd, root)
		}
	}
	fs := http.FileServer(http.Dir(root))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})

	log.Printf("墨册预览 http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
