# MCP Aggregate Gateway and Call Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single `/mcp` aggregate gateway for local and upstream tools, plus persistent, queryable, clearable MCP call logs with automatic retention cleanup.

**Architecture:** Add a dynamic upstream tool catalog that registers enabled upstream tools onto the local `mcp-go` server with namespaced names. Add an Ent `MCPCallLog` model and an app service for recording, querying, clearing, and cleanup. Wire local and aggregate tool calls through `mcp-go` middleware and upstream provider proxy calls through an HTTP response recorder. Expose the feature through `/api/mcp-logs` and the existing static console.

**Tech Stack:** Go 1.25.1, Ent v0.14.5, SQLite, `github.com/mark3labs/mcp-go` v0.44.0, plain HTML/CSS/JS console.

## Global Constraints

- Store metadata only; never persist request body, response body, Authorization headers, `X-MCP-Token`, custom provider headers, or full tool arguments.
- Aggregate upstream tools must use `<provider_slug>__<upstream_tool_name>` names after sanitizing non-word characters to `_`.
- First aggregate implementation supports `streamable_http` providers; `sse` providers stay available through `/mcp/apps/<slug>`.
- Default retention is 30 days, configurable through `LAZYCAT_MCP_LOG_RETENTION_DAYS`; `0` disables automatic age cleanup.
- List APIs must cap `limit` at 500 rows.
- Verification command is `go test ./...` after `go generate ./ent` and `gofmt`.

---

### Task 1: Persistent Log Model

**Files:**
- Create: `ent/schema/mcpcalllog.go`
- Generated: `ent/*`
- Modify: `internal/app/config.go`

**Interfaces:**
- Produces Ent type `ent.MCPCallLog`.
- Produces config field `MCPLogRetentionDays int`.

- [ ] Add Ent schema with metadata fields and indexes from the spec.
- [ ] Add retention config parsing with default `30`.
- [ ] Run `go generate ./ent`.
- [ ] Run `gofmt` on changed Go files.

### Task 2: Log Service

**Files:**
- Create: `internal/app/call_logs.go`
- Test: `internal/app/call_logs_test.go`

**Interfaces:**
- Produces `MCPCallLogService.Record(ctx, input)`.
- Produces `MCPCallLogService.List(ctx, filter)`.
- Produces `MCPCallLogService.Clear(ctx)`.
- Produces `MCPCallLogService.Cleanup(ctx, now)`.

- [ ] Add tests for record/list and retention cleanup.
- [ ] Implement DTO, input normalization, limit capping, and delete operations.
- [ ] Ensure error summaries are truncated and empty fields stay optional.

### Task 3: Recording Hooks

**Files:**
- Modify: `internal/app/server.go`
- Modify: `internal/app/mcp.go`
- Test: `internal/app/mcp_test.go`

**Interfaces:**
- Consumes `MCPCallLogService.Record`.
- Produces local tool-call records and upstream HTTP records.

- [ ] Add local tool middleware around `mcp-go` handlers.
- [ ] Wrap upstream provider proxy with an HTTP status recorder.
- [ ] Start a daily cleanup loop and stop it in `App.Close`.
- [ ] Test local tool recording and upstream provider status recording.

### Task 4: Aggregate Upstream Tool Catalog

**Files:**
- Create: `internal/app/upstream_tools.go`
- Modify: `internal/app/server.go`
- Modify: `internal/app/mcp.go`
- Modify: `internal/app/services.go`
- Test: `internal/app/upstream_tools_test.go`

**Interfaces:**
- Produces `App.refreshUpstreamTools(ctx context.Context) error`.
- Produces `App.callUpstreamTool(ctx context.Context, ref upstreamToolRef, request mcp.CallToolRequest)`.
- Produces dynamic `mcpserver.ServerTool` registrations on the local MCP server.

- [ ] Add provider detail listing for enabled providers.
- [ ] Fetch upstream `tools/list` through `mcp-go` streamable HTTP clients.
- [ ] Register namespaced aggregate tools on the local MCP server.
- [ ] Remove disabled/deleted provider tools.
- [ ] Route aggregate calls to the original upstream tool.
- [ ] Test registration, invocation, and removal.

### Task 5: API and Console

**Files:**
- Modify: `internal/app/handlers.go`
- Modify: `internal/web/console.html`
- Modify: `internal/web/console.css`

**Interfaces:**
- Produces `GET /api/mcp-logs`.
- Produces `DELETE /api/mcp-logs`.
- Produces `POST /api/mcp-logs/cleanup`.

- [ ] Add handlers and route parsing.
- [ ] Add console state, loading, rendering, reload, cleanup, and clear-all actions.
- [ ] Keep the panel compact and metadata-only.

### Task 6: Verification

**Files:**
- All changed files

- [ ] Run `go generate ./ent`.
- [ ] Run `gofmt` on Go files.
- [ ] Run `go test ./...`.
- [ ] Inspect `git diff --stat` and `git status --short`.
