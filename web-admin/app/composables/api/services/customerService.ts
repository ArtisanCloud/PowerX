import { useApiClient } from '../index'

export interface Pagination { total: number; page: number; page_size: number }
export interface CustomerAccount { uuid: string; type: 'person' | 'company' | ''; primary_contact_uuid?: string; display_name?: string; nickname?: string; primary_email?: string; primary_phone?: string; status: string; member_status?: string; member_source?: string; membership_uuid?: string; created_at?: string; updated_at?: string }
export interface CustomerAuthIdentity { uuid: string; provider: string; provider_subject?: string; email?: string; phone?: string; status: string; verified_at?: string }
export interface CustomerMembership { uuid: string; tenant_uuid: string; customer_uuid: string; status: string; source: string; expires_at?: string }
export interface CustomerLoginEvent { id: number; identity_provider?: string; event_type: string; ok: boolean; error_code?: string; created_at?: string }
export interface MiniAppEntry { uuid: string; entry_code: string; entry_type: string; channel?: string; campaign?: string; status: string; updated_at?: string }
export interface CustomerContact { uuid: string; customer_uuid: string; display_name: string; given_name?: string; family_name?: string; email?: string; phone?: string; status: 'active' | 'inactive' | 'temporary'; roles: string[]; tags: string[] }
export interface ContactIdentity { uuid: string; channel_dictionary_item_uuid: string; external_subject: string; status: string }
export interface ContactInput { display_name: string; given_name?: string; family_name?: string; email?: string; phone?: string; status: 'active' | 'inactive' | 'temporary'; roles?: string[]; tags?: string[]; creation_intent: 'explicit_create' | 'explicit_temporary' }

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
    async listAuthIdentities(customerUUID: string) { return list<CustomerAuthIdentity>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/auth-identities`))) },
    async listMemberships(customerUUID: string) { return list<CustomerMembership>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/tenant-memberships`))) },
    async listLoginEvents(customerUUID: string, page = 1, pageSize = 20) { return list<CustomerLoginEvent>(unwrap(await api.get(`/admin/customers/accounts/${encodeURIComponent(customerUUID)}/login-events?${pageQuery(page, pageSize)}`))) },
    async listMiniAppEntries(page = 1, pageSize = 20) { return list<MiniAppEntry>(unwrap(await api.get(`/admin/customers/mini-app-entries?${pageQuery(page, pageSize)}`))) },
    async listContacts(customerUUID: string) { return list<CustomerContact>(unwrap(await api.get(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts?page=1&page_size=100`))) },
    async createContact(customerUUID: string, input: ContactInput) { return unwrap<CustomerContact>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts`, input)) },
    async updateContact(customerUUID: string, contactUUID: string, input: Partial<Omit<ContactInput, 'creation_intent'>>) { return unwrap<CustomerContact>(await api.patch(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}`, input)) },
    async listContactIdentities(customerUUID: string, contactUUID: string) { return list<ContactIdentity>(unwrap(await api.get(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}/identities`))) },
    async resolveContactIdentity(customerUUID: string, channelDictionaryItemUUID: string, externalSubject: string) { return unwrap<any>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts:resolve-identity`, { channel_dictionary_item_uuid: channelDictionaryItemUUID, external_subject: externalSubject })) },
    async bindContactIdentity(customerUUID: string, contactUUID: string, channelDictionaryItemUUID: string, externalSubject: string) { return unwrap<ContactIdentity>(await api.post(`/admin/customers/${encodeURIComponent(customerUUID)}/contacts/${encodeURIComponent(contactUUID)}/identities`, { channel_dictionary_item_uuid: channelDictionaryItemUUID, external_subject: externalSubject })) }
  }
}
