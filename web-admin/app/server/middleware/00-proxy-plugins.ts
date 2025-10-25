import { defineEventHandler, getRequestURL, proxyRequest, setResponseHeader } from 'h3'

// 你的后端端口（承接代理）
const BACKEND = process.env.POWERX_BACKEND || 'http://127.0.0.1:8077'

export default defineEventHandler(async (event) => {
  const u = getRequestURL(event)
  const p = u.pathname || '/'
  const qs = u.search || ''

  // 只要是 /_p/** ：“只代理，不渲染”到后端，避免画中画
  if (p.startsWith('/_p/')) {
    const target = new URL(p + qs, BACKEND).toString()
    setResponseHeader(event, 'x-px-proxy-target', target) // Network 可见真实后端
    setResponseHeader(event, 'x-nitro-no-render', '1')    // 禁止 Nuxt 包壳
    return proxyRequest(event, target)
  }
})
