import { resolveTenantUUIDForRequest } from "~/utils/tenant-context";

export type KnowledgeSourceProvider = "notion" | "feishu";
export type KnowledgeCredentialAuthType = "oauth" | "token";
export type KnowledgeSyncMode = "incremental" | "full_then_incremental";
export type KnowledgeSyncJobStatus = "active" | "paused" | "failed";

export interface TenantSourceCredential {
  id: string;
  tenantUuid: string;
  provider: KnowledgeSourceProvider;
  authType: KnowledgeCredentialAuthType;
  label: string;
  status: "active" | "revoked" | "pending";
  maskedHint?: string;
  createdAt: string;
  updatedAt: string;
}

export interface TenantConnectorInstance {
  id: string;
  tenantUuid: string;
  provider: KnowledgeSourceProvider;
  credentialId: string;
  status: "active" | "paused" | "error";
  createdAt: string;
  updatedAt: string;
}

export interface SpaceSyncJob {
  id: string;
  tenantUuid: string;
  spaceId: string;
  provider: KnowledgeSourceProvider;
  connectorId: string;
  syncMode: KnowledgeSyncMode;
  schedule: string;
  scope: Record<string, any>;
  status: KnowledgeSyncJobStatus;
  lastRunAt?: string;
  lastOkAt?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SpaceSourceConnectionView {
  provider: KnowledgeSourceProvider;
  credential: TenantSourceCredential;
  connector: TenantConnectorInstance;
  jobs: SpaceSyncJob[];
}

const storageKey = (tenantUuid: string) => `px_knowledge_sources_v1:${tenantUuid}`;

interface StorageShape {
  credentials: TenantSourceCredential[];
  connectors: TenantConnectorInstance[];
  jobs: SpaceSyncJob[];
}

const nowISO = () => new Date().toISOString();

const newId = () =>
  typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `id-${Math.random().toString(16).slice(2)}-${Date.now()}`;

function loadTenantStore(tenantUuid: string): StorageShape {
  if (!process.client) return { credentials: [], connectors: [], jobs: [] };
  const raw = localStorage.getItem(storageKey(tenantUuid));
  if (!raw) return { credentials: [], connectors: [], jobs: [] };
  try {
    const parsed = JSON.parse(raw);
    return {
      credentials: Array.isArray(parsed?.credentials) ? parsed.credentials : [],
      connectors: Array.isArray(parsed?.connectors) ? parsed.connectors : [],
      jobs: Array.isArray(parsed?.jobs) ? parsed.jobs : [],
    };
  } catch {
    return { credentials: [], connectors: [], jobs: [] };
  }
}

function saveTenantStore(tenantUuid: string, data: StorageShape) {
  if (!process.client) return;
  localStorage.setItem(storageKey(tenantUuid), JSON.stringify(data));
}

export function useKnowledgeSpaceSources() {
  const resolveTenant = () => {
    const tenantUuid = String(resolveTenantUUIDForRequest() || "").trim();
    if (!tenantUuid) throw new Error("缺少租户上下文（TENANT UUID）");
    return tenantUuid;
  };

  const listTenantCredentials = (provider?: KnowledgeSourceProvider) => {
    const tenantUuid = resolveTenant();
    const store = loadTenantStore(tenantUuid);
    return store.credentials.filter((c) => (provider ? c.provider === provider : true));
  };

  const upsertTenantCredential = (input: {
    provider: KnowledgeSourceProvider;
    authType: KnowledgeCredentialAuthType;
    label: string;
    maskedHint?: string;
  }) => {
    const tenantUuid = resolveTenant();
    const store = loadTenantStore(tenantUuid);
    const id = newId();
    const ts = nowISO();
    const row: TenantSourceCredential = {
      id,
      tenantUuid,
      provider: input.provider,
      authType: input.authType,
      label: input.label,
      status: "active",
      maskedHint: input.maskedHint,
      createdAt: ts,
      updatedAt: ts,
    };
    store.credentials.unshift(row);
    saveTenantStore(tenantUuid, store);
    return row;
  };

  const ensureConnector = (input: { provider: KnowledgeSourceProvider; credentialId: string }) => {
    const tenantUuid = resolveTenant();
    const store = loadTenantStore(tenantUuid);
    const existing = store.connectors.find(
      (c) => c.provider === input.provider && c.credentialId === input.credentialId,
    );
    if (existing) return existing;
    const ts = nowISO();
    const row: TenantConnectorInstance = {
      id: newId(),
      tenantUuid,
      provider: input.provider,
      credentialId: input.credentialId,
      status: "active",
      createdAt: ts,
      updatedAt: ts,
    };
    store.connectors.unshift(row);
    saveTenantStore(tenantUuid, store);
    return row;
  };

  const createSpaceSyncJob = (input: {
    spaceId: string;
    provider: KnowledgeSourceProvider;
    connectorId: string;
    syncMode: KnowledgeSyncMode;
    schedule: string;
    scope: Record<string, any>;
  }) => {
    const tenantUuid = resolveTenant();
    const store = loadTenantStore(tenantUuid);
    const ts = nowISO();
    const row: SpaceSyncJob = {
      id: newId(),
      tenantUuid,
      spaceId: input.spaceId,
      provider: input.provider,
      connectorId: input.connectorId,
      syncMode: input.syncMode,
      schedule: input.schedule,
      scope: input.scope || {},
      status: "active",
      createdAt: ts,
      updatedAt: ts,
    };
    store.jobs.unshift(row);
    saveTenantStore(tenantUuid, store);
    return row;
  };

  const listSpaceConnections = (spaceId: string): SpaceSourceConnectionView[] => {
    const tenantUuid = resolveTenant();
    const store = loadTenantStore(tenantUuid);
    const jobs = store.jobs.filter((j) => j.spaceId === spaceId);
    const byConnector = new Map<string, SpaceSyncJob[]>();
    for (const job of jobs) {
      const list = byConnector.get(job.connectorId) || [];
      list.push(job);
      byConnector.set(job.connectorId, list);
    }
    const out: SpaceSourceConnectionView[] = [];
    for (const [connectorId, connectorJobs] of byConnector.entries()) {
      const connector = store.connectors.find((c) => c.id === connectorId);
      if (!connector) continue;
      const credential = store.credentials.find((c) => c.id === connector.credentialId);
      if (!credential) continue;
      out.push({
        provider: connector.provider,
        credential,
        connector,
        jobs: connectorJobs,
      });
    }
    return out;
  };

  return {
    listTenantCredentials,
    upsertTenantCredential,
    ensureConnector,
    createSpaceSyncJob,
    listSpaceConnections,
  };
}

