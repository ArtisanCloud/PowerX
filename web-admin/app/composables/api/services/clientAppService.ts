import { useApiClient } from '../index'

export interface ClientAppConnection { uuid: string; provider: string; platform: string; app_id: string; secret_configured: boolean }
export interface ClientApplication { uuid: string; name: string; kind: string; status: 'active' | 'disabled'; version: number; plugin_ids: string[]; connections: ClientAppConnection[]; updated_at: string }
export interface LegacyClientApplication { platform: string; app_id: string; name: string; secret_configured: boolean }
export interface ClientAppCatalog { kinds: { id: string; supported: boolean }[]; providers: { id: string; supported: boolean }[]; plugin_ids: string[]; legacy_applications: LegacyClientApplication[] }
export interface ClientApplicationInput { expected_version: number; name: string; kind: string; status: 'active' | 'disabled'; plugin_ids: string[]; connections: { uuid?: string; provider: string; app_id: string; app_secret?: string }[] }
const unwrap = <T>(response: any): T => {
  const body = response?.data ?? response
  return (body?.payload ?? body) as T
}
export function useClientAppService() {
  const api = useApiClient()
  const options = (signal?: AbortSignal) => ({ signal, useGlobalLoading: false, useGlobalError: false })
  const base = '/admin/customers/client-apps'
  return {
    async catalog(signal?: AbortSignal) { return unwrap<ClientAppCatalog>(await api.get(`${base}/catalog`, options(signal))) },
    async list(query: { page: number; kind: string; provider: string; q: string }, signal?: AbortSignal) {
      const params = new URLSearchParams({ page: String(query.page), page_size: '20', kind: query.kind, provider: query.provider, q: query.q })
      return unwrap<{ items: ClientApplication[]; pagination: { total: number; page: number; page_size: number } }>(await api.get(`${base}?${params}`, options(signal)))
    },
    async get(id: string, signal?: AbortSignal) { return unwrap<ClientApplication>(await api.get(`${base}/${encodeURIComponent(id)}`, options(signal))) },
    async save(id: string, input: ClientApplicationInput, signal?: AbortSignal) {
      return unwrap<ClientApplication>(id ? await api.patch(`${base}/${encodeURIComponent(id)}`, input, options(signal)) : await api.post(base, input, options(signal)))
    },
    async importLegacy(app: LegacyClientApplication, signal?: AbortSignal) { return unwrap<ClientApplication>(await api.post(`${base}/import`, { platform: app.platform, app_id: app.app_id }, options(signal))) }
  }
}
