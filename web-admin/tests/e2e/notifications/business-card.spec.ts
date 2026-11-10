import { writeFile } from 'node:fs/promises'
import { test, expect } from '@playwright/test'
test.use({ storageState: { cookies: [], origins: [] } })
const tenantUuid = '11111111-1111-4111-8111-111111111111'
const id = '33333333-3333-4333-8333-333333333333'
test.beforeEach(async ({ request }) => { await request.delete('/api/v1/__notification_fixture') })
for (const narrow of [false, true]) {
  test(`business notifications: ${narrow ? '390px' : 'desktop'} preview, refresh, duplicate and revoke`, async ({ browser, baseURL, request }, info) => {
    const expiresAt = Date.now() + 3600000
    const session = { version: 1, sessionId: 'card-browser-session', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({tid: tenantUuid, exp: Math.floor(expiresAt / 1000)})).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid }
    const context = await browser.newContext({ baseURL, viewport: narrow ? {width: 390, height: 844} : {width: 1440, height: 1000}, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'welcome_guide_completed', value: 'true' }, { name: 'powerx.session', value: JSON.stringify(session) }] }] } })
    const page = await context.newPage()
    const downloads: any[] = []; const pendingDownloads: Promise<void>[] = []
    page.on('response', response => {
      if (!response.url().includes('/_p/plugin.cards/') || !response.url().includes('/images/')) return
      pendingDownloads.push((async () => { try {
        await response.finished(); const sizes = await response.request().sizes()
        downloads.push({path: new URL(response.url()).pathname, status: response.status(), ...sizes, failure: response.request().failure()})
      } catch {} })())
    })
    const errors: string[] = []; const consoleMessages: any[] = []; page.on('pageerror', e => errors.push(e.message)); page.on('console', message => { if (['error', 'warning'].includes(message.type())) consoleMessages.push({type: message.type(), text: message.text()}) })
    const state = async () => (await (await request.get('/api/v1/__notification_fixture')).json())
    const action = async (action: string) => request.post('/api/v1/__notification_fixture', {data: {action}})
    const closeSidebar = async () => {
      await expect(page.getByText('启动中…', {exact: true})).toBeHidden()
      if (!narrow) return
      const collapse = page.getByRole('complementary').getByRole('button', {name: '收起', exact: true}).first()
      const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click()
    }
    try {
      await page.goto('/notifications')
      const skip = page.getByRole('button', {name: '跳过引导', exact: true}); if (await skip.isVisible()) await skip.click()
      await closeSidebar()
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v1'})).toBeVisible()
      await expect(page.getByTestId('notification-business-card').filter({hasText: '大单 B-508 · v1'})).toBeVisible()
      await page.locator('header').getByRole('button', {name: '查看所有通知', exact: true}).click()
      const bell = page.getByTestId('notification-bell'); const card = bell.getByTestId('notification-business-card').filter({hasText: '小单'})
      await expect(card.locator('img')).toBeVisible()
      await bell.evaluate(async element => { await Promise.all(element.getAnimations({subtree: true}).map(animation => animation.finished.catch(() => {}))) })
      const imageSize = await card.locator('img').boundingBox(); expect(imageSize!.width).toBeCloseTo(48, 1); expect(imageSize!.height).toBeCloseTo(48, 1)
      const thumbnailImage = await card.locator('img').evaluate(async (img: HTMLImageElement) => ({width:img.naturalWidth,height:img.naturalHeight,bytes:(await (await fetch(img.src)).blob()).size}))
      expect(thumbnailImage).toEqual({width:64,height:36,bytes:208})
      expect((await state()).requests.filter((r: any) => r.path.includes('/images/preview-'))).toHaveLength(0)
      await card.getByRole('button', {name: '放大图片', exact: true}).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await expect(page.getByRole('dialog').locator('img')).toBeVisible()
      const previewImage = await page.getByRole('dialog').locator('img').evaluate(async (img: HTMLImageElement) => ({width:img.naturalWidth,height:img.naturalHeight,bytes:(await (await fetch(img.src)).blob()).size}))
      expect(previewImage).toEqual({width:1600,height:900,bytes:6993})
      await page.getByRole('dialog').evaluate(async element => { await Promise.all(element.getAnimations({subtree: true}).map(animation => animation.finished.catch(() => {}))) })
      expect((await state()).items.find((n: any) => n.id === id).isRead).toBe(false)
      expect(new URL(page.url()).pathname).toBe('/notifications')
      await page.screenshot({path: info.outputPath(`card-preview-${narrow ? 'narrow' : 'desktop'}.png`)})
      await page.keyboard.press('Escape'); await page.keyboard.press('Escape')
      await action('advance')
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v2'}).first()).toBeVisible()
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v2'}).first().locator('img')).toBeVisible()
      const beforeDuplicate = await state(); await action('duplicate'); await page.waitForTimeout(200)
      expect((await state()).requests.length).toBe(beforeDuplicate.requests.length)
      const idle = (await state()).requests.length; await page.waitForTimeout(1200); expect((await state()).requests.length).toBe(idle)
      const beforeReconnect = (await state()).requests.filter((r: any) => r.path.endsWith('/presentation')).length
      await action('disconnect'); await expect.poll(async () => (await state()).subscribers).toBeGreaterThan(0)
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v2'}).first()).toBeVisible()
      await expect.poll(async () => (await state()).requests.filter((r: any) => r.path.endsWith('/presentation')).length).toBeGreaterThan(beforeReconnect)
      await action('revoke')
      await expect(page.getByTestId('notification-business-card').filter({hasText: '你已无权查看此内容'}).first()).toBeVisible()
      await expect(page.getByText('小单 A-102 · v2')).toHaveCount(0)
      await action('grant')
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v4'}).first()).toBeVisible()
      await page.goto(`/notifications?id=${id}`)
      await expect(page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v4'}).last()).toBeVisible()
      await closeSidebar()
      expect((await state()).items.find((n: any) => n.id === id).isRead).toBe(true)
      const snapshot = await state(); const pluginRequests = snapshot.requests.filter((r: any) => r.path.startsWith('/_p/'))
      expect(pluginRequests.length).toBeGreaterThan(0); expect(pluginRequests.every((r: any) => r.authenticated && !r.query)).toBe(true)
      expect(snapshot.items).toHaveLength(2)
      await page.screenshot({path: info.outputPath(`card-detail-${narrow ? 'narrow' : 'desktop'}.png`)})
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
      await page.getByTestId('notification-business-card').filter({hasText: '小单 A-102 · v4'}).last().getByRole('button', {name: '查看详情', exact: true}).click()
      await expect(page).toHaveURL(/\/profile\?order=sample_track$/)
      await Promise.all(pendingDownloads)
      await writeFile(info.outputPath('network-and-console.json'), JSON.stringify({thumbnailImage, previewImage, downloads, requests: (await state()).requests, console: consoleMessages, pageErrors: errors}, null, 2))
      expect(downloads.filter(d => d.status === 200 && d.path.includes('/images/thumb-')).every(d => d.responseBodySize <= 32768)).toBe(true)
      expect(downloads.filter(d => d.path.includes('/images/preview-'))).toHaveLength(1)
      expect(errors).toEqual([])
    } finally { await context.close() }
  })
}

