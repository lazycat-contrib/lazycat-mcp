---
name: lazycat-mcp-gateway
description: Use when discovering, loading, and calling LazyCat app MCP providers or Skill resources through LazyCat MCP Gateway, including aggregated tools, skill_prompt, device notifications, domain lookup, and power operations.
---

# LazyCat MCP Gateway

懒猫微服平台的 MCP 网关应用。将本机懒猫应用的 MCP 能力和其他外部 MCP 服务聚合为统一入口，
供 AI Agent（Hermes、Claude、Cursor 等）通过标准 MCP 协议调用。

## Agent 使用流程

1. 连接本应用的 MCP 端点：`POST /mcp`，认证头使用 `Authorization: Bearer lcmcp_<token>`。
2. 先调用 `lazycat_mcp_provider_list`，读取可用 provider、聚合端点和工具命名规则。
3. 如果 provider 的 `kind` 是 `skill`，或返回了 `skill_title` / `skill_summary` / `skill_prompts`，先调用 `skill_prompt` 并传入该 provider 的 `slug`，读取完整 `SKILL.md`。
4. 按目标应用的 `SKILL.md` 决定下一步：调用聚合工具 `<provider_slug>__<tool_name>`，或使用代理端点 `/mcp/apps/<provider_slug>` 直接访问该上游。
5. 如果 provider 只有 MCP 没有 Skill，直接使用 `lazycat_mcp_provider_list` 返回的聚合工具或代理端点。

不要凭应用名猜工具参数；先读 provider 列表，遇到 Skill 资源先读 `skill_prompt`。

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
  "providers": [{"slug": "...", "kind": "mcp", "endpoint": "/mcp/apps/<slug>", "transport": "streamable_http"}]
}
```

当 provider 有 Skill 资源时，`kind` 会是 `skill`，并带有 `skill_title`、`skill_summary`、`skill_prompts`。这时必须先用 `skill_prompt` 读取完整说明。

### `skill_prompt`

返回指定 Skill provider 的完整 `SKILL.md`。参数：

```json
{"slug": "<provider_slug>"}
```

`slug` 来自 `lazycat_mcp_provider_list` 的 `providers[].slug`。如果返回 `skill not found`，说明该 provider 没有注册 Skill、`SKILL.md` 缺失，或资源目录未正确导入。

### `lazycat_device_query`

查询懒猫设备列表。参数：`status_kind`（`online` / `offline` / `all-device`）。

### `lazycat_device_notify`

向懒猫客户端设备发送系统通知。参数：`title`、`body`、可选 `deeplink_url`；默认发送给全部在线设备，也可以传 `device_id` 或 `device_ids` 定向发送。

### `domain_base_info_lookup`

域名基本信息查询。参数：`domain`（要查询的域名）。

### `lazycat_power`

懒猫电源操作。参数：`operation`（`power-off` / `reboot` / `query-led-status` / `led-off` / `led-on`）。

> 注意：`lazycat_device_query`、`lazycat_device_notify`、`domain_base_info_lookup`、`lazycat_power` 需要懒猫设备 API 网关可用。
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

本应用既导入其他应用的 Skill/MCP 资源，也把自己的 Skill 导出给系统：

```yaml
# package.yml
import_resources:
  - kind: skills
  - kind: mcp-providers

# lzc-build.yml
resource_exports:
  - kind: skills
    source: ./resources/skills
  - kind: mcp-providers
    source: ./resources/mcp-providers
```

运行时默认从 `LAZYCAT_MCP_RESOURCE_ROOT` 读取资源，默认值是 `/lzcapp/run/resources`。

### 发现其他应用的 Skill

资源扫描路径：

```
/lzcapp/run/resources/skills/<app-id>/SKILL.md
/lzcapp/run/resources/skills/<app-id>/<skill-name>/SKILL.md
```

扫描规则：
- 如果 `<app-id>/SKILL.md` 存在，将它作为该应用的单 Skill 资源。
- 否则扫描 `<app-id>/*/SKILL.md`，每个一级子目录都是一个 Skill resource。
- 跳过 `.digest` 和以 `.` 开头的目录。
- 对外公开路径为 `/skills/<app-id>/SKILL.md` 或 `/skills/<app-id>/<skill-name>/SKILL.md`，无需认证。

### 发现其他应用的 MCP provider

资源扫描路径：

```
/lzcapp/run/resources/mcp-providers/<app-id>/<provider-slug>/mcp.yml
```

`mcp.yml` 必须包含非空 `endpoint`。网关读取后通过应用间访问拼接上游地址：

```
http://app.<app-id>.lzcx<endpoint>
```

访问其他应用时，本应用需要 `lzcapp.user_delegate` 权限，并依赖懒猫平台注入的用户票据。

## 典型使用流程

1. 通过 Web 控制台添加上游 MCP Provider（懒猫应用或自定义服务）
2. 用 `lcmcp_` Token 连接 `/mcp` 端点
3. 调用 `lazycat_mcp_provider_list` 获取已配置的上游列表
4. 如果 provider 是 Skill，先调用 `skill_prompt` 读取完整 `SKILL.md`
5. 直接调用聚合工具（`<slug>__<tool>`）或通过 `/mcp/apps/<slug>` 代理端点访问上游

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
        ├── SKILL.md             # 单 Skill 资源，可选
        └── <skill-name>/
            └── SKILL.md         # 多 Skill 资源
```

## Prompt Examples

- 帮我列出当前懒猫系统里有哪些应用提供了 MCP 或 Skill，并告诉我应该先读哪个 Skill。
- 读取某个 Skill provider 的 `SKILL.md`，再按它的说明调用对应工具。
- 给我的手机发送一条懒猫通知，通知内容是任务已完成。
- 查询一个域名的基础信息，并说明是否能通过 LazyCat MCP 工具完成。

## 常见问题

| 错误 | 原因 | 解决 |
|------|------|------|
| `mcp token is required` | 请求头缺少认证 | 添加 `Authorization: Bearer lcmcp_...` |
| `mcp token is invalid` | Token 值错误或已禁用 | 检查 Token 是否正确、是否启用 |
| `mcp token is expired` | Token 已过期 | 在 Web 控制台续期或创建新 Token |
| `lazycat user ticket is missing` | 访问懒猫应用 Provider 时缺少用户凭证 | 先通过 Web 控制台登录以获取 ticket |
| 上游工具不出现在 tools/list | Provider 未启用或上游不可达 | 检查 Provider 状态和上游服务健康 |
| `skill not found for slug` | 该 provider 没有 Skill 资源，或 `SKILL.md` 未被资源扫描器发现 | 检查 `/lzcapp/run/resources/skills/<app-id>/.../SKILL.md`，或确认提供方配置了 `resource_exports` |
