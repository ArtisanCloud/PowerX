import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";
import type {
  RegistrationPolicyEffective,
  RegistrationPolicyMode,
} from "~/composables/domain/registrationPolicy";

export interface RegistrationPolicyRule {
  type: string;
  values?: string[];
  batch_uuid?: string;
  value?: number;
  seed?: string;
}

export interface RegistrationPolicy {
  uuid: string;
  version: number;
  mode: RegistrationPolicyMode;
  status: string;
  requires_verification: boolean;
  requires_invite_code: boolean;
  requires_root_approval: boolean;
  daily_tenant_quota?: number;
  total_tenant_quota?: number;
  rules?: RegistrationPolicyRule[];
  activated_at?: string;
  created_at?: string;
  updated_at?: string;
  created_by_user_uuid?: string;
  updated_by_user_uuid?: string;
}

export interface RegistrationPolicyUpdateRequest {
  mode: RegistrationPolicyMode;
  requires_verification: boolean;
  requires_invite_code: boolean;
  requires_root_approval: boolean;
  daily_tenant_quota?: number;
  total_tenant_quota?: number;
  rules: RegistrationPolicyRule[];
}

export interface RegistrationRequestCreateRequest {
  tenant_key?: string;
  tenant_name: string;
  plan?: string;
  owner_email?: string;
  owner_phone?: string;
  owner_display_name?: string;
  invite_code?: string;
  channel?: string;
  campaign?: string;
}

export interface RegistrationRequestRecord {
  uuid: string;
  mode: "waitlist" | "approval_required";
  status: string;
  tenant_name: string;
  tenant_key?: string;
  plan?: string;
  policy_uuid: string;
  policy_version: number;
  created_tenant_uuid?: string;
  reject_reason_code?: string;
}

export interface RegistrationInviteBatchCreateRequest {
  name: string;
  max_codes: number;
  max_uses_per_code?: number;
  allowed_plan?: string;
  allowed_email_domains?: string[];
  allowed_channels?: string[];
}

export interface RegistrationInviteCodeRecord {
  uuid: string;
  batch_uuid: string;
  plain_code?: string;
  status: string;
  max_uses: number;
  use_count: number;
  last_used_at?: string;
  consumed_tenant_uuid?: string;
  revoked_at?: string;
}

export const useRegistrationPolicyService = () => {
  const apiClient = useApiClient();

  return {
    getEffectivePolicy: () =>
      apiClient.get<ApiResponse<{ policy: RegistrationPolicyEffective }>>(
        "/public/saas/registration-policy/effective",
        { skipAuth: true }
      ),
    submitRegistrationRequest: (data: RegistrationRequestCreateRequest) =>
      apiClient.post<ApiResponse<RegistrationRequestRecord>>(
        "/public/saas/registration-requests",
        data,
        { skipAuth: true }
      ),
    getPolicy: () =>
      apiClient.get<ApiResponse<RegistrationPolicy>>(
        "/admin/registration-policy"
      ),
    listPolicyHistory: () =>
      apiClient.get<ApiResponse<{ items: RegistrationPolicy[] }>>(
        "/admin/registration-policy/history",
        { params: { limit: 20 } }
      ),
    createPolicyDraft: (data: RegistrationPolicyUpdateRequest) =>
      apiClient.put<ApiResponse<RegistrationPolicy>>(
        "/admin/registration-policy",
        data
      ),
    activatePolicy: (policyUUID: string) =>
      apiClient.post<ApiResponse<RegistrationPolicy>>(
        "/admin/registration-policy/activate",
        { policy_uuid: policyUUID }
      ),
    listInviteBatches: () =>
      apiClient.get<ApiResponse<{ items: any[] }>>(
        "/admin/registration-invite-batches"
      ),
    createInviteBatch: (data: RegistrationInviteBatchCreateRequest) =>
      apiClient.post<ApiResponse<any>>(
        "/admin/registration-invite-batches",
        data
      ),
    deleteInviteBatches: (batchUUIDs: string[]) =>
      apiClient.delete<ApiResponse<{ deleted: number }>>(
        "/admin/registration-invite-batches",
        { params: { batch_uuid: batchUUIDs } }
      ),
    generateInviteCodes: (batchUUID: string, count: number) =>
      apiClient.post<ApiResponse<{ batch_uuid: string; plain_codes: string[] }>>(
        `/admin/registration-invite-batches/${batchUUID}/codes`,
        { count }
      ),
    listInviteCodes: (batchUUID: string) =>
      apiClient.get<ApiResponse<{ items: RegistrationInviteCodeRecord[] }>>(
        `/admin/registration-invite-batches/${batchUUID}/codes`,
        { params: { limit: 1000 } }
      ),
    resetMissingInviteCodePlaintext: (batchUUID: string) =>
      apiClient.post<ApiResponse<{ items: RegistrationInviteCodeRecord[] }>>(
        `/admin/registration-invite-batches/${batchUUID}/codes/reset-missing-plain`,
        {}
      ),
    listRequests: () =>
      apiClient.get<ApiResponse<{ items: RegistrationRequestRecord[] }>>(
        "/admin/registration-requests"
      ),
    approveRequest: (requestUUID: string) =>
      apiClient.post<ApiResponse<RegistrationRequestRecord>>(
        `/admin/registration-requests/${requestUUID}/approve`,
        {}
      ),
    rejectRequest: (requestUUID: string, rejectReasonCode: string) =>
      apiClient.post<ApiResponse<RegistrationRequestRecord>>(
        `/admin/registration-requests/${requestUUID}/reject`,
        { reject_reason_code: rejectReasonCode }
      ),
  };
};
