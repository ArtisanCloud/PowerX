import { test as setup, expect } from '@playwright/test'

const adminState = './tests/e2e/.auth/admin.json'

setup('authenticate as admin', async ({ page }) => {
  // 模拟登录（在实际项目中需要替换为真实的登录URL和凭据）
  await page.goto('/login')

  // 模拟输入登录信息
  await page.getByLabel(/邮箱|identifier/i).fill('admin@test.com')
  await page.getByLabel(/密码|password/i).fill('password123')

  // 点击登录按钮
  await page.getByRole('button', { name: /登录|登入|sign in/i }).click()

  // 等待跳转到仪表盘
  await page.waitForURL('/**', { timeout: 10000 }).catch(() => {})

  // 保存认证状态
  await page.context().storageState({ path: adminState })

  console.log('✅ Admin authentication completed')
})
