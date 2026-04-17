#!/usr/bin/env node
import { promises as fs } from 'node:fs'
import path from 'node:path'

async function ensureStub(dir) {
  const targetDir = path.resolve(process.cwd(), dir)
  const targetFile = path.join(targetDir, 'client.precomputed.mjs')
  const stub = 'export default () => ({ assets: {}, entries: [] })\n'

  try {
    await fs.access(targetFile)
    return
  } catch {
    await fs.mkdir(targetDir, { recursive: true })
    await fs.writeFile(targetFile, stub, 'utf8')
    console.info(`✅ 写入 Playwright stub: ${targetFile}`)
  }
}

async function main() {
  const dirs = [
    '.nuxt/dist/server',
    'node_modules/.cache/nuxt/.nuxt/dist/server',
  ]

  for (const dir of dirs) {
    await ensureStub(dir)
  }
}

main().catch((err) => {
  console.error('prepare-playwright.mjs 失败', err)
  process.exit(1)
})
