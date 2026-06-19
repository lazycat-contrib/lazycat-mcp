package proxy

import (
	"net/http"
	"testing"
)

func TestHeadersForUpstreamStripsCallerAuth(t *testing.T) {
	in := http.Header{}
	in.Set("Authorization", "Bearer caller-token")
	in.Set("X-MCP-Token", "caller-token")
	in.Set("Content-Type", "application/json")
	in.Set("Mcp-Session-Id", "session-1")

	out := HeadersForUpstream(in, "lazycat-ticket")
	if got := out.Get("Authorization"); got != "" {
		t.Fatalf("Authorization forwarded: %q", got)
	}
	if got := out.Get("X-MCP-Token"); got != "" {
		t.Fatalf("X-MCP-Token forwarded: %q", got)
	}
	if got := out.Get("X-HC-USER-TICKET"); got != "lazycat-ticket" {
		t.Fatalf("ticket = %q", got)
	}
	if got := out.Get("Mcp-Session-Id"); got != "session-1" {
		t.Fatalf("session header = %q", got)
	}
}

func TestTargetURLUsesLazyCatAppHost(t *testing.T) {
	got, err := targetURL("cloud.lazycat.app.photo", "/api/mcp?mode=default", "/tools/list", "cursor=1")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://app.cloud.lazycat.app.photo.lzcx/api/mcp/tools/list?mode=default&cursor=1"
	if got != want {
		t.Fatalf("target url = %q, want %q", got, want)
	}
}

func TestTargetURLRejectsAbsoluteEndpoint(t *testing.T) {
	if _, err := targetURL("cloud.lazycat.app.photo", "http://127.0.0.1:8080/mcp", "", ""); err == nil {
		t.Fatal("expected absolute endpoint to be rejected")
	}
}

func TestCustomTargetURLUsesConfiguredServiceURL(t *testing.T) {
	baseURL := "https://example.com/base"
	got, err := customTargetURL(&baseURL, "/api/mcp?mode=default", "/tools/list", "cursor=1")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/base/api/mcp/tools/list?mode=default&cursor=1"
	if got != want {
		t.Fatalf("target url = %q, want %q", got, want)
	}
}

func TestHeadersForCustomUpstreamUsesConfiguredHeadersWithoutLazyCatTicket(t *testing.T) {
	in := http.Header{}
	in.Set("Authorization", "Bearer caller-token")
	in.Set("X-MCP-Token", "caller-token")
	in.Set("Mcp-Session-Id", "session-1")
	in.Set("Accept", "application/json")

	out, err := HeadersForCustomUpstream(in, `[{"name":"Authorization","value":"Bearer upstream"},{"name":"X-Api-Key","value":"secret"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Get("Authorization"); got != "Bearer upstream" {
		t.Fatalf("authorization = %q", got)
	}
	if got := out.Get("X-Api-Key"); got != "secret" {
		t.Fatalf("api key = %q", got)
	}
	if got := out.Get("X-HC-USER-TICKET"); got != "" {
		t.Fatalf("lazycat ticket forwarded: %q", got)
	}
	if got := out.Get("X-MCP-Token"); got != "" {
		t.Fatalf("mcp token forwarded: %q", got)
	}
	if got := out.Get("Mcp-Session-Id"); got != "session-1" {
		t.Fatalf("session header = %q", got)
	}
}
