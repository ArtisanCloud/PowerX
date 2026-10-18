// V5 wire 对齐 contracts/tool-evidence.v1.schema.json；历史 V4 保留独立类型。
export interface EvidenceDatum {
  datum_uuid: string; field_key: string; label_key: string; datum_kind: 'quantity' | 'reported'
  literal: string; value: string; unit: string; scope: string
  input_state: 'user_supplied_unverified' | 'extracted_unverified' | 'authorized_record' | 'tool_result'
  source: { source_uuid: string; kind: 'run_input' | 'authorized_object' | 'tool_output'; object_type: string; object_uuid: string; snapshot_uuid: string; snapshot_sha256: string; pointer: string; quote: string; range: { start_byte: number; end_byte: number }; execution_uuid?: string }
}
export type EvidenceDerivation = { kind: 'expression'; expression: string; operand_bindings: Record<string, string>; precision: number; percent: boolean; rounding: 'half_away_from_zero' } | { kind: 'domain_algorithm'; algorithm_id: string; parameters_sha256: string }
export interface EvidenceComputed {
  datum_uuid: string; field_key: string; label_key: string; receipt_uuid: string; output_pointer: string; value: string; unit: string; scope: string; derivation: EvidenceDerivation
}
export interface EvidenceReceipt {
  schema: 'powerx.tool-receipt/v1'; execution_uuid: string; tenant_uuid: string; revision_uuid: string; run_id: string; trace_id: string; node_id: string; attempt: number
  tool: { dependency_key: string; tool_key: string; version: string; input_schema: { id: string; sha256: string }; output_schema: { id: string; sha256: string }; permission: { actor: string; code: string }; binding: { kind: 'builtin' | 'capability'; implementation_sha256: string; capability_uuid?: string; contract_uuid?: string; provider_version?: string; protocol?: string } }
  invocation_binding: { kind: 'builtin' | 'capability'; registration_version?: number; adapter_id?: string; catalog_sha256?: string }
  authorization: { decision_uuid: string; subject_uuid: string; actor: string }
  inputs: EvidenceDatum[]; request: { artifact_uuid: string; sha256: string }; started_at: string; ended_at: string; status: 'succeeded' | 'failed' | 'timed_out' | 'cancelled'
  result?: { artifact_uuid: string; sha256: string }; error?: { code: string; stage: string; retryable: boolean }
}
export interface AgentResponseEnvelopeV5 {
  schema: 'powerx.agent.response/v5'; kind: string; outcome: 'completed' | 'needs_action'; tenant_uuid: string; revision_uuid: string; run_id: string; trace_id: string; source_digest: string
  receipts: EvidenceReceipt[]
  presentation: {
    reported: EvidenceDatum[]; computed: EvidenceComputed[]
    conflicts: { conflict_uuid: string; reported_datum_uuid: string; computed_datum_uuid: string; reason: 'value_mismatch' | 'unit_mismatch' | 'scope_mismatch' }[]
    gaps: { key: string; label_key: string; missing_fields: string[] }[]
    hypotheses: string[]; actions: string[]
  }
}
export interface ResponseLabels { schema: 'powerx.response-labels/v1'; revision_uuid: string; locales: Record<string, Record<string, string>> }
