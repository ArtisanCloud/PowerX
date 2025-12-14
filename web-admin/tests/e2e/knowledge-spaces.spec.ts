import { test, expect } from './fixtures/authenticatedTest'

test.describe('知识空间向导', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.localStorage.setItem('token', 'test-token')
      window.localStorage.setItem(
        'user',
        JSON.stringify({
          id: 42,
          email: 'ops@powerx.io',
          role: 'admin',
        }),
      )
    })
  })

  test('管理员可以通过多步骤向导创建空间并看到 IAM 待确认提示', async ({ page }) => {
    let capturedPayload: any = null

    await page.route('**/api/admin/knowledge-spaces', async route => {
      const body = await route.request().postDataJSON()
      capturedPayload = body
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 201,
          message: 'success',
          data: {
            spaceId: 'space-e2e-001',
            tenantUuid: body.tenantUuid,
            spaceName: body.spaceName,
            departmentCode: body.departmentCode,
            status: 'pending_iam',
            auditToken: 'audit-e2e',
            retentionExpiresAt: null,
            policyTemplateVersionId: body.policyTemplateVersionId,
            featureFlags: body.featureFlags,
            quotas: body.quotas,
          },
        }),
      })
    })

    await page.goto('/knowledge-spaces/create')

    await page.getByLabel(/租户 UUID/i).fill('d86c5da9-35f4-4db8-9c2e-d879ed2b9e10')
    await page.getByLabel(/空间名称/i).fill('ops-handbook')
    await page.getByLabel(/部门编码/i).fill('OPS-01')

    await page.getByRole('button', { name: /下一步/i }).click()

    await page.getByRole('combobox', { name: /策略模版/i }).selectOption('default-v1')
    await page.getByLabel(/启用掩码审计/i).check()

    await page.getByRole('button', { name: /下一步/i }).click()

    await page.getByLabel(/CPU 核心/i).fill('6')
    await page.getByLabel(/存储容量/i).fill('256')
    await page.getByLabel(/IAM 通知邮箱/i).fill('iam-alerts@powerx.io')

    await page.getByRole('button', { name: /提交创建/i }).click()

    await expect(page.getByText(/空间创建成功/i)).toBeVisible()
    await expect(page.getByText(/IAM 待确认/i)).toBeVisible()
    await expect(page.getByText(/SLA 计时/i)).toBeVisible()

    expect(capturedPayload).not.toBeNull()
    expect(capturedPayload.tenantUuid).toBe('d86c5da9-35f4-4db8-9c2e-d879ed2b9e10')
    expect(capturedPayload.spaceName).toBe('ops-handbook')
    expect(capturedPayload.quotas.cpuCores).toBe(6)
    expect(capturedPayload.featureFlags).toContain('masking.audit')
  })
})
