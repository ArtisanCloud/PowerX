import { useApiClient } from "../index";
import type { ProtocolBinding } from "./capabilityRegistryService";

export interface PlatformCapabilityDebugExamples {
  tenantInvocationCurl: string;
  tenantInvocationPayload?: Record<string, any>;
}

export interface PlatformCapability {
  capabilityId: string;
  title: string;
  description?: string;
  module: string;
  source: string;
  pluginId: string;
  pluginVersion: string;
  docs: string[];
  capabilitiesHash: string;
  preferredProtocol?: string;
  protocols: ProtocolBinding[];
  debugExamples: PlatformCapabilityDebugExamples;
}

export interface PlatformCapabilityModule {
  module: string;
  displayName?: string;
  description?: string;
  capabilityCount: number;
  protocolChannels: string[];
  capabilities: PlatformCapability[];
}

export interface PlatformCapabilityModuleList {
  generatedAt?: string;
  totalModules: number;
  totalCapabilities: number;
  modules: PlatformCapabilityModule[];
}

type PlatformCapabilityDebugExamplesAPI = {
  tenant_invocation_curl?: string;
  tenant_invocation_payload?: Record<string, any>;
};

type PlatformCapabilityAPI = {
  capability_id: string;
  title: string;
  description?: string;
  module?: string;
  source?: string;
  plugin_id?: string;
  plugin_version?: string;
  docs?: string[];
  capabilities_hash: string;
  preferred_protocol?: string;
  protocols?: Array<{
    channel: string;
    endpoint?: string;
    schema_ref?: string;
    method?: string;
    rpc?: string;
    tool_ref?: string;
    auth_type?: string;
    health_state?: string;
    last_checked_at?: string;
  }>;
  debug_examples?: PlatformCapabilityDebugExamplesAPI;
};

type PlatformCapabilityModuleAPI = {
  module?: string;
  display_name?: string;
  description?: string;
  capability_count?: number;
  protocol_channels?: string[];
  capabilities?: PlatformCapabilityAPI[];
};

type PlatformCapabilityModuleListAPI = {
  generated_at?: string;
  total_modules?: number;
  total_capabilities?: number;
  modules?: PlatformCapabilityModuleAPI[];
};

type PlatformCapabilityModuleResponseAPI = {
  generated_at?: string;
  module?: PlatformCapabilityModuleAPI;
};

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const mapProtocolBinding = (
  binding: PlatformCapabilityAPI["protocols"][number]
): ProtocolBinding => ({
  channel: binding.channel,
  endpoint: binding.endpoint,
  schemaRef: binding.schema_ref,
  method: binding.method,
  rpc: binding.rpc,
  toolRef: binding.tool_ref,
  authType: binding.auth_type,
  healthState: binding.health_state,
  lastCheckedAt: binding.last_checked_at,
});

const mapCapability = (
  capability: PlatformCapabilityAPI
): PlatformCapability => ({
  capabilityId: capability.capability_id,
  title: capability.title,
  description: capability.description,
  module: capability.module || "corex",
  source: capability.source || "corex",
  pluginId: capability.plugin_id || "",
  pluginVersion: capability.plugin_version || "",
  docs: capability.docs || [],
  capabilitiesHash: capability.capabilities_hash,
  preferredProtocol: capability.preferred_protocol,
  protocols: (capability.protocols || []).map(mapProtocolBinding),
  debugExamples: {
    tenantInvocationCurl: capability.debug_examples?.tenant_invocation_curl || "",
    tenantInvocationPayload:
      capability.debug_examples?.tenant_invocation_payload || undefined,
  },
});

const mapModule = (
  module: PlatformCapabilityModuleAPI
): PlatformCapabilityModule => ({
  module: module.module || "corex",
  displayName: module.display_name || module.module || "CoreX",
  description: module.description,
  capabilityCount:
    module.capability_count ?? module.capabilities?.length ?? 0,
  protocolChannels: module.protocol_channels || [],
  capabilities: (module.capabilities || []).map(mapCapability),
});

export class PlatformCapabilityService {
  static async listModules(
    params: {
      module?: string;
      source?: "all" | "corex" | "plugin";
      page?: number;
      pageSize?: number;
    } = {}
  ): Promise<PlatformCapabilityModuleList> {
    const client = useApiClient();
    const query: Record<string, string> = {};
    if (params.module) query.module = params.module;
    if (params.source) query.source = params.source;
    if (params.page) query.page = String(params.page);
    if (params.pageSize) query.page_size = String(params.pageSize);
    const response = await client.get<PlatformCapabilityModuleListAPI>(
      "/admin/platform-capabilities",
      {
        params: Object.keys(query).length ? query : undefined,
        useGlobalLoading: false,
      }
    );
    const data = unwrap<PlatformCapabilityModuleListAPI>(response) || {};
    const modules = Array.isArray(data.modules)
      ? data.modules.map(mapModule)
      : [];
    return {
      generatedAt: data.generated_at,
      totalModules: data.total_modules ?? modules.length,
      totalCapabilities: data.total_capabilities ?? 0,
      modules,
    };
  }

  static async getModule(
    moduleKey: string,
    params: { source?: "all" | "corex" | "plugin" } = {}
  ): Promise<PlatformCapabilityModule | null> {
    const key = moduleKey?.trim();
    if (!key) {
      return null;
    }
    const client = useApiClient();
    const response = await client.get<PlatformCapabilityModuleResponseAPI>(
      `/admin/platform-capabilities/${encodeURIComponent(key)}`,
      {
        params: params.source ? { source: params.source } : undefined,
        useGlobalLoading: false,
      }
    );
    const data = unwrap<PlatformCapabilityModuleResponseAPI>(response) || {};
    if (!data.module) {
      return null;
    }
    return mapModule(data.module);
  }
}
