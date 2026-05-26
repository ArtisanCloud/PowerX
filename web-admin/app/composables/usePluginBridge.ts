import { computed, watch } from "vue";
import { useI18n, useColorMode } from "#imports";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import { getStoredTenantUUID } from "~/utils/tenant-context";

type PluginMeta = {
  pluginId: string
  instanceId?: string
  targetOrigin: string
  frame: HTMLIFrameElement
}

type PowerXToPlugin =
  | { source: 'powerx'; type: 'locale'; locale: string }
  | { source: 'powerx'; type: 'theme'; theme: 'light' | 'dark' | 'system' }
  | { source: 'powerx'; type: 'navigate'; path: string }
  | {
  source: 'powerx';
  type: 'sync';
  locale: string;
  theme: 'light' | 'dark' | 'system';
  hostOrigin: string;
  pluginId: string;
  instanceId?: string;
}
  | {
  source: 'powerx';
  type: 'auth-token';
  accessToken: string;
  refreshToken?: string;
  tokenType?: string;
  expiresIn?: number;
  expiresAt?: number;
  scope?: string;
  pluginId?: string;
  hostOrigin?: string;
  tenantUuid?: string;
  tenant_uuid?: string;
}

type PluginToPowerX =
  | { source: 'plugin'; type: 'ready'; pluginId?: string; instanceId?: string }
  | { source: 'plugin'; type: 'request-sync' }
  | { source: 'plugin'; type: 'ping'; ts: number }


type ThemeKey = "light" | "dark" | "system";
const normalizeThemePreference = (input?: string | null): ThemeKey => {
  const value = String(input ?? "").trim().toLowerCase();
  if (value === "dark" || value === "light") return value;
  if (value === "system" || value === "auto") return "system";
  return "system";
};

// 日志开关：仅当 NUXT_PUBLIC_BRIDGE_DEBUG === 'true' 时输出
const shouldLogBridge = process.env.NUXT_PUBLIC_BRIDGE_DEBUG === 'true'

// 统一把 locale 转成字符串（防止传入对象）
function toLocaleCode(input: any): string {
  if (typeof input === 'string') return input
  if (input && typeof input === 'object') {
    if (typeof input.code === 'string') return input.code
    if (typeof input.locale === 'string') return input.locale
  }
  return String(input ?? '')
}

const log = (...args: any[]) => {
  if (shouldLogBridge) {
    // eslint-disable-next-line no-console
    console.info('[Bridge][Admin]', ...args)
  }
}

if (shouldLogBridge) {
  console.info('[Bridge][Admin] debug mode enabled')
}

const maskToken = (token?: string | null) => {
  if (!token) return '<empty>'
  if (token.length <= 8) return `${token.slice(0, 3)}...`
  return `${token.slice(0, 4)}...${token.slice(-6)}`
}

