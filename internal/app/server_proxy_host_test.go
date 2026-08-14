package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestWithLazyCatProxyHost(t *testing.T) {
	const trustedDomain = "lazycat-mcp.example.test"
	tests := []struct {
		name       string
		domain     string
		host       string
		remoteAddr string
		localIP    string
		wantHost   string
	}{
		{
			name:       "trusted loopback proxy",
			domain:     trustedDomain,
			host:       trustedDomain,
			remoteAddr: "127.0.0.1:41000",
			localIP:    "127.0.0.1",
			wantHost:   "localhost",
		},
		{
			name:       "normalizes DNS case trailing dot and port",
			domain:     trustedDomain,
			host:       "LAZYCAT-MCP.EXAMPLE.TEST.:443",
			remoteAddr: "127.0.0.1:41000",
			localIP:    "127.0.0.1",
			wantHost:   "localhost",
		},
		{
			name:       "rejects foreign host",
			domain:     trustedDomain,
			host:       "attacker.example.test",
			remoteAddr: "127.0.0.1:41000",
			localIP:    "127.0.0.1",
			wantHost:   "attacker.example.test",
		},
		{
			name:       "fails closed without platform domain",
			domain:     "",
			host:       trustedDomain,
			remoteAddr: "127.0.0.1:41000",
			localIP:    "127.0.0.1",
			wantHost:   trustedDomain,
		},
		{
			name:       "rejects non loopback peer",
			domain:     trustedDomain,
			host:       trustedDomain,
			remoteAddr: "192.0.2.10:41000",
			localIP:    "127.0.0.1",
			wantHost:   trustedDomain,
		},
		{
			name:       "rejects non loopback listener",
			domain:     trustedDomain,
			host:       trustedDomain,
			remoteAddr: "127.0.0.1:41000",
			localIP:    "192.0.2.20",
			wantHost:   trustedDomain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LAZYCAT_APP_DOMAIN", tt.domain)
			seenHost := ""
			handler := withLazyCatProxyHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenHost = r.Host
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodPost, "http://example.test/mcp", nil)
			req.Host = tt.host
			req.RemoteAddr = tt.remoteAddr
			ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(tt.localIP), Port: 3000})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d", rec.Code)
			}
			if seenHost != tt.wantHost {
				t.Fatalf("handler host = %q, want %q", seenHost, tt.wantHost)
			}
			if req.Host != tt.host {
				t.Fatalf("original request host mutated to %q", req.Host)
			}
		})
	}
}

func TestLazyCatProxyHostPreservesSSEProtection(t *testing.T) {
	const trustedDomain = "lazycat-mcp.example.test"
	t.Setenv("LAZYCAT_APP_DOMAIN", trustedDomain)

	for _, tt := range []struct {
		name       string
		host       string
		wantStatus int
	}{
		{name: "platform host reaches SSE handler", host: trustedDomain, wantStatus: http.StatusBadRequest},
		{name: "foreign host", host: "attacker.example.test", wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcpserver.NewMCPServer("test", "1.0")
			handler := withLazyCatProxyHost(mcpserver.NewSSEServer(server))
			req := httptest.NewRequest(http.MethodPost, "http://example.test/message", strings.NewReader(`{}`))
			req.Host = tt.host
			req.RemoteAddr = "127.0.0.1:41000"
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3000})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestLazyCatProxyHostPreservesMCPGoProtection(t *testing.T) {
	const trustedDomain = "lazycat-mcp.example.test"
	t.Setenv("LAZYCAT_APP_DOMAIN", trustedDomain)

	for _, tt := range []struct {
		name       string
		host       string
		wantStatus int
	}{
		{name: "platform host", host: trustedDomain, wantStatus: http.StatusOK},
		{name: "foreign host", host: "attacker.example.test", wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcpserver.NewMCPServer("test", "1.0")
			handler := withLazyCatProxyHost(mcpserver.NewStreamableHTTPServer(server))
			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			req := httptest.NewRequest(http.MethodPost, "http://example.test/mcp", strings.NewReader(body))
			req.Host = tt.host
			req.RemoteAddr = "127.0.0.1:41000"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3000})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusForbidden && !strings.Contains(rec.Body.String(), "invalid Host header") {
				t.Fatalf("expected mcp-go Host rejection, body=%s", rec.Body.String())
			}
		})
	}
}
