<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import PanelCard from './components/PanelCard.vue'
import ViewShell from './components/ViewShell.vue'

// ── Navigation ──
const views = [
  { key: 'overview',   label: '总览',       icon: '◉' },
  { key: 'upstreams',  label: '资源管理',   icon: '◇' },
  { key: 'access',     label: '访问凭据',   icon: '○' },
  { key: 'observability', label: '调用观测', icon: '◎' }
]

const THEME_STORAGE_KEY = 'lazycat-mcp-theme'
const initialThemeMode = (() => {
  const saved = localStorage.getItem(THEME_STORAGE_KEY)
  return saved === 'light' || saved === 'dark' || saved === 'system' ? saved : 'system'
})()

// ── State ──
const state = reactive({
  activeView: 'overview',
  status: null,
  apps: [], tokens: [], providers: [], logs: [],
  logRetentionDays: 0,
  appSearch: '', providerSearch: '',
  providerFilter: 'all',
  logFilters: { source: '', status: '' },
  selectedAppId: '',
  upstreamQuery: '',
  upstreamStatus: 'all',
  upstreamPage: 1,
  upstreamPageSize: 10,
  language: localStorage.getItem('lazycat-mcp-lang') || ((navigator.language || '').toLowerCase().startsWith('zh') ? 'zh' : 'en'),
  themeMode: initialThemeMode
})

const customProvider = reactive({
  name: '', description: '', slug: '',
  base_url: '', endpoint: '/mcp',
  transport: 'streamable_http', headers: []
})

const tokenForm = reactive({ name: '' })
const onceToken = ref('')
const tokenSecrets = ref({})
const showPublishDialog = ref(false)
const showDetailDialog = ref(false)
const showDeleteDialog = ref(false)
const deleteDialog = reactive({
  title: '',
  message: '',
  confirmText: '',
  fields: []
})
const pendingDeleteAction = ref(null)
const publishMode = ref('lazycat')
const publishRow = ref(null)
const detailRow = ref(null)
const deleteCandidate = ref(null)
const publishDraft = reactive({ name: '', slug: '', endpoint: '/mcp', transport: 'streamable_http' })
const toast = ref('')
const sysPrefersDark = ref(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)
const resolvedTheme = computed(() => state.themeMode === 'system' ? (sysPrefersDark.value ? 'dark' : 'light') : state.themeMode)
const themeIcon = computed(() => (state.themeMode === 'light' ? '☀️' : state.themeMode === 'dark' ? '🌙' : '💻'))
const themeModeLabel = computed(() => (state.themeMode === 'light' ? t('白天', 'Light') : state.themeMode === 'dark' ? t('夜晚', 'Dark') : t('系统', 'System')))
const onHashChange = () => setActiveView(initialView(), false)
let mediaQueryList = null
let mediaQueryHandler = null
let toastTimer = null
const TOKEN_SECRET_STORAGE_KEY = 'lazycat-mcp-token-secrets'
const OPEN_APP_TARGETS = {
  'cloud.lazycat.app.lazycat-filedrop-skill': 'lazycat-filedrop',
  'cloud.lazycat.app.lazycat-agent-browser-skill': 'lazycat-agent-browser'
}

const t = (zh, en) => state.language === 'en' ? en : zh

function setLang(lang) {
  state.language = lang === 'en' ? 'en' : 'zh'
  localStorage.setItem('lazycat-mcp-lang', state.language)
}
function setThemeMode(mode) {
  const next = mode === 'light' || mode === 'dark' ? mode : 'system'
  state.themeMode = next
  localStorage.setItem(THEME_STORAGE_KEY, next)
}
function cycleThemeMode() {
  const order = ['light', 'dark', 'system']
  const idx = order.indexOf(state.themeMode)
  setThemeMode(order[(idx + 1) % order.length])
}
function applyTheme(theme) {
  const next = theme === 'light' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', next)
  document.documentElement.style.colorScheme = next
}

watch(resolvedTheme, (theme) => applyTheme(theme), { immediate: true })

// ── Computed ──
const workbenchStats = computed(() => {
  const pendingApps = state.apps.filter((app) => unpublishedResourceCount(app) > 0).length
  const pendingResources = state.apps.reduce((sum, app) => sum + unpublishedResourceCount(app), 0)
  const activeTokens = state.tokens.filter((token) => token.enabled).length
  const tokenRisk = state.tokens.filter((token) => !token.enabled || isExpired(token.expires_at)).length
  const errorProviders = state.providers.filter((provider) => provider.aggregate_error || provider.aggregate_ok === false).length
  const jumpableResources = state.providers.filter((provider) => provider.app_id).length
  const healthyProviders = state.providers.filter((provider) => provider.aggregate_ok && !provider.aggregate_error).length
  return { pendingApps, pendingResources, activeTokens, tokenRisk, errorProviders, jumpableResources, healthyProviders }
})

const filteredApps = computed(() => {
  const query = state.appSearch.trim().toLowerCase()
  const filtered = state.apps.filter((app) => {
    if (!query) return true
    return [app.title, app.app_id, app.default_endpoint, app.default_slug].join(' ').toLowerCase().includes(query)
  })
  return filtered.sort((a, b) => {
    const diff = unpublishedResourceCount(b) - unpublishedResourceCount(a)
    if (diff !== 0) return diff
    return (a.title || a.app_id).localeCompare(b.title || b.app_id, 'zh-CN')
  })
})

const selectedApp = computed(() => state.apps.find((app) => app.app_id === state.selectedAppId) || filteredApps.value[0] || null)

watch(filteredApps, (apps) => {
  if (!apps.length) { state.selectedAppId = ''; return }
  if (!apps.some((app) => app.app_id === state.selectedAppId)) state.selectedAppId = apps[0].app_id
}, { immediate: true })

