# LazyCat MCP console app shell design

## Goal

Replace the current all-in-one console with a task-oriented operator workspace that separates setup, access control, endpoint copying, and troubleshooting while keeping the existing plain HTML, CSS, and JavaScript deployment model.

## Current problem

The console currently renders upstream provider creation, provider management, token management, skills endpoint, aggregate MCP endpoint, build info, language switching, ticket state, and call logs in one two-column grid. The layout makes unrelated tasks visually equal, forces operators to scan too much content, and has no stable mental model for future features.

## Product model

The console is an operational control surface, not a landing page. It must help an operator answer four questions quickly:

- Is the gateway ready to use?
- Which endpoint should I copy into an MCP client or skill installer?
- How do I publish or stop an upstream provider?
- Why did a call succeed or fail?

## Information architecture

Use four primary workspaces:

- **Overview**: status summary, aggregate endpoint, skills endpoint, build info, and recent call preview.
- **Upstreams**: LazyCat app upstream form, custom MCP form, and published upstream list.
- **Access**: token creation, one-time token reveal, token list, enable/disable/delete actions.
- **Observability**: call log filters, reload, cleanup, clear all, metadata-only call list.

## Visual direction

Use a dense dark operator shell with a left navigation rail on desktop and a horizontal workspace switcher on small screens. Remove decorative grid backgrounds. Communicate depth through background steps and hairline separators rather than heavy shadows. Keep LazyCat yellow as a primary action and readiness accent, with green for healthy state and red for destructive actions.

## Interaction requirements

- Preserve the existing API surface: `/api/status`, `/api/apps`, `/api/tokens`, `/api/providers`, `/api/mcp-logs`, and `/api/mcp-logs/cleanup`.
- Preserve Chinese and English localization, with new copy added to the existing `messages` object.
- Maintain the provider mode tabs for LazyCat app and custom MCP creation.
- Add hash-based workspace navigation so reloads can reopen the same workspace.
- Add log source and status filters using existing query parameters.
- Keep token plaintext visible only in the one-time token reveal area.
- Do not expose request bodies, response bodies, provider secrets, token values, or custom header values in logs or lists.

## Accessibility and responsiveness

- Add a skip-to-content link.
- Use real buttons for workspace navigation.
- Mark the active workspace for assistive technology.
- Keep touch targets at least 40px high.
- Verify desktop and mobile widths, including 375px.
- Honor `prefers-reduced-motion`.

## Out of scope

- Introducing a build chain or runtime assets that cannot be embedded into the Go binary.
- Pulling frontend runtime code from a CDN at runtime.
- Changing backend APIs.
- Adding persisted UI preferences beyond the existing language setting and hash route.
