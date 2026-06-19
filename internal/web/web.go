package web

import (
	"bytes"
	_ "embed"
	"net/http"
	"time"
)

//go:embed console.html
var consoleHTML []byte

//go:embed console.css
var consoleCSS []byte

var consoleModTime = time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)

func Console() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/console.css" {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeContent(w, r, "console.css", consoleModTime, bytes.NewReader(consoleCSS))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "console.html", consoleModTime, bytes.NewReader(consoleHTML))
	})
}
