import { useApiClient } from '../index'

export interface Pagination { total: number; page: number; page_size: number }
export interface CustomerAccount { uuid: string; type: 'person' | 'company' | ''; primary_contact_uuid?: string; display_name?: string; nickname?: string; primary_email?: string; primary_phone?: string; profile_version?: string; status: string; member_status?: string; member_source?: string; membership_uuid?: string; created_at?: string; updated_at?: string }
export interface CustomerProfileUpdate { expected_profile_version: string; display_name: string; nickname: string; primary_email: string; primary_phone: string }
export interface CustomerAuthIdentity { uuid: string; provider: string; provider_subject?: string; email?: string; phone?: string; status: string; verified_at?: string }
export interface CustomerMembership { uuid: string; tenant_uuid: string; customer_uuid: string; status: string; source: string; expires_at?: string }
export interface CustomerLoginEvent { id: number; identity_provider?: string; event_type: string; ok: boolean; error_code?: string; created_at?: string }
export interface MiniAppEntry { uuid: string; entry_code: string; entry_type: string; channel?: string; campaign?: string; status: string; updated_at?: string }
export interface CustomerContact { uuid: string; customer_uuid: string; display_name: string; given_name?: string; family_name?: string; email?: string; phone?: string; status: 'active' | 'inactive' | 'temporary'; roles: string[]; tags: string[] }
export interface ContactIdentity { uuid: string; channel_dictionary_item_uuid: string; external_subject: string; status: string }
export interface ContactInput { display_name: string; given_name?: string; family_name?: string; email?: string; phone?: string; status: 'active' | 'inactive' | 'temporary'; roles?: string[]; tags?: string[]; creation_intent: 'explicit_create' | 'explicit_temporary' }
export interface ContactRepairInput { customer_uuid: string; type: 'person' | 'company'; contact_uuid?: string; primary_contact?: { display_name: string; email?: string; phone?: string } }
export interface ContactRepairResult { uuid: string; checksum: string; status: 'preview' | 'completed'; expires_at: string; trace_id?: string; primary_contact_uuid?: string; proposal: { customer_uuid: string; type: 'person' | 'company'; action: 'not_needed' | 'type_only' | 'reuse_contact' | 'create_contact'; expected_primary_contact_uuid: string; contact_uuid?: string; primary_contact?: { display_name: string; email?: string; phone?: string } } }

export interface CreateCustomerInput {
  type: 'person' | 'company'; display_name: string; primary_email?: string; primary_phone?: string;
  primary_contact?: { display_name: string; email?: string; phone?: string };
}
export interface CustomerImportRow {
  row: number; input: { type: string; display_name: string; primary_email: string; primary_phone: string; primary_contact_name: string; primary_contact_email: string; primary_contact_phone: string };
  status: 'ready' | 'invalid' | 'duplicate' | 'created' | 'failed'; error_code?: string; customer_uuid?: string; primary_contact_uuid?: string;
}
export interface CustomerImportResult {
  uuid: string; file_name: string; checksum: string; version: number; status: 'preview' | 'completed';
  expires_at: string; created_at: string; trace_id?: string; rows: CustomerImportRow[];
  total: number; ready: number; created: number; skipped: number; failed: number;
}

const unwrap = <T>(response: any): T => {
  const body = response?.data ?? response
  return (body?.payload ?? body) as T
}
const list = <T>(payload: any): { items: T[]; pagination: Pagination } => ({
  items: Array.isArray(payload?.items) ? payload.items : [],
  pagination: payload?.pagination || { total: 0, page: 1, page_size: 20 }
})

