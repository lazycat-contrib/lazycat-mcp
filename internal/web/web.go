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

var noModTime time.Time

func Console() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/console.css" {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeContent(w, r, "console.css", noModTime, bytes.NewReader(consoleCSS))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "console.html", noModTime, bytes.NewReader(consoleHTML))
	})
}
