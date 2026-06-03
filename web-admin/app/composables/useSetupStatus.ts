import { readonly } from "vue";

export type SetupStatus = {
  configured: boolean;
  requires_login: boolean;
  restart_required: boolean;
  version: string;
  saas_signup_enabled: boolean;
};

type SetupStatusCache = {
  data: SetupStatus | null;
  fetchedAt: number;
  inflight: Promise<SetupStatus | null> | null;
};

const getClientSetupStatusCache = (): SetupStatusCache => {
  const key = "__powerx_setup_status_cache__";
  const holder = globalThis as typeof globalThis & {
    [key]?: SetupStatusCache;
  };
  if (!holder[key]) {
    holder[key] = {
      data: null,
      fetchedAt: 0,
      inflight: null,
    };
  }
  return holder[key]!;
};

export function useSetupStatus() {
  const runtimeConfig = useRuntimeConfig();
  const apiBase = String(runtimeConfig.public?.apiBase || "/api").replace(/\/+$/, "");
  const setupStatusPath = `${apiBase}/admin/setup/status`;

  const cached = useState<SetupStatus | null>("setup:status:data", () => null);
  const fetchedAt = useState<number>("setup:status:ts", () => 0);

  const fetchOnce = async (timeout = 5000): Promise<SetupStatus | null> => {
    try {
      const resp: any = await $fetch(setupStatusPath, {
        method: "GET",
        timeout,
      });
      const payload = resp?.data ?? resp;
      return {
        configured: Boolean(payload?.configured),
        requires_login: Boolean(payload?.requires_login),
        restart_required: Boolean(payload?.restart_required),
        version: String(payload?.version || "").trim(),
        saas_signup_enabled: Boolean(payload?.saas_signup_enabled),
      };
    } catch {
      return null;
    }
  };

  const load = async (opts?: {
    force?: boolean;
    timeout?: number;
    ttlMs?: number;
  }): Promise<SetupStatus | null> => {
    const force = Boolean(opts?.force);
    const timeout = Number(opts?.timeout || 5000);
    const ttlMs = Number(opts?.ttlMs || 5000);
    const now = Date.now();
    const clientCache = import.meta.client ? getClientSetupStatusCache() : null;

    if (!force && cached.value && now - fetchedAt.value < ttlMs) {
      return cached.value;
    }

    if (
      !force &&
      clientCache?.data &&
      now - clientCache.fetchedAt < ttlMs
    ) {
      cached.value = clientCache.data;
      fetchedAt.value = clientCache.fetchedAt;
      return clientCache.data;
    }

    if (clientCache?.inflight) {
      return clientCache.inflight;
    }

    const task = (async () => {
      const next = await fetchOnce(timeout);
      if (next) {
        cached.value = next;
        fetchedAt.value = Date.now();
        if (clientCache) {
          clientCache.data = next;
          clientCache.fetchedAt = fetchedAt.value;
        }
      }
      return next;
    })();

    if (!import.meta.client) {
      return task;
    }

    clientCache!.inflight = task;
    try {
      return await task;
    } finally {
      if (clientCache!.inflight === task) {
        clientCache!.inflight = null;
      }
    }
  };

  const invalidate = () => {
    cached.value = null;
    fetchedAt.value = 0;
    if (import.meta.client) {
      const clientCache = getClientSetupStatusCache();
      clientCache.data = null;
      clientCache.fetchedAt = 0;
      clientCache.inflight = null;
    }
  };

  return {
    status: readonly(cached),
    load,
    invalidate,
  };
}
