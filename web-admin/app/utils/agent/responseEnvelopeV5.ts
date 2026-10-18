import type { AgentResponseEnvelopeV5, EvidenceDatum, ResponseLabels } from '~/types/agentEvidence'

const uuid = (v: unknown) => typeof v === 'string' && /^(?!00000000-0000-0000-0000-000000000000$)[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(v)
const text = (v: unknown) => typeof v === 'string' && v.length > 0
const decimal = (v: unknown) => typeof v === 'string' && v.length <= 1280 && /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/.test(v)
const digest = (v: unknown) => typeof v === 'string' && /^sha256:[0-9a-f]{64}$/.test(v)
const datum = (d: any): d is EvidenceDatum => uuid(d?.datum_uuid) && text(d.field_key) && text(d.label_key) && decimal(d.value) && text(d.unit) && text(d.scope)
  && ['quantity', 'reported'].includes(d.datum_kind) && ['user_supplied_unverified', 'extracted_unverified', 'authorized_record', 'tool_result'].includes(d.input_state)
  && text(d.literal) && uuid(d.source?.source_uuid) && uuid(d.source.object_uuid) && uuid(d.source.snapshot_uuid) && digest(d.source.snapshot_sha256)
  && ['run_input', 'authorized_object', 'tool_output'].includes(d.source.kind) && typeof d.source.pointer === 'string' && text(d.source.quote)
  && Number.isInteger(d.source.range?.start_byte) && Number.isInteger(d.source.range?.end_byte) && d.source.range.start_byte >= 0 && d.source.range.end_byte > d.source.range.start_byte
  && d.source.range.end_byte - d.source.range.start_byte === new TextEncoder().encode(d.source.quote).length
  && (d.datum_kind !== 'quantity' || !['%', '％'].includes(d.unit))
  && (d.source.kind === 'run_input' ? ['user_supplied_unverified', 'extracted_unverified'].includes(d.input_state) && !d.source.execution_uuid
    : d.source.kind === 'authorized_object' ? ['authorized_record', 'extracted_unverified'].includes(d.input_state) && !d.source.execution_uuid
      : d.input_state === 'tool_result' && uuid(d.source.execution_uuid))

// 客户端校验用于安全展示；真实性仍由 Core ledger 和受控历史存储判定。
export function isEvidenceReportV5(v: any): v is AgentResponseEnvelopeV5 {
  if (v?.schema !== 'powerx.agent.response/v5' || !text(v.kind) || !uuid(v.tenant_uuid) || !uuid(v.revision_uuid) || !text(v.run_id) || !text(v.trace_id) || !digest(v.source_digest) || !['completed', 'needs_action'].includes(v.outcome) || !Array.isArray(v.receipts)) return false
  const p = v.presentation
  if (!p || !['reported', 'computed', 'conflicts', 'gaps', 'hypotheses', 'actions'].every(k => Array.isArray(p[k]))) return false
  if (!p.reported.every(datum) || p.reported.length > 512 || p.computed.length > 64 || v.receipts.length > 64 || !p.reported.length && !p.computed.length && !p.gaps.length) return false
  if (v.outcome === 'completed' && (p.conflicts.length || p.gaps.length || p.hypotheses.length)) return false
  const receipts = new Map<string, any>()
  for (const r of v.receipts) {
    if (r?.schema !== 'powerx.tool-receipt/v1' || r.status !== 'succeeded' || !uuid(r.execution_uuid) || receipts.has(r.execution_uuid) || r.tenant_uuid !== v.tenant_uuid || r.revision_uuid !== v.revision_uuid || r.run_id !== v.run_id || r.trace_id !== v.trace_id || !Array.isArray(r.inputs) || !r.inputs.every(datum) || !text(r.tool?.tool_key) || !text(r.tool.version)) return false
    if (r.status === 'succeeded' ? !uuid(r.result?.artifact_uuid) || !digest(r.result?.sha256) || !!r.error : !['failed', 'timed_out', 'cancelled'].includes(r.status) || !!r.result || !text(r.error?.code)) return false
    receipts.set(r.execution_uuid, r)
  }
  const ids = new Set<string>(p.reported.map((d: EvidenceDatum) => d.datum_uuid))
  if (ids.size !== p.reported.length) return false
  for (const c of p.computed) {
    const r = receipts.get(c?.receipt_uuid)
    if (!uuid(c?.datum_uuid) || ids.has(c.datum_uuid) || r?.status !== 'succeeded' || !text(c.field_key) || !text(c.label_key) || !decimal(c.value) || !text(c.unit) || !text(c.scope) || typeof c.output_pointer !== 'string') return false
    const d = c.derivation
    if (d?.kind === 'expression') {
      if (!text(d.expression) || !d.operand_bindings || !Object.values(d.operand_bindings).length || !Object.values(d.operand_bindings).every(id => r.inputs.some((input: EvidenceDatum) => input.datum_uuid === id && input.datum_kind === 'quantity')) || !Number.isInteger(d.precision) || d.precision < 0 || d.precision > 12 || typeof d.percent !== 'boolean' || d.percent !== (c.unit === '%') || d.rounding !== 'half_away_from_zero') return false
    } else if (d?.kind !== 'domain_algorithm' || !text(d.algorithm_id) || !digest(d.parameters_sha256)) return false
    ids.add(c.datum_uuid)
  }
  return p.conflicts.every((c: any) => uuid(c?.conflict_uuid) && p.reported.some((d: EvidenceDatum) => d.datum_uuid === c.reported_datum_uuid) && p.computed.some((d: any) => d.datum_uuid === c.computed_datum_uuid) && ['value_mismatch', 'unit_mismatch', 'scope_mismatch'].includes(c.reason))
    && p.gaps.every((g: any) => text(g?.key) && text(g.label_key) && Array.isArray(g.missing_fields) && g.missing_fields.length && g.missing_fields.every(text))
    && [...p.hypotheses, ...p.actions].every(text)
}

const cell = (value: string) => String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\|/g, '&#124;').replace(/\r?\n/g, ' ')
const table = (title: string, headers: string[], rows: string[][]) => rows.length ? [`## ${title}`, '', `| ${headers.map(cell).join(' | ')} |`, `| ${headers.map(() => '---').join(' | ')} |`, ...rows.map(row => `| ${row.map(cell).join(' | ')} |`)].join('\n') : ''
const list = (title: string, rows: string[]) => rows.length ? `## ${title}\n\n${rows.map(v => `- ${cell(v)}`).join('\n')}` : ''
// 只补展示小数位，绝不截断、舍入或重新计算。
const display = (value: string, unit: string, precision?: number) => {
  const parts = value.split('.')
  const formatted = precision === undefined || precision === 0 ? value : `${parts[0]}.${(parts[1] || '').padEnd(precision, '0')}`
  return formatted + (unit === '1' ? '' : unit)
}
export function renderEvidenceReportV5(report: AgentResponseEnvelopeV5, t: (key: string) => string, options?: { labels?: ResponseLabels; locale?: string }): string {
  const locale = options?.locale?.startsWith('zh') ? 'zh-CN' : 'en-US'
  const labels = options?.labels?.schema === 'powerx.response-labels/v1' && options.labels.revision_uuid === report.revision_uuid ? options.labels.locales[locale] : undefined
  const label = (key: string) => labels?.[key] || (key.startsWith('agent.response.') ? t(key) : t('agent.response.metric'))
  const p = report.presentation
  const reported = table(t('agent.response.reported'), [t('agent.response.metric'), t('agent.response.value'), t('agent.response.inputState'), t('agent.response.sourceQuote')], p.reported.map(d => [label(d.label_key), display(d.value, d.unit), t(`agent.response.inputStates.${d.input_state}`), d.source.quote]))
  const computed = table(t('agent.response.computed'), [t('agent.response.metric'), t('agent.response.value'), t('agent.response.formula')], p.computed.map(d => [label(d.label_key), display(d.value, d.unit, d.derivation.kind === 'expression' ? d.derivation.precision : undefined), d.derivation.kind === 'expression' ? d.derivation.expression : t('agent.response.domainAlgorithm')]))
  const conflicts = table(t('agent.response.conflicts'), [t('agent.response.metric'), t('agent.response.reportedValue'), t('agent.response.calculatedValue'), t('agent.response.reason')], p.conflicts.map(c => {
    const a = p.reported.find(d => d.datum_uuid === c.reported_datum_uuid)!
    const b = p.computed.find(d => d.datum_uuid === c.computed_datum_uuid)!
    return [label(b.label_key), display(a.value, a.unit), display(b.value, b.unit), t(`agent.response.conflictReasons.${c.reason}`)]
  }))
  return [`## ${t('agent.response.conclusion')}\n\n${t(`agent.response.outcomes.${report.outcome}`)}`, reported, computed ? `${computed}\n\n${t('agent.response.calculationBoundary')}` : '', conflicts, list(t('agent.response.gaps'), p.gaps.map(g => `${label(g.label_key)}: ${g.missing_fields.map(k => label(`skill.fields.${k}`)).join(', ')}`)), list(t('agent.response.hypotheses'), p.hypotheses), list(t('agent.response.nextActions'), p.actions)].filter(Boolean).join('\n\n')
}