const filteredProviders = computed(() => {
  const query = state.providerSearch.trim().toLowerCase()
  return state.providers.filter((provider) => {
    if (state.providerFilter === 'errors' && !(provider.aggregate_error || provider.aggregate_ok === false)) return false
    if (state.providerFilter === 'custom' && provider.type !== 'custom') return false
    if (state.providerFilter === 'disabled' && provider.enabled) return false
    if (!query) return true
    return [provider.name, provider.slug, provider.public_endpoint, provider.app_id, provider.app_title, provider.resource_id, provider.kind, provider.skill_title].join(' ').toLowerCase().includes(query)
  }).sort((a, b) => {
    const errDiff = Number(!!b.aggregate_error || b.aggregate_ok === false) - Number(!!a.aggregate_error || a.aggregate_ok === false)
    if (errDiff !== 0) return errDiff
    return (a.name || '').localeCompare(b.name || '', 'zh-CN')
  })
})


const upstreamRows = computed(() => {
  const rows = []
  for (const app of state.apps) {
    for (const resource of (app.mcp_providers || [])) {
      const provider = state.providers.find((p) => p.app_id === app.app_id && p.resource_id === resource.resource_id)
      rows.push({
        key: `${app.app_id}:${resource.resource_id || 'default'}`,
        kind: 'app',
        app,
        resource,
        provider,
        title: app.title || app.app_id,
        appId: app.app_id,
        resourceId: resource.resource_id || 'default',
        endpoint: (provider?.public_endpoint || resource.endpoint || app.default_endpoint || '/mcp')
      })
    }
  }
  for (const provider of state.providers.filter((p) => p.type === 'custom')) {
    rows.push({
      key: `custom:${provider.id}`,
      kind: 'custom',
      app: null,
      resource: null,
      provider,
      title: provider.name || 'Custom MCP',
      appId: provider.slug || '-',
      resourceId: 'custom',
      endpoint: provider.public_endpoint || provider.endpoint || '/mcp'
    })
  }
  return rows.sort((a, b) => {
    const sa = a.provider ? (a.provider.enabled ? 1 : 2) : 0
    const sb = b.provider ? (b.provider.enabled ? 1 : 2) : 0
    if (sa !== sb) return sa - sb
    return (a.title || '').localeCompare(b.title || '', 'zh-CN')
  })
})

const filteredUpstreamRows = computed(() => {
  const q = state.upstreamQuery.trim().toLowerCase()
  return upstreamRows.value.filter((row) => {
    const status = row.provider ? (row.provider.enabled ? 'live' : 'offline') : 'pending'
    if (state.upstreamStatus === 'pending' && status !== 'pending') return false
    if (state.upstreamStatus === 'live' && status !== 'live') return false
    if (state.upstreamStatus === 'offline' && status !== 'offline') return false
    if (state.upstreamStatus === 'custom' && row.kind !== 'custom') return false
    if (!q) return true
    return [row.title, row.appId, row.resourceId, row.endpoint].join(' ').toLowerCase().includes(q)
  })
})

const upstreamTotalPages = computed(() => Math.max(1, Math.ceil(filteredUpstreamRows.value.length / state.upstreamPageSize)))
const pagedUpstreamRows = computed(() => {
  const page = Math.min(state.upstreamPage, upstreamTotalPages.value)
  const start = (page - 1) * state.upstreamPageSize
  return filteredUpstreamRows.value.slice(start, start + state.upstreamPageSize)
})

watch(() => [state.upstreamQuery, state.upstreamStatus], () => {
  state.upstreamPage = 1
})

const visibleLogs = computed(() => state.logs)
const recentLogs = computed(() => state.logs.slice(0, 6))

// ── System health ──
const systemStatus = computed(() => {
  const stats = workbenchStats.value
  if (!state.status?.has_user_ticket) return { tone: 'danger', text: t('等待票据', 'Awaiting ticket') }
  if (stats.errorProviders > 0) return { tone: 'warn', text: t(`${stats.errorProviders} 个异常入口`, `${stats.errorProviders} degraded`) }
  if (stats.pendingResources > 0) return { tone: 'warn', text: t(`${stats.pendingResources} 个待发布`, `${stats.pendingResources} pending`) }
  return { tone: 'ok', text: t('运行正常', 'Healthy') }
})

// ── Helpers ──
function appPublishedCount(app) { return state.providers.filter((p) => p.app_id === app.app_id).length }
function appResourceCount(app) { return (app.mcp_providers || []).length }
function unpublishedResourceCount(app) { return Math.max(0, appResourceCount(app) - appPublishedCount(app)) }
function providersForApp(appId) { return state.providers.filter((p) => p.app_id === appId) }
function providerOrigin(p) {
  if (p.type === 'custom') return t('自定义外部 MCP', 'Custom external MCP')
  if (p.app_id) return `${p.app_title || p.app_id}`
  return '-'
}
function resourceRows(app) {
  return (app?.mcp_providers || []).map((r) => ({
    ...r,
    published: state.providers.some((p) => p.app_id === app.app_id && p.resource_id === r.resource_id)
  })).sort((a, b) => Number(a.published) - Number(b.published))
}

function isSkillOnlyApp(app) {
  return !!(app?.has_skills && !app?.has_mcp)
}
function providerHealthMeta(provider) {
  if (provider.kind === 'skill') return { tone: 'info', text: t('Skill 资源', 'Skill resource') }
  if (provider.aggregate_error || provider.aggregate_ok === false) return { tone: 'danger', text: t('异常', 'Issue') }
  return { tone: 'ok', text: t('健康', 'OK') }
}

