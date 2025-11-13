#!/usr/bin/env node

import process from 'node:process'

const usage = `Usage: rollback_strategy.mjs <space-id> <strategy-id>

Environment variables:
  POWERX_API_BASE   Base API URL (default: http://127.0.0.1:8080/api)
  POWERX_TOKEN      Bearer token used for authentication
`

async function main() {
  const [, , spaceId, strategyId] = process.argv
  if (!spaceId || !strategyId) {
    console.error(usage)
    process.exit(1)
  }

  const baseURL = process.env.POWERX_API_BASE || 'http://127.0.0.1:8080/api'
  const token = process.env.POWERX_TOKEN || ''
  const endpoint = `${baseURL.replace(/\/$/, '')}/admin/knowledge-spaces/${spaceId}/fusion-strategies/${strategyId}/rollback`

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
  console.log('[fusion] rollback triggered:', JSON.stringify(payload.data ?? payload, null, 2))
}

main().catch(err => {
  console.error('[fusion] rollback script error:', err)
  process.exit(1)
})
