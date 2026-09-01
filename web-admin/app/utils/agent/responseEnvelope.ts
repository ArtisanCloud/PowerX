export type AgentResponseSource = { type: 'input' | 'task'; ref: string }
export type AgentResponseFact = { statement: string; source: AgentResponseSource }
export type AgentResponseMetric = {
  label: string
  numerator?: number | string
  denominator?: number | string
  formula: string
  display_value?: string
  source: AgentResponseSource
}
export type AgentResponseEnvelope = {
  schema: 'powerx.agent.response/v3'
  kind: string
  outcome: 'completed' | 'needs_action' | 'blocked' | 'failed'
  presentation: {
    facts?: AgentResponseFact[]
    metrics?: AgentResponseMetric[]
    hypotheses?: string[]
    gaps?: string[]
    actions?: string[]
  }
}

export function isAgentResponseEnvelope(value: any): value is AgentResponseEnvelope {
  return value?.schema === 'powerx.agent.response/v3'
    && value?.presentation && typeof value.presentation === 'object'
}

const textList = (title: string, values?: string[]) =>
  values?.length ? `## ${title}\n${values.map(value => `- ${value}`).join('\n')}` : ''

// PowerX, not an LLM, owns all visible structure. The Skill submits only data.
export function renderAgentResponseEnvelope(envelope: AgentResponseEnvelope, t: (key: string) => string): string {
  const presentation = envelope.presentation || {}
  const metrics = presentation.metrics?.length
    ? [
        `## ${t('agent.response.metrics')}`,
        `| ${t('agent.response.metric')} | ${t('agent.response.value')} | ${t('agent.response.formula')} |`,
        '| --- | --- | --- |',
        ...presentation.metrics.map(item => `| ${item.label} | ${item.display_value || ''} | ${item.formula} |`),
      ].join('\n')
    : ''
  const facts = presentation.facts?.length
    ? `## ${t('agent.response.facts')}\n${presentation.facts.map(item => `- ${item.statement}`).join('\n')}`
    : ''
  const acceptance = [
    ...(presentation.gaps || []),
    ...(presentation.actions || []),
  ]
  return [
    `## ${t('agent.response.conclusion')}\n${t(`agent.response.outcomes.${envelope.outcome}`)}`,
    metrics,
    facts,
    textList(t('agent.response.hypotheses'), presentation.hypotheses),
    textList(t('agent.response.gaps'), presentation.gaps),
    textList(t('agent.response.nextActions'), presentation.actions),
    textList(t('agent.response.acceptance'), acceptance),
  ].filter(Boolean).join('\n\n')
}
