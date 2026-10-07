export type AgentResponseDatum = { key: string; label: string; value: string; unit: string; scope: string; kind: 'quantity' | 'reported'; source: { pointer: string; quote: string; literal: string } }
export type AgentResponseCalculation = { uuid: string; tool_key: string; tool_version: string; display_value: string; request: { key: string; label: string; expression: string }; operands: Record<string, AgentResponseDatum> }
export type AgentResponseEnvelope = {
  schema: 'powerx.agent.response/v4'
  kind: string
  outcome: 'completed' | 'needs_action' | 'blocked' | 'failed'
  presentation: {
    reported: AgentResponseDatum[]
    computed: AgentResponseCalculation[]
    conflicts: { calculation_key: string; reported_key: string; reported_value: string; calculated_value: string }[]
    hypotheses: string[]
    gaps: string[]
    actions: string[]
  }
}

export function isAgentResponseEnvelope(value: any): value is AgentResponseEnvelope {
  return value?.schema === 'powerx.agent.response/v4'
    && value?.presentation && ['reported', 'computed', 'conflicts', 'hypotheses', 'gaps', 'actions']
      .every(key => Array.isArray(value.presentation[key]))
}

const cell = (value: string) => String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;')
  .replace(/>/g, '&gt;').replace(/\|/g, '&#124;').replace(/\r?\n/g, ' ')
const textList = (title: string, values: string[]) =>
  values.length ? `## ${title}\n\n${values.map(value => `- ${cell(value)}`).join('\n')}` : ''
const table = (title: string, columns: string[], rows: string[][]) => rows.length
  ? [`## ${title}`, '', `| ${columns.map(cell).join(' | ')} |`, `| ${columns.map(() => '---').join(' | ')} |`,
      ...rows.map(row => `| ${row.map(cell).join(' | ')} |`)].join('\n') : ''

// PowerX, not an LLM, owns all visible structure. The Skill submits only data.
export function renderAgentResponseEnvelope(envelope: AgentResponseEnvelope, t: (key: string) => string): string {
  const p = envelope.presentation
  const reported = table(t('agent.response.reported'),
    [t('agent.response.metric'), t('agent.response.value'), t('agent.response.sourceQuote')],
    p.reported.map(d => [d.label, d.value + d.unit, d.source.quote]))
  const computed = table(t('agent.response.computed'),
    [t('agent.response.metric'), t('agent.response.value'), t('agent.response.formula'), t('agent.response.operands')],
    p.computed.map(c => [c.request.label, c.display_value, c.request.expression,
      Object.entries(c.operands).map(([key, d]) => `${key} = ${d.value}${d.unit}`).join('; ')]))
  const conflicts = table(t('agent.response.conflicts'),
    [t('agent.response.metric'), t('agent.response.reportedValue'), t('agent.response.calculatedValue')],
    p.conflicts.map(c => [p.computed.find(x => x.request.key === c.calculation_key)?.request.label || t('agent.response.metric'), c.reported_value, c.calculated_value]))
  return [
    `## ${t('agent.response.conclusion')}\n\n${t(`agent.response.outcomes.${envelope.outcome}`)}`,
    reported,
    computed ? `${computed}\n\n${t('agent.response.calculationBoundary')}` : '',
    conflicts ? `${conflicts}\n\n${t('agent.response.conflictAction')}` : '',
    textList(t('agent.response.hypotheses'), p.hypotheses),
    textList(t('agent.response.gaps'), p.gaps),
    textList(t('agent.response.nextActions'), p.actions),
  ].filter(Boolean).join('\n\n')
}
