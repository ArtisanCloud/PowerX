#!/usr/bin/env node

import process from 'node:process'

const usage = `Usage: rollback_strategy.mjs <space-id> <strategy-id>

Environment variables:
  POWERX_BASE_URL   PowerX service base URL (default: http://127.0.0.1:8077)
  POWERX_API_BASE   Legacy alias for API base URL (e.g. http://127.0.0.1:8077/api/v1)
  POWERX_TOKEN      Bearer token used for authentication
`

async function main() {
  const [, , spaceId, strategyId] = process.argv
  if (!spaceId || !strategyId) {
    console.error(usage)
    process.exit(1)
  }

  const apiBase = normalizeApiBase(process.env.POWERX_API_BASE || process.env.POWERX_BASE_URL || '')
  const token = process.env.POWERX_TOKEN || ''
  const endpoint = `${apiBase}/admin/knowledge-spaces/${spaceId}/fusion-strategies/${strategyId}/rollback`

  const headers = { 'Content-Type': 'application/json' }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await fetch(endpoint, {
    method: 'POST',
    headers,
  })

  if (!response.ok) {
    const body = await response.text()
    console.error(`[fusion] rollback failed (${response.status}): ${body}`)
    process.exit(1)
  }

  const payload = await response.json()
  console.info('[fusion] rollback triggered:', JSON.stringify(payload.data ?? payload, null, 2))
}

function normalizeApiBase(raw) {
  const trimmed = String(raw || '').trim().replace(/\/$/, '')
  if (!trimmed) return 'http://127.0.0.1:8077/api/v1'
  if (trimmed.endsWith('/api/v1')) return trimmed
  if (trimmed.endsWith('/api')) return `${trimmed}/v1`
  if (trimmed.includes('/api/v1/')) return trimmed.replace(/\/$/, '')
  if (trimmed.includes('/api/')) return trimmed.replace(/\/$/, '')
  return `${trimmed}/api/v1`
}

main().catch(err => {
  console.error('[fusion] rollback script error:', err)
  process.exit(1)
})
