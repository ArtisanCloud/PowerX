import { test, expect } from './fixtures/authenticatedTest'

test.describe('反馈闭环', () => {
  const spaceId = 'space-feedback'

  test('提交反馈、SLA 倒计时、升级与回滚', async ({ page }) => {
    let capturedPayload: any = null
    let actionCalls = {
      escalate: 0,
      close: 0,
      reprocess: 0,
      rollback: 0,
      export: 0,
    }

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
                traceId: 'trace-001',
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
              traceId: capturedPayload.toolTraceRef,
              createdAt: new Date().toISOString(),
              updatedAt: new Date().toISOString(),
            },
          }),
        })
        return
      }

      await route.fallback()
    })

    await page.route('**/api/admin/knowledge-spaces/**/feedback/**/escalate', async route => {
      actionCalls.escalate++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            caseId: 'case-001',
            status: 'escalated',
            severity: 'high',
            issueType: 'accuracy',
            linkedChunks: ['chunk-1'],
            reportedBy: 'ops@powerx.io',
            slaDueAt: new Date().toISOString(),
            qualityScore: 0.8,
            traceId: 'trace-001',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            escalatedAt: new Date().toISOString(),
          },
        }),
      })
    })

    await page.route('**/api/admin/knowledge-spaces/**/feedback/**/rollback', async route => {
      actionCalls.rollback++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            caseId: 'case-001',
            status: 'closed',
            severity: 'high',
            issueType: 'accuracy',
            linkedChunks: ['chunk-1'],
            reportedBy: 'ops@powerx.io',
            slaDueAt: new Date().toISOString(),
            qualityScore: 0.8,
            traceId: 'trace-001',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            closedAt: new Date().toISOString(),
            resolutionNotes: 'rollback: reverted',
          },
        }),
      })
    })

    await page.route('**/api/admin/knowledge-spaces/**/feedback/**/close', async route => {
      actionCalls.close++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            caseId: 'case-001',
            status: 'closed',
            severity: 'high',
            issueType: 'accuracy',
            linkedChunks: ['chunk-1'],
            reportedBy: 'ops@powerx.io',
            slaDueAt: new Date().toISOString(),
            qualityScore: 0.8,
            traceId: 'trace-001',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            closedAt: new Date().toISOString(),
          },
        }),
      })
    })

    await page.route('**/api/admin/knowledge-spaces/**/feedback/**/reprocess', async route => {
      actionCalls.reprocess++
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            caseId: 'case-001',
            status: 'in_progress',
            severity: 'high',
            issueType: 'accuracy',
            linkedChunks: ['chunk-1'],
            reportedBy: 'ops@powerx.io',
            slaDueAt: new Date().toISOString(),
            qualityScore: 0.8,
            traceId: 'trace-001',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          },
        }),
      })
    })

    await page.route('**/api/admin/knowledge-spaces/**/feedback/export**', async route => {
      actionCalls.export++
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            cases: [],
            audits: [],
            meta: { exported_at: new Date().toISOString() },
          },
        }),
      })
    })

    await page.goto('/knowledge-spaces/feedback')
    await page.getByLabel(/空间 ID/i).fill(spaceId)
    await page.getByRole('button', { name: /加载反馈列表/i }).click()
    await expect(page.getByText(/case-001/i)).toBeVisible()
    await expect(page.getByText(/案例详情/i)).toBeVisible()
    await expect(page.getByText(/SLA/i)).toBeVisible()

    await page.getByLabel(/关联 Chunk ID/i).fill('chunk-123')
    await page.getByRole('button', { name: /提交反馈/i }).click()
    await expect(page.getByText(/已进入再加工/i)).toBeVisible()
    expect(capturedPayload?.linkedChunks).toContain('chunk-123')

    await page.getByLabel(/操作人/i).fill('sre@powerx.local')
    await page.getByLabel(/处理记录/i).fill('reverted')

    await page.getByRole('button', { name: /升级/i }).click()
    await page.getByRole('button', { name: /回滚/i }).click()
    await page.getByRole('button', { name: /一键 Reprocess/i }).click()
    await page.getByRole('button', { name: /导出/i }).click()

    expect(actionCalls.escalate).toBeGreaterThan(0)
    expect(actionCalls.rollback).toBeGreaterThan(0)
    expect(actionCalls.reprocess).toBeGreaterThan(0)
    expect(actionCalls.export).toBeGreaterThan(0)
  })
})
