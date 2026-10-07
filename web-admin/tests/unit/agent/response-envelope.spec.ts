import { describe, expect, it } from 'vitest'
import { isAgentResponseEnvelope, renderAgentResponseEnvelope, type AgentResponseEnvelope } from '~/utils/agent/responseEnvelope'
import { normalizeHistoryMessageMeta } from '~/utils/agent/historyMessageMeta'

describe('evidence report v4', () => {
  const datum = { key: 'cost', label: 'cost', value: '34.2', unit: '', scope: 'run', kind: 'quantity' as const, source: { pointer: '/message', quote: 'cost | 34.2\n<script>', literal: '34.2' } }
  const report: AgentResponseEnvelope = {
    schema: 'powerx.agent.response/v4', kind: 'analysis', outcome: 'needs_action',
    presentation: {
      reported: [datum], computed: [{ uuid: 'hidden-receipt-uuid', tool_key: 'hidden-tool-key', tool_version: '1.0.0', display_value: '0.848', request: { key: 'ratio', label: 'ratio', expression: 'a/b' }, operands: { b: datum } }],
      conflicts: [{ calculation_key: 'ratio', reported_key: 'claim', reported_value: '3.37', calculated_value: '0.848' }], hypotheses: [], gaps: ['counts_required'], actions: [],
    },
  }
  it('renders reported values, computed results and conflicts with separate tables', () => {
    const md = renderAgentResponseEnvelope(report, k => k)
    expect(md).toContain('## agent.response.reported\n\n|')
    expect(md).toContain('## agent.response.computed\n\n|')
    expect(md).toContain('## agent.response.conflicts\n\n|')
    expect(md).toContain('| ratio | 3.37 | 0.848 |')
    expect(md).toContain('agent.response.calculationBoundary')
    expect(md).toContain('&#124;')
    expect(md).not.toContain('<script>')
    expect(md).not.toContain('hidden-receipt-uuid')
    expect(md).not.toContain('hidden-tool-key')
  })
  it('renders the same persisted payload after history normalization', () => {
    const copy = JSON.parse(JSON.stringify(report))
    const meta = normalizeHistoryMessageMeta({ response_envelope: copy })
    expect(isAgentResponseEnvelope(meta.responseEnvelope)).toBe(true)
    expect(renderAgentResponseEnvelope(meta.responseEnvelope, k => k)).toBe(renderAgentResponseEnvelope(report, k => k))
  })
  it('rejects retired and incomplete contracts', () => {
    expect(isAgentResponseEnvelope({ ...report, schema: 'powerx.agent.response/v3' })).toBe(false)
    expect(isAgentResponseEnvelope({ schema: report.schema, presentation: {} })).toBe(false)
  })
})
