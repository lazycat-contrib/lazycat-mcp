---
name: lazycat-mcp-gateway
description: Use the LazyCat MCP gateway to discover and call MCP providers exposed by LazyCat apps.
---

# LazyCat MCP Gateway

1. Connect to this app's MCP endpoint: `/mcp`.
2. Send the gateway token in `Authorization: Bearer <token>`.
3. Call `lazycat_mcp_provider_list` first to discover the configured upstream programs and their `/mcp/apps/<subpath>` endpoints.
4. For upstream providers, keep using the same gateway token when connecting to the published endpoint.
