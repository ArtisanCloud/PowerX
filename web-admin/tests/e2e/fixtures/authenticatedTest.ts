import { test as base, expect } from '@playwright/test'

type UserContext = {
  is_root: boolean
  current_tenant_uuid: string
  current_member_id: number
  user: {
    id: number
    email: string
    phone: string
    display_name: string
    avatar_url: string
    status: number
  }
  members: Array<{
    tenant_uuid: string
    tenant_name: string
    member_id: number
    is_admin: boolean
  }>
}

const defaultContext: UserContext = {
  is_root: true,
  current_tenant_uuid: 'px-root-tenant',
  current_member_id: 1001,
  user: {
    id: 1,
    email: 'admin@powerx.io',
    phone: '+1-555-0000',
    display_name: 'Playwright Admin',
    avatar_url: '',
    status: 1,
  },
  members: [
    {
      tenant_uuid: 'px-root-tenant',
      tenant_name: 'PowerX Root Tenant',
      member_id: 1001,
      is_admin: true,
    },
  ],
}

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript(({ session }) => {
      const expiresAt = Date.now() + 60 * 60 * 1000
      window.localStorage.setItem('token', session.token)
      window.localStorage.setItem('access_token', session.token)
      window.localStorage.setItem('refresh_token', 'playwright-refresh')
      window.localStorage.setItem('token_type', 'Bearer')
      window.localStorage.setItem('expires_in', (60 * 60).toString())
      window.localStorage.setItem('expires_at', expiresAt.toString())
      window.localStorage.setItem('user', JSON.stringify(session.user))
      document.cookie = `token=${session.token}; path=/; SameSite=Lax`
    }, {
      session: {
        token: 'playwright-token',
        user: {
          id: defaultContext.user.id,
          email: defaultContext.user.email,
          role: 'admin',
        },
      },
    })

    await page.route('**/api/admin/auth/me/**', async route => {
      const url = new URL(route.request().url())

      if (url.pathname.endsWith('/context')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ data: defaultContext }),
        })
        return
      }

      if (url.pathname.endsWith('/switch-tenant')) {
        let nextContext = defaultContext
        try {
          const body = await route.request().postDataJSON()
          const target = body?.tenant_uuid || body?.tenantUuid
          if (target) {
            nextContext = {
              ...defaultContext,
              current_tenant_uuid: target,
            }
          }
        } catch (err) {
          console.warn('[playwright] failed to parse switch-tenant payload', err)
        }

        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ data: nextContext }),
        })
        return
      }

      if (url.pathname.endsWith('/tenants')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ data: defaultContext.members }),
        })
        return
      }

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: {} }),
      })
    })

    await use(page)
  },
})

export { expect }
