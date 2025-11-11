import { defineStore } from "pinia";
import { CostQuotaService } from "~/composables/api/services/costQuotaService";

export interface QuotaEntry {
  providerId: string;
  limit: number;
  usage: number;
  status: string;
  anomaly?: Record<string, any>;
  enforcement?: {
    history?: Record<string, any>[];
    [key: string]: any;
  };
}

interface CostQuotaState {
  loading: boolean;
  error: string | null;
  quotas: QuotaEntry[];
  lastFetchedAt: string | null;
}

export const useCostQuotaStore = defineStore("cost-quota", {
  state: (): CostQuotaState => ({
    loading: false,
    error: null,
    quotas: [],
    lastFetchedAt: null,
  }),
  actions: {
    async fetchQuotas(env: string, tenantId: string) {
      this.loading = true;
      this.error = null;
      try {
        const snapshot = await CostQuotaService.getQuotaSnapshot({
          env,
          tenantId,
        });
        this.quotas = snapshot?.quotas
          ? snapshot.quotas.map((quota) => ({
              ...quota,
              enforcement: {
                ...(quota.enforcement || {}),
                history: normalizeHistory(quota.enforcement),
              },
            }))
          : [];
        this.lastFetchedAt = new Date().toISOString();
      } catch (err: any) {
        this.error = err?.message || "Failed to load quotas";
        throw err;
      } finally {
        this.loading = false;
      }
    },
    async enforceAction(payload: {
      env: string;
      tenantId: string;
      providerId?: string;
      action: string;
      reason?: string;
      ticketId?: string;
      requestedBy?: string;
    }) {
      await CostQuotaService.enforceAction(payload);
    },
  },
});

function normalizeHistory(
  enforcement?: Record<string, any>
): Record<string, any>[] {
  if (!enforcement || !Array.isArray(enforcement.history)) {
    return [];
  }
  return enforcement.history.map((item: any) => ({
    action: item?.action || enforcement.action || "",
    reason: item?.reason || enforcement.reason || "",
    ticketId: item?.ticketId || enforcement.ticketId || "",
    requestedBy: item?.requestedBy || enforcement.requestedBy || "",
    timestamp: item?.timestamp || enforcement.updatedAt || "",
  }));
}
