package app

import (
    "net/http"
    "net/http/httptest"
    "os"
    "regexp"
    "testing"

    "lazycat-mcp/internal/web"
)

func TestServeHTTPServesFrameworkAssets(t *testing.T) {
    app := &App{ui: web.Console()}

    req := httptest.NewRequest(http.MethodGet, firstAssetFromIndex(t), nil)
    rec := httptest.NewRecorder()

    app.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
    }
    if got := rec.Header().Get("Cache-Control"); got != "no-store" {
        t.Fatalf("cache-control = %q", got)
    }
    if body := rec.Body.String(); body == "" {
        t.Fatal("expected asset body")
    }
}


func TestServeHTTPServesRootHTMLWithoutRedirectLoop(t *testing.T) {
    app := &App{ui: web.Console()}

    req := httptest.NewRequest(http.MethodGet, "/", nil)
    rec := httptest.NewRecorder()

    app.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
    }
    if got := rec.Header().Get("Location"); got != "" {
        t.Fatalf("unexpected redirect location = %q", got)
    }
    if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
        t.Fatalf("content-type = %q", got)
    }
    if body := rec.Body.String(); body == "" || body[:9] != "<!doctype" && body[:5] != "<html" {
        t.Fatalf("expected html body, got %q", body[:min(80, len(body))])
    }
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}


func firstAssetFromIndex(t *testing.T) string {
    t.Helper()
    html, err := os.ReadFile("../../internal/web/dist/index.html")
    if err != nil {
        t.Fatalf("read index.html: %v", err)
    }
    re := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+\.(?:js|css))"`)
    m := re.FindSubmatch(html)
    if len(m) < 2 {
        t.Fatalf("no asset path found in index.html")
    }
    return string(m[1])
}
