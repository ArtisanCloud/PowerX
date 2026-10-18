import { describe, expect, it } from 'vitest'
import cases from '../../../../specs/024-ai-engineering-skills/contracts/tool-evidence-cases.json'
import type { AgentResponseEnvelopeV5, ResponseLabels } from '~/types/agentEvidence'
import { isAgentResponseEnvelope, renderAgentResponseEnvelope } from '~/utils/agent/responseEnvelope'
import { normalizeHistoryMessageMeta } from '~/utils/agent/historyMessageMeta'

const fixture = () => JSON.parse(JSON.stringify(cases.fixtures.conflict_report)) as AgentResponseEnvelopeV5
describe('V5 evidence rendering', () => {
  it('uses frozen Revision labels and renders actual decimal values without recalculation', () => {
    const report = fixture()
    const labelKey = report.presentation.computed[0]!.label_key
    const labels: ResponseLabels = { schema: 'powerx.response-labels/v1', revision_uuid: report.revision_uuid, locales: { 'en-US': { [labelKey]: 'Turnover' }, 'zh-CN': { [labelKey]: '库存周转' } } }
    expect(isAgentResponseEnvelope(report)).toBe(true)
    const before = JSON.stringify(report)
    const en = renderAgentResponseEnvelope(report, k => k, { labels, locale: 'en' })
    const zh = renderAgentResponseEnvelope(report, k => k, { labels, locale: 'zh' })
    expect(en).toContain('Turnover'); expect(zh).toContain('库存周转')
    expect(en).toContain('2.50'); expect(en).toContain('agent.response.inputStates.extracted_unverified')
    expect(en).not.toContain(report.receipts[0]!.execution_uuid)
    expect(en).not.toContain(report.receipts[0]!.tool.tool_key)
    expect(JSON.stringify(report)).toBe(before)
    const restored = normalizeHistoryMessageMeta({ response_envelope: JSON.parse(before), response_labels: labels })
    expect(renderAgentResponseEnvelope(restored.responseEnvelope, k => k, { labels: restored.responseLabels, locale: 'en' })).toBe(en)
  })
  it.each(['receipt', 'run', 'outcome', 'derivation', 'decimal', 'failed_receipt'])('rejects invalid %s before rendering', kind => {
    const report = fixture()
    if (kind === 'failed_receipt') report.receipts[0]!.status = 'failed'
    if (kind === 'receipt') report.presentation.computed[0]!.receipt_uuid = 'unissued'
    if (kind === 'run') report.receipts[0]!.run_id = 'other_run'
    if (kind === 'outcome') report.outcome = 'completed'
    if (kind === 'derivation') (report.presentation.computed[0]!.derivation as any).operand_bindings = { a: 'forged' }
    if (kind === 'decimal') report.presentation.computed[0]!.value = 'NaN'
    expect(isAgentResponseEnvelope(report)).toBe(false)
  })
  it('normalizes unavailable evidence and failure stages for live/history consistency', () => {
    expect(normalizeHistoryMessageMeta({ response_evidence_state: 'unavailable' }).responseEvidenceState).toBe('unavailable')
    expect(normalizeHistoryMessageMeta({ failure_stage: 'final_validation' }).failureStage).toBe('final_validation')
  })
})