for (const count of [20, 50]) {
  test(`v2 ${count} cards fetch only visible small images until preview is requested`, async ({browser,baseURL,request}, info) => {
    await request.post('/api/v1/__notification_fixture',{data:{action:`rows-${count}`}})
    const expiresAt=Date.now()+3600000
    const session={version:1,sessionId:'card-size-session',revision:1,status:'active',accessToken:`header.${Buffer.from(JSON.stringify({tid:tenantUuid,exp:Math.floor(expiresAt/1000)})).toString('base64url')}.fixture`,refreshToken:'fixture',tokenType:'Bearer',expiresAt,scope:'access',tenantUuid}
    const context=await browser.newContext({baseURL,viewport:{width:1440,height:1000},storageState:{cookies:[],origins:[{origin:baseURL!,localStorage:[{name:'welcome_guide_completed',value:'true'},{name:'powerx.session',value:JSON.stringify(session)}]}]}})
    try {
      const page=await context.newPage();await page.goto('/notifications')
      await expect(page.getByText('启动中…',{exact:true})).toBeHidden()
      await expect(page.getByTestId('notification-business-card')).toHaveCount(count)
      const first=page.getByTestId('notification-business-card').first()
      await expect(first.locator('img')).toBeVisible()
      await page.waitForTimeout(250)
      const before=await (await request.get('/api/v1/__notification_fixture')).json()
      const small=before.requests.filter((r:any)=>r.path.includes('/images/thumb-'))
      expect(small.length).toBeGreaterThan(0);expect(small.length).toBeLessThan(count)
      expect(before.requests.filter((r:any)=>r.path.includes('/images/preview-'))).toHaveLength(0)
      const pixels=await first.locator('img').evaluate((img:HTMLImageElement)=>({width:img.naturalWidth,height:img.naturalHeight}))
      expect(pixels).toEqual({width:64,height:36})
      const idle=before.requests.length;await page.waitForTimeout(1000)
      expect((await (await request.get('/api/v1/__notification_fixture')).json()).requests.length).toBe(idle)
      const measured=await page.evaluate(()=>performance.getEntriesByType('resource').filter(e=>e.name.includes('/_p/plugin.cards/') && e.name.includes('/images/')).map(e=>({url:e.name,encodedBodySize:(e as PerformanceResourceTiming).encodedBodySize,transferSize:(e as PerformanceResourceTiming).transferSize})))
      await writeFile(info.outputPath('visible-image-downloads.json'),JSON.stringify({count,requestedSmallImages:small.length,pixels,measured},null,2))
      await page.screenshot({path:info.outputPath(`visible-cards-${count}.png`)})
    } finally {await context.close()}
  })
}
