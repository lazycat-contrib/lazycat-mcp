# MCP Aggregate Gateway and Call Log Design

## Goal

Make `/mcp` the single recommended MCP endpoint for AI agents by exposing local tools plus enabled upstream provider tools in one `tools/list`, and add persistent call logs so operators can inspect recent MCP usage, clear logs manually, and rely on automatic retention cleanup.

## Architecture

The local MCP server owns a dynamic upstream tool catalog. Local tools keep their existing names. Enabled upstream provider tools are fetched through `mcp-go` clients and registered on the local server using namespaced names. MCP call logs are stored as metadata-only audit events in SQLite through Ent. Local and aggregate tool invocations are recorded through `mcp-go` tool middleware. Published upstream provider proxy calls are recorded through an app-level response recorder around the provider proxy.

## Aggregate Tool Catalog

Enabled upstream provider tools are exposed on `/mcp` with this name format:

```text
<provider_slug>__<upstream_tool_name>
```

Tool name parts are sanitized to letters, digits, and underscores. For example, provider `context7` tool `resolve-library` becomes `context7__resolve_library`.

The catalog refreshes once at startup and after provider create, update, delete, enable, or disable operations. Refresh failures for one provider must not remove local tools or working tools from other providers. Disabled or deleted provider tools are removed from the local server.

The first implementation aggregates `streamable_http` providers. `sse` providers remain available through `/mcp/apps/<slug>` but are not registered into the aggregate tool catalog.

## Upstream Invocation

When an aggregate tool is called, the server parses the registered tool reference, loads the current provider by slug, creates an upstream MCP client, initializes it, calls the original upstream tool name with the original arguments, returns the upstream result, and closes the client. Secrets and caller authorization headers are not forwarded. LazyCat providers use the captured LazyCat ticket; custom providers use only configured provider headers.

## Data Model

Create `MCPCallLog` with these fields:

- `source`: `local` or `upstream`.
- `transport`: `streamable_http`, `sse`, or `http`.
- `method`: MCP tool name or HTTP method. Aggregate tool calls store the namespaced tool name here.
- `target`: local tool name, upstream original tool name, or upstream provider public path.
- `provider_slug`: optional upstream provider slug.
- `token_prefix`: optional token prefix for valid authenticated calls.
- `session_id`: optional MCP session id.
- `request_id`: optional caller request id.
- `status`: `success` or `error`.
- `status_code`: optional HTTP status code.
- `duration_ms`: non-negative integer duration.
- `error`: optional truncated error summary.
- `created_at`: immutable event time.

Indexes must support newest-first listing and retention cleanup: `created_at`, `source`, `status`, and `provider_slug`.

## Retention

Default retention is 30 days. Operators can override it with `LAZYCAT_MCP_LOG_RETENTION_DAYS`. A value of `0` disables automatic age cleanup. The app runs cleanup once during startup and then once every 24 hours while running.

## API

- `GET /api/mcp-logs?limit=100&source=&status=&provider_slug=` lists newest logs, capped at 500 rows.
- `DELETE /api/mcp-logs` deletes all call logs.
- `POST /api/mcp-logs/cleanup` deletes logs older than the configured retention window and returns a deleted count.

## Console

Add a compact "MCP call logs" panel with reload, cleanup, clear all, status badges, duration, target, provider slug, and timestamp. The UI must also make clear that `/mcp` is the recommended aggregate endpoint. The UI must not expose request bodies, headers, tokens, provider secrets, or full tool arguments.

## Testing

Add focused tests for the log service, cleanup behavior, local tool middleware recording, aggregate upstream tool registration/calling/removal, and upstream HTTP logging behavior. Run `go generate ./ent`, `gofmt`, and `go test ./...`.