function upstreamStatusMeta(row) {
  if (!row.provider) return { tone: 'warn', text: t('待发布', 'Pending') }
  return row.provider.enabled ? { tone: 'ok', text: t('已上线', 'Live') } : { tone: 'soft', text: t('已下线', 'Offline') }
}
function normalizeDomain(value) {
  if (!value) return ''
  const trimmed = String(value).trim()
  if (!trimmed) return ''
  try {
    if (trimmed.includes('://')) return new URL(trimmed).hostname.toLowerCase()
  } catch {
    return ''
  }
  return trimmed.replace(/^\.+|\.+$/g, '').toLowerCase()
}
function rootDomainFromHost(host) {
  const normalized = normalizeDomain(host)
  if (!normalized) return ''
  const parts = normalized.split('.').filter(Boolean)
  if (parts.length < 2) return ''
  if (parts.length >= 4) return parts.slice(1).join('.')
  return normalized
}
function openTargetPrefix(row) {
  return OPEN_APP_TARGETS[row?.appId || row?.app?.app_id] || ''
}
function openRootDomain(row) {
  const fromDomain = rootDomainFromHost(row?.app?.domain)
  if (fromDomain) return fromDomain
  const fromSubdomain = rootDomainFromHost(row?.app?.subdomain)
  if (fromSubdomain) return fromSubdomain
  const fromCurrent = rootDomainFromHost(window.location.hostname)
  if (fromCurrent) return fromCurrent
  return ''
}
function openURLForRow(row) {
  const prefix = openTargetPrefix(row)
  const root = openRootDomain(row)
  if (!prefix || !root) return ''
  return `https://${prefix}.${root}/`
}
function canOpenRow(row) {
  return row?.kind === 'app' && !!openURLForRow(row)
}
function openApp(row) {
  const url = openURLForRow(row)
  if (!url) {
    showToast(t('未获取到应用域名，无法自动打开', 'App domain not available, cannot open automatically'))
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}
function viewUpstreamDetail(row) {
  detailRow.value = row
  showDetailDialog.value = true
}

// ── Actions ──
function setActiveView(view, updateHash = true) {
  state.activeView = views.some((item) => item.key === view) ? view : 'overview'
  if (updateHash) history.replaceState(null, '', `#${state.activeView}`)
}
function initialView() {
  const hash = location.hash.replace('#', '')
  return views.some((item) => item.key === hash) ? hash : 'overview'
}
function copyText(text) {
  if (!text) {
    showToast(t('没有可复制内容', 'Nothing to copy'))
    return
  }
  navigator.clipboard.writeText(text).then(
    () => showToast(t('已复制', 'Copied')),
    () => showToast(t('复制失败', 'Copy failed'))
  )
}
function loadTokenSecrets() {
  try {
    const raw = localStorage.getItem(TOKEN_SECRET_STORAGE_KEY)
    if (!raw) return
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return
    tokenSecrets.value = parsed
  } catch {
    tokenSecrets.value = {}
  }
}
function persistTokenSecrets() {
  localStorage.setItem(TOKEN_SECRET_STORAGE_KEY, JSON.stringify(tokenSecrets.value))
}
function tokenSecretKey(token) {
  return String(token?.id || '')
}
function rememberTokenSecret(token) {
  if (!token?.id || !token?.token) return
  tokenSecrets.value[tokenSecretKey(token)] = token.token
  persistTokenSecrets()
}
function tokenSecret(token) {
  if (!token?.id) return ''
  return tokenSecrets.value[tokenSecretKey(token)] || ''
}
function pruneTokenSecrets(tokens) {
  const keep = new Set((tokens || []).map((item) => String(item.id)))
  let changed = false
  for (const key of Object.keys(tokenSecrets.value)) {
    if (!keep.has(key)) {
      delete tokenSecrets.value[key]
      changed = true
    }
  }
  if (changed) persistTokenSecrets()
}

function showToast(message) {
  toast.value = message
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = '' }, 2600)
}
function buildInfo() {
  if (!state.status) return '-'
  return `${t('构建', 'Build')}: ${formatDateTime(state.status.build_time)} · ${state.status.commit || ''}`
}
function primaryEndpoint() {
  return state.status?.mcp_endpoint ? `${location.origin}${state.status.mcp_endpoint}` : `${location.origin}/mcp`
}
function skillPath() {
  return state.status?.skill_install_path ? `${location.origin}${state.status.skill_install_path}` : '-'
}
function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(state.language === 'en' ? 'en-US' : 'zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false
  })
}
function isExpired(value) {
  if (!value) return false
  const date = new Date(value)
  return !Number.isNaN(date.getTime()) && date.getTime() < Date.now()
}

