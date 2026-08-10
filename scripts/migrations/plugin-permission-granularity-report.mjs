#!/usr/bin/env node

import fs from 'node:fs'
import path from 'node:path'

const legacyMappings = {
  'operations.order:read': [
    'production.sample_track:read',
  ],
  'operations.order:manage': [
    'production.sample_track:factory_schedule',
    'production.sample_track:delivery',
    'production.sample_track:planner_review',
    'production.sample_track:designer_acceptance',
    'production.bulk_order:manage',
  ],
}

const legacyPatterns = [
  {
    pattern: 'operations.order:read/manage',
    permissionCodes: ['operations.order:read', 'operations.order:manage'],
  },
  {
    pattern: 'operations.order:*',
    permissionCodes: ['operations.order:read', 'operations.order:manage'],
  },
]

const ignoredDirs = new Set([
  '.git',
  '.nuxt',
  '.output',
  'coverage',
  'dist',
  'node_modules',
  'tmp',
  'vendor',
])

const textExtensions = new Set([
  '.go',
  '.json',
  '.md',
  '.mjs',
  '.sql',
  '.ts',
  '.tsx',
  '.vue',
  '.yaml',
  '.yml',
])

const args = process.argv.slice(2)
const format = args.includes('--format=markdown') ? 'markdown' : 'json'
const rootArg = args.find((arg) => !arg.startsWith('--')) || '.'
const root = path.resolve(process.cwd(), rootArg)

if (!fs.existsSync(root)) {
  throw new Error(`scan root does not exist: ${root}`)
}

const findings = []

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (ignoredDirs.has(entry.name)) {
      continue
    }
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      walk(full)
      continue
    }
    if (!entry.isFile() || !textExtensions.has(path.extname(entry.name))) {
      continue
    }
    scanFile(full)
  }
}

function scanFile(file) {
  const rel = path.relative(root, file)
  const content = fs.readFileSync(file, 'utf8')
  const lines = content.split(/\r?\n/)
  lines.forEach((line, idx) => {
    for (const legacyCode of Object.keys(legacyMappings)) {
      if (!line.includes(legacyCode)) {
        continue
      }
      findings.push({
        file: rel,
        line: idx + 1,
        legacy_permission_code: legacyCode,
        target_permission_codes: legacyMappings[legacyCode],
      })
    }
    for (const legacyPattern of legacyPatterns) {
      if (!line.includes(legacyPattern.pattern)) {
        continue
      }
      for (const legacyCode of legacyPattern.permissionCodes) {
        findings.push({
          file: rel,
          line: idx + 1,
          legacy_permission_code: legacyCode,
          target_permission_codes: legacyMappings[legacyCode],
        })
      }
    }
  })
}

walk(root)

const uniqueFindings = []
const seenFindings = new Set()
for (const finding of findings) {
  const key = `${finding.file}:${finding.line}:${finding.legacy_permission_code}`
  if (seenFindings.has(key)) {
    continue
  }
  seenFindings.add(key)
  uniqueFindings.push(finding)
}

const missingAuthorizationChecklist = [
  ...new Set(uniqueFindings.flatMap((finding) => finding.target_permission_codes)),
].sort()

const report = {
  generated_at: new Date().toISOString(),
  scanned_root: root,
  legacy_permission_codes: Object.keys(legacyMappings),
  migration_matrix: legacyMappings,
  findings: uniqueFindings,
  missing_authorization_checklist: missingAuthorizationChecklist,
  runtime_policy: 'no_alias_no_compatibility; missing fine-grained permission must be denied',
}

if (format === 'markdown') {
  printMarkdown(report)
} else {
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`)
}

function printMarkdown(value) {
  process.stdout.write('# Plugin Permission Granularity Migration Report\n\n')
  process.stdout.write(`- Generated at: ${value.generated_at}\n`)
  process.stdout.write(`- Scanned root: ${value.scanned_root}\n`)
  process.stdout.write(`- Runtime policy: ${value.runtime_policy}\n\n`)
  process.stdout.write('## Migration Matrix\n\n')
  for (const [legacyCode, targetCodes] of Object.entries(value.migration_matrix)) {
    process.stdout.write(`- \`${legacyCode}\` -> ${targetCodes.map((code) => `\`${code}\``).join(', ')}\n`)
  }
  process.stdout.write('\n## Findings\n\n')
  if (value.findings.length === 0) {
    process.stdout.write('No legacy coarse permission usage found.\n\n')
  } else {
    for (const finding of value.findings) {
      process.stdout.write(`- ${finding.file}:${finding.line} uses \`${finding.legacy_permission_code}\`\n`)
    }
    process.stdout.write('\n')
  }
  process.stdout.write('## Missing Authorization Checklist\n\n')
  if (value.missing_authorization_checklist.length === 0) {
    process.stdout.write('No fine-grained permissions need backfill from scanned legacy usage.\n')
    return
  }
  for (const code of value.missing_authorization_checklist) {
    process.stdout.write(`- [ ] \`${code}\`\n`)
  }
}
