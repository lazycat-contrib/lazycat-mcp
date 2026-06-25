package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed dist/* dist/assets/*
var uiFS embed.FS

var noModTime time.Time

func Console() http.Handler {
	dist, err := fs.Sub(uiFS, "dist")
	if err != nil {
		panic(err)
	}
	indexHTML, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", noModTime, bytes.NewReader(indexHTML))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		cleanPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "." || cleanPath == "" || cleanPath == "index.html" {
			serveIndex(w, r)
			return
		}
		if strings.HasPrefix(cleanPath, "assets/") || strings.HasSuffix(cleanPath, ".css") || strings.HasSuffix(cleanPath, ".js") || strings.HasSuffix(cleanPath, ".ico") || strings.HasSuffix(cleanPath, ".png") || strings.HasSuffix(cleanPath, ".svg") {
			r.URL.Path = "/" + cleanPath
			files.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r)
	})
}
