import { useApiClient } from "../index";
import type { PluginInstallOrigin, PluginProviderIdentity } from "./adminPluginsService";

export interface MarketPlugin {
  id: string;
  name: string;
  summary: string;
  categories: string[];
  owner_dev_id: string;
  installation: {
    status: 'installed' | 'not_installed' | 'source_conflict' | 'unknown';
    version?: string;
    origin: PluginInstallOrigin;
    provider?: PluginProviderIdentity;
    installed_provider?: PluginProviderIdentity;
    revision?: string;
    can_replace?: boolean;
    reason?: string;
  };
}
export interface MarketVersion {
  id: string;
  plugin_id: string;
  semver: string;
  channel: string;
  min_core: string;
  os: string;
  arch: string;
  sha256: string;
  size: number;
}
export interface MarketPage<T> { items: T[]; total: number; page: number; size: number }
export interface MarketConnection {
  configured: boolean;
  download_ready: boolean;
  os: string;
  arch: string;
  core_version: string;
  source_id: string;
  instance?: {
    state: string;
    id?: string;
    last_heartbeat?: string;
    pending_reports: number;
    blocked_reports: number;
    unconfirmed_installs: number;
    error_code?: string;
  };
}
export type MarketCatalog = MarketPage<MarketPlugin> & { connection: MarketConnection; inventory_ready: boolean };

export function useMarketplaceService() {
  const api = useApiClient();
  const base = "/admin/plugins/marketplace";
  const unwrap = <T>(response: T | { data: T }): T =>
    response && typeof response === "object" && "data" in response ? response.data : response as T;
  return {
    catalog: async (page = 1, installation = "all") => {
      const result = unwrap<MarketCatalog>(await api.get(`${base}/catalog`, { params: { page, size: 12, installation } }));
      // 旧宿主尚未加载来源契约时保持可查看，禁止猜测为未安装。
      return { ...result, inventory_ready: result.inventory_ready === true, items: result.items.map(plugin => ({
        ...plugin, installation: plugin.installation || { status: 'unknown' as const, origin: { type: 'unknown' as const } },
      })) };
    },
    versions: async (id: string, page = 1) => unwrap<MarketPage<MarketVersion>>(await api.get(
      `${base}/catalog/${encodeURIComponent(id)}/versions`, { params: { page, size: 20 } },
    )),
    install: async (id: string, version: string, options: { enable: boolean; replace?: boolean; expected_revision?: string; confirm_legacy_provider?: boolean }) => unwrap<{ installed: { id: string; version: string; state: string; revision: string; provider: PluginProviderIdentity; install_origin: PluginInstallOrigin }; enabled: boolean }>(await api.post(
      `${base}/install`, { plugin_id: id, version, ...options }, { timeout: 120000 },
    )),
  };
}