// ── API ──
async function api(path, options = {}) {
  const headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {})
  const res = await fetch(path, Object.assign({}, options, { headers }))
  if (res.status === 204) return null
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data?.error?.message || `HTTP ${res.status}`)
  return data
}
function logsPath(limit) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (state.logFilters.source) params.set('source', state.logFilters.source)
  if (state.logFilters.status) params.set('status', state.logFilters.status)
  return `/api/mcp-logs?${params.toString()}`
}
async function loadAll() {
  const [status, apps, tokens, providers, logs] = await Promise.all([
    api('/api/status'), api('/api/apps'), api('/api/tokens'),
    api('/api/providers'), api(logsPath(100))
  ])
  state.status = status
  state.apps = apps.apps || []
  state.tokens = tokens.tokens || []
  pruneTokenSecrets(state.tokens)
  state.providers = providers.providers || []
  state.logs = logs.logs || []
  state.logRetentionDays = logs.retention_days ?? status.mcp_log_retention_days ?? 0
}
async function createToken() {
  const payload = { name: tokenForm.name.trim() || 'MCP Token' }
  const data = await api('/api/tokens', { method: 'POST', body: JSON.stringify(payload) })
  onceToken.value = data.token.token || ''
  rememberTokenSecret(data.token)
  tokenForm.name = ''
  await loadAll()
  showToast(t('Token 已创建', 'Token created'))
}
async function setTokenEnabled(token, enabled) {
  await api(`/api/tokens/${token.id}`, { method: 'PATCH', body: JSON.stringify({ enabled }) })
  await loadAll()
}
async function deleteToken(token) {
  askDeleteToken(token)
}
async function saveLazycatProvider(app, resource) {
  await api('/api/providers', { method: 'POST', body: JSON.stringify({
    type: 'lazycat', name: app.title || app.app_id, slug: app.default_slug || app.app_id,
    app_id: app.app_id, app_title: app.title, deploy_id: app.deploy_id || '',
    resource_id: resource?.resource_id || '', endpoint: resource?.endpoint || app.default_endpoint || '/mcp',
    transport: 'streamable_http', enabled: true
  })})
  await loadAll()
  showToast(t('资源已发布', 'Published'))
  showPublishDialog.value = false
}
function openPublishDialog(row) {
  if (!row || row.kind !== 'app' || !row.app || !row.resource) return
  publishMode.value = 'lazycat'
  publishRow.value = row
  publishDraft.name = row.provider?.name || row.app.title || row.app.app_id
  publishDraft.slug = row.provider?.slug || row.app.default_slug || row.app.app_id
  publishDraft.endpoint = row.resource.endpoint || row.app.default_endpoint || '/mcp'
  publishDraft.transport = row.provider?.transport || 'streamable_http'
  showPublishDialog.value = true
}
async function submitLazycatPublish() {
  if (!publishRow.value || !publishRow.value.app || !publishRow.value.resource) return
  const app = publishRow.value.app
  const resource = publishRow.value.resource
  const payload = {
    type: 'lazycat',
    name: publishDraft.name.trim() || app.title || app.app_id,
    slug: publishDraft.slug.trim() || app.default_slug || app.app_id,
    app_id: app.app_id,
    app_title: app.title,
    deploy_id: app.deploy_id || '',
    resource_id: resource.resource_id || '',
    endpoint: (publishDraft.endpoint || app.default_endpoint || '/mcp').trim(),
    transport: publishDraft.transport || 'streamable_http',
    enabled: true
  }
  if (publishRow.value.provider?.id) {
    await api(`/api/providers/${publishRow.value.provider.id}`, { method: 'PATCH', body: JSON.stringify(payload) })
  } else {
    await api('/api/providers', { method: 'POST', body: JSON.stringify(payload) })
  }
  await loadAll()
  showToast(t('资源已发布', 'Published'))
  showPublishDialog.value = false
  publishRow.value = null
}
async function saveCustomProvider() {
  await api('/api/providers', { method: 'POST', body: JSON.stringify({
    type: 'custom', name: customProvider.name.trim(), description: customProvider.description.trim(),
    slug: customProvider.slug.trim(), base_url: customProvider.base_url.trim(),
    endpoint: customProvider.endpoint.trim(), transport: customProvider.transport,
    headers: customProvider.headers.filter((h) => h.name.trim() || h.value.trim()), enabled: true
  })})
  Object.assign(customProvider, { name: '', description: '', slug: '', base_url: '', endpoint: '/mcp', transport: 'streamable_http', headers: [] })
  await loadAll()
  showToast(t('自定义 MCP 已添加', 'Custom MCP added'))
  showPublishDialog.value = false
}
function addHeaderRow() { customProvider.headers.push({ name: '', value: '' }) }
function removeHeaderRow(index) { customProvider.headers.splice(index, 1) }

async function setProviderEnabled(provider, enabled) {
  await api(`/api/providers/${provider.id}`, { method: 'PATCH', body: JSON.stringify({ enabled }) })
  await loadAll()
}
function openDeleteDialog({ title, message, confirmText, fields = [] }, onConfirm) {
  deleteDialog.title = title
  deleteDialog.message = message
  deleteDialog.confirmText = confirmText
  deleteDialog.fields = fields
  pendingDeleteAction.value = onConfirm
  showDeleteDialog.value = true
}
function askDeleteProvider(provider) {
  if (!provider) return
  deleteCandidate.value = provider
  openDeleteDialog({
    title: t('确认删除入口', 'Confirm route deletion'),
    message: t('确认删除该入口？删除后可通过“发布/重新发布”恢复。', 'Delete this route? You can recover it later via Publish/Republish.'),
    confirmText: t('确定删除', 'Delete'),
    fields: [
      { label: t('Provider ID', 'Provider ID'), value: String(provider.id) },
      { label: t('名称', 'Name'), value: provider.name || provider.slug || '-' },
      { label: t('入口路径', 'Route'), value: provider.public_endpoint || provider.endpoint || '-' },
      { label: t('类型', 'Type'), value: provider.type || '-' }
    ]
  }, async () => {
    await api(`/api/providers/${provider.id}`, { method: 'DELETE', headers: {} })
    await loadAll()
  })
}
function askDeleteToken(token) {
  if (!token) return
  openDeleteDialog({
    title: t('确认删除 Token', 'Confirm token deletion'),
    message: t('删除后该 Token 将无法继续调用 MCP。', 'After deletion this token can no longer call MCP.'),
    confirmText: t('确定删除', 'Delete'),
    fields: [
      { label: t('Token ID', 'Token ID'), value: String(token.id) },
      { label: t('名称', 'Name'), value: token.name || '-' },
      { label: t('前缀', 'Prefix'), value: token.prefix || '-' },
      { label: t('状态', 'Status'), value: token.enabled ? t('启用', 'Enabled') : t('停用', 'Disabled') }
    ]
  }, async () => {
    await api(`/api/tokens/${token.id}`, { method: 'DELETE', headers: {} })
    await loadAll()
  })
}
function askClearLogs() {
  openDeleteDialog({
    title: t('确认清空日志', 'Confirm log cleanup'),
    message: t('确认清空全部调用日志？该操作不可撤销。', 'Clear all call logs? This cannot be undone.'),
    confirmText: t('确定清空', 'Clear logs'),
    fields: [
      { label: t('当前日志条数', 'Current log count'), value: String(state.logs.length || 0) },
      { label: t('保留天数', 'Retention days'), value: String(state.logRetentionDays || 0) }
    ]
  }, async () => {
    const data = await api('/api/mcp-logs', { method: 'DELETE', headers: {} })
    showToast(t(`已清空 ${data.deleted || 0} 条`, `Cleared ${data.deleted || 0}`))
    await loadAll()
  })
}
function cancelDeleteProvider() {
  showDeleteDialog.value = false
  deleteDialog.title = ''
  deleteDialog.message = ''
  deleteDialog.confirmText = ''
  deleteDialog.fields = []
  pendingDeleteAction.value = null
  deleteCandidate.value = null
}
async function confirmDeleteProvider() {
  if (!pendingDeleteAction.value) return
  const action = pendingDeleteAction.value
  await action()
  cancelDeleteProvider()
}
async function reloadLogs() {
  const data = await api(logsPath(100))
  state.logs = data.logs || []
  state.logRetentionDays = data.retention_days ?? state.logRetentionDays
}
async function cleanupLogs() {
  const data = await api('/api/mcp-logs/cleanup', { method: 'POST', body: '{}' })
  showToast(t(`已清理 ${data.deleted || 0} 条`, `Cleaned ${data.deleted || 0}`))
  await loadAll()
}
async function clearLogs() {
  askClearLogs()
}

