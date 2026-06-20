---
name: lazycat-mcp-gateway
description: LazyCat MCP 多实例网关 — 发现、聚合、调用懒猫应用和自定义外部 MCP 服务。
---

# LazyCat MCP Gateway

懒猫微服平台的 MCP 网关应用。将本机懒猫应用的 MCP 能力和其他外部 MCP 服务聚合为统一入口，
供 AI Agent（Hermes、Claude、Cursor 等）通过标准 MCP 协议调用。

## 端点与传输

| 端点 | 传输方式 | 说明 |
|------|---------|------|
| `POST /mcp` | Streamable HTTP（推荐） | 请求/响应一体，最简单 |
| `GET /sse` | SSE | 建立长连接，返回 sessionId |
| `POST /message?sessionId=<id>` | SSE 消息通道 | 通过 SSE 连接接收响应 |

所有 MCP 端点都需要 Bearer Token 认证。

## 认证

```
Authorization: Bearer lcmcp_<token>
```

或使用 `X-MCP-Token` 头：

```
X-MCP-Token: lcmcp_<token>
```

Token 通过 Web 控制台（`/`）的"API 令牌"面板创建和管理，格式为 `lcmcp_` 前缀 + 随机字符串。

## 内置工具

### `lazycat_mcp_provider_list`

列出已配置的上游 MCP 提供者及其公开端点。返回结构：

```json
{
  "local": {"name": "LazyCat MCP", "endpoint": "/mcp", "transport": "streamable_http"},
  "aggregate": {"endpoint": "/mcp", "tool_naming": "<provider_slug>__<tool_name>"},
  "providers": [{"name": "...", "endpoint": "/mcp/apps/<slug>", "transport": "streamable_http"}]
}
```

### `lazycat_device_query`

查询懒猫设备列表。参数：`status_kind`（`online` / `offline` / `all-device`）。

### `domain_base_info_lookup`

域名基本信息查询。参数：`domain`（要查询的域名）。

### `lazycat_power`

懒猫电源操作。参数：`operation`（`power-off` / `reboot` / `query-led-status` / `led-off` / `led-on`）。

> 注意：`lazycat_device_query`、`domain_base_info_lookup`、`lazycat_power` 需要懒猫设备 API 网关可用。
> 如果网关不可用，这些工具不会出现在 `tools/list` 中。

## 上游 Provider 代理

### 两种 Provider 类型

| 类型 | 说明 | 配置字段 |
|------|------|---------|
| `lazycat` | 懒猫平台上的其他应用 | `app_id`、`endpoint`、`deploy_id` |
| `custom` | 外部自定义 MCP 服务 | `base_url`、`endpoint`、`headers`、`transport` |

### 上游工具聚合

启用的上游 Provider 的工具会自动聚合到网关的 `tools/list` 中，命名规则：

```
<provider_slug>__<upstream_tool_name>
```

例如：provider slug 为 `my-app`，上游工具名为 `search`，则聚合工具名为 `my-app__search`。

直接调用聚合工具名即可，网关自动转发到对应上游。

### 代理端点

每个上游 Provider 还有独立的代理端点：

```
/mcp/apps/<provider_slug>
```

可以绕过聚合，直接用该端点与上游 MCP 服务通信（使用相同 Token）。

## Web 控制台

访问 `/` 打开 Web 管理界面（需要通过懒猫平台登录），提供：

- **上游管理**：添加/编辑/删除懒猫应用和自定义 MCP 提供者
- **API 令牌**：创建和管理 MCP Token
- **调用日志**：查看 MCP 工具调用记录和耗时

## REST API

通过 Web 控制台登录后（`/api/*`），可管理：

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/status` | GET | 系统状态 |
| `/api/apps` | GET | 可用的懒猫应用列表 |
| `/api/tokens` | GET/POST | Token 列表/创建 |
| `/api/tokens/<id>` | PATCH/DELETE | Token 更新/删除 |
| `/api/providers` | GET/POST | Provider 列表/创建 |
| `/api/providers/<id>` | PATCH/DELETE | Provider 更新/删除 |
| `/api/mcp-logs` | GET/DELETE | 调用日志查询/清空 |

## Skill 资源

懒猫应用可通过 `resources/skills/<app-id>/<skill-name>/SKILL.md` 暴露技能文件。
这些文件通过 `/skills/<app-id>/<skill-name>/SKILL.md` 公开访问（无需认证）。

## 典型使用流程

1. 通过 Web 控制台添加上游 MCP Provider（懒猫应用或自定义服务）
2. 用 `lcmcp_` Token 连接 `/mcp` 端点
3. 调用 `lazycat_mcp_provider_list` 获取已配置的上游列表
4. 直接调用聚合工具（`<slug>__<tool>`）或通过 `/mcp/apps/<slug>` 代理端点访问上游

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LAZYCAT_MCP_ADDR` | `:3000` | 监听地址 |
| `LAZYCAT_MCP_DB` | `/lzcapp/var/data/lazycat-mcp.db` | 数据库路径 |
| `LAZYCAT_MCP_RESOURCE_ROOT` | `/lzcapp/run/resources` | 资源根目录 |
| `LAZYCAT_MCP_LOG_LEVEL` | `info` | 日志级别 |
| `LAZYCAT_MCP_LOG_DIR` | `/lzcapp/var/logs` | 日志目录 |
| `LAZYCAT_MCP_LOG_RETENTION_DAYS` | `30` | 调用日志保留天数 |

## 资源目录结构

```
<resource_root>/
├── mcp-providers/
│   └── <app-id>/
│       └── <provider-slug>/
│           └── mcp.yml          # endpoint: /mcp
└── skills/
    └── <app-id>/
        └── <skill-name>/
            └── SKILL.md
```

## 常见问题

| 错误 | 原因 | 解决 |
|------|------|------|
| `mcp token is required` | 请求头缺少认证 | 添加 `Authorization: Bearer lcmcp_...` |
| `mcp token is invalid` | Token 值错误或已禁用 | 检查 Token 是否正确、是否启用 |
| `mcp token is expired` | Token 已过期 | 在 Web 控制台续期或创建新 Token |
| `lazycat user ticket is missing` | 访问懒猫应用 Provider 时缺少用户凭证 | 先通过 Web 控制台登录以获取 ticket |
| 上游工具不出现在 tools/list | Provider 未启用或上游不可达 | 检查 Provider 状态和上游服务健康 |
