import { defineEventHandler, getHeader, setCookie } from 'h3'

// 将后端注入的上下文头转存为 cookie，便于前端桥接读取
export default defineEventHandler((event) => {
  const ctx = getHeader(event, 'x-powerx-ctx')
  const sig = getHeader(event, 'x-powerx-ctx-sig')
  const jwt = getHeader(event, 'x-powerx-ctx-jwt')

  const opts = {
    path: '/',
    httpOnly: false, // 需要前端读取
    sameSite: 'lax' as const
  }
  if (ctx) setCookie(event, 'px_ctx', ctx, opts)
  if (sig) setCookie(event, 'px_ctx_sig', sig, opts)
  if (jwt) setCookie(event, 'px_ctx_jwt', jwt, opts)
})
