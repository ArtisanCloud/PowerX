#!/usr/bin/env node

import fs from 'node:fs/promises'
import path from 'node:path'

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')

async function main() {
  const args = parseArgs(process.argv.slice(2))

  const knowledgeSpacesPath = path.join(repoRoot, 'reports', '_state', 'knowledge-spaces.json')
  const knowledgeUpdatePath = path.join(repoRoot, 'reports', '_state', 'knowledge-update.json')
  const decayPath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-decay.json')
  const releasePath = path.join(repoRoot, 'backend', 'reports', '_state', 'knowledge-release.json')

  const errors = []

  await ensureFile(knowledgeSpacesPath, errors)
  await ensureFile(knowledgeUpdatePath, errors)
  await ensureFile(decayPath, errors)
  await ensureFile(releasePath, errors)

  if (errors.length) {
    printErrors(errors)
    process.exit(1)
  }

  const ks = await readJSON(knowledgeSpacesPath)
  const ku = await readJSON(knowledgeUpdatePath)

  validateKnowledgeSpacesReport(ks, errors)
  validateKnowledgeUpdateReport(ku, errors)

  if (errors.length) {
    printErrors(errors)
    process.exit(1)
  }

  if (!args.quiet) {
    console.info('✅ smoke verify ok')
    console.info(`- ${rel(knowledgeSpacesPath)}`)
    console.info(`- ${rel(knowledgeUpdatePath)}`)
    console.info(`- ${rel(decayPath)}`)
    console.info(`- ${rel(releasePath)}`)
  }
}

function parseArgs(argv) {
  const args = { quiet: false }
  for (const token of argv) {
    if (token === '--quiet') args.quiet = true
  }
  return args
}

async function ensureFile(filePath, errors) {
  try {
    await fs.stat(filePath)
  } catch {
    errors.push(`缺少文件：${rel(filePath)}`)
  }
}

async function readJSON(filePath) {
  const raw = await fs.readFile(filePath, 'utf8')
  try {
    return JSON.parse(raw)
  } catch (err) {
    throw new Error(`无法解析 JSON：${rel(filePath)}: ${err.message}`)
  }
}

function validateKnowledgeSpacesReport(data, errors) {
  if (!data || typeof data !== 'object') {
    errors.push('reports/_state/knowledge-spaces.json 内容为空或不是对象')
    return
  }
  const keys = Object.keys(data)
  if (!keys.length) {
    errors.push('reports/_state/knowledge-spaces.json 未包含任何 space 记录')
    return
  }
  const sample = data[keys[0]]
  if (!sample || typeof sample !== 'object') {
    errors.push('reports/_state/knowledge-spaces.json space 记录结构异常')
    return
  }
  if (!('ingestion' in sample)) errors.push('reports/_state/knowledge-spaces.json 缺少 ingestion 段')
  if (!('feedback' in sample)) errors.push('reports/_state/knowledge-spaces.json 缺少 feedback 段')
}

function validateKnowledgeUpdateReport(data, errors) {
  if (!data || typeof data !== 'object') {
    errors.push('reports/_state/knowledge-update.json 内容为空或不是对象')
    return
  }
  for (const key of ['delta', 'event', 'decay', 'release']) {
    if (!(key in data)) errors.push(`reports/_state/knowledge-update.json 缺少 ${key} 段`)
  }
}

function printErrors(errors) {
  console.error('❌ smoke verify failed:')
  for (const msg of errors) console.error(`- ${msg}`)
}

function rel(p) {
  return path.relative(repoRoot, p) || p
}

main().catch((err) => {
  console.error(`❌ smoke verify error: ${err.message}`)
  process.exit(1)
})