export function usePluginBridge() {
  const registry = useState<Map<HTMLIFrameElement, PluginMeta>>('px:iframes', () => new Map())
  const pendingNavigate = useState<Map<HTMLIFrameElement, string>>('px:pendingNavigate', () => new Map())
  const auth = useAuth()
  const userStore = useUserStore()
  const { currentTenantUuid } = storeToRefs(userStore)

  const {locale} = useI18n()
  const colorMode = useColorMode()
  const themePreference = computed<ThemeKey>(() =>
    normalizeThemePreference(colorMode.preference)
  )
  const resolvedTheme = computed<'light' | 'dark'>(() =>
    colorMode.value === 'dark' ? 'dark' : 'light'
  )

  const sendTo = (meta: PluginMeta, msg: PowerXToPlugin) => {
    try {
      // 统一使用 '*', 避免 127/localhost 或代理导致的 origin 不匹配
      meta.frame.contentWindow?.postMessage(msg, '*')
      log('postMessage sent', { target: '*', type: (msg as any).type, pluginId: meta.pluginId })
    } catch (err) {
      console.warn('[DBG][Admin->Plugin] postMessage error', err)
    }
  }

  const broadcast = (msg: PowerXToPlugin) => {
    if (!process.client) return
    // console.info('[DBG][Admin] broadcast', msg)
    registry.value.forEach(meta => sendTo(meta, msg))
  }

  const syncMeta = (m: PluginMeta) => {
    const payload = {
      source: 'powerx',
      type: 'sync',
      locale: toLocaleCode(locale?.value),
      theme: themePreference.value,
      hostOrigin: window.location.origin,
      pluginId: m.pluginId,
      instanceId: m.instanceId,
    } as const
    // console.info('[DBG][Admin->Plugin] syncMeta', { to: m.targetOrigin, pluginId: m.pluginId, instanceId: m.instanceId, payload })
    sendTo(m, payload)
  }

  const resolveTenantUUID = () =>
    currentTenantUuid.value || getStoredTenantUUID() || undefined

  const buildAuthPayload = (pluginId?: string): PowerXToPlugin | null => {
    if (!process.client) return null
    const accessToken = localStorage.getItem('access_token') || ''
    const refreshToken = localStorage.getItem('refresh_token') || ''
    const tokenType = localStorage.getItem('token_type') || 'Bearer'
    const expiresAtRaw = Number(localStorage.getItem('expires_at') || 0)
    const scope = localStorage.getItem('scope') || undefined
    const now = Date.now()
    const expiresIn =
      expiresAtRaw && expiresAtRaw > now
        ? Math.max(1, Math.floor((expiresAtRaw - now) / 1000))
        : 0
    const tenantUuid = resolveTenantUUID()

    if (!accessToken || expiresIn <= 0) {
      log('skip auth-token broadcast: missing or expired', { pluginId, expiresAt: expiresAtRaw })
      return null
    }

    const payload = {
      source: 'powerx',
      type: 'auth-token',
      accessToken,
      refreshToken: refreshToken || undefined,
      tokenType,
      expiresIn,
      expiresAt: expiresAtRaw || undefined,
      scope,
      pluginId,
      hostOrigin: typeof window !== 'undefined' ? window.location.origin : undefined,
      tenantUuid,
      tenant_uuid: tenantUuid,
    }
    log('prepared auth-token', { pluginId, token: maskToken(accessToken), expiresIn })
    return payload
  }

  const sendAuthToken = (meta: PluginMeta) => {
    const payload = buildAuthPayload(meta.pluginId)
    if (payload) {
      log('broadcast auth-token', { pluginId: meta.pluginId })
      sendTo(meta, payload)
    } else {
      log('auth-token payload empty', { pluginId: meta.pluginId })
    }
  }

  const broadcastAuthToken = () => {
    if (!process.client) return
    log('broadcast auth-token to all iframes', { count: registry.value.size })
    registry.value.forEach(meta => sendAuthToken(meta))
  }

  const syncFrame = (frame?: HTMLIFrameElement | null) => {
    if (!process.client) return
    if (frame) {
      const m = registry.value.get(frame)
      if (m) {
        syncMeta(m)
        sendAuthToken(m)
        const pendingPath = pendingNavigate.value.get(frame)
        if (pendingPath) {
          sendTo(m, { source: 'powerx', type: 'navigate', path: pendingPath })
        }
      }
    } else {
      registry.value.forEach(m => {
        syncMeta(m)
        sendAuthToken(m)
        const pendingPath = pendingNavigate.value.get(m.frame)
        if (pendingPath) {
          sendTo(m, { source: 'powerx', type: 'navigate', path: pendingPath })
        }
      })
    }
  }

  const navigateFrame = (frame?: HTMLIFrameElement | null, path?: string) => {
    if (!process.client || !frame || !path) return
    pendingNavigate.value.set(frame, path)
    const m = registry.value.get(frame)
    if (m) {
      sendTo(m, { source: 'powerx', type: 'navigate', path })
      return
    }
    try {
      frame.contentWindow?.postMessage({ source: 'powerx', type: 'navigate', path }, '*')
    } catch {
      // ignore
    }
  }

  // 语言/主题变化即广播
  if (process.client) {
    watch(locale, (val) => {
      broadcast({source: 'powerx', type: 'locale', locale: toLocaleCode(val)})
    })
    watch(
      themePreference,
      (pref) => {
        broadcast({ source: 'powerx', type: 'theme', theme: pref })
      },
      { immediate: true }
    )
    watch(
      resolvedTheme,
      (_val, _prev) => {
        if (themePreference.value === 'system') {
          broadcast({ source: 'powerx', type: 'theme', theme: 'system' })
        }
      }
    )
    watch(
      () => auth.token.value,
      () => broadcastAuthToken()
    )
    watch(
      () => currentTenantUuid.value,
      () => broadcastAuthToken()
    )
  }

  const register = (el?: HTMLIFrameElement | null, meta?: { pluginId: string; instanceId?: string }) => {
    if (!el || !process.client) return
    const origin = '*'
    const m: PluginMeta = {
      pluginId: meta?.pluginId || 'unknown',
      instanceId: meta?.instanceId,
      targetOrigin: origin,
      frame: el
    }
    registry.value.set(el, m)

    // 不再写 byWindow；只做首包 sync
    el.addEventListener('load', () => syncFrame(el), {once: true})
    // 注册后立即尝试一次，避免某些情况下 load 未触发或 token 已存在
    syncFrame(el)
  }

  const unregister = (el?: HTMLIFrameElement | null) => {
    if (!el || !process.client) return
    registry.value.delete(el)
    pendingNavigate.value.delete(el)
  }

  if (process.client && !(window as any).__pxBridgeBound__) {
    window.addEventListener('message', (e: MessageEvent) => {
      const data = e.data as PluginToPowerX
      if (!data || data.source !== 'plugin') return
      // 关键日志：记录插件->宿主的 ready/request-sync
      log('from plugin', { origin: e.origin, data })
      let hit: PluginMeta | undefined
      registry.value.forEach(meta => {
        if (meta.frame?.contentWindow === e.source) hit = meta
      })
      if (!hit) return
      if (data.type === 'ready' || data.type === 'request-sync') {
        // 插件显式告知 ready/request-sync 时，补一次完整同步（含 auth-token）
        log('sync on plugin msg', { type: data.type, pluginId: hit.pluginId, target: hit.targetOrigin })
        syncFrame(hit.frame)
      }
    })
    ;(window as any).__pxBridgeBound__ = true
  }

  return {register, unregister, syncFrame, navigateFrame, broadcast}
}
