import { readonly } from "vue";

export type SetupStatus = {
  configured: boolean;
  requires_login: boolean;
  restart_required: boolean;
};

let clientInflight: Promise<SetupStatus | null> | null = null;

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

    if (!force && cached.value && now - fetchedAt.value < ttlMs) {
      return cached.value;
    }

    if (import.meta.client && clientInflight) {
      return clientInflight;
    }

    const task = (async () => {
      const next = await fetchOnce(timeout);
      if (next) {
        cached.value = next;
        fetchedAt.value = Date.now();
      }
      return next;
    })();

    if (!import.meta.client) {
      return task;
    }

    clientInflight = task;
    try {
      return await task;
    } finally {
      clientInflight = null;
    }
  };

  const invalidate = () => {
    cached.value = null;
    fetchedAt.value = 0;
  };

  return {
    status: readonly(cached),
    load,
    invalidate,
  };
}
