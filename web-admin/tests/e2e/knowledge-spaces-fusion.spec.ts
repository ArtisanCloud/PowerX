import { test, expect } from './fixtures/authenticatedTest'

test.describe('融合策略管理', () => {
  const spaceId = 'space-e2e'

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.localStorage.setItem('token', 'test-token')
      window.localStorage.setItem(
        'user',
        JSON.stringify({
          id: 99,
          email: 'fusion-ops@powerx.io',
          role: 'admin',
        }),
      )
    })
  })

  test('发布策略并触发回滚', async ({ page }) => {
    let publishPayload: any = null
    let rollbackTarget: string | null = null

    await page.route('**/api/admin/knowledge-spaces/**/fusion-strategies', async route => {
      const url = route.request().url()
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [
              {
                strategyId: '1001',
                label: 'baseline',
                bm25Weight: 0.4,
                vectorWeight: 0.6,
                deploymentState: 'active',
                conflictPolicy: 'allow_with_flag',
                degraded: false,
                degradeReasons: [],
              },
              {
                strategyId: '1000',
                label: 'rolled-back',
                bm25Weight: 0.5,
                vectorWeight: 0.5,
                deploymentState: 'rollback',
                conflictPolicy: 'queue',
                degraded: false,
                degradeReasons: [],
              },
              {
                strategyId: '999',
                label: 'degraded-bm25-only',
                bm25Weight: 1,
                vectorWeight: 0,
                deploymentState: 'draft',
                conflictPolicy: 'queue',
                degraded: true,
                degradeReasons: ['vector_unavailable'],
              },
            ],
          }),
        })
        return
      }

      if (url.endsWith('/fusion-strategies')) {
        publishPayload = await route.request().postDataJSON()
        const queued = publishPayload.conflictPolicy === 'queue'
        await route.fulfill({
          status: queued ? 202 : 201,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              strategyId: queued ? '1003' : '1002',
              label: publishPayload.label,
              bm25Weight: publishPayload.bm25Weight,
              vectorWeight: publishPayload.vectorWeight,
              deploymentState: queued ? 'draft' : 'active',
              conflictPolicy: publishPayload.conflictPolicy ?? 'allow_with_flag',
              degraded: false,
              degradeReasons: [],
            },
          }),
        })
        return
      }

      if (url.includes('/rollback')) {
        rollbackTarget = url.split('/').slice(-2, -1)[0]
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              strategyId: rollbackTarget,
              label: 'baseline',
              bm25Weight: 0.4,
              vectorWeight: 0.6,
              deploymentState: 'active',
              conflictPolicy: 'allow_with_flag',
              degraded: false,
              degradeReasons: [],
            },
          }),
        })
        return
      }

      await route.fallback()
    })

    await page.goto('/knowledge-spaces/fusion')

    await page.getByLabel(/空间 ID/i).fill(spaceId)
    await page.getByRole('button', { name: /加载策略/i }).click()

    await expect(page.getByText(/baseline/i)).toBeVisible()
    await expect(page.getByText(/active/i)).toBeVisible()
    await expect(page.getByText(/已降级：vector_unavailable/i)).toBeVisible()

    await page.getByLabel(/策略名称/i).fill('weighted-search')
    await page.getByLabel(/BM25 权重/i).fill('0.3')
    await page.getByLabel(/向量权重/i).fill('0.7')
    await page.getByLabel(/图谱约束/i).fill('tenant:default')
    await page.getByLabel(/Reranker 模型/i).fill('cross-encoder-v1')
    await page.getByRole('button', { name: /发布策略/i }).click()

    await expect(page.getByText(/策略发布成功/i)).toBeVisible()
    expect(publishPayload).not.toBeNull()
    expect(publishPayload.label).toBe('weighted-search')

    await page.getByRole('button', { name: /回滚 baseline/i }).click()
    await expect(page.getByText(/回滚已触发/i)).toBeVisible()
    expect(rollbackTarget).toBe('1001')
  })

  test('排队发布时显示草稿提示', async ({ page }) => {
    await page.route('**/api/admin/knowledge-spaces/**/fusion-strategies', async route => {
      const url = route.request().url()
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ data: [] }),
        })
        return
      }
      if (url.endsWith('/fusion-strategies')) {
        const body = await route.request().postDataJSON()
        await route.fulfill({
          status: 202,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              strategyId: '2001',
              label: body.label,
              bm25Weight: body.bm25Weight,
              vectorWeight: body.vectorWeight,
              deploymentState: 'draft',
              conflictPolicy: body.conflictPolicy ?? 'queue',
              degraded: false,
              degradeReasons: [],
            },
          }),
        })
        return
      }
      await route.fallback()
    })

    await page.goto('/knowledge-spaces/fusion')
    await page.getByLabel(/空间 ID/i).fill(spaceId)
    await page.getByLabel(/策略名称/i).fill('queued-search')
    await page.getByLabel(/BM25 权重/i).fill('0.5')
    await page.getByLabel(/向量权重/i).fill('0.5')
    await page.getByLabel(/图谱约束/i).fill('tenant:default')
    await page.getByLabel(/Reranker 模型/i).fill('cross-encoder-v1')
    await page.getByLabel(/冲突策略/i).selectOption('queue')
    await page.getByRole('button', { name: /发布策略/i }).click()
    await expect(page.getByText(/策略已排队等待发布/i)).toBeVisible()
  })
})
