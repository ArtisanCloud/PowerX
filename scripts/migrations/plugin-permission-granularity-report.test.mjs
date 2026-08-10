#!/usr/bin/env node

import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const currentDir = path.dirname(fileURLToPath(import.meta.url))
const reportScript = path.join(currentDir, 'plugin-permission-granularity-report.mjs')
const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'powerx-plugin-permission-report-'))

try {
  fs.writeFileSync(
    path.join(tempDir, 'legacy-permissions.md'),
    [
      'old exact merged form: operations.order:read/manage',
      'old wildcard form: operations.order:*',
    ].join('\n'),
  )

  const raw = execFileSync(process.execPath, [reportScript, tempDir], {
    encoding: 'utf8',
  })
  const report = JSON.parse(raw)

  assert.equal(report.runtime_policy, 'no_alias_no_compatibility; missing fine-grained permission must be denied')
  assert.deepEqual(report.legacy_permission_codes, [
    'operations.order:read',
    'operations.order:manage',
  ])
  assert.deepEqual(report.missing_authorization_checklist, [
    'production.bulk_order:manage',
    'production.sample_track:delivery',
    'production.sample_track:designer_acceptance',
    'production.sample_track:factory_schedule',
    'production.sample_track:planner_review',
    'production.sample_track:read',
  ])

  const legacyCodes = new Set(report.findings.map((finding) => finding.legacy_permission_code))
  assert.equal(legacyCodes.has('operations.order:read'), true)
  assert.equal(legacyCodes.has('operations.order:manage'), true)
  assert.equal(report.findings.length, 4)

  console.log('plugin permission granularity report test ok')
} finally {
  fs.rmSync(tempDir, { recursive: true, force: true })
}
