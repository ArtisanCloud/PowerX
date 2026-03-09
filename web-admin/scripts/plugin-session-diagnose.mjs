import { chromium } from '@playwright/test'

const HOST_URL = process.env.HOST_URL || 'http://localhost:3030'
const PLUGIN_URL =
  process.env.PLUGIN_URL ||
  'http://127.0.0.1:8077/_p/com.powerx.helloworld/admin/intro'
const AUTH_PATH = process.env.AUTH_PATH || '/api/v1/admin/user/auth/me/context'
const HEADLESS = process.env.HEADLESS !== '0'
const AUTO_CONTINUE = process.env.AUTO_CONTINUE === '1'
const WAIT_AFTER_PLUGIN_MS = Number(process.env.WAIT_AFTER_PLUGIN_MS || 5000)

const authCalls = []

const browser = await chromium.launch({ headless: HEADLESS })
const context = await browser.newContext({ ignoreHTTPSErrors: true })
const page = await context.newPage()

context.on('requestfinished', async request => {
  if (!request.url().includes(AUTH_PATH)) return

  const response = await request.response()
  const headers = request.headers()
  const cookieHeader = headers.cookie || headers.Cookie
  const responseHeaders = response ? await response.allHeaders() : {}

  authCalls.push({
    url: request.url(),
    method: request.method(),
    status: response?.status(),
    cookieHeader,
    wwwAuthenticate: responseHeaders['www-authenticate'],
    contentType: responseHeaders['content-type'],
  })
})

context.on('requestfailed', request => {
  if (!request.url().includes(AUTH_PATH)) return
  authCalls.push({
    url: request.url(),
    method: request.method(),
    status: 'failed',
    cookieHeader: request.headers().cookie,
  })
})

console.log('打开宿主地址以复用同一浏览器会话:', HOST_URL)
await page.goto(HOST_URL, { waitUntil: 'domcontentloaded' })

if (!AUTO_CONTINUE) {
  await waitForEnter(
    '如需登录宿主，请在已打开的浏览器里完成登录后按回车继续...'
  )
}

console.log('跳转到插件 iframe 页:', PLUGIN_URL)
let pluginResponseStatus = null
try {
  const res = await page.goto(PLUGIN_URL, { waitUntil: 'domcontentloaded' })
  pluginResponseStatus = res?.status() ?? null
} catch (error) {
  console.error('加载插件页失败:', error?.message || error)
}

await page.waitForTimeout(WAIT_AFTER_PLUGIN_MS)

const hostCookies = await context.cookies(HOST_URL)
const pluginCookies = await context.cookies(PLUGIN_URL)
const pluginDocumentCookie = await page.evaluate(() => document.cookie)

console.log('\n=== auth/me/context 请求捕获 ===')
if (!authCalls.length) {
  console.log('未捕获到匹配请求，请确认插件前端是否会调用该接口。')
} else {
  authCalls.forEach((call, index) => {
    console.log(
      `#${index + 1} ${call.method} ${call.url} -> ${call.status ?? 'n/a'}`
    )
    console.log('  Cookie 头:', call.cookieHeader || '<空>')
    console.log('  Content-Type:', call.contentType || '<空>')
    console.log('  WWW-Authenticate:', call.wwwAuthenticate || '<空>')
  })
}

console.log('\n=== Cookie 对比 ===')
console.log('宿主域 Cookie:', formatCookies(hostCookies))
console.log('插件域 Cookie:', formatCookies(pluginCookies))
console.log('插件 document.cookie:', pluginDocumentCookie || '<空>')
console.log('插件页面初始响应状态:', pluginResponseStatus ?? '<未知>')

await browser.close()
process.exit(0)

function formatCookies(cookies) {
  if (!cookies?.length) return '<空>'
  return cookies
    .map(
      cookie =>
        `${cookie.name}=${cookie.value}; Domain=${cookie.domain}; Path=${cookie.path}`
    )
    .join(' | ')
}

function waitForEnter(message) {
  return new Promise(resolve => {
    process.stdin.setEncoding('utf-8')
    process.stdin.resume()
    process.stdout.write(`${message}\n`)
    process.stdin.once('data', () => resolve())
  })
}