onMounted(async () => {
  loadTokenSecrets()
  setActiveView(initialView(), false)
  window.addEventListener('hashchange', onHashChange)
  if (window.matchMedia) {
    mediaQueryList = window.matchMedia('(prefers-color-scheme: dark)')
    sysPrefersDark.value = mediaQueryList.matches
    mediaQueryHandler = (event) => {
      sysPrefersDark.value = event.matches
    }
    if (mediaQueryList.addEventListener) mediaQueryList.addEventListener('change', mediaQueryHandler)
    else mediaQueryList.addListener(mediaQueryHandler)
  }
  await loadAll().catch((error) => showToast(error.message))
})

onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHashChange)
  if (!mediaQueryList || !mediaQueryHandler) return
  if (mediaQueryList.removeEventListener) mediaQueryList.removeEventListener('change', mediaQueryHandler)
  else mediaQueryList.removeListener(mediaQueryHandler)
})
</script>

<template>
  <a class="skip-link" href="#mainContent">{{ t('跳到主要内容', 'Skip to main content') }}</a>
  <div class="shell">
    <!-- ── Sidebar ── -->
    <aside class="sidebar" aria-label="Console navigation">
      <div class="brand">
        <div class="mark">M</div>
        <h1>{{ t('懒猫 MCP', 'LazyCat MCP') }}</h1>
        <button
          type="button"
          class="theme-toggle"
          :title="`${t('主题', 'Theme')}：${themeModeLabel}（${t('点击切换', 'click to switch')}）`"
          :aria-label="`${t('主题', 'Theme')}：${themeModeLabel}`"
          @click="cycleThemeMode"
        >{{ themeIcon }}</button>
      </div>

      <nav class="nav">
        <button
          v-for="item in views" :key="item.key" type="button"
          :class="{ active: state.activeView === item.key }"
          @click="setActiveView(item.key)"
        >
          <span class="nav-icon">{{ item.icon }}</span>
          <span class="nav-label">{{ item.label }}</span>
        </button>
      </nav>

      <div class="sidebar-actions">
        <button type="button" :class="{ active: state.language === 'zh' }" @click="setLang('zh')">中</button>
        <button type="button" :class="{ active: state.language === 'en' }" @click="setLang('en')">EN</button>
        <button type="button" class="ghost" @click="loadAll().catch((error) => showToast(error.message))">↻</button>
      </div>
    </aside>

    <!-- ── Main ── -->
    <main id="mainContent" class="workspace" tabindex="-1">
      <!-- ══ OVERVIEW ══ -->
      <template v-if="state.activeView === 'overview'">
        <ViewShell :title="t('总览', 'Overview')" />

        <div class="overview-grid">
          <!-- System status -->
          <div class="status-bar" :class="systemStatus.tone">
            <div class="status-dot" :class="systemStatus.tone"></div>
            <span>{{ systemStatus.text }}</span>
            <span class="status-divider">·</span>
            <span class="muted">{{ t(`入口 ${state.providers.length}`, `${state.providers.length} routes`) }}</span>
            <span class="status-divider">·</span>
            <span class="muted">{{ t(`Token ${state.tokens.length}`, `${state.tokens.length} tokens`) }}</span>
            <span class="status-spacer"></span>
            <span class="muted mono">{{ state.status?.version || '-' }}</span>
          </div>

          <!-- Two column layout -->
          <div class="overview-two-col">
            <!-- Left: metrics + recent calls -->
            <div class="overview-left">
              <PanelCard :title="t('最近调用', 'Recent calls')">
                <div v-if="recentLogs.length" class="log-list">
                  <div v-for="log in recentLogs" :key="log.id" class="log-row">
                    <div>
                      <span class="pill" :class="log.status === 'error' ? 'danger' : 'ok'">{{ log.status === 'error' ? 'ERR' : 'OK' }}</span>
                      <span class="mono">{{ log.provider_slug ? `${log.method} ${log.provider_slug}/${log.target}` : log.method }}</span>
                    </div>
                    <span class="muted mono">{{ formatDateTime(log.created_at) }}</span>
                  </div>
                </div>
                <div v-else class="empty-compact">{{ t('暂无调用日志', 'No call logs yet') }}</div>
              </PanelCard>
            </div>

            <!-- Right: connection info -->
            <PanelCard :title="t('接入', 'Connection')">
              <div class="connection-block">
                <div class="conn-row">
                  <span class="muted">{{ t('MCP 接入地址', 'MCP endpoint') }}</span>
                  <div class="mono conn-value">{{ primaryEndpoint() }}</div>
                  <button type="button" class="ghost compact" @click="copyText(primaryEndpoint())">{{ t('复制', 'Copy') }}</button>
                </div>
                <div class="conn-row">
                  <span class="muted">{{ t('Skill 安装地址', 'Skill install URL') }}</span>
                  <div class="mono conn-value">{{ skillPath() }}</div>
                  <button type="button" class="ghost compact" @click="copyText(skillPath())">{{ t('复制', 'Copy') }}</button>
                </div>
                <div class="build-line muted mono">{{ buildInfo() }}</div>
              </div>
            </PanelCard>
          </div>
        </div>
      </template>

      <!-- ══ PUBLISHING ══ -->
      <template v-else-if="state.activeView === 'upstreams'">
        <ViewShell :title="t('资源管理', 'Management')">
          <template #actions>
            <span class="pill soft">{{ t(`共 ${filteredUpstreamRows.length} 条`, `${filteredUpstreamRows.length} items`) }}</span>
            <button type="button" class="ghost compact" @click="publishMode = 'custom'; publishRow = null; showPublishDialog = true">{{ t('新增自定义', 'New custom') }}</button>
          </template>
        </ViewShell>

        <div class="upstream-list-toolbar">
          <input v-model="state.upstreamQuery" autocomplete="off" :placeholder="t('查询应用 / 资源 / 入口...', 'Search app / resource / route...')" />
          <select v-model="state.upstreamStatus">
            <option value="all">{{ t('全部状态', 'All status') }}</option>
            <option value="pending">{{ t('待发布', 'Pending') }}</option>
            <option value="live">{{ t('已上线', 'Live') }}</option>
            <option value="offline">{{ t('已下线', 'Offline') }}</option>
            <option value="custom">{{ t('自定义', 'Custom') }}</option>
          </select>
          <button type="button" class="ghost compact" @click="loadAll().catch((error) => showToast(error.message))">{{ t('刷新', 'Refresh') }}</button>
        </div>

        <div class="upstream-list">
          <div class="upstream-list-head">
            <span>{{ t('应用', 'App') }}</span>
            <span>{{ t('资源', 'Resource') }}</span>
            <span>{{ t('入口', 'Route') }}</span>
            <span>{{ t('状态', 'Status') }}</span>
            <span>{{ t('操作', 'Actions') }}</span>
          </div>

          <div v-if="!pagedUpstreamRows.length" class="empty-compact muted">{{ t('暂无数据', 'No items') }}</div>

          <div v-for="row in pagedUpstreamRows" :key="row.key" class="upstream-row">
            <div>
              <strong>{{ row.title }}</strong>
              <div class="mono muted">{{ row.appId }}</div>
            </div>
            <div class="mono">{{ row.resourceId }}</div>
            <div class="mono">{{ row.endpoint }}</div>
            <span class="pill" :class="upstreamStatusMeta(row).tone">{{ upstreamStatusMeta(row).text }}</span>
            <div class="row-actions">
              <button v-if="!row.provider && row.kind === 'app'" type="button" class="row-btn row-btn-publish" @click="openPublishDialog(row)">{{ t('发布', 'Publish') }}</button>
              <button v-if="row.provider?.enabled" type="button" class="row-btn row-btn-toggle" @click="setProviderEnabled(row.provider, false).catch((error) => showToast(error.message))">{{ t('下线', 'Offline') }}</button>
              <button v-if="row.provider && !row.provider.enabled && row.kind === 'app'" type="button" class="row-btn row-btn-publish" @click="openPublishDialog(row)">{{ t('重新发布', 'Republish') }}</button>
              <button v-if="row.provider" type="button" class="row-btn row-btn-delete" @click="askDeleteProvider(row.provider)">{{ t('删除', 'Delete') }}</button>
              <button v-if="canOpenRow(row)" type="button" class="row-btn row-btn-open" @click="openApp(row)">{{ t('打开', 'Open') }}</button>
              <button type="button" class="row-btn row-btn-detail" @click="viewUpstreamDetail(row)">{{ t('详情', 'Detail') }}</button>
            </div>
          </div>
        </div>

        <div class="upstream-pagination">
          <button type="button" class="ghost compact" :disabled="state.upstreamPage <= 1" @click="state.upstreamPage = Math.max(1, state.upstreamPage - 1)">{{ t('上一页', 'Prev') }}</button>
          <span class="muted">{{ t(`第 ${Math.min(state.upstreamPage, upstreamTotalPages)} / ${upstreamTotalPages} 页`, `Page ${Math.min(state.upstreamPage, upstreamTotalPages)} / ${upstreamTotalPages}`) }}</span>
          <button type="button" class="ghost compact" :disabled="state.upstreamPage >= upstreamTotalPages" @click="state.upstreamPage = Math.min(upstreamTotalPages, state.upstreamPage + 1)">{{ t('下一页', 'Next') }}</button>
        </div>
      </template>

      <!-- ══ ACCESS ══ -->
      <template v-else-if="state.activeView === 'access'">
        <ViewShell :title="t('访问凭据', 'Access')">
          <template #actions>
            <span class="pill ok">{{ workbenchStats.activeTokens }} {{ t('活跃', 'active') }}</span>
          </template>
        </ViewShell>

        <div class="access-layout">
          <PanelCard :title="t('创建 Token', 'Create token')">
            <form class="token-form" @submit.prevent="createToken().catch((error) => showToast(error.message))">
              <input v-model="tokenForm.name" :placeholder="t('Token 名称', 'Token name')" autocomplete="off" />
              <button class="primary" type="submit">{{ t('创建', 'Create') }}</button>
            </form>
            <div v-if="onceToken" class="token-result">
              <span class="muted">{{ t('新 Token（仅显示一次）', 'New token (shown once)') }}</span>
              <div class="mono token-value">{{ onceToken }}</div>
              <button type="button" class="ghost compact" @click="copyText(onceToken)">{{ t('复制', 'Copy') }}</button>
            </div>
          </PanelCard>

          <PanelCard :title="t('已发放', 'Issued')">
            <div v-if="!state.tokens.length" class="empty-compact muted">{{ t('还没有 Token', 'No tokens yet') }}</div>
            <div v-for="token in state.tokens" :key="token.id" class="token-row">
              <div>
                <strong>{{ token.name }}</strong>
                <span class="mono muted">{{ token.prefix }}</span>
                <span class="muted">{{ formatDateTime(token.created_at) }}</span>
              </div>
              <div class="row-actions">
                <span class="pill" :class="token.enabled ? 'ok' : 'warn'">{{ token.enabled ? t('启用', 'On') : t('停用', 'Off') }}</span>
                <span v-if="token.expires_at" class="pill" :class="isExpired(token.expires_at) ? 'danger' : 'soft'">{{ formatDateTime(token.expires_at) }}</span>
                <button type="button" class="ghost compact" @click="setTokenEnabled(token, !token.enabled).catch((error) => showToast(error.message))">{{ token.enabled ? t('停用', 'Disable') : t('启用', 'Enable') }}</button>
                <button v-if="tokenSecret(token)" type="button" class="ghost compact" @click="copyText(tokenSecret(token))">{{ t('复制 Token', 'Copy token') }}</button>
                <button type="button" class="ghost compact danger" @click="askDeleteToken(token)">{{ t('删除', 'Delete') }}</button>
              </div>
            </div>
          </PanelCard>
        </div>
      </template>

      <!-- ══ OBSERVABILITY ══ -->
      <template v-else>
        <ViewShell :title="t('调用观测', 'Observability')">
          <template #actions>
            <span v-if="workbenchStats.errorProviders" class="pill danger">{{ workbenchStats.errorProviders }} {{ t('异常', 'issues') }}</span>
          </template>
        </ViewShell>

        <div class="obs-layout">
          <PanelCard :title="t('调用日志', 'Call logs')">
            <template #head-actions>
              <div class="head-actions-row">
                <button class="ghost compact" type="button" @click="reloadLogs().catch((error) => showToast(error.message))">{{ t('刷新', 'Refresh') }}</button>
                <button class="ghost compact" type="button" @click="cleanupLogs().catch((error) => showToast(error.message))">{{ t('清理', 'Cleanup') }}</button>
                <button class="ghost compact danger" type="button" @click="askClearLogs()">{{ t('清空', 'Clear') }}</button>
              </div>
            </template>

            <div class="log-filters">
              <select v-model="state.logFilters.source" @change="reloadLogs().catch((error) => showToast(error.message))">
                <option value="">{{ t('全部来源', 'All sources') }}</option>
                <option value="local">{{ t('本机', 'Local') }}</option>
                <option value="upstream">{{ t('上游', 'Upstream') }}</option>
              </select>
              <select v-model="state.logFilters.status" @change="reloadLogs().catch((error) => showToast(error.message))">
                <option value="">{{ t('全部状态', 'All') }}</option>
                <option value="success">{{ t('成功', 'Success') }}</option>
                <option value="error">{{ t('错误', 'Error') }}</option>
              </select>
            </div>

            <div class="log-list">
              <div v-if="!visibleLogs.length" class="empty-compact muted">{{ t('暂无调用日志', 'No call logs') }}</div>
              <div v-for="log in visibleLogs" :key="log.id" class="log-row">
                <div>
                  <span class="pill" :class="log.status === 'error' ? 'danger' : 'ok'">{{ log.status === 'error' ? 'ERR' : 'OK' }}</span>
                  <span>{{ log.provider_slug ? `${log.method} → ${log.provider_slug}/${log.target}` : log.method }}</span>
                </div>
                <div class="log-row-meta">
                  <span class="muted mono">{{ formatDateTime(log.created_at) }} · {{ log.duration_ms || 0 }}ms</span>
                  <span class="pill soft">{{ log.source === 'upstream' ? t('上游', 'Up') : t('本机', 'Local') }}</span>
                  <span v-if="log.status_code" class="pill soft">HTTP {{ log.status_code }}</span>
                </div>
                <div v-if="log.error" class="log-error mono">{{ log.error }}</div>
              </div>
            </div>
          </PanelCard>

          <PanelCard :title="t('异常入口', 'Degraded routes')">
            <div v-if="!filteredProviders.some((p) => p.aggregate_error || p.aggregate_ok === false)" class="empty-compact ok">{{ t('全部健康', 'All healthy') }}</div>
            <div v-for="p in filteredProviders.filter((p) => p.aggregate_error || p.aggregate_ok === false).slice(0, 6)" :key="p.id" class="deg-row">
              <div>
                <strong>{{ p.name }}</strong>
                <span class="mono muted">{{ p.public_endpoint }}</span>
              </div>
              <span class="pill danger">{{ t('异常', 'Issue') }}</span>
              <div v-if="p.aggregate_error" class="muted">{{ p.aggregate_error }}</div>
            </div>
          </PanelCard>
        </div>
      </template>
    </main>
  </div>

  <!-- ── Publish Dialog ── -->
  <div v-if="showPublishDialog" class="modal-shell" @click.self="showPublishDialog = false">
    <div class="modal-card">
      <div class="modal-head">
        <h3>{{ t('发布动作', 'Publishing') }}</h3>
        <button type="button" class="ghost compact" @click="showPublishDialog = false; publishRow = null">{{ t('关闭', 'Close') }}</button>
      </div>
      <div class="mode-tabs">
        <button type="button" :class="{ active: publishMode === 'lazycat' }" @click="publishMode = 'lazycat'">{{ t('懒猫应用', 'LazyCat app') }}</button>
        <button type="button" :class="{ active: publishMode === 'custom' }" @click="publishMode = 'custom'">{{ t('自定义 MCP', 'Custom MCP') }}</button>
      </div>
      <form v-if="publishMode === 'lazycat'" class="form-grid modal-body" @submit.prevent="submitLazycatPublish().catch((error) => showToast(error.message))">
        <template v-if="publishRow">
          <div class="detail-header span-2">
            <div>
              <strong>{{ publishRow.title }}</strong>
              <span class="mono muted">{{ publishRow.appId }} / {{ publishRow.resourceId }}</span>
            </div>
          </div>
          <label><span>{{ t('名称', 'Name') }}</span><input v-model="publishDraft.name" autocomplete="off" required /></label>
          <label><span>{{ t('公开路径', 'Slug') }}</span><input v-model="publishDraft.slug" autocomplete="off" required /></label>
          <label class="span-2"><span>{{ t('MCP 路径', 'MCP path') }}</span><input v-model="publishDraft.endpoint" autocomplete="off" required /></label>
          <label><span>{{ t('传输方式', 'Transport') }}</span>
            <select v-model="publishDraft.transport">
              <option value="streamable_http">Streamable HTTP</option>
              <option value="sse">SSE</option>
            </select>
          </label>
          <div class="actions span-2 modal-actions">
            <button class="primary" type="submit">{{ t('确认发布', 'Publish now') }}</button>
          </div>
        </template>
        <div v-else class="empty-compact muted">{{ t('请先在列表里点击发布', 'Click publish from list first') }}</div>
      </form>
      <form v-else class="form-grid modal-body" @submit.prevent="saveCustomProvider().catch((error) => showToast(error.message))">
        <label><span>{{ t('名称', 'Name') }}</span><input v-model="customProvider.name" autocomplete="off" required /></label>
        <label><span>{{ t('公开路径', 'Path') }}</span><input v-model="customProvider.slug" autocomplete="off" required /></label>
        <label class="span-2"><span>{{ t('说明', 'Description') }}</span><input v-model="customProvider.description" autocomplete="off" /></label>
        <label class="span-2"><span>{{ t('服务地址', 'Service URL') }}</span><input v-model="customProvider.base_url" autocomplete="off" required /></label>
        <label><span>{{ t('MCP 路径', 'MCP path') }}</span><input v-model="customProvider.endpoint" autocomplete="off" required /></label>
        <label><span>{{ t('传输方式', 'Transport') }}</span>
          <select v-model="customProvider.transport">
            <option value="streamable_http">Streamable HTTP</option>
            <option value="sse">SSE</option>
          </select>
        </label>
        <div class="header-editor span-2">
          <div class="field-head">
            <span>{{ t('请求头', 'Headers') }}</span>
            <button type="button" class="ghost compact" @click="addHeaderRow">{{ t('添加', 'Add') }}</button>
          </div>
          <div class="header-rows">
            <div v-for="(h, i) in customProvider.headers" :key="i" class="header-row">
              <input v-model="h.name" :placeholder="t('名称', 'Name')" autocomplete="off" />
              <input v-model="h.value" :placeholder="t('值', 'Value')" autocomplete="off" />
              <button type="button" class="danger compact" @click="removeHeaderRow(i)">×</button>
            </div>
          </div>
        </div>
        <div class="actions span-2 modal-actions">
          <button class="primary" type="submit">{{ t('保存', 'Save') }}</button>
        </div>
      </form>
    </div>
  </div>

  <div v-if="showDetailDialog && detailRow" class="modal-shell" @click.self="showDetailDialog = false">
    <div class="modal-card detail-modal">
      <div class="modal-head">
        <div class="detail-title-wrap">
          <h3>{{ t('入口详情', 'Route detail') }}</h3>
          <span class="pill" :class="upstreamStatusMeta(detailRow).tone">{{ upstreamStatusMeta(detailRow).text }}</span>
        </div>
        <button type="button" class="ghost compact" @click="showDetailDialog = false">{{ t('关闭', 'Close') }}</button>
      </div>
      <div class="detail-grid">
        <div class="detail-item"><span class="muted">{{ t('名称', 'Name') }}</span><strong>{{ detailRow.title }}</strong></div>
        <div class="detail-item"><span class="muted">{{ t('类型', 'Kind') }}</span><strong>{{ detailRow.kind }}</strong></div>
        <div class="detail-item"><span class="muted">{{ t('应用ID', 'App ID') }}</span><code class="mono">{{ detailRow.appId }}</code></div>
        <div class="detail-item"><span class="muted">{{ t('资源ID', 'Resource ID') }}</span><code class="mono">{{ detailRow.resourceId }}</code></div>
        <div class="detail-item span-2"><span class="muted">{{ t('入口路径', 'Route') }}</span><code class="mono">{{ detailRow.endpoint }}</code></div>
        <div v-if="detailRow.provider" class="detail-item span-2"><span class="muted">{{ t('Provider ID', 'Provider ID') }}</span><code class="mono">{{ detailRow.provider.id }}</code></div>
      </div>
      <div class="actions modal-actions">
        <button type="button" class="primary" @click="showDetailDialog = false">{{ t('确定', 'OK') }}</button>
      </div>
    </div>
  </div>


  <div v-if="showDeleteDialog" class="modal-shell" @click.self="cancelDeleteProvider">
    <div class="modal-card delete-modal">
      <div class="modal-head">
        <h3>{{ deleteDialog.title || t('确认删除', 'Confirm deletion') }}</h3>
        <button type="button" class="ghost compact" @click="cancelDeleteProvider">{{ t('关闭', 'Close') }}</button>
      </div>
      <div class="modal-body">
        <p class="delete-modal-message">{{ deleteDialog.message }}</p>
        <div class="delete-modal-fields">
          <div v-for="(field, idx) in deleteDialog.fields" :key="idx" class="delete-field">
            <span class="delete-field-label">{{ field.label }}</span>
            <code class="delete-field-value mono">{{ field.value }}</code>
          </div>
        </div>
        <div class="actions modal-actions">
          <button type="button" class="ghost compact" @click="cancelDeleteProvider">{{ t('取消', 'Cancel') }}</button>
          <button type="button" class="danger" @click="confirmDeleteProvider().catch((error) => showToast(error.message))">{{ deleteDialog.confirmText || t('确定删除', 'Delete') }}</button>
        </div>
      </div>
    </div>
  </div>

  <teleport to="body">
    <div v-if="toast" class="toast">{{ toast }}</div>
  </teleport>
</template>
