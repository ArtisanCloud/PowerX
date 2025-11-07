import { test, expect } from '@playwright/test'

/**
 * 插件发布功能 E2E 测试
 *
 * 测试流程：
 * 1. 登录并进入离线包入库页面
 * 2. 提交离线包
 * 3. 查看Marketplace审核列表
 * 4. 审核Listing
 * 5. 查看审核详情
 */

test.describe('插件发布功能', () => {
  test.beforeEach(async ({ page }) => {
    // 设置测试数据
    await page.addInitScript(() => {
      window.localStorage.setItem('token', 'test-token')
      window.localStorage.setItem(
        'user',
        JSON.stringify({
          id: 1,
          email: 'admin@test.com',
          role: 'admin'
        })
      )
    })
  })

  test.describe('离线包入库', () => {
    test('应该能够提交离线包', async ({ page }) => {
      // 模拟API响应
      await page.route('**/api/admin/plugin-release/offline-packages', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 1,
            status: 'pending',
            checksum: 'test-checksum',
            auditId: 'audit-123'
          })
        })
      })

      // 访问页面
      await page.goto('/admin/plugin-release/offline-packages')

      // 验证页面元素
      await expect(page.getByText('离线包入库')).toBeVisible()
      await expect(page.getByText('提交离线包')).toBeVisible()

      // 填写表单
      await page.getByLabel(/发布候选ID|candidate/i).fill('candidate-123')
      await page.getByLabel(/校验和|checksum/i).fill('abc123def456')
      await page.getByLabel(/包体 URI|package/i).fill('s3://bucket/package.pxp')

      // 提交表单
      await page.getByRole('button', { name: /提交审核/i }).click()

      // 验证提交成功
      await expect(page.getByText('提交成功')).toBeVisible()
      await expect(page.getByText(/审计参考|audit/i)).toBeVisible()
    })

    test('应该验证必填字段', async ({ page }) => {
      await page.goto('/admin/plugin-release/offline-packages')

      // 尝试不填写必填字段提交
      await page.getByRole('button', { name: /提交审核/i }).click()

      // 验证浏览器原生验证
      const candidateInput = page.getByLabel(/发布候选ID|candidate/i)
      const checksumInput = page.getByLabel(/校验和|checksum/i)

      await expect(candidateInput).toHaveAttribute('required')
      await expect(checksumInput).toHaveAttribute('required')
    })
  })

  test.describe('Marketplace审核列表', () => {
    test('应该显示审核列表', async ({ page }) => {
      // 模拟有数据的API响应
      await page.route('**/api/admin/plugin-release/marketplace/listings*', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [
              {
                id: 1,
                offlinePackageId: 123,
                channel: 'online',
                reviewStatus: 'pending',
                reviewCount: 0,
                createdAt: new Date().toISOString(),
                slaDeadline: new Date(Date.now() + 48 * 60 * 60 * 1000).toISOString()
              },
              {
                id: 2,
                offlinePackageId: 124,
                channel: 'offline',
                reviewStatus: 'approved',
                reviewCount: 1,
                createdAt: new Date().toISOString()
              }
            ],
            total: 2,
            page: 1,
            size: 20
          })
        })
      })

      // 访问列表页
      await page.goto('/admin/plugin-release/marketplace')

      // 验证页面元素
      await expect(page.getByText('Marketplace 审核列表')).toBeVisible()
      await expect(page.getByText('ID')).toBeVisible()
      await expect(page.getByText('状态')).toBeVisible()

      // 验证数据行
      const table = page.getByRole('table')
      await expect(table).toBeVisible()

      // 验证至少有一行数据
      const rows = page.locator('tbody tr')
      await expect(rows.first()).toBeVisible()
    })

    test('应该支持筛选功能', async ({ page }) => {
      await page.route('**/api/admin/plugin-release/marketplace/listings*', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [
              {
                id: 1,
                reviewStatus: 'pending',
                reviewCount: 0,
                createdAt: new Date().toISOString()
              }
            ],
            total: 1,
            page: 1,
            size: 20
          })
        })
      })

      await page.goto('/admin/plugin-release/marketplace')

      // 选择状态筛选
      const statusSelect = page.getByLabel(/状态|status/i)
      await statusSelect.selectOption('pending')

      // 等待请求发送
      const [request] = await Promise.all([
        page.waitForRequest(request =>
          request.url().includes('marketplace/listings') &&
          request.url().includes('status=pending')
        ),
        page.getByRole('button', { name: /刷新|refresh/i }).click()
      ])

      // 验证筛选参数
      expect(request.url()).toContain('status=pending')
    })
  })

  test.describe('审核操作', () => {
    test('应该能够审核Listing', async ({ page }) => {
      // 模拟列表数据
      await page.route('**/api/admin/plugin-release/marketplace/listings*', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [
              {
                id: 1,
                reviewStatus: 'pending',
                reviewCount: 0,
                createdAt: new Date().toISOString()
              }
            ],
            total: 1,
            page: 1,
            size: 20
          })
        })
      })

      await page.goto('/admin/plugin-release/marketplace')

      // 点击审核按钮
      await page.getByRole('button', { name: /审核|review/i }).click()

      // 等待模态框出现
      await expect(page.getByText('审核操作')).toBeVisible()

      // 选择审核结果
      await page.getByLabel(/审核结果|decision/i).selectOption('approved')
      await page.getByLabel(/审核意见|comment/i).fill('审核通过')

      // 模拟审核API
      await page.route('**/api/admin/plugin-release/marketplace/listings/1/reviews*', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 1,
            reviewStatus: 'approved',
            reviewCount: 1
          })
        })
      })

      // 提交审核
      await page.getByRole('button', { name: /提交审核/i }).click()

      // 验证成功提示
      await expect(page.getByText('审核成功')).toBeVisible()
    })
  })

  test.describe('审核详情页', () => {
    test('应该显示详细信息', async ({ page }) => {
      // 模拟详情API
      await page.route('**/api/admin/plugin-release/marketplace/listings/1', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 1,
            offlinePackageId: 123,
            channel: 'online',
            reviewStatus: 'pending',
            reviewCount: 0,
            createdAt: new Date().toISOString(),
            slaDeadline: new Date(Date.now() + 48 * 60 * 60 * 1000).toISOString(),
            pricing: { tier: 'enterprise', price: 999 },
            supportPolicy: { sla: '24x7', responseTime: '1h' }
          })
        })
      })

      // 访问详情页
      await page.goto('/admin/plugin-release/marketplace/1')

      // 验证页面元素
      await expect(page.getByText(/审核详情/i)).toBeVisible()
      await expect(page.getByText('基本信息')).toBeVisible()
      await expect(page.getByText('ID')).toBeVisible()

      // 验证数据显示
      await expect(page.getByText('1')).toBeVisible()
      await expect(page.getByText('online')).toBeVisible()
    })

    test('应该显示SLA倒计时', async ({ page }) => {
      await page.route('**/api/admin/plugin-release/marketplace/listings/1', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 1,
            reviewStatus: 'pending',
            slaDeadline: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString()
          })
        })
      })

      await page.goto('/admin/plugin-release/marketplace/1')

      await expect(page.getByText(/SLA 监控/i)).toBeVisible()
      await expect(page.getByText(/剩余/i)).toBeVisible()
    })

    test('应该能够返回列表页', async ({ page }) => {
      await page.goto('/admin/plugin-release/marketplace/1')

      // 点击返回按钮
      await page.getByRole('button', { name: /返回|back/i }).click()

      // 验证跳转
      await expect(page).toHaveURL(/.*\/marketplace$/)
    })
  })

  test.describe('导航测试', () => {
    test('应该能够在页面间导航', async ({ page }) => {
      // 从离线包页到审核列表
      await page.goto('/admin/plugin-release/offline-packages')
      await page.getByRole('link', { name: /审核/i }).click()
      await expect(page).toHaveURL(/.*\/marketplace$/)

      // 从审核列表到详情页
      await page.route('**/api/admin/plugin-release/marketplace/listings*', async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: [{ id: 1, reviewStatus: 'pending', reviewCount: 0, createdAt: new Date().toISOString() }],
            total: 1,
            page: 1,
            size: 20
          })
        })
      })

      await page.getByRole('button', { name: /详情/i }).click()
      await expect(page).toHaveURL(/.*\/marketplace\/1$/)

      // 返回列表
      await page.getByRole('button', { name: /返回|back/i }).click()
      await expect(page).toHaveURL(/.*\/marketplace$/)
    })
  })
})
