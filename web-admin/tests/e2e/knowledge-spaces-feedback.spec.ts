import { test, expect } from './fixtures/authenticatedTest'

test.describe('反馈闭环', () => {
  const spaceId = 'space-feedback'

  test('提交反馈并查看列表', async ({ page }) => {
    let capturedPayload: any = null

    await page.route('**/api/admin/knowledge-spaces/**/feedback', async route => {
      const url = route.request().url()
      if (route.request().method() === 'GET') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [
              {
                caseId: 'case-001',
                status: 'in_progress',
                severity: 'high',
                issueType: 'accuracy',
                linkedChunks: ['chunk-1'],
                reportedBy: 'ops@powerx.io',
                slaDueAt: new Date().toISOString(),
                qualityScore: 0.8,
                createdAt: new Date().toISOString(),
                updatedAt: new Date().toISOString(),
              },
            ],
          }),
        })
        return
      }

      if (route.request().method() === 'POST') {
        capturedPayload = await route.request().postDataJSON()
        await route.fulfill({
          status: 202,
          contentType: 'application/json',
          body: JSON.stringify({
            data: {
              caseId: 'case-new',
              status: 'in_progress',
              severity: capturedPayload.severity,
              issueType: capturedPayload.issueType,
              linkedChunks: capturedPayload.linkedChunks,
              reportedBy: capturedPayload.reportedBy,
              slaDueAt: new Date().toISOString(),
              qualityScore: 0.6,
              createdAt: new Date().toISOString(),
              updatedAt: new Date().toISOString(),
            },
          }),
        })
        return
      }

      await route.fallback()
    })

    await page.goto('/knowledge-spaces/feedback')
    await page.getByLabel(/空间 ID/i).fill(spaceId)
    await page.getByRole('button', { name: /加载反馈列表/i }).click()
    await expect(page.getByText(/case-001/i)).toBeVisible()

    await page.getByLabel(/关联 Chunk ID/i).fill('chunk-123')
    await page.getByRole('button', { name: /提交反馈/i }).click()
    await expect(page.getByText(/已进入再加工/i)).toBeVisible()
    expect(capturedPayload?.linkedChunks).toContain('chunk-123')
  })
})
