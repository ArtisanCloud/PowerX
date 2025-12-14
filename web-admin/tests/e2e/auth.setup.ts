import type { FullConfig } from '@playwright/test'
import { promises as fs } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const adminState = path.resolve(__dirname, './.auth/admin.json')

export default async function globalSetup(config: FullConfig) {
  const baseURL =
    (config.projects[0]?.use?.baseURL as string | undefined) ||
    process.env.PLAYWRIGHT_BASE_URL ||
    'http://127.0.0.1:3000'
  const origin = baseURL.endsWith('/') ? baseURL.slice(0, -1) : baseURL

  const expiresAt = Date.now() + 60 * 60 * 1000
  const storageState = {
    cookies: [
      {
        name: 'token',
        value: 'test-token',
        domain: '127.0.0.1',
        path: '/',
        httpOnly: false,
        sameSite: 'Lax' as const,
        expires: Math.floor(expiresAt / 1000),
      },
    ],
    origins: [
      {
        origin,
        localStorage: [
          { name: 'token', value: 'test-token' },
          { name: 'access_token', value: 'test-token' },
          { name: 'refresh_token', value: 'test-refresh-token' },
          { name: 'token_type', value: 'Bearer' },
          { name: 'expires_in', value: (60 * 60).toString() },
          { name: 'expires_at', value: expiresAt.toString() },
          { name: 'scope', value: 'admin' },
          {
            name: 'user',
            value: JSON.stringify({
              id: 1,
              email: 'admin@test.com',
              role: 'admin',
            }),
          },
        ],
      },
    ],
  }

  await fs.mkdir(path.dirname(adminState), { recursive: true })
  await fs.writeFile(adminState, JSON.stringify(storageState, null, 2), 'utf-8')
  console.log(`✅ Global auth storage prepared at ${adminState}`)
}
