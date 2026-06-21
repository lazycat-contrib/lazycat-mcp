# Console App Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the embedded LazyCat MCP console as a task-oriented operator app shell.

**Architecture:** Keep the current static console served by Go embed. Replace the all-in-one grid with hash-based workspaces in `console.html`, reuse the current API calls and render functions, and replace the visual system in `console.css` with a dark operator shell.

**Tech Stack:** Go embed, plain HTML, plain CSS, vanilla JavaScript, existing REST API. A no-build static framework is acceptable only if it is vendored locally and embedded into the Go binary.

## Global Constraints

- No frontend build chain.
- New JavaScript dependencies are allowed only when vendored locally and served through Go embed.
- Preserve all existing backend API routes.
- Preserve bilingual Chinese and English UI.
- Keep logs metadata-only.
- Verify with `go test ./...` and rendered browser checks at desktop and mobile widths.

---

### Task 1: Document the approved design

**Files:**
- Create: `docs/superpowers/specs/2026-06-21-console-app-shell-design.md`
- Create: `docs/superpowers/plans/2026-06-21-console-app-shell.md`

**Interfaces:**
- Produces a source-of-truth design and execution checklist for the console rebuild.

- [x] Write the design spec.
- [x] Write this implementation plan.

### Task 2: Rebuild the console structure

**Files:**
- Modify: `internal/web/console.html`

**Interfaces:**
- Consumes existing element IDs used by JavaScript render functions.
- Produces four workspaces: `overview`, `upstreams`, `access`, and `observability`.

- [x] Add skip link, sidebar navigation, status cluster, and language switch.
- [x] Move provider form and provider list into the `upstreams` workspace.
- [x] Move token form, token reveal, and token list into the `access` workspace.
- [x] Move endpoint copying and build info into the `overview` workspace.
- [x] Move log controls and log list into the `observability` workspace.

### Task 3: Update frontend behavior

**Files:**
- Modify: `internal/web/console.html`

**Interfaces:**
- Produces `setActiveView(view, updateHash)`.
- Produces `reloadLogs()` that honors source and status filters.

- [x] Add localized labels for navigation, page headings, endpoint rows, metrics, filters, and log preview.
- [x] Add hash navigation and active workspace rendering.
- [x] Update status rendering to drive overview metrics and sidebar readiness.
- [x] Add copy behavior for aggregate MCP endpoint.
- [x] Add log filter event handlers.

### Task 4: Replace visual system

**Files:**
- Modify: `internal/web/console.css`

**Interfaces:**
- Produces responsive app shell styling for the same static markup.

- [x] Replace decorative grid background with solid layered surfaces.
- [x] Add sidebar, workspace, page header, metric strip, endpoint rows, panels, lists, filters, and mobile navigation styles.
- [x] Keep all controls keyboard-visible and at least 40px high.
- [x] Add mobile layout rules for 860px and 560px widths.

### Task 5: Verify

**Files:**
- Check: `internal/web/console.html`
- Check: `internal/web/console.css`
- Check: changed docs

**Interfaces:**
- Produces verification evidence before handoff.

- [x] Run `go test ./...`.
- [x] Start a local static/mock server for the embedded console.
- [x] Capture desktop and 375px screenshots.
- [x] Inspect screenshots for overlapping text, unusable mobile layout, and empty rendered views.
- [x] Inspect `git diff --stat` and `git status --short`.
