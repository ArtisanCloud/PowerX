import { useApiClient } from '../index'

export type AgentCapabilityApproval = {
  approval_uuid: string
  status: 'pending'
  created_at: string
}

const base = '/admin/agents/capability-approvals'

const unwrap = <T>(response: any): T => response?.data ?? response

export const useAgentCapabilityApprovalService = () => {
  const api = useApiClient()
  return {
    async listPending() {
      return unwrap<{ items: AgentCapabilityApproval[] }>(await api.get(base))
    },
    async approve(approvalUUID: string) {
      return unwrap<{ approval_uuid: string, status: 'approved' }>(await api.post(`${base}/${encodeURIComponent(approvalUUID)}/approve`))
    },
    async reject(approvalUUID: string) {
      return unwrap<{ approval_uuid: string, status: 'rejected' }>(await api.post(`${base}/${encodeURIComponent(approvalUUID)}/reject`))
    }
  }
}
