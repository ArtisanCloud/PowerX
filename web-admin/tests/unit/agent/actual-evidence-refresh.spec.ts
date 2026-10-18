import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import en from '../../../i18n/locales/en.json'
import zh from '../../../i18n/locales/zh.json'
import { normalizeHistoryMessageMeta } from '~/utils/agent/historyMessageMeta'
import { isAgentResponseEnvelope, renderAgentResponseEnvelope } from '~/utils/agent/responseEnvelope'

// 输入由本次真实 native 插件 → Core V5/归档 → 新 Service ProjectHistory 导出。
// 缺少原件时明确 skip；不能退回手写报告并把它当真实刷新验收。
const artifact = process.env.POWERX_TEST_EVIDENCE_EXPORT
interface ActualEvidenceRow {
  case: string
  skill_id: string
  http_base: string
  expected_value?: string
  live?: Record<string, any>
  history: Record<string, any>
}
const rows: ActualEvidenceRow[] = artifact
  ? readFileSync(artifact, 'utf8').trim().split('\n').map(line => JSON.parse(line))
  : []
const translate = (locale: Record<string, any>) => (key: string) => {
  const value = key.split('.').reduce<any>((current, part) => current?.[part], locale)
  if (typeof value !== 'string') throw new Error(`missing locale: ${key}`)
  return value
}

describe.skipIf(!artifact)('actual backend evidence refresh projection', () => {
  it('contains all real numeric cases and denied projections', () => {
    expect(rows.filter(row => row.live)).toHaveLength(18)
    expect(rows.filter(row => row.case === 'revoked')).toHaveLength(6)
    expect(new Set(rows.filter(row => row.live).map(row => row.case))).toEqual(new Set(['original', 'decimal', 'large_exact']))
  })
  it.each(rows.map((row, index) => ({ row, key: `${row.http_base}/${row.skill_id}/${row.case}/${index}` })))('$key survives JSON refresh or remains unavailable', ({ row }) => {
    const fresh = normalizeHistoryMessageMeta(JSON.parse(JSON.stringify(row.history)))
    if (!row.live) {
      expect(fresh.responseEvidenceState).toBe('unavailable')
      expect(fresh.responseEnvelope).toBeUndefined()
      expect(fresh.responseLabels).toBeUndefined()
      return
    }
    const live = normalizeHistoryMessageMeta(row.live)
    expect(fresh.responseEvidenceState).toBe('available')
    expect(isAgentResponseEnvelope(live.responseEnvelope)).toBe(true)
    expect(isAgentResponseEnvelope(fresh.responseEnvelope)).toBe(true)
    const original = JSON.stringify(fresh.responseEnvelope)
    for (const [locale, messages] of [['en-US', en], ['zh-CN', zh]] as const) {
      const t = translate(messages)
      const before = renderAgentResponseEnvelope(live.responseEnvelope, t, { labels: live.responseLabels, locale })
      const after = renderAgentResponseEnvelope(fresh.responseEnvelope, t, { labels: fresh.responseLabels, locale })
      expect(after).toBe(before)
      expect(after).toContain(row.expected_value)
      expect(after).toContain(t('agent.response.inputStates.user_supplied_unverified'))
      const report = fresh.responseEnvelope
      const key = report.presentation.computed[0].label_key
      expect(after).toContain(fresh.responseLabels.locales[locale][key])
      expect(after).not.toContain(report.receipts[0].execution_uuid)
      expect(after).not.toContain(report.receipts[0].tool.tool_key)
    }
    expect(JSON.stringify(fresh.responseEnvelope)).toBe(original)
  })
})