export function useCustomerService() {
  const api = useApiClient()
  const pageQuery = (page = 1, pageSize = 20) => `page=${page}&page_size=${pageSize}`
  return {
    async list(page = 1, pageSize = 20, q = '') {
      const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
      if (q.trim()) query.set('q', q.trim())
      return list<CustomerAccount>(unwrap(await api.get(`/admin/customers/accounts?${query}`)))
    },
    async get(customerUUID: string) { return unwrap<CustomerAccount>(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}`)) },
    async create(input: CreateCustomerInput) { return unwrap<CustomerAccount>(await api.post('/admin/customers/accounts', input, { useGlobalLoading: false, useGlobalError: false })) },
    async updateProfile(id: string, input: CustomerProfileUpdate, signal?: AbortSignal) { return unwrap<CustomerAccount>(await api.patch(`/admin/customers/accounts/${encodeURIComponent(id)}`, input, { signal, useGlobalLoading: false, useGlobalError: false })) },
    async previewImport(fileName: string, csv: string) { return unwrap<CustomerImportResult>(await api.post('/admin/customers/imports/preview', { file_name: fileName, csv }, { useGlobalLoading: false, useGlobalError: false })) },
    async getImport(id: string) { return unwrap<CustomerImportResult>(await api.get(`/admin/customers/imports/${encodeURIComponent(id)}`, { useGlobalLoading: false, useGlobalError: false })) },
    async commitImport(id: string, checksum: string) { return unwrap<CustomerImportResult>(await api.post(`/admin/customers/imports/${encodeURIComponent(id)}/commit`, { checksum, confirm: true }, { timeout: 120000, useGlobalLoading: false, useGlobalError: false })) },
    async listAuthIdentities(customerUUID: string) { return list<CustomerAuthIdentity>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/auth-identities`))) },
    async listMemberships(customerUUID: string) { return list<CustomerMembership>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/tenant-memberships`))) },
    async listLoginEvents(customerUUID: string, page = 1, pageSize = 20) { return list<CustomerLoginEvent>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/login-events?${pageQuery(page, pageSize)}`))) },
    async listMiniAppEntries(page = 1, pageSize = 20) { return list<MiniAppEntry>(unwrap(await api.get(`/admin/customers/mini-app-entries?${pageQuery(page, pageSize)}`))) },
    async listContacts(customerUUID: string) { return list<CustomerContact>(unwrap(await api.get(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts?page=1&page_size=100`))) },
    async setPrimaryContact(customerUUID: string, contactUUID: string, expectedPrimaryContactUUID: string) { return unwrap<{ customer_uuid: string; primary_contact_uuid: string; previous_primary_contact_uuid: string }>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}/primary`, { expected_primary_contact_uuid: expectedPrimaryContactUUID }, { useGlobalLoading: false, useGlobalError: false })) },
    async previewContactRepair(input: ContactRepairInput) { return unwrap<ContactRepairResult>(await api.post('/admin/customers/contact-repairs/preview', input, { useGlobalLoading: false, useGlobalError: false })) },
    async getContactRepair(id: string) { return unwrap<ContactRepairResult>(await api.get(`/admin/customers/contact-repairs/${encodeURIComponent(id)}`, { useGlobalLoading: false, useGlobalError: false })) },
    async commitContactRepair(id: string, checksum: string) { return unwrap<ContactRepairResult>(await api.post(`/admin/customers/contact-repairs/${encodeURIComponent(id)}/commit`, { checksum, confirm: true }, { useGlobalLoading: false, useGlobalError: false })) },
    async createContact(customerUUID: string, input: ContactInput) { return unwrap<CustomerContact>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts`, input)) },
    async updateContact(customerUUID: string, contactUUID: string, input: Partial<Omit<ContactInput, 'creation_intent'>>) { return unwrap<CustomerContact>(await api.patch(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}`, input)) },
    async listContactIdentities(customerUUID: string, contactUUID: string) { return list<ContactIdentity>(unwrap(await api.get(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}/identities`))) },
    async resolveContactIdentity(customerUUID: string, channelDictionaryItemUUID: string, externalSubject: string) { return unwrap<any>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts:resolve-identity`, { channel_dictionary_item_uuid: channelDictionaryItemUUID, external_subject: externalSubject })) },
    async bindContactIdentity(customerUUID: string, contactUUID: string, channelDictionaryItemUUID: string, externalSubject: string) { return unwrap<ContactIdentity>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}/identities`, { channel_dictionary_item_uuid: channelDictionaryItemUUID, external_subject: externalSubject })) }
  }
}
